// Package ttssherpa registers the Sherpa-ONNX Kokoro speech provider.
package ttssherpa

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "tts-sherpa-onnx",
			Family:      provider.FamilyTTS,
			Label:       "Sherpa-ONNX Kokoro (Built-in)",
			Description: "High-quality Kokoro TTS in-process, downloading the model on demand.",
			Source:      "builtin",
			Features:    []provider.Feature{provider.FeatureOffline, provider.FeatureVoiceCatalog},
			Presets: []provider.Preset{
				{ID: "sherpa-onnx", Order: 1, Label: "Sherpa-ONNX Kokoro (Built-in Neural TTS)",
					Description: "High-quality Kokoro TTS running in-process via Sherpa-ONNX (downloads model on demand).",
					Config: map[string]interface{}{
						"type": "builtin", "builtin_name": "sherpa-onnx",
						"default_voice": "af_bella", "pitch": 1.0, "speech_rate": 1.0,
						"auto_play": true, "master_volume": 1.0,
						"voice_profiles": media.KokoroVoiceProfiles(),
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
			return NewSherpaTTSClient(payload.Config.ModelPath), nil
		},
	})
}
