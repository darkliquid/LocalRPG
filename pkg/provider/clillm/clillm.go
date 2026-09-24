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
			Presets: []provider.Preset{
				{ID: "llama-cli", Order: 5, Label: "llama-cli (Local Executable)",
					Description: "Direct llama.cpp command execution without a background server.",
					Config: map[string]interface{}{
						"type": "cli", "command": "llama-cli",
						"args":        []interface{}{"-m", "models/model.gguf", "-p"},
						"temperature": 0.7, "max_tokens": 1024,
					}},
				{ID: "claude-cli", Order: 6, Label: "Claude Code CLI",
					Description: "Executes Anthropic Claude CLI directly from command line.",
					Config: map[string]interface{}{
						"type": "cli", "command": "claude",
						"args": []interface{}{"-p"},
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload harness.ModelBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			id := payload.ID
			if id == "" {
				id = "cli"
			}
			return NewCLIProviderWithOptions(id, payload.Config.Command, payload.Config.Args, harness.GenerationOptions{
				Temperature: payload.Config.Temperature,
				MaxTokens:   payload.Config.MaxTokens,
			}), nil
		},
	})
}
