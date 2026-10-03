// Package ttscartesia registers the Cartesia Sonic text-to-speech provider.
package ttscartesia

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyTTSCartesia),
			Family:      provider.FamilyTTS,
			Label:       "Cartesia Sonic (Cloud, metered)",
			Description: "Ultra-fast neural voice synthesis via Cartesia Sonic.",
			Source:      "http",
			Features: []provider.Feature{
				provider.FeatureMetered,
				provider.FeatureKeyRequired,
				provider.FeatureVoiceCatalog,
				provider.FeatureVoiceOptions,
				provider.FeatureSpeechCues,
			},
			Presets: []provider.Preset{
				{
					ID:          "cartesia",
					Order:       8,
					Label:       "Cartesia Sonic (Cloud, metered)",
					Description: "Fast, natural speech via Cartesia Sonic. Set key here or via CARTESIA_API_KEY; charges per request.",
					Config: map[string]interface{}{
						"type":          "builtin",
						"builtin_name":  "cartesia",
						"model":         "sonic-3.6",
						"default_voice": "db6b0ed5-d5d3-463d-ae85-518a07d3c2b4",
						"pitch":         1.0,
						"speech_rate":   1.0,
						"master_volume": 1.0,
						"auto_play":     true,
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
			return NewCartesiaTTSClient(payload.Config, payload.SharedKey)
		},
	})
}
