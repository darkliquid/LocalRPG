package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/telemetry"
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

// NewSherpaTTSProvider builds the Sherpa-ONNX Kokoro client.
func NewSherpaTTSProvider(cfg config.TTSConfig) TTSClient {
	modelDir := cfg.ModelPath
	if modelDir == "" {
		modelDir = "./cache/models/tts/kokoro"
	}
	return NewSherpaTTSClient(modelDir)
}

// NewNativeOSTTSProvider builds the host-OS speech client.
func NewNativeOSTTSProvider() TTSClient { return NewNativeOSTTSClient() }

// NewElevenLabsTTSProvider builds the ElevenLabs client.
func NewElevenLabsTTSProvider(cfg config.TTSConfig) (TTSClient, error) {
	return NewElevenLabsTTSClient(cfg)
}

// NewGeminiTTSProvider builds the Gemini TTS client.
func NewGeminiTTSProvider(cfg config.TTSConfig, sharedKey string) (TTSClient, error) {
	return NewGeminiTTSClient(cfg, sharedKey)
}

// NewCLITTSProvider builds the command-line TTS client (for example piper).
func NewCLITTSProvider(cfg config.TTSConfig) TTSClient {
	return &cliTTSClient{command: cfg.Command, args: cfg.Args}
}

// NewHTTPTTSProvider builds the OpenAI-compatible HTTP TTS client.
func NewHTTPTTSProvider(cfg config.TTSConfig) TTSClient {
	return &httpTTSClient{
		endpoint: cfg.Endpoint,
		model:    cfg.Model,
		apiKey:   cfg.APIKey,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 30 * time.Second},
	}
}
