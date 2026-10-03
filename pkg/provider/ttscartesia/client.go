package ttscartesia

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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// ErrMissingAPIKey reports that a Cartesia client was built without a key.
var ErrMissingAPIKey = errors.New("cartesia: an API key is required; set media.tts.api_key, providers.cartesia.api_key, or CARTESIA_API_KEY")

const (
	cartesiaAPIVersion     = "2026-08-14"
	cartesiaDefaultBaseURL = "https://api.cartesia.ai"
	cartesiaDefaultModel   = "sonic-3.6"
	cartesiaDefaultVoiceID = "db6b0ed5-d5d3-463d-ae85-518a07d3c2b4"
	cartesiaRequestTimeout = 60 * time.Second
)

// CartesiaTTSClient is the built-in provider for Cartesia Sonic text-to-speech.
// It implements media.TTSClient, media.VoiceCatalog, media.VoiceOptions,
// media.SpeechCueCapabilities, and media.MeteredProvider.
type CartesiaTTSClient struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
	logger  trace.Logger

	usageMu   sync.Mutex
	lastUsage media.Usage
}

// NewCartesiaTTSClient builds a Cartesia TTS client, resolving credentials in order:
// config API key -> shared provider key -> CARTESIA_API_KEY env var.
func NewCartesiaTTSClient(cfg config.TTSConfig, sharedKey string) (*CartesiaTTSClient, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(sharedKey)
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("CARTESIA_API_KEY"))
	}
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = cartesiaDefaultModel
	}

	trace.RegisterSecret(apiKey)

	return &CartesiaTTSClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: cartesiaDefaultBaseURL,
		client:  &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: cartesiaRequestTimeout},
	}, nil
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (c *CartesiaTTSClient) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

// Metered reports that Cartesia charges per request.
func (c *CartesiaTTSClient) Metered() bool { return true }

// LastUsage reports character usage from the last synthesis request.
func (c *CartesiaTTSClient) LastUsage() media.Usage {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return c.lastUsage
}

func (c *CartesiaTTSClient) setLastUsage(text string) {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	c.lastUsage = media.Usage{
		Characters: len([]rune(text)),
		Requests:   1,
	}
}

// VoiceOptions declares the tunables Cartesia accepts.
func (c *CartesiaTTSClient) VoiceOptions() []media.VoiceOption {
	return []media.VoiceOption{
		{
			Key:     "emotion",
			Label:   "Emotion",
			Kind:    "enum",
			Options: []string{"neutral", "calm", "angry", "content", "sad", "scared", "happy", "excited", "curious", "sympathetic"},
			Default: "neutral",
			Help:    "Emotional tone for voice delivery.",
		},
		{
			Key:     "volume",
			Label:   "Volume",
			Kind:    "float",
			Min:     0.5,
			Max:     2.0,
			Step:    0.05,
			Default: 1.0,
			Help:    "Volume multiplier for speech generation.",
		},
	}
}

// SpeechCueCapabilities advertises Cartesia's support for inline tags and bracketed emotions.
func (c *CartesiaTTSClient) SpeechCueCapabilities() media.SpeechCueCapabilities {
	return media.SpeechCueCapabilities{
		AudioTags:        true,
		MarkdownEmphasis: false,
		SupportedTags: []string{
			"laughter", "sigh", "whisper", "gasp", "angry",
			"excited", "happy", "sad", "scared", "curious",
		},
		PromptGuidance: "Use bracketed emotions or inline cues immediately before dialogue to steer delivery.",
	}
}

type cartesiaErrorResponse struct {
	ErrorCode string `json:"error_code"`
	Title     string `json:"title"`
	Message   string `json:"message"`
}

func parseCartesiaError(body []byte, statusCode int) error {
	var errResp cartesiaErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && (errResp.ErrorCode != "" || errResp.Message != "") {
		if errResp.ErrorCode != "" && errResp.Message != "" {
			return fmt.Errorf("cartesia: %s - %s", errResp.ErrorCode, errResp.Message)
		}
		if errResp.Message != "" {
			return fmt.Errorf("cartesia: %s", errResp.Message)
		}
		if errResp.Title != "" {
			return fmt.Errorf("cartesia: %s", errResp.Title)
		}
	}
	return fmt.Errorf("cartesia: unexpected status %d: %s", statusCode, strings.TrimSpace(string(body)))
}

