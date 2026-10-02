# Inworld AI Provider Ecosystem Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate Inworld AI into LocalRPG across LLM routing, Text-to-Speech (TTS), and Speech-to-Text (STT) with unified credentials (`providers.inworld.api_key`), dynamic voice catalogs, and full offline test coverage.

**Architecture:** Implement three dedicated provider packages in `pkg/provider/`: `inworldllm` (OpenAI-compatible streaming router at `https://api.inworld.ai/v1/chat/completions`), `inworldtts` (neural speech at `https://api.inworld.ai/tts/v1/voice` using `inworld-tts-2` and `Ashley`/`Dennis`), and `inworldstt` (audio transcription at `https://api.inworld.ai/stt/v1/transcribe` using `inworld/inworld-stt-1`). Introduce shared credential resolution (`providers.inworld.api_key`, `INWORLD_API_KEY`, per-service overrides) and wire the new providers into LocalRPG's provider registry, facades, embedded documentation, and frontend Settings Studio.

**Tech Stack:** Go 1.27 (`net/http`, `httptest`, `encoding/json`), TypeScript, React 19, Tailwind CSS v4.

---

## File Map

- **Configuration & Provider Keys:**
  - Modify: `pkg/config/types.go` (Add `Inworld InworldProviderConfig` to `ProvidersConfig`)
  - Test: `pkg/config/types_test.go` (Verify unmarshaling `providers.inworld.api_key`)
  - Modify: `pkg/provider/keys.go` (Add `KeyLLMInworld`, `KeyTTSInworld`, `KeySTTInworld` to constants and `AllKeys()`)
  - Test: `pkg/provider/key_test.go` (Verify key validation)
- **Inworld LLM Router Provider:**
  - Create: `pkg/provider/inworldllm/client.go` (Implements `harness.ModelProvider`, `harness.ToolCaller`, `trace.LogAware`, `media.MeteredProvider`, `Basic` auth)
  - Create: `pkg/provider/inworldllm/inworldllm.go` (Registers `llm:inworld`, descriptor, preset `inworld-frontier`)
  - Create: `pkg/provider/inworldllm/client_test.go` (Offline tests: SSE streaming, tool calling, auth header, key precedence)
  - Modify: `pkg/provider/all/all.go` (Blank import `inworldllm`)
  - Modify: `pkg/harness/exports.go` (`KeyFor`: map `inworld` to `provider.KeyLLMInworld`)
  - Modify: `pkg/harness/factory.go` (Pass `SharedKey` from `cfg.Providers.Inworld.APIKey`)
  - Test: `pkg/harness/key_test.go` (Verify `KeyFor` maps Inworld LLM)
- **Inworld TTS Provider:**
  - Create: `pkg/provider/inworldtts/client.go` (Implements `media.TTSClient`, `media.VoiceCatalog`, `media.MeteredProvider`, base64 MP3 decode, offline curated voices)
  - Create: `pkg/provider/inworldtts/inworldtts.go` (Registers `tts:inworld`, descriptor, preset `inworld-tts`)
  - Create: `pkg/provider/inworldtts/client_test.go` (Offline tests: synthesis, audio decode, auth header, voice catalog)
  - Modify: `pkg/provider/all/all.go` (Blank import `inworldtts`)
  - Modify: `pkg/media/exports.go` (`TTSKeyFor`: map `inworld` to `provider.KeyTTSInworld`)
  - Modify: `pkg/gui/service.go` (Pass Inworld shared key when building TTS client)
  - Test: `pkg/media/key_test.go` (Verify `TTSKeyFor` maps Inworld TTS)
- **Inworld STT Provider:**
  - Modify: `pkg/media/exports.go` (Define `STTBuildPayload`, update `BuildSTT` to accept shared key)
  - Modify: `pkg/media/providers.go` (Add `NewSTTClientWithSharedKey`)
  - Create: `pkg/provider/inworldstt/client.go` (Implements `media.STTClient`, `media.MeteredProvider`, WAV/LINEAR16/OGG_OPUS audio encoding adaptation)
  - Create: `pkg/provider/inworldstt/inworldstt.go` (Registers `stt:inworld`, descriptor, preset `inworld-stt`)
  - Create: `pkg/provider/inworldstt/client_test.go` (Offline tests: transcription, audio adaptation, auth header, key precedence)
  - Modify: `pkg/provider/all/all.go` (Blank import `inworldstt`)
  - Modify: `pkg/media/exports.go` (`STTKeyFor`: map `inworld` to `provider.KeySTTInworld`)
  - Modify: `pkg/gui/service.go` (Pass Inworld shared key to `NewSTTClientWithSharedKey` for transcription and test)
  - Test: `pkg/media/key_test.go` (Verify `STTKeyFor` maps Inworld STT)
- **Registry Validation & Embedded Documentation:**
  - Modify: `pkg/gui/docs_catalogue_test.go` (Update docs with `-update-docs`)
- **Frontend Settings Studio:**
  - Modify: `frontend/src/types.ts` (Add `inworld?: InworldProviderConfig` to `ProvidersConfig`)
  - Modify: `frontend/src/components/SettingsStudio.tsx` (Inworld credential card in Providers tab, key hint placeholders)

---

### Task 1: Configuration Schema & Provider Keys

**Files:**
- Modify: `pkg/config/types.go:238-265`
- Test: `pkg/config/types_test.go`
- Modify: `pkg/provider/keys.go:1-43`
- Test: `pkg/provider/key_test.go`

- [ ] **Step 1: Write failing test for Inworld config parsing**

In `pkg/config/types_test.go`, add:
```go
func TestConfigParsesInworldProvider(t *testing.T) {
	yamlStr := `
