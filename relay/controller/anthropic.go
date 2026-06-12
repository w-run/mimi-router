package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/w-run/mimi-router/common"
	"github.com/w-run/mimi-router/common/ctxkey"
	"github.com/w-run/mimi-router/common/logger"
	"github.com/w-run/mimi-router/relay"
	"github.com/w-run/mimi-router/relay/adaptor/openai"
	"github.com/w-run/mimi-router/relay/billing"
	billingratio "github.com/w-run/mimi-router/relay/billing/ratio"
	"github.com/w-run/mimi-router/relay/meta"
	relaymodel "github.com/w-run/mimi-router/relay/model"
	"github.com/w-run/mimi-router/relay/relaymode"
	anthropicconv "github.com/w-run/mimi-router/relay/relaymode/anthropic"
)

// RelayAnthropicHelper 处理 /v1/messages 入站请求。
//
// 流程：
//  1. 解析 Anthropic body
//  2. 转 OpenAI body 并回填到 c.Request.Body
//  3. 走与 OpenAI /v1/chat/completions 同一套 helper(preConsume / DoRequest
//     / 错误检查)但跳过 DoResponse 写 c.Writer 的部分——自己接管响应
//  4. 把 OpenAI 响应(非流式 JSON / 流式 SSE)实时转 Anthropic 协议
//     写回 c.Writer
//
// 错误处理：返回 *ErrorWithStatusCode 让 controller.Relay 的回退引擎介入
// (与 OpenAI 链路行为一致)；controller.Relay 在写错误响应时按协议类型
// 选用 Anthropic error body 而非 OpenAI error body。
func RelayAnthropicHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	ctx := c.Request.Context()
	m := meta.GetByContext(c)

	// 1) 解析 + 转换
	body, err := common.GetRequestBody(c)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_request", http.StatusBadRequest)
	}
	var ar anthropicconv.AnthropicRequest
	if err := json.Unmarshal(body, &ar); err != nil {
		return openai.ErrorWrapper(fmt.Errorf("invalid json: %w", err), "invalid_request_error", http.StatusBadRequest)
	}
	if ar.MaxTokens <= 0 {
		return openai.ErrorWrapper(errors.New("max_tokens is required"), "invalid_request_error", http.StatusBadRequest)
	}
	or, err := anthropicconv.ToOpenAIRequest(&ar)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_request_error", http.StatusBadRequest)
	}

	// 2) meta 准备（强制走 ChatCompletions 路径，让 getPromptTokens/计费
	//    走 OpenAI 文本逻辑；AnthropicMessages 在 helper.GetByPath 已经被识别过）
	m.Mode = relaymode.ChatCompletions
	m.IsStream = or.Stream
	m.OriginModelName = or.Model
	or.Model, _ = getMappedModelName(or.Model, m.ModelMapping)
	m.ActualModelName = or.Model
	systemPromptReset := setSystemPrompt(ctx, or, m.ForcedSystemPrompt)

	// 3) 计费
	modelRatio := billingratio.GetModelRatio(or.Model, m.ChannelType)
	groupRatio := billingratio.GetGroupRatio(m.Group)
	ratio := modelRatio * groupRatio
	promptTokens := getPromptTokens(or, m.Mode)
	m.PromptTokens = promptTokens
	preConsumedQuota, bizErr := preConsumeQuota(ctx, or, promptTokens, ratio, m)
	if bizErr != nil {
		return bizErr
	}

	// 4) 把转换后的 OpenAI body 回填到 c.Request.Body，覆盖 ctx 缓存。
	//    这样 controller.Relay 的回退循环在下一次 relayHelper 调用时，
	//    GetRequestBody() 拿到的也是 OpenAI body（而非原始 Anthropic body）。
	newBody, err := json.Marshal(or)
	if err != nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, m.TokenId)
		return openai.ErrorWrapper(err, "marshal_openai_request_failed", http.StatusInternalServerError)
	}
	c.Set(ctxkey.KeyRequestBody, newBody)
	c.Request.Body = io.NopCloser(bytes.NewReader(newBody))
	c.Request.ContentLength = int64(len(newBody))

	// 5) adaptor
	adaptor := relay.GetAdaptor(m.APIType)
	if adaptor == nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, m.TokenId)
		return openai.ErrorWrapper(fmt.Errorf("invalid api type: %d", m.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(m)

	// 6) request body（adaptor 内部 ConvertRequest 转换）
	requestBody, err := getRequestBody(c, m, or, adaptor)
	if err != nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, m.TokenId)
		return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
	}

	// 7) DoRequest
	resp, err := adaptor.DoRequest(c, m, requestBody)
	if err != nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, m.TokenId)
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if isErrorHappened(m, resp) {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, m.TokenId)
		// 把上游 OpenAI 错误体透传成 Anthropic 错误体（写到 c.JSON 后
		// controller.Relay 看到 err==nil 即不再走回退循环）。
		// 但我们仍然返回 bizErr，让回退能触发——controller.Relay.writeRelayError
		// 会按 Anthropic 协议格式写错误。
		_ = readAndDiscardBody(resp)
		return newAnthropicBizErrFromUpstream(resp.StatusCode, resp)
	}

	// 8) 正常响应
	if m.IsStream {
		return handleAnthropicStream(c, m, resp, or, preConsumedQuota, modelRatio, groupRatio, ratio, systemPromptReset)
	}
	return handleAnthropicNonStream(c, m, resp, or, preConsumedQuota, modelRatio, groupRatio, ratio, systemPromptReset)
}

