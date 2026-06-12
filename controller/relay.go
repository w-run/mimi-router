package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w-run/mimi-router/common"
	"github.com/w-run/mimi-router/common/config"
	"github.com/w-run/mimi-router/common/ctxkey"
	"github.com/w-run/mimi-router/common/helper"
	"github.com/w-run/mimi-router/common/logger"
	"github.com/w-run/mimi-router/middleware"
	dbmodel "github.com/w-run/mimi-router/model"
	"github.com/w-run/mimi-router/monitor"
	"github.com/w-run/mimi-router/relay/controller"
	"github.com/w-run/mimi-router/relay/fallback"
	anthropicmod "github.com/w-run/mimi-router/relay/relaymode/anthropic"
	"github.com/w-run/mimi-router/relay/model"
	"github.com/w-run/mimi-router/relay/relaymode"
)

// https://platform.openai.com/docs/api-reference/chat

func relayHelper(c *gin.Context, relayMode int) *model.ErrorWithStatusCode {
	var err *model.ErrorWithStatusCode
	switch relayMode {
	case relaymode.ImagesGenerations:
		err = controller.RelayImageHelper(c, relayMode)
	case relaymode.AudioSpeech:
		fallthrough
	case relaymode.AudioTranslation:
		fallthrough
	case relaymode.AudioTranscription:
		err = controller.RelayAudioHelper(c, relayMode)
	case relaymode.Proxy:
		err = controller.RelayProxyHelper(c, relayMode)
	case relaymode.AnthropicMessages:
		err = controller.RelayAnthropicHelper(c)
	case relaymode.AnthropicCountTokens:
		err = controller.RelayAnthropicCountTokensHelper(c)
	default:
		err = controller.RelayTextHelper(c)
	}
	return err
}

func Relay(c *gin.Context) {
	ctx := c.Request.Context()
	relayMode := relaymode.GetByPath(c.Request.URL.Path)
	if config.DebugEnabled {
		requestBody, _ := common.GetRequestBody(c)
		logger.Debugf(ctx, "request body: %s", string(requestBody))
	}
	channelId := c.GetInt(ctxkey.ChannelId)
	userId := c.GetInt(ctxkey.Id)
	bizErr := relayHelper(c, relayMode)
	if bizErr == nil {
		monitor.Emit(channelId, true)
		return
	}
	channelName := c.GetString(ctxkey.ChannelName)
	group := c.GetString(ctxkey.Group)
	originalModel := c.GetString(ctxkey.OriginalModel)
	go processChannelRelayError(ctx, userId, channelId, channelName, bizErr)
	requestId := c.GetString(helper.RequestIdKey)

	// 分类错误：429 / 5xx / 网络层错误
	trigger := fallback.ClassifyError(bizErr.StatusCode, nil)
	// 收到 429 且上游给出 Retry-After 时，软禁用该渠道
	fallback.SoftBanFromError(ctx, channelId, channelName, bizErr)

	if trigger == "" {
		logger.Errorf(ctx, "non-retryable error (status %d): %s", bizErr.StatusCode, bizErr.Error.Message)
		writeRelayError(c, bizErr, requestId)
		return
	}

	// 询问当前渠道：是否允许回退
	currentChannel, _ := dbmodel.GetChannelById(channelId, false)
	if !fallback.ShouldFallback(currentChannel, trigger) {
		logger.Errorf(ctx, "channel #%d (%s) not configured to fallback on %s", channelId, currentChannel.Name, trigger)
		writeRelayError(c, bizErr, requestId)
		return
	}

	retryTimes := config.RetryTimes
	if retryTimes <= 0 {
		writeRelayError(c, bizErr, requestId)
		return
	}

	usedIDs := []int{channelId}
	for i := retryTimes; i > 0; i-- {
		channel, err := dbmodel.CachePickFallbackChannel(group, originalModel, usedIDs)
		if err != nil {
			logger.Errorf(ctx, "CachePickFallbackChannel failed: %+v", err)
			break
		}
		logger.Infof(ctx, "fallback to channel #%d (%s) (remain times %d, trigger %s)", channel.Id, channel.Name, i, trigger)
		middleware.SetupContextForSelectedChannel(c, channel, originalModel)
		requestBody, err := common.GetRequestBody(c)
		if err != nil {
			logger.Errorf(ctx, "GetRequestBody failed: %+v", err)
			break
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		bizErr = relayHelper(c, relayMode)
		if bizErr == nil {
			return
		}
		channelId = c.GetInt(ctxkey.ChannelId)
		channelName = c.GetString(ctxkey.ChannelName)
		usedIDs = append(usedIDs, channelId)
		go processChannelRelayError(ctx, userId, channelId, channelName, bizErr)

		// 处理新一轮失败的软禁用；遇到不可重试错误立即终止
		newTrigger := fallback.ClassifyError(bizErr.StatusCode, nil)
		fallback.SoftBanFromError(ctx, channelId, channelName, bizErr)
		if newTrigger == "" {
			logger.Errorf(ctx, "channel #%d (%s) non-retryable error in fallback (status %d)", channelId, channelName, bizErr.StatusCode)
			break
		}
	}

	if bizErr != nil {
		writeRelayError(c, bizErr, requestId)
	}
}

func writeRelayError(c *gin.Context, bizErr *model.ErrorWithStatusCode, requestId string) {
	if bizErr.StatusCode == http.StatusTooManyRequests {
		bizErr.Error.Message = "当前分组上游负载已饱和，请稍后再试"
	}
	// BUG: bizErr is in race condition
	bizErr.Error.Message = helper.MessageWithRequestId(bizErr.Error.Message, requestId)
	// Anthropic 入站协议：按 Anthropic error 格式返回（type/message 字段不同）
	relayMode := relaymode.GetByPath(c.Request.URL.Path)
	if relayMode == relaymode.AnthropicMessages || relayMode == relaymode.AnthropicCountTokens {
		body := anthropicmod.BuildErrorBodyFromOpenAI(bizErr.StatusCode, bizErr.Error)
		c.JSON(bizErr.StatusCode, body)
		return
	}
	c.JSON(bizErr.StatusCode, gin.H{
		"error": bizErr.Error,
	})
}

func shouldRetry(c *gin.Context, statusCode int) bool {
	if _, ok := c.Get(ctxkey.SpecificChannelId); ok {
		return false
	}
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	if statusCode/100 == 5 {
		return true
	}
	if statusCode == http.StatusBadRequest {
		return false
	}
	if statusCode/100 == 2 {
		return false
	}
	return true
}

func processChannelRelayError(ctx context.Context, userId int, channelId int, channelName string, err *model.ErrorWithStatusCode) {
	logger.Errorf(ctx, "relay error (channel #%d %s, user id: %d): %s", channelId, channelName, userId, err.Message)
	// https://platform.openai.com/docs/guides/error-codes/api-errors
	if monitor.ShouldDisableChannel(&err.Error, err.StatusCode) {
		monitor.DisableChannel(channelId, channelName, err.Message)
	} else {
		monitor.Emit(channelId, false)
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := model.Error{
		Message: "API not implemented",
		Type:    "mimi_router_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := model.Error{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}
