package anthropic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/w-run/mimi-router/relay/adaptor/openai"
	"github.com/w-run/mimi-router/relay/model"
)

// WriteAnthropicSSE 把 OpenAI 风格 SSE 流(读自 src)实时转换为 Anthropic
// 风格 SSE 流写到 dst。flusher 在每段写入后调用（nil 时跳过）。
//
// 调用方需要在 Anthropic 客户端响应里设置 SSE 头(text/event-stream)。
// 一旦 src 关闭或解析失败，转换器发出 message_delta + message_stop 后退出。
//
// 转换规则（按 Anthropic 协议顺序）：
//  1. 第一个有效 chunk -> message_start
//  2. 紧跟其后       -> ping
//  3. delta.content   -> content_block_start(text) + 一或多 text_delta +
//     若后续转 tool  -> content_block_stop
//  4. delta.tool_calls -> content_block_start(tool_use) + 一或多
//     input_json_delta + content_block_stop
//  5. 收到 finish_reason / 流的最后 -> message_delta + message_stop
func WriteAnthropicSSE(dst io.Writer, flusher func(), src io.Reader, modelName string) error {
	conv := newStreamConverter(dst, flusher, modelName)
	return conv.run(src)
}

// ---------- 内部 ----------

type streamConverter struct {
	w       io.Writer
	flusher func()

	msgID string
	model string

	// 是否已经发出 message_start / ping，避免重复
	sentMessageStart bool
	sentPing         bool

	// content block 跟踪
	nextBlockIndex    int
	activeBlockIndex  int    // -1 表示无活动 block
	activeBlockType   string // "text" | "tool_use"
	textBlockStarted  bool
	toolBlockStarted  bool
	pendingToolID     string
	pendingToolName   string

	// 累计
	finalStopReason *string
	outputTokens    int
}

func newStreamConverter(w io.Writer, flusher func(), model string) *streamConverter {
	return &streamConverter{
		w:              w,
		flusher:        flusher,
		msgID:          "msg_" + randHex(24),
		model:          model,
		nextBlockIndex: 0,
		activeBlockIndex: -1,
	}
}

func (c *streamConverter) run(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" || payload == "" {
			break
		}
		var chunk openai.ChatCompletionsStreamResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// 解析失败不终止流，丢弃坏行
			continue
		}
		if err := c.handleChunk(&chunk); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	// 收尾：关闭可能还开着的 block，发 message_delta + message_stop
	c.closeActiveBlock()
	c.writeMessageDelta()
	c.writeMessageStop()
	c.flush()
	return nil
}

func (c *streamConverter) handleChunk(chunk *openai.ChatCompletionsStreamResponse) error {
	// 1) 第一个有效 chunk -> message_start + ping
	if !c.sentMessageStart {
		c.sentMessageStart = true
		c.writeMessageStart()
	}
	if !c.sentPing {
		c.sentPing = true
		c.writePing()
	}

	for _, choice := range chunk.Choices {
		d := choice.Delta

		// 2) 文本 delta
		if text := extractDeltaText(d); text != "" {
			c.openTextBlockIfNeeded()
			c.writeTextDelta(text)
		}

		// 3) tool_calls delta
		for _, tc := range d.ToolCalls {
			// 一次 tool call 由若干 chunks 组成。识别"新 call 起点"：
			//   - 有 id 字段（OpenAI 通常只在新 call 第一 chunk 给 id）
			//   - 有 name 字段
			if tc.Id != "" || tc.Function.Name != "" {
				c.closeActiveBlock() // 关闭之前可能还开着的 text/tool block
				c.openToolBlock(tc.Id, tc.Function.Name)
			}
			// 参数增量 - 即便 tool block 还没开（id/name 缺），也开一下。
			if !c.toolBlockStarted {
				// 极少见：上游发来"只有 arguments，没有 id/name 的 chunk"。
				// 此时当作续上之前的 call，id/name 由首个 chunk 提供。
				c.openToolBlock(tc.Id, tc.Function.Name)
			}
			if arg := toolCallArgsString(tc); arg != "" {
				c.writeInputJSONDelta(arg)
			}
		}

		// 4) finish_reason -> 记录最终 stop_reason
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			c.finalStopReason = stopReasonFromOpenAI(*choice.FinishReason)
		}
	}

	// 5) OpenAI 在 stream_options.include_usage=true 时，最后一个 chunk 带 usage
	if chunk.Usage != nil {
		c.outputTokens = chunk.Usage.CompletionTokens
	}

	c.flush()
	return nil
}

// ---------- block 生命周期 ----------

func (c *streamConverter) openTextBlockIfNeeded() {
	if c.textBlockStarted {
		return
	}
	// 关闭可能还活着的 tool block
	c.closeToolBlockOnly()
	c.activeBlockIndex = c.nextBlockIndex
	c.nextBlockIndex++
	c.activeBlockType = "text"
	c.textBlockStarted = true
	c.writeEvent("content_block_start", EventContentBlockStart{
		Type:  "content_block_start",
		Index: c.activeBlockIndex,
		ContentBlock: AnthropicContentBlock{
			Type: ContentTypeText,
			Text: "",
		},
	})
}

