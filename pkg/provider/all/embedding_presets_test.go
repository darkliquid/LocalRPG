package all_test

import (
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/provider"
)

// The provider manager's add menu is populated from descriptor presets, so every
// embedding adapter a user can pick must offer at least one, and each must name
// the provider type it builds.
func TestEmbeddingAdaptersOfferPresets(t *testing.T) {
	for _, key := range []provider.Key{
		provider.KeyEmbeddingONNX,
		provider.KeyEmbeddingOpenAI,
		provider.KeyEmbeddingGemini,
	} {
		reg, ok := provider.Lookup(string(key))
		if !ok {
			t.Fatalf("%s is not registered", key)
		}
		if len(reg.Descriptor.Presets) == 0 {
			t.Errorf("%s offers no preset", key)
		}
		for _, preset := range reg.Descriptor.Presets {
			if preset.Config["type"] == nil {
				t.Errorf("%s preset %q has no provider type", key, preset.ID)
			}
		}
	}
}
