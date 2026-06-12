// Package anthropic 实现 Anthropic Messages API 协议 <-> 内部 OpenAI 协议
// 的双向转换。mimi-router 对外暴露 /v1/messages 端点，接收 Anthropic 协议
// 请求，内部转成 model.GeneralOpenAIRequest 走现成 relay 链路分发到任意
// OpenAI 兼容渠道；响应（含流式）再转回 Anthropic 协议返回。
//
// 设计原则：
//   - 鉴权仍走 mimi-router 颁发的 sk-*（Authorization: Bearer ...）。
//     客户端 SDK 仍可使用，只需 base_url 指向 mimi-router、api_key 用 sk-xxx。
//     anthropic-version 头被忽略（mimi-router 内部统一版本）。
//   - 不修改 meta.Mode 之外的现有中间件链，/v1/messages 与 /v1/chat/completions
//     共享 TokenAuth / Distribute / RateLimit / 限流 / 回退 / 计费 / 日志。
//   - v1 范围：文本 + 多模态（图） + 工具；流式（多 event）；count_tokens 端点。
//   - 工具调用语义：tool_use <-> tool_calls；tool_result <-> role=tool 消息。
package anthropic

// ---------- 入站请求 ----------

// AnthropicRequest 是 /v1/messages 接收的请求体。
// 字段参考 https://docs.anthropic.com/en/api/messages 。
type AnthropicRequest struct {
	Model         string             `json:"model"`
	Messages      []AnthropicMessage `json:"messages"`
	System        any                `json:"system,omitempty"`          // string | []SystemBlock
	MaxTokens     int                `json:"max_tokens"`                // Anthropic 必填；mimi-router 会校验
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	TopK          *int               `json:"top_k,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	Tools         []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice    any                `json:"tool_choice,omitempty"` // "auto" | "any" | {"type":"tool","name":..} | nil
	Metadata      map[string]any     `json:"metadata,omitempty"`
	UserID        string             `json:"user_id,omitempty"` // 透传字段
}

// AnthropicMessage 是入站 messages 数组中的元素。
// Content 形如 string 或 []ContentBlock。Role 仅允许 user / assistant。
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string | []AnthropicContentBlock
}

// AnthropicContentBlock 是 messages[].content 数组里的元素。
// 支持 type: text / image / tool_use / tool_result。
type AnthropicContentBlock struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// image
	Source *AnthropicImageSource `json:"source,omitempty"`

	// tool_use（assistant 消息里出现）
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`

	// tool_result（user 消息里出现，对应 assistant 的 tool_use）
	ToolUseID string `json:"tool_use_id,omitempty"`
	// tool_result 的 content 可以是 string 或 []ContentBlock（多模态结果）。
	// v1 只处理 string；数组时取 text 字段拼起来。
	Content    any  `json:"content,omitempty"`
	IsError    bool `json:"is_error,omitempty"`
	CacheCtrl  *AnthropicCacheControl `json:"cache_control,omitempty"`
}

// AnthropicImageSource 描述图片来源（base64）。
type AnthropicImageSource struct {
	Type      string `json:"type"`            // "base64"
	MediaType string `json:"media_type,omitempty"` // "image/png" | "image/jpeg" | "image/gif" | "image/webp"
	Data      string `json:"data"`            // base64 字符串
}

// AnthropicCacheControl 是 prompt caching 提示。v1 透传忽略。
type AnthropicCacheControl struct {
	Type string `json:"type,omitempty"`
}

// SystemBlock 用于 system 字段为数组形式时。
type SystemBlock struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

// AnthropicTool 是入站 tools 元素，扁平结构（无 Function 包装）。
type AnthropicTool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"input_schema,omitempty"`
	CacheCtrl    *AnthropicCacheControl `json:"cache_control,omitempty"`
}

// ---------- 出站响应 ----------

// AnthropicResponse 是 /v1/messages 的非流式响应。
type AnthropicResponse struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`         // 固定 "message"
	Role         string                 `json:"role"`         // 固定 "assistant"
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                 `json:"model"`
	StopReason   *string                `json:"stop_reason"`  // end_turn | max_tokens | tool_use | stop_sequence | null
	StopSequence *string                `json:"stop_sequence"`
	Usage        AnthropicUsage         `json:"usage"`
}

