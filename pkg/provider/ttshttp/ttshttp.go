// Package ttshttp registers the OpenAI-compatible HTTP speech provider.
package ttshttp

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "tts-openai-http",
			Family:      provider.FamilyTTS,
			Label:       "OpenAI-Compatible Speech (HTTP)",
			Description: "Any OpenAI-compatible speech endpoint, local or cloud.",
			Source:      "http",
			Features:    []provider.Feature{provider.FeatureKeyRequired},
			Presets: []provider.Preset{
				{ID: "kokoro-fastapi", Order: 2, Label: "Kokoro-FastAPI (Local HTTP)",
					Description: "High quality 82M open-weights TTS running via local FastAPI server on port 8880.",
					Config: map[string]any{
						"type": "http", "endpoint": "http://localhost:8880", "model": "kokoro",
						"default_voice": "af_bella", "pitch": 1.0, "speech_rate": 1.0,
						"auto_play": true, "master_volume": 1.0,
						"voice_profiles": media.KokoroVoiceProfiles(),
					}},
				{ID: "alltalk", Order: 3, Label: "AllTalk TTS (Local HTTP)",
					Description: "Coqui XTTSv2 / AllTalk web UI API running on port 7851.",
					Config: map[string]any{
						"type": "http", "endpoint": "http://localhost:7851/api/tts-generate",
						"default_voice": "default", "pitch": 1.0, "speech_rate": 1.0,
						"auto_play": true, "master_volume": 1.0,
					}},
				{ID: "openai-speech", Order: 6, Label: "OpenAI Audio Speech (Cloud API)",
					Description: "Cloud synthesis with OpenAI tts-1 model.",
					Config: map[string]any{
						"type": "http", "endpoint": "https://api.openai.com/v1/audio/speech",
						"model": "tts-1", "default_voice": "alloy",
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
			return NewHTTPTTSClient(payload.Config), nil
		},
	})
}
