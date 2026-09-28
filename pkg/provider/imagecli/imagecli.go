// Package imagecli registers the command-line image provider.
package imagecli

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyImageCLI),
			Family:      provider.FamilyImage,
			Label:       "Image CLI",
			Description: "Runs an image binary such as stable-diffusion.cpp.",
			Source:      "cli",
			Features:    []provider.Feature{provider.FeatureOffline},
			Presets: []provider.Preset{
				{ID: "sd-cli", Order: 4, Label: "stable-diffusion.cpp (Local CLI)",
					Description: "Direct SD inference binary using quantized GGUF weights.",
					Config: map[string]interface{}{
						"type": "cli", "command": "sd",
						"args": []interface{}{"-m", "models/sd-v1-5.gguf", "-p"}, "auto_generate": false,
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.ImageBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewCLIImageClient(payload.Config.Command, payload.Config.Args), nil
		},
	})
}
