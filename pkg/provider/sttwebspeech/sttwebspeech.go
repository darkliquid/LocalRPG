// Package sttwebspeech registers the browser-native speech recognition provider.
package sttwebspeech

import (
	"context"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeySTTWebSpeech),
			Family:      provider.FamilySTT,
			Label:       "Web Speech API (Browser Native)",
			Description: "Real-time in-browser recognition; the backend client is a placeholder.",
			Source:      "builtin",
			Features:    []provider.Feature{provider.FeatureOffline},
			Presets: []provider.Preset{
				{ID: "web-speech", Order: 1, Label: "Web Speech API (Browser Native)",
					Description: "Zero-setup, real-time in-browser speech recognition without a background server.",
					Config: map[string]interface{}{
						"type": "web-speech",
					}},
			},
		},
		Build: func(_ context.Context, _ []byte) (interface{}, error) {
			return media.NewWebSpeechSTTProvider(), nil
		},
	})
}
