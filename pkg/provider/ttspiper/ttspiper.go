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
			ID:          string(provider.KeyTTSPiper),
			Family:      provider.FamilyTTS,
			Label:       "Piper TTS (CLI)",
			Description: "Fast, lightweight neural TTS via the piper binary.",
			Source:      "cli",
			Tier:        provider.TierLocalServer,
			Features:    []provider.Feature{provider.FeatureOffline},
			Presets: []provider.Preset{
				{ID: "piper", Order: 4, Label: "Piper TTS (Local CLI)",
					Description: "Fast, lightweight neural TTS running directly via the piper binary.",
					Config: map[string]interface{}{
						"type": "cli", "command": "piper",
						"args":  []interface{}{"--model", "en_US-lessac-medium.onnx", "--output_file", "-"},
						"pitch": 1.0, "speech_rate": 1.0, "auto_play": true, "master_volume": 1.0,
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewCLITTSClient(payload.Config.Command, payload.Config.Args), nil
		},
	})
}
