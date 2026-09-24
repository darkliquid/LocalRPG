// Package oracle registers the deterministic narrative oracle provider.
package oracle

import (
	"context"
	"encoding/json"

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
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			id := "narrative-oracle"
			var payload harness.ModelBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
				if payload.ID != "" {
					id = payload.ID
				}
			}
			return NewNarrativeOracleProvider(id), nil
		},
	})
}
