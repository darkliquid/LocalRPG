package all_test

import (
	"context"
	"encoding/json"
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// derivableFeatures are the features the capability derivation can prove from an
// adapter's interfaces. Informational features (key required, offline, model
// catalogue) are not derivable and are not checked here.
var derivableFeatures = map[provider.Feature]func(harness.Capabilities) bool{
	provider.FeatureStreaming:    func(c harness.Capabilities) bool { return c.Streaming },
	provider.FeatureTools:        func(c harness.Capabilities) bool { return c.Tools },
	provider.FeatureThinking:     func(c harness.Capabilities) bool { return c.Thinking },
	provider.FeatureSessions:     func(c harness.Capabilities) bool { return c.Sessions },
	provider.FeatureContextCache: func(c harness.Capabilities) bool { return c.ContextCache },
}

func TestLLMDescriptorsBuildAndFeaturesAreBacked(t *testing.T) {
	descs := provider.List(provider.FamilyLLM)
	if len(descs) == 0 {
		t.Fatal("expected at least one registered llm provider")
	}

	for _, desc := range descs {
		desc := desc
		t.Run(desc.ID, func(t *testing.T) {
			reg, ok := provider.Lookup(desc.ID)
			if !ok {
				t.Fatalf("descriptor %q has no registration", desc.ID)
			}
			cfg := harness.ProviderConfig{Type: "http", APIKey: "test-key"}
			raw, err := json.Marshal(cfg)
			if err != nil {
				t.Fatalf("encode config: %v", err)
			}
			built, err := reg.Build(context.Background(), raw)
			if err != nil {
				t.Fatalf("build %s: %v", desc.ID, err)
			}
			model, ok := built.(harness.ModelProvider)
			if !ok {
				t.Fatalf("provider %q is not a model provider", desc.ID)
			}
			caps := harness.Describe(model)
			for _, feature := range desc.Features {
				check, derivable := derivableFeatures[feature]
				if !derivable {
					continue
				}
				if !check(caps) {
					t.Errorf("provider %q declares %q but the adapter does not back it (%+v)", desc.ID, feature, caps)
				}
			}
		})
	}
}
