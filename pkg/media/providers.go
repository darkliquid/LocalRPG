package media

import (
	"context"
	"errors"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/trace"
)

var ErrProviderDisabled = errors.New("provider is disabled")

// Disabled implementations
type disabledTTSClient struct{}

func (d *disabledTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return nil, ErrProviderDisabled
}

// NewCacheOnlyTTSClient is a client that never synthesizes. It lets an export
// play the clips a campaign already has when no provider can be built (no key, no
// server), where a cache hit plays and a miss is a silent beat rather than a
// reason to skip every line.
func NewCacheOnlyTTSClient() TTSClient { return &disabledTTSClient{} }

type disabledSTTClient struct{}

func (d *disabledSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	return "", ErrProviderDisabled
}

type disabledImageClient struct{}

func (d *disabledImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	return nil, ErrProviderDisabled
}

// Builtin / Echo implementations
type echoTTSClient struct{}

func (e *echoTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	// A real, decodable tone: the pipeline normalises every clip to Opus, so the
	// built-in probe must return valid PCM rather than a placeholder string.
	return GenerateToneWAV(440, 0.1), nil
}

type echoSTTClient struct{}

func (e *echoSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	return "Transcribed audio sample", nil
}

type echoImageClient struct{}

func (e *echoImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	return []byte("fake-image-bytes-for-" + prompt), nil
}

func NewTTSClient(cfg config.TTSConfig) (TTSClient, error) {
	return NewTTSClientWithSharedKey(cfg, "")
}

// NewTTSClientWithSharedKey builds a TTSClient from configuration and an optional shared key.
func NewTTSClientWithSharedKey(cfg config.TTSConfig, sharedKey string) (TTSClient, error) {
	// The registry is authoritative when the binary imported pkg/provider/all;
	// otherwise the inline switch below still builds the client.
	if key, ok := TTSKeyFor(cfg); ok {
		if _, found := provider.Lookup(string(key.Parent())); found {
			return BuildTTS(string(key.Parent()), cfg, sharedKey)
		}
	}
	switch cfg.Type {
	case "disabled", "":
		return &disabledTTSClient{}, nil
	case "builtin":
		return &echoTTSClient{}, nil
	default:
		return nil, fmt.Errorf("unsupported tts provider type: %s", cfg.Type)
	}
}

func NewSTTClient(cfg config.STTConfig) (STTClient, error) {
	return NewSTTClientWithSharedKey(cfg, "")
}

// NewSTTClientWithSharedKey builds an STTClient from configuration and an optional shared key.
func NewSTTClientWithSharedKey(cfg config.STTConfig, sharedKey string) (STTClient, error) {
	// Registry-first when pkg/provider/all was imported; inline otherwise.
	if key, ok := STTKeyFor(cfg); ok {
		if _, found := provider.Lookup(string(key.Parent())); found {
			return BuildSTT(string(key.Parent()), cfg, sharedKey)
		}
	}
	switch cfg.Type {
	case "disabled", "":
		return &disabledSTTClient{}, nil
	case "builtin":
		return &echoSTTClient{}, nil
	default:
		return nil, fmt.Errorf("unsupported stt provider type: %s", cfg.Type)
	}
}

// fallbackImageClient covers a disabled or failing provider with the built-in
// generator, so a scene always has art offline.
type fallbackImageClient struct {
	primary  ImageClient
	fallback ImageClient
	logger   trace.Logger
}

// GenerateImage covers a failing primary with the built-in generator. A fallback
// success is logged as a degraded path; a double failure reports both errors.
func (c *fallbackImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	data, err := c.primary.GenerateImage(ctx, prompt)
	if err == nil {
		return data, nil
	}
	if c.logger != nil {
		c.logger.Event("provider.error", map[string]interface{}{
			"role":     "image",
			"fallback": true,
			"error":    err.Error(),
		})
	}
	fbData, fbErr := c.fallback.GenerateImage(ctx, prompt)
	if fbErr == nil {
		return fbData, nil
	}
	return nil, &harness.GenerationFailure{
		Code:    harness.FailureProviderError,
		Message: fmt.Sprintf("image provider failed: %v; built-in fallback failed: %v", err, fbErr),
		Cause:   err,
		Attempts: []harness.Attempt{
			{Role: "image", Provider: "primary", Code: harness.ClassifyProviderError(err), Detail: err.Error()},
			{Role: "image", Provider: "builtin", Code: harness.ClassifyProviderError(fbErr), Detail: fbErr.Error()},
		},
	}
}

// NewSceneImageClient builds the image client used for scene art: the configured
// provider, wrapping the built-in generator when the fallback is enabled.
func NewSceneImageClient(cfg config.ImageConfig) (ImageClient, error) {
	return NewSceneImageClientWithSharedKey(cfg, "")
}

func NewSceneImageClientWithSharedKey(cfg config.ImageConfig, sharedKey string, logger ...trace.Logger) (ImageClient, error) {
	primary, err := NewImageClientWithSharedKey(cfg, sharedKey)
	if err != nil {
		return nil, err
	}
	if !cfg.BuiltinFallback {
		return primary, nil
	}

	fallback, err := NewImageClient(config.ImageConfig{Type: "builtin", BuiltinName: "procedural-art"})
	if err != nil {
		return nil, err
	}
	var sink trace.Logger
	if len(logger) > 0 {
		sink = trace.OrNil(logger[0])
	}
	return &fallbackImageClient{primary: primary, fallback: fallback, logger: sink}, nil
}

func NewImageClient(cfg config.ImageConfig) (ImageClient, error) {
	return NewImageClientWithSharedKey(cfg, "")
}

func NewImageClientWithSharedKey(cfg config.ImageConfig, sharedKey string) (ImageClient, error) {
	// Registry-first when pkg/provider/all was imported; inline otherwise.
	if key, ok := ImageKeyFor(cfg); ok {
		if _, found := provider.Lookup(string(key.Parent())); found {
			return BuildImage(string(key.Parent()), cfg, sharedKey)
		}
	}
	switch cfg.Type {
	case "disabled", "":
		return &disabledImageClient{}, nil
	case "builtin":
		if cfg.BuiltinName == "procedural-art" {
			return NewProceduralArtClient(), nil
		}
		return &echoImageClient{}, nil
	default:
		return nil, fmt.Errorf("unsupported image provider type: %s", cfg.Type)
	}
}
