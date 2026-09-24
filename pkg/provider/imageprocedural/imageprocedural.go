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
			ID:          "image-procedural-art",
			Family:      provider.FamilyImage,
			Label:       "Procedural Dark Fantasy (Built-in)",
			Description: "Pure-Go vector landscape generator that needs no model or network.",
			Source:      "builtin",
			Features:    []provider.Feature{provider.FeatureOffline},
		},
		Build: func(_ context.Context, _ []byte) (interface{}, error) {
			return media.NewProceduralImageProvider(), nil
		},
	})
}
