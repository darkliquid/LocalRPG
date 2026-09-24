// Package sttwhisperhttp registers the HTTP transcription provider.
package sttwhisperhttp

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "stt-whisper-http",
			Family:      provider.FamilySTT,
			Label:       "Whisper (HTTP)",
			Description: "OpenAI-compatible transcription endpoint, local or cloud.",
			Source:      "http",
			Features:    []provider.Feature{provider.FeatureKeyRequired},
			Presets: []provider.Preset{
				{ID: "faster-whisper", Order: 2, Label: "Faster-Whisper (Local HTTP)",
					Description: "Local OpenAI-compatible transcription server running on port 8000.",
					Config: map[string]interface{}{
						"type": "http", "endpoint": "http://localhost:8000/v1/audio/transcriptions",
						"model": "whisper-1",
					}},
				{ID: "openai-whisper", Order: 4, Label: "OpenAI Whisper (Cloud API)",
					Description: "Cloud transcription via OpenAI Whisper API.",
					Config: map[string]interface{}{
						"type": "http", "endpoint": "https://api.openai.com/v1/audio/transcriptions",
						"model": "whisper-1",
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
			return NewHTTPSTTClient(cfg), nil
		},
	})
}
