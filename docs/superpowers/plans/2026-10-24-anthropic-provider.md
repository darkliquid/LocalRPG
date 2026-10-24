# Anthropic Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A first-class Anthropic LLM provider: `pkg/provider/anthropicllm` speaking the Messages API natively (content blocks, tool loops, Anthropic SSE, prompt caching, effort), with presets and pricing.

**Architecture:** One new self-registering package, `pkg/provider/anthropicllm`, hand-rolled HTTP like `openaichat` (no SDK). Config selects it with `type: anthropic`, key `llm:anthropic`, shared key `providers.anthropic.api_key`. Media families are deliberately untouched: Anthropic has no image/TTS/STT/embeddings API.

**Tech Stack:** Go stdlib (`net/http`, `encoding/json`, `bufio`), `harness.ModelProvider`/`StreamChunk`, `pkg/trace`, `pkg/telemetry.HTTPTransport`, Vitest for the frontend edit.

**Spec:** `docs/superpowers/specs/2026-10-24-anthropic-provider-design.md` (bead LocalRPG-00i). Sections 5.2 (request table) and 5.3 (streaming table) are the contract; the plan carries the code.

## Global Constraints

- Work on branch `feat/anthropic-provider`; never commit on `main`.
- Stdlib-only tests (`testing`, `httptest`); no testify.
- Wrap errors with `fmt.Errorf("...: %w", err)`; error bodies read through `io.LimitReader(resp.Body, provider.MaxProviderDetailBytes)` and `provider.TruncateDetail`.
- Every exported symbol gets a doc comment answering what it is for (`mise run lint:revive` must not grow).
- `*http.Client` built as `&http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: ...}`.
- `gofmt`, `go vet`, staticcheck clean; no em dashes in code comments.
- The request **never** sends `temperature`, `top_p` or `top_k` (400 under API version `2026-10-21`), and always sends `max_tokens`. `anthropic-version: 2026-10-21` is a compile-time constant.
- The wire keeps strict user/assistant alternation: the converter merges consecutive same-role messages, because a tool result becomes a user-role block and the player's next turn is a user message too.
- Model IDs and prices re-verified against `https://platform.claude.com/docs/en/models/overview` at implementation time; rows below captured 2026-10-24.
- API keys are never defaulted or logged; docs use `api_key: "env:ANTHROPIC_API_KEY"`.
- If the OpenAI plan (`docs/superpowers/plans/2026-10-24-openai-provider-family.md`) landed first, `harness.ProviderConfig.Effort` and `config.AgentRoleConfig.Effort` already exist: skip that struct edit, keep everything else. If not, Task 1 adds them here.

---

### Task 1: Branch, Config Fields, Registry Key, Wiring

**Files:**
- Create: branch `feat/anthropic-provider`
- Modify: `pkg/harness/types.go` (ProviderConfig: `Effort` add-if-absent, `PromptCache` new), `pkg/config/types.go` (AgentRoleConfig: `Effort` add-if-absent, `PromptCache` new; ProvidersConfig: `Anthropic AnthropicProviderConfig`; new `AnthropicProviderConfig` struct beside `InworldProviderConfig`), `pkg/provider/keys.go` (`KeyLLMAnthropic` + `AllKeys()`), `pkg/harness/exports.go` (`KeyFor` case `"anthropic"`), `pkg/harness/factory.go` (`NewModelProvider` switch clause, `sharedKeyFor`, the four ProviderConfig copy sites at ~202-216, ~309-320, ~370-390), `pkg/harness/summariser.go:~167-183` (same two fields)
- Test: `pkg/harness/key_test.go` (add case)

**Interfaces:**
- Produces: `provider.KeyLLMAnthropic Key = "llm:anthropic"`; `harness.ProviderConfig.PromptCache string` (`"auto"` default behaviour, `"off"` disables); `config.ProvidersConfig.Anthropic.AnthropicProviderConfig{APIKey string}`; `KeyFor` case `"anthropic"` → `llm:anthropic` (instance-only discrimination, like Gemini).

- [ ] **Step 1: Branch**

```bash
git checkout -b feat/anthropic-provider
```

- [ ] **Step 2: Write the failing key test**

Add to `pkg/harness/key_test.go`:

