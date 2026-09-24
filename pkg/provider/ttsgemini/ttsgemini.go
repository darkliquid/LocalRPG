// Package ttsgemini registers the Google Gemini speech provider.
package ttsgemini

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "tts-gemini",
			Family:      provider.FamilyTTS,
			Label:       "Google Gemini TTS (Cloud, metered)",
			Description: "Expressive cloud synthesis with prebuilt and extended voices.",
			Source:      "gemini",
			Features: []provider.Feature{
				provider.FeatureMetered,
				provider.FeatureKeyRequired,
				provider.FeatureVoiceCatalog,
				provider.FeatureVoiceOptions,
				provider.FeatureSpeechCues,
				provider.FeatureExtendedVoices,
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return media.NewGeminiTTSProvider(payload.Config, payload.SharedKey)
		},
	})
}
