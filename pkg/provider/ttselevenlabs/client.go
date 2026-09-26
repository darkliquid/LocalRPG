package ttselevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/darkliquid/localrpg/pkg/media"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/telemetry"
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
// implements media.TTSClient, media.VoiceCatalog, VoiceOptions, and media.MeteredProvider, so the
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
		client:    &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: elevenLabsRequestTimeout},
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
func (c *ElevenLabsTTSClient) VoiceOptions() []media.VoiceOption {
	return []media.VoiceOption{
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
	options, _ := media.ValidateVoiceOptions(c.VoiceOptions(), media.VoiceOptionsOf(voice))

	model := c.model
	if value, ok := options["model"].(string); ok && value != "" {
		model = value
	}
	outputFormat := c.outputFmt
	if value, ok := options["format"].(string); ok && value != "" {
		outputFormat = value
	}

	body := map[string]any{
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
func voiceOptions(voice *entity.VoiceConfig) map[string]any {
	if voice == nil {
		return nil
	}
	return voice.Options
}

// voiceSettings maps the declared options onto ElevenLabs' voice_settings. It
// returns an empty map when nothing was tuned, so the voice's stored server-side
// settings apply unchanged.
func voiceSettings(voice *entity.VoiceConfig, options map[string]any) map[string]any {
	settings := make(map[string]any)
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

// elevenLabsError maps a failed response onto an actionable message. It extracts
// detail messages from structured error payloads while avoiding wholesale echoing
// of response bodies.
func elevenLabsError(resp *http.Response) error {
	if resp == nil || resp.StatusCode == http.StatusOK {
		return nil
	}

	detailMsg := ""
	if resp.Body != nil {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(body) > 0 {
			detailMsg = extractElevenLabsErrorMessage(body)
		}
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		if detailMsg != "" {
			return fmt.Errorf("elevenlabs: %s", detailMsg)
		}
		return errors.New("elevenlabs rejected the API key; check media.tts.api_key")
	case http.StatusPaymentRequired, http.StatusTooManyRequests:
		if detailMsg != "" {
			return fmt.Errorf("elevenlabs: %s", detailMsg)
		}
		return errors.New("elevenlabs quota or rate limit reached; check your plan and credits")
	case http.StatusUnprocessableEntity:
		if detailMsg != "" {
			return fmt.Errorf("elevenlabs: %s", detailMsg)
		}
		return errors.New("elevenlabs: that voice or model is not available on this account")
	default:
		if detailMsg != "" {
			return fmt.Errorf("elevenlabs: %s", detailMsg)
		}
		if resp.StatusCode >= 500 {
			return fmt.Errorf("elevenlabs: the provider returned %d; try again later", resp.StatusCode)
		}
		return fmt.Errorf("elevenlabs: request failed with status %d", resp.StatusCode)
	}
}

// extractElevenLabsErrorMessage attempts to decode ElevenLabs' structured error
// payload to extract an actionable detail message without echoing the entire body.
func extractElevenLabsErrorMessage(body []byte) string {
	var errResp struct {
		Detail  json.RawMessage `json:"detail"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil {
		return ""
	}
	if len(errResp.Detail) > 0 {
		var strDetail string
		if err := json.Unmarshal(errResp.Detail, &strDetail); err == nil && strings.TrimSpace(strDetail) != "" {
			return strings.TrimSpace(strDetail)
		}
		var objDetail struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		}
		if err := json.Unmarshal(errResp.Detail, &objDetail); err == nil && strings.TrimSpace(objDetail.Message) != "" {
			return strings.TrimSpace(objDetail.Message)
		}
		var listDetail []struct {
			Msg string `json:"msg"`
		}
		if err := json.Unmarshal(errResp.Detail, &listDetail); err == nil && len(listDetail) > 0 && strings.TrimSpace(listDetail[0].Msg) != "" {
			return strings.TrimSpace(listDetail[0].Msg)
		}
	}
	if strings.TrimSpace(errResp.Message) != "" {
		return strings.TrimSpace(errResp.Message)
	}
	return ""
}

// elevenLabsVoice is one entry of the /v2/voices response. Labels are free-form
// strings, never enums, so they are carried as a map.
type elevenLabsVoice struct {
	VoiceID           string            `json:"voice_id"`
	Name              string            `json:"name"`
	Category          string            `json:"category"`
	Description       string            `json:"description"`
	PreviewURL        string            `json:"preview_url"`
	Labels            map[string]string `json:"labels"`
	Settings          map[string]any    `json:"settings"`
	AvailableForTiers []string          `json:"available_for_tiers"`
	VerifiedLanguages []struct {
		Language string `json:"language"`
	} `json:"verified_languages"`
}

// elevenLabsVoicePage is one page of the catalog. Pagination is mandatory: the
// v2 endpoint pages and the legacy one stops working past 500 voices.
type elevenLabsVoicePage struct {
	Voices        []elevenLabsVoice `json:"voices"`
	HasMore       bool              `json:"has_more"`
	TotalCount    int               `json:"total_count"`
	NextPageToken string            `json:"next_page_token"`
}

// ListVoices pages the account's voices to completion and maps them into the
// shared shape. Cloned and professional voices appear because they belong to the
// key in use, which is the point of fetching rather than shipping a list.
func (c *ElevenLabsTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	voices := make([]media.ProviderVoice, 0)
	token := ""
	for {
		query := url.Values{"page_size": {fmt.Sprintf("%d", elevenLabsCatalogPageSize)}}
		if token != "" {
			query.Set("next_page_token", token)
		}
		endpoint := fmt.Sprintf("%s/v2/voices?%s", strings.TrimRight(c.baseURL, "/"), query.Encode())

		resp, err := c.do(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if err := elevenLabsError(resp); err != nil {
			resp.Body.Close()
			return nil, err
		}
		var page elevenLabsVoicePage
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("elevenlabs: decode voice catalog: %w", err)
		}

		for _, voice := range page.Voices {
			voices = append(voices, mapElevenLabsVoice(voice))
		}
		if !page.HasMore || page.NextPageToken == "" {
			return voices, nil
		}
		token = page.NextPageToken
	}
}

// mapElevenLabsVoice maps a catalog entry onto the shared voice shape.
func mapElevenLabsVoice(voice elevenLabsVoice) media.ProviderVoice {
	mapped := media.ProviderVoice{
		ID:          voice.VoiceID,
		Name:        voice.Name,
		Gender:      strings.ToLower(strings.TrimSpace(voice.Labels["gender"])),
		Accent:      voice.Labels["accent"],
		Description: voice.Description,
		PreviewURL:  voice.PreviewURL,
		Defaults:    voice.Settings,
	}
	if mapped.Description == "" {
		mapped.Description = voice.Labels["description"]
	}
	if len(voice.VerifiedLanguages) > 0 {
		mapped.Language = voice.VerifiedLanguages[0].Language
	}
	if voice.Category != "" {
		mapped.Categories = []string{voice.Category}
	}
	mapped.Tags = media.NormaliseVoiceTags(
		voice.Labels["age"],
		voice.Labels["use_case"],
		voice.Labels["gender"],
		voice.Labels["accent"],
		voice.Category,
	)
	mapped.Metadata = map[string]any{
		"category":            voice.Category,
		"available_for_tiers": voice.AvailableForTiers,
		"verified_languages":  voice.VerifiedLanguages,
	}
	return mapped
}

// media.SpeechCueCapabilities advertises ElevenLabs' support for bracketed audio tags.
func (c *ElevenLabsTTSClient) SpeechCueCapabilities() media.SpeechCueCapabilities {
	return media.SpeechCueCapabilities{
		AudioTags:        true,
		MarkdownEmphasis: false,
		SupportedTags: []string{
			"whispers", "sighs", "laughs", "gasp", "clears throat",
			"chuckles", "softly", "loudly", "excited", "angry",
			"nervous", "sad", "playful", "tired",
		},
		PromptGuidance: "Use bracketed tags immediately before dialogue or delivery beats to steer voice acting.",
	}
}