func (c *streamConverter) openToolBlock(id, name string) {
	if c.toolBlockStarted {
		// 已经有一个 tool block 在进行中；同名/id 续上即可。
		if id != "" {
			c.pendingToolID = id
		}
		if name != "" {
			c.pendingToolName = name
		}
		return
	}
	c.activeBlockIndex = c.nextBlockIndex
	c.nextBlockIndex++
	c.activeBlockType = "tool_use"
	c.toolBlockStarted = true
	c.pendingToolID = id
	c.pendingToolName = name
	c.writeEvent("content_block_start", EventContentBlockStart{
		Type:  "content_block_start",
		Index: c.activeBlockIndex,
		ContentBlock: AnthropicContentBlock{
			Type: ContentTypeToolUse,
			ID:   id,
			Name: name,
			Input: map[string]any{},
		},
	})
}

func (c *streamConverter) closeActiveBlock() {
	if c.activeBlockIndex < 0 {
		return
	}
	c.writeEvent("content_block_stop", EventContentBlockStop{
		Type:  "content_block_stop",
		Index: c.activeBlockIndex,
	})
	c.activeBlockIndex = -1
	c.activeBlockType = ""
	c.textBlockStarted = false
	c.toolBlockStarted = false
}

func (c *streamConverter) closeToolBlockOnly() {
	if c.toolBlockStarted && c.activeBlockIndex >= 0 {
		c.writeEvent("content_block_stop", EventContentBlockStop{
			Type:  "content_block_stop",
			Index: c.activeBlockIndex,
		})
	}
	c.toolBlockStarted = false
}

// ---------- 事件写入 ----------

func (c *streamConverter) writeMessageStart() {
	c.writeEvent("message_start", EventMessageStart{
		Type: "message_start",
		Message: AnthropicMessageStart{
			ID:           c.msgID,
			Type:         "message",
			Role:         "assistant",
			Content:      []any{},
			Model:        c.model,
			StopReason:   nil,
			StopSequence: nil,
			Usage:        AnthropicUsage{InputTokens: 0, OutputTokens: 0},
		},
	})
}

func (c *streamConverter) writePing() {
	c.writeEvent("ping", EventPing{Type: "ping"})
}

func (c *streamConverter) writeTextDelta(text string) {
	c.writeEvent("content_block_delta", EventContentBlockDelta{
		Type:  "content_block_delta",
		Index: c.activeBlockIndex,
		Delta: ContentBlockDelta{
			Type: "text_delta",
			Text: text,
		},
	})
}

func (c *streamConverter) writeInputJSONDelta(partial string) {
	c.writeEvent("content_block_delta", EventContentBlockDelta{
		Type:  "content_block_delta",
		Index: c.activeBlockIndex,
		Delta: ContentBlockDelta{
			Type:        "input_json_delta",
			PartialJSON: partial,
		},
	})
}

func (c *streamConverter) writeMessageDelta() {
	c.writeEvent("message_delta", EventMessageDelta{
		Type: "message_delta",
		Delta: MessageDelta{
			StopReason:   c.finalStopReason,
			StopSequence: nil,
		},
		Usage: &AnthropicUsage{OutputTokens: c.outputTokens},
	})
}

func (c *streamConverter) writeMessageStop() {
	c.writeEvent("message_stop", EventMessageStop{Type: "message_stop"})
}

func (c *streamConverter) writeEvent(eventName string, payload any) {
	bb, err := json.Marshal(payload)
	if err != nil {
		return
	}
	line := fmt.Sprintf("event: %s\ndata: %s\n\n", eventName, string(bb))
	_, _ = io.WriteString(c.w, line)
}

func (c *streamConverter) flush() {
	if c.flusher == nil {
		return
	}
	c.flusher()
}

// ---------- 解析 helpers ----------

func extractDeltaText(d model.Message) string {
	// Message.Content 是 any；可能 string、可能 []any(content parts)
	if s, ok := d.Content.(string); ok {
		return s
	}
	if arr, ok := d.Content.([]any); ok {
		var b strings.Builder
		for _, it := range arr {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if m["type"] == ContentTypeText {
				if t, ok := m["text"].(string); ok {
					b.WriteString(t)
				}
			}
		}
		return b.String()
	}
	return ""
}

func toolCallArgsString(tc model.Tool) string {
	if tc.Function.Arguments == nil {
		return ""
	}
	switch v := tc.Function.Arguments.(type) {
	case string:
		return v
	case map[string]any:
		bb, _ := json.Marshal(v)
		return string(bb)
	default:
		bb, _ := json.Marshal(v)
		return string(bb)
	}
}
