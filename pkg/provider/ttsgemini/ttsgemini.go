// Package ttsgemini registers the Google Gemini speech provider.
package ttsgemini

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func geminiPreset(id string, order int, label, description, model string) provider.Preset {
	return provider.Preset{
		ID: id, Order: order, Label: label, Description: description,
		Config: map[string]interface{}{
			"type": "gemini", "model": model, "default_voice": "Aoede",
			"pitch": 1.0, "speech_rate": 1.0, "auto_play": true, "master_volume": 1.0,
		},
	}
}

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
			Presets: []provider.Preset{
				geminiPreset("gemini-3.8-flash-tts", 8, "Google Gemini 3.8 Flash TTS",
					"Most expressive audio model with deep creative direction and character design.",
					"gemini-3.8-flash-tts"),
				geminiPreset("gemini-3.8-flash-lite-tts", 9, "Google Gemini 3.8 Flash-Lite TTS",
					"High-volume, cost-efficient expressive voice generation with low latency.",
					"gemini-3.8-flash-lite-tts"),
				geminiPreset("gemini-3.1-flash-tts", 10, "Google Gemini 3.1 Flash TTS (Preview)",
					"Fast, natural cloud TTS with 30 prebuilt voices and audio tags support.",
					"gemini-3.1-flash-tts-preview"),
				geminiPreset("gemini-2.5-flash-tts", 11, "Google Gemini 2.5 Flash TTS (Preview)",
					"Lightweight cloud TTS with natural prosody and voice acting cues.",
					"gemini-2.5-flash-preview-tts"),
				geminiPreset("gemini-2.5-pro-tts", 12, "Google Gemini 2.5 Pro TTS (Preview)",
					"Highest quality expressive cloud TTS for nuanced storytelling.",
					"gemini-2.5-pro-preview-tts"),
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
