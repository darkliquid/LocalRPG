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
			ID:          string(provider.KeyLLMGemini),
			Family:      provider.FamilyLLM,
			Label:       "Google Gemini",
			Description: "Cloud model with shared-key support and a live model catalogue.",
			Source:      "gemini",
			Tier:        provider.TierCloud,
			Features: []provider.Feature{
				provider.FeatureStreaming,
				provider.FeatureTools,
				provider.FeatureThinking,
				provider.FeatureKeyRequired,
				provider.FeatureModelCatalogue,
				provider.FeatureSessions,
			},
			Presets: []provider.Preset{
				{ID: "gemini", Order: 8, Label: "Google Gemini (Cloud API)",
					Description: "Cloud model with a shared key from the Providers tab. Pick the exact model after loading.",
					Config: map[string]interface{}{
						"type": "gemini", "model": "gemini-3.8-flash",
						"temperature": 0.7, "max_tokens": 4096,
						"thinking_budget": 0, "top_p": 0.95, "top_k": 40,
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload harness.ModelBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			id := payload.ID
			if id == "" {
				id = "gemini"
			}
			apiKey, err := harness.ResolveGeminiAPIKey(payload.Config.APIKey, payload.Config.SharedAPIKey)
			if err != nil {
				return nil, err
			}
			return NewGeminiProvider(id, GeminiProviderOptions{
				Model:          payload.Config.Model,
				APIKey:         apiKey,
				Temperature:    &payload.Config.Temperature,
				MaxTokens:      &payload.Config.MaxTokens,
				ThinkingBudget: payload.Config.ThinkingBudget,
				TopP:           payload.Config.TopP,
				TopK:           payload.Config.TopK,
			})
		},
	})
}