```go
func TestKeyForAnthropic(t *testing.T) {
	key, ok := harness.KeyFor(harness.ProviderConfig{Type: "anthropic"})
	if !ok || key != provider.KeyLLMAnthropic {
		t.Fatalf("KeyFor(anthropic) = %q, %v; want %q, true", key, ok, provider.KeyLLMAnthropic)
	}
	key, ok = harness.KeyFor(harness.ProviderConfig{Type: "anthropic", Instance: "work"})
	if !ok || string(key) != "llm:anthropic@work" {
		t.Fatalf("KeyFor(anthropic@work) = %q, %v", key, ok)
	}
}
```

Run: `go test ./pkg/harness -run TestKeyForAnthropic -count=1`
Expected: FAIL (`KeyLLMAnthropic` undefined).

- [ ] **Step 3: Add the key and fields**

`pkg/provider/keys.go` const block (LLM group, next to `KeyLLMGemini`) and `AllKeys()`:

```go
	KeyLLMAnthropic Key = "llm:anthropic"
```

`pkg/harness/types.go` `ProviderConfig`: ensure `Effort` exists (add it here if the OpenAI plan has not landed), and add:

```go
	// PromptCache is "auto" (default: send Anthropic's top-level cache_control
	// so the growing narration context reads at a fraction of base input) or
	// "off". Other providers ignore it.
	PromptCache string `yaml:"prompt_cache,omitempty"`
```

`pkg/config/types.go`:

```go
// In AgentRoleConfig, beside ThinkingBudget (add Effort first if absent):

	// PromptCache is "auto" (default) or "off"; see harness.ProviderConfig.PromptCache.
	PromptCache string `yaml:"prompt_cache,omitempty" json:"prompt_cache,omitempty"`

// In ProvidersConfig:

	Anthropic AnthropicProviderConfig `yaml:"anthropic,omitempty" json:"anthropic,omitempty"`

// Beside InworldProviderConfig:

// AnthropicProviderConfig is the Anthropic-specific part of the provider
// configuration.
type AnthropicProviderConfig struct {
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}
```

- [ ] **Step 4: Wire the resolvers**

`pkg/harness/exports.go` `KeyFor`, add a case before `"builtin"`:

```go
	case "anthropic":
		return provider.InstanceOrSelf(provider.KeyLLMAnthropic, provider.InstanceDiscriminator(cfg.Instance, "")), true
```

`pkg/harness/factory.go`:
- `NewModelProvider` switch: extend `case "cli", "http", "gemini", "inworld", "":` with `"anthropic"` (and `"openai"` if the OpenAI plan has not landed).
- `sharedKeyFor` (replace the whole function; the `KeyLLMOpenAI` line exists already if the OpenAI plan landed, add-if-absent):

```go
func sharedKeyFor(cfg *config.Config, key provider.Key, hasKey bool) string {
	if hasKey {
		switch key.Parent() {
		case provider.KeyLLMInworld:
			return cfg.Providers.Inworld.APIKey
		case provider.KeyLLMOpenAI:
			return cfg.Providers.OpenAI.APIKey
		case provider.KeyLLMAnthropic:
			return cfg.Providers.Anthropic.APIKey
		}
	}
	return cfg.Providers.Gemini.APIKey
}
```

- All four copy sites (factory.go registerConfiguredRoles/ExtractorFromConfig/CompletionFromConfig, summariser.go): add the two lines beside the existing `ThinkingBudget`/`TopP`/`TopK` block:

```go
		Effort:      roleCfg.Effort,
		PromptCache: roleCfg.PromptCache,
```

- [ ] **Step 5: Verify and commit**

```bash
go test ./pkg/harness ./pkg/config ./pkg/provider -count=1 && go test ./pkg/gui -update-docs
```

```bash
git add -A && git commit -m "feat(provider): anthropic key, prompt_cache config, and wiring"
```

---

### Task 2: `anthropicllm` Adapter — Request Layer

**Files:**
- Create: `pkg/provider/anthropicllm/provider.go` (struct, options, request building), `pkg/provider/anthropicllm/anthropicllm.go` (registration)
- Test: `pkg/provider/anthropicllm/provider_test.go`

**Interfaces:**
- Consumes: Task 1's wiring; `harness.GenerateRequest`, `harness.ModelBuildPayload`.
- Produces: `anthropicllm.NewAnthropicProvider(id string, opts Options) *AnthropicProvider`; `Options{Endpoint, Model, APIKey string; Temperature float64; TopP *float64; TopK *int; MaxTokens int; Effort string; PromptCache string; ThinkingBudget *int}` (the sampling fields exist only to detect and warn about them); methods `ID`, `Generate`, `Stream`, `ToolCallerCapable` (true), `SetLogger`, `SetChunkLimit`. Tests live in package `anthropicllm` and exercise the unexported `buildRequest` directly.

