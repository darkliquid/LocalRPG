// Package imagehttp registers the HTTP image provider (A1111, ComfyUI, LocalAI).
package imagehttp

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "image-http",
			Family:      provider.FamilyImage,
			Label:       "Image HTTP Endpoint",
			Description: "Stable Diffusion WebUI, ComfyUI, or any compatible image endpoint.",
			Source:      "http",
			Features:    []provider.Feature{provider.FeatureAutoGenerate},
			Presets: []provider.Preset{
				{ID: "comfyui", Order: 1, Label: "ComfyUI (Local HTTP)",
					Description: "Connects to local ComfyUI graph execution server on port 8188.",
					Config: map[string]any{
						"type": "http", "endpoint": "http://127.0.0.1:8188", "auto_generate": false,
					}},
				{ID: "automatic1111", Order: 2, Label: "Stable Diffusion WebUI / A1111 (Local HTTP)",
					Description: "Connects to AUTOMATIC1111 txt2img API on port 7860.",
					Config: map[string]any{
						"type": "http", "endpoint": "http://127.0.0.1:7860/sdapi/v1/txt2img", "auto_generate": false,
					}},
				{ID: "localai-image", Order: 3, Label: "LocalAI Image (Local HTTP)",
					Description: "LocalAI image generation endpoint on port 8080.",
					Config: map[string]any{
						"type": "http", "endpoint": "http://127.0.0.1:8080/v1/images/generations",
						"model": "stablediffusion", "auto_generate": false,
					}},
				{ID: "dall-e-3", Order: 6, Label: "OpenAI DALL-E 3 (Cloud API)",
					Description: "Cloud generation using OpenAI DALL-E 3 endpoint.",
					Config: map[string]any{
						"type": "http", "endpoint": "https://api.openai.com/v1/images/generations",
						"model": "dall-e-3", "auto_generate": false,
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
			return NewHTTPImageClient(payload.Config), nil
		},
	})
}
