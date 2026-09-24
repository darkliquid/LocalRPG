package media

import (
	"context"
	"errors"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/provider"
)

var ErrProviderDisabled = errors.New("provider is disabled")

// Disabled implementations
type disabledTTSClient struct{}

func (d *disabledTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return nil, ErrProviderDisabled
}

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
	return []byte("RIFF....WAVEfmt ....data" + text), nil
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
	if regID := TTSProviderIDFor(cfg); regID != "" {
		if _, ok := provider.Lookup(regID); ok {
			return BuildTTS(regID, cfg, sharedKey)
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
	// Registry-first when pkg/provider/all was imported; inline otherwise.
	if regID := STTProviderIDFor(cfg); regID != "" {
		if _, ok := provider.Lookup(regID); ok {
			return BuildSTT(regID, cfg)
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
}

func (c *fallbackImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	data, err := c.primary.GenerateImage(ctx, prompt)
	if err == nil {
		return data, nil
	}
	return c.fallback.GenerateImage(ctx, prompt)
}

// NewSceneImageClient builds the image client used for scene art: the configured
// provider, wrapping the built-in generator when the fallback is enabled.
func NewSceneImageClient(cfg config.ImageConfig) (ImageClient, error) {
	return NewSceneImageClientWithSharedKey(cfg, "")
}

func NewSceneImageClientWithSharedKey(cfg config.ImageConfig, sharedKey string) (ImageClient, error) {
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
	return &fallbackImageClient{primary: primary, fallback: fallback}, nil
}

func NewImageClient(cfg config.ImageConfig) (ImageClient, error) {
	return NewImageClientWithSharedKey(cfg, "")
}

func NewImageClientWithSharedKey(cfg config.ImageConfig, sharedKey string) (ImageClient, error) {
	// Registry-first when pkg/provider/all was imported; inline otherwise.
	if regID := ImageProviderIDFor(cfg); regID != "" {
		if _, ok := provider.Lookup(regID); ok {
			return BuildImage(regID, cfg, sharedKey)
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