- [ ] **Step 1: Write the failing request-shape tests**

```go
func TestBuildRequestShape(t *testing.T) {
	p := NewAnthropicProvider("gm", Options{Model: "claude-sonnet-5-5"})
	req := harness.GenerateRequest{
		System: "You are the GM.",
		Messages: []harness.Message{
			{Role: "system", Content: "Rules: be dramatic."},
			{Role: "user", Content: "look around"},
			{Role: "assistant", Content: "A cellar.", ToolCalls: []harness.ToolCall{{ID: "t1", Name: "roll", Arguments: `{"d":"d20"}`}}},
			{Role: "tool", ToolCallID: "t1", Content: "14"},
			{Role: "user", Content: "climb down"},
		},
		Tools:      []harness.ToolSpec{{Name: "roll", Description: "roll dice", Parameters: map[string]any{"type": "object"}}},
		ToolChoice: "required",
	}
	out := p.buildRequest(req)

	if out.System != "You are the GM.\n\nRules: be dramatic." {
		t.Errorf("system = %q", out.System)
	}
	if out.MaxTokens != 1024 {
		t.Errorf("max_tokens = %d, want the 1024 default", out.MaxTokens)
	}
	// system folded out, tool folded into a user block, the two user turns merged:
	// user(look around), assistant(text + tool_use), user(tool_result + climb down)
	if len(out.Messages) != 3 {
		t.Fatalf("messages = %d, want 3: %v", len(out.Messages), out.Messages)
	}
	assistant := out.Messages[1]
	if assistant.Role != "assistant" || len(assistant.Content) != 2 {
		t.Fatalf("assistant = %+v", assistant)
	}
	if assistant.Content[0].Type != "text" || assistant.Content[0].Text != "A cellar." {
		t.Errorf("assistant text block = %+v", assistant.Content[0])
	}
	use := assistant.Content[1]
	if use.Type != "tool_use" || use.ToolUseID != "t1" || use.Name != "roll" || use.Input["d"] != "d20" {
		t.Errorf("tool_use block = %+v", use)
	}
	merged := out.Messages[2]
	if merged.Role != "user" || len(merged.Content) != 2 {
		t.Fatalf("merged user = %+v", merged)
	}
	if merged.Content[0].Type != "tool_result" || merged.Content[0].ToolUseID != "t1" || merged.Content[0].Text != "14" {
		t.Errorf("tool_result block = %+v", merged.Content[0])
	}
	if merged.Content[1].Type != "text" || merged.Content[1].Text != "climb down" {
		t.Errorf("trailing user text = %+v", merged.Content[1])
	}
	if len(out.Tools) != 1 || out.Tools[0].Name != "roll" || out.ToolChoice == nil || out.ToolChoice.Type != "any" {
		t.Errorf("tools = %+v tool_choice = %+v", out.Tools, out.ToolChoice)
	}
	if out.CacheControl == nil || out.CacheControl.Type != "ephemeral" {
		t.Errorf("cache_control = %+v, want ephemeral under the auto default", out.CacheControl)
	}
}

func TestBuildRequestSamplingCascade(t *testing.T) {
	p := NewAnthropicProvider("gm", Options{Model: "m", MaxTokens: 0, PromptCache: "off"})
	out := p.buildRequest(harness.GenerateRequest{Prompt: "hi", MaxTokens: 512})
	if out.MaxTokens != 512 {
		t.Errorf("max_tokens = %d, want 512", out.MaxTokens)
	}
	if out.CacheControl != nil {
		t.Errorf("cache_control sent with prompt_cache off")
	}
	if out.Thinking != nil {
		t.Errorf("thinking sent without a budget: %+v", out.Thinking)
	}
}

func TestBuildRequestThinkingBudget(t *testing.T) {
	budget := 2048
	p := NewAnthropicProvider("gm", Options{Model: "m", ThinkingBudget: &budget})
	out := p.buildRequest(harness.GenerateRequest{Prompt: "hi"})
	if out.Thinking == nil || out.Thinking.Type != "enabled" || out.Thinking.BudgetTokens != 2048 {
		t.Errorf("thinking = %+v, want enabled/2048", out.Thinking)
	}
	withTools := p.buildRequest(harness.GenerateRequest{Prompt: "hi", Tools: []harness.ToolSpec{{Name: "t"}}})
	if withTools.Thinking != nil {
		t.Errorf("thinking sent on a tool request: %+v", withTools.Thinking)
	}
	zero := 0
	off := NewAnthropicProvider("gm", Options{Model: "m", ThinkingBudget: &zero})
	if got := off.buildRequest(harness.GenerateRequest{Prompt: "hi"}); got.Thinking != nil {
		t.Errorf("thinking sent with a zero budget: %+v", got.Thinking)
	}
}
```

