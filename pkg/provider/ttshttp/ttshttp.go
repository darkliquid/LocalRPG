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
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return media.NewHTTPTTSProvider(payload.Config), nil
		},
	})
}
