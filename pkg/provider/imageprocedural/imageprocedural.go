// Package imageprocedural registers the built-in procedural art provider.
package imageprocedural

import (
	"context"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyImageProceduralArt),
			Family:      provider.FamilyImage,
			Label:       "Procedural Dark Fantasy (Built-in)",
			Description: "Pure-Go vector landscape generator that needs no model or network.",
			Source:      "builtin",
			Tier:        provider.TierOfflineBasic,
			Features:    []provider.Feature{provider.FeatureOffline},
			Presets: []provider.Preset{
				{ID: "procedural-art", Order: 5, Label: "Procedural Dark Fantasy (Built-in Zero-GPU)",
					Description: "Pure-Go vector landscape and fortress generator creating atmospheric SVG illustrations.",
					Config: map[string]interface{}{
						"type": "builtin", "builtin_name": "procedural-art", "auto_generate": false,
					}},
			},
		},
		Build: func(_ context.Context, _ []byte) (interface{}, error) {
			return media.NewProceduralImageProvider(), nil
		},
	})
}
