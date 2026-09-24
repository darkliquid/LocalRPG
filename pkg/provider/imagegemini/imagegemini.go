// Package imagegemini registers the Google Gemini/Imagen image provider.
package imagegemini

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "image-gemini",
			Family:      provider.FamilyImage,
			Label:       "Google Gemini / Imagen (Cloud)",
			Description: "Imagen and native Gemini image models, with shared-key support.",
			Source:      "gemini",
			Features:    []provider.Feature{provider.FeatureKeyRequired, provider.FeatureAutoGenerate},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.ImageBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return media.NewGeminiImageProvider(payload.Config, payload.SharedKey)
		},
	})
}