Run: `go test ./pkg/provider/anthropicllm -count=1`
Expected: FAIL (package does not exist).

- [ ] **Step 2: Implement the request layer**

`pkg/provider/anthropicllm/provider.go`:

```go
// Package anthropicllm registers the Anthropic Messages API chat provider.
package anthropicllm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
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

var _ harness.ModelProvider = (*AnthropicProvider)(nil)
```

The two interface methods land as stubs now (Task 3 replaces them):

```go
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
```

`pkg/provider/anthropicllm/anthropicllm.go` (registration):

```go
// Package anthropicllm registers the Anthropic Messages API chat provider.
package anthropicllm

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyLLMAnthropic),
			Family:      provider.FamilyLLM,
			Label:       "Anthropic Claude (Messages API)",
			Description: "Hosted Claude models over the Messages API: tool calling, prompt caching and reasoning effort included.",
			Source:      "anthropic",
			Tier:        provider.TierCloud,
			Features:    []provider.Feature{provider.FeatureStreaming, provider.FeatureTools, provider.FeatureKeyRequired, provider.FeatureContextCache},
			Presets: []provider.Preset{
				{ID: "anthropic-sonnet", Order: 9, Label: "Claude Sonnet 5.5 (Cloud API)",
					Description: "The recommended Claude narration model. Good balance of cost and depth.",
					Config: map[string]any{
						"type": "anthropic", "model": "claude-sonnet-5-5", "max_tokens": 4096,
					}},
				{ID: "anthropic-haiku", Order: 10, Label: "Claude Haiku 5.5 (Cloud API)",
					Description: "Budget Claude for the extractor and completion roles.",
					Config: map[string]any{
						"type": "anthropic", "model": "claude-haiku-5-5", "max_tokens": 2048,
					}},
				{ID: "anthropic-opus", Order: 11, Label: "Claude Opus 5.5 (Cloud API)",
					Description: "Deeper narration for a higher rate.",
					Config: map[string]any{
						"type": "anthropic", "model": "claude-opus-5-5", "max_tokens": 4096,
					}},
				{ID: "anthropic-fable", Order: 12, Label: "Claude Fable 5.1 (Cloud API)",
					Description: "The frontier model, for the most demanding campaigns.",
					Config: map[string]any{
						"type": "anthropic", "model": "claude-fable-5-1", "max_tokens": 4096,
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (any, error) {
			var payload harness.ModelBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			id := payload.ID
			if id == "" {
				id = "anthropic"
			}
			apiKey := payload.Config.APIKey
			if apiKey == "" {
				apiKey = payload.Config.SharedAPIKey
			}
			return NewAnthropicProvider(id, Options{
				Endpoint:       payload.Config.Endpoint,
				Model:          payload.Config.Model,
				APIKey:         apiKey,
				Temperature:    payload.Config.Temperature,
				TopP:           payload.Config.TopP,
				TopK:           payload.Config.TopK,
				MaxTokens:      payload.Config.MaxTokens,
				Effort:         payload.Config.Effort,
				PromptCache:    payload.Config.PromptCache,
				ThinkingBudget: payload.Config.ThinkingBudget,
			}), nil
		},
	})
}
```

Add `_ "github.com/darkliquid/localrpg/pkg/provider/anthropicllm"` to `pkg/provider/all/all.go` (first import, alphabetical).

- [ ] **Step 3: Verify**

