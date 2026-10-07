package embeddings

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/trace"
)

var (
	modelDirMu  sync.RWMutex
	modelDir    string
	loggerMu    sync.RWMutex
	eventLogger trace.Logger = trace.Nop()
)

// SetModelDir records where local embedding models are cached. The factory uses
// it for a provider whose config names no model path of its own. It is set once
// at startup by the application.
func SetModelDir(dir string) {
	modelDirMu.Lock()
	defer modelDirMu.Unlock()
	modelDir = dir
}

// ModelDir returns the configured local model directory, or an empty string
// when none was set.
func ModelDir() string {
	modelDirMu.RLock()
	defer modelDirMu.RUnlock()
	return modelDir
}

// SetLogger installs the logger the factory reports model events through, such
// as a missing local encoder.
func SetLogger(logger trace.Logger) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	eventLogger = trace.OrNil(logger)
}

func logEvent(name string, fields map[string]interface{}) {
	loggerMu.RLock()
	logger := eventLogger
	loggerMu.RUnlock()
	logger.Event(name, fields)
}

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
	case "onnx":
		return onnxFromConfig(cfg, pCfg)
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

// onnxFromConfig builds the local ONNX encoder, falling back to the hash
// projection when its runtime or model is unavailable, so a search still answers
// and the client can prompt a download.
func onnxFromConfig(cfg config.EmbeddingsConfig, pCfg config.EmbeddingProviderConfig) (Provider, error) {
	dims := cfg.Dimensions
	if dims <= 0 {
		dims = 384
	}
	dir := pCfg.ModelPath
	if dir == "" {
		dir = ModelDir()
	}
	p, err := NewONNXProvider(dir)
	if err != nil {
		logEvent("model_missing", map[string]interface{}{
			"provider": string(provider.KeyEmbeddingONNX),
			"model":    models.EmbeddingEncoderModelID,
			"reason":   err.Error(),
		})
		return NewBuiltinHashProjectionProvider(dims), nil
	}
	return p, nil
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
		return provider.InstanceOrSelf(provider.KeyEmbeddingBuiltin, provider.InstanceDiscriminator(pCfg.Instance, "default")), true
	case "onnx":
		return provider.InstanceOrSelf(provider.KeyEmbeddingONNX, provider.InstanceDiscriminator(pCfg.Instance, "default")), true
	case "gemini":
		return provider.InstanceOrSelf(provider.KeyEmbeddingGemini, provider.InstanceDiscriminator(pCfg.Instance, "default")), true
	case "http":
		endpoint := pCfg.URL
		if endpoint == "" {
			endpoint = pCfg.Endpoint
		}
		disc := provider.HostDiscriminator(endpoint)
		if disc == "" {
			disc = "default"
		}
		return provider.InstanceOrSelf(provider.KeyEmbeddingOpenAI, provider.InstanceDiscriminator(pCfg.Instance, disc)), true
	}
	return "", false
}
