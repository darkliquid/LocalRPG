package embeddingonnx

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestONNXDescriptor(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeyEmbeddingONNX))
	if !ok {
		t.Fatal("embedding:onnx is not registered")
	}
	if reg.Descriptor.Tier != provider.TierOfflineNeural {
		t.Fatalf("tier = %q, want %q", reg.Descriptor.Tier, provider.TierOfflineNeural)
	}
	if reg.Descriptor.Family != provider.FamilyEmbedding {
		t.Fatalf("family = %q, want %q", reg.Descriptor.Family, provider.FamilyEmbedding)
	}
	offline := false
	for _, f := range reg.Descriptor.Features {
		if f == provider.FeatureOffline {
			offline = true
		}
	}
	if !offline {
		t.Fatalf("descriptor must declare the offline feature, got %v", reg.Descriptor.Features)
	}
	if reg.Build == nil {
		t.Fatal("descriptor has no Build function")
	}
}

func TestBuildFallsBackWhenModelMissing(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeyEmbeddingONNX))
	if !ok {
		t.Fatal("embedding:onnx is not registered")
	}
	inst, err := reg.Build(t.Context(), []byte(`{"model_dir":"`+t.TempDir()+`","dimensions":384}`))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	p, ok := inst.(embeddings.Provider)
	if !ok {
		t.Fatalf("Build returned %T, want an embeddings.Provider", inst)
	}
	if _, isFallback := p.(*embeddings.BuiltinHashProjectionProvider); !isFallback {
		t.Fatalf("expected the hash projection fallback, got %T", p)
	}
	if p.Dimensions() != 384 {
		t.Fatalf("fallback dimensions = %d, want 384", p.Dimensions())
	}
}
