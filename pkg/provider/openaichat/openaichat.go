// Package openaichat registers the OpenAI-compatible HTTP chat provider.
package openaichat

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "openaichat",
			Family:      provider.FamilyLLM,
			Label:       "OpenAI-Compatible (HTTP)",
			Description: "Any OpenAI-compatible chat completions endpoint, local or cloud.",
			Source:      "http",
			Features:    []provider.Feature{provider.FeatureStreaming, provider.FeatureTools},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var cfg harness.ProviderConfig
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return nil, err
			}
			return harness.NewOpenAIChatProvider("openaichat", cfg), nil
		},
	})
}
