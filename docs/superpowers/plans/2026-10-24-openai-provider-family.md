# OpenAI Provider Family Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A first-class OpenAI provider family: a Responses-API chat adapter (`llm:openai`), a TTS adapter (`tts:openai`), an image adapter (`image:openai`), presets, and pricing; STT gets a preset refresh and embeddings are already native.

**Architecture:** Three new packages under `pkg/provider/` self-registering with the registry (`provider.Register` in `init()`), hand-rolled HTTP like `openaichat` (no SDK). Config selects them with `type: openai`; keys resolve through `KeyFor`/`TTSKeyFor`/`ImageKeyFor` like every other family. `llm:openaichat` and the media `http` adapters stay untouched for local OpenAI-compatible servers.

**Tech Stack:** Go (stdlib `net/http`, `encoding/json`, `bufio`), `harness.ModelProvider`/`StreamChunk`, `media.TTSClient`/`ImageClient`/`VoiceCatalog`/`VoiceOptions`, `pkg/trace`, `pkg/telemetry.HTTPTransport`, Vitest for frontend edits.

**Spec:** `docs/superpowers/specs/2026-10-24-openai-provider-design.md` (bead LocalRPG-3lz). Read the spec's mapping tables before each adapter task; the plan carries the code, the spec carries the reasons.

## Global Constraints

- Work on branch `feat/openai-provider-family`; never commit on `main`.
- Stdlib-only tests (`testing`, `httptest`, `t.TempDir()`); no testify.
- Wrap errors with `fmt.Errorf("...: %w", err)`; error bodies read through `io.LimitReader(resp.Body, provider.MaxProviderDetailBytes)` and `provider.TruncateDetail`.
- Every exported symbol gets a doc comment answering what it is for (`mise run lint:revive` must not grow).
- Every new `*http.Client` uses `&http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: ...}`.
- `gofmt`, `go vet`, staticcheck clean; no em dashes in code comments.
- Model IDs and prices must be re-verified against `<https://developers.openai.com/api/docs/models>` and the pricing page at implementation time (see spec §9); the rows below were captured 2026-10-24.
- After any change to `pkg/config/types.go` structs or presets: `go test ./pkg/gui -update-docs`.
- API keys are never defaulted or logged; presets carry no key and docs use `api_key: "env:OPENAI_API_KEY"`.

---

### Task 1: Branch, Config Fields, Registry Keys, Wiring

**Files:**
- Create: branch `feat/openai-provider-family`
- Modify: `pkg/harness/types.go:135-152` (ProviderConfig), `pkg/config/types.go` (AgentRoleConfig ~43-76, ProvidersConfig ~350-360, ImageConfig ~266-299), `pkg/provider/keys.go:6-50`, `pkg/harness/exports.go:9-35`, `pkg/harness/factory.go` (~46-58 switch, `sharedKeyFor` ~99-104, and the four ProviderConfig copy sites at ~202-216, ~309-320, ~370-390), `pkg/harness/summariser.go:~167-183`, `pkg/media/exports.go` (TTSKeyFor, ImageKeyFor, SharedProviderKey)
- Test: `pkg/harness/key_test.go`, `pkg/media/key_test.go` (existing; add cases)

**Interfaces:**
- Consumes: nothing new.
- Produces: `provider.KeyLLMOpenAI`, `provider.KeyTTSOpenAI`, `provider.KeyImageOpenAI`; `harness.ProviderConfig.Effort string`; `config.AgentRoleConfig.Effort string`; `config.ImageConfig.Quality string`; `config.ProvidersConfig.OpenAI config.OpenAIProviderConfig`; `KeyFor` case `"openai"` → `llm:openai`; `TTSKeyFor`/`ImageKeyFor` case `"openai"`; `sharedKeyFor`/`SharedProviderKey` resolve `cfg.Providers.OpenAI.APIKey` for the new keys.

- [ ] **Step 1: Branch**

```bash
git checkout -b feat/openai-provider-family
```

- [ ] **Step 2: Write the failing key tests**

Add to `pkg/harness/key_test.go` (mirroring the existing `"http"` case):

```go
func TestKeyForOpenAI(t *testing.T) {
	key, ok := harness.KeyFor(harness.ProviderConfig{Type: "openai"})
	if !ok || key != provider.KeyLLMOpenAI {
		t.Fatalf("KeyFor(openai) = %q, %v; want %q, true", key, ok, provider.KeyLLMOpenAI)
	}
	key, ok = harness.KeyFor(harness.ProviderConfig{Type: "openai", Instance: "work"})
	if !ok || string(key) != "llm:openai@work" {
		t.Fatalf("KeyFor(openai@work) = %q, %v", key, ok)
	}
}
```

Add to `pkg/media/key_test.go`:

```go
func TestTTSKeyForOpenAI(t *testing.T) {
	key, ok := media.TTSKeyFor(config.TTSConfig{Type: "openai"})
	if !ok || key != provider.KeyTTSOpenAI {
		t.Fatalf("TTSKeyFor(openai) = %q, %v", key, ok)
	}
}

func TestImageKeyForOpenAI(t *testing.T) {
	key, ok := media.ImageKeyFor(config.ImageConfig{Type: "openai"})
	if !ok || key != provider.KeyImageOpenAI {
		t.Fatalf("ImageKeyFor(openai) = %q, %v", key, ok)
	}
}
```

Run: `go test ./pkg/harness -run TestKeyForOpenAI ./pkg/media -run "TestTTSKeyForOpenAI|TestImageKeyForOpenAI" -count=1`
Expected: FAIL (`KeyLLMOpenAI` undefined).

- [ ] **Step 3: Add the keys and config fields**

In `pkg/provider/keys.go` add to the const block and `AllKeys()`:

```go
	KeyLLMOpenAI   Key = "llm:openai"
	KeyTTSOpenAI   Key = "tts:openai"
	KeyImageOpenAI Key = "image:openai"
```

In `pkg/harness/types.go` `ProviderConfig` add after `ThinkingBudget`:

```go
	// Effort is the reasoning effort a reasoning model runs at ("low",
	// "medium", "high", and provider-specific levels). Empty uses the model
	// default. Adapters that have no effort control ignore it.
	Effort string `yaml:"effort,omitempty"`
```

