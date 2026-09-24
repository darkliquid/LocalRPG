// Package sttwhispercli registers the command-line transcription provider.
package sttwhispercli

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
			ID:          "stt-whisper-cli",
			Family:      provider.FamilySTT,
			Label:       "Whisper.cpp (CLI)",
			Description: "Runs whisper-cli directly with a GGML model.",
			Source:      "cli",
			Features:    []provider.Feature{provider.FeatureOffline},
			Presets: []provider.Preset{
				{ID: "whisper-cli", Order: 3, Label: "Whisper.cpp (Local CLI)",
					Description: "Whisper.cpp command-line tool with GGML model.",
					Config: map[string]interface{}{
						"type": "cli", "command": "whisper-cli",
						"args": []interface{}{"-m", "models/ggml-base.bin", "-f", "%INPUT%", "-nt"},
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var cfg config.STTConfig
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &cfg); err != nil {
					return nil, err
				}
			}
			return media.NewCLISTTProvider(cfg), nil
		},
	})
}