providers:
  inworld:
    api_key: "test-inworld-key-123"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := cfg.Providers.Inworld.APIKey; got != "test-inworld-key-123" {
		t.Errorf("cfg.Providers.Inworld.APIKey = %q, want test-inworld-key-123", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestConfigParsesInworldProvider ./pkg/config/`
Expected: FAIL with `cfg.Providers.Inworld undefined`

- [ ] **Step 3: Implement InworldProviderConfig in `pkg/config/types.go`**

In `pkg/config/types.go`, add `Inworld InworldProviderConfig` to `ProvidersConfig` and declare the type:
```go
// ProvidersConfig groups shared credentials and defaults for external ecosystem providers.
type ProvidersConfig struct {
	Gemini   GeminiProviderConfig  `yaml:"gemini,omitempty" json:"gemini,omitempty"`
	Inworld  InworldProviderConfig `yaml:"inworld,omitempty" json:"inworld,omitempty"`
	Currency string                `yaml:"currency,omitempty" json:"currency,omitempty"`
	Prices   []PriceConfig         `yaml:"prices,omitempty" json:"prices,omitempty"`
}

type InworldProviderConfig struct {
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}
```

- [ ] **Step 4: Add Inworld canonical keys to `pkg/provider/keys.go`**

In `pkg/provider/keys.go`:
```go
const (
	KeyLLMOpenAIChat      Key = "llm:openaichat"
	KeyLLMGemini          Key = "llm:gemini"
	KeyLLMCLI             Key = "llm:cli"
	KeyLLMNarrativeOracle Key = "llm:narrative-oracle"
	KeyLLMInworld         Key = "llm:inworld"

	KeyTTSGemini     Key = "tts:gemini"
	KeyTTSElevenLabs Key = "tts:elevenlabs"
	KeyTTSNativeOS   Key = "tts:native-os"
	KeyTTSSherpaONNX Key = "tts:sherpa-onnx"
	KeyTTSPiper      Key = "tts:piper"
	KeyTTSHTTP       Key = "tts:http"
	KeyTTSInworld    Key = "tts:inworld"

	KeySTTWhisperHTTP Key = "stt:whisper-http"
	KeySTTWhisperCLI  Key = "stt:whisper-cli"
	KeySTTWebSpeech   Key = "stt:web-speech"
	KeySTTInworld     Key = "stt:inworld"
...
```
And add `KeyLLMInworld`, `KeyTTSInworld`, `KeySTTInworld` to `AllKeys()`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -v -run 'TestConfigParsesInworldProvider' ./pkg/config/ && go test -v ./pkg/provider/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go pkg/provider/keys.go
git commit -m "feat(config): add inworld provider config and canonical keys"
```

---

### Task 2: Inworld LLM Router Provider Package (`pkg/provider/inworldllm`)

**Files:**
- Create: `pkg/provider/inworldllm/client.go`
- Create: `pkg/provider/inworldllm/inworldllm.go`
- Create: `pkg/provider/inworldllm/client_test.go`
- Modify: `pkg/provider/all/all.go`
- Modify: `pkg/harness/exports.go:9-30`
- Modify: `pkg/harness/factory.go:70-80`
- Modify: `pkg/harness/key_test.go`

- [ ] **Step 1: Write failing tests for Inworld LLM client**

Create `pkg/provider/inworldllm/client_test.go`:
```go
package inworldllm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestInworldLLM_PrecedenceAndAuthHeader(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Narrative response from Inworld.",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     12,
				"completion_tokens": 8,
			},
		})
	}))
	defer server.Close()

	// 1. Role key takes highest precedence
	client, err := NewInworldLLMClient(config.AgentRoleConfig{
		Endpoint: server.URL,
		APIKey:   "role-key",
	}, "shared-key")
	if err != nil {
		t.Fatalf("NewInworldLLMClient: %v", err)
	}

	resp, err := client.Generate(context.Background(), harness.GenerateRequest{
		Messages: []harness.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "Narrative response from Inworld." {
		t.Errorf("resp.Text = %q", resp.Text)
	}
	if gotAuth != "Basic role-key" {
		t.Errorf("gotAuth = %q, want Basic role-key", gotAuth)
	}

	// 2. Shared key fallback
	client2, err := NewInworldLLMClient(config.AgentRoleConfig{
		Endpoint: server.URL,
	}, "shared-key-2")
	if err != nil {
		t.Fatalf("NewInworldLLMClient: %v", err)
	}
	_, _ = client2.Generate(context.Background(), harness.GenerateRequest{Prompt: "test"})
	if gotAuth != "Basic shared-key-2" {
		t.Errorf("gotAuth = %q, want Basic shared-key-2", gotAuth)
	}

	// 3. Environment variable fallback
	os.Setenv("INWORLD_API_KEY", "env-key")
	defer os.Unsetenv("INWORLD_API_KEY")
	client3, err := NewInworldLLMClient(config.AgentRoleConfig{
		Endpoint: server.URL,
	}, "")
	if err != nil {
		t.Fatalf("NewInworldLLMClient: %v", err)
	}
	_, _ = client3.Generate(context.Background(), harness.GenerateRequest{Prompt: "test"})
	if gotAuth != "Basic env-key" {
		t.Errorf("gotAuth = %q, want Basic env-key", gotAuth)
	}
}

func TestInworldLLM_MissingKeyError(t *testing.T) {
	os.Unsetenv("INWORLD_API_KEY")
	_, err := NewInworldLLMClient(config.AgentRoleConfig{}, "")
	if err == nil || !strings.Contains(err.Error(), "inworld: an API key is required") {
		t.Fatalf("expected missing key error, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/provider/inworldllm/`
Expected: FAIL with package not found / compilation error

- [ ] **Step 3: Implement `pkg/provider/inworldllm/client.go`**

Create `pkg/provider/inworldllm/client.go`:
```go
package inworldllm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

var ErrMissingAPIKey = errors.New("inworld: an API key is required; set providers.inworld.api_key, agents.roles.<role>.api_key, or INWORLD_API_KEY")

const (
	defaultEndpoint = "https://api.inworld.ai/v1/chat/completions"
	defaultModel    = "inworld/compare-frontier-models"
	requestTimeout  = 120 * time.Second
)

type InworldLLMClient struct {
	id          string
	endpoint    string
	model       string
	apiKey      string
	temperature float64
	maxTokens   int
	client      *http.Client
	logger      trace.Logger

	usageMu   sync.Mutex
	lastUsage media.Usage
}

func NewInworldLLMClient(cfg config.AgentRoleConfig, sharedKey string) (*InworldLLMClient, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(sharedKey)
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("INWORLD_API_KEY"))
	}
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}

	trace.RegisterSecret(apiKey)

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	} else {
		if !strings.HasSuffix(endpoint, "/chat/completions") {
			endpoint = strings.TrimRight(endpoint, "/") + "/chat/completions"
		}
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}

	return &InworldLLMClient{
		id:          "inworld",
		endpoint:    endpoint,
		model:       model,
		apiKey:      apiKey,
		temperature: cfg.Temperature,
		maxTokens:   cfg.MaxTokens,
		client:      &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: requestTimeout},
	}, nil
}

func (c *InworldLLMClient) ID() string { return c.id }

func (c *InworldLLMClient) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

func (c *InworldLLMClient) ToolCallerCapable() bool { return true }

func (c *InworldLLMClient) Metered() bool { return true }

func (c *InworldLLMClient) LastUsage() media.Usage {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return c.lastUsage
}

func (c *InworldLLMClient) setUsage(promptTokens, completionTokens int) {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	c.lastUsage = media.Usage{
		Requests:     1,
		InputTokens:  promptTokens,
		OutputTokens: completionTokens,
	}
}

type openAIChatMessage struct {
	Role       string             `json:"role"`
	Content    string             `json:"content,omitempty"`
	ToolCalls  []harness.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float64             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Stream      bool                `json:"stream"`
	Tools       []openAITool        `json:"tools,omitempty"`
}

type openAITool struct {
	Type     string           `json:"type"`
	Function openAIFunction   `json:"function"`
}

type openAIFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Role      string             `json:"role"`
			Content   string             `json:"content"`
			ToolCalls []harness.ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *InworldLLMClient) buildRequest(req harness.GenerateRequest, stream bool) openAIChatRequest {
	var msgs []openAIChatMessage
	if req.System != "" {
		msgs = append(msgs, openAIChatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		role := m.Role
		if role == "tool" {
			msgs = append(msgs, openAIChatMessage{Role: "tool", Content: m.Content, ToolCallID: m.ToolCallID})
		} else {
			msgs = append(msgs, openAIChatMessage{Role: role, Content: m.Content, ToolCalls: m.ToolCalls})
		}
	}
	if len(msgs) == 0 && req.Prompt != "" {
		msgs = append(msgs, openAIChatMessage{Role: "user", Content: req.Prompt})
	}

	temp := c.temperature
	if req.Temperature > 0 {
		temp = req.Temperature
	}
	maxTokens := c.maxTokens
	if req.MaxTokens > 0 {
		maxTokens = req.MaxTokens
	}

	var tools []openAITool
	for _, t := range req.Tools {
		tools = append(tools, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	return openAIChatRequest{
		Model:       c.model,
		Messages:    msgs,
		Temperature: temp,
		MaxTokens:   maxTokens,
		Stream:      stream,
		Tools:       tools,
	}
}

func (c *InworldLLMClient) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	payload := c.buildRequest(req, false)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Basic "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("inworld llm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, c.mapError(resp.StatusCode, body)
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode inworld response: %w", err)
	}

	c.setUsage(chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens)

	if len(chatResp.Choices) == 0 {
		return &harness.GenerateResponse{}, nil
	}

	choice := chatResp.Choices[0]
	return &harness.GenerateResponse{
		Text:         choice.Message.Content,
		ToolCalls:    choice.Message.ToolCalls,
		FinishReason: choice.FinishReason,
	}, nil
}

func (c *InworldLLMClient) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	payload := c.buildRequest(req, true)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Basic "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("inworld llm stream request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return c.mapError(resp.StatusCode, body)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := strings.TrimPrefix(line, "data: ")
		if strings.TrimSpace(dataStr) == "[DONE]" {
			out <- harness.StreamChunk{Done: true}
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string             `json:"content"`
					ToolCalls []harness.ToolCall `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil {
			c.setUsage(chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens)
		}

		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			out <- harness.StreamChunk{
				Text:         choice.Delta.Content,
				ToolCalls:    choice.Delta.ToolCalls,
				FinishReason: choice.FinishReason,
			}
		}
	}

	return scanner.Err()
}

