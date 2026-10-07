package embeddings

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/trace"
)

func onnxConfig(modelPath string) config.EmbeddingsConfig {
	return config.EmbeddingsConfig{
		Enabled:    true,
		Provider:   "local",
		Dimensions: 384,
		Providers: map[string]config.EmbeddingProviderConfig{
			"local": {Type: "onnx", ModelPath: modelPath},
		},
	}
}

func TestFactorySelectsONNX(t *testing.T) {
	cfg := onnxConfig(testModelDir(t))
	p, err := NewProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewProviderFromConfig: %v", err)
	}
	if p == nil || p.ID() == "" {
		t.Fatal("expected an onnx provider")
	}
	if p.Dimensions() != onnxEmbeddingDimensions {
		t.Fatalf("Dimensions = %d, want %d", p.Dimensions(), onnxEmbeddingDimensions)
	}
}

func TestFactoryFallsBackWhenModelMissing(t *testing.T) {
	logger := trace.NewMemory(trace.LevelSummary)
	SetLogger(logger)
	t.Cleanup(func() { SetLogger(trace.Nop()) })

	p, err := NewProviderFromConfig(onnxConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("NewProviderFromConfig: %v", err)
	}
	if _, ok := p.(*BuiltinHashProjectionProvider); !ok {
		t.Fatalf("expected the hash projection fallback, got %T", p)
	}
	if p.Dimensions() != 384 {
		t.Fatalf("fallback dimensions = %d, want 384", p.Dimensions())
	}
	if _, found := logger.Find("model_missing"); !found {
		t.Fatalf("expected a model_missing event, got %v", logger.Names())
	}
}

func TestFactoryUsesConfiguredModelDir(t *testing.T) {
	dir := t.TempDir()
	SetModelDir(dir)
	t.Cleanup(func() { SetModelDir("") })
	if got := ModelDir(); got != dir {
		t.Fatalf("ModelDir() = %q, want %q", got, dir)
	}
}

func TestKeyForONNX(t *testing.T) {
	key, ok := KeyFor(onnxConfig("/tmp/model"))
	if !ok {
		t.Fatal("KeyFor returned ok=false for a configured onnx provider")
	}
	if key != provider.InstanceOrSelf(provider.KeyEmbeddingONNX, "default") {
		t.Fatalf("KeyFor = %q, want %q", key, "embedding:onnx@default")
	}
}
