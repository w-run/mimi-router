package anthropic

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// simulateOpenAISSE 构造一段假 OpenAI 风格 SSE 字节流。
func simulateOpenAISSE(events []string) []byte {
	var b bytes.Buffer
	for _, e := range events {
		b.WriteString("data: " + e + "\n\n")
	}
	return b.Bytes()
}

// parseAnthropicSSE 把 Anthropic SSE 输出解析成 map[event]json
func parseAnthropicSSE(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	var curEvent string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			curEvent = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			payload := strings.TrimPrefix(line, "data: ")
			var m map[string]any
			// 不严格 json 解析，仅检查有内容
			_ = m
			out = append(out, map[string]any{
				"event":   curEvent,
				"payload": payload,
			})
		}
	}
	return out
}

func TestWriteAnthropicSSE_TextOnly(t *testing.T) {
	openaiSSE := simulateOpenAISSE([]string{
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`[DONE]`,
	})
	var dst bytes.Buffer
	if err := WriteAnthropicSSE(&dst, nil, bytes.NewReader(openaiSSE), "claude-test"); err != nil {
		t.Fatalf("convert: %v", err)
	}
	events := parseAnthropicSSE(t, dst.Bytes())
	// 预期: message_start, ping, content_block_start, content_block_delta(Hello),
	//       content_block_delta( world), content_block_stop, message_delta, message_stop
	gotNames := make([]string, 0, len(events))
	for _, e := range events {
		gotNames = append(gotNames, e["event"].(string))
	}
	want := []string{
		"message_start", "ping",
		"content_block_start",
		"content_block_delta",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}
	if !equalStringSlice(gotNames, want) {
		t.Errorf("event sequence mismatch:\ngot:  %v\nwant: %v", gotNames, want)
	}
	// 找 message_start
	for _, e := range events {
		if e["event"] == "message_start" {
			pl := e["payload"].(string)
			if !strings.Contains(pl, `"claude-test"`) {
				t.Errorf("model missing in message_start: %s", pl)
			}
			if !strings.Contains(pl, `"msg_`) {
				t.Errorf("msg_ id missing: %s", pl)
			}
		}
		if e["event"] == "message_delta" {
			pl := e["payload"].(string)
			if !strings.Contains(pl, `"end_turn"`) {
				t.Errorf("end_turn stop_reason missing: %s", pl)
			}
		}
	}
}

func TestWriteAnthropicSSE_ToolUse(t *testing.T) {
	openaiSSE := simulateOpenAISSE([]string{
		`{"id":"x","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		`{"id":"x","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"loc"}}]}}]}`,
		`{"id":"x","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ation\":\"SF\"}"}}]}}]}`,
		`{"id":"x","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`[DONE]`,
	})
	var dst bytes.Buffer
	if err := WriteAnthropicSSE(&dst, nil, bytes.NewReader(openaiSSE), "claude-test"); err != nil {
		t.Fatalf("convert: %v", err)
	}
	events := parseAnthropicSSE(t, dst.Bytes())
	// 预期序列里有 tool_use 的 content_block_start + 多个 input_json_delta + content_block_stop
	var sawToolStart, sawJSONDelta, sawToolStop, sawFinishReason bool
	for _, e := range events {
		pl := e["payload"].(string)
		switch e["event"] {
		case "content_block_start":
			if strings.Contains(pl, `"tool_use"`) {
				sawToolStart = true
				if !strings.Contains(pl, `"call_1"`) {
					t.Errorf("tool id missing: %s", pl)
				}
				if !strings.Contains(pl, `"get_weather"`) {
					t.Errorf("tool name missing: %s", pl)
				}
			}
		case "content_block_delta":
			if strings.Contains(pl, `"input_json_delta"`) {
				sawJSONDelta = true
			}
		case "content_block_stop":
			// 只看 tool block stop，需要在 tool_use 之后
			if sawToolStart {
				sawToolStop = true
			}
		case "message_delta":
			if strings.Contains(pl, `"tool_use"`) {
				sawFinishReason = true
			}
		}
	}
	if !sawToolStart {
		t.Errorf("no content_block_start with tool_use")
	}
	if !sawJSONDelta {
		t.Errorf("no input_json_delta")
	}
	if !sawToolStop {
		t.Errorf("no content_block_stop for tool_use")
	}
	if !sawFinishReason {
		t.Errorf("no tool_use stop_reason in message_delta")
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
