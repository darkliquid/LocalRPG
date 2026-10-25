package ttsopenai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// client calls one OpenAI speech endpoint with one model and key.
type client struct {
	endpoint      string
	model         string
	apiKey        string
	defaultVoice  string
	configOptions map[string]any
	httpClient    *http.Client
}

// NewClient builds the OpenAI speech client. The shared key covers a
// configuration that carries no key of its own.
func NewClient(cfg config.TTSConfig, sharedKey string) media.TTSClient {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = sharedKey
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/audio/speech"
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini-tts"
	}
	return &client{
		endpoint:      endpoint,
		model:         model,
		apiKey:        apiKey,
		defaultVoice:  cfg.DefaultVoice,
		configOptions: cfg.Options,
		httpClient:    &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 60 * time.Second},
	}
}

// Synthesize renders one clip. Opus is requested because the pipeline
// normalises every clip to Ogg/Opus, so the download is already the stored
// format. The voice's options override the configuration's: "instructions"
// steers tone and accent, "model" swaps the speech model.
func (c *client) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	voiceID := c.defaultVoice
	if voiceID == "" {
		voiceID = "alloy"
	}
	model := c.model
	var instructions string
	if voice != nil {
		if voice.VoiceID != "" {
			voiceID = voice.VoiceID
		}
		if v, ok := voice.Options["model"].(string); ok && v != "" {
			model = v
		}
		if v, ok := voice.Options["instructions"].(string); ok {
			instructions = v
		}
	}
	if instructions == "" {
		if v, ok := c.configOptions["instructions"].(string); ok {
			instructions = v
		}
	}
	payload := map[string]any{
		"model":           model,
		"input":           text,
		"voice":           voiceID,
		"response_format": "opus",
	}
	if instructions != "" {
		payload["instructions"] = instructions
	}
	if voice != nil && voice.SpeechRate > 0 {
		payload["speed"] = voice.SpeechRate
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal speech request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai tts request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, fmt.Errorf("openai tts failed (%d): %s", resp.StatusCode, provider.TruncateDetail(b))
	}
	return io.ReadAll(resp.Body)
}

// VoiceOptions declares the provider's tunables: the delivery instructions and
// the model. Pitch is not offered because the API has no pitch control.
func (c *client) VoiceOptions() []media.VoiceOption {
	return []media.VoiceOption{
		{Key: "instructions", Label: "Delivery instructions", Kind: "string",
			Help: "Free text tone and accent steering, e.g. 'breathy, elderly, faintly amused'."},
		{Key: "model", Label: "Model", Kind: "enum", Options: []string{"gpt-4o-mini-tts"}, Default: "gpt-4o-mini-tts"},
	}
}

var (
	_ media.TTSClient    = (*client)(nil)
	_ media.VoiceCatalog = (*client)(nil)
	_ media.VoiceOptions = (*client)(nil)
)
