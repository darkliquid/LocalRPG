package harness_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/trace"
)

func TestResolveGeminiAPIKeyPriority(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	// 1. None provided
	key, err := harness.ResolveGeminiAPIKey("", "")
	if err == nil {
		t.Errorf("expected error when no key provided, got %q", key)
	}

	// 2. Fallback to GOOGLE_API_KEY env
	t.Setenv("GOOGLE_API_KEY", "env-google-key")
	key, err = harness.ResolveGeminiAPIKey("", "")
	if err != nil || key != "env-google-key" {
		t.Errorf("expected env-google-key, got %q (err: %v)", key, err)
	}

	// 3. GEMINI_API_KEY env overrides GOOGLE_API_KEY
	t.Setenv("GEMINI_API_KEY", "env-gemini-key")
	key, err = harness.ResolveGeminiAPIKey("", "")
	if err != nil || key != "env-gemini-key" {
		t.Errorf("expected env-gemini-key, got %q (err: %v)", key, err)
	}

	// 4. Shared provider key overrides env
	key, err = harness.ResolveGeminiAPIKey("", "shared-config-key")
	if err != nil || key != "shared-config-key" {
		t.Errorf("expected shared-config-key, got %q (err: %v)", key, err)
	}

	// 5. Role key overrides shared config key
	key, err = harness.ResolveGeminiAPIKey("role-override-key", "shared-config-key")
	if err != nil || key != "role-override-key" {
		t.Errorf("expected role-override-key, got %q (err: %v)", key, err)
	}
}

func TestGeminiProviderGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)

		// Assert system instruction is sent
		if !strings.Contains(bodyStr, "You are the GM") {
			t.Errorf("expected body to contain system instruction, got: %s", bodyStr)
		}
		// Assert prompt text is sent
		if !strings.Contains(bodyStr, "Look around the tavern") {
			t.Errorf("expected body to contain prompt text, got: %s", bodyStr)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"candidates": [
				{
					"content": {
						"parts": [
							{"thought": true, "text": "I should describe the fire and guests."},
							{"text": "The hearth crackles with welcoming warmth."}
						],
						"role": "model"
					},
					"finishReason": "STOP"
				}
			]
		}`)
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     "test-key",
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	budget := 0
	provider, err := harness.NewGeminiProvider("test-gemini", harness.GeminiProviderOptions{
		Model:          "gemini-2.5-flash",
		APIKey:         "test-key",
		ThinkingBudget: &budget,
		Client:         client,
	})
	if err != nil {
		t.Fatalf("NewGeminiProvider: %v", err)
	}

	resp, err := provider.Generate(ctx, harness.GenerateRequest{
		System: "You are the GM",
		Prompt: "Look around the tavern",
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	// Thought must NOT be included in generated text
	if strings.Contains(resp.Text, "I should describe") {
		t.Errorf("expected thought to be filtered from text, got %q", resp.Text)
	}
	if !strings.Contains(resp.Text, "The hearth crackles") {
		t.Errorf("expected narration text, got %q", resp.Text)
	}
}

func TestGeminiProviderStreamSeparatesThoughtsAndEmitsTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", `{"candidates": [{"content": {"parts": [{"thought": true, "text": "Evaluating player action..."}], "role": "model"}}]}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"candidates": [{"content": {"parts": [{"text": "The ancient door groans open."}], "role": "model"}}]}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"candidates": [{"content": {"parts": [{"functionCall": {"id": "call-1", "name": "get_entity", "args": {"id": "iron-gate"}}}], "role": "model"}, "finishReason": "STOP"}]}`)
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     "test-key",
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	})
	if err != nil {
		t.Fatalf("genai.NewClient: %v", err)
	}

	provider, err := harness.NewGeminiProvider("test-gemini", harness.GeminiProviderOptions{
		Model:  "gemini-2.5-flash",
		APIKey: "test-key",
		Client: client,
	})
	if err != nil {
		t.Fatalf("NewGeminiProvider: %v", err)
	}

	mem := trace.NewMemory(trace.LevelFull)
	provider.SetLogger(mem)

	out := make(chan harness.StreamChunk, 10)
	err = provider.Stream(ctx, harness.GenerateRequest{Prompt: "open door"}, out)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var chunks []harness.StreamChunk
	for c := range out {
		chunks = append(chunks, c)
	}

	// 1. Verify thoughts are NOT in stream chunks
	for _, chunk := range chunks {
		if strings.Contains(chunk.Text, "Evaluating player action") {
			t.Errorf("thought leaked into stream chunk: %s", chunk.Text)
		}
	}

	// 2. Verify narration text is in stream chunks
	foundNarration := false
	for _, chunk := range chunks {
		if strings.Contains(chunk.Text, "The ancient door groans") {
			foundNarration = true
		}
	}
	if !foundNarration {
		t.Errorf("expected narration chunk in stream")
	}

	// 3. Verify function call is parsed
	foundTool := false
	for _, chunk := range chunks {
		if len(chunk.ToolCalls) > 0 && chunk.ToolCalls[0].Name == "get_entity" {
			foundTool = true
			if !strings.Contains(chunk.ToolCalls[0].Arguments, "iron-gate") {
				t.Errorf("unexpected tool args: %s", chunk.ToolCalls[0].Arguments)
			}
		}
	}
	if !foundTool {
		t.Errorf("expected tool call in stream")
	}

	// 4. Verify thought was recorded in trace
	foundThoughtEvent := false
	for _, ev := range mem.Events() {
		if ev.Name == "gemini_thought" {
			if txt, ok := ev.Fields["text"].(string); ok && strings.Contains(txt, "Evaluating player action") {
				foundThoughtEvent = true
			}
		}
	}
	if !foundThoughtEvent {
		t.Errorf("expected gemini_thought trace event to be recorded")
	}
}