In `pkg/config/types.go`:

```go
// In AgentRoleConfig, after ThinkingBudget:

	// Effort is the reasoning effort passed to reasoning models on providers
	// that expose one (OpenAI, Anthropic). Empty uses the model default.
	Effort string `yaml:"effort,omitempty" json:"effort,omitempty"`

// In ImageConfig, after AspectRatio/PersonGeneration:

	// Quality selects the image model's quality tier ("low", "medium",
	// "high", "auto"). Empty means the provider default. Adapters without
	// quality tiers ignore it.
	Quality string `yaml:"quality,omitempty" json:"quality,omitempty"`

// In ProvidersConfig:

	OpenAI   OpenAIProviderConfig   `yaml:"openai,omitempty" json:"openai,omitempty"`

// New struct next to InworldProviderConfig:

// OpenAIProviderConfig is the OpenAI-specific part of the provider
// configuration.
type OpenAIProviderConfig struct {
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}
```

- [ ] **Step 4: Wire the resolvers and key resolution**

`pkg/harness/exports.go` `KeyFor`, add a case before `"builtin"`:

```go
	case "openai":
		return provider.InstanceOrSelf(provider.KeyLLMOpenAI, provider.InstanceDiscriminator(cfg.Instance, "")), true
```

`pkg/harness/factory.go` `NewModelProvider` switch: change the clause `case "cli", "http", "gemini", "inworld", "":` to include `"openai"`.

`sharedKeyFor` (factory.go:~99):

```go
func sharedKeyFor(cfg *config.Config, key provider.Key, hasKey bool) string {
	if hasKey {
		switch key.Parent() {
		case provider.KeyLLMInworld:
			return cfg.Providers.Inworld.APIKey
		case provider.KeyLLMOpenAI:
			return cfg.Providers.OpenAI.APIKey
		}
	}
	return cfg.Providers.Gemini.APIKey
}
```

All four ProviderConfig copy sites (factory.go registerConfiguredRoles/ExtractorFromConfig/CompletionFromConfig, summariser.go): add `Effort: roleCfg.Effort,` beside `ThinkingBudget: roleCfg.ThinkingBudget,`.

`pkg/media/exports.go`: add `case "openai":` to `TTSKeyFor` and `ImageKeyFor` (same shape as the `gemini` case, instance-only discrimination), and in `SharedProviderKey` add:

```go
		case string(provider.KeyTTSOpenAI), string(provider.KeyImageOpenAI):
			return cfg.Providers.OpenAI.APIKey
```

- [ ] **Step 5: Verify tests pass, run gates**

```bash
go test ./pkg/harness ./pkg/media ./pkg/config ./pkg/provider -count=1 && go test ./pkg/gui -update-docs
```

(The `-update-docs` run regenerates the config reference for the new fields.)

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat(provider): add openai family keys, effort config, and wiring"
```

---

### Task 2: `openairesponses` Adapter — Request Layer

**Files:**
- Create: `pkg/provider/openairesponses/openairesponses.go`, `pkg/provider/openairesponses/provider.go`, `pkg/provider/openairesponses/provider_test.go`
- Modify: `pkg/provider/all/all.go` (blank import)

**Interfaces:**
- Consumes: Task 1's keys/wiring; `harness.GenerateRequest`, `harness.ToolSpec`, `harness.ModelBuildPayload`.
- Produces: `openairesponses.NewResponsesProvider(id string, opts Options) *ResponsesProvider`; `Options{Endpoint, Model, APIKey string; Temperature float64; MaxTokens int; Effort string}`. Methods `ID`, `Generate`, `Stream`, `ToolCallerCapable` (true), `SetLogger`, `SetChunkLimit`. `Generate` collects `Stream` (same pattern as `openaichat.HTTPProvider.Generate`).

- [ ] **Step 1: Write the failing request-shape test**

```go
func TestResponsesRequestShape(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n")
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{
		Endpoint: server.URL, Model: "gpt-6.1-sol", APIKey: "k",
		Temperature: 0.7, MaxTokens: 512,
	})
	req := harness.GenerateRequest{
		System: "You are the GM.",
		Messages: []harness.Message{
			{Role: "user", Content: "look around"},
			{Role: "assistant", Content: "A cellar.", ToolCalls: []harness.ToolCall{{ID: "c1", Name: "roll", Arguments: `{"d":"d20"}`}}},
			{Role: "tool", ToolCallID: "c1", Content: "14"},
			{Role: "user", Content: "climb down"},
		},
		Tools:      []harness.ToolSpec{{Name: "roll", Description: "roll dice", Parameters: map[string]any{"type": "object"}}},
		ToolChoice: "required",
	}
	out := make(chan harness.StreamChunk, 16)
	if err := p.Stream(context.Background(), req, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range out {
	}

	if body["store"] != false {
		t.Errorf("store = %v, want false", body["store"])
	}
	if body["stream"] != true || body["max_output_tokens"] != 512.0 || body["temperature"] != 0.7 {
		t.Errorf("stream/max_output_tokens/temperature: %v %v %v", body["stream"], body["max_output_tokens"], body["temperature"])
	}
	if body["instructions"] != "You are the GM." {
		t.Errorf("instructions = %v", body["instructions"])
	}
	input := body["input"].([]any)
	// message(user look around), message(assistant), function_call, function_call_output, message(user climb down)
	if len(input) != 5 {
		t.Fatalf("input items = %d, want 5: %v", len(input), input)
	}
	fc := input[2].(map[string]any)
	if fc["type"] != "function_call" || fc["call_id"] != "c1" || fc["name"] != "roll" || fc["arguments"] != `{"d":"d20"}` {
		t.Errorf("function_call item = %v", fc)
	}
	outItem := input[3].(map[string]any)
	if outItem["type"] != "function_call_output" || outItem["call_id"] != "c1" {
		t.Errorf("function_call_output item = %v", outItem)
	}
	if body["tool_choice"] != "required" {
		t.Errorf("tool_choice = %v", body["tool_choice"])
	}
	tools := body["tools"].([]any)[0].(map[string]any)
	if tools["type"] != "function" || tools["name"] != "roll" || tools["strict"] != false {
		t.Errorf("tools[0] = %v", tools)
	}
}
```

Second failing test in the same file: `effort` set omits `temperature` and sends `reasoning`:

```go
func TestEffortOmitsTemperature(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{Endpoint: server.URL, Model: "gpt-6-astra", Temperature: 0.7, Effort: "high"})
	out := make(chan harness.StreamChunk, 4)
	if err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out); err != nil {
		t.Fatal(err)
	}
	for range out {
	}
	if _, sent := body["temperature"]; sent {
		t.Errorf("temperature sent alongside effort: %v", body)
	}
	if r := body["reasoning"].(map[string]any); r["effort"] != "high" {
		t.Errorf("reasoning = %v", body["reasoning"])
	}
}
```

Run: `go test ./pkg/provider/openairesponses -count=1`
Expected: FAIL (package does not exist).

- [ ] **Step 2: Implement the provider**

`pkg/provider/openairesponses/provider.go` — complete file:

```go
package openairesponses

