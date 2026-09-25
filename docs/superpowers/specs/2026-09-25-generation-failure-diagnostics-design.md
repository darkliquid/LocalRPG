# Generation Failure Diagnostics Design

**Date:** 2026-09-25
**Status:** Proposed
**Scope:** Backend generation paths (one-shot text, turn, image), frontend generation controls
**Related:** On-Demand Text Generation Design (2026-09-24), Agentic Turns and Tools Design (2026-09-22), Narrative Coherence and Trace Design (2026-09-22), Creation Flow Asset Generation Design (2026-09-24)

## 1. Overview & Goals

Generation failures are currently invisible to the user and poorly explained in
the trace. When an AI-generated field, an image, or a full turn produces nothing,
the app either returns a successful-looking empty response or an error with no
reason attached. The trace records that a failure happened but not *why*: there
is no record of the assembled prompt, the role fallback chain, whether the
provider errored, or whether it returned an empty completion.

This specification defines a single, shared generation-failure contract so that
every failure carries a machine-readable code, a human-readable message, and the
attempt history, and so that this information reaches both the trace and the
client.

**Goals:**

- No generation failure is silently discarded anywhere in the stack.
- Every failure has a stable code, a human message, and an attempt history
  (role, provider, code, duration).
- One-shot endpoints (`POST /api/generate-text`, `POST /api/character/generate`,
  asset generation) return a structured JSON error body and a non-2xx status for
  hard failures.
- The router treats a whitespace-only model reply as a failed attempt and uses
  the configured fallback, instead of returning "success" with no text.
- The frontend surfaces generation errors visibly (inline on the button and in
  the existing per-form toast), never only to `console.error`.
- Turn generation records the same diagnostics in the trace and streams a
  structured failure to the client.
- Image generation rejects empty byte payloads instead of returning a zero-byte
  file.
- Every generation failure is exposed through OpenTelemetry: spans carry a
  bounded failure code and an error status, and counters/histograms record
  failure kind, rate, and latency.

**Non-Goals:**

- Changing provider transport implementations beyond wrapping their errors into
  the shared failure codes.
- Automatic retries beyond the existing role-fallback chain.
- Introducing a global toast provider framework; existing per-form toast state
  and a self-contained inline error on the button are sufficient.
- Changing turn recovery/trimming behaviour (`recoverReply`, `classifyCut`);
  this spec only makes its outcomes diagnosable.
- Making trace capture mandatory. Trace stays opt-in; failures are additionally
  reported to the client and to the (nil-safe) logger.

**Success Criteria:**

- With a provider configured to return an empty completion, clicking an AI
  generate button shows an error naming the reason instead of a spinner that
  stops.
- The trace contains a `generate.request`/`generate.attempt`/`generate.error`
  (or `generate.complete`) event for one-shot generation, including the failure
  code and per-role attempts.
- A stalled or timed-out turn reports `timeout` with the provider and elapsed
  time, both in the trace and in the streamed `error` event.
- `POST /api/generate-text` with a provider that errors returns `502` and a JSON
  body whose `error.code` is `provider_error`; an empty completion returns `502`
  with `error.code == "empty_response"`; unparseable text returns `422` with
  `parse_error`.
- Asset preview with an image client that yields zero bytes returns an error; no
  `.webp`/`octet-stream` empty file is returned.
- With telemetry enabled, a failed one-shot generation produces a `generate.text`
  span with `Status=Error` and `localrpg.generation.failure_code`, and increments
  `localrpg.generation.errors`.
- With telemetry enabled, a failed turn marks the root `turn` span as an error
  with `turn.outcome=error`, records the failure code on `provider.generate`, and
  is counted as a failed turn rather than a completed one.

## 2. Investigation Findings (Root Causes)

These are the concrete behaviours this spec corrects, all verified in the
current tree.

1. **One-shot generation never reports failure as an error.**
   `Service.GenerateText` returns `(resp, nil)` on every path; provider errors,
   `nil` results, whitespace-only text, and JSON parse failures all collapse to
   `200 {"fields":{},"generated_by":"none"}`. `Service.GenerateCharacter` does the
   same. The doc comments state this is intentional
   (`pkg/gui/text_generate.go:116-176`, `pkg/gui/character_generate.go:56-114`).
2. **The router never falls back on empty text.** `Router.GenerateForRole`
   returns immediately when `err == nil`, even when the response text is empty,
   so a model that answers `200 ""` defeats the configured fallback
   (`pkg/harness/router.go:71-96`).
