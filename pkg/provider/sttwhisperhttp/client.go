package sttwhisperhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// NewHTTPSTTClient builds the HTTP transcription client.
func NewHTTPSTTClient(cfg config.STTConfig) media.STTClient {
	return &httpSTTClient{
		endpoint: cfg.Endpoint,
		model:    cfg.Model,
		apiKey:   cfg.APIKey,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 60 * time.Second},
	}
}

type httpSTTClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client

	usageMu   sync.Mutex
	lastUsage media.Usage
}

// LastUsage reports what the last transcription consumed. OpenAI-style response
// bodies may carry a usage block; when they do not, one request is the honest
// estimate.
func (h *httpSTTClient) LastUsage() media.Usage {
	h.usageMu.Lock()
	defer h.usageMu.Unlock()
	return h.lastUsage
}

func (h *httpSTTClient) setLastUsage(totalTokens int) {
	usage := media.Usage{Requests: 1, Estimated: true}
	if totalTokens > 0 {
		usage.InputTokens = totalTokens
		usage.Estimated = false
	}
	h.usageMu.Lock()
	h.lastUsage = usage
	h.usageMu.Unlock()
}

func (h *httpSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	modelName := h.model
	if modelName == "" {
		modelName = "whisper-1"
	}
	if err := writer.WriteField("model", modelName); err != nil {
		return "", fmt.Errorf("write model field: %w", err)
	}

	part, err := writer.CreateFormFile("file", "audio.webm")
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return "", fmt.Errorf("write audio data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.endpoint, &body)
	if err != nil {
		return "", fmt.Errorf("create stt request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("stt request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("stt failed (%d): %s", resp.StatusCode, string(respBytes))
	}

	var res struct {
		Text  string `json:"text"`
		Usage *struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("decode stt response: %w", err)
	}

	totalTokens := 0
	if res.Usage != nil {
		totalTokens = res.Usage.TotalTokens
	}
	h.setLastUsage(totalTokens)

	return strings.TrimSpace(res.Text), nil
}
