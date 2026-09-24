// Package ttspiper registers the command-line Piper speech provider.
package ttspiper

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "tts-piper",
			Family:      provider.FamilyTTS,
			Label:       "Piper TTS (CLI)",
			Description: "Fast, lightweight neural TTS via the piper binary.",
			Source:      "cli",
			Features:    []provider.Feature{provider.FeatureOffline},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return media.NewCLITTSProvider(payload.Config), nil
		},
	})
}
