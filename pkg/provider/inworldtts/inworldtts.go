// Package inworldtts registers the Inworld AI Text-to-Speech provider.
package inworldtts

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyTTSInworld),
			Family:      provider.FamilyTTS,
			Label:       "Inworld TTS (Cloud, metered)",
			Description: "Natural-sounding dialogue and narration with inworld-tts-2.",
			Source:      "http",
			Tier:        provider.TierCloud,
			Features: []provider.Feature{
				provider.FeatureMetered,
				provider.FeatureKeyRequired,
				provider.FeatureVoiceCatalog,
			},
			Presets: []provider.Preset{
				{
					ID:          "inworld-tts",
					Order:       8,
					Label:       "Inworld TTS",
					Description: "Inworld Cloud TTS using inworld-tts-2. Set key in Providers tab or via INWORLD_API_KEY.",
					Config: map[string]interface{}{
						"type":          "builtin",
						"builtin_name":  "inworld",
						"model":         "inworld-tts-2",
						"default_voice": "Ashley",
						"pitch":         1.0,
						"speech_rate":   1.0,
						"auto_play":     true,
						"master_volume": 1.0,
					},
				},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewInworldTTSClient(payload.Config, payload.SharedKey)
		},
	})
}