// Synthesize generates linear PCM WAV audio bytes for the given text.
func (c *CartesiaTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	voiceID := cartesiaDefaultVoiceID
	speed := 1.0
	volume := 1.0
	emotion := "neutral"
	language := "en"

	if voice != nil {
		if strings.TrimSpace(voice.VoiceID) != "" {
			voiceID = strings.TrimSpace(voice.VoiceID)
		}
		if voice.SpeechRate > 0 {
			speed = voice.SpeechRate
			if speed < 0.6 {
				speed = 0.6
			}
			if speed > 1.5 {
				speed = 1.5
			}
		}
		if voice.Options != nil {
			if v, ok := voice.Options["volume"]; ok {
				if f, ok := toFloat(v); ok {
					volume = f
					if volume < 0.5 {
						volume = 0.5
					}
					if volume > 2.0 {
						volume = 2.0
					}
				}
			}
			if e, ok := voice.Options["emotion"].(string); ok && strings.TrimSpace(e) != "" {
				emotion = strings.TrimSpace(e)
			}
			if l, ok := voice.Options["language"].(string); ok && strings.TrimSpace(l) != "" {
				language = strings.TrimSpace(l)
			}
		}
	}

	reqBody := map[string]interface{}{
		"model_id":   c.model,
		"transcript": text,
		"voice": map[string]interface{}{
			"id": voiceID,
		},
		"output_format": map[string]interface{}{
			"container":   "wav",
			"encoding":    "pcm_s16le",
			"sample_rate": 44100,
		},
		"generation_config": map[string]interface{}{
			"speed":   speed,
			"volume":  volume,
			"emotion": emotion,
		},
		"language": language,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("cartesia: encode request: %w", err)
	}

	endpoint := strings.TrimRight(c.baseURL, "/") + "/tts/bytes"

	var resp *http.Response
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("cartesia: create request: %w", err)
		}
		req.Header.Set("Cartesia-Version", cartesiaAPIVersion)
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err = c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("cartesia: request failed: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxAttempts {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryAfter):
				continue
			}
		}
		break
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cartesia: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseCartesiaError(bodyBytes, resp.StatusCode)
	}

	c.setLastUsage(text)
	return bodyBytes, nil
}

type cartesiaVoiceEntry struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Tagline        string `json:"tagline"`
	Description    string `json:"description"`
	Gender         string `json:"gender"`
	Language       string `json:"language"`
	PreviewFileURL string `json:"preview_file_url"`
	Accents        []struct {
		Accent   string `json:"accent"`
		Locale   string `json:"locale"`
		IsNative bool   `json:"is_native"`
	} `json:"accents"`
}

type cartesiaVoiceListResponse struct {
	Data     []cartesiaVoiceEntry `json:"data"`
	HasMore  bool                 `json:"has_more"`
	NextPage string               `json:"next_page"`
}

// ListVoices enumerates available voices from Cartesia's catalog.
func (c *CartesiaTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	var voices []media.ProviderVoice
	cursor := ""

	for {
		u, err := url.Parse(strings.TrimRight(c.baseURL, "/") + "/voices")
		if err != nil {
			return nil, fmt.Errorf("cartesia: parse url: %w", err)
		}
		q := u.Query()
		q.Set("limit", "100")
		q.Add("expand[]", "preview_file_url")
		if cursor != "" {
			q.Set("starting_after", cursor)
		}
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("cartesia: create voices request: %w", err)
		}
		req.Header.Set("Cartesia-Version", cartesiaAPIVersion)
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("cartesia: voices request failed: %w", err)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("cartesia: read voices response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, parseCartesiaError(bodyBytes, resp.StatusCode)
		}

		var listResp cartesiaVoiceListResponse
		if err := json.Unmarshal(bodyBytes, &listResp); err != nil {
			return nil, fmt.Errorf("cartesia: unmarshal voices response: %w", err)
		}

		for _, item := range listResp.Data {
			gender := normaliseGender(item.Gender)
			desc := strings.TrimSpace(item.Tagline)
			if item.Description != "" {
				if desc != "" {
					desc = desc + ": " + strings.TrimSpace(item.Description)
				} else {
					desc = strings.TrimSpace(item.Description)
				}
			}

			lang := strings.TrimSpace(item.Language)
			var tags []string
			if gender != "" {
				tags = append(tags, gender)
			}
			if len(item.Accents) > 0 {
				if lang == "" && item.Accents[0].Locale != "" {
					lang = item.Accents[0].Locale
				}
				for _, acc := range item.Accents {
					if acc.Accent != "" {
						tags = append(tags, strings.ToLower(acc.Accent))
					}
					if acc.Locale != "" {
						tags = append(tags, strings.ToLower(acc.Locale))
					}
				}
			}

			voices = append(voices, media.ProviderVoice{
				ID:          item.ID,
				Name:        item.Name,
				Language:    lang,
				Gender:      gender,
				Description: desc,
				PreviewURL:  item.PreviewFileURL,
				Tags:        tags,
			})
		}

		if !listResp.HasMore || listResp.NextPage == "" {
			break
		}
		cursor = listResp.NextPage
	}

	return voices, nil
}

func normaliseGender(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch lower {
	case "feminine", "female":
		return "female"
	case "masculine", "male":
		return "male"
	case "gender_neutral", "neutral":
		return "neutral"
	default:
		return lower
	}
}

func parseRetryAfter(raw string) time.Duration {
	if raw == "" {
		return 50 * time.Millisecond
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		if seconds <= 0 {
			return 10 * time.Millisecond
		}
		if seconds > 10 {
			seconds = 10
		}
		return time.Duration(seconds) * time.Second
	}
	return 50 * time.Millisecond
}

func toFloat(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}
