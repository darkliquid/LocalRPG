package media

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	case "inworld":
		return provider.KeyTTSInworld, true
	case "builtin":
		switch cfg.BuiltinName {
		case "gemini":
			return provider.KeyTTSGemini, true
		case "inworld":
			return provider.KeyTTSInworld, true
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

// SharedProviderKey is the provider-wide credential a media client inherits when
// its own config carries none: an Inworld adapter uses providers.inworld.api_key,
// every other adapter keeps the Gemini key it has always used.
func SharedProviderKey(cfg *config.Config, key provider.Key, ok bool) string {
	if cfg == nil {
		return ""
	}
	if ok && isInworldKey(key) {
		return cfg.Providers.Inworld.APIKey
	}
	return cfg.Providers.Gemini.APIKey
}

// isInworldKey reports whether a canonical key names an Inworld adapter.
func isInworldKey(key provider.Key) bool {
	return strings.HasSuffix(string(key.Parent()), ":inworld")
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
