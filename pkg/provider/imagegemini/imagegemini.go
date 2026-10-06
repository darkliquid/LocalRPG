// Package imagegemini registers the Google Gemini/Imagen image provider.
package imagegemini

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func geminiImagePreset(id string, order int, label, description, model string) provider.Preset {
	return provider.Preset{
		ID: id, Order: order, Label: label, Description: description,
		Config: map[string]interface{}{
			"type": "gemini", "model": model, "aspect_ratio": "16:9",
			"person_generation": "ALLOW_ADULT", "auto_generate": false,
		},
	}
}

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyImageGemini),
			Family:      provider.FamilyImage,
			Label:       "Google Gemini / Imagen (Cloud)",
			Description: "Imagen and native Gemini image models, with shared-key support.",
			Source:      "gemini",
			Tier:        provider.TierCloud,
			Features:    []provider.Feature{provider.FeatureKeyRequired, provider.FeatureAutoGenerate},
			Presets: []provider.Preset{
				geminiImagePreset("imagen-3", 7, "Google Imagen 3 (Cloud API)",
					"High-fidelity cinematic and dark fantasy illustration via Google Imagen 3.0.",
					"imagen-3.0-generate-002"),
				geminiImagePreset("imagen-3-fast", 8, "Google Imagen 3 Fast (Cloud API)",
					"Rapid turnaround low-latency generation for turn-by-turn scene updates.",
					"imagen-3.0-fast-generate-001"),
				geminiImagePreset("nano-banana-2", 9, "Google Nano Banana 2 (Gemini 3.1 Flash Image)",
					"Generalist native Gemini image model balancing speed, 4K rendering, and scene consistency.",
					"gemini-3.1-flash-image"),
				geminiImagePreset("nano-banana-2-lite", 10, "Google Nano Banana 2 Lite (Gemini 3.1 Flash-Lite Image)",
					"Fastest and most lightweight native Gemini image generator.",
					"gemini-3.1-flash-lite-image"),
				geminiImagePreset("nano-banana-pro", 11, "Google Nano Banana Pro (Gemini 3 Pro Image)",
					"Complex visual composition, deep world knowledge, and fine creative steering.",
					"gemini-3-pro-image"),
				geminiImagePreset("nano-banana", 12, "Google Nano Banana Original (Gemini 2.5 Flash Image)",
					"Original high-volume low-latency Gemini image generator.",
					"gemini-2.5-flash-image"),
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.ImageBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewGeminiImageClient(payload.Config, payload.SharedKey)
		},
	})
}
