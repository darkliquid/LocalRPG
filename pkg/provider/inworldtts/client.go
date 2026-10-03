package inworldtts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/provider/inworldshared"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

const (
	defaultEndpoint = "https://api.inworld.ai/tts/v1/voice"
	defaultModel    = "inworld-tts-2"
	defaultVoice    = "Ashley"
	requestTimeout  = 60 * time.Second
)

// InworldTTSClient synthesizes speech through Inworld's cloud TTS API. It
// implements media.TTSClient, media.VoiceCatalog, and media.MeteredProvider.
type InworldTTSClient struct {
	apiKey       string
	model        string
	defaultVoice string
	endpoint     string
	client       *http.Client
	logger       trace.Logger

	usageMu   sync.Mutex
	lastUsage media.Usage
}

// NewInworldTTSClient builds the Inworld TTS client, resolving the credential
// from the media config, the shared providers.inworld.api_key, or the
// environment.
func NewInworldTTSClient(cfg config.TTSConfig, sharedKey string) (*InworldTTSClient, error) {
	apiKey, err := inworldshared.ResolveAPIKey(cfg.APIKey, sharedKey)
	if err != nil {
		return nil, err
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}
	voice := strings.TrimSpace(cfg.DefaultVoice)
	if voice == "" {
		voice = defaultVoice
	}

	return &InworldTTSClient{
		apiKey:       apiKey,
		model:        model,
		defaultVoice: voice,
		endpoint:     voiceEndpoint(cfg.Endpoint),
		client:       &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: requestTimeout},
	}, nil
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (c *InworldTTSClient) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

// Metered reports that Inworld TTS charges per character.
func (c *InworldTTSClient) Metered() bool { return true }

// LastUsage reports the characters the last synthesis consumed.
func (c *InworldTTSClient) LastUsage() media.Usage {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return c.lastUsage
}

// Synthesize renders text in one voice and returns the audio bytes. The API
// answers with base64 MP3, which the media pipeline decodes and re-encodes.
func (c *InworldTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	voiceID := c.defaultVoice
	if voice != nil && strings.TrimSpace(voice.VoiceID) != "" {
		voiceID = strings.TrimSpace(voice.VoiceID)
	}

	payload, err := json.Marshal(map[string]interface{}{
		"text":     text,
		"voice_id": voiceID,
		"model_id": c.model,
		"audio_config": map[string]interface{}{
			"audio_encoding":    "MP3",
			"sample_rate_hertz": 48000,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal inworld tts request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create inworld tts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("inworld tts request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, inworldshared.MapError(resp.StatusCode, body)
	}

	var decoded struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode inworld tts response: %w", err)
	}
	audio, err := base64.StdEncoding.DecodeString(decoded.AudioContent)
	if err != nil {
		return nil, fmt.Errorf("decode base64 audioContent: %w", err)
	}

	c.usageMu.Lock()
	c.lastUsage = media.Usage{Requests: 1, Characters: len([]rune(text))}
	c.usageMu.Unlock()

	return audio, nil
}

// ListVoices enumerates Inworld's voices. The live catalogue needs credentials
// and a reachable endpoint, so an offline or unauthenticated caller gets a small
// curated list instead of an error: character creation and previews still work.
func (c *InworldTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, voicesEndpoint(c.endpoint), nil)
	if err != nil {
		return fallbackVoices(), nil
	}
	req.Header.Set("Authorization", "Basic "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return fallbackVoices(), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fallbackVoices(), nil
	}

	var decoded struct {
		Voices []struct {
			VoiceID     string `json:"voice_id"`
			Name        string `json:"name"`
			Gender      string `json:"gender"`
			Accent      string `json:"accent"`
			Language    string `json:"language"`
			Description string `json:"description"`
		} `json:"voices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil || len(decoded.Voices) == 0 {
		return fallbackVoices(), nil
	}

	voices := make([]media.ProviderVoice, 0, len(decoded.Voices))
	for _, v := range decoded.Voices {
		name := v.Name
		if name == "" {
			name = v.VoiceID
		}
		voices = append(voices, media.ProviderVoice{
			ID:          v.VoiceID,
			Name:        name,
			Gender:      v.Gender,
			Accent:      v.Accent,
			Language:    v.Language,
			Description: v.Description,
		})
	}
	return voices, nil
}

// fallbackVoices is the curated catalogue used when the live one is unavailable.
func fallbackVoices() []media.ProviderVoice {
	return []media.ProviderVoice{
		{ID: "Ashley", Name: "Ashley", Gender: "female", Accent: "American", Language: "en", Description: "Natural, expressive American English female voice"},
		{ID: "Dennis", Name: "Dennis", Gender: "male", Accent: "American", Language: "en", Description: "Calm, conversational American English male voice"},
		{ID: "Sarah", Name: "Sarah", Gender: "female", Accent: "American", Language: "en", Description: "Warm, articulate narrator voice"},
		{ID: "Alex", Name: "Alex", Gender: "neutral", Accent: "American", Language: "en", Description: "Clear, adaptable neutral voice"},
	}
}

// voiceEndpoint resolves the synthesis URL. A configured base with no path is
// treated as the host and given the standard path; a full endpoint is used as
// given.
func voiceEndpoint(base string) string {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return defaultEndpoint
	}
	if parsed, err := url.Parse(trimmed); err != nil || parsed.Path == "" || parsed.Path == "/" {
		return strings.TrimRight(trimmed, "/") + "/tts/v1/voice"
	}
	return trimmed
}

// voicesEndpoint derives the catalogue URL from the synthesis URL.
func voicesEndpoint(voiceURL string) string {
	if strings.HasSuffix(voiceURL, "/voice") {
		return strings.TrimSuffix(voiceURL, "/voice") + "/voices"
	}
	return strings.TrimRight(voiceURL, "/") + "/voices"
}