// handleAnthropicNonStream 处理非流式：读 OpenAI JSON 转 Anthropic 写回。
func handleAnthropicNonStream(
	c *gin.Context,
	m *meta.Meta,
	resp *http.Response,
	or *relaymodel.GeneralOpenAIRequest,
	preConsumedQuota int64,
	modelRatio, groupRatio, ratio float64,
	systemPromptReset bool,
) *relaymodel.ErrorWithStatusCode {
	ctx := c.Request.Context()
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, m.TokenId)
		return openai.ErrorWrapper(err, "read_response_failed", http.StatusInternalServerError)
	}

	// 上游可能返回 SlimTextResponse（无 created/model）或 TextResponse。
	// 用 Content-Type 头判断不可靠；直接尝试两种解析。
	var slim openai.SlimTextResponse
	var full openai.TextResponse
	var usage *relaymodel.Usage
	var anthropicResp *anthropicconv.AnthropicResponse
	if err := json.Unmarshal(respBody, &full); err == nil && len(full.Choices) > 0 {
		anthropicResp = anthropicconv.FromOpenAITextResponse(&full)
		usage = &full.Usage
	} else if err := json.Unmarshal(respBody, &slim); err == nil && len(slim.Choices) > 0 {
		anthropicResp = anthropicconv.FromOpenAISlimTextResponse(&slim)
		usage = &slim.Usage
	} else {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, m.TokenId)
		// 不是 chat 响应（如 200 + 错误体）；按原状回写
		c.Data(http.StatusOK, "application/json", respBody)
		go postConsumeQuota(ctx, &relaymodel.Usage{}, m, or, ratio, preConsumedQuota, modelRatio, groupRatio, systemPromptReset)
		return nil
	}

	if usage == nil {
		usage = &relaymodel.Usage{}
	}
	go postConsumeQuota(ctx, usage, m, or, ratio, preConsumedQuota, modelRatio, groupRatio, systemPromptReset)
	c.JSON(http.StatusOK, anthropicResp)
	return nil
}

