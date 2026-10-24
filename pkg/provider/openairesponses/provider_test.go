package openairesponses

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

func TestResponsesStreamingText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Once upon \"}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"a time.\"}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":11,\"output_tokens\":4,\"input_tokens_details\":{\"cached_tokens\":7}}}}\n\n")
		flusher.Flush()
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{Endpoint: server.URL, Model: "gpt-6.1-sol"})
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
	if usage == nil || usage.InputTokens != 11 || usage.OutputTokens != 4 {
		t.Errorf("usage = %+v", usage)
	}
	resp, err := p.Generate(context.Background(), harness.GenerateRequest{Prompt: "again"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.CachedTokens != 7 {
		t.Errorf("cached = %d, want 7", resp.CachedTokens)
	}
}

func TestResponsesToolCallAccumulation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			`{"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"search_entities","arguments":""}}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"que"}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"ry\":\"Kae\"}"}`,
			`{"type":"response.completed","response":{"status":"completed"}}`,
		}
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
		}
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{Endpoint: server.URL, Model: "gpt-6.1-sol"})
	out := make(chan harness.StreamChunk, 16)
	var calls []harness.ToolCall
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
	}
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "search_entities" || calls[0].Arguments != `{"query":"Kae"}` {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestResponsesTemperatureRetry(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"Unsupported parameter: 'temperature' is not supported with this model."}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{Endpoint: server.URL, Model: "gpt-6-astra", Temperature: 0.7})
	out := make(chan harness.StreamChunk, 4)
	if err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range out {
	}
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(bodies))
	}
	if _, sent := bodies[1]["temperature"]; sent {
		t.Errorf("retry still sent temperature: %v", bodies[1])
	}
}

func TestResponsesErrorBodyInError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model name"}}`))
	}))
	defer srv.Close()
	p := NewResponsesProvider("openai", Options{Endpoint: srv.URL, Model: "nope"})
	out := make(chan harness.StreamChunk, 1)
	err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out)
	if err == nil || !strings.Contains(err.Error(), "bad model name") {
		t.Fatalf("error = %v, want the provider body", err)
	}
}
