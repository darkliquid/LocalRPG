package openaiembedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

type ClientConfig struct {
	Endpoint   string `json:"endpoint"`
	APIKey     string `json:"api_key"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
}

type Client struct {
	endpoint   string
	apiKey     string
	model      string
	dimensions int
	httpClient *http.Client

	mu        sync.Mutex
	lastUsage harness.Usage
}

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyEmbeddingOpenAI),
			Family:      provider.FamilyEmbedding,
			Label:       "OpenAI / Ollama Embedding API",
			Description: "Vector embeddings via standard OpenAI-compatible /v1/embeddings endpoint",
			Source:      "http",
			Tier:        provider.TierCloud,
			Features:    []provider.Feature{provider.FeatureKeyRequired},
		},
		Build: func(ctx context.Context, raw []byte) (interface{}, error) {
			var cfg ClientConfig
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &cfg); err != nil {
					return nil, err
				}
			}
			return NewClient(cfg), nil
		},
	})
}

func NewClient(cfg ClientConfig) *Client {
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	model := cfg.Model
	if model == "" {
		model = "text-embedding-3-small"
	}
	dims := cfg.Dimensions
	if dims <= 0 {
		if strings.Contains(model, "large") {
			dims = 3072
		} else {
			dims = 1536
		}
	}
	return &Client{
		endpoint:   endpoint,
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: dims,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) ID() string {
	return "openai-embedding"
}

func (c *Client) Dimensions() int {
	return c.dimensions
}

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// LastUsage reports the token usage of the most recent request, so the caller
// can price and record it. A zero value means nothing was reported.
func (c *Client) LastUsage() harness.Usage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastUsage
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	url := c.endpoint
	if !strings.HasSuffix(url, "/embeddings") {
		url += "/embeddings"
	}

	reqBody := embeddingRequest{
		Model: c.model,
		Input: texts,
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("embedding API error: %s", parsed.Error.Message)
	}

	result := make([][]float32, len(texts))
	for _, item := range parsed.Data {
		if item.Index >= 0 && item.Index < len(result) {
			result[item.Index] = item.Embedding
		}
	}

	c.mu.Lock()
	c.lastUsage = harness.Usage{
		Model:       c.model,
		InputTokens: parsed.Usage.PromptTokens,
		Requests:    1,
	}
	c.mu.Unlock()
	return result, nil
}

var _ embeddings.Provider = (*Client)(nil)
