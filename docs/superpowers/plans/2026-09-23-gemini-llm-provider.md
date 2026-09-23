# Google Gemini LLM Provider (Phase 1: Text Generation & Role Routing) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Google Gemini as a first-class LLM provider in LocalRPG for high-throughput streaming text generation, tool calling, reasoning/thinking budget controls, and thought-token filtering across all engine roles.

**Architecture:** Leverage the official Google GenAI Go SDK (`google.golang.org/genai`) to implement `harness.ModelProvider`, `harness.ToolCaller`, and `trace.LogAware`. Introduce a shared provider credential hierarchy (`providers.gemini.api_key` with per-role override and env var fallback). Wire the provider into the role factory, presets, and frontend settings studio.

**Tech Stack:** Go 1.27 (`google.golang.org/genai`), React 19, TypeScript, Tailwind CSS v4.

---

## File Map

- **Configuration:**
  - Modify: `pkg/config/types.go` (Add `ProvidersConfig`, `GeminiProviderConfig` to `Config`; extend `AgentRoleConfig` with `ThinkingBudget`, `TopP`, `TopK`)
  - Test: `pkg/config/types_test.go`
  - Modify: `pkg/config/presets.go` (Add `gemini-2.5-flash`, `gemini-2.5-pro`, `gemini-2.0-flash`, `gemini-2.0-flash-lite` presets)
  - Test: `pkg/config/presets_test.go`
- **Harness & Provider:**
  - Create: `pkg/harness/gemini_provider.go` (Implements `GeminiProvider`, credential resolution, request mapping, streaming with thought filtering, tool calling, error mapping)
  - Create: `pkg/harness/gemini_provider_test.go` (Unit tests for credential resolution, thought separation, tool calling, tunables mapping, and error handling)
  - Modify: `pkg/harness/factory.go` (Wire `builtin_name == "gemini"` and `type == "gemini"` into `NewModelProvider`)
  - Modify: `pkg/harness/factory_test.go` (Verify provider creation and options passing)
- **Frontend Settings Studio:**
  - Modify: `frontend/src/types.ts` (Add `providers?: { gemini?: { api_key?: string } }` to `AppConfig`; add `thinking_budget`, `top_p`, `top_k` to `AgentRoleConfig`)
  - Modify: `frontend/src/templates/providerPresets.ts` (Add Gemini agent presets)
  - Modify: `frontend/src/components/SettingsStudio.tsx` (Add Gemini role settings controls, thinking budget selector, and global Gemini API key setting)

---

### Task 1: Configuration Schema & Presets

**Files:**
- Modify: `pkg/config/types.go:1-60` and `pkg/config/types.go:200-260`
- Test: `pkg/config/types_test.go`
- Modify: `pkg/config/presets.go:1-50`
- Test: `pkg/config/presets_test.go`

- [ ] **Step 1: Write failing tests for configuration and presets**

In `pkg/config/types_test.go`, add:
```go
func TestConfigParsesGeminiProviderAndRoleTunables(t *testing.T) {
	yamlStr := `
providers:
  gemini:
    api_key: "test-shared-gemini-key"
