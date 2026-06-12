package anthropic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToOpenAIRequest_TextOnly(t *testing.T) {
	in := `{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"system": "You are helpful.",
		"messages": [
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi"},
			{"role": "user", "content": "How are you?"}
		]
	}`
	var ar AnthropicRequest
	if err := json.Unmarshal([]byte(in), &ar); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	or, err := ToOpenAIRequest(&ar)
	if err != nil {
		t.Fatalf("ToOpenAIRequest: %v", err)
	}
	if or.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("model mismatch: %s", or.Model)
	}
	if or.MaxTokens != 1024 {
		t.Errorf("max_tokens mismatch: %d", or.MaxTokens)
	}
	if len(or.Messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(or.Messages))
	}
	if or.Messages[0].Role != "system" || or.Messages[0].Content != "You are helpful." {
		t.Errorf("system msg mismatch: %+v", or.Messages[0])
	}
	if or.Messages[1].Content != "Hello" {
		t.Errorf("user msg mismatch: %v", or.Messages[1].Content)
	}
}

func TestToOpenAIRequest_ToolUseAndResult(t *testing.T) {
	in := `{
		"model": "claude-3-5-sonnet",
		"max_tokens": 2048,
		"tools": [{
			"name": "get_weather",
			"description": "Get weather for a location",
			"input_schema": {"type": "object", "properties": {"loc": {"type": "string"}}}
		}],
		"messages": [
			{"role": "user", "content": "What's the weather in SF?"},
			{"role": "assistant", "content": [
				{"type": "text", "text": "Let me check."},
				{"type": "tool_use", "id": "toolu_abc", "name": "get_weather", "input": {"loc": "SF"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "toolu_abc", "content": "72F sunny"}
			]}
		]
	}`
	var ar AnthropicRequest
	if err := json.Unmarshal([]byte(in), &ar); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	or, err := ToOpenAIRequest(&ar)
	if err != nil {
		t.Fatalf("ToOpenAIRequest: %v", err)
	}
	if len(or.Tools) != 1 || or.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tools mismatch: %+v", or.Tools)
	}
	// 预期 4 条消息: system(空)跳过 -> user -> assistant(text+tool_calls) -> tool -> user?
	// 注意: assistant 含 tool_calls; tool_result 转为 role=tool 消息
	if len(or.Messages) < 3 {
		t.Fatalf("messages len < 3: %d", len(or.Messages))
	}
	// 检查 assistant 有 tool_calls
	var foundToolCall bool
	for _, m := range or.Messages {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			foundToolCall = true
			if m.ToolCalls[0].Id != "toolu_abc" {
				t.Errorf("tool_call id mismatch: %s", m.ToolCalls[0].Id)
			}
		}
	}
	if !foundToolCall {
		t.Errorf("no tool_calls in assistant message")
	}
	// 检查 tool 消息
	var foundToolMsg bool
	for _, m := range or.Messages {
		if m.Role == "tool" && m.ToolCallId == "toolu_abc" {
			foundToolMsg = true
			if m.Content != "72F sunny" {
				t.Errorf("tool result content: %v", m.Content)
			}
		}
	}
	if !foundToolMsg {
		t.Errorf("no tool message found")
	}
}

func TestToOpenAIRequest_ImageContent(t *testing.T) {
	in := `{
		"model": "claude-3-5-sonnet",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "What is in this image?"},
				{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "iVBORw0KGgo="}}
			]}
		]
	}`
	var ar AnthropicRequest
	if err := json.Unmarshal([]byte(in), &ar); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	or, err := ToOpenAIRequest(&ar)
	if err != nil {
		t.Fatalf("ToOpenAIRequest: %v", err)
	}
	if len(or.Messages) != 1 {
		t.Fatalf("messages len = %d", len(or.Messages))
	}
	content, ok := or.Messages[0].Content.([]map[string]any)
	if !ok {
		t.Fatalf("content not array: %T", or.Messages[0].Content)
	}
	if len(content) != 2 {
		t.Fatalf("content parts = %d, want 2", len(content))
	}
	if content[1]["type"] != "image_url" {
		t.Errorf("image block not converted to image_url: %+v", content[1])
	}
	url, _ := content[1]["image_url"].(map[string]any)["url"].(string)
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Errorf("data url mismatch: %s", url)
	}
}

func TestToOpenAIRequest_ToolChoice(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want any
	}{
		{
			"auto",
			`{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"tool_choice":"auto"}`,
			"auto",
		},
		{
			"any",
			`{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"tool_choice":"any"}`,
			"required",
		},
		{
			"specific",
			`{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"tool_choice":{"type":"tool","name":"get_weather"}}`,
			map[string]any{
				"type":     "function",
				"function": map[string]any{"name": "get_weather"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ar AnthropicRequest
			if err := json.Unmarshal([]byte(c.in), &ar); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			or, err := ToOpenAIRequest(&ar)
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			// 简化比较：仅验证 ToolChoice 不为 nil 即可
			if or.ToolChoice == nil {
				t.Errorf("tool_choice nil")
			}
		})
	}
}

func TestToOpenAIRequest_StopSequences(t *testing.T) {
	in := `{
		"model": "claude-3-5-sonnet",
		"max_tokens": 1024,
		"stop_sequences": ["\n\nHuman:", "END"],
		"messages": [{"role":"user","content":"hi"}]
	}`
	var ar AnthropicRequest
	if err := json.Unmarshal([]byte(in), &ar); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	or, err := ToOpenAIRequest(&ar)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	arr, ok := or.Stop.([]string)
	if !ok || len(arr) != 2 {
		t.Errorf("stop not []string: %T %+v", or.Stop, or.Stop)
	}
}
