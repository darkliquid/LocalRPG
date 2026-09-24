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
			ID:          "image-cli",
			Family:      provider.FamilyImage,
			Label:       "Image CLI",
			Description: "Runs an image binary such as stable-diffusion.cpp.",
			Source:      "cli",
			Features:    []provider.Feature{provider.FeatureOffline},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.ImageBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return media.NewCLIImageProvider(payload.Config), nil
		},
	})
}