3. **Providers treat absence of content as success.** `openaichat`, `geminillm`,
   and `clillm` all return `(&GenerateResponse{Text: ""}, nil)` on an empty
   completion; only transport/HTTP/exit-code failures produce an error
   (`pkg/provider/openaichat/http.go:173-194`,
   `pkg/provider/geminillm/provider.go:429-472`,
   `pkg/provider/clillm/cli.go:75-99`).
4. **Parse failure is indistinguishable from empty reply.**
   `decodeGeneratedValues` returns an empty map on JSON parse failure, so the
   caller cannot tell "the model said nothing" from "the model said something
   unparseable" (`pkg/gui/character_generate.go:146-178`).
5. **No generation-level trace for one-shot endpoints.** Providers emit
   `provider.request`/`provider.response`/`provider.error`, but nothing records
   the assembled prompt, the role fallback, the decode outcome, or
   `GeneratedBy == "none"`. Asset generation emits no trace events at all
   (`pkg/gui/text_generate.go`, `pkg/gui/character_generate.go`,
   `pkg/gui/service.go:2687-2778`).
6. **Turn failures carry thin diagnostics.** `generation.error` records only
   `error` and sometimes `finish_reason`; the streamed `TurnEvent{Type:"error"}`
   carries only `Message`. There is no code, provider id, prompt/context size, or
   elapsed time (`pkg/engine/orchestrator.go:573-609`, `pkg/gui/server.go:1063-1065`).
7. **Empty image bytes are returned as a file.** `GenerateAssetPreview` and the
   persisted asset methods do not check `len(imgBytes) > 0`; a zero-byte result
   becomes a `.webp`/`application/octet-stream` response
   (`pkg/gui/service.go:2687-2778`, `pkg/media/image.go:115-130`).
8. **The frontend has no error surface.** `AIGenerateButton` only calls
   `console.error` on a thrown error and does nothing when the field is missing
   from a successful response (`frontend/src/components/ui/AIGenerateButton.tsx:49-58`).
   `NewCampaignModal.handleGenerateAll` likewise only `console.error`s
   (`frontend/src/components/launcher/NewCampaignModal.tsx:144-146`), and
   `WorldsStudio`/`SystemsStudio` auto-fill set a success toast unconditionally.

Note: the On-Demand Text Generation Design (2026-09-24) section 6 already claimed
400/503 responses and a frontend error toast. That behaviour was never built.
This spec supersedes that section.

## 3. Error Taxonomy

A single code enum is used by the harness, the GUI service, the trace, and the
frontend.

| Code                   | Meaning                                                        | HTTP (one-shot) |
|------------------------|----------------------------------------------------------------|-----------------|
| `provider_unavailable` | No role could be built (disabled/unconfigured/registry miss).  | 503             |
| `provider_error`       | Provider returned a non-nil error.                             | 502             |
| `empty_response`       | Provider returned success with whitespace-only text.          | 502             |
| `parse_error`          | Text returned but no usable fields could be decoded.          | 422             |
| `timeout`              | Deadline exceeded or stream stalled (`ErrGenerationStalled`).  | 504             |
| `context_too_large`    | Provider reported a context/token limit.                       | 413             |
| `invalid_request`      | Malformed body, oversized body, missing required field.        | 400 (existing)  |

`empty_response` is decided by `strings.TrimSpace(text) == ""`. `context_too_large`
is detected by classifying the provider error message (case-insensitive match on
`context length`, `too many tokens`, `maximum context`, `token limit`); anything
not matching stays `provider_error`.

## 4. Architecture

A transport-neutral failure type lives in `pkg/harness` (the shared home for the
router and providers). The GUI service converts it to a DTO for JSON responses
and to `trace` events; the engine embeds it in turn results; the frontend decodes
it.