func (c *InworldLLMClient) mapError(status int, body []byte) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("inworld: invalid API key or permission denied; check providers.inworld.api_key or INWORLD_API_KEY")
	case http.StatusTooManyRequests:
		return errors.New("inworld: quota exceeded or rate limit reached; check your Inworld account credits")
	default:
		detail := provider.TruncateDetail(body)
		if detail != "" {
			return fmt.Errorf("inworld: %s (status %d)", detail, status)
		}
		return fmt.Errorf("inworld: request failed with status %d", status)
	}
}
```

- [ ] **Step 4: Implement `pkg/provider/inworldllm/inworldllm.go`**

Create `pkg/provider/inworldllm/inworldllm.go`:
```go
// Package inworldllm registers the Inworld AI LLM router provider.
package inworldllm

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyLLMInworld),
			Family:      provider.FamilyLLM,
			Label:       "Inworld LLM Router",
			Description: "Gateway routing prompts across frontier models with automatic fallbacks and tool calling.",
			Source:      "http",
			Features: []provider.Feature{
				provider.FeatureStreaming,
				provider.FeatureTools,
				provider.FeatureKeyRequired,
				provider.FeatureMetered,
			},
			Presets: []provider.Preset{
				{
					ID:          "inworld-frontier",
					Order:       5,
					Label:       "Inworld Frontier Router",
					Description: "Routes prompts across frontier LLMs with automatic fallbacks.",
					Config: map[string]interface{}{
						"type":         "builtin",
						"builtin_name": "inworld",
						"model":        "inworld/compare-frontier-models",
						"temperature":  0.7,
						"max_tokens":   2048,
					},
				},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload harness.ModelBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewInworldLLMClient(payload.Config, payload.SharedKey)
		},
	})
}
```

- [ ] **Step 5: Wire Inworld into `pkg/provider/all/all.go`, `pkg/harness/exports.go`, and `pkg/harness/factory.go`**

In `pkg/provider/all/all.go`, add:
```go
_ "github.com/darkliquid/localrpg/pkg/provider/inworldllm"
```

In `pkg/harness/exports.go`, add `inworld` to `KeyFor`:
```go
func KeyFor(cfg ProviderConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "inworld":
		return provider.KeyLLMInworld, true
	case "http":
...
	case "builtin", "":
		switch cfg.BuiltinName {
		case "inworld":
			return provider.KeyLLMInworld, true
		case "gemini":
...
```

In `pkg/harness/factory.go`, pass `cfg.Providers.Inworld.APIKey`:
In `ModelBuildPayload` construction:
```go
	sharedKey := cfg.Providers.Gemini.APIKey
	if strings.Contains(string(key), "inworld") {
		sharedKey = cfg.Providers.Inworld.APIKey
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test -v ./pkg/provider/inworldllm/ && go test -v -run TestKeyFor ./pkg/harness/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/provider/inworldllm/ pkg/provider/all/all.go pkg/harness/exports.go pkg/harness/factory.go pkg/harness/key_test.go
git commit -m "feat(llm): implement inworld llm router provider"
```

---

### Task 3: Inworld TTS Provider Package & Voice Catalog (`pkg/provider/inworldtts`)

**Files:**
- Create: `pkg/provider/inworldtts/client.go`
- Create: `pkg/provider/inworldtts/inworldtts.go`
- Create: `pkg/provider/inworldtts/client_test.go`
- Modify: `pkg/provider/all/all.go`
- Modify: `pkg/media/exports.go:19-43`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/media/key_test.go`

- [ ] **Step 1: Write failing tests for Inworld TTS client**

Create `pkg/provider/inworldtts/client_test.go`:
```go
package inworldtts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestInworldTTS_SynthesizeAndAuth(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}
	fakeAudio := []byte("ID3fake-mp3-audio-bytes")
	b64Audio := base64.StdEncoding.EncodeToString(fakeAudio)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"audioContent": b64Audio,
		})
	}))
	defer server.Close()

	client, err := NewInworldTTSClient(config.TTSConfig{
		Endpoint: server.URL,
		APIKey:   "custom-key",
	}, "shared-key")
	if err != nil {
		t.Fatalf("NewInworldTTSClient: %v", err)
	}

	data, err := client.Synthesize(context.Background(), "Hello world", &entity.VoiceConfig{VoiceID: "Ashley"})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(data) != string(fakeAudio) {
		t.Errorf("got %q, want %q", string(data), string(fakeAudio))
	}
	if gotAuth != "Basic custom-key" {
		t.Errorf("gotAuth = %q, want Basic custom-key", gotAuth)
	}
	if gotBody["voice_id"] != "Ashley" {
		t.Errorf("voice_id = %v, want Ashley", gotBody["voice_id"])
	}
	if client.LastUsage().Characters != 11 {
		t.Errorf("LastUsage.Characters = %d, want 11", client.LastUsage().Characters)
	}
}

func TestInworldTTS_ListVoicesOfflineFallback(t *testing.T) {
	// Server returns 404 for voices endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewInworldTTSClient(config.TTSConfig{
		Endpoint: server.URL,
		APIKey:   "key",
	}, "")
	if err != nil {
		t.Fatalf("NewInworldTTSClient: %v", err)
	}

	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	if len(voices) == 0 {
		t.Fatalf("expected fallback voices, got 0")
	}
	foundAshley := false
	for _, v := range voices {
		if v.ID == "Ashley" {
			foundAshley = true
			break
		}
	}
	if !foundAshley {
		t.Errorf("expected Ashley in fallback voices")
	}
}

func TestInworldTTS_MissingKey(t *testing.T) {
	os.Unsetenv("INWORLD_API_KEY")
	_, err := NewInworldTTSClient(config.TTSConfig{}, "")
	if err == nil || !strings.Contains(err.Error(), "inworld: an API key is required") {
		t.Fatalf("expected missing key error, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/provider/inworldtts/`
Expected: FAIL with package not found

- [ ] **Step 3: Implement `pkg/provider/inworldtts/client.go`**

Create `pkg/provider/inworldtts/client.go`:
```go
package inworldtts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

var ErrMissingAPIKey = errors.New("inworld: an API key is required; set providers.inworld.api_key, media.tts.api_key, or INWORLD_API_KEY")

const (
	defaultEndpoint     = "https://api.inworld.ai/tts/v1/voice"
	defaultModel        = "inworld-tts-2"
	defaultVoice        = "Ashley"
	requestTimeout      = 60 * time.Second
)

type InworldTTSClient struct {
	apiKey       string
	model        string
	defaultVoice string
	endpoint     string
	client       *http.Client
	logger       trace.Logger

	usageMu   sync.Mutex
	lastUsage media.Usage
}

func NewInworldTTSClient(cfg config.TTSConfig, sharedKey string) (*InworldTTSClient, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(sharedKey)
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("INWORLD_API_KEY"))
	}
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}

	trace.RegisterSecret(apiKey)

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}

	defVoice := strings.TrimSpace(cfg.DefaultVoice)
	if defVoice == "" {
		defVoice = defaultVoice
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	return &InworldTTSClient{
		apiKey:       apiKey,
		model:        model,
		defaultVoice: defVoice,
		endpoint:     endpoint,
		client:       &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: requestTimeout},
	}, nil
}

func (c *InworldTTSClient) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

func (c *InworldTTSClient) Metered() bool { return true }

func (c *InworldTTSClient) LastUsage() media.Usage {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return c.lastUsage
}

func (c *InworldTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	voiceID := c.defaultVoice
	if voice != nil && strings.TrimSpace(voice.VoiceID) != "" {
		voiceID = strings.TrimSpace(voice.VoiceID)
	}

	reqBody := map[string]interface{}{
		"text":     text,
		"voice_id": voiceID,
		"model_id": c.model,
		"audio_config": map[string]interface{}{
			"audio_encoding":    "MP3",
			"sample_rate_hertz": 48000,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal inworld tts request: %w", err)
	}

	url := c.endpoint
	if !strings.HasSuffix(url, "/voice") && !strings.Contains(url, "127.0.0.1") && !strings.Contains(url, "localhost") {
		url = strings.TrimRight(url, "/") + "/tts/v1/voice"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create inworld tts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("inworld tts request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, c.mapError(resp.StatusCode, respBytes)
	}

	var res struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode inworld tts response: %w", err)
	}

	audioBytes, err := base64.StdEncoding.DecodeString(res.AudioContent)
	if err != nil {
		return nil, fmt.Errorf("decode base64 audioContent: %w", err)
	}

	c.usageMu.Lock()
	c.lastUsage = media.Usage{
		Requests:   1,
		Characters: len([]rune(text)),
	}
	c.usageMu.Unlock()

	return audioBytes, nil
}

func (c *InworldTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	// Fallback curated voices
	fallback := []media.ProviderVoice{
		{ID: "Ashley", Name: "Ashley", Gender: "female", Accent: "American", Description: "Natural, expressive American English female voice"},
		{ID: "Dennis", Name: "Dennis", Gender: "male", Accent: "American", Description: "Calm, conversational American English male voice"},
		{ID: "Sarah", Name: "Sarah", Gender: "female", Accent: "American", Description: "Warm, articulate narrator voice"},
		{ID: "Alex", Name: "Alex", Gender: "neutral", Accent: "American", Description: "Clear, adaptable neutral voice"},
	}

	voicesURL := "https://api.inworld.ai/tts/v1/voices"
	if strings.Contains(c.endpoint, "127.0.0.1") || strings.Contains(c.endpoint, "localhost") {
		voicesURL = c.endpoint + "/voices"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", voicesURL, nil)
	if err != nil {
		return fallback, nil
	}
	req.Header.Set("Authorization", "Basic "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return fallback, nil
	}
	defer resp.Body.Close()

	var apiRes struct {
		Voices []struct {
			VoiceID     string `json:"voice_id"`
			Name        string `json:"name"`
			Gender      string `json:"gender"`
			Description string `json:"description"`
		} `json:"voices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiRes); err != nil || len(apiRes.Voices) == 0 {
		return fallback, nil
	}

	var voices []media.ProviderVoice
	for _, v := range apiRes.Voices {
		voices = append(voices, media.ProviderVoice{
			ID:          v.VoiceID,
			Name:        v.Name,
			Gender:      v.Gender,
			Description: v.Description,
		})
	}
	return voices, nil
}

func (c *InworldTTSClient) mapError(status int, body []byte) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("inworld: invalid API key or permission denied; check providers.inworld.api_key or INWORLD_API_KEY")
	case http.StatusTooManyRequests:
		return errors.New("inworld: quota exceeded or rate limit reached; check your Inworld account credits")
	default:
		detail := provider.TruncateDetail(body)
		if detail != "" {
			return fmt.Errorf("inworld: %s (status %d)", detail, status)
		}
		return fmt.Errorf("inworld tts failed with status %d", status)
	}
}
```

- [ ] **Step 4: Implement `pkg/provider/inworldtts/inworldtts.go`**

Create `pkg/provider/inworldtts/inworldtts.go`:
```go
// Package inworldtts registers the Inworld AI Text-to-Speech provider.
package inworldtts

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyTTSInworld),
			Family:      provider.FamilyTTS,
			Label:       "Inworld TTS (Cloud, metered)",
			Description: "Natural-sounding dialogue and narration with inworld-tts-2.",
			Source:      "http",
			Features: []provider.Feature{
				provider.FeatureMetered,
				provider.FeatureKeyRequired,
				provider.FeatureVoiceCatalog,
			},
			Presets: []provider.Preset{
				{
					ID:          "inworld-tts",
					Order:       8,
					Label:       "Inworld TTS",
					Description: "Inworld Cloud TTS using inworld-tts-2. Set key in Providers tab or via INWORLD_API_KEY.",
					Config: map[string]interface{}{
						"type":          "builtin",
						"builtin_name":  "inworld",
						"model":         "inworld-tts-2",
						"default_voice": "Ashley",
						"pitch":         1.0,
						"speech_rate":   1.0,
						"auto_play":     true,
						"master_volume": 1.0,
					},
				},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewInworldTTSClient(payload.Config, payload.SharedKey)
		},
	})
}
```

- [ ] **Step 5: Wire Inworld into `pkg/provider/all/all.go`, `pkg/media/exports.go`, and `pkg/gui/service.go`**

In `pkg/provider/all/all.go`, add:
```go
_ "github.com/darkliquid/localrpg/pkg/provider/inworldtts"
```

In `pkg/media/exports.go`, add `inworld` to `TTSKeyFor`:
```go
func TTSKeyFor(cfg config.TTSConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "inworld":
		return provider.KeyTTSInworld, true
	case "gemini":
...
	case "builtin":
		switch cfg.BuiltinName {
		case "inworld":
			return provider.KeyTTSInworld, true
...
```

In `pkg/gui/service.go`:
In `ttsClientFor` / `TestProvider` for TTS, resolve shared key:
```go
	sharedKey := cfg.Providers.Gemini.APIKey
	if cfg.Media.TTS.Type == "inworld" || (cfg.Media.TTS.Type == "builtin" && cfg.Media.TTS.BuiltinName == "inworld") {
		sharedKey = cfg.Providers.Inworld.APIKey
	}
	return media.NewTTSClientWithSharedKey(cfg.Media.TTS, sharedKey)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test -v ./pkg/provider/inworldtts/ && go test -v -run TestTTSKeyFor ./pkg/media/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/provider/inworldtts/ pkg/provider/all/all.go pkg/media/exports.go pkg/gui/service.go pkg/media/key_test.go
git commit -m "feat(tts): implement inworld tts provider and voice catalog"
```

---

### Task 4: Inworld STT Provider Package & Shared Key Parity (`pkg/provider/inworldstt`)

**Files:**
- Modify: `pkg/media/exports.go:65-98`
- Modify: `pkg/media/providers.go:85-102`
- Create: `pkg/provider/inworldstt/client.go`
- Create: `pkg/provider/inworldstt/inworldstt.go`
- Create: `pkg/provider/inworldstt/client_test.go`
- Modify: `pkg/provider/all/all.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/media/key_test.go`

- [ ] **Step 1: Write failing tests for Inworld STT client**

Create `pkg/provider/inworldstt/client_test.go`:
```go
package inworldstt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestInworldSTT_TranscribeWAVAndOpus(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"transcription": map[string]interface{}{
				"transcript": "Hello from Inworld STT",
				"isFinal":    true,
			},
		})
	}))
	defer server.Close()

	client, err := NewInworldSTTClient(config.STTConfig{
		Endpoint: server.URL,
		APIKey:   "custom-stt-key",
	}, "shared-key")
	if err != nil {
		t.Fatalf("NewInworldSTTClient: %v", err)
	}

	// Test with WAV tone
	wavData := media.GenerateToneWAV(440, 0.1)
	transcript, err := client.Transcribe(context.Background(), wavData)
	if err != nil {
		t.Fatalf("Transcribe WAV: %v", err)
	}
	if transcript != "Hello from Inworld STT" {
		t.Errorf("transcript = %q, want Hello from Inworld STT", transcript)
	}
	if gotAuth != "Basic custom-stt-key" {
		t.Errorf("gotAuth = %q, want Basic custom-stt-key", gotAuth)
	}
	transConfig := gotBody["transcribe_config"].(map[string]interface{})
	if transConfig["audio_encoding"] != "LINEAR16" {
		t.Errorf("expected LINEAR16 for WAV, got %v", transConfig["audio_encoding"])
	}

	// Test with Opus bytes
	opusData := []byte("OggSfake-opus-data")
	_, err = client.Transcribe(context.Background(), opusData)
	if err != nil {
		t.Fatalf("Transcribe Opus: %v", err)
	}
	transConfig2 := gotBody["transcribe_config"].(map[string]interface{})
	if transConfig2["audio_encoding"] != "OGG_OPUS" {
		t.Errorf("expected OGG_OPUS for Ogg, got %v", transConfig2["audio_encoding"])
	}
}