import (
	"bytes"
	"context"
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

// Options configures one Responses-API provider. Endpoint defaults to the
// public OpenAI API; Effort, when set, switches the request to a reasoning
// model's shape (temperature is then omitted, because reasoning models reject it).
type Options struct {
	Endpoint    string
	Model       string
	APIKey      string
	Temperature float64
	MaxTokens   int
	Effort      string
}

// ResponsesProvider talks to OpenAI's Responses API. It streams every call,
// including the ones Generate collects.
type ResponsesProvider struct {
	id                 string
	endpoint           string
	model              string
	apiKey             string
	opts               Options
	client             *http.Client
	logger             trace.Logger
	chunkLimitOverride int
}

// NewResponsesProvider builds the provider for one model on one endpoint.
func NewResponsesProvider(id string, opts Options) *ResponsesProvider {
	endpoint := strings.TrimRight(opts.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	opts.Endpoint = endpoint
	return &ResponsesProvider{
		id:       id,
		endpoint: endpoint,
		model:    opts.Model,
		apiKey:   opts.APIKey,
		opts:     opts,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 5 * time.Minute},
	}
}

// ID is the configured provider id.
func (p *ResponsesProvider) ID() string { return p.id }

// ToolCallerCapable marks the adapter as able to accept tools and return calls.
func (p *ResponsesProvider) ToolCallerCapable() bool { return true }

// SetLogger attaches a trace sink. A nil logger records nothing.
func (p *ResponsesProvider) SetLogger(logger trace.Logger) { p.logger = trace.OrNil(logger) }

// SetChunkLimit bounds how many wire events one call records.
func (p *ResponsesProvider) SetChunkLimit(limit int) { p.chunkLimitOverride = limit }

func (p *ResponsesProvider) chunkLimit() int {
	if p.chunkLimitOverride <= 0 {
		return 500
	}
	return p.chunkLimitOverride
}

type responsesRequest struct {
	Model           string             `json:"model"`
	Input           []responseItem     `json:"input"`
	Instructions    string             `json:"instructions,omitempty"`
	Stream          bool               `json:"stream"`
	Store           bool               `json:"store"`
	Temperature     float64            `json:"temperature,omitempty"`
	MaxOutputTokens int                `json:"max_output_tokens,omitempty"`
	Tools           []responseTool     `json:"tools,omitempty"`
	ToolChoice      string             `json:"tool_choice,omitempty"`
	Reasoning       *responseReasoning `json:"reasoning,omitempty"`
}

type responseItem struct {
	Type      string            `json:"type"`
	Role      string            `json:"role,omitempty"`
	Content   []responseContent `json:"content,omitempty"`
	CallID    string            `json:"call_id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Arguments string            `json:"arguments,omitempty"`
	Output    string            `json:"output,omitempty"`
}

type responseContent struct {
	Type string `json:"type"` // "input_text" for user turns, "output_text" for assistant turns
	Text string `json:"text"`
}

type responseTool struct {
	Type        string         `json:"type"` // "function"
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      bool           `json:"strict"`
}

type responseReasoning struct {
	Effort string `json:"effort,omitempty"`
}

// buildInstructions is the system text: the request's System plus any
// system-role messages, which the engine sometimes folds into the conversation.
func buildInstructions(req harness.GenerateRequest) string {
	parts := []string{}
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

// buildInput maps the conversation onto Responses input items. A caller that
// set only Prompt gets one user item, matching the single-prompt path every
// other adapter serves.
func buildInput(req harness.GenerateRequest) []responseItem {
	userItem := func(text string) responseItem {
		return responseItem{Type: "message", Role: "user", Content: []responseContent{{Type: "input_text", Text: text}}}
	}
	if len(req.Messages) == 0 {
		return []responseItem{userItem(req.PromptText())}
	}
	items := make([]responseItem, 0, len(req.Messages)+1)
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			continue
		case "assistant":
			item := responseItem{Type: "message", Role: "assistant"}
			if strings.TrimSpace(m.Content) != "" {
				item.Content = []responseContent{{Type: "output_text", Text: m.Content}}
			}
			items = append(items, item)
			for _, call := range m.ToolCalls {
				items = append(items, responseItem{Type: "function_call", CallID: call.ID, Name: call.Name, Arguments: call.Arguments})
			}
		case "tool":
			if m.ToolCallID == "" {
				items = append(items, userItem(m.Content))
				continue
			}
			items = append(items, responseItem{Type: "function_call_output", CallID: m.ToolCallID, Output: m.Content})
		default:
			items = append(items, userItem(m.Content))
		}
	}
	return items
}

// buildTools maps the offered tools, or returns none when they are withdrawn.
func buildTools(req harness.GenerateRequest, allowTools bool) []responseTool {
	if !allowTools || len(req.Tools) == 0 {
		return nil
	}
	tools := make([]responseTool, 0, len(req.Tools))
	for _, spec := range req.Tools {
		tools = append(tools, responseTool{
			Type: "function", Name: spec.Name, Description: spec.Description,
			Parameters: spec.Parameters, Strict: false,
		})
	}
	return tools
}

// requestFor builds the wire payload. temperature is omitted whenever effort
// is set, because reasoning models reject the pair; allowTemperature is the
// retry ladder's escape when a model rejects temperature anyway.
func (p *ResponsesProvider) requestFor(req harness.GenerateRequest, allowTools, allowTemperature bool) responsesRequest {
	payload := responsesRequest{
		Model:        p.model,
		Input:        buildInput(req),
		Instructions: buildInstructions(req),
		Stream:       true,
		Store:        false,
		Tools:        buildTools(req, allowTools),
	}
	if maxTokens := max(req.MaxTokens, p.opts.MaxTokens); maxTokens > 0 {
		payload.MaxOutputTokens = maxTokens
	}
	if p.opts.Effort != "" {
		payload.Reasoning = &responseReasoning{Effort: p.opts.Effort}
	} else if allowTemperature {
		payload.Temperature = req.Temperature
		if payload.Temperature == 0 {
			payload.Temperature = p.opts.Temperature
		}
	}
	switch req.ToolChoice {
	case "required":
		if len(payload.Tools) > 0 {
			payload.ToolChoice = "required"
		}
	case "none":
		payload.Tools = nil
	}
	return payload
}

func (p *ResponsesProvider) postRequest(ctx context.Context, data []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/responses", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		p.logger.Event("provider.error", map[string]any{"role": p.id, "error": err.Error()})
		return nil, fmt.Errorf("http request: %w", err)
	}
	return resp, nil
}

var _ harness.ModelProvider = (*ResponsesProvider)(nil)
```

(Generate/Stream/streamOnce land in Task 3; declare them now so the file compiles:)

```go
// Generate streams the reply and collects it, so non-streaming callers share
// the stream's timeout behaviour.
func (p *ResponsesProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	out := make(chan harness.StreamChunk, 20)
	var sb strings.Builder
	var usage *harness.Usage
	var cached int

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.streamWithUsage(ctx, req, out, &cached)
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
func (p *ResponsesProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	var cached int
	return p.streamWithUsage(ctx, req, out, &cached)
}
```

- [ ] **Step 3: Register the adapter**

`pkg/provider/openairesponses/openairesponses.go`:

```go
// Package openairesponses registers the OpenAI Responses API chat provider.
package openairesponses

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyLLMOpenAI),
			Family:      provider.FamilyLLM,
			Label:       "OpenAI (Responses API)",
			Description: "OpenAI's hosted models over the Responses API: tool calling and reasoning effort included.",
			Source:      "openai",
			Tier:        provider.TierCloud,
			Features:    []provider.Feature{provider.FeatureStreaming, provider.FeatureTools, provider.FeatureKeyRequired},
			Presets: []provider.Preset{
				{ID: "openai", Order: 5, Label: "OpenAI (Cloud API)",
					Description: "Hosted GPT models over the Responses API. Needs an API key or providers.openai.api_key.",
					Config: map[string]any{
						"type": "openai", "model": "gpt-6.1-sol", "max_tokens": 4096,
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
				id = "openai"
			}
			apiKey := payload.Config.APIKey
			if apiKey == "" {
				apiKey = payload.Config.SharedAPIKey
			}
			return NewResponsesProvider(id, Options{
				Endpoint:    payload.Config.Endpoint,
				Model:       payload.Config.Model,
				APIKey:      apiKey,
				Temperature: payload.Config.Temperature,
				MaxTokens:   payload.Config.MaxTokens,
				Effort:      payload.Config.Effort,
			}), nil
		},
	})
}
```

Add `_ "github.com/darkliquid/localrpg/pkg/provider/openairesponses"` to `pkg/provider/all/all.go` (alphabetical position).

- [ ] **Step 4: Stub the stream so it compiles, run the failing tests**

Add to provider.go, temporarily returning an error so Task 2's tests still fail on the stream (Task 3 replaces it):

```go
func (p *ResponsesProvider) streamWithUsage(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk, cached *int) error {
	defer close(out)
	return fmt.Errorf("openairesponses: stream not implemented")
}
```

Run: `go build ./... && go test ./pkg/provider/openairesponses -count=1`
Expected: tests FAIL with "stream not implemented" (request shape code already written; the assertions fail at Stream).

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(provider): openai responses adapter request layer and registration"
```

---

### Task 3: `openairesponses` SSE Stream, Tool Accumulation, Retry Ladder

**Files:**
- Modify: `pkg/provider/openairesponses/provider.go` (replace `streamWithUsage` stub; add stream structs + `consumeStream`)
- Test: `pkg/provider/openairesponses/provider_test.go`

**Interfaces:**
- Consumes: Task 2's `requestFor`, `postRequest`, `ResponsesProvider`.
- Produces: `streamWithUsage` final contract: emits `StreamChunk{Text}` deltas, a final `StreamChunk{Done, FinishReason, ToolCalls, Usage}`; writes cache read count through `cached`; retry ladder `allowTools` then `allowTemperature` on 400.

- [ ] **Step 1: Write the failing stream tests**

Three tests in provider_test.go, following the SSE-fixture pattern of `pkg/provider/openaichat/http_test.go` (`httptest.NewServer` + `http.Flusher`):

```go
func TestResponsesStreamingText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Once upon \"}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"a time.\"}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":11,\"output_tokens\":4,\"input_tokens_details\":{\"cached_tokens\":7}}}}\n\n")
		flusher.Flush()
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{Endpoint: server.URL, Model: "gpt-6.1-sol"})
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
	if usage == nil || usage.InputTokens != 11 || usage.OutputTokens != 4 {
		t.Errorf("usage = %+v", usage)
	}
	resp, err := p.Generate(context.Background(), harness.GenerateRequest{Prompt: "again"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.CachedTokens != 7 {
		t.Errorf("cached = %d, want 7", resp.CachedTokens)
	}
}

func TestResponsesToolCallAccumulation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			`{"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"search_entities","arguments":""}}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"que"}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"ry\":\"Kae\"}"}`,
			`{"type":"response.completed","response":{"status":"completed"}}`,
		}
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
		}
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{Endpoint: server.URL, Model: "gpt-6.1-sol"})
	out := make(chan harness.StreamChunk, 16)
	var calls []harness.ToolCall
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
	}
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "search_entities" || calls[0].Arguments != `{"query":"Kae"}` {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestResponsesTemperatureRetry(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"Unsupported parameter: 'temperature' is not supported with this model."}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	t.Cleanup(server.Close)

	p := NewResponsesProvider("gm", Options{Endpoint: server.URL, Model: "gpt-6-astra", Temperature: 0.7})
	out := make(chan harness.StreamChunk, 4)
	if err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range out {
	}
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(bodies))
	}
	if _, sent := bodies[1]["temperature"]; sent {
		t.Errorf("retry still sent temperature: %v", bodies[1])
	}
}