```go
// pkg/harness/failure.go

type FailureCode string

const (
    FailureProviderUnavailable FailureCode = "provider_unavailable"
    FailureProviderError       FailureCode = "provider_error"
    FailureEmptyResponse       FailureCode = "empty_response"
    FailureParseError          FailureCode = "parse_error"
    FailureTimeout             FailureCode = "timeout"
    FailureContextTooLarge     FailureCode = "context_too_large"
    FailureInvalidRequest      FailureCode = "invalid_request"
)

// Attempt records one provider invocation in a fallback chain.
type Attempt struct {
    Role       string      `json:"role"`
    Provider   string      `json:"provider"`
    Code       FailureCode `json:"code"`
    Detail     string      `json:"detail,omitempty"`
    DurationMS int64       `json:"duration_ms"`
}

// GenerationFailure is the single error shape for every generation path.
type GenerationFailure struct {
    Code         FailureCode `json:"code"`
    Message      string      `json:"message"`
    Attempts     []Attempt   `json:"attempts,omitempty"`
    FinishReason string      `json:"finish_reason,omitempty"`
    PromptChars  int         `json:"prompt_chars,omitempty"`
    ContextChars int         `json:"context_chars,omitempty"`
    ElapsedMS    int64       `json:"elapsed_ms,omitempty"`
}

func (f *GenerationFailure) Error() string { return f.Message }
```

Helpers:

- `func ClassifyProviderError(err error) FailureCode` — maps context-limit
  messages to `context_too_large`, deadlines to `timeout`, everything else to
  `provider_error`.
- `func FailureFrom(err error) (*GenerationFailure, bool)` — `errors.As` wrapper
  for callers that only have an `error`.
- `func (f *GenerationFailure) Summary() string` — "empty_response (gm via
  openai-chat): model returned no text" for logs/trace.

### 4.1 Router: empty is a failed attempt

`pkg/harness/router.go`:

- Add `func (r *Router) ProviderIDForRole(role string) string` (empty when the
  role is unassigned) so attempts and trace events can name the provider.
- `GenerateForRole` records an `Attempt` for the primary and each fallback. It
  treats `err != nil` **and** `strings.TrimSpace(res.Text) == ""` as failure. If
  a fallback exists it is tried; otherwise the method returns a
  `*GenerationFailure` carrying every attempt and the most informative code
  (`provider_unavailable` if the role was unbuildable, else the last attempt's
  code). Previously `GenerateForRole` had three call sites; only
  `text_generate.go` and `character_generate.go` remain (plus a test), so this
  semantics change is contained.
- `StreamForRole` gains the same attempt tracking and returns a
  `*GenerationFailure` on empty close, in addition to its existing first-chunk
  error handling.

### 4.2 One-shot text service

`Service.GenerateText` and `Service.GenerateCharacter` change from
"never returns an error" to "returns the raw result, plus a structured failure":

```go
type GenerateTextResponse struct {
    Fields      map[string]string    `json:"fields"`
    GeneratedBy string               `json:"generated_by"`
    Warning     *GenerationFailure   `json:"warning,omitempty"` // partial success only
}
```

- If the router cannot be built at all, return
  `nil, &GenerationFailure{Code: FailureProviderUnavailable, ...}`.
- For each attempted role, append an `Attempt`. A role that returns empty text,
  or whose text decodes to zero usable fields, is recorded as `empty_response`
  or `parse_error` respectively and the next role is tried.
- If no role produced fields, return `nil, lastFailure` (the handler maps the
  code to a status).
- If at least one field was produced but some requested work failed (relevant to
  `field_name: "_all"`), return the partial response with `Warning` populated and
  a `200`.
- Remove the doc comments asserting that failures are never errors, and correct
  them to describe the new contract.

`decodeGeneratedValues` gains a sibling
`decodeGeneratedValuesChecked(text) (map[string]string, error)` that returns
`ErrNoDecodableFields` on parse failure so the caller can record `parse_error`
rather than guessing. The existing function is kept for compatibility.

### 4.3 HTTP handlers

Add `func writeGenerationFailure(w http.ResponseWriter, f *harness.GenerationFailure)`
in `pkg/gui/server.go` (or a small `generation_errors.go`):

- Maps `f.Code` to the status in section 3 and writes
  `{"error":{"code":...,"message":...,"attempts":[...]}}` as JSON.
- Emits a `generate.error` trace event and a nil-safe logger event.
- Recognises `invalid_request` for existing `http.Error` sites by converting them
  to the same JSON shape where practical (turn body limits, malformed bodies).

`handleGenerateTextRoute` and the character handler call it when the service
returns a failure.

### 4.4 Turn path

`pkg/engine/orchestrator.go`:

- `streamResult` gains `ProviderID string` and `Failure *GenerationFailure`.
- `stream` sets `Failure.Code = FailureTimeout` on `ErrGenerationStalled` / ctx
  deadline, and `FailureEmptyResponse` on a clean close with no text.
