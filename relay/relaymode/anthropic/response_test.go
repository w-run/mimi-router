package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/w-run/mimi-router/relay/adaptor/openai"
	"github.com/w-run/mimi-router/relay/model"
)

func TestFromOpenAITextResponse_Basic(t *testing.T) {
	resp := &openai.TextResponse{
		Id:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1700000000,
		Model:   "gpt-4o",
		Choices: []openai.TextResponseChoice{
			{
				Index: 0,
				Message: model.Message{
					Role:    "assistant",
					Content: "Hello there!",
				},
				FinishReason: "stop",
			},
		},
		Usage: model.Usage{
			PromptTokens:     10,
			CompletionTokens: 3,
			TotalTokens:      13,
		},
	}
	out := FromOpenAITextResponse(resp)
	if out.Type != "message" || out.Role != "assistant" {
		t.Errorf("type/role mismatch: %+v", out)
	}
	if !strings.HasPrefix(out.ID, "msg_") {
		t.Errorf("id not msg_*: %s", out.ID)
	}
	if out.Model != "gpt-4o" {
		t.Errorf("model mismatch: %s", out.Model)
	}
	if out.StopReason == nil || *out.StopReason != StopReasonEndTurn {
		t.Errorf("stop_reason: %v", out.StopReason)
	}
	if len(out.Content) != 1 || out.Content[0].Type != "text" || out.Content[0].Text != "Hello there!" {
		t.Errorf("content: %+v", out.Content)
	}
	if out.Usage.InputTokens != 10 || out.Usage.OutputTokens != 3 {
		t.Errorf("usage: %+v", out.Usage)
	}
}

func TestFromOpenAITextResponse_ToolCalls(t *testing.T) {
	resp := &openai.TextResponse{
		Id:    "x",
		Model: "gpt-4o",
		Choices: []openai.TextResponseChoice{
			{
				Index: 0,
				Message: model.Message{
					Role:    "assistant",
					Content: "",
					ToolCalls: []model.Tool{
						{
							Id:   "call_abc",
							Type: "function",
							Function: model.Function{
								Name:      "get_weather",
								Arguments: `{"location":"SF"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: model.Usage{PromptTokens: 5, CompletionTokens: 8, TotalTokens: 13},
	}
	out := FromOpenAITextResponse(resp)
	if out.StopReason == nil || *out.StopReason != StopReasonToolUse {
		t.Errorf("stop_reason: %v", out.StopReason)
	}
	if len(out.Content) != 1 {
		t.Fatalf("content len = %d, want 1", len(out.Content))
	}
	b := out.Content[0]
	if b.Type != "tool_use" || b.ID != "call_abc" || b.Name != "get_weather" {
		t.Errorf("tool_use block: %+v", b)
	}
	if b.Input["location"] != "SF" {
		t.Errorf("input: %+v", b.Input)
	}
}

func TestFromOpenAITextResponse_FinishReasonMapping(t *testing.T) {
	cases := []struct {
		openaiReason string
		wantReason   string
	}{
		{"stop", StopReasonEndTurn},
		{"length", StopReasonMaxTokens},
		{"tool_calls", StopReasonToolUse},
		{"content_filter", StopReasonEndTurn},
		{"", ""}, // nil
	}
	for _, c := range cases {
		t.Run(c.openaiReason, func(t *testing.T) {
			resp := &openai.TextResponse{
				Model: "m",
				Choices: []openai.TextResponseChoice{
					{Message: model.Message{Content: "x"}, FinishReason: c.openaiReason},
				},
			}
			out := FromOpenAITextResponse(resp)
			if c.wantReason == "" {
				if out.StopReason != nil {
					t.Errorf("expected nil stop_reason, got %v", *out.StopReason)
				}
				return
			}
			if out.StopReason == nil || *out.StopReason != c.wantReason {
				t.Errorf("got %v, want %s", out.StopReason, c.wantReason)
			}
		})
	}
}

func TestBuildErrorBodyFromOpenAI(t *testing.T) {
	bb := BuildErrorBodyFromOpenAI(401, model.Error{Message: "invalid api key", Type: "invalid_api_key"})
	if bb.Type != "error" {
		t.Errorf("type: %s", bb.Type)
	}
	if bb.Error.Type != "authentication_error" {
		t.Errorf("error.type: %s", bb.Error.Type)
	}
	if bb.Error.Message != "invalid api key" {
		t.Errorf("error.message: %s", bb.Error.Message)
	}
	// 序列化应是合法 JSON
	bb2, err := json.Marshal(bb)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(bb2), `"type":"error"`) {
		t.Errorf("json missing type:error: %s", string(bb2))
	}
}

func TestBuildErrorBody_StatusCodeMapping(t *testing.T) {
	cases := map[int]string{
		400: ErrorTypeInvalidRequest,
		401: ErrorTypeAuthentication,
		403: ErrorTypePermission,
		404: ErrorTypeNotFound,
		429: ErrorTypeRateLimit,
		500: ErrorTypeAPI,
		529: ErrorTypeOverloaded,
	}
	for code, want := range cases {
		t.Run(string(rune(code)), func(t *testing.T) {
			bb := BuildErrorBody(code, "msg")
			if bb.Error.Type != want {
				t.Errorf("code %d -> %s, want %s", code, bb.Error.Type, want)
			}
		})
	}
}
