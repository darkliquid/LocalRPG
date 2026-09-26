// Package ttsnativeos registers the host-OS speech provider.
package ttsnativeos

import (
	"context"

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
			Presets: []provider.Preset{
				{ID: "native-os", Order: 5, Label: "Native OS Speech (Built-in Fallback)",
					Description: "Uses spd-say (Linux), say (macOS), or PowerShell (Windows) with procedural audio fallback.",
					Config: map[string]any{
						"type": "builtin", "builtin_name": "native-os",
						"pitch": 1.0, "speech_rate": 1.0, "auto_play": true, "master_volume": 1.0,
					}},
			},
		},
		Build: func(_ context.Context, _ []byte) (any, error) {
			return NewNativeOSTTSClient(), nil
		},
	})
}