- `generateRequest` records each fallback attempt and preserves the primary
  failure when the fallback also fails.
- `ProcessActionStream`:
  - `generation.error` includes `code`, `provider`, `role`, `finish_reason`,
    `prompt_chars`, `context_chars`, `elapsed_ms`, `chunk_count`, and
    `partial_chars`.
  - The empty-narration branch uses `FailureEmptyResponse` (or a recovery
    outcome) rather than a bare string.
  - `turnSpan.SetAttributes(attribute.String("localrpg.generation.failure_code", ...))`.
- `TurnEvent` for streaming gains optional `Code string`, `Detail string`, and
  `Failure *harness.GenerationFailure` fields; `Message` is retained for
  compatibility. `handleTurnSubmit` populates them from the returned error.

### 4.5 Image generation

`pkg/gui/service.go` (preview, game, world asset methods):

- After the image client returns, require `len(imgBytes) > 0` and a detectable
  content type; otherwise return
  `&harness.GenerationFailure{Code: FailureProviderError, Message: "image provider returned no data"}`.
- Wrap client construction and generation errors in the same type, preserving
  the existing detail text.
- Emit `image.request` (kind, prompt chars), `image.complete` (bytes, elapsed) or
  `image.error` (code, detail) trace events.

### 4.6 Trace and logging

Add a small helper in `pkg/gui` to emit generation events through the
nil-safe `trace.Logger`:

- `generate.request`: `form_type`, `field_name`, `roles`, `prompt_chars`,
  `context_keys`, `world_id`, `system_id`.
- `generate.attempt`: `role`, `provider`, `code`, `duration_ms`.
- `generate.complete`: `generated_by`, `field_count`, `response_chars`,
  `elapsed_ms`.
- `generate.error`: `code`, `message`, `attempts` (compact), plus the same
  sizing fields.

These are written to the existing rotating trace file
(`<cache>/trace/trace.jsonl`) and are also sent to the logger, so failures are
visible even when trace is off in the UI, provided the sink is configured.

## 5. OpenTelemetry Instrumentation

This section follows the OpenTelemetry Instrumentation Specification
(2026-09-24): dotted `noun.verb` span names, `localrpg.*` custom attributes with
`gen_ai.*`/`error.*`/`http.*` semantic-convention attributes where they exist, no
payload text on spans (`trace.Sanitize` is the single redaction point), `game.id`
on spans only, and `localrpg.<domain>.<measure>` metric names with bounded
attribute sets.

The failure code enum from section 3 is the bounded key that ties the trace, the
JSON response, and telemetry together. `localrpg.generation.failure_code` is the
canonical attribute name.

### 5.1 Spans

Two new spans cover the currently-uninstrumented one-shot paths; the turn path
extends existing spans.

| Span | Where | Attributes | Status on failure |
|------|-------|-----------|-------------------|
| `generate.text` | `pkg/gui` service (`GenerateText`, `GenerateCharacter`), child of `http.server` | `localrpg.form_type`, `localrpg.field_name`, `localrpg.generated_by`, `localrpg.field_count`, `localrpg.generation.attempts`, `gen_ai.system` | `SetStatus(codes.Error, code)` + `RecordError(*GenerationFailure)` |
| `generate.image` | `pkg/gui` service (`GenerateAssetPreview`, `GenerateGameAsset`, `GenerateWorldAsset`), child of `http.server` | `localrpg.image.kind`, `localrpg.image.provider`, `localrpg.image.bytes`, `localrpg.generation.failure_code` | `SetStatus(codes.Error, code)` + `RecordError` |
| `provider.generate` (existing) | `pkg/engine` `runGenerationLoop` | add `localrpg.generation.failure_code`, `localrpg.finish_reason` | already `RecordError`+`SetStatus`; status description becomes the bounded code |
| `turn` (existing root) | `pkg/engine` `ProcessActionStream` | add `localrpg.generation.failure_code`, `localrpg.generation.attempts`, `turn.outcome=error` | currently none; add `SetStatus`+`RecordError` |
| `http.server` (existing, otelhttp) | `pkg/gui` server | automatic | otelhttp records `http.response.status_code`; 5xx marks the span error automatically once handlers return 5xx |

Span tree for a one-shot request:

```
http.server                    otelhttp, routePattern route
└── generate.text              pkg/gui service
    └── http.client (auto)     telemetry.HTTPTransport outbound provider call
```

