// Package sttwhisperhttp registers the HTTP transcription provider.
package sttwhisperhttp

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
			ID:          "stt-whisper-http",
			Family:      provider.FamilySTT,
			Label:       "Whisper (HTTP)",
			Description: "OpenAI-compatible transcription endpoint, local or cloud.",
			Source:      "http",
			Features:    []provider.Feature{provider.FeatureKeyRequired},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var cfg config.STTConfig
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &cfg); err != nil {
					return nil, err
				}
			}
			return media.NewHTTPSTTProvider(cfg), nil
		},
	})
}
