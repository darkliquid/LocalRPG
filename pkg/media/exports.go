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

// TTSKeyFor maps a TTS configuration to its canonical key. ok is false when the
// configuration has no registered adapter.
func TTSKeyFor(cfg config.TTSConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "gemini":
		return provider.KeyTTSGemini, true
	case "builtin":
		switch cfg.BuiltinName {
		case "gemini":
			return provider.KeyTTSGemini, true
		case "sherpa-onnx", "kokoro":
			return provider.KeyTTSSherpaONNX, true
		case "native-os":
			return provider.KeyTTSNativeOS, true
		case "elevenlabs":
			return provider.KeyTTSElevenLabs, true
		}
		return "", false
	case "cli":
		return provider.InstanceOrSelf(provider.KeyTTSPiper, provider.CommandDiscriminator(cfg.Command)), true
	case "http":
		return provider.InstanceOrSelf(provider.KeyTTSHTTP, provider.HostDiscriminator(cfg.Endpoint)), true
	}
	return "", false
}

// TTSProviderIDFor maps a TTS configuration to the registry ID a facade should
// build. It is the adapter half of TTSKeyFor.
func TTSProviderIDFor(cfg config.TTSConfig) string {
	key, ok := TTSKeyFor(cfg)
	if !ok {
		return ""
	}
	return string(key.Parent())
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

// STTKeyFor maps an STT configuration to its canonical key. Browser-only values
// have no key: they never reach the server-side factory.
func STTKeyFor(cfg config.STTConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "http":
		return provider.InstanceOrSelf(provider.KeySTTWhisperHTTP, provider.HostDiscriminator(cfg.Endpoint)), true
	case "cli":
		return provider.InstanceOrSelf(provider.KeySTTWhisperCLI, provider.CommandDiscriminator(cfg.Command)), true
	}
	return "", false
}

// STTProviderIDFor maps an STT configuration to the registry ID a facade should
// build. It is the adapter half of STTKeyFor.
func STTProviderIDFor(cfg config.STTConfig) string {
	key, ok := STTKeyFor(cfg)
	if !ok {
		return ""
	}
	return string(key.Parent())
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

// ImageKeyFor maps an image configuration to its canonical key.
func ImageKeyFor(cfg config.ImageConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "gemini":
		return provider.KeyImageGemini, true
	case "builtin":
		if cfg.BuiltinName == "procedural-art" {
			return provider.KeyImageProceduralArt, true
		}
		return "", false
	case "cli":
		return provider.InstanceOrSelf(provider.KeyImageCLI, provider.CommandDiscriminator(cfg.Command)), true
	case "comfyui", "http":
		return provider.InstanceOrSelf(provider.KeyImageHTTP, provider.HostDiscriminator(cfg.Endpoint)), true
	}
	return "", false
}

// ImageProviderIDFor maps an image configuration to the registry ID a facade
// should build. It is the adapter half of ImageKeyFor.
func ImageProviderIDFor(cfg config.ImageConfig) string {
	key, ok := ImageKeyFor(cfg)
	if !ok {
		return ""
	}
	return string(key.Parent())
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