The `generate.text` span records one `attempt` event per role in the fallback
chain, so the primary/fallback history is visible without reading the JSONL
trace.

### 5.2 Span events

| Span | Event | Attributes |
|------|-------|-----------|
| `generate.text` | `attempt` | `localrpg.role`, `gen_ai.system`, `localrpg.generation.failure_code` (empty on success), `duration_ms` |
| `generate.text` | `fallback` | `localrpg.role`, `localrpg.role.next`, `localrpg.generation.failure_code` (why) |
| `generate.text` | `error` | `localrpg.generation.failure_code`, `localrpg.generation.failure_message` (sanitised, truncated) |
| `generate.image` | `attempt`, `error` | same shape as above with `localrpg.image.kind` |
| `provider.generate` | `attempt`, `fallback`, `error` | mirrors the above, alongside the existing `first_chunk`, `tool_calls`, `thinking`, `retry` events |

`fallback` overlaps the OTel spec's existing `retry` event on
`provider.generate`; recommendation: keep `retry` for streaming rounds and use
`fallback` for the non-streaming router chain, or unify them in the plan.

### 5.3 Attributes and cardinality

- `localrpg.generation.failure_code` is the bounded enum from section 3 and is
  the only generation-failure attribute on spans. The free-form message stays in
  a span event and in the log bridge, after `trace.Sanitize`.
- `localrpg.generation.attempts` is an int count, never the list of attempts.
- `gen_ai.system` comes from the attempt's provider, so failures are attributable
  to a provider, matching the OTel spec's GenAI conventions.
- `localrpg.form_type` is bounded (`character`, `world`, `system`, `campaign`,
  `image`) and safe as a metric attribute.
- `game.id` is never added to these metrics; it stays on spans only.

### 5.4 Error recording and turn outcome

- Every generation failure return calls `span.RecordError(err)` with the
  `*GenerationFailure` and `span.SetStatus(codes.Error, string(code))`. The
  status description is the bounded code, not the free-form message, so status
  text does not explode cardinality.
- The `turn` root span is currently never marked on a generation failure
  (`pkg/engine/orchestrator.go:573-577`), and the deferred block still records
  `turnCompleted` with an empty `turn.outcome`. This spec fixes both:
  `turn.outcome` becomes `error` on the failure return, the span gets
  `SetStatus`/`RecordError`, and `localrpg.turn.completed` is recorded with the
  `turn.outcome` attribute so failures are separable rather than counted as
  completed turns.
- Extractor failures are currently swallowed on the span
  (`pkg/engine/orchestrator.go:632-640` only records a count when `err == nil`).
  A failed extractor is a generation failure, so record an `error` span event
  with the failure code and call `RecordError` when the extractor returns one.
- `http.server` spans are handled automatically by `otelhttp`; the handlers'
  move to non-2xx statuses for hard failures (section 4.3) is what makes 5xx
  generation failures visible there.

### 5.5 Metrics

Amendments to the catalogue in the OpenTelemetry Instrumentation Specification:
`localrpg.provider.errors` `error.kind` takes the section-3 codes instead of the
hardcoded `"provider"`, and three generation metrics are added.

| Metric | Kind | Unit | Attributes |
|--------|------|------|------------|
| `localrpg.generation.errors` | counter | 1 | `localrpg.form_type`, `localrpg.generation.failure_code`, `localrpg.role` |
| `localrpg.generation.duration` | histogram | ms | `localrpg.form_type`, `localrpg.generation.outcome` (success/failure), `localrpg.role` |
| `localrpg.provider.fallbacks` | counter | 1 | `localrpg.role`, `localrpg.generation.failure_code` (why the fallback engaged) |

- `localrpg.provider.errors` (existing): `error.kind` is set from the failure
  code (`provider_error`, `empty_response`, `timeout`, `context_too_large`,
  `provider_unavailable`, `parse_error`) and `gen_ai.system` is added, matching
  the OTel spec's intended attribute set.
- `localrpg.turn.duration` and `localrpg.turn.completed` (existing): recorded
  with `turn.outcome` so failed turns are visible in both.
