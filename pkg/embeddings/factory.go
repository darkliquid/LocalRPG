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
		reg, ok := provider.Lookup(string(provider.KeyEmbeddingOpenAI))
		if !ok {
			return nil, fmt.Errorf("provider %s not registered", provider.KeyEmbeddingOpenAI)
		}
		url := pCfg.URL
		if url == "" {
			url = pCfg.Endpoint
		}
		model := pCfg.Model
		if model == "" {
			model = cfg.Model
		}
		raw, _ := json.Marshal(map[string]interface{}{
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
		reg, ok := provider.Lookup(string(provider.KeyEmbeddingGemini))
		if !ok {
			return nil, fmt.Errorf("provider %s not registered", provider.KeyEmbeddingGemini)
		}
		model := pCfg.Model
		if model == "" {
			model = cfg.Model
		}
		raw, _ := json.Marshal(map[string]interface{}{
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

// KeyFor maps an embeddings configuration to its canonical key. The
// discriminator is the endpoint host, or the literal "default" when no endpoint
// is named. ok is false for disabled or unconfigured embeddings.
func KeyFor(cfg config.EmbeddingsConfig) (provider.Key, bool) {
	if !cfg.Enabled || cfg.Provider == "" || cfg.Provider == "disabled" {
		return "", false
	}
	pCfg, ok := cfg.Providers[cfg.Provider]
	if !ok {
		return provider.InstanceOrSelf(provider.KeyEmbeddingBuiltin, "default"), true
	}
	switch pCfg.Type {
	case "", "builtin":
		return provider.InstanceOrSelf(provider.KeyEmbeddingBuiltin, "default"), true
	case "gemini":
		return provider.InstanceOrSelf(provider.KeyEmbeddingGemini, "default"), true
	case "http":
		endpoint := pCfg.URL
		if endpoint == "" {
			endpoint = pCfg.Endpoint
		}
		disc := provider.HostDiscriminator(endpoint)
		if disc == "" {
			disc = "default"
		}
		return provider.InstanceOrSelf(provider.KeyEmbeddingOpenAI, disc), true
	}
	return "", false
}
