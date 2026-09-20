package harness

import (
	"context"
	"testing"
)

type mockProvider struct {
	id     string
	output string
	fail   bool
}

func (m *mockProvider) ID() string { return m.id }
func (m *mockProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if m.fail {
		return nil, context.DeadlineExceeded
	}
	return &GenerateResponse{Text: m.output}, nil
}
func (m *mockProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)
	if m.fail {
		out <- StreamChunk{Error: context.DeadlineExceeded}
		return context.DeadlineExceeded
	}
	out <- StreamChunk{Text: m.output, Done: true}
	return nil
}

func TestRouterRoleDispatchAndFallback(t *testing.T) {
	ctx := context.Background()
	router := NewRouter()

	primary := &mockProvider{id: "primary-claude", fail: true}
	fallback := &mockProvider{id: "fallback-ollama", output: "Fallback story response"}

	router.RegisterProvider(primary)
	router.RegisterProvider(fallback)

	router.AssignRole("gm", "primary-claude")
	router.SetFallback("gm", "fallback-ollama")

	res, err := router.GenerateForRole(ctx, "gm", GenerateRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("GenerateForRole failed: %v", err)
	}

	if res.Text != "Fallback story response" {
		t.Errorf("expected fallback response, got %q", res.Text)
	}
}
