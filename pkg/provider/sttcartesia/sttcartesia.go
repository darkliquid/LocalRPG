// Package sttcartesia registers the Cartesia Ink speech-to-text provider.
package sttcartesia

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeySTTCartesia),
			Family:      provider.FamilySTT,
			Label:       "Cartesia Ink (Cloud, metered)",
			Description: "Cloud speech transcription via Cartesia Ink Whisper.",
			Source:      "http",
			Features: []provider.Feature{
				provider.FeatureMetered,
				provider.FeatureKeyRequired,
			},
			Presets: []provider.Preset{
				{
					ID:          "cartesia",
					Order:       4,
					Label:       "Cartesia Ink (Cloud, metered)",
					Description: "Accurate cloud transcription with Cartesia Ink Whisper. Set key here or via CARTESIA_API_KEY; charges per request.",
					Config: map[string]interface{}{
						"type":         "builtin",
						"builtin_name": "cartesia",
						"model":        "ink-whisper",
					},
				},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.STTBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err == nil && (payload.Config.Type != "" || payload.SharedKey != "") {
					return NewCartesiaSTTClient(payload.Config, payload.SharedKey)
				}
				var cfg config.STTConfig
				if err := json.Unmarshal(raw, &cfg); err != nil {
					return nil, err
				}
				return NewCartesiaSTTClient(cfg, "")
			}
			return NewCartesiaSTTClient(config.STTConfig{}, "")
		},
	})
}