func TestInworldSTT_MissingKey(t *testing.T) {
	os.Unsetenv("INWORLD_API_KEY")
	_, err := NewInworldSTTClient(config.STTConfig{}, "")
	if err == nil || !strings.Contains(err.Error(), "inworld: an API key is required") {
		t.Fatalf("expected missing key error, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/provider/inworldstt/`
Expected: FAIL with package not found

- [ ] **Step 3: Update `pkg/media/exports.go` and `pkg/media/providers.go` for shared key parity**

In `pkg/media/exports.go`:
Add `STTBuildPayload`:
```go
// STTBuildPayload is what BuildSTT hands an STT provider package.
type STTBuildPayload struct {
	Config    config.STTConfig `json:"config"`
	SharedKey string           `json:"shared_key,omitempty"`
}

// STTKeyFor maps an STT configuration to its canonical key.
func STTKeyFor(cfg config.STTConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "inworld":
		return provider.KeySTTInworld, true
	case "builtin":
		if cfg.BuiltinName == "inworld" {
			return provider.KeySTTInworld, true
		}
		return "", false
	case "http":
		return provider.InstanceOrSelf(provider.KeySTTWhisperHTTP, provider.HostDiscriminator(cfg.Endpoint)), true
	case "cli":
		return provider.InstanceOrSelf(provider.KeySTTWhisperCLI, provider.CommandDiscriminator(cfg.Command)), true
	}
	return "", false
}

// BuildSTT constructs an STT client from the registry by ID.
func BuildSTT(id string, cfg config.STTConfig, sharedKey string) (STTClient, error) {
	reg, ok := provider.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("media: no provider registered for %q", id)
	}
	raw, err := json.Marshal(STTBuildPayload{Config: cfg, SharedKey: sharedKey})
	if err != nil {
		return nil, fmt.Errorf("media: encode %s config: %w", id, err)
	}
	built, err := reg.Build(context.Background(), raw)
	if err != nil {
		return nil, err
	}
	client, ok := built.(STTClient)
	if !ok {
		return nil, fmt.Errorf("media: provider %q is not an stt client", id)
	}
	return client, nil
}
```

In `pkg/media/providers.go`:
```go
func NewSTTClient(cfg config.STTConfig) (STTClient, error) {
	return NewSTTClientWithSharedKey(cfg, "")
}

// NewSTTClientWithSharedKey builds an STTClient from configuration and an optional shared key.
func NewSTTClientWithSharedKey(cfg config.STTConfig, sharedKey string) (STTClient, error) {
	// Registry-first when pkg/provider/all was imported; inline otherwise.
	if key, ok := STTKeyFor(cfg); ok {
		if _, found := provider.Lookup(string(key.Parent())); found {
			return BuildSTT(string(key.Parent()), cfg, sharedKey)
		}
	}
	switch cfg.Type {
	case "disabled", "":
		return &disabledSTTClient{}, nil
	case "builtin":
		return &echoSTTClient{}, nil
	default:
		return nil, fmt.Errorf("unsupported stt provider type: %s", cfg.Type)
	}
}
```

- [ ] **Step 4: Implement `pkg/provider/inworldstt/client.go`**

Create `pkg/provider/inworldstt/client.go`:
```go
package inworldstt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

var ErrMissingAPIKey = errors.New("inworld: an API key is required; set providers.inworld.api_key, media.stt.api_key, or INWORLD_API_KEY")

const (
	defaultEndpoint = "https://api.inworld.ai/stt/v1/transcribe"
	defaultModel    = "inworld/inworld-stt-1"
	requestTimeout  = 60 * time.Second
)

type InworldSTTClient struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
	logger   trace.Logger

	usageMu   sync.Mutex
	lastUsage media.Usage
}

func NewInworldSTTClient(cfg config.STTConfig, sharedKey string) (*InworldSTTClient, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(sharedKey)
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("INWORLD_API_KEY"))
	}
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}

	trace.RegisterSecret(apiKey)

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	return &InworldSTTClient{
		apiKey:   apiKey,
		model:    model,
		endpoint: endpoint,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: requestTimeout},
	}, nil
}

func (c *InworldSTTClient) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

func (c *InworldSTTClient) Metered() bool { return true }

func (c *InworldSTTClient) LastUsage() media.Usage {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return c.lastUsage
}

func (c *InworldSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	if len(audioData) == 0 {
		return "", errors.New("empty audio data")
	}

	encoding := "LINEAR16"
	payloadBytes := audioData

	switch {
	case bytes.HasPrefix(audioData, []byte("OggS")):
		encoding = "OGG_OPUS"
	case bytes.HasPrefix(audioData, []byte("ID3")) || (len(audioData) > 1 && audioData[0] == 0xFF && audioData[1]&0xE0 == 0xE0):
		encoding = "MP3"
	case bytes.HasPrefix(audioData, []byte("RIFF")):
		// Decode WAV to raw PCM s16 samples
		pcm, _, _, err := media.DecodeProviderAudio(audioData, "audio/wav")
		if err == nil && len(pcm) > 0 {
			buf := make([]byte, len(pcm)*2)
			for i, sample := range pcm {
				binary.LittleEndian.PutUint16(buf[i*2:], uint16(sample))
			}
			payloadBytes = buf
		}
		encoding = "LINEAR16"
	}

	b64Content := base64.StdEncoding.EncodeToString(payloadBytes)

	reqBody := map[string]interface{}{
		"transcribe_config": map[string]interface{}{
			"model_id":       c.model,
			"language":       "en-US",
			"audio_encoding": encoding,
		},
		"audio_data": map[string]interface{}{
			"content": b64Content,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal inworld stt request: %w", err)
	}

	url := c.endpoint
	if !strings.HasSuffix(url, "/transcribe") && !strings.Contains(url, "127.0.0.1") && !strings.Contains(url, "localhost") {
		url = strings.TrimRight(url, "/") + "/stt/v1/transcribe"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create inworld stt request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("inworld stt request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return "", c.mapError(resp.StatusCode, respBytes)
	}

	var res struct {
		Transcription struct {
			Transcript string `json:"transcript"`
		} `json:"transcription"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("decode inworld stt response: %w", err)
	}

	c.usageMu.Lock()
	c.lastUsage = media.Usage{Requests: 1}
	c.usageMu.Unlock()

	return res.Transcription.Transcript, nil
}

func (c *InworldSTTClient) mapError(status int, body []byte) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("inworld: invalid API key or permission denied; check providers.inworld.api_key or INWORLD_API_KEY")
	case http.StatusTooManyRequests:
		return errors.New("inworld: quota exceeded or rate limit reached; check your Inworld account credits")
	default:
		detail := provider.TruncateDetail(body)
		if detail != "" {
			return fmt.Errorf("inworld: %s (status %d)", detail, status)
		}
		return fmt.Errorf("inworld stt failed with status %d", status)
	}
}
```

- [ ] **Step 5: Implement `pkg/provider/inworldstt/inworldstt.go`**

Create `pkg/provider/inworldstt/inworldstt.go`:
```go
// Package inworldstt registers the Inworld AI Speech-to-Text provider.
package inworldstt

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeySTTInworld),
			Family:      provider.FamilySTT,
			Label:       "Inworld STT (Cloud, metered)",
			Description: "Cloud speech recognition with voice profiling using inworld/inworld-stt-1.",
			Source:      "http",
			Features: []provider.Feature{
				provider.FeatureMetered,
				provider.FeatureKeyRequired,
			},
			Presets: []provider.Preset{
				{
					ID:          "inworld-stt",
					Order:       5,
					Label:       "Inworld STT",
					Description: "Cloud transcription via Inworld STT. Set key in Providers tab or via INWORLD_API_KEY.",
					Config: map[string]interface{}{
						"type":         "builtin",
						"builtin_name": "inworld",
						"model":        "inworld/inworld-stt-1",
					},
				},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.STTBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					// Fallback if raw was config.STTConfig directly
					var cfg config.STTConfig
					if err2 := json.Unmarshal(raw, &cfg); err2 == nil {
						payload.Config = cfg
					} else {
						return nil, err
					}
				}
			}
			return NewInworldSTTClient(payload.Config, payload.SharedKey)
		},
	})
}
```

- [ ] **Step 6: Wire Inworld STT into `pkg/provider/all/all.go` and `pkg/gui/service.go`**

In `pkg/provider/all/all.go`:
```go
_ "github.com/darkliquid/localrpg/pkg/provider/inworldstt"
```

In `pkg/gui/service.go`:
In `TranscribeAudio` and `TestProvider` for `"stt"`, pass `cfg.Providers.Inworld.APIKey` as `sharedKey`:
```go
	sharedKey := ""
	if sttCfg.Type == "inworld" || (sttCfg.Type == "builtin" && sttCfg.BuiltinName == "inworld") {
		sharedKey = cfg.Providers.Inworld.APIKey
	}
	client, err := media.NewSTTClientWithSharedKey(sttCfg, sharedKey)
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test -v ./pkg/provider/inworldstt/ && go test -v -run TestSTTKeyFor ./pkg/media/`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add pkg/provider/inworldstt/ pkg/provider/all/all.go pkg/media/exports.go pkg/media/providers.go pkg/gui/service.go pkg/media/key_test.go
git commit -m "feat(stt): implement inworld stt provider and shared key support"
```