- `localrpg.media.image.duration` (spec'd, unbuilt): record for `generate.image`;
  failures additionally increment `localrpg.generation.errors` with
  `localrpg.form_type=image`.
- Bucket boundaries reuse the OTel spec's latency set. Attribute sets stay
  bounded; no attempt details or messages become metric attributes.

### 5.6 Log bridge

The `generate.request`/`generate.attempt`/`generate.complete`/`generate.error`
events from section 4.6 already flow through the nil-safe `trace.Logger`. The
telemetry bridge (`pkg/telemetry/bridge.go`) fans them out to OTel log records
and span events. Where `ctx` is available (the GUI service and the orchestrator
both hold it), use the `EventCtx` variant so the event attaches to the active
`generate.text`/`turn` span; the JSONL trace remains the payload-rich record and
`trace.Sanitize` remains the single redaction point.

### 5.7 Wiring and lifecycle

- `pkg/gui` gets cached instruments following the existing pattern
  (`pkg/engine/metrics.go:30-47`, `pkg/harness/metrics.go:24-37`): a
  `generationMetrics` struct rebuilt when the global `MeterProvider` changes,
  with a tracer from `telemetry.Tracer("github.com/darkliquid/localrpg")` and a
  meter from `telemetry.Meter(telemetry.MeterName)`.
- The GUI `Service` already holds a `trace.Logger`; no new construction seam is
  needed because `telemetry.Tracer`/`Meter` resolve the global provider at call
  time, and a disabled provider returns no-ops.
- The engine reads the same failure code from `streamResult.Failure`, so the
  orchestrator and the GUI service share one enum.
- No new config. The existing `telemetry` block gates everything; with telemetry
  off all of this is no-op and behaviour is limited to the JSON response, trace
  events, and frontend errors.

### 5.8 Testing

All OTel tests use `telemetry.NewInMemory()` and assert without a network:

- A `GenerateText` call with an empty provider response produces a `generate.text`
  span with `Status=Error`, `localrpg.generation.failure_code=empty_response`,
  two `attempt` events, and a `fallback` event; `localrpg.generation.errors` is
  incremented with the same code; `localrpg.provider.errors` gets
  `error.kind=empty_response`.
- A `GenerateText` partial success produces a span with `Status=Unset`, no error
  event, and `localrpg.generation.outcome=success` on the duration histogram.
- A failed turn asserts the root `turn` span has `Status=Error` and
  `turn.outcome=error`, that `localrpg.turn.completed` carries
  `turn.outcome=error`, and that `provider.generate` has the failure code.
- An image generation that yields zero bytes asserts `generate.image` has
  `Status=Error` and `localrpg.generation.errors{localrpg.form_type=image}`.
- With telemetry disabled, none of the above opens a connection; the existing
  no-op tests still pass.

Cross-spec note: the metric table, span hierarchy, span attributes, span events,
and error-recording section of the OpenTelemetry Instrumentation Specification
(2026-09-24) have been amended to include `localrpg.generation.errors`,
`localrpg.generation.duration`, the `generate.text`/`generate.image` spans, and
the amended `localrpg.provider.errors`/`localrpg.provider.fallbacks` rows, so
the two specs stay consistent.

## 6. Frontend Changes

### 6.1 Types (`frontend/src/types.ts`)

```ts
export type GenerationFailureCode =
  | 'provider_unavailable' | 'provider_error' | 'empty_response'
  | 'parse_error' | 'timeout' | 'context_too_large' | 'invalid_request';

export interface GenerationAttempt {
  role: string; provider: string; code: GenerationFailureCode;
  detail?: string; duration_ms: number;
}

export interface GenerationFailure {
  code: GenerationFailureCode; message: string;
  attempts?: GenerationAttempt[]; finish_reason?: string;
  prompt_chars?: number; context_chars?: number; elapsed_ms?: number;
}

export interface GenerateTextResponse {
  fields: Record<string, string>;
  generated_by: string;
  warning?: GenerationFailure;
}
```

### 6.2 API client (`frontend/src/api/client.ts`)

- Add `class GenerationError extends HTTPError` carrying the parsed
  `GenerationFailure`. When a response is non-2xx, attempt to parse
  `{"error":{...}}`; fall back to the raw text.
- `generateText` throws `GenerationError` on non-2xx and returns the body
  otherwise (so callers can read `warning`).
- `generateCharacter` parses the JSON error body instead of using
  `res.statusText`.
- `generateAssetPreview`, `generateGameAsset`, `generateWorldAsset` throw
  `GenerationError` when the body is JSON-shaped.

### 6.3 `AIGenerateButton`

- New optional prop `onError?: (failure: GenerationFailure) => void`.
- On success with a value: `onGenerated(value)` as today.
- On success without the expected field: synthesise a failure from
  `res.warning` or a default `{ code: 'empty_response', ... }`, invoke `onError`
  and show an inline error.
- On thrown error: coerce to `GenerationFailure` (via `GenerationError`, or a
  generic message), invoke `onError`, show inline error.
- Inline error: render a small red `AlertCircle` badge adjacent to the button
  with a title/tooltip containing the code and message, auto-clearing after a few
  seconds. This keeps the component usable in every form without a global toast.

### 6.4 Call sites

- `NewCampaignModal.handleGenerateAll`: collect `warning` from the campaign and
  character responses; on thrown errors and on warnings, call
  `setGenError(message)` (the existing error UI already renders `genError`).
- `WorldsStudio` auto-fill and `SystemsStudio` auto-fill: replace the
  unconditional success toast with an error toast when a warning/failure is
  returned or the call throws, and include the failure code in the message.
- Individual `AIGenerateButton` users pass `onError` to feed the form's existing
  `setToast({ type: 'error', ... })`.
- Turn UI: render `TurnEvent.Failure`/`Code` in the error banner so a failed turn
  shows the reason instead of a bare message.

## 7. Error Handling Summary

| Situation                        | Backend                                             | Frontend                                   |
|----------------------------------|-----------------------------------------------------|--------------------------------------------|
| Provider not configured/disabled | 503 `provider_unavailable`                          | Inline error + form toast                  |
| Provider network/transport error | 502 `provider_error` (detail retained)              | Inline error + form toast                  |
| Empty completion                 | 502 `empty_response` (router falls back first)      | Inline error + form toast                  |
| Unparseable completion           | 422 `parse_error`                                   | Inline error + form toast                  |
| Timeout / stalled stream         | 504 `timeout` (turn: streamed failure)              | Turn error banner with reason              |
| Context too large                | 413 `context_too_large`                             | Message advises shortening the input       |
| Partial `_all` success           | 200 with `warning`                                  | Error toast, generated fields retained     |
| Empty image bytes                | `provider_error` "returned no data"                 | Asset preview `genError` message           |

## 8. Testing & Verification

**Backend (standard library only, `t.TempDir`):**

- Router: primary returns empty/nil -> fallback invoked; both empty ->
  `*GenerationFailure` with code `empty_response` and two attempts.
- Router: primary returns a context-limit error -> `context_too_large`.
- `GenerateText`: provider errors -> `provider_error`; empty -> `empty_response`;
  unparseable JSON -> `parse_error`; `_all` with one decodable field -> `200`
  plus `Warning`.
- `GenerateCharacter`: same shape.
- Handler: each code maps to the status in section 3 and the body decodes into
  the failure shape.
- Asset preview: injected zero-byte image client -> error, no file written.
- Turn: empty narration records `generation.error` with `code == empty_response`;
  a stalled stream records `timeout`; the streamed `error` event carries the
  failure.
- `TraceEvents` returns the new `generate.*` events for a one-shot call.

**Frontend:**

- `mise run test:frontend` (`tsc --noEmit`) must pass; there is no frontend unit
  runner, so behaviour is verified manually.
- Manual: set the `gm` role (or `character`/`world` roles) to a provider that
  returns empty text (a `cli` provider running a command that prints nothing),
  click an AI generate button, and confirm an inline error and form toast appear
  naming `empty_response`.
- Manual: point the role at a failing command and confirm the message names
  `provider_error` and the underlying detail.
- Manual: run a turn with a provider that stalls and confirm the error banner
  names the timeout and the trace contains `generation.error` with the sizing
  fields.

## 9. Compatibility & Migration

- `POST /api/character/generate` now returns non-2xx on failure instead of a
  `200` empty body. This is a deliberate correction to the never-implemented
  section 6 of the On-Demand Text Generation Design. Any caller relying on the
  old silent-empty behaviour must read `error` instead. Both endpoints are
  versioned by usage, not by URL, so no route changes.
- The trace event names are additive; existing consumers of `provider.*` and
  `generation.complete` keep working.
- Go code uses `interface{}` (not `any`) per project convention.

## 10. Open Questions

- Should failed one-shot generations be retried once at the router level before
  reporting, or is single-pass fallback chain sufficient? (Current proposal:
  fallback chain only.)
- Should `context_too_large` trigger automatic prompt trimming for one-shot
  generation, or only report? (Current proposal: report only; trimming is turn
  logic and out of scope.)
