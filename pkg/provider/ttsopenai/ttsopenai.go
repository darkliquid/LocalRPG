// Package ttsopenai registers the OpenAI speech (audio/speech) provider.
package ttsopenai

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyTTSOpenAI),
			Family:      provider.FamilyTTS,
			Label:       "OpenAI Speech",
			Description: "Cloud speech with a fixed 13-voice catalogue and delivery instructions. Metered per character.",
			Source:      "http",
			Tier:        provider.TierCloud,
			Caveat:      "The /v1/audio/speech endpoint is deprecated by OpenAI with shutdown announced for 2027-01-06.",
			Features:    []provider.Feature{provider.FeatureKeyRequired, provider.FeatureVoiceCatalog, provider.FeatureVoiceOptions, provider.FeatureMetered},
			Presets: []provider.Preset{
				{ID: "openai", Order: 5, Label: "OpenAI Speech (Cloud API)",
					Description: "Cloud speech with a fixed 13-voice catalogue and delivery instructions. Metered per character.",
					Config: map[string]any{
						"type": "openai", "model": "gpt-4o-mini-tts", "default_voice": "marin",
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
			return NewClient(payload.Config, payload.SharedKey), nil
		},
	})
}
