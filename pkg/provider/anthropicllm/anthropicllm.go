package anthropicllm

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyLLMAnthropic),
			Family:      provider.FamilyLLM,
			Label:       "Anthropic Claude (Messages API)",
			Description: "Hosted Claude models over the Messages API: tool calling, prompt caching and reasoning effort included.",
			Source:      "http",
			Tier:        provider.TierCloud,
			Features:    []provider.Feature{provider.FeatureStreaming, provider.FeatureTools, provider.FeatureKeyRequired, provider.FeatureContextCache},
			Presets: []provider.Preset{
				{ID: "anthropic-sonnet", Order: 9, Label: "Claude Sonnet 5.5 (Cloud API)",
					Description: "The recommended Claude narration model. Good balance of cost and depth.",
					Config: map[string]any{
						"type": "anthropic", "model": "claude-sonnet-5-5", "max_tokens": 4096,
					}},
				{ID: "anthropic-haiku", Order: 10, Label: "Claude Haiku 5.5 (Cloud API)",
					Description: "Budget Claude for the extractor and completion roles.",
					Config: map[string]any{
						"type": "anthropic", "model": "claude-haiku-5-5", "max_tokens": 2048,
					}},
				{ID: "anthropic-opus", Order: 11, Label: "Claude Opus 5.5 (Cloud API)",
					Description: "Deeper narration for a higher rate.",
					Config: map[string]any{
						"type": "anthropic", "model": "claude-opus-5-5", "max_tokens": 4096,
					}},
				{ID: "anthropic-fable", Order: 12, Label: "Claude Fable 5.1 (Cloud API)",
					Description: "The frontier model, for the most demanding campaigns.",
					Config: map[string]any{
						"type": "anthropic", "model": "claude-fable-5-1", "max_tokens": 4096,
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
				id = "anthropic"
			}
			apiKey := payload.Config.APIKey
			if apiKey == "" {
				apiKey = payload.Config.SharedAPIKey
			}
			return NewAnthropicProvider(id, Options{
				Endpoint:       payload.Config.Endpoint,
				Model:          payload.Config.Model,
				APIKey:         apiKey,
				Temperature:    payload.Config.Temperature,
				TopP:           payload.Config.TopP,
				TopK:           payload.Config.TopK,
				MaxTokens:      payload.Config.MaxTokens,
				Effort:         payload.Config.Effort,
				PromptCache:    payload.Config.PromptCache,
				ThinkingBudget: payload.Config.ThinkingBudget,
			}), nil
		},
	})
}
