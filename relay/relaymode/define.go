package relaymode

const (
	Unknown = iota
	ChatCompletions
	Completions
	Embeddings
	Moderations
	ImagesGenerations
	Edits
	AudioSpeech
	AudioTranscription
	AudioTranslation
	// Proxy is a special relay mode for proxying requests to custom upstream
	Proxy
	// AnthropicMessages 是入站 Anthropic Messages API 的协议模式。
	// 内部依然走 ChatCompletions 链路（relay.TextHelper），
	// 区别仅在 controller 层做一次 Anthropic<->OpenAI 双向转换。
	AnthropicMessages
	// AnthropicCountTokens 是入站 Anthropic count_tokens 端点。
	// 仅返回估算 token 数，不消耗上游额度。
	AnthropicCountTokens
)
