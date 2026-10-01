# Structured GM Generation & Response Schema Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement native structured outputs / response schema and required turn submission in the GM generation loop, eliminating secondary extractor calls while preserving fallback support.

**Architecture:** Extend provider capabilities with `StructuredOutputCapable` and `ResponseSchemaSpec`. Wire OpenAI and Gemini providers to native schema formats. Add prompt protocol instructions to `ContextAssembler` and update `TurnOrchestrator` to parse direct JSON submissions, validate mechanical changes via `rules.ApplyStateChanges`, and fall back to `harness.Extractor` only when structured generation fails.

**Tech Stack:** Go standard library, `modernc.org/sqlite`, OpenAI Chat Completions API / JSON Schema, Google Gemini GenAI SDK.

---

### File Map

- **`pkg/harness/types.go`**: Add `ResponseSchemaSpec` to `GenerateRequest`. Add `StructuredOutputCapable` interface.
- **`pkg/harness/capabilities.go`**: Add `ResponseSchema` / `StructuredOutput` to `Capabilities` and update `Describe()`.
- **`pkg/harness/turn.go`**: Add `TurnSubmissionSchema()` returning the JSON schema for `TurnSubmission`.
- **`pkg/harness/turn_test.go`**: Test `TurnSubmissionSchema()` validity.
- **`pkg/provider/openaichat/http.go`**: Serialize `ResponseSchema` to `response_format: { type: "json_schema", ... }`.
- **`pkg/provider/openaichat/http_test.go`**: Test `response_format` serialization.
- **`pkg/provider/geminillm/provider.go`**: Set `ResponseMIMEType = "application/json"` and `ResponseJsonSchema`.
- **`pkg/provider/geminillm/provider_test.go`**: Test Gemini schema configuration.
- **`pkg/harness/context.go`**: Add `turnProtocolSection` instructing the model to resolve uncertainty with `request_check` and submit a structured turn.
- **`pkg/harness/context_test.go`**: Verify protocol section rendering.
- **`pkg/engine/orchestrator.go`**: Handle direct JSON response bodies as `TurnSubmission`, set `ToolChoice` / response format, and invoke `Extractor` only when `!structured`.
- **`pkg/engine/structured_turn_test.go`**: Verify end-to-end structured generation without extractor.

---

### Task 1: Add ResponseSchemaSpec and TurnSubmissionSchema in `pkg/harness`

- [ ] **Step 1.1**: In `pkg/harness/types.go`, define `ResponseSchemaSpec`:
  ```go
  type ResponseSchemaSpec struct {
      Name        string                 `json:"name"`
      Description string                 `json:"description,omitempty"`
      Schema      map[string]interface{} `json:"schema"`
      Strict      bool                   `json:"strict,omitempty"`
  }
  ```
  Add `ResponseSchema *ResponseSchemaSpec` to `GenerateRequest`.
  Define `StructuredOutputProvider` interface:
  ```go
  type StructuredOutputProvider interface {
      StructuredOutputCapable() bool
  }
  ```
- [ ] **Step 1.2**: In `pkg/harness/capabilities.go`, add `StructuredOutput bool` to `Capabilities` and probe for `StructuredOutputProvider` in `Describe()`.
- [ ] **Step 1.3**: In `pkg/harness/turn.go`, implement `TurnSubmissionSchema() map[string]interface{}` which returns the JSON schema matching `submit_turn`'s parameter schema.
- [ ] **Step 1.4**: Write unit test in `pkg/harness/turn_test.go` asserting `TurnSubmissionSchema()` has `type: object`, required fields (`action_verdict`, `segments`), and matches `TurnSubmission`.
- [ ] **Step 1.5**: Run `go test ./pkg/harness/...` and verify all tests pass.
- [ ] **Step 1.6**: Commit: `git commit -am "feat(harness): add response schema spec and turn submission schema"`

---

### Task 2: Implement ResponseSchema in OpenAI Chat Provider

- [ ] **Step 2.1**: In `pkg/provider/openaichat/http.go`:
  - Implement `StructuredOutputCapable() bool { return true }`.
  - Add `ResponseFormat *openAIResponseFormat` to `openAIChatRequest`.
  - Define `openAIResponseFormat`:
    ```go
    type openAIResponseFormat struct {
        Type       string                    `json:"type"` // "json_schema" or "json_object"
        JSONSchema *openAIJSONSchemaWrapper  `json:"json_schema,omitempty"`
    }
    type openAIJSONSchemaWrapper struct {
        Name        string                 `json:"name"`
        Description string                 `json:"description,omitempty"`
        Schema      map[string]interface{} `json:"schema"`
        Strict      bool                   `json:"strict,omitempty"`
    }
    ```
  - In `streamOnce`, if `req.ResponseSchema != nil`, populate `payload.ResponseFormat`.
