package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/w-run/mimi-router/relay/model"
)

// ToOpenAIRequest 把入站 AnthropicRequest 转成内部 GeneralOpenAIRequest。
// 失败原因集中在 ErrInvalidRequest；调用方把它包成 400 + invalid_request_error。
func ToOpenAIRequest(req *AnthropicRequest) (*model.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: empty body", ErrInvalidRequest)
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("%w: model is required", ErrInvalidRequest)
	}
	if req.MaxTokens <= 0 {
		// Anthropic 必填。给个合理兜底，避免 OpenAI 渠道再报 422。
		req.MaxTokens = 4096
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("%w: messages is required and must not be empty", ErrInvalidRequest)
	}

	out := &model.GeneralOpenAIRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		TopK:        ifZero(req.TopK, 0),
	}

	// stop_sequences -> Stop
	if len(req.StopSequences) > 0 {
		if len(req.StopSequences) == 1 {
			out.Stop = req.StopSequences[0]
		} else {
			out.Stop = req.StopSequences
		}
	}

	// system -> messages[0] role=system
	sysMsgs, err := convertSystem(req.System)
	if err != nil {
		return nil, err
	}

	// messages
	msgs := make([]model.Message, 0, len(req.Messages)+len(sysMsgs))
	msgs = append(msgs, sysMsgs...)
	for i, m := range req.Messages {
		converted, err := convertMessage(m, i)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, converted...)
	}
	out.Messages = msgs

	// tools
	if len(req.Tools) > 0 {
		tools := make([]model.Tool, 0, len(req.Tools))
		for _, t := range req.Tools {
			if strings.TrimSpace(t.Name) == "" {
				return nil, fmt.Errorf("%w: tools[].name is required", ErrInvalidRequest)
			}
			tools = append(tools, model.Tool{
				Type: "function",
				Function: model.Function{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.InputSchema,
				},
			})
		}
		out.Tools = tools
	}

	// tool_choice
	if req.ToolChoice != nil {
		tc, err := convertToolChoice(req.ToolChoice)
		if err != nil {
			return nil, err
		}
		out.ToolChoice = tc
	}

	return out, nil
}

// ---------- helpers ----------

func ifZero(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// convertSystem 把 system 字段（string | []SystemBlock | nil）转成 OpenAI
// 风格的 system 消息。空时返回 nil。
func convertSystem(sys any) ([]model.Message, error) {
	if sys == nil {
		return nil, nil
	}
	switch v := sys.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil, nil
		}
		return []model.Message{{Role: "system", Content: s}}, nil
	case []any:
		var b strings.Builder
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if m["type"] == ContentTypeText {
				if t, ok := m["text"].(string); ok {
					if b.Len() > 0 {
						b.WriteString("\n\n")
					}
					b.WriteString(t)
				}
			}
		}
		if b.Len() == 0 {
			return nil, nil
		}
		return []model.Message{{Role: "system", Content: b.String()}}, nil
	default:
		// 兼容 raw json 解析后类型异常；忽略而非 400
		return nil, nil
	}
}

