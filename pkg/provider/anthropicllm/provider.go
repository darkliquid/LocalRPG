// Package anthropicllm registers the Anthropic Messages API chat provider.
package anthropicllm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// anthropicVersion pins the API version every request carries, and it is the
// version whose rules this adapter obeys: temperature (and top_p/top_k) are
// removed, effort and thinking replace them.
const anthropicVersion = "2026-10-21"

// Options configures one Messages-API provider. Endpoint defaults to the
// public Anthropic API. Temperature/TopP/TopK exist only so a copied-over
// config is detected and explained rather than silently dropped; they are
// never sent. Effort maps to output_config.effort; PromptCache "off"
// suppresses the top-level cache_control the "auto" default sends;
// ThinkingBudget maps to manual thinking on tool-free requests only.
type Options struct {
	Endpoint       string
	Model          string
	APIKey         string
	Temperature    float64
	TopP           *float64
	TopK           *int
	MaxTokens      int
	Effort         string
	PromptCache    string
	ThinkingBudget *int
}

// AnthropicProvider talks to Anthropic's Messages API. It streams every call,
// including the ones Generate collects.
type AnthropicProvider struct {
	id                 string
	endpoint           string
	model              string
	apiKey             string
	opts               Options
	client             *http.Client
	logger             trace.Logger
	chunkLimitOverride int
	warned             bool
}

// NewAnthropicProvider builds the provider for one model on one endpoint.
func NewAnthropicProvider(id string, opts Options) *AnthropicProvider {
	endpoint := strings.TrimRight(opts.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.anthropic.com"
	}
	opts.Endpoint = endpoint
	return &AnthropicProvider{
		id:       id,
		endpoint: endpoint,
		model:    opts.Model,
		apiKey:   opts.APIKey,
		opts:     opts,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 5 * time.Minute},
		logger:   trace.Nop(),
	}
}

// ID is the configured provider id.
func (p *AnthropicProvider) ID() string { return p.id }

// ToolCallerCapable marks the adapter as able to accept tools and return calls.
func (p *AnthropicProvider) ToolCallerCapable() bool { return true }

// SetLogger attaches a trace sink. A nil logger records nothing.
func (p *AnthropicProvider) SetLogger(logger trace.Logger) { p.logger = trace.OrNil(logger) }

// SetChunkLimit bounds how many wire events one call records.
func (p *AnthropicProvider) SetChunkLimit(limit int) { p.chunkLimitOverride = limit }

func (p *AnthropicProvider) chunkLimit() int {
	if p.chunkLimitOverride <= 0 {
		return 500
	}
	return p.chunkLimitOverride
}

// contentBlock is one block of an Anthropic message: text, tool_use, or tool_result.
type contentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
}

// anthropicMessage is one turn on the wire. Anthropic has no system role
// messages and no tool role: a tool result is a tool_result block inside a
// user turn.
type anthropicMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
}

type anthropicCacheControl struct {
	Type string `json:"type"`
}

type anthropicOutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type anthropicRequest struct {
	Model        string                 `json:"model"`
	Messages     []anthropicMessage     `json:"messages"`
	System       string                 `json:"system,omitempty"`
	MaxTokens    int                    `json:"max_tokens"`
	Stream       bool                   `json:"stream"`
	Tools        []anthropicTool        `json:"tools,omitempty"`
	ToolChoice   *anthropicToolChoice   `json:"tool_choice,omitempty"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
	OutputConfig *anthropicOutputConfig `json:"output_config,omitempty"`
	Thinking     *anthropicThinking     `json:"thinking,omitempty"`
}

// buildSystem is the top-level system text: the request's System plus any
// system-role messages folded into the conversation, separated by a blank
// line the way the engine's single-prompt path rendered them.
func buildSystem(req harness.GenerateRequest) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(req.System) != "" {
		parts = append(parts, req.System)
	}
	for _, m := range req.Messages {
		if m.Role == "system" && strings.TrimSpace(m.Content) != "" {
			parts = append(parts, m.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

// mappedInput parses a tool call's Arguments, because Anthropic carries a
// tool_use block's input as a JSON object rather than a string. Malformed
// arguments pass through as one field rather than failing the turn.
func mappedInput(raw string) map[string]any {
	var input map[string]any
	if err := json.Unmarshal([]byte(raw), &input); err != nil || input == nil {
		return map[string]any{"arguments": raw}
	}
	return input
}

func mappedTools(req harness.GenerateRequest) []anthropicTool {
	tools := make([]anthropicTool, 0, len(req.Tools))
	for _, spec := range req.Tools {
		tools = append(tools, anthropicTool{Name: spec.Name, Description: spec.Description, InputSchema: spec.Parameters})
	}
	return tools
}

// warnOnce reports unsupported sampling parameters once per provider, so a
// role config copied from an OpenAI setup is explained, not silently dropped.
func (p *AnthropicProvider) warnOnce() {
	if p.warned {
		return
	}
	p.warned = true
	p.logger.Event("provider.unsupported", map[string]any{
		"role":   p.id,
		"params": "temperature, top_p, top_k",
		"reason": "removed by Anthropic API 2026-10-21; use effort instead",
	})
}

// buildRequest maps a generation request onto the wire shape. max_tokens is
// required at every level, so the cascade is request, then config, then 1024.
// Consecutive same-role messages merge: a tool result is a user block and the
// player's next turn is a user message, and Anthropic rejects a conversation
// that does not alternate.
func (p *AnthropicProvider) buildRequest(req harness.GenerateRequest) anthropicRequest {
	payload := anthropicRequest{
		Model:  p.model,
		System: buildSystem(req),
		Stream: true,
	}
	payload.MaxTokens = 1024
	if p.opts.MaxTokens > 0 {
		payload.MaxTokens = p.opts.MaxTokens
	}
	if req.MaxTokens > 0 {
		payload.MaxTokens = req.MaxTokens
	}
	if p.opts.PromptCache != "off" {
		payload.CacheControl = &anthropicCacheControl{Type: "ephemeral"}
	}
	if p.opts.Temperature != 0 || p.opts.TopP != nil || p.opts.TopK != nil {
		p.warnOnce()
	}
	if p.opts.Effort != "" {
		payload.OutputConfig = &anthropicOutputConfig{Effort: p.opts.Effort}
	}
	if len(req.Tools) > 0 {
		payload.Tools = mappedTools(req)
		switch req.ToolChoice {
		case "required":
			payload.ToolChoice = &anthropicToolChoice{Type: "any"}
		case "none":
			payload.Tools = nil
		}
	} else if p.opts.ThinkingBudget != nil && *p.opts.ThinkingBudget > 0 {
		payload.Thinking = &anthropicThinking{Type: "enabled", BudgetTokens: max(1024, *p.opts.ThinkingBudget)}
	}

	messages := make([]anthropicMessage, 0, len(req.Messages))
	push := func(role string, block contentBlock) {
		if n := len(messages); n > 0 && messages[n-1].Role == role {
			messages[n-1].Content = append(messages[n-1].Content, block)
			return
		}
		messages = append(messages, anthropicMessage{Role: role, Content: []contentBlock{block}})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			continue
		case "assistant":
			if strings.TrimSpace(m.Content) != "" {
				push("assistant", contentBlock{Type: "text", Text: m.Content})
			}
			for _, call := range m.ToolCalls {
				push("assistant", contentBlock{Type: "tool_use", ToolUseID: call.ID, Name: call.Name, Input: mappedInput(call.Arguments)})
			}
		case "tool":
			if m.ToolCallID == "" {
				push("user", contentBlock{Type: "text", Text: m.Content})
				continue
			}
			push("user", contentBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Text: m.Content})
		default:
			push("user", contentBlock{Type: "text", Text: m.Content})
		}
	}
	payload.Messages = messages
	return payload
}

func (p *AnthropicProvider) postRequest(ctx context.Context, data []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	if p.apiKey != "" {
		httpReq.Header.Set("x-api-key", p.apiKey)
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		p.logger.Event("provider.error", map[string]any{"role": p.id, "error": err.Error()})
		return nil, fmt.Errorf("http request: %w", err)
	}
	return resp, nil
}

// logRequest records the request shape without the content; the prompt itself
// is recorded once, on context.assembled.
func (p *AnthropicProvider) logRequest(req harness.GenerateRequest) {
	sum := sha256.Sum256([]byte(req.PromptText()))
	p.logger.Event("provider.request", map[string]any{
		"role":       p.id,
		"model":      p.model,
		"prompt_sha": hex.EncodeToString(sum[:]),
		"messages":   len(req.Messages),
		"tools":      len(req.Tools),
	})
}

// Generate streams the reply and collects it; implemented fully in the
// streaming step of the plan.
func (p *AnthropicProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return nil, fmt.Errorf("anthropicllm: stream not implemented")
}

// Stream forwards each delta as it arrives.
func (p *AnthropicProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	return fmt.Errorf("anthropicllm: stream not implemented")
}

var _ harness.ModelProvider = (*AnthropicProvider)(nil)
