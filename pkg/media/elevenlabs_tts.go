package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// ErrMissingAPIKey reports that an ElevenLabs client was built without a key.
var ErrMissingAPIKey = errors.New("elevenlabs: an API key is required; set media.tts.api_key or ELEVENLABS_API_KEY")

const (
	elevenLabsDefaultBaseURL  = "https://api.elevenlabs.io"
	elevenLabsDefaultModel    = "eleven_multilingual_v2"
	elevenLabsDefaultFormat   = "mp3_44100_128"
	elevenLabsDefaultVoiceID  = "EXAVITQu4vr4xnSDxMaL"
	elevenLabsCatalogPageSize = 100
	elevenLabsRequestTimeout  = 60 * time.Second
)

// ElevenLabsTTSClient is the built-in provider for ElevenLabs speech. It
// implements TTSClient, VoiceCatalog, VoiceOptions, and MeteredProvider, so the
// pipeline, the catalog cache, and the settings UI need no provider-specific
// handling.
type ElevenLabsTTSClient struct {
	apiKey    string
	model     string
	baseURL   string
	outputFmt string
	client    *http.Client
	logger    trace.Logger
}

// NewElevenLabsTTSClient builds a client, preferring the configured key and
// falling back to ELEVENLABS_API_KEY so a user need not write a key to disk. It
// registers the key for redaction and never records it.
func NewElevenLabsTTSClient(cfg config.TTSConfig) (*ElevenLabsTTSClient, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY"))
	}
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = elevenLabsDefaultModel
	}

	trace.RegisterSecret(apiKey)

	return &ElevenLabsTTSClient{
		apiKey:    apiKey,
		model:     model,
		baseURL:   elevenLabsDefaultBaseURL,
		outputFmt: elevenLabsDefaultFormat,
		client:    &http.Client{Timeout: elevenLabsRequestTimeout},
	}, nil
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (c *ElevenLabsTTSClient) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

// Metered reports that ElevenLabs charges per request.
func (c *ElevenLabsTTSClient) Metered() bool { return true }

// VoiceOptions declares the tunables ElevenLabs accepts. Pitch is deliberately
// absent: the API exposes speed but no pitch, and speech rate is the portable
// baseline, so declaring it twice would put two controls on one knob.
func (c *ElevenLabsTTSClient) VoiceOptions() []VoiceOption {
	return []VoiceOption{
		{Key: "stability", Label: "Stability", Kind: "float", Min: 0, Max: 1, Step: 0.05, Default: 0.5,
			Help: "Lower is more expressive, higher is more consistent."},
		{Key: "similarity_boost", Label: "Similarity", Kind: "float", Min: 0, Max: 1, Step: 0.05, Default: 0.75,
			Help: "How closely to follow the original voice."},
		{Key: "style", Label: "Style", Kind: "float", Min: 0, Max: 1, Step: 0.05, Default: 0,
			Help: "Amplifies the voice's character; adds latency when above zero."},
		{Key: "use_speaker_boost", Label: "Speaker boost", Kind: "bool", Default: true,
			Help: "Improves similarity; adds latency."},
		{Key: "model", Label: "Model", Kind: "enum",
			Options: []string{"eleven_multilingual_v2", "eleven_turbo_v2_5", "eleven_flash_v2_5"},
			Default: "eleven_multilingual_v2",
			Help:    "Turbo and Flash are faster and cheaper; Multilingual has the widest language support."},
		{Key: "format", Label: "Audio format", Kind: "enum",
			Options: []string{"mp3_44100_128", "mp3_22050_32", "pcm_24000", "ulaw_8000"},
			Default: "mp3_44100_128",
			Help:    "Higher bitrates and PCM/WAV may require a paid tier."},
	}
}

// Synthesize renders one utterance. It returns the provider's bytes unmodified;
// the pipeline names and caches the clip from its content.
func (c *ElevenLabsTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	voiceID := elevenLabsDefaultVoiceID
	if voice != nil && strings.TrimSpace(voice.VoiceID) != "" {
		voiceID = voice.VoiceID
	}

	// Defence in depth for a hand-edited note: clamp and drop against the schema
	// before anything reaches the wire.
	options, _ := ValidateVoiceOptions(c.VoiceOptions(), voiceOptions(voice))

	model := c.model
	if value, ok := options["model"].(string); ok && value != "" {
		model = value
	}
	outputFormat := c.outputFmt
	if value, ok := options["format"].(string); ok && value != "" {
		outputFormat = value
	}

	body := map[string]interface{}{
		"text":     text,
		"model_id": model,
	}
	if settings := voiceSettings(voice, options); len(settings) > 0 {
		body["voice_settings"] = settings
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1/text-to-speech/%s?%s",
		strings.TrimRight(c.baseURL, "/"), url.PathEscape(voiceID), url.Values{"output_format": {outputFormat}}.Encode())

	resp, err := c.do(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := elevenLabsError(resp); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

// voiceOptions is a voice's provider options, or nil when it carries none.
func voiceOptions(voice *entity.VoiceConfig) map[string]interface{} {
	if voice == nil {
		return nil
	}
	return voice.Options
}

// voiceSettings maps the declared options onto ElevenLabs' voice_settings. It
// returns an empty map when nothing was tuned, so the voice's stored server-side
// settings apply unchanged.
func voiceSettings(voice *entity.VoiceConfig, options map[string]interface{}) map[string]interface{} {
	settings := make(map[string]interface{})
	for _, key := range []string{"stability", "similarity_boost", "style", "use_speaker_boost"} {
		if value, ok := options[key]; ok {
			settings[key] = value
		}
	}

	// Speed is only sent when it was deliberately set away from the provider's
	// default, because a stock voice must keep its own server-side settings.
	if voice != nil && voice.SpeechRate > 0 && voice.SpeechRate != 1.0 {
		settings["speed"] = voice.SpeechRate
	}
	return settings
}

// do sends a request, retrying once on 429 after honouring Retry-After.
func (c *ElevenLabsTTSClient) do(ctx context.Context, method, endpoint string, payload []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("elevenlabs: build request: %w", err)
		}
		req.Header.Set("xi-api-key", c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("elevenlabs: request to %s failed: %w", requestHost(endpoint), err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			delay := retryAfter(resp)
			resp.Body.Close()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		return resp, nil
	}
}

// retryAfter is the server's Retry-After in seconds, or one second.
func retryAfter(resp *http.Response) time.Duration {
	value := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if seconds, err := time.ParseDuration(value + "s"); err == nil && seconds > 0 {
		return seconds
	}
	return time.Second
}

// requestHost names an endpoint without its path or query, for error messages.
func requestHost(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "the provider"
	}
	return parsed.Host
}

// elevenLabsError maps a failed response onto an actionable message. The body is
// never included wholesale, because it can echo request context.
func elevenLabsError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return errors.New("elevenlabs rejected the API key; check media.tts.api_key")
	case http.StatusPaymentRequired, http.StatusTooManyRequests:
		return errors.New("elevenlabs quota or rate limit reached; check your plan and credits")
	case http.StatusUnprocessableEntity:
		return errors.New("elevenlabs: that voice or model is not available on this account")
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("elevenlabs: the provider returned %d; try again later", resp.StatusCode)
		}
		return fmt.Errorf("elevenlabs: request failed with status %d", resp.StatusCode)
	}
}
