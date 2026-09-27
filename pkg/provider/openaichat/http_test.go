package openaichat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/trace"
)

func TestHTTPProviderStreaming(t *testing.T) {
	// Mock OpenAI/Ollama SSE server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Once upon \"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a time.\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := NewHTTPProvider("mock-ollama", server.URL, "llama3", "")
	req := harness.GenerateRequest{Prompt: "Tell a story"}

	out := make(chan harness.StreamChunk, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(ctx, req, out)
	}()

	var received []string
	for chunk := range out {
		if chunk.Text != "" {
			received = append(received, chunk.Text)
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	result := strings.Join(received, "")
	if result != "Once upon a time." {
		t.Errorf("expected 'Once upon a time.', got %q", result)
	}
}

func TestHTTPProviderSendsGenerationOptionsAndReportsFinish(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	provider := NewHTTPProviderWithOptions("mock-ollama", server.URL, "llama3", "", harness.GenerationOptions{
		Temperature: 0.4,
		MaxTokens:   256,
	})

	out := make(chan harness.StreamChunk, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(context.Background(), harness.GenerateRequest{Prompt: "Tell a story"}, out)
	}()

	var finishReason string
	for chunk := range out {
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	if got := gotBody["max_tokens"]; got != float64(256) {
		t.Errorf("max_tokens = %v, want 256", got)
	}
	if got := gotBody["temperature"]; got != 0.4 {
		t.Errorf("temperature = %v, want 0.4", got)
	}
	if finishReason != "length" {
		t.Errorf("finish reason = %q, want length", finishReason)
	}
}

func TestHTTPProviderTracesTheEnvelopeButNeverThePromptAtSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	const prompt = "the whole prompt"
	memory := trace.NewMemory(trace.LevelSummary)
	provider := NewHTTPProviderWithLogger("gm", server.URL, "gemma", "sk-secret-key", harness.GenerationOptions{MaxTokens: 256}, memory)

	out := make(chan harness.StreamChunk, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(context.Background(), harness.GenerateRequest{Prompt: prompt}, out)
	}()
	for range out {
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	request, ok := memory.Find("provider.request")
	if !ok {
		t.Fatalf("expected a provider.request event, got %v", memory.Names())
	}
	if _, present := request.Fields["prompt"]; present {
		t.Errorf("summary level must not carry the prompt")
	}
	if request.Fields["prompt_sha256"] == nil || request.Fields["prompt_chars"] != len([]rune(prompt)) {
		t.Errorf("expected a prompt hash and length, got %+v", request.Fields)
	}
	if request.Fields["auth_set"] != true {
		t.Errorf("expected auth_set to record that a key was used, got %+v", request.Fields)
	}
	if request.Fields["max_tokens"] != 256 {
		t.Errorf("expected the envelope's max_tokens, got %+v", request.Fields)
	}

	// The key itself must not appear anywhere, at any level.
	for _, event := range memory.Events() {
		for key, value := range event.Fields {
			if text, ok := value.(string); ok && strings.Contains(text, "sk-secret-key") {
				t.Errorf("event %s field %s leaked the API key", event.Name, key)
			}
		}
	}

	response, ok := memory.Find("provider.response")
	if !ok {
		t.Fatalf("expected a provider.response event, got %v", memory.Names())
	}
	if response.Fields["finish_reason"] != "stop" || response.Fields["chunks"] != 1 {
		t.Errorf("unexpected response fields: %+v", response.Fields)
	}
}

func TestHTTPProviderRecordsRawWireLinesOnlyAtFull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	names := func(level trace.Level) []string {
		memory := trace.NewMemory(level)
		provider := NewHTTPProviderWithLogger("gm", server.URL, "gemma", "", harness.GenerationOptions{}, memory)
		out := make(chan harness.StreamChunk, 10)
		errCh := make(chan error, 1)
		go func() { errCh <- provider.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out) }()
		for range out {
		}
		if err := <-errCh; err != nil {
			t.Fatalf("Stream failed: %v", err)
		}
		return memory.Names()
	}

	full := names(trace.LevelFull)
	found := false
	for _, name := range full {
		if name == "provider.wire" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected provider.wire at full level, got %v", full)
	}

	for _, name := range names(trace.LevelSummary) {
		if name == "provider.wire" {
			t.Errorf("did not expect provider.wire at summary level")
		}
	}
}

func TestHTTPProviderHonoursTheChunkLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 10; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"},\"finish_reason\":null}]}\n\n")
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	memory := trace.NewMemory(trace.LevelFull)
	provider := NewHTTPProviderWithLogger("gm", server.URL, "gemma", "", harness.GenerationOptions{}, memory)
	provider.SetChunkLimit(3)

	out := make(chan harness.StreamChunk, 20)
	errCh := make(chan error, 1)
	go func() { errCh <- provider.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out) }()
	for range out {
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	wire := 0
	for _, name := range memory.Names() {
		if name == "provider.wire" {
			wire++
		}
	}
	if wire != 3 {
		t.Errorf("wire events = %d, want the configured limit of 3", wire)
	}
}

func TestHTTPProviderAccumulatesStreamedToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["tools"]; !ok {
			t.Errorf("expected a tools field, got %v", body)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"search_entities","arguments":"{\"que"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ry\":\"Kae"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"l\"}"}}]},"finish_reason":"tool_calls"}]}`,
		}
		for _, frame := range frames {
			fmt.Fprintf(w, "data: %s\n\n", frame)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	provider := NewHTTPProvider("gm", server.URL, "test", "")
	provider.logger = trace.Nop()

	out := make(chan harness.StreamChunk, 20)
	var calls []harness.ToolCall
	done := make(chan struct{})
	go func() {
		for chunk := range out {
			if len(chunk.ToolCalls) > 0 {
				calls = chunk.ToolCalls
			}
		}
		close(done)
	}()

	req := harness.GenerateRequest{
		Messages: []harness.Message{{Role: "user", Content: "who is Kael?"}},
		Tools:    []harness.ToolSpec{{Name: "search_entities", Description: "search", Parameters: map[string]interface{}{"type": "object"}}},
	}
	if err := provider.Stream(context.Background(), req, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	<-done

	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want one assembled call", calls)
	}
	if calls[0].ID != "call_1" || calls[0].Name != "search_entities" {
		t.Errorf("call = %+v", calls[0])
	}
	if calls[0].Arguments != `{"query":"Kael"}` {
		t.Errorf("arguments = %q, want the reassembled JSON", calls[0].Arguments)
	}
}

func TestHTTPProviderDegradesOnceWhenToolsAreRejected(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["tools"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"unknown field tools"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Fine.\"},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	provider := NewHTTPProvider("gm", server.URL, "test", "")
	memory := trace.NewMemory(trace.LevelFull)
	provider.SetLogger(memory)

	out := make(chan harness.StreamChunk, 20)
	go func() {
		for range out {
		}
	}()

	req := harness.GenerateRequest{
		Messages: []harness.Message{{Role: "user", Content: "hello"}},
		Tools:    []harness.ToolSpec{{Name: "search_entities"}},
	}
	if err := provider.Stream(context.Background(), req, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want one retry without tools", attempts)
	}
	event, ok := memory.Find("provider.tools")
	if !ok {
		t.Fatalf("expected a provider.tools trace event")
	}
	if event.Fields["rejected"] != true {
		t.Errorf("rejected = %v, want true", event.Fields["rejected"])
	}
}

func TestStreamIncludesProviderBodyInError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model name"}}`))
	}))
	defer srv.Close()

	p := NewHTTPProvider("openai-test", srv.URL, "test-model", "")
	out := make(chan harness.StreamChunk, 1)
	err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "bad model name") {
		t.Fatalf("error %q does not include the provider body", err.Error())
	}
}

func TestToolChoiceRequiredMarshals(t *testing.T) {
	payload := openAIChatRequest{Model: "m", Stream: true, ToolChoice: "required"}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tool_choice":"required"`) {
		t.Fatalf("body omitted tool_choice: %s", data)
	}
}
