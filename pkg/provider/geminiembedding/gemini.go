package geminiembedding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/provider"
	"google.golang.org/genai"
)

type ClientConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

type Client struct {
	apiKey     string
	model      string
	dimensions int
}

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "gemini-embedding",
			Family:      provider.FamilyEmbedding,
			Label:       "Google Gemini Embeddings",
			Description: "Vector embeddings via Google GenAI embedding API (text-embedding-004)",
			Source:      "gemini",
		},
		Build: func(ctx context.Context, raw []byte) (any, error) {
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
	model := cfg.Model
	if model == "" {
		model = "text-embedding-004"
	}
	return &Client{
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: 768,
	}
}

func (c *Client) ID() string {
	return "gemini-embedding"
}

func (c *Client) Dimensions() int {
	return c.dimensions
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create gemini client: %w", err)
	}

	modelName := c.model
	if !strings.HasPrefix(modelName, "models/") {
		modelName = "models/" + modelName
	}

	contents := make([]*genai.Content, len(texts))
	for i, text := range texts {
		contents[i] = &genai.Content{
			Parts: []*genai.Part{{Text: text}},
		}
	}

	resp, err := client.Models.EmbedContent(ctx, modelName, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("gemini embed content: %w", err)
	}

	result := make([][]float32, len(texts))
	for i, emb := range resp.Embeddings {
		if i < len(result) && emb != nil {
			result[i] = emb.Values
		}
	}
	return result, nil
}

var _ embeddings.Provider = (*Client)(nil)
