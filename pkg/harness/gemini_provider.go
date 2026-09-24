package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/telemetry"
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
			APIKey:     apiKey,
			Backend:    genai.BackendGeminiAPI,
			HTTPClient: &http.Client{Transport: telemetry.HTTPTransport(nil)},
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

func (g *GeminiProvider) buildGenerateConfig(req GenerateRequest) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{}

	// System instruction
	if sys := strings.TrimSpace(req.System); sys != "" {
		cfg.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: sys}},
		}
	}

	// Temperature
	if req.Temperature > 0 {
		temp := float32(req.Temperature)
		cfg.Temperature = &temp
	} else if g.temperature != nil {
		temp := float32(*g.temperature)
		cfg.Temperature = &temp
	}

	// Max tokens
	if req.MaxTokens > 0 {
		cfg.MaxOutputTokens = int32(req.MaxTokens)
	} else if g.maxTokens != nil {
		cfg.MaxOutputTokens = int32(*g.maxTokens)
	}

	// TopP and TopK
	if g.topP != nil {
		topP := float32(*g.topP)
		cfg.TopP = &topP
	}
	if g.topK != nil {
		topK := float32(*g.topK)
		cfg.TopK = &topK
	}

	// ThinkingConfig
	if g.thinkingBudget != nil {
		cfg.ThinkingConfig = &genai.ThinkingConfig{}
		budget := *g.thinkingBudget
		if budget == 0 {
			cfg.ThinkingConfig.ThinkingBudget = genai.Ptr(int32(0))
			cfg.ThinkingConfig.IncludeThoughts = false
		} else if budget > 0 {
			cfg.ThinkingConfig.ThinkingBudget = genai.Ptr(int32(budget))
			cfg.ThinkingConfig.IncludeThoughts = true
		} else {
			// -1 indicates dynamic thinking
			cfg.ThinkingConfig.IncludeThoughts = true
		}
	}

	// Tools
	if len(req.Tools) > 0 {
		var declarations []*genai.FunctionDeclaration
		for _, tool := range req.Tools {
			decl := &genai.FunctionDeclaration{
				Name:                 tool.Name,
				Description:          tool.Description,
				ParametersJsonSchema: tool.Parameters,
			}
			declarations = append(declarations, decl)
		}
		cfg.Tools = []*genai.Tool{
			{FunctionDeclarations: declarations},
		}
	}

	return cfg
}

func (g *GeminiProvider) buildContents(req GenerateRequest) []*genai.Content {
	if len(req.Messages) == 0 {
		promptText := req.PromptText()
		if promptText == "" {
			return nil
		}
		return []*genai.Content{
			{
				Role:  "user",
				Parts: []*genai.Part{{Text: promptText}},
			},
		}
	}

	var contents []*genai.Content
	// Gemini matches a function response to its call by name (and optionally id),
	// but the harness tool message only carries the call id. Remember each
	// assistant call in order so a tool message can recover the function name;
	// the orchestrator appends responses in the same order it received the calls.
	type pendingToolCall struct {
		id   string
		name string
	}
	var pendingToolCalls []pendingToolCall

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			// Handled in SystemInstruction
			continue
		case "user":
			contents = append(contents, &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{Text: msg.Content}},
			})
		case "assistant":
			var parts []*genai.Part
			if strings.TrimSpace(msg.Content) != "" {
				parts = append(parts, &genai.Part{Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Arguments), &args)
				parts = append(parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   tc.ID,
						Name: tc.Name,
						Args: args,
					},
				})
				pendingToolCalls = append(pendingToolCalls, pendingToolCall{id: tc.ID, name: tc.Name})
			}
			if len(parts) > 0 {
				contents = append(contents, &genai.Content{
					Role:  "model",
					Parts: parts,
				})
			}
		case "tool":
			var respMap map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Content), &respMap); err != nil {
				respMap = map[string]interface{}{"result": msg.Content}
			}

			// Recover the function name from the matching call. Prefer an id
			// match, then fall back to positional order when ids are absent,
			// which is the common case for Gemini.
			call := pendingToolCall{}
			matchIndex := -1
			if msg.ToolCallID != "" {
				for i, candidate := range pendingToolCalls {
					if candidate.id == msg.ToolCallID {
						call = candidate
						matchIndex = i
						break
					}
				}
			}
			if matchIndex < 0 && len(pendingToolCalls) > 0 {
				call = pendingToolCalls[0]
				matchIndex = 0
			}
			if matchIndex >= 0 {
				pendingToolCalls = append(pendingToolCalls[:matchIndex], pendingToolCalls[matchIndex+1:]...)
			}

			response := &genai.FunctionResponse{
				Name:     call.name,
				Response: respMap,
			}
			if call.id != "" && call.id == msg.ToolCallID {
				response.ID = call.id
			}
			contents = append(contents, &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{FunctionResponse: response}},
			})
		}
	}
	return contents
}

func (g *GeminiProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	cfg := g.buildGenerateConfig(req)
	contents := g.buildContents(req)

	resp, err := g.client.Models.GenerateContent(ctx, g.model, contents, cfg)
	if err != nil {
		return nil, mapGeminiError(err)
	}

	var sb strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if part.Thought {
				continue
			}
			if part.Text != "" {
				sb.WriteString(part.Text)
			}
		}
	}

	return &GenerateResponse{Text: sb.String()}, nil
}

func (g *GeminiProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)

	cfg := g.buildGenerateConfig(req)
	contents := g.buildContents(req)

	iter := g.client.Models.GenerateContentStream(ctx, g.model, contents, cfg)

	for resp, err := range iter {
		if err != nil {
			mappedErr := mapGeminiError(err)
			out <- StreamChunk{Error: mappedErr, Done: true}
			return mappedErr
		}

		for _, cand := range resp.Candidates {
			if cand.Content == nil {
				continue
			}

			var toolCalls []ToolCall
			var textParts []string

			for _, part := range cand.Content.Parts {
				if part.Thought {
					if g.logger != nil && part.Text != "" {
						g.logger.Event("gemini_thought", map[string]interface{}{
							"text": part.Text,
						})
					}
					continue
				}

				if part.Text != "" {
					textParts = append(textParts, part.Text)
				}

				if part.FunctionCall != nil {
					argsBytes, _ := json.Marshal(part.FunctionCall.Args)
					toolCalls = append(toolCalls, ToolCall{
						ID:        part.FunctionCall.ID,
						Name:      part.FunctionCall.Name,
						Arguments: string(argsBytes),
					})
				}
			}

			if len(textParts) > 0 || len(toolCalls) > 0 {
				out <- StreamChunk{
					Text:      strings.Join(textParts, ""),
					ToolCalls: toolCalls,
				}
			}
		}
	}

	out <- StreamChunk{Done: true}
	return nil
}

func mapGeminiError(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") || strings.Contains(errStr, "PERMISSION_DENIED") {
		return errors.New("gemini: invalid API key or permission denied; check providers.gemini.api_key or GEMINI_API_KEY")
	}
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "RESOURCE_EXHAUSTED") {
		return errors.New("gemini: quota exceeded or rate limit reached; check your Google AI Studio plan and credits")
	}
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "NOT_FOUND") {
		return fmt.Errorf("gemini: model not found: %w", err)
	}
	return fmt.Errorf("gemini: request failed: %w", err)
}

func MapGeminiErrorForTest(err error) error {
	return mapGeminiError(err)
}
