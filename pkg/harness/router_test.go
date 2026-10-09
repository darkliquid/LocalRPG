package harness

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/trace"
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

func TestRouterTriesTheChainInOrder(t *testing.T) {
	ctx := context.Background()
	router := NewRouter()
	router.RegisterProvider(&mockProvider{id: "a", fail: true})
	router.RegisterProvider(&mockProvider{id: "b", output: "second"})
	router.SetChain("gm", []string{"a", "b"}, config.SelectFirst, "")

	res, err := router.GenerateForRole(ctx, "gm", GenerateRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("GenerateForRole failed: %v", err)
	}
	if res.Text != "second" {
		t.Fatalf("expected the second chain member, got %q", res.Text)
	}
}

func TestRouterRecordsAnAttemptPerChainFailure(t *testing.T) {
	ctx := context.Background()
	router := NewRouter()
	router.RegisterProvider(&mockProvider{id: "a", fail: true})
	router.RegisterProvider(&mockProvider{id: "b", fail: true})
	router.SetChain("gm", []string{"a", "b"}, config.SelectFirst, "")

	_, err := router.GenerateForRole(ctx, "gm", GenerateRequest{Prompt: "hello"})
	failure, ok := FailureFrom(err)
	if !ok {
		t.Fatalf("expected a GenerationFailure, got %v", err)
	}
	if len(failure.Attempts) != 2 {
		t.Fatalf("expected one attempt per chain member, got %d", len(failure.Attempts))
	}
	if failure.Attempts[0].Provider != "a" || failure.Attempts[1].Provider != "b" {
		t.Fatalf("attempts are not in chain order: %+v", failure.Attempts)
	}
}

func TestRouterChainCheapestReorders(t *testing.T) {
	ctx := context.Background()
	router := NewRouter()
	router.RegisterProvider(&mockProvider{id: "a", output: "expensive"})
	router.RegisterProvider(&mockProvider{id: "b", output: "cheap"})
	router.SetChain("gm", []string{"a", "b"}, config.SelectCheapest, "")
	router.SetChainPrice(func(id string) (int64, bool) {
		return map[string]int64{"a": 10, "b": 1}[id], true
	})

	res, err := router.GenerateForRole(ctx, "gm", GenerateRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("GenerateForRole failed: %v", err)
	}
	if res.Text != "cheap" {
		t.Fatalf("cheapest should pick b, got %q", res.Text)
	}
}

func TestRouterNoChainIsUnchanged(t *testing.T) {
	ctx := context.Background()
	router := NewRouter()
	router.RegisterProvider(&mockProvider{id: "primary", fail: true})
	router.RegisterProvider(&mockProvider{id: "fallback", output: "fallback"})
	router.AssignRole("gm", "primary")
	router.SetFallback("gm", "fallback")
	// An empty chain must clear rather than shadow the primary/fallback path.
	router.SetChain("gm", nil, config.SelectCheapest, "")

	res, err := router.GenerateForRole(ctx, "gm", GenerateRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("GenerateForRole failed: %v", err)
	}
	if res.Text != "fallback" {
		t.Fatalf("expected the configured fallback, got %q", res.Text)
	}
}

func TestRouterStreamTriesTheChainInOrder(t *testing.T) {
	ctx := context.Background()
	router := NewRouter()
	router.RegisterProvider(&mockProvider{id: "a", fail: true})
	router.RegisterProvider(&mockProvider{id: "b", output: "streamed"})
	router.SetChain("gm", []string{"a", "b"}, config.SelectFirst, "")

	out := make(chan StreamChunk, 4)
	if err := router.StreamForRole(ctx, "gm", GenerateRequest{Prompt: "hello"}, out); err != nil {
		t.Fatalf("StreamForRole failed: %v", err)
	}
	var text string
	for chunk := range out {
		text += chunk.Text
	}
	if text != "streamed" {
		t.Fatalf("expected the second chain member to stream, got %q", text)
	}
}

func TestChainSelectionIsTraced(t *testing.T) {
	ctx := context.Background()
	mem := trace.NewMemory(trace.LevelSummary)
	router := NewRouter()
	router.SetLogger(mem)
	router.RegisterProvider(&mockProvider{id: "a", output: "a"})
	router.RegisterProvider(&mockProvider{id: "b", output: "b"})
	router.SetChain("gm", []string{"a", "b"}, config.SelectCheapest, "")
	router.SetChainPrice(func(id string) (int64, bool) {
		return map[string]int64{"a": 10, "b": 1}[id], true
	})

	if _, err := router.GenerateForRole(ctx, "gm", GenerateRequest{Prompt: "hello"}); err != nil {
		t.Fatalf("GenerateForRole failed: %v", err)
	}
	event, ok := mem.Find("router.select")
	if !ok {
		t.Fatalf("router.select was not traced: %v", mem.Names())
	}
	if event.Fields["rule"] != config.SelectCheapest || event.Fields["selected"] != "b" {
		t.Fatalf("router.select fields = %+v", event.Fields)
	}
}
