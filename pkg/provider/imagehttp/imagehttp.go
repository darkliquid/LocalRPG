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
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.ImageBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return media.NewHTTPImageProvider(payload.Config), nil
		},
	})
}