agents:
  default_role: "gm"
  roles:
    gm:
      type: "builtin"
      builtin_name: "gemini"
      model: "gemini-2.5-flash"
      thinking_budget: 0
      top_p: 0.95
      top_k: 40
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if cfg.Providers.Gemini.APIKey != "test-shared-gemini-key" {
		t.Errorf("expected shared api_key 'test-shared-gemini-key', got %q", cfg.Providers.Gemini.APIKey)
	}
	role := cfg.Agents.Roles["gm"]
	if role.ThinkingBudget == nil || *role.ThinkingBudget != 0 {
		t.Errorf("expected thinking_budget 0, got %v", role.ThinkingBudget)
	}
	if role.TopP == nil || *role.TopP != 0.95 {
		t.Errorf("expected top_p 0.95, got %v", role.TopP)
	}
	if role.TopK == nil || *role.TopK != 40 {
		t.Errorf("expected top_k 40, got %v", role.TopK)
	}
}
```

In `pkg/config/presets_test.go`, add:
```go
func TestGetGeminiAgentPresets(t *testing.T) {
	preset, ok := GetAgentPreset("gemini-2.5-flash")
	if !ok {
		t.Fatal("expected gemini-2.5-flash preset to exist")
	}
	if preset.BuiltinName != "gemini" || preset.Model != "gemini-2.5-flash" {
		t.Errorf("unexpected preset: %+v", preset)
	}
	if preset.ThinkingBudget == nil || *preset.ThinkingBudget != 0 {
		t.Errorf("expected thinking_budget 0 for flash, got %v", preset.ThinkingBudget)
	}

	proPreset, ok := GetAgentPreset("gemini-2.5-pro")
	if !ok {
		t.Fatal("expected gemini-2.5-pro preset to exist")
	}
	if proPreset.ThinkingBudget == nil || *proPreset.ThinkingBudget != -1 {
		t.Errorf("expected thinking_budget -1 for pro, got %v", proPreset.ThinkingBudget)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run "TestConfigParsesGeminiProviderAndRoleTunables|TestGetGeminiAgentPresets" ./pkg/config/`  
Expected: FAIL compilation errors (`ThinkingBudget undefined`, `Providers undefined`).

- [ ] **Step 3: Implement config schema and presets**

In `pkg/config/types.go`:
Add `ProvidersConfig` and `GeminiProviderConfig`:
```go
type ProvidersConfig struct {
	Gemini GeminiProviderConfig `yaml:"gemini,omitempty" json:"gemini,omitempty"`
}

type GeminiProviderConfig struct {
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}
```
Add `Providers ProvidersConfig` to `Config`:
```go
type Config struct {
	Version     string            `yaml:"version" json:"version"`
	Paths       PathsConfig       `yaml:"paths" json:"paths"`
	Providers   ProvidersConfig   `yaml:"providers,omitempty" json:"providers,omitempty"`
	Agents      AgentsConfig      `yaml:"agents" json:"agents"`
	Media       MediaConfig       `yaml:"media" json:"media"`
	Preferences PreferencesConfig `yaml:"preferences" json:"preferences"`
}
```
Extend `AgentRoleConfig`:
```go
type AgentRoleConfig struct {
	Type          string   `yaml:"type" json:"type"`
	InheritFrom   string   `yaml:"inherit_from,omitempty" json:"inherit_from,omitempty"`
	BuiltinName   string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command       string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args          []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint      string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model         string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey        string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Temperature   float64  `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens     int      `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	SupportsTools string   `yaml:"supports_tools,omitempty" json:"supports_tools,omitempty"`

	ThinkingBudget *int     `yaml:"thinking_budget,omitempty" json:"thinking_budget,omitempty"`
	TopP           *float64 `yaml:"top_p,omitempty" json:"top_p,omitempty"`
	TopK           *int     `yaml:"top_k,omitempty" json:"top_k,omitempty"`
}
```

In `pkg/config/presets.go`, add to `AgentPresets`:
```go
func intPtr(i int) *int { return &i }
func floatPtr(f float64) *float64 { return &f }

// Add inside AgentPresets map:
	"gemini-2.5-flash": {
		Type:           "builtin",
		BuiltinName:    "gemini",
		Model:          "gemini-2.5-flash",
		Temperature:    0.7,
		MaxTokens:      2048,
		ThinkingBudget: intPtr(0),
		TopP:           floatPtr(0.95),
		TopK:           intPtr(40),
	},
	"gemini-2.5-pro": {
		Type:           "builtin",
		BuiltinName:    "gemini",
		Model:          "gemini-2.5-pro",
		Temperature:    0.7,
		MaxTokens:      4096,
		ThinkingBudget: intPtr(-1),
		TopP:           floatPtr(0.95),
		TopK:           intPtr(40),
	},
	"gemini-2.0-flash": {
		Type:        "builtin",
		BuiltinName: "gemini",
		Model:       "gemini-2.0-flash",
		Temperature: 0.7,
		MaxTokens:   2048,
		TopP:        floatPtr(0.95),
		TopK:        intPtr(40),
	},
	"gemini-2.0-flash-lite": {
		Type:        "builtin",
		BuiltinName: "gemini",
		Model:       "gemini-2.0-flash-lite",
		Temperature: 0.7,
		MaxTokens:   2048,
		TopP:        floatPtr(0.95),
		TopK:        intPtr(40),
	},
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -run "TestConfigParsesGeminiProviderAndRoleTunables|TestGetGeminiAgentPresets" ./pkg/config/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/config/
git commit -m "feat(config): add Gemini provider schema, role tunables, and presets"
```

---

### Task 2: Gemini Provider Structure & Credential Resolution

**Files:**
- Create: `pkg/harness/gemini_provider.go`
- Create: `pkg/harness/gemini_provider_test.go`

- [ ] **Step 1: Write failing tests for credential resolution**

In `pkg/harness/gemini_provider_test.go`:
```go
package harness_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestResolveGeminiAPIKeyPriority(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	// 1. None provided
	key, err := harness.ResolveGeminiAPIKey("", "")
	if err == nil {
		t.Errorf("expected error when no key provided, got %q", key)
	}

	// 2. Fallback to GOOGLE_API_KEY env
	t.Setenv("GOOGLE_API_KEY", "env-google-key")
	key, err = harness.ResolveGeminiAPIKey("", "")
	if err != nil || key != "env-google-key" {
		t.Errorf("expected env-google-key, got %q (err: %v)", key, err)
	}

	// 3. GEMINI_API_KEY env overrides GOOGLE_API_KEY
	t.Setenv("GEMINI_API_KEY", "env-gemini-key")
	key, err = harness.ResolveGeminiAPIKey("", "")
	if err != nil || key != "env-gemini-key" {
		t.Errorf("expected env-gemini-key, got %q (err: %v)", key, err)
	}

	// 4. Shared provider key overrides env
	key, err = harness.ResolveGeminiAPIKey("", "shared-config-key")
	if err != nil || key != "shared-config-key" {
		t.Errorf("expected shared-config-key, got %q (err: %v)", key, err)
	}

	// 5. Role key overrides shared config key
	key, err = harness.ResolveGeminiAPIKey("role-override-key", "shared-config-key")
	if err != nil || key != "role-override-key" {
		t.Errorf("expected role-override-key, got %q (err: %v)", key, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestResolveGeminiAPIKeyPriority ./pkg/harness/`  
Expected: FAIL (`undefined: harness.ResolveGeminiAPIKey`)

- [ ] **Step 3: Implement credential resolution and provider types**

In `pkg/harness/gemini_provider.go`:
```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestResolveGeminiAPIKeyPriority ./pkg/harness/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/gemini_provider.go pkg/harness/gemini_provider_test.go
git commit -m "feat(harness): implement Gemini credential resolution and provider struct"
```

---

### Task 3: Request Mapping & Tunables (Thinking Budget, System Instruction, Tools)

**Files:**
- Modify: `pkg/harness/gemini_provider.go`
- Test: `pkg/harness/gemini_provider_test.go`

- [ ] **Step 1: Write failing test for Generate and request translation**

In `pkg/harness/gemini_provider_test.go`, add:
```go
func TestGeminiProviderGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)

		// Assert system instruction is sent
		if !strings.Contains(bodyStr, "You are the GM") {
			t.Errorf("expected body to contain system instruction, got: %s", bodyStr)
		}
		// Assert prompt text is sent
		if !strings.Contains(bodyStr, "Look around the tavern") {
			t.Errorf("expected body to contain prompt text, got: %s", bodyStr)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"candidates": [
				{
					"content": {
						"parts": [
							{"thought": true, "text": "I should describe the fire and guests."},
							{"text": "The hearth crackles with welcoming warmth."}
						],
						"role": "model"
					},
					"finishReason": "STOP"
				}
			]
		}`)
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     "test-key",
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	budget := 0
	provider, err := harness.NewGeminiProvider("test-gemini", harness.GeminiProviderOptions{
		Model:          "gemini-2.5-flash",
		APIKey:         "test-key",
		ThinkingBudget: &budget,
		Client:         client,
	})
	if err != nil {
		t.Fatalf("NewGeminiProvider: %v", err)
	}

	resp, err := provider.Generate(ctx, harness.GenerateRequest{
		System: "You are the GM",
		Prompt: "Look around the tavern",
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	// Thought must NOT be included in generated text
	if strings.Contains(resp.Text, "I should describe") {
		t.Errorf("expected thought to be filtered from text, got %q", resp.Text)
	}
	if !strings.Contains(resp.Text, "The hearth crackles") {
		t.Errorf("expected narration text, got %q", resp.Text)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestGeminiProviderGenerate ./pkg/harness/`  
Expected: FAIL (`provider.Generate undefined`)

- [ ] **Step 3: Implement request mapping, Generate(), and tool conversion**

In `pkg/harness/gemini_provider.go`:
```go
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
			contents = append(contents, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name:     msg.ToolCallID,
							Response: respMap,
						},
					},
				},
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestGeminiProviderGenerate ./pkg/harness/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/gemini_provider.go pkg/harness/gemini_provider_test.go
git commit -m "feat(harness): implement Gemini request conversion and Generate method"
```

---

### Task 4: Streaming Generation with Thought Token Filtering & Tracing

**Files:**
- Modify: `pkg/harness/gemini_provider.go`
- Test: `pkg/harness/gemini_provider_test.go`

- [ ] **Step 1: Write failing test for Stream with thought filtering and tool calling**

In `pkg/harness/gemini_provider_test.go`, add:
```go
type mockTraceSink struct {
	events []trace.Event
}

func (m *mockTraceSink) Record(e trace.Event) {
	m.events = append(m.events, e)
}

func TestGeminiProviderStreamSeparatesThoughtsAndEmitsTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Simulate streaming response chunks
		fmt.Fprint(w, `[
			{
				"candidates": [
					{
						"content": {
							"parts": [
								{"thought": true, "text": "Evaluating player action..."}
							]
						}
					}
				]
			},
			{
				"candidates": [
					{
						"content": {
							"parts": [
								{"text": "The ancient door groans open."}
							]
						}
					}
				]
			},
			{
				"candidates": [
					{
						"content": {
							"parts": [
								{
									"functionCall": {
										"id": "call-1",
										"name": "get_entity",
										"args": {"id": "iron-gate"}
									}
								}
							]
						},
						"finishReason": "STOP"
					}
				]
			}
		]`)
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     "test-key",
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	})
	if err != nil {
		t.Fatalf("genai.NewClient: %v", err)
	}

	provider, err := harness.NewGeminiProvider("test-gemini", harness.GeminiProviderOptions{
		Model:   "gemini-2.5-flash",
		APIKey:  "test-key",
		Client:  client,
	})
	if err != nil {
		t.Fatalf("NewGeminiProvider: %v", err)
	}

	sink := &mockTraceSink{}
	provider.SetLogger(trace.NewTraceLogger(sink))

	out := make(chan harness.StreamChunk, 10)
	err = provider.Stream(ctx, harness.GenerateRequest{Prompt: "open door"}, out)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var chunks []harness.StreamChunk
	for c := range out {
		chunks = append(chunks, c)
	}

	// 1. Verify thoughts are NOT in stream chunks
	for _, chunk := range chunks {
		if strings.Contains(chunk.Text, "Evaluating player action") {
			t.Errorf("thought leaked into stream chunk: %s", chunk.Text)
		}
	}

	// 2. Verify narration text is in stream chunks
	foundNarration := false
	for _, chunk := range chunks {
		if strings.Contains(chunk.Text, "The ancient door groans") {
			foundNarration = true
		}
	}
	if !foundNarration {
		t.Errorf("expected narration chunk in stream")
	}

	// 3. Verify function call is parsed
	foundTool := false
	for _, chunk := range chunks {
		if len(chunk.ToolCalls) > 0 && chunk.ToolCalls[0].Name == "get_entity" {
			foundTool = true
			if !strings.Contains(chunk.ToolCalls[0].Arguments, "iron-gate") {
				t.Errorf("unexpected tool args: %s", chunk.ToolCalls[0].Arguments)
			}
		}
	}
	if !foundTool {
		t.Errorf("expected tool call in stream")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestGeminiProviderStreamSeparatesThoughtsAndEmitsTools ./pkg/harness/`  
Expected: FAIL (`provider.Stream undefined`)

- [ ] **Step 3: Implement Stream with thought filtering and function calling**

In `pkg/harness/gemini_provider.go`:
```go
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
						g.logger.Record(trace.Event{
							Type: "gemini_thought",
							Fields: map[string]interface{}{
								"text": part.Text,
							},
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestGeminiProviderStreamSeparatesThoughtsAndEmitsTools ./pkg/harness/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/gemini_provider.go pkg/harness/gemini_provider_test.go
git commit -m "feat(harness): implement Gemini streaming with thought filtering and tool calling"
```

---

### Task 5: Factory Wiring & Error Mapping

**Files:**
- Modify: `pkg/harness/gemini_provider.go` (Add `mapGeminiError`)
- Modify: `pkg/harness/factory.go:40-120`
- Test: `pkg/harness/factory_test.go`
- Test: `pkg/harness/gemini_provider_test.go`

- [ ] **Step 1: Write failing test for factory and error mapping**

In `pkg/harness/gemini_provider_test.go`:
```go
func TestGeminiErrorMapping(t *testing.T) {
	cases := []struct {
		errStr   string
		expected string
	}{
		{"401 Unauthorized: API key invalid", "gemini: invalid API key or permission denied"},
		{"429 RESOURCE_EXHAUSTED", "gemini: quota exceeded or rate limit reached"},
		{"404 NOT_FOUND: models/unknown", "gemini: model not found"},
	}

	for _, tc := range cases {
		mapped := harness.MapGeminiErrorForTest(errors.New(tc.errStr))
		if !strings.Contains(mapped.Error(), tc.expected) {
			t.Errorf("error %q mapped to %q, want %q", tc.errStr, mapped.Error(), tc.expected)
		}
	}
}
```

In `pkg/harness/factory_test.go`:
```go
func TestNewModelProviderBuildsGemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	provider, err := harness.NewModelProvider("gm", harness.ProviderConfig{
		Type:        "builtin",
		BuiltinName: "gemini",
		Model:       "gemini-2.5-flash",
	})
	if err != nil {
		t.Fatalf("NewModelProvider failed: %v", err)
	}
	if provider.ID() != "gm" {
		t.Errorf("ID = %q, want gm", provider.ID())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGeminiErrorMapping|TestNewModelProviderBuildsGemini" ./pkg/harness/`  
Expected: FAIL (`MapGeminiErrorForTest undefined`, `unknown model provider`)

- [ ] **Step 3: Implement error mapping and factory support**

In `pkg/harness/gemini_provider.go`:
```go
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
```

In `pkg/harness/factory.go`:
Update `NewModelProvider`:
```go
	case "builtin", "mock", "":
		if cfg.BuiltinName == "narrative-oracle" {
			return NewNarrativeOracleProvider(id), nil
		}
		if cfg.BuiltinName == "gemini" {
			apiKey, err := ResolveGeminiAPIKey(cfg.APIKey, "")
			if err != nil {
				return nil, err
			}
			return NewGeminiProvider(id, GeminiProviderOptions{
				Model:          cfg.Model,
				APIKey:         apiKey,
				Temperature:    &cfg.Temperature,
				MaxTokens:      &cfg.MaxTokens,
				ThinkingBudget: cfg.ThinkingBudget,
				TopP:           cfg.TopP,
				TopK:           cfg.TopK,
			})
		}
```
And in `switch cfg.Type`:
```go
	case "gemini":
		apiKey, err := ResolveGeminiAPIKey(cfg.APIKey, "")
		if err != nil {
			return nil, err
		}
		return NewGeminiProvider(id, GeminiProviderOptions{
			Model:          cfg.Model,
			APIKey:         apiKey,
			Temperature:    &cfg.Temperature,
			MaxTokens:      &cfg.MaxTokens,
			ThinkingBudget: cfg.ThinkingBudget,
			TopP:           cfg.TopP,
			TopK:           cfg.TopK,
		})
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -run "TestGeminiErrorMapping|TestNewModelProviderBuildsGemini" ./pkg/harness/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/gemini_provider.go pkg/harness/gemini_provider_test.go pkg/harness/factory.go pkg/harness/factory_test.go
git commit -m "feat(harness): wire Gemini provider into harness factory and error mapper"
```

---

### Task 6: Frontend Settings Studio Integration

**Files:**
- Modify: `frontend/src/types.ts:275-300` and `frontend/src/types.ts:470-480`
- Modify: `frontend/src/templates/providerPresets.ts:80-95`
- Modify: `frontend/src/components/SettingsStudio.tsx:430-680`

- [ ] **Step 1: Update frontend types**

In `frontend/src/types.ts`:
Update `AgentRoleConfig`:
```typescript
export interface AgentRoleConfig {
  type: 'builtin' | 'http' | 'cli' | 'inherit' | 'disabled' | 'gemini';
  inherit_from?: string;
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  temperature?: number;
  max_tokens?: number;
  supports_tools?: 'auto' | 'yes' | 'no';
  thinking_budget?: number;
  top_p?: number;
  top_k?: number;
}
```

Add `ProvidersConfig` to `AppConfig`:
```typescript
export interface ProvidersConfig {
  gemini?: {
    api_key?: string;
  };
}

export interface AppConfig {
  version: string;
  paths: PathsConfig;
  providers?: ProvidersConfig;
  agents: AgentsConfig;
  media: MediaConfig;
  preferences: PreferencesConfig;
}
```

- [ ] **Step 2: Add Gemini presets in providerPresets.ts**

In `frontend/src/templates/providerPresets.ts`:
Add to `AGENT_PRESETS`:
```typescript
  'gemini-2.5-flash': {
    label: 'Google Gemini 2.5 Flash',
    description: 'Fast multimodal model with zero-latency thinking for responsive narration.',
    config: {
      type: 'builtin',
      builtin_name: 'gemini',
      model: 'gemini-2.5-flash',
      temperature: 0.7,
      max_tokens: 2048,
      thinking_budget: 0,
      top_p: 0.95,
      top_k: 40,
    },
  },
  'gemini-2.5-pro': {
    label: 'Google Gemini 2.5 Pro',
    description: 'Advanced reasoning model with dynamic thinking budget for complex GM logic.',
    config: {
      type: 'builtin',
      builtin_name: 'gemini',
      model: 'gemini-2.5-pro',
      temperature: 0.7,
      max_tokens: 4096,
      thinking_budget: -1,
      top_p: 0.95,
      top_k: 40,
    },
  },
```

- [ ] **Step 3: Update SettingsStudio.tsx with Gemini controls**

In `frontend/src/components/SettingsStudio.tsx`:
1. Add `gemini` to Builtin Engine options:
```tsx
<option value="gemini">Google Gemini (GenAI Cloud)</option>
```
2. When `currentRoleConfig.builtin_name === 'gemini'` or `currentRoleConfig.type === 'gemini'`:
- Render Model dropdown with presets + freeform input (`gemini-2.5-flash`, `gemini-2.5-pro`, `gemini-2.0-flash`, `gemini-2.0-flash-lite`).
- Render API Key override field with helper text indicating fallback to `config.providers?.gemini?.api_key` or `GEMINI_API_KEY`.
- Render Thinking Budget control (Disabled [0], Dynamic [-1], or custom tokens).
- Render Top P and Top K number inputs.
3. In Global Settings or Providers section:
- Add an input for `config.providers?.gemini?.api_key` so users can configure a single API key for all Gemini features.

- [ ] **Step 4: Run frontend TypeScript verification**

Run: `mise run test:frontend` (in `frontend/`, `npx tsc --noEmit`)  
Expected: PASS with 0 type errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/templates/providerPresets.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add Gemini controls, presets, and thinking budget settings"
```

---

### Task 7: Full Verification & Integration Check

**Files:**
- All touched files

- [ ] **Step 1: Run complete backend test suite**

Run: `mise run test:backend`  
Expected: PASS (all packages `./...` exit code 0)

- [ ] **Step 2: Run linter**

Run: `mise run lint`  
Expected: PASS (`go vet ./...` clean)

- [ ] **Step 3: Run complete frontend build**

Run: `mise run build:frontend`  
Expected: PASS (Vite bundles successfully into `pkg/gui/dist`)

- [ ] **Step 4: Commit any cleanup or final test adjustments**

```bash
git commit --allow-empty -m "chore(harness): verify Gemini LLM provider test and build gates"
```