func TestResponsesErrorBodyInError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model name"}}`))
	}))
	defer srv.Close()
	p := NewResponsesProvider("openai", Options{Endpoint: srv.URL, Model: "nope"})
	out := make(chan harness.StreamChunk, 1)
	err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out)
	if err == nil || !strings.Contains(err.Error(), "bad model name") {
		t.Fatalf("error = %v, want the provider body", err)
	}
}
```

Run: `go test ./pkg/provider/openairesponses -count=1` → all FAIL.

- [ ] **Step 2: Implement the stream**

Add to provider.go (replacing the stub):

```go
type streamState struct {
	finishReason string
	usage        *harness.Usage
	cached       int
	toolCalls    *toolCallAccumulator
	wireLines    int
}

// toolCallAccumulator reassembles streamed function calls, which arrive as an
// item announcement plus argument deltas keyed by the item id.
type toolCallAccumulator struct {
	order []string
	calls map[string]*harness.ToolCall
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{calls: make(map[string]*harness.ToolCall)}
}

func (a *toolCallAccumulator) open(itemID, callID, name string) {
	if _, ok := a.calls[itemID]; !ok {
		a.calls[itemID] = &harness.ToolCall{ID: callID, Name: name}
		a.order = append(a.order, itemID)
	}
}

func (a *toolCallAccumulator) appendArgs(itemID, delta string) {
	call, ok := a.calls[itemID]
	if !ok {
		a.open(itemID, "", "")
		call = a.calls[itemID]
	}
	call.Arguments += delta
}