---

### Task 5: Registry Validation & Documentation Generation

**Files:**
- Test: `pkg/provider/provider_test.go`
- Test: `pkg/gui/docs_catalogue_test.go`
- Generated docs: `pkg/gui/docs/`

- [ ] **Step 1: Verify provider registry passes validation**

Run: `go test -v ./pkg/provider/all/ && go test -v ./pkg/provider/`
Expected: PASS (`provider.Validate()` passes with `llm:inworld`, `tts:inworld`, and `stt:inworld`)

- [ ] **Step 2: Regenerate embedded documentation**

Run: `go test ./pkg/gui -update-docs`
Expected: Updates `pkg/gui/docs/` articles with Inworld providers and presets

- [ ] **Step 3: Run documentation catalogue test**

Run: `go test -v ./pkg/gui/docs_catalogue_test.go`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/gui/docs/
git commit -m "docs: regenerate provider catalogue and config docs with inworld"
```

---

### Task 6: Frontend Settings Studio UI

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [ ] **Step 1: Update frontend types in `frontend/src/types.ts`**

In `frontend/src/types.ts`:
Add `InworldProviderConfig` and update `ProvidersConfig`:
```typescript
export interface InworldProviderConfig {
  api_key?: string;
}

export interface ProvidersConfig {
  gemini?: GeminiProviderConfig;
  inworld?: InworldProviderConfig;
  currency?: string;
  prices?: PriceConfig[];
}
```

- [ ] **Step 2: Add Inworld AI credential card to Providers Tab in `SettingsStudio.tsx`**

In `frontend/src/components/SettingsStudio.tsx`, in the Providers tab (next to the Google Gemini card):
Add the Inworld AI credential card:
```tsx
            {/* Inworld AI Provider Card */}
            <div className="bg-stone-900/60 border border-stone-800 rounded-lg p-5 space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Cloud className="w-5 h-5 text-indigo-400" />
                  <h3 className="font-medium text-stone-200">Inworld AI</h3>
                </div>
                <span
                  className={`text-xs px-2 py-0.5 rounded border ${
                    config.providers?.inworld?.api_key
                      ? 'bg-emerald-950/60 border-emerald-800 text-emerald-400'
                      : 'bg-stone-800/60 border-stone-700 text-stone-400'
                  }`}
                >
                  {config.providers?.inworld?.api_key ? '✓ Custom Key Saved' : 'Optional if INWORLD_API_KEY is set'}
                </span>
              </div>
              <p className="text-xs text-stone-400 leading-relaxed">
                Shared Basic (Base64) authorization key for Inworld LLM Router, Inworld TTS (<code>inworld-tts-2</code>), and Inworld STT.
                Generate a key at{' '}
                <a
                  href="https://platform.inworld.ai/api-keys"
                  target="_blank"
                  rel="noreferrer"
                  className="text-amber-400 hover:underline"
                >
                  platform.inworld.ai/api-keys
                </a>{' '}
                or authenticate via CLI (<code>inworld auth login</code>).
              </p>
              <div>
                <label className="block text-xs font-mono text-stone-400 mb-1">
                  API Key (Base64)
                </label>
                <input
                  type="password"
                  value={config.providers?.inworld?.api_key || ''}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      providers: {
                        ...config.providers,
                        inworld: {
                          ...config.providers?.inworld,
                          api_key: e.target.value,
                        },
                      },
                    })
                  }
                  placeholder="Paste Inworld Basic (Base64) key or set INWORLD_API_KEY"
                  className="w-full bg-stone-950 border border-stone-800 rounded px-3 py-2 text-sm text-stone-200 focus:outline-none focus:border-amber-500 font-mono"
                />
              </div>
            </div>
