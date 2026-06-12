package anthropic

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/w-run/mimi-router/relay/adaptor/openai"
	"github.com/w-run/mimi-router/relay/model"
)

// FromOpenAITextResponse 把 OpenAI 风格非流式响应转成 Anthropic 响应。
// 输入兼容 openai.TextResponse（带 created/model）和 openai.SlimTextResponse
// （只有 choices/usage），调用方按实际取到哪种结构用对应转换函数。
func FromOpenAITextResponse(resp *openai.TextResponse) *AnthropicResponse {
	if resp == nil {
		return nil
	}
	out := &AnthropicResponse{
		ID:           "msg_" + randHex(24),
		Type:         "message",
		Role:         "assistant",
		Model:        resp.Model,
		Content:      []AnthropicContentBlock{},
		StopReason:   stopReasonFromOpenAI(firstFinishReason(resp.Choices)),
		StopSequence: nil,
		Usage: AnthropicUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}
	if len(resp.Choices) > 0 {
		out.Content = messageContentToAnthropic(resp.Choices[0].Message)
	}
	return out
}

// FromOpenAISlimTextResponse 同上，但输入是 SlimTextResponse（无 created/model）。
func FromOpenAISlimTextResponse(resp *openai.SlimTextResponse) *AnthropicResponse {
	if resp == nil {
		return nil
	}
	out := &AnthropicResponse{
		ID:           "msg_" + randHex(24),
		Type:         "message",
		Role:         "assistant",
		Content:      []AnthropicContentBlock{},
		StopReason:   stopReasonFromOpenAI(firstFinishReason(resp.Choices)),
		StopSequence: nil,
	}
	if len(resp.Choices) > 0 {
		out.Content = messageContentToAnthropic(resp.Choices[0].Message)
	}
	out.Usage = AnthropicUsage{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}
	return out
}

func firstFinishReason(choices []openai.TextResponseChoice) string {
	if len(choices) == 0 {
		return ""
	}
	return choices[0].FinishReason
}

// messageContentToAnthropic 把 OpenAI message.content + tool_calls 拆成
// Anthropic content blocks：先 text，然后按顺序 tool_use。
func messageContentToAnthropic(m model.Message) []AnthropicContentBlock {
	out := make([]AnthropicContentBlock, 0, 2)

	// 1) text
	if s := m.StringContent(); s != "" {
		out = append(out, AnthropicContentBlock{
			Type: ContentTypeText,
			Text: s,
		})
	} else {
		// StringContent() 拿不到时，尝试用 ParseContent 提取
		parsed := m.ParseContent()
		for _, p := range parsed {
			if p.Type == model.ContentTypeText && p.Text != "" {
				out = append(out, AnthropicContentBlock{
					Type: ContentTypeText,
					Text: p.Text,
				})
			}
		}
	}

	// 2) tool_calls
	for _, tc := range m.ToolCalls {
		if tc.Type != "" && tc.Type != "function" {
			continue
		}
		input := map[string]any{}
		if tc.Function.Arguments != nil {
			switch v := tc.Function.Arguments.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					_ = json.Unmarshal([]byte(v), &input)
				}
			case map[string]any:
				input = v
			default:
				bb, _ := json.Marshal(v)
				_ = json.Unmarshal(bb, &input)
			}
		}
		if input == nil {
			input = map[string]any{}
		}
		out = append(out, AnthropicContentBlock{
			Type:  ContentTypeToolUse,
			ID:    tc.Id,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	if len(out) == 0 {
		// 极端情况：content 和 tool_calls 都为空，Anthropic 仍要返回空 content 数组
		// 不是 null。
		out = []AnthropicContentBlock{}
	}
	return out
}

// stopReasonFromOpenAI 映射 OpenAI finish_reason -> Anthropic stop_reason。
func stopReasonFromOpenAI(fr string) *string {
	var v string
	switch fr {
	case "stop":
		v = StopReasonEndTurn
	case "length":
		v = StopReasonMaxTokens
	case "tool_calls":
		v = StopReasonToolUse
	case "content_filter":
		v = StopReasonEndTurn
	case "":
		// 不强制设置；Anthropic 允许 null。
		return nil
	default:
		v = StopReasonEndTurn
	}
	return &v
}

// randHex 返回 n 字节随机十六进制（48 字符对应 Anthropic msg_ id 长度）。
func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------- 错误转换 ----------

// MakeErrorBody 把 OpenAI 风格 Error 包装为 Anthropic error body。
type StatusCodeError struct {
	StatusCode int
	Type       string
	Message    string
}

// BuildErrorBody 把内部错误渲染成 Anthropic error body。
// statusCode: HTTP 状态码（200/400/401/403/404/429/500/529）。
// 状态码 -> Anthropic error.type 映射：
//   400 invalid_request_error | 401 authentication_error |
//   403 permission_error | 404 not_found_error |
//   429 rate_limit_error | 5xx api_error / overloaded_error
func BuildErrorBody(statusCode int, msg string) AnthropicErrorBody {
	t := ErrorTypeAPI
	switch statusCode {
	case 400:
		t = ErrorTypeInvalidRequest
	case 401:
		t = ErrorTypeAuthentication
	case 403:
		t = ErrorTypePermission
	case 404:
		t = ErrorTypeNotFound
	case 429:
		t = ErrorTypeRateLimit
	case 529:
		t = ErrorTypeOverloaded
	}
	return AnthropicErrorBody{
		Type: "error",
		Error: AnthropicErrorInfo{
			Type:    t,
			Message: strings.TrimSpace(msg),
		},
	}
}

// BuildErrorBodyFromOpenAI 把 OpenAI model.Error 包装成 Anthropic 错误。
// 优先用 OpenAI 自带 type；空时按 HTTP 状态码回退。
func BuildErrorBodyFromOpenAI(statusCode int, e model.Error) AnthropicErrorBody {
	t := e.Type
	if t == "" {
		return BuildErrorBody(statusCode, e.Message)
	}
	// 一些上游用 "invalid_api_key"/"insufficient_qua" 等非标准 type，统一收敛
	switch t {
	case "invalid_request_error", "invalid_api_key":
		return BuildErrorBody(statusCode, e.Message)
	}
	return AnthropicErrorBody{
		Type: "error",
		Error: AnthropicErrorInfo{
			Type:    t,
			Message: e.Message,
		},
	}
}
