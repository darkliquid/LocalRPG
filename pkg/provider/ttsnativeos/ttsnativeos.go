// Package ttsnativeos registers the host-OS speech provider.
package ttsnativeos

import (
	"context"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "tts-native-os",
			Family:      provider.FamilyTTS,
			Label:       "Native OS Speech (Built-in)",
			Description: "Uses spd-say, say, or PowerShell with a procedural fallback.",
			Source:      "builtin",
			Features:    []provider.Feature{provider.FeatureOffline},
		},
		Build: func(_ context.Context, _ []byte) (interface{}, error) {
			return media.NewNativeOSTTSProvider(), nil
		},
	})
}
