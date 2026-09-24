// Package oracle registers the deterministic narrative oracle provider.
package oracle

import (
	"context"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "narrative-oracle",
			Family:      provider.FamilyLLM,
			Label:       "Narrative Oracle (Built-in)",
			Description: "Deterministic pure-Go storyteller that needs no model or network.",
			Source:      "builtin",
			Features:    []provider.Feature{provider.FeatureStreaming, provider.FeatureOffline},
			Presets: []provider.Preset{
				{ID: "narrative-oracle", Order: 7, Label: "Narrative Oracle (Built-in Zero-GPU)",
					Description: "Deterministic pure-Go procedural storyteller with rule-based narrative outcomes.",
					Config: map[string]interface{}{
						"type": "builtin", "builtin_name": "narrative-oracle",
					}},
			},
		},
		Build: func(_ context.Context, _ []byte) (interface{}, error) {
			return harness.NewOracleModelProvider("narrative-oracle"), nil
		},
	})
}