func (a *toolCallAccumulator) result() []harness.ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	calls := make([]harness.ToolCall, 0, len(a.order))
	for _, id := range a.order {
		calls = append(calls, *a.calls[id])
	}
	return calls
}

type responseEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
	Item  struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		CallID   string `json:"call_id"`
		Name     string `json:"name"`
	} `json:"item"`
	ItemID   string `json:"item_id"`
	Response struct {
		Status string `json:"status"`
		Usage  *struct {
			InputTokens         int `json:"input_tokens"`
			OutputTokens        int `json:"output_tokens"`
			InputTokensDetails  struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (p *ResponsesProvider) streamWithUsage(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk, cached *int) error {
	defer close(out)
	return p.streamOnce(ctx, req, out, cached, true, true)
}

func (p *ResponsesProvider) streamOnce(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk, cached *int, allowTools, allowTemperature bool) error {
	payload := p.requestFor(req, allowTools, allowTemperature)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	resp, err := p.postRequest(ctx, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		detail := provider.TruncateDetail(body)
		if resp.StatusCode == http.StatusBadRequest {
			if allowTools && len(req.Tools) > 0 && strings.Contains(detail, "tools") {
				return p.streamOnce(ctx, req, out, cached, false, allowTemperature)
			}
			if allowTemperature && payload.Temperature != 0 && strings.Contains(detail, "temperature") {
				return p.streamOnce(ctx, req, out, cached, allowTools, false)
			}
		}
		if detail != "" {
			return fmt.Errorf("http error %s from %s/responses: %s", resp.Status, p.endpoint, detail)
		}
		return fmt.Errorf("http error %s from %s/responses", resp.Status, p.endpoint)
	}

	state := &streamState{toolCalls: newToolCallAccumulator()}
	if err := p.consumeStream(resp.Body, state, out); err != nil {
		return err
	}
	*cached = state.cached
	if state.finishReason == "" {
		state.finishReason = "stop"
	}
	out <- harness.StreamChunk{Done: true, FinishReason: state.finishReason, ToolCalls: state.toolCalls.result(), Usage: state.usage}
	return nil
}

// consumeStream reads the typed SSE events until response.completed. Events the
// adapter does not model are skipped rather than failing the turn, because the
// wire carries in-progress markers alongside the content.
func (p *ResponsesProvider) consumeStream(body io.Reader, state *streamState, out chan<- harness.StreamChunk) error {
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if p.logger.Enabled(trace.LevelFull) && state.wireLines < p.chunkLimit() {
			state.wireLines++
			p.logger.Event("provider.wire", map[string]any{"role": p.id, "direction": "recv", "line": line})
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev responseEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				out <- harness.StreamChunk{Text: ev.Delta}
			}
		case "response.output_item.added":
			if ev.Item.Type == "function_call" {
				state.toolCalls.open(ev.Item.ID, ev.Item.CallID, ev.Item.Name)
			}
		case "response.function_call_arguments.delta":
			state.toolCalls.appendArgs(ev.ItemID, ev.Delta)
		case "response.completed":
			if ev.Response.Usage != nil {
				state.usage = &harness.Usage{
					Model:        p.model,
					InputTokens:  ev.Response.Usage.InputTokens,
					OutputTokens: ev.Response.Usage.OutputTokens,
				}
				state.cached = ev.Response.Usage.InputTokensDetails.CachedTokens
			}
			if ev.Response.Status != "" && ev.Response.Status != "completed" {
				return fmt.Errorf("openai responses: status %q", ev.Response.Status)
			}
		case "error":
			if ev.Error != nil {
				return fmt.Errorf("openai responses: %s", ev.Error.Message)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		p.logger.Event("provider.error", map[string]any{"role": p.id, "error": err.Error()})
		return err
	}
	return nil
}
```

Add `"bufio"` and `"bytes"` (already) imports; remove nothing else.

- [ ] **Step 3: Run the tests**

Run: `go test ./pkg/provider/openairesponses -count=1 && go test ./pkg/harness ./pkg/engine -count=1`
Expected: PASS. `provider.Validate` still passes (`go test ./pkg/provider -count=1`).

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(provider): openai responses streaming, tool calls, usage and retries"
```

---

### Task 4: `ttsopenai` Adapter

**Files:**
- Create: `pkg/provider/ttsopenai/ttsopenai.go`, `pkg/provider/ttsopenai/client.go`, `pkg/provider/ttsopenai/voices.go`, `pkg/provider/ttsopenai/client_test.go`
- Modify: `pkg/provider/all/all.go`

**Interfaces:**
- Consumes: `config.TTSConfig` (incl. `Options map[string]any`), `media.TTSBuildPayload`, `media.VoiceCatalog`/`VoiceOptions` interfaces, `entity.VoiceConfig` (incl. `Options`).
- Produces: `ttsopenai.NewClient(cfg config.TTSConfig, sharedKey string) media.TTSClient`; static `ListVoices` (13 voices); `VoiceOptions` with `instructions`, `model`, `format`.

- [ ] **Step 1: Write the failing test**

```go
func TestOpenAITTSSynthesizeShape(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "audio/opus")
		_, _ = w.Write([]byte("fake-opus-audio"))
	}))
	t.Cleanup(server.Close)

	client := NewClient(config.TTSConfig{
		Type: "openai", Endpoint: server.URL, Model: "gpt-4o-mini-tts", APIKey: "k",
	}, "")
	audio, err := client.Synthesize(context.Background(), "Hello", &entity.VoiceConfig{
		VoiceID: "marin", SpeechRate: 1.1,
		Options: map[string]any{"instructions": "breathy, elderly"},
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(audio) != "fake-opus-audio" {
		t.Errorf("audio = %q", audio)
	}
	if body["model"] != "gpt-4o-mini-tts" || body["voice"] != "marin" || body["response_format"] != "opus" {
		t.Errorf("body = %v", body)
	}
	if body["instructions"] != "breathy, elderly" {
		t.Errorf("instructions = %v", body["instructions"])
	}
	if speed, ok := body["speed"].(float64); !ok || speed != 1.1 {
		t.Errorf("speed = %v", body["speed"])
	}
}

func TestOpenAITTSVoiceCatalog(t *testing.T) {
	client := NewClient(config.TTSConfig{Type: "openai"}, "")
	catalog, ok := client.(media.VoiceCatalog)
	if !ok {
		t.Fatal("client does not implement media.VoiceCatalog")
	}
	voices, err := catalog.ListVoices(context.Background())
	if err != nil || len(voices) != 13 {
		t.Fatalf("voices = %d, %v; want 13", len(voices), err)
	}
}
```

Run: `go test ./pkg/provider/ttsopenai -count=1` → FAIL.

- [ ] **Step 2: Implement**

`voices.go`: a `var voices = []media.ProviderVoice{...}` listing the 13 voice IDs (`alloy`, `ash`, `ballad`, `coral`, `echo`, `fable`, `nova`, `onyx`, `sage`, `shimmer`, `verse`, `marin`, `cedar`), `Tags: []string{"recommended"}` on `marin` and `cedar`, plus:

```go
// ListVoices returns OpenAI's fixed speech voice set. The API has no voices
// endpoint, so the catalogue is a constant list rather than a fetch.
func (c *client) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	out := make([]media.ProviderVoice, len(voices))
	copy(out, voices)
	return out, nil
}
```

`client.go`:

```go
// client calls one OpenAI speech endpoint with one model and key.
type client struct {
	endpoint      string
	model         string
	apiKey        string
	defaultVoice  string
	configOptions map[string]any
	httpClient    *http.Client
}

// NewClient builds the OpenAI speech client. The shared key covers a role that
// carries no key of its own.
func NewClient(cfg config.TTSConfig, sharedKey string) media.TTSClient {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = sharedKey
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/audio/speech"
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini-tts"
	}
	return &client{endpoint: endpoint, model: model, apiKey: apiKey,
		defaultVoice: cfg.DefaultVoice, configOptions: cfg.Options,
		httpClient: &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 60 * time.Second}}
}

func (c *client) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	voiceID := c.defaultVoice
	if voiceID == "" {
		voiceID = "alloy"
	}
	model := c.model
	var instructions string
	if voice != nil {
		if voice.VoiceID != "" {
			voiceID = voice.VoiceID
		}
		if v, ok := voice.Options["model"].(string); ok && v != "" {
			model = v
		}
		if v, ok := voice.Options["instructions"].(string); ok {
			instructions = v
		}
	}
	if instructions == "" {
		if v, ok := c.configOptions["instructions"].(string); ok {
			instructions = v
		}
	}
	payload := map[string]any{
		"model": model, "input": text, "voice": voiceID, "response_format": "opus",
	}
	if instructions != "" {
		payload["instructions"] = instructions
	}
	if voice != nil && voice.SpeechRate > 0 {
		payload["speed"] = voice.SpeechRate
	}
	body, _ := json.Marshal(payload)
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
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, fmt.Errorf("openai tts failed (%d): %s", resp.StatusCode, provider.TruncateDetail(b))
	}
	return io.ReadAll(resp.Body)
}

// VoiceOptions declares the provider's tunables: the model alias and the free
// text instructions that steer tone. Pitch is not offered: the API has none.
func (c *client) VoiceOptions() []media.VoiceOption {
	return []media.VoiceOption{
		{Key: "instructions", Label: "Delivery instructions", Kind: "string",
			Help: "Free text tone and accent steering, e.g. 'breathy, elderly, faintly amused'."},
		{Key: "model", Label: "Model", Kind: "enum", Options: []string{"gpt-4o-mini-tts"}, Default: "gpt-4o-mini-tts"},
	}
}
```

`ttsopenai.go` registration: key `tts:openai`, Family TTS, Tier Cloud, Features `{FeatureKeyRequired, FeatureVoiceCatalog, FeatureVoiceOptions, FeatureMetered}`, Caveat `"The /v1/audio/speech endpoint is deprecated by OpenAI with shutdown 2027-01-06; see the provider spec."`, preset:

```go
{ID: "openai", Order: 5, Label: "OpenAI Speech (Cloud API)",
	Description: "Cloud speech with a fixed 13-voice catalogue and delivery instructions. Metered per character.",
	Config: map[string]any{
		"type": "openai", "model": "gpt-4o-mini-tts", "default_voice": "marin",
		"pitch": 1.0, "speech_rate": 1.0, "auto_play": true, "master_volume": 1.0,
	}},
```

Build unmarshals `media.TTSBuildPayload` and calls `NewClient(payload.Config, payload.SharedKey)`. Blank-import in `all.go`.

- [ ] **Step 3: Verify**

Run: `go test ./pkg/provider/ttsopenai ./pkg/media -count=1` → PASS.

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(provider): openai tts adapter with voice catalogue and instructions"
```

---

### Task 5: `imageopenai` Adapter

**Files:**
- Create: `pkg/provider/imageopenai/imageopenai.go`, `pkg/provider/imageopenai/client.go`, `pkg/provider/imageopenai/client_test.go`
- Modify: `pkg/provider/all/all.go`

**Interfaces:**
- Consumes: `config.ImageConfig` (incl. Task 1's `Quality`), `media.ImageBuildPayload`, `media.ImageClient` (`GenerateImage(ctx, prompt) ([]byte, error)`), `media.Usage`.
- Produces: `imageopenai.NewClient(cfg config.ImageConfig, sharedKey string) media.ImageClient` with `LastUsage() media.Usage`.

- [ ] **Step 1: Write the failing test**

```go
func TestOpenAIImageShape(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString([]byte("\x89PNG-fake")))
	}))
	t.Cleanup(server.Close)

	client := NewClient(config.ImageConfig{
		Type: "openai", Endpoint: server.URL, Model: "gpt-image-2.5-flare",
		APIKey: "k", AspectRatio: "3:2", Quality: "high",
	}, "")
	img, err := client.GenerateImage(context.Background(), "a windswept moor")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(img) != "\x89PNG-fake" {
		t.Errorf("image = %q", img)
	}
	if _, sent := body["response_format"]; sent {
		t.Errorf("response_format sent; gpt-image models reject it: %v", body)
	}
	if body["size"] != "1536x1024" || body["quality"] != "high" || body["model"] != "gpt-image-2.5-flare" || body["n"] != 1.0 {
		t.Errorf("body = %v", body)
	}
}
```

Also a table test for `sizeFor`: `"1:1"`→`1024x1024`, `"3:2"`/`"16:9"`→`1536x1024`, `"2:3"`→`1024x1536`, `""` and unknown → `1024x1024`.

Run: `go test ./pkg/provider/imageopenai -count=1` → FAIL.

- [ ] **Step 2: Implement**

`client.go`:

```go
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