```

- [ ] **Step 3: Add shared key placeholder hints in LLM, TTS, and STT tabs**

In `SettingsStudio.tsx`:
- In the LLM tab API key input, when the selected role uses Inworld (`currentRoleConfig.type === 'inworld' || currentRoleConfig.builtin_name === 'inworld'`):
  Display helper placeholder: `"Using shared key from providers.inworld.api_key"` if `config.providers?.inworld?.api_key` is set.
- In the TTS tab API key input, when `config.media.tts.builtin_name === 'inworld'`:
  Display helper placeholder: `"Using shared key from providers.inworld.api_key"` if `config.providers?.inworld?.api_key` is set.
- In the STT tab, add API key input field for Inworld with shared key placeholder support.

- [ ] **Step 4: Verify frontend builds cleanly**

Run: `npx tsc --noEmit` in `frontend/`
Expected: PASS (no type errors, no unused variables)

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(gui): add inworld ai credential management and settings controls"
```

---

### Task 7: End-to-End Verification & Quality Gates

**Files:**
- All modified and created files

- [ ] **Step 1: Run full backend test suite**

Run: `go test -v -count=1 ./...`
Expected: PASS across all packages

- [ ] **Step 2: Run Go vet linting**

Run: `go vet ./...`
Expected: PASS (clean output)

- [ ] **Step 3: Run frontend build**

Run: `cd frontend && npm run build`
Expected: PASS (Vite builds bundle into `pkg/gui/dist`)

- [ ] **Step 4: Scan for secrets**

Run: `mise run secrets:scan`
Expected: PASS (no leaked keys)

- [ ] **Step 5: Final git status check and verification**

Run: `git status`
Expected: Working tree clean, all commits scoped and structured.
