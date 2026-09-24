// Package ttselevenlabs registers the ElevenLabs speech provider.
package ttselevenlabs

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "tts-elevenlabs",
			Family:      provider.FamilyTTS,
			Label:       "ElevenLabs (Cloud, metered)",
			Description: "Cloud voices fetched from your account; charges per request.",
			Source:      "builtin",
			Features: []provider.Feature{
				provider.FeatureMetered,
				provider.FeatureKeyRequired,
				provider.FeatureVoiceCatalog,
				provider.FeatureVoiceOptions,
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return media.NewElevenLabsTTSProvider(payload.Config)
		},
	})
}