// NewClient builds the OpenAI image client for one model and endpoint.
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
	return &client{endpoint: endpoint, model: model, apiKey: apiKey,
		quality: cfg.Quality, aspectRatio: cfg.AspectRatio,
		httpClient: &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 120 * time.Second}}
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

func (c *client) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	c.setLastUsage(media.Usage{Requests: 1, Estimated: true})
	payload := map[string]any{
		"model": c.model, "prompt": prompt, "n": 1,
		"size": sizeFor(c.aspectRatio), "output_format": "png",
	}
	if c.quality != "" && c.quality != "auto" {
		payload["quality"] = c.quality
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp2, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai image request: %w", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(io.LimitReader(resp2.Body, provider.MaxProviderDetailBytes))
		return nil, fmt.Errorf("openai image failed (%d): %s", resp2.StatusCode, provider.TruncateDetail(rb))
	}
	respBody, err := io.ReadAll(resp2.Body)
	if err != nil {
		return nil, fmt.Errorf("openai image body: %w", err)
	}
	var respData struct {
		Data []struct {
			B64 string `json:"b64_json"`
			URL  string `json:"url"`
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
```

(The POST helper mirrors `imagehttp.postImageRequest`; the URL branch of the response is not fetched, unlike `imagehttp`: the adapter requests `output_format` so b64 is the only shape it accepts, and a URL response is the error above.)

`imageopenai.go` registration: key `image:openai`, Family Image, Tier Cloud, Features `{FeatureKeyRequired, FeatureMetered, FeatureAutoGenerate}`, preset:

```go
{ID: "openai", Order: 5, Label: "OpenAI Images (Cloud API)",
	Description: "gpt-image generation with size and quality mapping. Metered per image.",
	Config: map[string]any{
		"type": "openai", "model": "gpt-image-2.5-flare", "aspect_ratio": "3:2",
		"quality": "auto", "auto_generate": false,
	}},
```

Blank-import in `all.go`.

- [ ] **Step 3: Verify and commit**

Run: `go test ./pkg/provider/imageopenai ./pkg/media -count=1` → PASS.

```bash
git add -A && git commit -m "feat(provider): openai image adapter with size and quality mapping"
```

---

### Task 6: Presets, Stale Preset Refresh, Pricing

**Files:**
- Modify: `pkg/config/presets.go`, `pkg/provider/ttshttp/ttshttp.go` (preset model), `pkg/provider/sttwhisperhttp/sttwhisperhttp.go` (preset model), `pkg/provider/imagehttp/imagehttp.go` (drop the dall-e-3 preset), `pkg/pricing/pricing.go`
- Test: `pkg/pricing` tests (existing table tests), `pkg/config` preset tests

**Interfaces:**
- Consumes: Tasks 1-5.
- Produces: refreshed presets that run against the live API; `BuiltinPrices` rows for the new keys.

- [ ] **Step 1: Re-verify prices, then write the pricing rows**

Read `<https://developers.openai.com/api/docs/models>` for the current per-model rates and adjust before editing. In `pkg/pricing/pricing.go`, after the existing OpenAI chat rows:

```go
	// OpenAI Responses chat, keyed by model. The adapter-wide row follows the
	// shipped preset model, as with Gemini. Rates captured 2026-10-24.
	{Provider: string(provider.KeyLLMOpenAI), Model: "gpt-6-astra", PerMillionInput: 10_000_000, PerMillionOutput: 50_000_000},
	{Provider: string(provider.KeyLLMOpenAI), Model: "gpt-6.1-sol", PerMillionInput: 2_000_000, PerMillionOutput: 10_000_000},
	{Provider: string(provider.KeyLLMOpenAI), Model: "gpt-6-luna", PerMillionInput: 100_000, PerMillionOutput: 500_000},
	{Provider: string(provider.KeyLLMOpenAI), PerMillionInput: 2_000_000, PerMillionOutput: 10_000_000},
	{Provider: string(provider.KeyTTSOpenAI), PerCharacter: 15},
```

- [ ] **Step 2: Refresh the stale presets**

`pkg/config/presets.go`:
- `openai-speech`: `Model: "tts-1"` → `Model: "gpt-4o-mini-tts"`.
- `openai-whisper`: `Model: "whisper-1"` → `Model: "gpt-transcribe"`.
- `dall-e-3`: delete the entry (the dall-e models are shut down, and `imagehttp` sends `response_format`, which gpt-image models reject; the replacement is the `image:openai` preset from Task 5). Add a new entry:

```go
	"openai-image": {
		Type:  "openai",
		Model: "gpt-image-2.5-flare",
	},
```

Add LLM agent preset:

```go
	"openai": {
		Type:      "openai",
		Model:     "gpt-6.1-sol",
		MaxTokens: 4096,
	},
```

`pkg/provider/ttshttp/ttshttp.go` `openai-speech` preset: `"model": "tts-1"` → `"model": "gpt-4o-mini-tts"`.
`pkg/provider/sttwhisperhttp/sttwhisperhttp.go` `openai-whisper` preset: `"model": "whisper-1"` → `"model": "gpt-transcribe"`.
`pkg/provider/imagehttp/imagehttp.go`: delete the `dall-e-3` preset entry.

- [ ] **Step 3: Grep for tests pinning the old models, update, run**

```bash
grep -rn "dall-e-3\|tts-1\b\|openai-whisper\|openai-speech" --include="*.go" pkg/ | grep -i test
```

Update any assertions to the new models/presets. Then:

```bash
go test ./pkg/pricing ./pkg/config ./pkg/provider/... -count=1 && go test ./pkg/gui -update-docs
```

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(provider): openai presets, refreshed stale model ids, and pricing rows"
```

---

### Task 7: Frontend, Docs, Final Gates

**Files:**
- Modify: `frontend/src/types.ts:1134` (AgentRoleConfig union + fields), `frontend/src/types.ts:1295` (TTSConfig union), `frontend/src/types.ts:1336` (ImageConfig union), `frontend/src/components/providers/LLMRoleEditor.tsx` (type select ~53-73, http block condition ~100, effort field), `frontend/src/components/providers/TTSProviderEditor.tsx:~245-254`, `frontend/src/components/providers/ImageProviderEditor.tsx:~105-114`, `pkg/gui/docs/05-providers.md` (prose)
- Test: `frontend/src/test/` (typecheck + a vitest for the editor options if one exists for the other types)

**Interfaces:**
- Consumes: all previous tasks.
- Produces: `type: 'openai'` selectable in the three editors; `effort` editable for openai roles.

- [ ] **Step 1: types.ts**

```ts
// AgentRoleConfig.type gains 'openai' and 'anthropic' is NOT added here (sibling plan):
  type: 'builtin' | 'http' | 'cli' | 'inherit' | 'disabled' | 'gemini' | 'openai';
// plus:
  /** Reasoning effort for reasoning models (openai). Empty means the model default. */
  effort?: string;
// TTSConfig.type gains: | 'openai'
// ImageConfig.type gains: | 'openai'
// ImageConfig gains:  quality?: string;
```

- [ ] **Step 2: Editors**

`LLMRoleEditor.tsx`:
- Add `<option value="openai">OpenAI (Responses API)</option>` after the gemini option.
- Change the http-only block condition at ~line 100 from `roleConfig.type === 'http'` to `(roleConfig.type === 'http' || roleConfig.type === 'openai')`.
- After the gemini thinking block (~line 235-...), add an effort select rendered for `roleConfig.type === 'openai'`:

```tsx
{roleConfig.type === 'openai' && (
  <div className="space-y-1.5">
    <label className="text-xs font-sans uppercase text-stone-300">Reasoning Effort</label>
    <select
      value={roleConfig.effort || ''}
      onChange={(e) => {
        const updated = { ...roleConfig, effort: e.target.value || undefined };
        setConfig({
          ...config,
          agents: { ...config.agents, roles: { ...config.agents.roles, [role]: updated } },
        });
      }}
      className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
    >
      <option value="">Model default</option>
      <option value="low">low</option>
      <option value="medium">medium</option>
      <option value="high">high</option>
      <option value="xhigh">xhigh</option>
    </select>
  </div>
)}
```

`TTSProviderEditor.tsx`: add `<option value="openai">OpenAI Speech (Cloud, metered)</option>` after the gemini option.
`ImageProviderEditor.tsx`: add `<option value="openai">OpenAI Images (Cloud, metered)</option>` after the gemini option.

- [ ] **Step 3: Docs**

`pkg/gui/docs/05-providers.md`: extend the provider section with a short OpenAI paragraph (Responses API chat, the 13-voice speech catalogue with `instructions`, gpt-image generation, `providers.openai.api_key` shared key, `env:` convention). Then regenerate generated docs:

```bash
go test ./pkg/gui -update-docs
```

- [ ] **Step 4: Full gates**

```bash
mise run test:backend:fast && mise run test:frontend && mise run lint
```

Oxlint/Prettier must pass on the edited editors; `tsc --noEmit` catches a missed union member.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(frontend): openai provider options, effort control, and provider docs"
```

---

## Self-Review Notes

- Spec §4.1 (family layout): Tasks 1-5. §4.2 (LLM adapter): Tasks 2-3. §4.3 (TTS): Task 4. §4.4 (image): Task 5. §4.5 (shared key): Task 1 step 4 + adapter constructors. §4.6 (catalogue/presets/pricing/GUI): Tasks 6-7. §3 non-goals: no SIWC, no Realtime, no edits API anywhere in the plan.
- Cross-plan field: `harness.ProviderConfig.Effort` is added here; the Anthropic plan's Task 1 adds it too with an add-if-absent note. Whichever lands first wins; the second plan skips the struct edit but keeps everything else.
