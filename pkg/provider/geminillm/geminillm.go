// Package geminillm registers the Google Gemini chat provider.
package geminillm

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "gemini",
			Family:      provider.FamilyLLM,
			Label:       "Google Gemini",
			Description: "Cloud model with shared-key support and a live model catalogue.",
			Source:      "gemini",
			Features: []provider.Feature{
				provider.FeatureStreaming,
				provider.FeatureTools,
				provider.FeatureThinking,
				provider.FeatureKeyRequired,
				provider.FeatureModelCatalogue,
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var cfg harness.ProviderConfig
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return nil, err
			}
			return harness.NewGeminiModelProvider("gemini", cfg)
		},
	})
}
