package anthropicllm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestStreamTextUsageAndHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("anthropic-version") != anthropicVersion {
			t.Errorf("anthropic-version = %q, want %q", r.Header.Get("anthropic-version"), anthropicVersion)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Error("x-api-key missing")
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q, want /v1/messages", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, sent := body["temperature"]; sent {
			t.Errorf("temperature sent: %v", body)
		}
		if body["max_tokens"] == nil {
			t.Errorf("max_tokens missing: %v", body)
		}
		if body["cache_control"] == nil {
			t.Errorf("cache_control missing under the auto default: %v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			"event: message_start",
			`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":100}}}`,
			"",
			"event: ping",
			`data: {"type":"ping"}`,
			"",
			"event: content_block_start",
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			"",
			"event: content_block_delta",
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Once "}}`,
			"",
			"event: content_block_delta",
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"upon a time."}}`,
			"",
			"event: content_block_stop",
			`data: {"type":"content_block_stop","index":0}`,
			"",
			"event: message_delta",
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
			"",
			"event: message_stop",
			`data: {"type":"message_stop"}`,
			"",
		}
		for _, f := range frames {
			fmt.Fprintln(w, f)
		}
	}))
	t.Cleanup(server.Close)

	p := NewAnthropicProvider("gm", Options{Endpoint: server.URL, Model: "claude-sonnet-5-5", APIKey: "k"})
	var received []string
	var usage *harness.Usage
	out := make(chan harness.StreamChunk, 16)
	if err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "Tell a story"}, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for chunk := range out {
		if chunk.Text != "" {
			received = append(received, chunk.Text)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	if got := strings.Join(received, ""); got != "Once upon a time." {
		t.Errorf("text = %q", got)
	}
	if usage == nil || usage.InputTokens != 100 || usage.OutputTokens != 4 {
		t.Errorf("usage = %+v, want 100/4", usage)
	}
}

func TestStreamToolUseBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			"event: content_block_start",
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			"",
			"event: content_block_delta",
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me check. "}}`,
			"",
			"event: content_block_stop",
			`data: {"type":"content_block_stop","index":0}`,
			"",
			"event: content_block_start",
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"search_entities","input":{}}}`,
			"",
			"event: content_block_delta",
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"que"}}`,
			"",
			"event: content_block_delta",
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"ry\":\"Kael\"}"}}`,
			"",
			"event: content_block_stop",
			`data: {"type":"content_block_stop","index":1}`,
			"",
			"event: message_delta",
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}`,
			"",
			"event: message_stop",
			`data: {"type":"message_stop"}`,
			"",
		}
		for _, f := range frames {
			fmt.Fprintln(w, f)
		}
	}))
	t.Cleanup(server.Close)

	p := NewAnthropicProvider("gm", Options{Endpoint: server.URL, Model: "claude-sonnet-5-5", APIKey: "k"})
	out := make(chan harness.StreamChunk, 16)
	var calls []harness.ToolCall
	var finish string
	if err := p.Stream(context.Background(), harness.GenerateRequest{
		Messages: []harness.Message{{Role: "user", Content: "who is Kael?"}},
		Tools:    []harness.ToolSpec{{Name: "search_entities", Description: "s", Parameters: map[string]any{"type": "object"}}},
	}, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for chunk := range out {
		if len(chunk.ToolCalls) > 0 {
			calls = chunk.ToolCalls
		}
		if chunk.Done && chunk.FinishReason != "" {
			finish = chunk.FinishReason
		}
	}
	if len(calls) != 1 || calls[0].ID != "toolu_1" || calls[0].Name != "search_entities" || calls[0].Arguments != `{"query":"Kael"}` {
		t.Fatalf("calls = %+v", calls)
	}
	if finish != "tool_calls" {
		t.Errorf("finish = %q, want tool_calls", finish)
	}
}

func TestStreamErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid x-api-key"}}`))
	}))
	t.Cleanup(srv.Close)

	p := NewAnthropicProvider("gm", Options{Endpoint: srv.URL, Model: "m", APIKey: "bad"})
	out := make(chan harness.StreamChunk, 1)
	err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out)
	if err == nil || !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Fatalf("error = %v, want the provider body", err)
	}
}

func TestGenerateReportsCacheReads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, "event: message_start")
		fmt.Fprintln(w, `data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":300,"cache_creation_input_tokens":20}}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "event: content_block_delta")
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "event: message_delta")
		fmt.Fprintln(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "event: message_stop")
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))
	t.Cleanup(server.Close)

	p := NewAnthropicProvider("gm", Options{Endpoint: server.URL, Model: "m", APIKey: "k"})
	resp, err := p.Generate(context.Background(), harness.GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.CachedTokens != 300 {
		t.Errorf("cached = %d, want 300", resp.CachedTokens)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 330 {
		t.Errorf("usage = %+v, want input 330 (10 + 300 read + 20 creation)", resp.Usage)
	}
}

func TestStreamRefusalIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, "event: message_delta")
		fmt.Fprintln(w, `data: {"type":"message_delta","delta":{"stop_reason":"refusal"},"usage":{"output_tokens":1}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "event: message_stop")
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))
	t.Cleanup(server.Close)

	p := NewAnthropicProvider("gm", Options{Endpoint: server.URL, Model: "m", APIKey: "k"})
	out := make(chan harness.StreamChunk, 4)
	err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "do something harmful"}, out)
	if err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("error = %v, want the refusal surfaced as an error", err)
	}
}
