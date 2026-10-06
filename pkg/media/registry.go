package media

import (
	"sync"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// TTSRegistry holds one TTS client per configured name, built lazily and cached
// so a named provider's model loads once.
type TTSRegistry struct {
	cfg    *config.Config
	logger trace.Logger
	mu     sync.Mutex
	byName map[string]TTSClient
}

// NewTTSRegistry builds an empty registry for cfg.
func NewTTSRegistry(cfg *config.Config, logger trace.Logger) *TTSRegistry {
	return &TTSRegistry{cfg: cfg, logger: trace.OrNil(logger), byName: map[string]TTSClient{}}
}

// For returns the client for a name, or the default when the name is empty or
// unknown.
func (r *TTSRegistry) For(name string) (TTSClient, error) {
	cacheKey := name
	if cacheKey == "" {
		cacheKey = config.ReservedProviderName
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if client, ok := r.byName[cacheKey]; ok {
		return client, nil
	}
	cfg := r.cfg.Media.TTSFor(name)
	pkey, ok := TTSKeyFor(cfg)
	client, err := NewTTSClientWithSharedKey(cfg, SharedProviderKey(r.cfg, pkey, ok))
	if err != nil {
		return nil, err
	}
	r.byName[cacheKey] = client
	return client, nil
}

// Default returns the client for the singleton configuration.
func (r *TTSRegistry) Default() (TTSClient, error) { return r.For("") }

// Names returns every configured name, default first.
func (r *TTSRegistry) Names() []string { return r.cfg.Media.TTSNames() }

// Invalidate drops every cached client, so a settings change rebuilds them.
func (r *TTSRegistry) Invalidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byName = map[string]TTSClient{}
}

// STTRegistry holds one STT client per configured name.
type STTRegistry struct {
	cfg    *config.Config
	mu     sync.Mutex
	byName map[string]STTClient
}

// NewSTTRegistry builds an empty registry for cfg.
func NewSTTRegistry(cfg *config.Config) *STTRegistry {
	return &STTRegistry{cfg: cfg, byName: map[string]STTClient{}}
}

// For returns the client for a name, or the default when the name is empty or
// unknown.
func (r *STTRegistry) For(name string) (STTClient, error) {
	cacheKey := name
	if cacheKey == "" {
		cacheKey = config.ReservedProviderName
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if client, ok := r.byName[cacheKey]; ok {
		return client, nil
	}
	cfg := r.cfg.Media.STTFor(name)
	pkey, ok := STTKeyFor(cfg)
	client, err := NewSTTClientWithSharedKey(cfg, SharedProviderKey(r.cfg, pkey, ok))
	if err != nil {
		return nil, err
	}
	r.byName[cacheKey] = client
	return client, nil
}

// Default returns the client for the singleton configuration.
func (r *STTRegistry) Default() (STTClient, error) { return r.For("") }

// Names returns every configured name, default first.
func (r *STTRegistry) Names() []string { return r.cfg.Media.STTNames() }

// Invalidate drops every cached client.
func (r *STTRegistry) Invalidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byName = map[string]STTClient{}
}

// ImageRegistry holds one image client per configured name.
type ImageRegistry struct {
	cfg    *config.Config
	logger trace.Logger
	mu     sync.Mutex
	byName map[string]ImageClient
}

// NewImageRegistry builds an empty registry for cfg.
func NewImageRegistry(cfg *config.Config, logger trace.Logger) *ImageRegistry {
	return &ImageRegistry{cfg: cfg, logger: trace.OrNil(logger), byName: map[string]ImageClient{}}
}

// For returns the client for a name, or the default when the name is empty or
// unknown.
func (r *ImageRegistry) For(name string) (ImageClient, error) {
	cacheKey := name
	if cacheKey == "" {
		cacheKey = config.ReservedProviderName
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if client, ok := r.byName[cacheKey]; ok {
		return client, nil
	}
	cfg := r.cfg.Media.ImageFor(name)
	pkey, ok := ImageKeyFor(cfg)
	client, err := NewSceneImageClientWithSharedKey(cfg, SharedProviderKey(r.cfg, pkey, ok), r.logger)
	if err != nil {
		return nil, err
	}
	r.byName[cacheKey] = client
	return client, nil
}

// Default returns the client for the singleton configuration.
func (r *ImageRegistry) Default() (ImageClient, error) { return r.For("") }

// Names returns every configured name, default first.
func (r *ImageRegistry) Names() []string { return r.cfg.Media.ImageNames() }

// Invalidate drops every cached client.
func (r *ImageRegistry) Invalidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byName = map[string]ImageClient{}
}