// convertMessage 把单条 AnthropicMessage 转成 0~N 条 model.Message。
// 一条 Anthropic 消息可能展开成多条 OpenAI 消息（典型场景：
// user 消息里有 tool_result + text，需要拆成 tool 消息 + user 消息）。
func convertMessage(m AnthropicMessage, idx int) ([]model.Message, error) {
	role := strings.ToLower(strings.TrimSpace(m.Role))
	if role != "user" && role != "assistant" && role != "system" {
		return nil, fmt.Errorf("%w: messages[%d].role=%q invalid", ErrInvalidRequest, idx, m.Role)
	}

	// 简化形式：string
	if s, ok := m.Content.(string); ok {
		return []model.Message{{Role: role, Content: s}}, nil
	}

	// 列表形式
	rawList, ok := m.Content.([]any)
	if !ok {
		// raw json 落地后 Content 偶尔是 []AnthropicContentBlock，但 GIN 解析时
		// 通常是 []any。这里兜底：再尝试一次。
		return nil, fmt.Errorf("%w: messages[%d].content must be string or array", ErrInvalidRequest, idx)
	}

	// 先把每个 block 归类
	type bucket struct {
		text          []string
		images        []map[string]any
		toolCalls     []model.Tool
		toolResults   []model.Message
	}
	var b bucket
	bucketIndex := -1
	toolCallIndexMap := make(map[int]int) // rawIndex -> toolCallIndex

	for rawIdx, item := range rawList {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t, _ := obj["type"].(string)
		switch t {
		case ContentTypeText:
			if txt, ok := obj["text"].(string); ok {
				b.text = append(b.text, txt)
			}
		case ContentTypeImage:
			src, _ := obj["source"].(map[string]any)
			if src == nil {
				continue
			}
			mt, _ := src["media_type"].(string)
			data, _ := src["data"].(string)
			if data == "" {
				continue
			}
			if mt == "" {
				mt = "image/png"
			}
			b.images = append(b.images, map[string]any{
				"type": model.ContentTypeImageURL,
				"image_url": map[string]any{
					"url": fmt.Sprintf("data:%s;base64,%s", mt, data),
				},
			})
		case ContentTypeToolUse:
			bucketIndex++
			toolCallIndexMap[rawIdx] = bucketIndex
			id, _ := obj["id"].(string)
			name, _ := obj["name"].(string)
			input, _ := obj["input"].(map[string]any)
			if id == "" || name == "" {
				return nil, fmt.Errorf("%w: tool_use missing id or name", ErrInvalidRequest)
			}
			// OpenAI tool_calls.function.arguments 期望 string。
			// 没有 input 时给 "{}"，避免下游反序列化失败。
			var argsStr string
			if input == nil {
				argsStr = "{}"
			} else {
				b, _ := json.Marshal(input)
				argsStr = string(b)
			}
			b.toolCalls = append(b.toolCalls, model.Tool{
				Id:   id,
				Type: "function",
				Function: model.Function{
					Name:      name,
					Arguments: argsStr,
				},
			})
		case ContentTypeToolResult:
			tuID, _ := obj["tool_use_id"].(string)
			if tuID == "" {
				return nil, fmt.Errorf("%w: tool_result missing tool_use_id", ErrInvalidRequest)
			}
			content := extractToolResultContent(obj["content"])
			b.toolResults = append(b.toolResults, model.Message{
				Role:       "tool",
				Content:    content,
				ToolCallId: tuID,
			})
		}
	}

	// 拼装
	out := make([]model.Message, 0, 2)

	// assistant 消息（带 text + tool_calls）
	if role == "assistant" {
		// 构造 content：优先 text 数组
		var content any
		if len(b.text) > 0 {
			if len(b.text) == 1 {
				content = b.text[0]
			} else {
				arr := make([]map[string]any, 0, len(b.text))
				for _, t := range b.text {
					arr = append(arr, map[string]any{
						"type": model.ContentTypeText,
						"text": t,
					})
				}
				content = arr
			}
		}
		msg := model.Message{Role: "assistant", Content: content}
		if len(b.toolCalls) > 0 {
			msg.ToolCalls = b.toolCalls
		}
		out = append(out, msg)
		return out, nil
	}

	// user/system 消息：text + image
	if len(b.text) > 0 || len(b.images) > 0 {
		var content any
		if len(b.images) == 0 && len(b.text) == 1 {
			content = b.text[0]
		} else {
			arr := make([]map[string]any, 0, len(b.text)+len(b.images))
			for _, t := range b.text {
				arr = append(arr, map[string]any{
					"type": model.ContentTypeText,
					"text": t,
				})
			}
			arr = append(arr, b.images...)
			content = arr
		}
		out = append(out, model.Message{Role: role, Content: content})
	}

	// tool_result 消息：放在 user 消息之后，逐条 role=tool
	for _, tm := range b.toolResults {
		out = append(out, tm)
	}

	if len(out) == 0 {
		// 空消息兜底
		out = append(out, model.Message{Role: role, Content: ""})
	}
	return out, nil
}

// extractToolResultContent 把 tool_result.content 归一为 string。
// string 直接返回；[]any 时把 text block 拼起来；其他原样 json 序列化。
func extractToolResultContent(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if obj["type"] == ContentTypeText {
				if t, ok := obj["text"].(string); ok {
					if b.Len() > 0 {
						b.WriteString("\n")
					}
					b.WriteString(t)
				}
			}
		}
		return b.String()
	default:
		// 上游允许复杂对象；序列化兜底
		if c == nil {
			return ""
		}
		bb, err := json.Marshal(c)
		if err != nil {
			return fmt.Sprintf("%v", c)
		}
		return string(bb)
	}
}

// convertToolChoice 把 Anthropic 工具选择策略转成 OpenAI 形式。
//   - "auto"        -> "auto"
//   - "any"         -> "required"（强制至少一个 tool call）
//   - {"type":"tool","name":"x"} -> {"type":"function","function":{"name":"x"}}
//   - nil           -> nil
func convertToolChoice(tc any) (any, error) {
	if tc == nil {
		return nil, nil
	}
	switch v := tc.(type) {
	case string:
		switch v {
		case "auto":
			return "auto", nil
		case "any":
			return "required", nil
		case "none":
			return "none", nil
		default:
			return nil, fmt.Errorf("%w: tool_choice=%q invalid", ErrInvalidRequest, v)
		}
	case map[string]any:
		t, _ := v["type"].(string)
		if t != "tool" {
			return nil, fmt.Errorf("%w: tool_choice.type=%q invalid", ErrInvalidRequest, t)
		}
		name, _ := v["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("%w: tool_choice.name is required", ErrInvalidRequest)
		}
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": name},
		}, nil
	default:
		return nil, fmt.Errorf("%w: tool_choice must be string or object", ErrInvalidRequest)
	}
}

// ErrInvalidRequest 标记入参语义错误（HTTP 400 + invalid_request_error）。
var ErrInvalidRequest = errors.New("invalid_request_error")
