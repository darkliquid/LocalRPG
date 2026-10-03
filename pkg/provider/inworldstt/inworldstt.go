// Package inworldstt registers the Inworld AI Speech-to-Text provider.
package inworldstt

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeySTTInworld),
			Family:      provider.FamilySTT,
			Label:       "Inworld STT (Cloud, metered)",
			Description: "Cloud speech recognition with voice profiling using inworld/inworld-stt-1.",
			Source:      "http",
			Features: []provider.Feature{
				provider.FeatureMetered,
				provider.FeatureKeyRequired,
			},
			Presets: []provider.Preset{
				{
					ID:          "inworld-stt",
					Order:       5,
					Label:       "Inworld STT",
					Description: "Cloud transcription via Inworld STT. Set key in Providers tab or via INWORLD_API_KEY.",
					Config: map[string]interface{}{
						"type":         "builtin",
						"builtin_name": "inworld",
						"model":        "inworld/inworld-stt-1",
					},
				},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.STTBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewInworldSTTClient(payload.Config, payload.SharedKey)
		},
	})
}
