package sttcartesia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// ErrMissingAPIKey reports that a Cartesia STT client was built without a key.
var ErrMissingAPIKey = errors.New("cartesia: an API key is required; set media.stt.api_key, providers.cartesia.api_key, or CARTESIA_API_KEY")

const (
	cartesiaAPIVersion     = "2026-08-14"
	cartesiaDefaultBaseURL = "https://api.cartesia.ai"
	cartesiaDefaultModel   = "ink-whisper"
	cartesiaRequestTimeout = 60 * time.Second
)

// CartesiaSTTClient is the built-in provider for Cartesia Ink speech transcription.
// It implements media.STTClient and media.MeteredProvider.
type CartesiaSTTClient struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
	logger  trace.Logger

	usageMu   sync.Mutex
	lastUsage media.Usage
}

// NewCartesiaSTTClient builds a Cartesia STT client, resolving credentials in order:
// config API key -> shared provider key -> CARTESIA_API_KEY env var.
func NewCartesiaSTTClient(cfg config.STTConfig, sharedKey string) (*CartesiaSTTClient, error) {
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

	return &CartesiaSTTClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: cartesiaDefaultBaseURL,
		client:  &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: cartesiaRequestTimeout},
	}, nil
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (c *CartesiaSTTClient) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

// Metered reports that Cartesia STT charges per request.
func (c *CartesiaSTTClient) Metered() bool { return true }

// LastUsage reports usage metrics from the last transcription request.
func (c *CartesiaSTTClient) LastUsage() media.Usage {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return c.lastUsage
}

func (c *CartesiaSTTClient) setLastUsage() {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	c.lastUsage = media.Usage{
		Requests: 1,
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

type cartesiaSTTResponse struct {
	Type      string  `json:"type"`
	RequestID string  `json:"request_id"`
	Text      string  `json:"text"`
	Language  string  `json:"language"`
	Duration  float64 `json:"duration"`
}

// Transcribe uploads audio bytes to Cartesia's STT endpoint and returns transcribed text.
func (c *CartesiaSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	if len(audioData) == 0 {
		return "", errors.New("cartesia: empty audio data")
	}

	endpoint := strings.TrimRight(c.baseURL, "/") + "/stt"

	var resp *http.Response
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		buf := bytes.NewBuffer(nil)
		writer := multipart.NewWriter(buf)

		if err := writer.WriteField("model", c.model); err != nil {
			return "", fmt.Errorf("cartesia: write model field: %w", err)
		}

		part, err := writer.CreateFormFile("file", "audio.webm")
		if err != nil {
			return "", fmt.Errorf("cartesia: create form file: %w", err)
		}
		if _, err := part.Write(audioData); err != nil {
			return "", fmt.Errorf("cartesia: write audio data: %w", err)
		}
		if err := writer.Close(); err != nil {
			return "", fmt.Errorf("cartesia: close multipart writer: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, buf)
		if err != nil {
			return "", fmt.Errorf("cartesia: create request: %w", err)
		}
		req.Header.Set("Cartesia-Version", cartesiaAPIVersion)
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err = c.client.Do(req)
		if err != nil {
			return "", fmt.Errorf("cartesia: request failed: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxAttempts {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(retryAfter):
				continue
			}
		}
		break
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cartesia: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", parseCartesiaError(bodyBytes, resp.StatusCode)
	}

	var sttResp cartesiaSTTResponse
	if err := json.Unmarshal(bodyBytes, &sttResp); err != nil {
		return "", fmt.Errorf("cartesia: decode response: %w", err)
	}

	c.setLastUsage()
	return sttResp.Text, nil
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
