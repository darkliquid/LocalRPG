# Structured GM Generation & Native Response Schema Design

**Date:** 2026-10-01  
**Status:** Approved  
**Scope:** Engine turn generation, native response schema / structured outputs, hybrid state validation, fallback extractor deprecation  
**Related:** `docs/superpowers/specs/2026-09-25-structured-turn-protocol-design.md`, `pkg/engine/orchestrator.go`, `pkg/harness/turn_tools.go`, `pkg/harness/context.go`, `pkg/provider/openaichat`, `pkg/provider/geminillm`

---

## 1. Overview & Problem Statement

Today, LocalRPG supports a `submit_turn` tool call specified in the Structured Turn Protocol, but models frequently output freeform prose rather than calling `submit_turn`. When this occurs:
1. `result.Submission` is `nil`, bypassing structured extraction.
2. The orchestrator triggers an asynchronous secondary model call (`harness.Extractor.Extract`) to reconstruct mentions, speakers, and new entities via regex, wikilinks, and secondary prompts.
3. This doubles latency, wastes tokens, and introduces inaccuracies in speaker attribution and entity state.

Furthermore, modern LLM providers (OpenAI, Gemini, Ollama, LM Studio) now support **Native Structured Outputs / JSON Schema response formats**, which guarantee 100% adherence to a target schema without function-calling loops.

This specification elevates structured generation to the **primary path**:
- Employs native structured outputs (`response_format` / `response_schema`) or required tool calls (`submit_turn`).
- Supports mid-stream tabletop mechanics checks (`request_check` / `propose_check`) before terminal turn submission.
- Implements **Option C (Hybrid Validation)**: mechanical state mutations (`hp`, `inventory`, `stats`) are routed through `mechanics.js` and `GameHostAPI` validation hooks, while narrative entity introduction, entity memories, and dialogue segments are committed directly.
- Retains the secondary `harness.Extractor` strictly as an offline fallback for legacy text-only models or malformed submissions.

---

## 2. Architecture & Design

### 2.1 Provider Capability Model & Response Schema

In `pkg/harness/capabilities.go` and `pkg/harness/types.go`:
- Extend `Capabilities` and `GenerateRequest`:
  ```go
  type ResponseSchemaSpec struct {
      Name        string
      Description string
      Schema      map[string]interface{}
      Strict      bool
  }

  type GenerateRequest struct {
      // ... existing fields ...
      ResponseSchema *ResponseSchemaSpec `json:"response_schema,omitempty"`
  }
  ```
- Providers declare `StructuredOutputCapable() bool`.
  - `pkg/provider/openaichat`: maps `ResponseSchema` to `response_format: { type: "json_schema", json_schema: { name: ..., schema: ..., strict: true } }`. (Supported by OpenAI, Ollama `/api/chat`, vLLM, and LM Studio).
  - `pkg/provider/geminillm`: maps `ResponseSchema` to `genai.GenerateContentConfig.ResponseMIMEType = "application/json"` and `ResponseJsonSchema`.

### 2.2 Turn Execution Flow

```
[Start Turn: ProcessAction]
       │
       ▼
[Assemble Context Prompt]
   - Rules, Lore, Scene Canon, Recent History
   - Explicit "TURN PROTOCOL" instruction: must resolve uncertainty with request_check, 
     then terminate with structured turn payload.
       │
       ▼
[Offer Mechanics Check Tools (Round 0)]
   - Tools offered: request_check, propose_check, search_entities, etc.
   - If model calls request_check -> Resolve roll via rules -> feed result back as tool message.
       │
       ▼
[Terminal Generation (Structured Output / submit_turn)]
   - If provider supports ResponseSchema and no further tool calls are needed:
       Request final completion with TurnSubmission ResponseSchema.
       Model streams valid JSON directly in body.
   - Else if provider supports Tool Calling:
       Offer submit_turn (ToolChoice: "required" on final round).
       Model emits submit_turn arguments.
       │
       ▼
[Validate Submission (Option C: Hybrid Validation)]
   - Verify verdict, check references, and personae.
   - Split state changes:
       * Mechanical changes -> rules.ApplyStateChanges (GameHostAPI / mechanics.js).
       * Narrative personae, memories, dialogue -> directly committed to timeline.
   - If protocol validation fails:
       Retry once with validation error feedback.
       If second attempt fails -> Fallback to raw text & Extractor.
       │
       ▼
[RecordTurnContextStructured]
   - Commit segments, mentions, personae, memories, and validated stats to SQLite and history.jsonl.
```

### 2.3 Prompt Guidance (`pkg/harness/context.go`)

Add a dedicated `protocol` section to `ContextAssembler`:
```markdown
## TURN PROTOCOL & RESOLUTION RULES
1. If the protagonist's action involves uncertainty, danger, or opposition, you MUST call `request_check` before narrating the outcome. Never invent dice roll outcomes that the engine has not provided.
2. Complete your response by providing the final structured turn payload containing your verdict, ordered narration/dialogue segments, and any introduced personae or state changes.
3. Every speech segment must name its speaker. If the speaker is a new character, declare them under `personae`.
```

### 2.4 Hybrid Validation Rules (Option C)

In `pkg/engine/submission.go` and `pkg/rules/state_changes.go`:
1. **Mechanical Properties**:
   - Dotted paths declared in `core.MechanicsSpec` (`stats`, `health`, etc.) must be evaluated by `rules.ApplyStateChanges`.
   - If `mechanics.js` implements a `validateStateChange(entity, path, op, value)` hook, it is executed. Any rejected or out-of-bounds mutation is recorded as a trace event and skipped without failing the turn.
2. **Narrative Properties**:
   - Entities declared in `personae` are slugified and created as stub markdown notes if not already present.
   - Memories are indexed into SQLite `entity_memories` directly.
   - Dialogue segments are mapped to `entity.TurnSegment` and voiced accordingly.

---

## 3. Implementation Steps

1. **`pkg/harness`**:
   - Add `ResponseSchemaSpec` to `harness.GenerateRequest`.
   - Add `TurnSubmissionSchema()` helper generating the canonical JSON Schema for `TurnSubmission`.
   - Update `Describe()` and add `StructuredOutputCapable()` interface probe.
2. **Providers (`pkg/provider/openaichat`, `pkg/provider/geminillm`)**:
   - Wire `req.ResponseSchema` to OpenAI JSON Schema `response_format` and Gemini `ResponseJsonSchema`.
3. **`pkg/harness/context.go`**:
   - Add the turn protocol section instructing the model on structured completion and check resolution.
4. **`pkg/engine/orchestrator.go`**:
   - In `runGenerationLoop`, if the provider supports `ResponseSchema` and checks are complete, request structured output directly.
   - Parse direct JSON response body as `TurnSubmission` when structured response format is used.
   - Retain `harness.Extractor` only when `structured == false` (fallback path).
5. **Validation & Testing**:
   - Unit tests covering OpenAI and Gemini structured output formatting.
   - Unit tests in `pkg/engine` verifying structured turn parsing, hybrid state dispatch, and extractor fallback triggers.

---

## 4. Success Criteria

- With OpenAI-compatible or Gemini providers, turns produce structured segments, verdict, personae, and memories with **zero calls** to `harness.Extractor`.
- Latency per turn decreases because the secondary extraction model pass is skipped.
- Mechanics checks (`request_check`) correctly resolve and feed back into the final structured output.
- Legacy text-only or fallback models still operate seamlessly using the asynchronous extractor.
