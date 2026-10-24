// Package openairesponses registers the OpenAI Responses API chat provider.
package openairesponses

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyLLMOpenAI),
			Family:      provider.FamilyLLM,
			Label:       "OpenAI (Responses API)",
			Description: "OpenAI's hosted models over the Responses API: tool calling and reasoning effort included.",
			Source:      "openai",
			Tier:        provider.TierCloud,
			Features:    []provider.Feature{provider.FeatureStreaming, provider.FeatureTools, provider.FeatureKeyRequired},
			Presets: []provider.Preset{
				{ID: "openai", Order: 5, Label: "OpenAI (Cloud API)",
					Description: "Hosted GPT models over the Responses API. Needs an API key or providers.openai.api_key.",
					Config: map[string]any{
						"type": "openai", "model": "gpt-6.1-sol", "max_tokens": 4096,
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (any, error) {
			var payload harness.ModelBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			id := payload.ID
			if id == "" {
				id = "openai"
			}
			apiKey := payload.Config.APIKey
			if apiKey == "" {
				apiKey = payload.Config.SharedAPIKey
			}
			return NewResponsesProvider(id, Options{
				Endpoint:    payload.Config.Endpoint,
				Model:       payload.Config.Model,
				APIKey:      apiKey,
				Temperature: payload.Config.Temperature,
				MaxTokens:   payload.Config.MaxTokens,
				Effort:      payload.Config.Effort,
			}), nil
		},
	})
}
