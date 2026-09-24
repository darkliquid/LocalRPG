// Package clillm registers the command-line chat provider.
package clillm

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "cli",
			Family:      provider.FamilyLLM,
			Label:       "Command Line (CLI)",
			Description: "Runs a local binary such as llama-cli or claude and reads its output.",
			Source:      "cli",
			Features:    []provider.Feature{provider.FeatureStreaming},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var cfg harness.ProviderConfig
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return nil, err
			}
			return harness.NewCLIModelProvider("cli", cfg), nil
		},
	})
}