- [ ] **Step 2.2**: Write unit tests in `pkg/provider/openaichat/http_test.go` testing that `req.ResponseSchema` serializes to `response_format` with `type: "json_schema"`.
- [ ] **Step 2.3**: Run `go test ./pkg/provider/openaichat/...` and verify tests pass.
- [ ] **Step 2.4**: Commit: `git commit -am "feat(openaichat): support native json_schema response format"`

---

### Task 3: Implement ResponseSchema in Gemini Provider

- [ ] **Step 3.1**: In `pkg/provider/geminillm/provider.go`:
  - Implement `StructuredOutputCapable() bool { return true }`.
  - In `buildGenerateConfig`, if `req.ResponseSchema != nil`, set:
    ```go
    cfg.ResponseMIMEType = "application/json"
    cfg.ResponseJsonSchema = req.ResponseSchema.Schema
    ```
- [ ] **Step 3.2**: Write unit tests in `pkg/provider/geminillm/provider_test.go` verifying `ResponseMIMEType` and `ResponseJsonSchema` are populated.
- [ ] **Step 3.3**: Run `go test ./pkg/provider/geminillm/...` and verify tests pass.
- [ ] **Step 3.4**: Commit: `git commit -am "feat(geminillm): support native response schema"`

---

### Task 4: Add Turn Protocol Instruction to ContextAssembler

- [ ] **Step 4.1**: In `pkg/harness/context.go`, define `turnProtocolInstruction`:
  ```markdown
  ## TURN RESOLUTION PROTOCOL
  1. For any action with uncertain consequences, resolve it by calling `request_check` before narrating the outcome. Never invent dice roll outcomes.
  2. End your turn by providing the structured turn output (action verdict, ordered segments, introduced personae, memories, state changes).
  3. Every speech segment must name its speaker. If introducing a new character, declare them under personae.
  ```
  Add a `protocol` section to `buildSections()` with high rank (non-droppable).
- [ ] **Step 4.2**: In `pkg/harness/context_test.go`, verify the assembled prompt includes the `TURN RESOLUTION PROTOCOL` section.
- [ ] **Step 4.3**: Run `go test ./pkg/harness/...` and verify tests pass.
- [ ] **Step 4.4**: Commit: `git commit -am "feat(harness): include turn protocol instructions in context prompt"`

---

### Task 5: Orchestrator Structured Response Handling & Direct JSON Parsing

- [ ] **Step 5.1**: In `pkg/engine/orchestrator.go`:
  - When configuring the terminal generation request in `runGenerationLoop`:
    - If the provider supports `StructuredOutputCapable()` and no checks remain pending, set `req.ResponseSchema = &harness.ResponseSchemaSpec{ Name: "turn_submission", Schema: harness.TurnSubmissionSchema(), Strict: true }`.
  - In `runGenerationLoop`, when receiving pure text (`len(result.ToolCalls) == 0`):
    - Attempt `harness.ParseSubmission(result.Text)`.
    - If valid, populate `result.Submission` directly and skip fallback.
    - If invalid or unparseable, log trace and fall back to raw text.
- [ ] **Step 5.2**: In `pkg/engine/orchestrator.go:960`:
  - Ensure `o.extractor.Extract` is strictly skipped when `structured == true`.
  - Log `turn.structured_generation` event in telemetry/trace.
- [ ] **Step 5.3**: Write unit tests in `pkg/engine/structured_turn_test.go`:
  - Test model returning JSON string body -> parsed as structured turn without calling extractor.
  - Test model returning tool call `submit_turn` -> parsed as structured turn without calling extractor.
  - Test malformed submission -> gracefully falls back to extractor.
- [ ] **Step 5.4**: Run `go test ./pkg/engine/...` and ensure all engine tests pass.
- [ ] **Step 5.5**: Commit: `git commit -am "feat(engine): support direct structured output parsing in turn orchestrator"`

---

### Task 6: Full Verification & Integration Testing

- [ ] **Step 6.1**: Run `mise run test:backend` to run the full Go test suite.
- [ ] **Step 6.2**: Run `mise run lint` to verify go vet, markdownlint, and actionlint.
- [ ] **Step 6.3**: Commit: `git commit -am "chore: verify backend tests and linting for structured turn milestone"`