// handleAnthropicStream 处理流式：实时把 OpenAI SSE 转 Anthropic 多 event。
func handleAnthropicStream(
	c *gin.Context,
	m *meta.Meta,
	resp *http.Response,
	or *relaymodel.GeneralOpenAIRequest,
	preConsumedQuota int64,
	modelRatio, groupRatio, ratio float64,
	systemPromptReset bool,
) *relaymodel.ErrorWithStatusCode {
	ctx := c.Request.Context()
	defer resp.Body.Close()

	// 探测 adaptor 是否声明 Content-Type；保留上游的 chunked/encoding，
	// 但固定 SSE 头以匹配 Anthropic 客户端期望。
	common.SetEventStreamHeaders(c)
	// gin 默认会按 JSON 序列化；我们写的是原始字节，关掉它的 auto-sniff
	c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Writer.WriteHeader(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)
	flushFn := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}

	// 入站时的 model（Anthropic 客户端写的）填到 message_start。
	// 实际转发用 m.ActualModelName；Anthropic 模型字段不影响计费。
	err := anthropicconv.WriteAnthropicSSE(c.Writer, flushFn, resp.Body, m.ActualModelName)
	if err != nil {
		logger.Errorf(ctx, "anthropic stream convert: %s", err.Error())
	}

	// 异步 post-consume：流式场景下 completion_tokens 通常拿不到（依赖
	// upstream 是否在最后 chunk 带 usage）。为保守计费，按 prompt + best-effort
	// completion 累加；没有就 0。
	usage := &relaymodel.Usage{
		PromptTokens:     m.PromptTokens,
		CompletionTokens: 0,
	}
	go postConsumeQuota(ctx, usage, m, or, ratio, preConsumedQuota, modelRatio, groupRatio, systemPromptReset)
	return nil
}

// readAndDiscardBody 排空 resp.Body 以便连接复用。
func readAndDiscardBody(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	_, err := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return err
}

// newAnthropicBizErrFromUpstream 读上游 OpenAI 错误体，包装成 OpenAI 风格
// ErrorWithStatusCode。controller.Relay.writeRelayError 会按协议类型
// 再把它转成 Anthropic 错误格式。
func newAnthropicBizErrFromUpstream(statusCode int, resp *http.Response) *relaymodel.ErrorWithStatusCode {
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	// 尝试解析 OpenAI 错误体
	var oe struct {
		Error relaymodel.Error `json:"error"`
	}
	_ = json.Unmarshal(body, &oe)
	if oe.Error.Message == "" {
		// 没有 error 字段，构造一个
		return openai.ErrorWrapper(
			errors.New(string(body)),
			"upstream_error",
			statusCode,
		)
	}
	return &relaymodel.ErrorWithStatusCode{
		Error:      oe.Error,
		StatusCode: statusCode,
	}
}

// RelayAnthropicCountTokensHelper 处理 /v1/messages/count_tokens 端点。
//
// 不消耗上游额度、不预扣配额，只是把 Anthropic 入参里的 system + messages
// + tools 估算成 input_tokens。复用 ToOpenAIRequest 做归一，然后调 OpenAI
// 的 tiktoken 估算（与 chat 链路一致）。
func RelayAnthropicCountTokensHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	ctx := c.Request.Context()
	_ = ctx

	body, err := common.GetRequestBody(c)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_request", http.StatusBadRequest)
	}
	var ar anthropicconv.AnthropicCountTokensRequest
	if err := json.Unmarshal(body, &ar); err != nil {
		return openai.ErrorWrapper(fmt.Errorf("invalid json: %w", err), "invalid_request_error", http.StatusBadRequest)
	}
	if ar.Model == "" {
		return openai.ErrorWrapper(errors.New("model is required"), "invalid_request_error", http.StatusBadRequest)
	}
	if len(ar.Messages) == 0 {
		return openai.ErrorWrapper(errors.New("messages is required"), "invalid_request_error", http.StatusBadRequest)
	}

	// 复用 ToOpenAIRequest：构造一个 dummy AnthropicRequest
	// max_tokens 必填但不影响估算
	dummy := anthropicconv.AnthropicRequest{
		Model:    ar.Model,
		Messages: ar.Messages,
		System:   ar.System,
		Tools:    ar.Tools,
		MaxTokens: 1,
	}
	or, err := anthropicconv.ToOpenAIRequest(&dummy)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_request_error", http.StatusBadRequest)
	}

	tokens := openai.CountTokenMessages(or.Messages, or.Model)
	c.JSON(http.StatusOK, anthropicconv.AnthropicCountTokensResponse{InputTokens: tokens})
	return nil
}
