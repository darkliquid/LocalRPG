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
		},
		Build: func(_ context.Context, _ []byte) (interface{}, error) {
			return harness.NewOracleModelProvider("narrative-oracle"), nil
		},
	})
}
