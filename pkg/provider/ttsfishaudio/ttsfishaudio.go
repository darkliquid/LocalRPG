package ttsfishaudio

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyTTSFishAudio),
			Family:      provider.FamilyTTS,
			Label:       "Fish Audio S2 (vLLM-Omni)",
			Description: "Dual-AR speech synthesis with fine-grained emotional tags and zero-shot voice cloning.",
			Source:      "http",
			Tier:        provider.TierLocalServer,
			Features: []provider.Feature{
				provider.FeatureVoiceCatalog,
				provider.FeatureVoiceOptions,
				provider.FeatureSpeechCues,
				provider.FeatureOffline,
			},
			Presets: []provider.Preset{
				{
					ID:          "fish-audio-s2-vllm",
					Order:       5,
					Label:       "Fish Audio S2 Pro (Local vLLM-Omni)",
					Description: "Local Fish Audio S2 Pro running via vLLM-Omni on port 8091.",
					Config: map[string]interface{}{
						"type":          "http",
						"endpoint":      "http://localhost:8091",
						"model":         "fishaudio/s2-pro",
						"default_voice": "default",
						"pitch":         1.0,
						"speech_rate":   1.0,
						"auto_play":     true,
						"master_volume": 1.0,
					},
				},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewFishAudioTTSClient(payload.Config), nil
		},
	})
}
