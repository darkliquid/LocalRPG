package tools

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// usageEmbedder reports a fixed token count, the way a real provider's
// LastUsage does after a call.
type usageEmbedder struct{ last harness.Usage }

func (u *usageEmbedder) ID() string      { return "usage-embedder" }
func (u *usageEmbedder) Dimensions() int { return 2 }
func (u *usageEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	u.last = harness.Usage{InputTokens: 11, Requests: 1}
	return [][]float32{{1, 0}}, nil
}
func (u *usageEmbedder) LastUsage() harness.Usage { return u.last }

func TestSearchEntitiesReportsEmbeddingUsage(t *testing.T) {
	exec := newTestExecutor(t)
	exec.SetEmbeddingsProvider(&usageEmbedder{})

	var gotKey, gotModel string
	var gotTokens, gotRequests int
	exec.SetEmbeddingUsage(func(providerKey, model string, inputTokens, requests int) {
		gotKey, gotModel, gotTokens, gotRequests = providerKey, model, inputTokens, requests
	}, "embedding:gemini@default", "text-embedding-004")

	if _, ok := exec.Execute(context.Background(), call("search_entities", `{"query":"warden"}`)); !ok {
		t.Fatal("Execute reported failure")
	}
	if gotKey != "embedding:gemini@default" || gotModel != "text-embedding-004" || gotTokens != 11 || gotRequests != 1 {
		t.Fatalf("usage = %q/%q/%d/%d, want the resolved key, model, 11 tokens, 1 request", gotKey, gotModel, gotTokens, gotRequests)
	}
}

func TestSearchWithoutASinkDoesNotPanic(t *testing.T) {
	exec := newTestExecutor(t)
	exec.SetEmbeddingsProvider(&usageEmbedder{})
	if _, ok := exec.Execute(context.Background(), call("search_entities", `{"query":"warden"}`)); !ok {
		t.Fatal("Execute reported failure")
	}
}
