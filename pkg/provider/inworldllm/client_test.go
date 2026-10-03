package inworldllm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// sseServer answers a chat completion with a short SSE stream, recording the
// Authorization header of the request it receives.
func sseServer(t *testing.T, gotAuth *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Narrative \"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"response.\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func TestInworldLLMPrecedenceAndBasicAuth(t *testing.T) {
	os.Unsetenv("INWORLD_API_KEY")
	var gotAuth string
	server := sseServer(t, &gotAuth)
	defer server.Close()

	// 1. A role key wins over the shared key.
	client, err := NewInworldLLMClient(config.AgentRoleConfig{Endpoint: server.URL, APIKey: "role-key"}, "shared-key")
	if err != nil {
		t.Fatalf("NewInworldLLMClient: %v", err)
	}
	resp, err := client.Generate(context.Background(), harness.GenerateRequest{Prompt: "Hello"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "Narrative response." {
		t.Errorf("resp.Text = %q, want the streamed text", resp.Text)
	}
	if gotAuth != "Basic role-key" {
		t.Errorf("Authorization = %q, want Basic role-key", gotAuth)
	}

	// 2. The shared key is used when the role has none.
	shared, err := NewInworldLLMClient(config.AgentRoleConfig{Endpoint: server.URL}, "shared-key-2")
	if err != nil {
		t.Fatalf("NewInworldLLMClient: %v", err)
	}
	if _, err := shared.Generate(context.Background(), harness.GenerateRequest{Prompt: "test"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if gotAuth != "Basic shared-key-2" {
		t.Errorf("Authorization = %q, want Basic shared-key-2", gotAuth)
	}

	// 3. INWORLD_API_KEY is the last resort.
	t.Setenv("INWORLD_API_KEY", "env-key")
	env, err := NewInworldLLMClient(config.AgentRoleConfig{Endpoint: server.URL}, "")
	if err != nil {
		t.Fatalf("NewInworldLLMClient: %v", err)
	}
	if _, err := env.Generate(context.Background(), harness.GenerateRequest{Prompt: "test"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if gotAuth != "Basic env-key" {
		t.Errorf("Authorization = %q, want Basic env-key", gotAuth)
	}
}

func TestInworldLLMMissingKeyError(t *testing.T) {
	os.Unsetenv("INWORLD_API_KEY")
	_, err := NewInworldLLMClient(config.AgentRoleConfig{}, "")
	if err == nil || !strings.Contains(err.Error(), "inworld: an API key is required") {
		t.Fatalf("expected missing key error, got: %v", err)
	}
}

func TestInworldLLMDefaults(t *testing.T) {
	client, err := NewInworldLLMClient(config.AgentRoleConfig{APIKey: "k"}, "")
	if err != nil {
		t.Fatalf("NewInworldLLMClient: %v", err)
	}
	if client.ID() != "inworld" {
		t.Errorf("ID = %q, want inworld", client.ID())
	}
	if !client.ToolCallerCapable() {
		t.Errorf("expected the router to be tool-call capable")
	}
}
