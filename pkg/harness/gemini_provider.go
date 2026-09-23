package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/trace"
)

var ErrGeminiAPIKeyRequired = errors.New("gemini: an API key is required; set providers.gemini.api_key, agents.roles.<role>.api_key, or GEMINI_API_KEY")

func ResolveGeminiAPIKey(roleKey, sharedKey string) (string, error) {
	if k := strings.TrimSpace(roleKey); k != "" {
		return k, nil
	}
	if k := strings.TrimSpace(sharedKey); k != "" {
		return k, nil
	}
	if k := strings.TrimSpace(os.Getenv("GEMINI_API_KEY")); k != "" {
		return k, nil
	}
	if k := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")); k != "" {
		return k, nil
	}
	return "", ErrGeminiAPIKeyRequired
}

type GeminiProvider struct {
	id                 string
	model              string
	apiKey             string
	thinkingBudget     *int
	temperature        *float64
	topP               *float64
	topK               *int
	maxTokens          *int
	client             *genai.Client
	logger             trace.Logger
	chunkLimitOverride int
}

type GeminiProviderOptions struct {
	Model          string
	APIKey         string
	ThinkingBudget *int
	Temperature    *float64
	TopP           *float64
	TopK           *int
	MaxTokens      *int
	Client         *genai.Client // Optional client override for testing
}

func NewGeminiProvider(id string, opts GeminiProviderOptions) (*GeminiProvider, error) {
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = "gemini-2.5-flash"
	}

	apiKey := opts.APIKey
	client := opts.Client
	if client == nil {
		ctx := context.Background()
		var err error
		client, err = genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  apiKey,
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			return nil, fmt.Errorf("gemini: create client: %w", err)
		}
	}

	return &GeminiProvider{
		id:             id,
		model:          model,
		apiKey:         apiKey,
		thinkingBudget: opts.ThinkingBudget,
		temperature:    opts.Temperature,
		topP:           opts.TopP,
		topK:           opts.TopK,
		maxTokens:      opts.MaxTokens,
		client:         client,
	}, nil
}

func (g *GeminiProvider) ID() string {
	return g.id
}

func (g *GeminiProvider) ToolCallerCapable() bool {
	return true
}

func (g *GeminiProvider) SetLogger(logger trace.Logger) {
	g.logger = trace.OrNil(logger)
}

func (g *GeminiProvider) SetChunkLimit(limit int) {
	g.chunkLimitOverride = limit
}
