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
	case "fish-audio":
		return provider.InstanceOrSelf(provider.KeyTTSFishAudio, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint))), true
	case "gemini":
		return provider.InstanceOrSelf(provider.KeyTTSGemini, provider.InstanceDiscriminator(cfg.Instance, "")), true
	case "inworld":
		return provider.InstanceOrSelf(provider.KeyTTSInworld, provider.InstanceDiscriminator(cfg.Instance, "")), true
	case "cartesia":
		return provider.InstanceOrSelf(provider.KeyTTSCartesia, provider.InstanceDiscriminator(cfg.Instance, "")), true
	case "builtin":
		switch cfg.BuiltinName {
		case "gemini":
			return provider.InstanceOrSelf(provider.KeyTTSGemini, provider.InstanceDiscriminator(cfg.Instance, "")), true
		case "inworld":
			return provider.InstanceOrSelf(provider.KeyTTSInworld, provider.InstanceDiscriminator(cfg.Instance, "")), true
		case "sherpa-onnx", "kokoro":
			return provider.InstanceOrSelf(provider.KeyTTSSherpaONNX, provider.InstanceDiscriminator(cfg.Instance, "")), true
		case "native-os":
			return provider.InstanceOrSelf(provider.KeyTTSNativeOS, provider.InstanceDiscriminator(cfg.Instance, "")), true
		case "elevenlabs":
			return provider.InstanceOrSelf(provider.KeyTTSElevenLabs, provider.InstanceDiscriminator(cfg.Instance, "")), true
		case "cartesia":
			return provider.InstanceOrSelf(provider.KeyTTSCartesia, provider.InstanceDiscriminator(cfg.Instance, "")), true
		}
		return "", false
	case "cli":
		return provider.InstanceOrSelf(provider.KeyTTSPiper, provider.InstanceDiscriminator(cfg.Instance, provider.CommandDiscriminator(cfg.Command))), true
	case "http":
		lowerModel := strings.ToLower(cfg.Model)
		if strings.Contains(lowerModel, "fishaudio") || strings.Contains(lowerModel, "s2-pro") {
			return provider.InstanceOrSelf(provider.KeyTTSFishAudio, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint))), true
		}
		return provider.InstanceOrSelf(provider.KeyTTSHTTP, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint))), true
	}
	return "", false
}

// SharedProviderKey is the provider-wide credential a media client inherits when
// its own config carries none: an Inworld adapter uses providers.inworld.api_key,
// a Cartesia adapter uses providers.cartesia.api_key, and every other adapter
// keeps the Gemini key it has always used.
func SharedProviderKey(cfg *config.Config, key provider.Key, ok bool) string {
	if cfg == nil {
		return ""
	}
	if ok {
		switch string(key.Parent()) {
		case string(provider.KeyTTSInworld), string(provider.KeySTTInworld):
			return cfg.Providers.Inworld.APIKey
		case string(provider.KeyTTSCartesia), string(provider.KeySTTCartesia):
			return cfg.Providers.Cartesia.APIKey
		}
	}
	return cfg.Providers.Gemini.APIKey
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

// STTBuildPayload is what BuildSTT hands an STT provider package: the family
// config plus the shared provider key, which is not part of STTConfig.
type STTBuildPayload struct {
	Config    config.STTConfig `json:"config"`
	SharedKey string           `json:"shared_key,omitempty"`
}

// sttBuildWire carries both the nested payload and the flattened config so an
// STT adapter can decode whichever shape it expects.
type sttBuildWire struct {
	config.STTConfig
	Config    config.STTConfig `json:"config"`
	SharedKey string           `json:"shared_key,omitempty"`
}

// STTKeyFor maps an STT configuration to its canonical key. Browser-only values
// have no key: they never reach the server-side factory.
func STTKeyFor(cfg config.STTConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "inworld":
		return provider.InstanceOrSelf(provider.KeySTTInworld, provider.InstanceDiscriminator(cfg.Instance, "")), true
	case "cartesia":
		return provider.InstanceOrSelf(provider.KeySTTCartesia, provider.InstanceDiscriminator(cfg.Instance, "")), true
	case "builtin":
		if cfg.BuiltinName == "inworld" {
			return provider.InstanceOrSelf(provider.KeySTTInworld, provider.InstanceDiscriminator(cfg.Instance, "")), true
		}
		if cfg.BuiltinName == "cartesia" {
			return provider.InstanceOrSelf(provider.KeySTTCartesia, provider.InstanceDiscriminator(cfg.Instance, "")), true
		}
		return "", false
	case "http":
		return provider.InstanceOrSelf(provider.KeySTTWhisperHTTP, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint))), true
	case "cli":
		return provider.InstanceOrSelf(provider.KeySTTWhisperCLI, provider.InstanceDiscriminator(cfg.Instance, provider.CommandDiscriminator(cfg.Command))), true
	}
	return "", false
}

// BuildSTT constructs an STT client from the registry by ID.
func BuildSTT(id string, cfg config.STTConfig, sharedKey ...string) (STTClient, error) {
	reg, ok := provider.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("media: no provider registered for %q", id)
	}
	var sk string
	if len(sharedKey) > 0 {
		sk = sharedKey[0]
	}
	raw, err := json.Marshal(sttBuildWire{STTConfig: cfg, Config: cfg, SharedKey: sk})
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
		return provider.InstanceOrSelf(provider.KeyImageGemini, provider.InstanceDiscriminator(cfg.Instance, "")), true
	case "builtin":
		if cfg.BuiltinName == "procedural-art" {
			return provider.InstanceOrSelf(provider.KeyImageProceduralArt, provider.InstanceDiscriminator(cfg.Instance, "")), true
		}
		return "", false
	case "cli":
		return provider.InstanceOrSelf(provider.KeyImageCLI, provider.InstanceDiscriminator(cfg.Instance, provider.CommandDiscriminator(cfg.Command))), true
	case "comfyui", "http":
		return provider.InstanceOrSelf(provider.KeyImageHTTP, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint))), true
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