// AnthropicUsage 是 token 计数。
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicError 是错误响应体。
type AnthropicErrorBody struct {
	Type  string             `json:"type"`  // 固定 "error"
	Error AnthropicErrorInfo `json:"error"`
}

// AnthropicErrorInfo 是错误详情。
type AnthropicErrorInfo struct {
	Type    string `json:"type"`    // invalid_request_error | authentication_error | ...
	Message string `json:"message"`
}

// ---------- 流式事件 ----------

// 流式事件按 Anthropic 协议以 `event: <type>\ndata: <json>\n\n` 形式发出。
// 下面这些 struct 是事件 data 字段的 JSON 形态。

// EventMessageStart 是流式第一个事件。
type EventMessageStart struct {
	Type    string             `json:"type"` // "message_start"
	Message AnthropicMessageStart `json:"message"`
}

// AnthropicMessageStart 是 message_start 事件中 message 字段。
type AnthropicMessageStart struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"` // "message"
	Role         string          `json:"role"` // "assistant"
	Content      []any           `json:"content"` // 流式时为空 []
	Model        string          `json:"model"`
	StopReason   *string         `json:"stop_reason"`
	StopSequence *string         `json:"stop_sequence"`
	Usage        AnthropicUsage  `json:"usage"`
}

// EventContentBlockStart 是 content_block 起始事件。
type EventContentBlockStart struct {
	Type         string                 `json:"type"` // "content_block_start"
	Index        int                    `json:"index"`
	ContentBlock AnthropicContentBlock  `json:"content_block"`
}

// EventPing 是 keep-alive。
type EventPing struct {
	Type string `json:"type"` // "ping"
}

// EventContentBlockDelta 是文本/参数增量。
type EventContentBlockDelta struct {
	Type  string             `json:"type"` // "content_block_delta"
	Index int                `json:"index"`
	Delta ContentBlockDelta  `json:"delta"`
}

// ContentBlockDelta 是增量内容。Type 为 text_delta 或 input_json_delta。
type ContentBlockDelta struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`          // text_delta
	PartialJSON  string `json:"partial_json,omitempty"`  // input_json_delta
}

// EventContentBlockStop 是 content_block 结束事件。
type EventContentBlockStop struct {
	Type  string `json:"type"` // "content_block_stop"
	Index int    `json:"index"`
}

// EventMessageDelta 是 stop_reason / 累计 usage 更新。
type EventMessageDelta struct {
	Type  string             `json:"type"` // "message_delta"
	Delta MessageDelta       `json:"delta"`
	Usage *AnthropicUsage    `json:"usage,omitempty"`
}

// MessageDelta 是 message_delta 事件 delta 字段。
type MessageDelta struct {
	StopReason   *string `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

// EventMessageStop 是流式结束事件。
type EventMessageStop struct {
	Type string `json:"type"` // "message_stop"
}

// ---------- count_tokens 端点 ----------

// AnthropicCountTokensRequest 是 /v1/messages/count_tokens 的入参。
// 字段与 AnthropicRequest 几乎一致，但 stream / tools_choice 等被忽略。
type AnthropicCountTokensRequest struct {
	Model    string             `json:"model"`
	Messages []AnthropicMessage `json:"messages"`
	System   any                `json:"system,omitempty"`
	Tools    []AnthropicTool    `json:"tools,omitempty"`
}

// AnthropicCountTokensResponse 是 /v1/messages/count_tokens 的出参。
type AnthropicCountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

// ---------- 工具类型转换映射 ----------

// Anthropic tool type -> 内部 model 事件相关常量。
const (
	ContentTypeText         = "text"
	ContentTypeImage        = "image"
	ContentTypeToolUse      = "tool_use"
	ContentTypeToolResult   = "tool_result"

	StopReasonEndTurn       = "end_turn"
	StopReasonMaxTokens     = "max_tokens"
	StopReasonToolUse       = "tool_use"
	StopReasonStopSequence  = "stop_sequence"
	StopReasonRefusal       = "refusal"

	ErrorTypeInvalidRequest = "invalid_request_error"
	ErrorTypeAuthentication = "authentication_error"
	ErrorTypePermission     = "permission_error"
	ErrorTypeNotFound       = "not_found_error"
	ErrorTypeRateLimit      = "rate_limit_error"
	ErrorTypeAPI            = "api_error"
	ErrorTypeOverloaded     = "overloaded_error"
)
