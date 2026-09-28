package embeddings_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/embeddings"
)

func TestKeyFor(t *testing.T) {
	cfg := config.EmbeddingsConfig{
		Enabled:  true,
		Provider: "gemini",
		Providers: map[string]config.EmbeddingProviderConfig{
			"gemini": {Type: "gemini"},
		},
	}
	got, ok := embeddings.KeyFor(cfg)
	if !ok || got != "embedding:gemini@default" {
		t.Errorf("KeyFor(gemini) = %q/%v, want embedding:gemini@default", got, ok)
	}

	cfg.Providers["openai"] = config.EmbeddingProviderConfig{Type: "http", Endpoint: "https://api.openai.com/v1"}
	cfg.Provider = "openai"
	got, ok = embeddings.KeyFor(cfg)
	if !ok || got != "embedding:openai@api.openai.com" {
		t.Errorf("KeyFor(openai) = %q/%v, want embedding:openai@api.openai.com", got, ok)
	}

	cfg.Provider = "builtin-local"
	got, ok = embeddings.KeyFor(cfg)
	if !ok || got != "embedding:builtin@default" {
		t.Errorf("KeyFor(builtin) = %q/%v, want embedding:builtin@default", got, ok)
	}

	if _, ok := embeddings.KeyFor(config.EmbeddingsConfig{Provider: "disabled"}); ok {
		t.Error("disabled embeddings must have no key")
	}
}
