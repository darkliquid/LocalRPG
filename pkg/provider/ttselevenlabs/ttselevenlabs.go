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
			Presets: []provider.Preset{
				{ID: "elevenlabs", Order: 7, Label: "ElevenLabs (Cloud, metered)",
					Description: "Cloud voices fetched from your account. Set a key here or via ELEVENLABS_API_KEY; charges per request.",
					Config: map[string]any{
						"type": "builtin", "builtin_name": "elevenlabs",
						"model": "eleven_multilingual_v2", "default_voice": "EXAVITQu4vr4xnSDxMaL",
						"pitch": 1.0, "speech_rate": 1.0, "auto_play": true, "master_volume": 1.0,
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (any, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewElevenLabsTTSClient(payload.Config)
		},
	})
}
