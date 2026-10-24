package openairesponses

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestResponsesRequestShape(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n")
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{
		Endpoint: server.URL, Model: "gpt-6.1-sol", APIKey: "k",
		Temperature: 0.7, MaxTokens: 512,
	})
	req := harness.GenerateRequest{
		System: "You are the GM.",
		Messages: []harness.Message{
			{Role: "user", Content: "look around"},
			{Role: "assistant", Content: "A cellar.", ToolCalls: []harness.ToolCall{{ID: "c1", Name: "roll", Arguments: `{"d":"d20"}`}}},
			{Role: "tool", ToolCallID: "c1", Content: "14"},
			{Role: "user", Content: "climb down"},
		},
		Tools:      []harness.ToolSpec{{Name: "roll", Description: "roll dice", Parameters: map[string]any{"type": "object"}}},
		ToolChoice: "required",
	}
	out := make(chan harness.StreamChunk, 16)
	if err := p.Stream(context.Background(), req, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range out {
	}

	if body["store"] != false {
		t.Errorf("store = %v, want false", body["store"])
	}
	if body["stream"] != true || body["max_output_tokens"] != 512.0 || body["temperature"] != 0.7 {
		t.Errorf("stream/max_output_tokens/temperature: %v %v %v", body["stream"], body["max_output_tokens"], body["temperature"])
	}
	if body["instructions"] != "You are the GM." {
		t.Errorf("instructions = %v", body["instructions"])
	}
	input, ok := body["input"].([]any)
	if !ok {
		t.Fatalf("input missing or wrong type: %v", body)
	}
	// message(user look around), message(assistant), function_call, function_call_output, message(user climb down)
	if len(input) != 5 {
		t.Fatalf("input items = %d, want 5: %v", len(input), input)
	}
	fc := input[2].(map[string]any)
	if fc["type"] != "function_call" || fc["call_id"] != "c1" || fc["name"] != "roll" || fc["arguments"] != `{"d":"d20"}` {
		t.Errorf("function_call item = %v", fc)
	}
	outItem := input[3].(map[string]any)
	if outItem["type"] != "function_call_output" || outItem["call_id"] != "c1" {
		t.Errorf("function_call_output item = %v", outItem)
	}
	if body["tool_choice"] != "required" {
		t.Errorf("tool_choice = %v", body["tool_choice"])
	}
	tools := body["tools"].([]any)[0].(map[string]any)
	if tools["type"] != "function" || tools["name"] != "roll" || tools["strict"] != false {
		t.Errorf("tools[0] = %v", tools)
	}
}

func TestEffortOmitsTemperature(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{Endpoint: server.URL, Model: "gpt-6-astra", Temperature: 0.7, Effort: "high"})
	out := make(chan harness.StreamChunk, 4)
	if err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out); err != nil {
		t.Fatal(err)
	}
	for range out {
	}
	if _, sent := body["temperature"]; sent {
		t.Errorf("temperature sent alongside effort: %v", body)
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" {
		t.Errorf("reasoning = %v", body["reasoning"])
	}
}

func TestResponsesRequestOmitsToolChoiceWhenToolsDropped(t *testing.T) {
	p := NewResponsesProvider("gm", Options{Model: "m"})
	req := harness.GenerateRequest{Prompt: "hi", ToolChoice: "required"}
	payload := p.requestFor(req, false, true)
	if payload.ToolChoice != "" {
		t.Errorf("tool_choice = %q, want empty when tools are withdrawn", payload.ToolChoice)
	}
	payload = p.requestFor(req, true, true)
	if payload.ToolChoice != "" || len(payload.Tools) != 0 {
		t.Errorf("no tools but tool_choice set: %v", payload)
	}
}

func TestSinglePromptIsOneUserItem(t *testing.T) {
	p := NewResponsesProvider("gm", Options{Model: "m"})
	payload := p.requestFor(harness.GenerateRequest{Prompt: "hello"}, true, true)
	if len(payload.Input) != 1 || payload.Input[0].Role != "user" {
		t.Fatalf("input = %+v", payload.Input)
	}
	if payload.Instructions != "" {
		t.Errorf("instructions = %q, want empty", payload.Instructions)
	}
}

func TestAuthorizationHeader(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{Endpoint: server.URL, Model: "m", APIKey: "secret"})
	out := make(chan harness.StreamChunk, 4)
	if err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out); err != nil {
		t.Fatal(err)
	}
	for range out {
	}
	if auth != "Bearer secret" {
		t.Errorf("Authorization = %q", auth)
	}
}
