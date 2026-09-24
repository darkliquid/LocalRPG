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
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return media.NewSherpaTTSProvider(payload.Config), nil
		},
	})
}
