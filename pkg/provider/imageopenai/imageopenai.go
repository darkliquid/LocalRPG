// Package imageopenai registers the OpenAI image generation provider.
package imageopenai

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyImageOpenAI),
			Family:      provider.FamilyImage,
			Label:       "OpenAI Images",
			Description: "gpt-image generation with size and quality mapping. Metered per image.",
			Source:      "http",
			Tier:        provider.TierCloud,
			Features:    []provider.Feature{provider.FeatureKeyRequired, provider.FeatureMetered, provider.FeatureAutoGenerate},
			Presets: []provider.Preset{
				{ID: "openai", Order: 5, Label: "OpenAI Images (Cloud API)",
					Description: "gpt-image generation with size and quality mapping. Metered per image.",
					Config: map[string]any{
						"type": "openai", "model": "gpt-image-2.5-flare", "aspect_ratio": "3:2",
						"quality": "auto", "auto_generate": false,
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (any, error) {
			var payload media.ImageBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewClient(payload.Config, payload.SharedKey), nil
		},
	})
}