Run: `go build ./... && go test ./pkg/provider/anthropicllm -run TestBuildRequest -count=1`
Expected: PASS. `go test ./pkg/provider -count=1` keeps `Validate` green.

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(provider): anthropic messages request layer and registration"
```

---

### Task 3: `anthropicllm` SSE Stream, Tool Blocks, Cache-Aware Usage

**Files:**
- Create: `pkg/provider/anthropicllm/stream.go`
- Modify: `pkg/provider/anthropicllm/provider.go` (replace the two stubs with the real `Generate`/`Stream`)
- Test: `pkg/provider/anthropicllm/provider_test.go`

**Interfaces:**
- Produces: `Generate`/`Stream` final contract: text deltas as chunks, one final `StreamChunk{Done, FinishReason, ToolCalls, Usage}`; `GenerateResponse.CachedTokens` carries the cache-read count; `FinishReason` values `stop`, `length`, `tool_calls`; a `refusal` stop surfaces as a chunk error.

- [ ] **Step 1: Write the failing stream tests**

```go
func TestStreamTextUsageAndHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("anthropic-version") != anthropicVersion {
			t.Errorf("anthropic-version = %q, want %q", r.Header.Get("anthropic-version"), anthropicVersion)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Error("x-api-key missing")
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q, want /v1/messages", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, sent := body["temperature"]; sent {
			t.Errorf("temperature sent: %v", body)
		}
		if body["max_tokens"] == nil {
			t.Errorf("max_tokens missing: %v", body)
		}
		if body["cache_control"] == nil {
			t.Errorf("cache_control missing under the auto default: %v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			"event: message_start",
			`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":100}}}`,
			"",
			"event: ping",
			`data: {"type":"ping"}`,
			"",
			"event: content_block_start",
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			"",
			"event: content_block_delta",
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Once "}}`,
			"",
			"event: content_block_delta",
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"upon a time."}}`,
			"",
			"event: content_block_stop",
			`data: {"type":"content_block_stop","index":0}`,
			"",
			"event: message_delta",
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
			"",
			"event: message_stop",
			`data: {"type":"message_stop"}`,
			"",
		}
		for _, f := range frames {
			fmt.Fprintln(w, f)
		}
	}))
	t.Cleanup(server.Close)

	p := NewAnthropicProvider("gm", Options{Endpoint: server.URL, Model: "claude-sonnet-5-5", APIKey: "k"})
	var received []string
	var usage *harness.Usage
	out := make(chan harness.StreamChunk, 16)
	if err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "Tell a story"}, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for chunk := range out {
		if chunk.Text != "" {
			received = append(received, chunk.Text)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	if got := strings.Join(received, ""); got != "Once upon a time." {
		t.Errorf("text = %q", got)
	}
	if usage == nil || usage.InputTokens != 100 || usage.OutputTokens != 4 {
		t.Errorf("usage = %+v, want 100/4", usage)
	}
}

func TestStreamToolUseBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			"event: content_block_start",
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			"",
			"event: content_block_delta",
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me check. "}}`,
			"",
			"event: content_block_stop",
			`data: {"type":"content_block_stop","index":0}`,
			"",
			"event: content_block_start",
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"search_entities","input":{}}}`,
			"",
			"event: content_block_delta",
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"que"}}`,
			"",
			"event: content_block_delta",
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"ry\":\"Kael\"}"}}`,
			"",
			"event: content_block_stop",
			`data: {"type":"content_block_stop","index":1}`,
			"",
			"event: message_delta",
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}`,
			"",
			"event: message_stop",
			`data: {"type":"message_stop"}`,
			"",
		}
		for _, f := range frames {
			fmt.Fprintln(w, f)
		}
	}))
	t.Cleanup(server.Close)

	p := NewAnthropicProvider("gm", Options{Endpoint: server.URL, Model: "claude-sonnet-5-5", APIKey: "k"})
	out := make(chan harness.StreamChunk, 16)
	var calls []harness.ToolCall
	var finish string
	if err := p.Stream(context.Background(), harness.GenerateRequest{
		Messages: []harness.Message{{Role: "user", Content: "who is Kael?"}},
		Tools:    []harness.ToolSpec{{Name: "search_entities", Description: "s", Parameters: map[string]any{"type": "object"}}},
	}, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for chunk := range out {
		if len(chunk.ToolCalls) > 0 {
			calls = chunk.ToolCalls
		}
		if chunk.Done && chunk.FinishReason != "" {
			finish = chunk.FinishReason
		}
	}
	if len(calls) != 1 || calls[0].ID != "toolu_1" || calls[0].Name != "search_entities" || calls[0].Arguments != `{"query":"Kael"}` {
		t.Fatalf("calls = %+v", calls)
	}
	if finish != "tool_calls" {
		t.Errorf("finish = %q, want tool_calls", finish)
	}
}

func TestStreamErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid x-api-key"}}`))
	}))
	t.Cleanup(srv.Close)

	p := NewAnthropicProvider("gm", Options{Endpoint: srv.URL, Model: "m", APIKey: "bad"})
	out := make(chan harness.StreamChunk, 1)
	err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out)
	if err == nil || !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Fatalf("error = %v, want the provider body", err)
	}
}

func TestGenerateReportsCacheReads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, "event: message_start")
		fmt.Fprintln(w, `data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":300,"cache_creation_input_tokens":20}}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "event: content_block_delta")
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "event: message_delta")
		fmt.Fprintln(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "event: message_stop")
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))
	t.Cleanup(server.Close)

	p := NewAnthropicProvider("gm", Options{Endpoint: server.URL, Model: "m", APIKey: "k"})
	resp, err := p.Generate(context.Background(), harness.GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.CachedTokens != 300 {
		t.Errorf("cached = %d, want 300", resp.CachedTokens)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 330 {
		t.Errorf("usage = %+v, want input 330 (10 + 300 read + 20 creation)", resp.Usage)
	}
}
```

Run: `go test ./pkg/provider/anthropicllm -count=1` → stream tests FAIL with "not implemented".

- [ ] **Step 2: Implement the stream**

`pkg/provider/anthropicllm/stream.go`:

```go
package anthropicllm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// streamState is what one streamed response builds up as it is read.
type streamState struct {
	inputTokens   int
	cacheRead     int
	cacheCreation int
	outputTokens  int
	finishReason  string
	wireLines     int
	toolCalls     *toolAccumulator
}

// toolAccumulator reassembles streamed tool_use blocks, which arrive as a
// block announcement plus input JSON deltas keyed by the block index.
type toolAccumulator struct {
	order []int
	calls map[int]*harness.ToolCall
}

func newToolAccumulator() *toolAccumulator {
	return &toolAccumulator{calls: make(map[int]*harness.ToolCall)}
}

func (a *toolAccumulator) open(index int, id, name string) {
	if _, ok := a.calls[index]; !ok {
		a.calls[index] = &harness.ToolCall{ID: id, Name: name}
		a.order = append(a.order, index)
	}
}

func (a *toolAccumulator) appendArgs(index int, delta string) {
	call, ok := a.calls[index]
	if !ok {
		a.open(index, "", "")
		call = a.calls[index]
	}
	call.Arguments += delta
}

func (a *toolAccumulator) result() []harness.ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	calls := make([]harness.ToolCall, 0, len(a.order))
	for _, index := range a.order {
		calls = append(calls, *a.calls[index])
	}
	return calls
}

// finishFor maps Anthropic's stop reasons onto the harness vocabulary the
// engine checks: a turn that stops on a tool call reads as tool_calls.
func finishFor(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}

// usage sums what the turn billed: uncached input, cache writes and cache
// reads all arrive as input tokens, and the read count is what Generate
// reports as CachedTokens.
func (p *AnthropicProvider) usage(state *streamState) *harness.Usage {
	return &harness.Usage{
		Model:        p.model,
		InputTokens:  state.inputTokens + state.cacheCreation + state.cacheRead,
		OutputTokens: state.outputTokens,
	}
}

// buildError shapes a non-200 body the way the other HTTP adapters do, so the
// failure region shows the provider's own words.
func buildError(status string, body []byte) error {
	if detail := provider.TruncateDetail(body); detail != "" {
		return fmt.Errorf("anthropic http error %s: %s", status, detail)
	}
	return fmt.Errorf("anthropic http error %s", status)
}

// Generate streams the reply and collects it, so non-streaming callers share
// the stream's timeout behaviour. The cache-read count rides out on CachedTokens.
func (p *AnthropicProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	out := make(chan harness.StreamChunk, 20)
	var sb strings.Builder
	var usage *harness.Usage
	var cached int

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.streamOnce(ctx, req, out, &cached)
	}()

	for chunk := range out {
		if chunk.Error != nil {
			return nil, chunk.Error
		}
		sb.WriteString(chunk.Text)
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	if err := <-errCh; err != nil {
		return nil, err
	}
	return &harness.GenerateResponse{Text: sb.String(), Usage: usage, CachedTokens: cached}, nil
}

// Stream forwards each delta as it arrives.
func (p *AnthropicProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	var cached int
	return p.streamOnce(ctx, req, out, &cached)
}

func (p *AnthropicProvider) streamOnce(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk, cached *int) error {
	payload := p.buildRequest(req)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	p.logRequest(req)

	resp, err := p.postRequest(ctx, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return buildError(resp.Status, body)
	}

	state := &streamState{toolCalls: newToolAccumulator()}
	if err := p.consumeStream(resp.Body, state, out); err != nil {
		return err
	}
	if state.finishReason == "refusal" {
		return fmt.Errorf("anthropic: the model declined this request")
	}
	*cached = state.cacheRead
	out <- harness.StreamChunk{Done: true, FinishReason: finishFor(state.finishReason), ToolCalls: state.toolCalls.result(), Usage: p.usage(state)}
	return nil
}

// consumeStream reads Anthropic's event/data SSE frames until message_stop.
// Frames the adapter does not model are skipped rather than failing the turn,
// because the wire carries pings and progress markers alongside the content.
func (p *AnthropicProvider) consumeStream(body io.Reader, state *streamState, out chan<- harness.StreamChunk) error {
	scanner := bufio.NewScanner(body)
	var lastEvent string
	for scanner.Scan() {
		line := scanner.Text()
		if p.logger.Enabled(trace.LevelFull) && state.wireLines < p.chunkLimit() {
			state.wireLines++
			p.logger.Event("provider.wire", map[string]any{"role": p.id, "direction": "recv", "line": line})
		}
		if strings.HasPrefix(line, "event: ") {
			lastEvent = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		switch lastEvent {
		case "message_start":
			var frame struct {
				Message struct {
					Usage struct {
						InputTokens            int `json:"input_tokens"`
						CacheReadInputTokens   int `json:"cache_read_input_tokens"`
						CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err == nil {
				state.inputTokens = frame.Message.Usage.InputTokens
				state.cacheRead = frame.Message.Usage.CacheReadInputTokens
				state.cacheCreation = frame.Message.Usage.CacheCreationInputTokens
			}
		case "content_block_start":
			var frame struct {
				Index        int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err == nil && frame.ContentBlock.Type == "tool_use" {
				state.toolCalls.open(frame.Index, frame.ContentBlock.ID, frame.ContentBlock.Name)
			}
		case "content_block_delta":
			var frame struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err != nil {
				continue
			}
			switch frame.Delta.Type {
			case "text_delta":
				if frame.Delta.Text != "" {
					out <- harness.StreamChunk{Text: frame.Delta.Text}
				}
			case "input_json_delta":
				state.toolCalls.appendArgs(frame.Index, frame.Delta.PartialJSON)
			}
		case "message_delta":
			var frame struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err == nil {
				if frame.Delta.StopReason != "" {
					state.finishReason = frame.Delta.StopReason
				}
				if frame.Usage.OutputTokens > 0 {
					state.outputTokens = frame.Usage.OutputTokens
				}
			}
		case "message_stop":
			return nil
		case "error":
			var frame struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err == nil && frame.Error.Message != "" {
				return fmt.Errorf("anthropic: %s", frame.Error.Message)
			}
		}
	}
	return scanner.Err()
}
```

- [ ] **Step 3: Run tests and the backend suite**

```bash
go test ./pkg/provider/... ./pkg/harness ./pkg/engine -count=1 && go test ./pkg/provider -count=1
```

Expected: PASS (the `Validate` registry check included).

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(provider): anthropic streaming, tool blocks and cache-aware usage"
```

---

### Task 4: Presets, Pricing, Frontend, Docs, Full Gates

**Files:**
- Modify: `pkg/config/presets.go` (add the four `anthropic-*` AgentPresets; keep `claude-cli` untouched), `pkg/pricing/pricing.go` (four rows), `frontend/src/types.ts:1134` (AgentRoleConfig union + `prompt_cache` field), `frontend/src/components/providers/LLMRoleEditor.tsx` (option, endpoint block condition at ~line 100, effort select condition), `pkg/gui/docs/05-providers.md`
- Test: pricing/config tests, vitest typecheck

**Interfaces:**
- Produces: `type: anthropic` selectable in the LLM editor; the four model presets in the GUI catalogue.

- [ ] **Step 1: Presets**

Re-verify the four model IDs against `https://platform.claude.com/docs/en/models/overview` before committing (retirements rotate). Add to `AgentPresets` in `pkg/config/presets.go` (no `temperature` key anywhere: the API rejects it):

```go
	"anthropic-sonnet": {
		Type:      "anthropic",
		Model:     "claude-sonnet-5-5",
		MaxTokens: 4096,
	},
	"anthropic-haiku": {
		Type:      "anthropic",
		Model:     "claude-haiku-5-5",
		MaxTokens: 2048,
	},
	"anthropic-opus": {
		Type:      "anthropic",
		Model:     "claude-opus-5-5",
		MaxTokens: 4096,
	},
	"anthropic-fable": {
		Type:      "anthropic",
		Model:     "claude-fable-5-1",
		MaxTokens: 4096,
	},
```

- [ ] **Step 2: Pricing rows**

In `pkg/pricing/pricing.go`, after the Gemini rows. Cache reads are billed at a fraction of base input, and the current `Price` shape cannot express that, so cache-read tokens are counted at the base input rate: it over-charges rather than under-charges, and the separate-cache-pricing work belongs to the cost-guardrails bead.

```go
	// Anthropic, keyed by model. Rates captured 2026-10-24; the adapter-wide
	// row follows the shipped preset model.
	{Provider: string(provider.KeyLLMAnthropic), Model: "claude-fable-5-1", PerMillionInput: 10_000_000, PerMillionOutput: 50_000_000},
	{Provider: string(provider.KeyLLMAnthropic), Model: "claude-opus-5-5", PerMillionInput: 4_000_000, PerMillionOutput: 20_000_000},
	{Provider: string(provider.KeyLLMAnthropic), Model: "claude-sonnet-5-5", PerMillionInput: 2_000_000, PerMillionOutput: 10_000_000},
	{Provider: string(provider.KeyLLMAnthropic), Model: "claude-haiku-5-5", PerMillionInput: 100_000, PerMillionOutput: 500_000},
	{Provider: string(provider.KeyLLMAnthropic), PerMillionInput: 2_000_000, PerMillionOutput: 10_000_000},
```

- [ ] **Step 3: Frontend**

`frontend/src/types.ts` `AgentRoleConfig`:
- Union: add `'anthropic'` after `'openai'` (final order: `'builtin' | 'http' | 'cli' | 'inherit' | 'disabled' | 'gemini' | 'openai' | 'anthropic'`).
- Field (beside `effort`):

```ts
  /** Prompt caching: 'auto' (default) or 'off'; Anthropic only. */
  prompt_cache?: string;
```

`frontend/src/components/providers/LLMRoleEditor.tsx`:
- Type select: add `<option value="anthropic">Anthropic Claude (Messages API)</option>` after the openai option.
- Endpoint/model/key block condition at ~line 100: extend to `(roleConfig.type === 'http' || roleConfig.type === 'openai' || roleConfig.type === 'anthropic')`.
- Effort select (added by the OpenAI plan): extend its render condition to include `roleConfig.type === 'anthropic'` and add `<option value="max">max</option>` (Anthropic supports `xhigh` and `max`).

- [ ] **Step 4: Docs**

`pkg/gui/docs/05-providers.md`: replace the paragraph that describes Anthropic as a generic `http` target with the native provider sentence (Messages API, `providers.anthropic.api_key`, `env:` convention, prompt caching on by default, the four presets). Then regenerate:

```bash
go test ./pkg/gui -update-docs
```

- [ ] **Step 5: Full gates**

```bash
mise run test:backend:fast && mise run test:frontend && mise run lint
```

`tsc --noEmit` catches a missed union member; Oxlint/Prettier must pass on the edited editor.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(frontend): anthropic provider option, presets, pricing and docs"
```

---

## Self-Review Notes

- Spec §5.1 (registration/wiring): Task 1. §5.2 (request table, prompt cache, warn-once, thinking rule): Task 2. §5.3 (streaming frames, finish mapping, usage/cache accounting, refusal): Task 3. §5.4 (presets/pricing/frontend/docs): Task 4. §7 limitations: no thinking-block round-trip anywhere (spec non-goal), refusal is an error chunk path, `pause_turn` maps to `stop` (documented in `finishFor`).
- Cross-plan: only this plan adds `PromptCache`; `Effort` is add-if-absent per the header constraint.
