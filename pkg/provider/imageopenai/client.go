package imageopenai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// client calls one OpenAI image endpoint with one model and key.
type client struct {
	endpoint    string
	model       string
	apiKey      string
	quality     string
	aspectRatio string
	httpClient  *http.Client

	usageMu   sync.Mutex
	lastUsage media.Usage
}

// NewClient builds the OpenAI image client for one model and endpoint. The
// shared key covers a configuration that carries no key of its own.
func NewClient(cfg config.ImageConfig, sharedKey string) media.ImageClient {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = sharedKey
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/images/generations"
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-image-2.5-flare"
	}
	return &client{
		endpoint:    endpoint,
		model:       model,
		apiKey:      apiKey,
		quality:     cfg.Quality,
		aspectRatio: cfg.AspectRatio,
		httpClient:  &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 120 * time.Second},
	}
}

// LastUsage reports what the last image generation consumed.
func (c *client) LastUsage() media.Usage {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return c.lastUsage
}

func (c *client) setLastUsage(u media.Usage) {
	c.usageMu.Lock()
	c.lastUsage = u
	c.usageMu.Unlock()
}

// sizeFor maps the config's aspect vocabulary onto OpenAI's three sizes. An
// unknown or empty ratio is square, which is what the pipeline assumes.
func sizeFor(aspectRatio string) string {
	switch aspectRatio {
	case "3:2", "16:9":
		return "1536x1024"
	case "2:3":
		return "1024x1536"
	default:
		return "1024x1024"
	}
}

// GenerateImage renders one image. The gpt-image models always return b64
// payloads and reject the legacy response_format parameter, so it is never
// sent. A quality of "auto" is omitted so the model picks.
func (c *client) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	c.setLastUsage(media.Usage{Requests: 1, Estimated: true})
	payload := map[string]any{
		"model":         c.model,
		"prompt":        prompt,
		"n":             1,
		"size":          sizeFor(c.aspectRatio),
		"output_format": "png",
	}
	if c.quality != "" && c.quality != "auto" {
		payload["quality"] = c.quality
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal image request: %w", err)
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
		return nil, fmt.Errorf("openai image request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, fmt.Errorf("openai image failed (%d): %s", resp.StatusCode, provider.TruncateDetail(b))
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai image body: %w", err)
	}
	var respData struct {
		Data []struct {
			B64 string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return nil, fmt.Errorf("openai image response: %w", err)
	}
	if len(respData.Data) == 0 || respData.Data[0].B64 == "" {
		return nil, fmt.Errorf("openai image response carried no image")
	}
	image, err := base64.StdEncoding.DecodeString(respData.Data[0].B64)
	if err != nil {
		return nil, fmt.Errorf("decode openai b64_json: %w", err)
	}
	return image, nil
}
