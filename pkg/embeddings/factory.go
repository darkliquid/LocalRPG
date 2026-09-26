package embeddings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// NewProviderFromConfig constructs an embedding provider from config.
func NewProviderFromConfig(cfg config.EmbeddingsConfig) (Provider, error) {
	if !cfg.Enabled || cfg.Provider == "disabled" || cfg.Provider == "" {
		return nil, nil
	}

	pCfg, ok := cfg.Providers[cfg.Provider]
	if !ok {
		// Fallback to builtin-local if named provider missing
		dims := cfg.Dimensions
		if dims <= 0 {
			dims = 384
		}
		return NewBuiltinHashProjectionProvider(dims), nil
	}

	switch pCfg.Type {
	case "builtin":
		dims := cfg.Dimensions
		if dims <= 0 {
			dims = 384
		}
		return NewBuiltinHashProjectionProvider(dims), nil
	case "http":
		reg, ok := provider.Lookup("openai-embedding")
		if !ok {
			return nil, fmt.Errorf("provider openai-embedding not registered")
		}
		url := pCfg.URL
		if url == "" {
			url = pCfg.Endpoint
		}
		model := pCfg.Model
		if model == "" {
			model = cfg.Model
		}
		raw, _ := json.Marshal(map[string]any{
			"endpoint":   url,
			"api_key":    pCfg.APIKey,
			"model":      model,
			"dimensions": cfg.Dimensions,
		})
		inst, err := reg.Build(context.Background(), raw)
		if err != nil {
			return nil, err
		}
		return inst.(Provider), nil
	case "gemini":
		reg, ok := provider.Lookup("gemini-embedding")
		if !ok {
			return nil, fmt.Errorf("provider gemini-embedding not registered")
		}
		model := pCfg.Model
		if model == "" {
			model = cfg.Model
		}
		raw, _ := json.Marshal(map[string]any{
			"api_key": pCfg.APIKey,
			"model":   model,
		})
		inst, err := reg.Build(context.Background(), raw)
		if err != nil {
			return nil, err
		}
		return inst.(Provider), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider type: %s", pCfg.Type)
	}
}
