package media

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// TTSBuildPayload is what BuildTTS hands a provider package: the family config
// plus the shared provider key, which is not part of TTSConfig.
type TTSBuildPayload struct {
	Config    config.TTSConfig `json:"config"`
	SharedKey string           `json:"shared_key,omitempty"`
}

// TTSProviderIDFor maps a TTS configuration to the registry ID a facade should
// build. An empty result means the configuration has no registry provider.
func TTSProviderIDFor(cfg config.TTSConfig) string {
	switch cfg.Type {
	case "gemini":
		return "tts-gemini"
	case "builtin":
		switch cfg.BuiltinName {
		case "gemini":
			return "tts-gemini"
		case "sherpa-onnx", "kokoro":
			return "tts-sherpa-onnx"
		case "native-os":
			return "tts-native-os"
		case "elevenlabs":
			return "tts-elevenlabs"
		}
	case "cli":
		return "tts-piper"
	case "http":
		return "tts-openai-http"
	}
	return ""
}

// BuildTTS constructs a TTS client from the registry by ID.
func BuildTTS(id string, cfg config.TTSConfig, sharedKey string) (TTSClient, error) {
	reg, ok := provider.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("media: no provider registered for %q", id)
	}
	raw, err := json.Marshal(TTSBuildPayload{Config: cfg, SharedKey: sharedKey})
	if err != nil {
		return nil, fmt.Errorf("media: encode %s config: %w", id, err)
	}
	built, err := reg.Build(context.Background(), raw)
	if err != nil {
		return nil, err
	}
	client, ok := built.(TTSClient)
	if !ok {
		return nil, fmt.Errorf("media: provider %q is not a tts client", id)
	}
	return client, nil
}

// NewElevenLabsTTSProvider builds the ElevenLabs client.
func NewElevenLabsTTSProvider(cfg config.TTSConfig) (TTSClient, error) {
	return NewElevenLabsTTSClient(cfg)
}

// NewGeminiTTSProvider builds the Gemini TTS client.
func NewGeminiTTSProvider(cfg config.TTSConfig, sharedKey string) (TTSClient, error) {
	return NewGeminiTTSClient(cfg, sharedKey)
}

// STTProviderIDFor maps an STT configuration to the registry ID a facade should
// build.
func STTProviderIDFor(cfg config.STTConfig) string {
	switch cfg.Type {
	case "cli":
		return "stt-whisper-cli"
	case "http":
		return "stt-whisper-http"
	}
	return ""
}

// BuildSTT constructs an STT client from the registry by ID.
func BuildSTT(id string, cfg config.STTConfig) (STTClient, error) {
	reg, ok := provider.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("media: no provider registered for %q", id)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("media: encode %s config: %w", id, err)
	}
	built, err := reg.Build(context.Background(), raw)
	if err != nil {
		return nil, err
	}
	client, ok := built.(STTClient)
	if !ok {
		return nil, fmt.Errorf("media: provider %q is not an stt client", id)
	}
	return client, nil
}

// ImageBuildPayload is what BuildImage hands an image provider package.
type ImageBuildPayload struct {
	Config    config.ImageConfig `json:"config"`
	SharedKey string             `json:"shared_key,omitempty"`
}

// ImageProviderIDFor maps an image configuration to the registry ID a facade
// should build.
func ImageProviderIDFor(cfg config.ImageConfig) string {
	switch cfg.Type {
	case "gemini":
		return "image-gemini"
	case "builtin":
		if cfg.BuiltinName == "procedural-art" {
			return "image-procedural-art"
		}
	case "cli":
		return "image-cli"
	case "comfyui", "http":
		return "image-http"
	}
	return ""
}

// BuildImage constructs an image client from the registry by ID.
func BuildImage(id string, cfg config.ImageConfig, sharedKey string) (ImageClient, error) {
	reg, ok := provider.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("media: no provider registered for %q", id)
	}
	raw, err := json.Marshal(ImageBuildPayload{Config: cfg, SharedKey: sharedKey})
	if err != nil {
		return nil, fmt.Errorf("media: encode %s config: %w", id, err)
	}
	built, err := reg.Build(context.Background(), raw)
	if err != nil {
		return nil, err
	}
	client, ok := built.(ImageClient)
	if !ok {
		return nil, fmt.Errorf("media: provider %q is not an image client", id)
	}
	return client, nil
}

// NewProceduralImageProvider builds the built-in procedural art client.
func NewProceduralImageProvider() ImageClient { return NewProceduralArtClient() }
