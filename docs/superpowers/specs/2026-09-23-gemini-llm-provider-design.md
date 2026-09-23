# Design Spec: Google Gemini LLM Provider (Phase 1: Text Generation & Role Routing)

**Date:** 2026-09-23  
**Status:** Approved  
**Target:** `pkg/config`, `pkg/harness`, `pkg/gui`, `frontend`  
**Phase:** 1 of 4 (Phase 1: LLM Text Generation, Phase 2: Imagen Image Generation, Phase 3: Gemini Audio TTS, Phase 4: SQLite Vector Embeddings)  

---

## 1. Executive Summary

This specification defines the integration of Google Gemini as a first-class LLM provider in LocalRPG. Built using the official Google GenAI Go SDK (`google.golang.org/genai`), it provides high-throughput streaming text generation, tool/function calling for campaign queries (`get_entity`, `search_entities`), fine-grained reasoning/thinking budget controls (`thinking_budget`), and sampling tunables (`top_p`, `top_k`, `temperature`).

To prevent internal reasoning chains from contaminating narrative transcripts or being voiced by TTS synthesizers, the streaming pipeline filters out thinking tokens (`part.Thought = true`) and captures them in the turn trace sink (`pkg/trace`).

Authentication supports a shared top-level `providers.gemini.api_key` configuration with per-role overrides and environment variable fallbacks (`GEMINI_API_KEY`, `GOOGLE_API_KEY`), establishing a unified credential model for subsequent Gemini media expansions (Imagen 3 image generation and Gemini TTS).

---

## 2. Architecture & Configuration

### 2.1 Credential Resolution & Provider Hierarchy

A campaign or user often uses Gemini across multiple roles (e.g. GM, Narrator, Extractor, Summariser) and will subsequently use it for images and voice. Rather than requiring duplicate API keys across every role and media block, a global `providers.gemini` block is introduced in `config.yaml`.

Resolution priority order for any Gemini operation:
1. Role-specific API key: `agents.roles.<role>.api_key` (if non-empty).
2. Shared provider API key: `providers.gemini.api_key` (if non-empty).
3. Environment variables: `GEMINI_API_KEY`, then `GOOGLE_API_KEY`.
4. If unresolved: Returns actionable error `gemini: an API key is required; set providers.gemini.api_key, agents.roles.<role>.api_key, or GEMINI_API_KEY`.

### 2.2 Configuration Schema

#### `pkg/config/types.go`

```go
// ProvidersConfig groups shared credentials and defaults for external ecosystem providers.
type ProvidersConfig struct {
    Gemini GeminiProviderConfig `yaml:"gemini,omitempty" json:"gemini,omitempty"`
}

type GeminiProviderConfig struct {
    APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}
```

Add `Providers ProvidersConfig` to the root `Config` struct.

#### `AgentRoleConfig` Extensions

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

    // Advanced Gemini & reasoning tunables
    ThinkingBudget *int     `yaml:"thinking_budget,omitempty" json:"thinking_budget,omitempty"`
    TopP           *float64 `yaml:"top_p,omitempty" json:"top_p,omitempty"`
    TopK           *int     `yaml:"top_k,omitempty" json:"top_k,omitempty"`
}
```

**Thinking Budget Semantics:**
- `ThinkingBudget == nil`: Model default reasoning behavior.
- `*ThinkingBudget == 0`: Thinking explicitly disabled (e.g. for instant, low-latency narration turns).
- `*ThinkingBudget == -1`: Dynamic reasoning (the model allocates thinking tokens automatically based on prompt complexity).
- `*ThinkingBudget > 0`: Hard cap on reasoning tokens (e.g. 1024, 2048, 8192).

### 2.3 Role Presets

In `pkg/config/presets.go`, provide built-in presets for Gemini models:

- `gemini-2.5-flash`:
  - `Type: "builtin"`, `BuiltinName: "gemini"`, `Model: "gemini-2.5-flash"`
  - `Temperature: 0.7`, `MaxTokens: 2048`, `ThinkingBudget: ptr(0)` (instant response)
- `gemini-2.5-pro`:
  - `Type: "builtin"`, `BuiltinName: "gemini"`, `Model: "gemini-2.5-pro"`
  - `Temperature: 0.7`, `MaxTokens: 4096`, `ThinkingBudget: ptr(-1)` (deep reasoning)
- `gemini-2.0-flash`:
  - `Type: "builtin"`, `BuiltinName: "gemini"`, `Model: "gemini-2.0-flash"`
  - `Temperature: 0.7`, `MaxTokens: 2048`
- `gemini-2.0-flash-lite`:
  - `Type: "builtin"`, `BuiltinName: "gemini"`, `Model: "gemini-2.0-flash-lite"`
  - `Temperature: 0.7`, `MaxTokens: 2048`

---

## 3. Provider Implementation

### 3.1 Structure & Interfaces

`GeminiProvider` is located in `pkg/harness/gemini_provider.go`.

```go
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
```

It implements:
- `harness.ModelProvider`:
  - `ID() string`
  - `Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)`
  - `Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error`
- `harness.ToolCaller`:
  - `ToolCallerCapable() bool`: Returns `true`.
- `trace.LogAware`:
  - `SetLogger(trace.Logger)`
  - `SetChunkLimit(int)`

### 3.2 Request & Context Mapping

Given `harness.GenerateRequest`:

1. **System Instruction**:
   - `req.System` (containing world rules, lore, and voice profiles assembled by `AssembleContextWithProfiles`) is assigned to `genai.GenerateContentConfig.SystemInstruction`:
     ```go
     cfg.SystemInstruction = &genai.Content{
         Parts: []*genai.Part{{Text: req.System}},
     }
     ```
2. **Messages & History**:
   - `req.Messages` maps to `[]*genai.Content`:
     - `Role: "user"`: Converted to `Role: "user"`, `Parts: []*genai.Part{{Text: msg.Content}}`.
     - `Role: "assistant"`: Converted to `Role: "model"`. If `msg.ToolCalls` are present, map them to `FunctionCall` parts.
     - `Role: "tool"`: Converted to `Role: "user"` containing a `FunctionResponse` part:
       ```go
       &genai.FunctionResponse{
           Name: toolCallName,
           Response: map[string]interface{}{"result": msg.Content},
       }
       ```
   - Fallback: If `req.Messages` is empty, single-turn prompts fall back to `req.PromptText()` as a user part.
3. **Tools / Function Declarations**:
   - Each `harness.ToolSpec` is mapped into a `genai.FunctionDeclaration`:
     - `Name: spec.Name`
     - `Description: spec.Description`
     - `Parameters: convertToGenAISchema(spec.Parameters)`
4. **Tunables Configuration**:
   - `Temperature`: Copied from `req.Temperature` or provider config if positive.
   - `TopP` & `TopK`: Mapped to `cfg.TopP` and `cfg.TopK`.
   - `MaxOutputTokens`: Mapped from `req.MaxTokens` or provider config.
   - `ThinkingConfig`:
     - If `thinkingBudget` is set:
       ```go
       cfg.ThinkingConfig = &genai.ThinkingConfig{
           ThinkingBudget: genai.Ptr(int32(*h.thinkingBudget)),
       }
       ```

### 3.3 Streaming & Thought Separation Flow

```
[Google GenAI Stream Chunk]
       │
       ├── part.Thought == true
       │       │
       │       └──► [Trace Logger (gemini_thought)]  (Filtered from chronicle)
       │
       ├── part.Text != "" && !part.Thought
       │       │
       │       └──► [outChan <- StreamChunk{Text: text}] (Chronicle & TTS)
       │
       └── part.FunctionCall != nil
               │
               └──► [outChan <- StreamChunk{ToolCalls: [...]}]
