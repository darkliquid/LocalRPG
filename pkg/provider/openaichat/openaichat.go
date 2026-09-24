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
			Presets: []provider.Preset{
				{ID: "ollama", Order: 1, Label: "Ollama (Local HTTP)",
					Description: "Connects to local Ollama server running on port 11434 with llama3.2.",
					Config: map[string]interface{}{
						"type": "http", "endpoint": "http://localhost:11434/v1",
						"model": "llama3.2", "temperature": 0.7, "max_tokens": 1024,
					}},
				{ID: "lm-studio", Order: 2, Label: "LM Studio (Local HTTP)",
					Description: "Connects to LM Studio local server on port 1234.",
					Config: map[string]interface{}{
						"type": "http", "endpoint": "http://localhost:1234/v1",
						"model": "default", "temperature": 0.7, "max_tokens": 1024,
					}},
				{ID: "localai", Order: 3, Label: "LocalAI (Local HTTP)",
					Description: "Connects to LocalAI server on port 8080.",
					Config: map[string]interface{}{
						"type": "http", "endpoint": "http://localhost:8080/v1",
						"model": "gpt-4", "temperature": 0.7, "max_tokens": 1024,
					}},
				{ID: "vllm", Order: 4, Label: "vLLM (Local HTTP)",
					Description: "Connects to high-throughput vLLM instance on port 8000.",
					Config: map[string]interface{}{
						"type": "http", "endpoint": "http://localhost:8000/v1",
						"model": "default", "temperature": 0.7, "max_tokens": 1024,
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var cfg harness.ProviderConfig
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &cfg); err != nil {
					return nil, err
				}
			}
			return harness.NewOpenAIChatProvider("openaichat", cfg), nil
		},
	})
}