```

1. **Thought Filtering**:
   - Each received chunk part is inspected.
   - If `part.Thought` is true:
     - The text is withheld from `out chan<- StreamChunk`.
     - If `h.logger != nil`, it is written to the trace sink as event `gemini_thought`.
2. **Narration Streaming**:
   - Parts with `!part.Thought` and non-empty `Text` are emitted immediately to `out <- StreamChunk{Text: part.Text}`.
3. **Tool Invocations**:
   - Parts containing `FunctionCall` are converted into `harness.ToolCall` with serialized JSON arguments and emitted via `StreamChunk{ToolCalls: [...]}`.
4. **Completion & Cleanup**:
   - On iterator completion, emit `StreamChunk{Done: true, FinishReason: reason}` and close `out`.

---

## 4. Error Handling & Redaction

### 4.1 Error Mapping

Gemini API errors are mapped to clean, user-friendly messages:
- **HTTP 401 / 403**: `"gemini: invalid API key or permission denied; check providers.gemini.api_key or GEMINI_API_KEY"`
- **HTTP 429**: `"gemini: quota exceeded or rate limit reached; check your Google AI Studio plan and credits"`
- **HTTP 404**: `"gemini: model %q not found or not supported for this API key"`
- **Safety Blocks**: If a response candidate contains finish reason `SAFETY` or `BLOCKLIST`, return error: `"gemini: generation blocked by safety policies: %s"`.

### 4.2 Key Redaction

API keys must never be logged or echoed in traces. `trace.RedactKey(apiKey)` is registered upon client initialization.

---

## 5. Frontend Settings UI (LLM Tab)

In `frontend/src/`:
1. **Provider Type Selector**:
   - Add "Google Gemini" (`builtin:gemini`) to the provider dropdown.
2. **Model Selection**:
   - Dropdown with curated presets (`gemini-2.5-flash`, `gemini-2.5-pro`, `gemini-2.0-flash`, `gemini-2.0-flash-lite`).
   - Freeform text input for custom or newly released models.
3. **Thinking Budget Control**:
   - Segmented/slider input:
     - "Disabled (0)" - Instant narration
     - "Dynamic (-1)" - Model decides reasoning tokens
     - "Custom" - Number input for specific token budget (e.g. 1024 to 8192)
4. **Credential Indicator**:
   - Shows "Inherited from global Gemini key" or "Configured via environment variable" when the per-role API key field is blank but a global key is active.

---

## 6. Testing Strategy

All automated tests execute offline without requiring active internet connectivity or a live API key:

1. **Credential Resolution Tests** (`pkg/harness/gemini_provider_test.go`):
   - Role key > Global key > Environment variables > Missing key error.
2. **Request Translation Tests**:
   - Validates mapping of `req.System` to `SystemInstruction`.
   - Validates multi-turn role translation (`user`, `assistant`, `tool`).
   - Validates `ToolSpec` JSON schema conversion to GenAI function declarations.
   - Validates `ThinkingConfig` values (0, -1, positive limits).
3. **Stream & Thought Filter Tests**:
   - Simulates chunks with `part.Thought = true` and `part.Thought = false`.
   - Asserts that zero thought tokens are sent to `out chan<- StreamChunk`.
   - Asserts that all narration tokens reach `StreamChunk.Text`.
   - Asserts that function call chunks correctly parse into `StreamChunk.ToolCalls`.
4. **Trace Recording Tests**:
   - Asserts that `gemini_thought` events are recorded to `trace.Logger` when enabled.
5. **Quality Gates**:
   - `mise run test:backend` (`go test -v -count=1 ./...`)
   - `mise run lint` (`go vet ./...`)
   - `mise run test:frontend` (`npx tsc --noEmit`)
