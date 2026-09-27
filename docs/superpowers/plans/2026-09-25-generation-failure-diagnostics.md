# Generation Failure Diagnostics Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every generation failure visible and explained, end to end: a shared failure contract in the backend, structured JSON errors and non-2xx statuses from the one-shot endpoints, richer turn diagnostics, OpenTelemetry spans/metrics/logs, and a visible error surface in the frontend instead of silent spinners.

**Architecture:** A transport-neutral `harness.GenerationFailure` with a bounded `FailureCode` enum is the single error shape. The router treats whitespace-only model text as a failed attempt and falls back, recording one `Attempt` per role. The GUI service returns the failure to HTTP handlers, which map the code to a status and a JSON `{"error":{...}}` body, and to an OpenTelemetry span/metric pair. The engine embeds the same type in `streamResult` so a failed turn is recorded on the `turn` span and streamed to the client. The frontend decodes the failure into a `GenerationError`, and `AIGenerateButton` plus the form-level callers show it.

**Tech Stack:** Go 1.27.1 (`net/http`, `pkg/harness`, `pkg/gui`, `pkg/engine`, `go.opentelemetry.io/otel`), React 19 + TypeScript + Tailwind v4, Lucide icons (`AlertCircle`).

**Spec:** `docs/superpowers/specs/2026-09-25-generation-failure-diagnostics-design.md`

## Global Constraints

- Go 1.27.1. Standard library only for tests (`testing`, `t.TempDir()`); no testify.
- Use `interface{}`, not `any`; wrap errors with `fmt.Errorf("...: %w", err)`; `go vet ./...` must stay clean.
- Failure codes are the fixed enum `provider_unavailable`, `provider_error`, `empty_response`, `parse_error`, `timeout`, `context_too_large`, `invalid_request`.
- `empty_response` means `strings.TrimSpace(text) == ""`.
- Trace is off by default and telemetry is off by default; both must be no-ops when disabled.
- Payload text (prompts, narration, raw messages) is never a span attribute; `trace.Sanitize` is the only redaction point. `game.id` is never a metric attribute.
- OpenTelemetry attributes use `localrpg.*` except semantic conventions (`gen_ai.*`, `error.*`, `http.*`); the bounded failure attribute is `localrpg.generation.failure_code`. Metric names are `localrpg.<domain>.<measure>`.
- TypeScript: `strict`, `noUnusedLocals`, `noUnusedParameters`; `npx tsc --noEmit` is the frontend gate.
- Conventional Commits with a scope; subject under 72 characters.

---

## File Map

**Create**
- `pkg/harness/failure.go` — failure code enum, `Attempt`, `GenerationFailure`, classification helpers.
- `pkg/harness/failure_test.go` — classification and wrapping tests.
- `pkg/harness/router_failure_test.go` — empty-as-failure and fallback tests.
- `pkg/gui/generation_telemetry.go` — cached generation instruments, span/event/metric recording helper.
- `pkg/gui/generation_errors.go` — `writeGenerationFailure` HTTP mapping.
- `pkg/gui/generation_errors_test.go` — status and body mapping tests.
- `pkg/gui/generation_telemetry_test.go` — in-memory OTel assertions.
- `pkg/gui/text_generate_failure_test.go` — `GenerateText` failure paths.
- `pkg/gui/character_generate_failure_test.go` — `GenerateCharacter` failure paths.
- `pkg/gui/asset_generate_failure_test.go` — empty image bytes guard.

**Modify**
- `pkg/harness/router.go` — `ProviderIDForRole`, empty-as-failure, attempt tracking.
- `pkg/gui/character_generate.go` — `ErrNoDecodableFields`, `decodeGeneratedValuesChecked`, `GenerateCharacter` returns failures.
- `pkg/gui/text_generate.go` — `Warning` field, `GenerateText` returns failures, record telemetry.
- `pkg/gui/server.go` — `handleGenerateTextRoute`, `handleCharacterGenerateRoute`, `handleGenerateAssetPreview` call `writeGenerationFailure`.
- `pkg/gui/service.go` — `GenerateAssetPreview`/`GenerateGameAsset`/`GenerateWorldAsset` validate image bytes and record telemetry.
- `pkg/gui/types.go` — `TurnEvent` gains `Code`, `Detail`, `Failure`.
- `pkg/engine/orchestrator.go` — `streamResult.Failure`/`ProviderID`, stream/generateRequest/runGenerationLoop/ProcessActionStream diagnostics, turn span error status.
- `frontend/src/types.ts` — `GenerationFailureCode`, `GenerationAttempt`, `GenerationFailure`; `GenerateTextResponse.warning`.
- `frontend/src/api/client.ts` — `GenerationError`; parse JSON error bodies.
- `frontend/src/components/ui/AIGenerateButton.tsx` — `onError`, inline error.
- `frontend/src/components/launcher/NewCampaignModal.tsx` — surface generate-all failures.
- `frontend/src/components/WorldsStudio.tsx` — auto-fill and `AIGenerateButton` error toasts.
- `frontend/src/components/SystemsStudio.tsx` — auto-fill and `AIGenerateButton` error toasts.

---

### Task 1: Shared failure type

**Files:**
- Create: `pkg/harness/failure.go`
- Test: `pkg/harness/failure_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `harness.FailureCode` constants; `harness.Attempt`; `harness.GenerationFailure` (implements `error`); `harness.ClassifyProviderError(error) FailureCode`; `harness.FailureFrom(error) (*GenerationFailure, bool)`; `harness.NewFailure(code, message, role, provider string, elapsed time.Duration) *GenerationFailure`.

- [x] **Step 1: Write the failing test**

Create `pkg/harness/failure_test.go`:
```go
package harness

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestClassifyProviderError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want FailureCode
	}{
		{"deadline", context.DeadlineExceeded, FailureTimeout},
		{"wrapped deadline", fmt.Errorf("call: %w", context.DeadlineExceeded), FailureTimeout},
		{"context length", errors.New("This model's maximum context length is 8192 tokens"), FailureContextTooLarge},
		{"too many tokens", errors.New("too many tokens requested"), FailureContextTooLarge},
		{"generic", errors.New("connection reset by peer"), FailureProviderError},
		{"nil", nil, FailureProviderError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyProviderError(tt.err); got != tt.want {
				t.Fatalf("ClassifyProviderError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFailureFrom(t *testing.T) {
	want := &GenerationFailure{Code: FailureEmptyResponse, Message: "empty"}
	got, ok := FailureFrom(fmt.Errorf("role failed: %w", want))
	if !ok || got != want {
		t.Fatalf("FailureFrom() = %v, %v; want the original failure", got, ok)
	}
	if _, ok := FailureFrom(errors.New("plain")); ok {
		t.Fatal("FailureFrom() found a failure in a plain error")
	}
}

func TestGenerationFailureError(t *testing.T) {
	var nilFailure *GenerationFailure
	if nilFailure.Error() != "" {
		t.Fatal("nil failure should have an empty message")
	}
	if (&GenerationFailure{Message: "boom"}).Error() != "boom" {
		t.Fatal("failure message not returned by Error()")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestClassifyProviderError|TestFailureFrom|TestGenerationFailureError' ./pkg/harness/`
Expected: FAIL with `undefined: FailureCode`, `undefined: GenerationFailure`, etc.

- [x] **Step 3: Write the implementation**

Create `pkg/harness/failure.go`:
```go
package harness

import (
	"context"
	"errors"
	"strings"
	"time"
)

// FailureCode is the bounded reason a generation failed. It is the single key
// shared by the JSON response, the trace, the OpenTelemetry spans, and the
// frontend, so a failure is never described by an arbitrary string.
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

func (f *GenerationFailure) Error() string {
	if f == nil {
		return ""
	}
	return f.Message
}

// ClassifyProviderError maps an arbitrary provider error to a bounded code. The
// classification is textual because providers surface their own error types, and
// a context-length failure is the one case a user can act on.
func ClassifyProviderError(err error) FailureCode {
	if err == nil {
		return FailureProviderError
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureTimeout
	}
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{
		"context length", "maximum context", "too many tokens", "token limit", "context window",
	} {
		if strings.Contains(lower, marker) {
			return FailureContextTooLarge
		}
	}
	return FailureProviderError
}

// FailureFrom extracts a *GenerationFailure from an error chain.
func FailureFrom(err error) (*GenerationFailure, bool) {
	var failure *GenerationFailure
	if errors.As(err, &failure) {
		return failure, true
	}
	return nil, false
}

// NewFailure builds a failure with a single attempt.
func NewFailure(code FailureCode, message, role, provider string, elapsed time.Duration) *GenerationFailure {
	return &GenerationFailure{
		Code:      code,
		Message:   message,
		ElapsedMS: elapsed.Milliseconds(),
		Attempts: []Attempt{{
			Role:       role,
			Provider:   provider,
			Code:       code,
			DurationMS: elapsed.Milliseconds(),
		}},
	}
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestClassifyProviderError|TestFailureFrom|TestGenerationFailureError' ./pkg/harness/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/harness/failure.go pkg/harness/failure_test.go
git commit -m "feat(harness): add a shared generation failure type"
```

---

### Task 2: Router treats empty text as a failure

**Files:**
- Modify: `pkg/harness/router.go:55-96`
- Test: `pkg/harness/router_failure_test.go`

**Interfaces:**
- Consumes: `harness.GenerationFailure`, `harness.Attempt`, `harness.ClassifyProviderError` from Task 1.
- Produces: `(*Router).ProviderIDForRole(role string) string`; `GenerateForRole` now returns a `*GenerationFailure` when every attempt produces no usable text.

- [x] **Step 1: Write the failing test**

Create `pkg/harness/router_failure_test.go`:
```go
package harness

import (
	"context"
	"testing"
)

type stubProvider struct {
	id    string
	text  string
	err   error
	calls int
}

func (s *stubProvider) ID() string { return s.id }

func (s *stubProvider) Generate(context.Context, GenerateRequest) (*GenerateResponse, error) {
	s.calls++
	return &GenerateResponse{Text: s.text}, s.err
}

func (s *stubProvider) Stream(context.Context, GenerateRequest, chan<- StreamChunk) error { return nil }

func TestGenerateForRoleFallsBackOnEmpty(t *testing.T) {
	primary := &stubProvider{id: "primary", text: ""}
	fallback := &stubProvider{id: "fallback", text: "hello"}
	router := NewRouter()
	router.RegisterProvider(primary)
	router.RegisterProvider(fallback)
	router.AssignRole("gm", "primary")
	router.SetFallback("gm", "fallback")

	res, err := router.GenerateForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("GenerateForRole returned an error: %v", err)
	}
	if res.Text != "hello" {
		t.Fatalf("GenerateForRole text = %q, want the fallback text", res.Text)
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("calls = primary %d fallback %d, want both once", primary.calls, fallback.calls)
	}
}

func TestGenerateForRoleEmptyEverywhereIsFailure(t *testing.T) {
	router := NewRouter()
	router.RegisterProvider(&stubProvider{id: "primary", text: "   "})
	router.RegisterProvider(&stubProvider{id: "fallback", text: ""})
	router.AssignRole("gm", "primary")
	router.SetFallback("gm", "fallback")

	_, err := router.GenerateForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"})
	failure, ok := FailureFrom(err)
	if !ok {
		t.Fatalf("error = %v, want a *GenerationFailure", err)
	}
	if failure.Code != FailureEmptyResponse {
		t.Fatalf("code = %q, want %q", failure.Code, FailureEmptyResponse)
	}
	if len(failure.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(failure.Attempts))
	}
}

func TestGenerateForRoleUnassignedIsUnavailable(t *testing.T) {
	_, err := NewRouter().GenerateForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"})
	failure, ok := FailureFrom(err)
	if !ok || failure.Code != FailureProviderUnavailable {
		t.Fatalf("error = %v, want provider_unavailable", err)
	}
}

func TestProviderIDForRole(t *testing.T) {
	router := NewRouter()
	router.RegisterProvider(&stubProvider{id: "p1", text: "x"})
	router.AssignRole("gm", "p1")
	if got := router.ProviderIDForRole("gm"); got != "p1" {
		t.Fatalf("ProviderIDForRole(gm) = %q, want p1", got)
	}
	if got := router.ProviderIDForRole("other"); got != "" {
		t.Fatalf("ProviderIDForRole(other) = %q, want empty", got)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestGenerateForRole|TestProviderIDForRole' ./pkg/harness/`
Expected: FAIL (`TestGenerateForRoleFallsBackOnEmpty` gets empty success, `TestProviderIDForRole` undefined).

- [x] **Step 3: Implement the router change**

In `pkg/harness/router.go`, add `strings` and `time` to the imports, then replace `GenerateForRole` (currently lines 71-96) with:
```go
// ProviderIDForRole names the provider assigned to a role, or "" when none is.
func (r *Router) ProviderIDForRole(role string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.roleMap[role]
}

// attemptOutcome pairs a provider result with the bounded reason it is unusable.
func attemptOutcome(res *GenerateResponse, err error) (FailureCode, string) {
	if err != nil {
		return ClassifyProviderError(err), err.Error()
	}
	if res == nil || strings.TrimSpace(res.Text) == "" {
		return FailureEmptyResponse, "model returned no text"
	}
	return "", ""
}

// GenerateForRole streams one role's reply, treating a whitespace-only response
// as a failed attempt and falling back, so a model that answers 200 "" cannot
// silently defeat the configured fallback.
func (r *Router) GenerateForRole(ctx context.Context, role string, req GenerateRequest) (*GenerateResponse, error) {
	primary, err := r.GetProviderForRole(role)
	if err != nil {
		return nil, &GenerationFailure{
			Code:    FailureProviderUnavailable,
			Message: fmt.Sprintf("no provider available for role %q: %v", role, err),
		}
	}

	attempts := make([]Attempt, 0, 2)
	started := time.Now()

	primaryStarted := time.Now()
	res, callErr := primary.Generate(ctx, req)
	if code, _ := attemptOutcome(res, callErr); code == "" {
		return res, nil
	} else {
		_, detail := attemptOutcome(res, callErr)
		attempts = append(attempts, Attempt{
			Role: role, Provider: primary.ID(), Code: code, Detail: detail,
			DurationMS: time.Since(primaryStarted).Milliseconds(),
		})
	}

	if fallback, ok := r.FallbackForRole(role); ok && fallback != nil {
		fallbackStarted := time.Now()
		fbRes, fbErr := fallback.Generate(ctx, req)
		if code, _ := attemptOutcome(fbRes, fbErr); code == "" {
			return fbRes, nil
		} else {
			_, detail := attemptOutcome(fbRes, fbErr)
			attempts = append(attempts, Attempt{
				Role: role, Provider: fallback.ID(), Code: code, Detail: detail,
				DurationMS: time.Since(fallbackStarted).Milliseconds(),
			})
		}
	}

	return nil, &GenerationFailure{
		Code:      attempts[len(attempts)-1].Code,
		Message:   fmt.Sprintf("role %q produced no usable response", role),
		Attempts:  attempts,
		ElapsedMS: time.Since(started).Milliseconds(),
	}
}
```
Leave `StreamForRole` unchanged.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestGenerateForRole|TestProviderIDForRole|TestRouter' ./pkg/harness/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/harness/router.go pkg/harness/router_failure_test.go
git commit -m "fix(harness): treat an empty model reply as a failed attempt"
```

---

### Task 3: Make decode failure explicit

**Files:**
- Modify: `pkg/gui/character_generate.go:146-178`
- Test: `pkg/gui/character_generate_failure_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `gui.ErrNoDecodableFields`; `gui.decodeGeneratedValuesChecked(text string) (map[string]string, error)`.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/character_generate_failure_test.go`:
```go
package gui

import (
	"errors"
	"testing"
)

func TestDecodeGeneratedValuesChecked(t *testing.T) {
	values, err := decodeGeneratedValuesChecked(`{"name":"Vela","age":31}`)
	if err != nil {
		t.Fatalf("decodeGeneratedValuesChecked returned an error: %v", err)
	}
	if values["name"] != "Vela" || values["age"] != "31" {
		t.Fatalf("values = %v, want name and age", values)
	}

	if _, err := decodeGeneratedValuesChecked("I am afraid I cannot do that."); !errors.Is(err, ErrNoDecodableFields) {
		t.Fatalf("err = %v, want ErrNoDecodableFields", err)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestDecodeGeneratedValuesChecked ./pkg/gui/`
Expected: FAIL (`undefined: decodeGeneratedValuesChecked`).

- [x] **Step 3: Implement**

In `pkg/gui/character_generate.go`, add `errors` to the imports and append after `decodeGeneratedValues`:
```go
// ErrNoDecodableFields reports that a model returned text from which no field
// values could be decoded. It is distinct from an empty reply.
var ErrNoDecodableFields = errors.New("no decodable fields in model response")

// decodeGeneratedValuesChecked is decodeGeneratedValues with the parse failure
// made explicit, so a caller can record parse_error rather than guessing.
func decodeGeneratedValuesChecked(text string) (map[string]string, error) {
	values := decodeGeneratedValues(text)
	if len(values) == 0 {
		return nil, ErrNoDecodableFields
	}
	return values, nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestDecodeGeneratedValuesChecked ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/character_generate.go pkg/gui/character_generate_failure_test.go
git commit -m "refactor(gui): make generation parse failures explicit"
```

---

### Task 4: Generation telemetry helper

**Files:**
- Create: `pkg/gui/generation_telemetry.go`
- Test: `pkg/gui/generation_telemetry_test.go`

**Interfaces:**
- Consumes: `harness.GenerationFailure`; `telemetry.Int64Counter`, `telemetry.Float64Histogram`, `telemetry.MeterName`; `trace.OrNil`, `trace.LogEvent`.
- Produces:
  - `generationMetrics() generationInstruments`
  - `(*Service).startGenerationSpan(ctx context.Context, name, formType, fieldName string) (context.Context, oteltrace.Span)`
  - `(*Service).recordGeneration(ctx context.Context, span oteltrace.Span, formType string, started time.Time, failure *harness.GenerationFailure)`
  - `(*Service).recordGenerationAttempts(ctx context.Context, span oteltrace.Span, attempts []harness.Attempt)`
  - `outcomeLabel(failure *harness.GenerationFailure) string`

- [x] **Step 1: Write the failing test**

Create `pkg/gui/generation_telemetry_test.go`:
```go
package gui

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

func TestRecordGenerationFailureEmitsSpanAndMetric(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	service := &Service{}
	ctx, span := service.startGenerationSpan(context.Background(), "generate.text", "world", "lore_prompt")
	failure := &harness.GenerationFailure{Code: harness.FailureEmptyResponse, Message: "no text"}
	service.recordGeneration(ctx, span, "world", time.Now(), failure)
	span.End()

	spans := recorder.Spans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	var found bool
	for _, attr := range spans[0].Attributes() {
		if attr.Key == attribute.Key("localrpg.generation.failure_code") && attr.Value.AsString() == "empty_response" {
			found = true
		}
	}
	if !found {
		t.Fatal("span is missing the localrpg.generation.failure_code attribute")
	}

	rm, err := recorder.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if len(rm.ScopeMetrics) == 0 || len(rm.ScopeMetrics[0].Metrics) == 0 {
		t.Fatal("no generation metrics were recorded")
	}
}

func TestRecordGenerationAttemptsCountsFallbacks(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	service := &Service{}
	ctx, span := service.startGenerationSpan(context.Background(), "generate.text", "world", "_all")
	service.recordGenerationAttempts(ctx, span, []harness.Attempt{
		{Role: "character", Provider: "p1", Code: harness.FailureEmptyResponse},
		{Role: "gm", Provider: "p2", Code: harness.FailureEmptyResponse},
	})
	span.End()

	if spans := recorder.Spans(); len(spans) != 1 || len(spans[0].Events()) != 2 {
		t.Fatalf("span events = %v, want one attempt and one fallback", spans)
	}
}

func TestOutcomeLabel(t *testing.T) {
	if got := outcomeLabel(nil); got != "success" {
		t.Fatalf("outcomeLabel(nil) = %q, want success", got)
	}
	if got := outcomeLabel(&harness.GenerationFailure{Code: harness.FailureTimeout}); got != "failure" {
		t.Fatalf("outcomeLabel(failure) = %q, want failure", got)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestRecordGenerationFailureEmitsSpanAndMetric|TestOutcomeLabel' ./pkg/gui/`
Expected: FAIL (`undefined: startGenerationSpan`, `undefined: outcomeLabel`).

- [x] **Step 3: Implement the helper**

Create `pkg/gui/generation_telemetry.go`:
```go
package gui

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// generationInstruments caches the one-shot generation instruments. It is
// rebuilt whenever the global meter provider changes so a test that installs an
// in-memory provider binds to it.
type generationInstruments struct {
	errors    otelmetric.Int64Counter
	duration  otelmetric.Float64Histogram
	fallbacks otelmetric.Int64Counter
}

var (
	genInstrumentsMu       sync.Mutex
	genInstrumentsProvider otelmetric.MeterProvider
	genInstruments         generationInstruments
)

func generationMetrics() generationInstruments {
	provider := otel.GetMeterProvider()
	genInstrumentsMu.Lock()
	defer genInstrumentsMu.Unlock()
	if provider == genInstrumentsProvider {
		return genInstruments
	}
	meter := provider.Meter(telemetry.MeterName)
	genInstruments = generationInstruments{
		errors:    telemetry.Int64Counter(meter, "localrpg.generation.errors", "1", "Generation requests that failed."),
		duration:  telemetry.Float64Histogram(meter, "localrpg.generation.duration", "ms", "Wall-clock duration of one generation request."),
		fallbacks: telemetry.Int64Counter(meter, "localrpg.provider.fallbacks", "1", "Fallback providers engaged after a failed attempt."),
	}
	genInstrumentsProvider = provider
	return genInstruments
}

// startGenerationSpan opens the span for one generation request. The caller must
// end it. A disabled provider yields a no-op span.
func (s *Service) startGenerationSpan(ctx context.Context, name, formType, fieldName string) (context.Context, oteltrace.Span) {
	return telemetry.Tracer("github.com/darkliquid/localrpg/pkg/gui").Start(ctx, name,
		oteltrace.WithAttributes(
			attribute.String("localrpg.form_type", formType),
			attribute.String("localrpg.field_name", fieldName),
		),
	)
}

func outcomeLabel(failure *harness.GenerationFailure) string {
	if failure == nil {
		return "success"
	}
	return "failure"
}

// recordGeneration writes the outcome of one generation request to the trace
// logger (and, through the telemetry bridge, to an OTel log record and span
// event), marks the span, and records the duration and failure counters.
func (s *Service) recordGeneration(ctx context.Context, span oteltrace.Span, formType string, started time.Time, failure *harness.GenerationFailure) {
	elapsed := time.Since(started)
	fields := map[string]interface{}{
		"form_type":   formType,
		"outcome":     outcomeLabel(failure),
		"duration_ms": elapsed.Milliseconds(),
	}
	if failure != nil {
		fields["code"] = string(failure.Code)
		fields["message"] = failure.Message
		fields["attempts"] = len(failure.Attempts)
		generationMetrics().errors.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.form_type", formType),
			attribute.String("localrpg.generation.failure_code", string(failure.Code)),
		))
		if span != nil {
			span.SetAttributes(attribute.String("localrpg.generation.failure_code", string(failure.Code)))
			span.SetStatus(codes.Error, string(failure.Code))
			span.RecordError(failure)
		}
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.error", fields)
	} else {
		if span != nil {
			span.SetAttributes(attribute.String("localrpg.generation.failure_code", ""))
		}
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.complete", fields)
	}
	generationMetrics().duration.Record(ctx, float64(elapsed.Milliseconds()), otelmetric.WithAttributes(
		attribute.String("localrpg.form_type", formType),
		attribute.String("localrpg.generation.outcome", outcomeLabel(failure)),
	))
}

// recordGenerationAttempts makes the fallback chain visible as span events and
// counts each engaged fallback. The free-form detail stays in the JSONL trace
// and the structured failure body, never on the span.
func (s *Service) recordGenerationAttempts(ctx context.Context, span oteltrace.Span, attempts []harness.Attempt) {
	for i, attempt := range attempts {
		if span != nil {
			span.AddEvent("attempt", oteltrace.WithAttributes(
				attribute.String("localrpg.role", attempt.Role),
				attribute.String("gen_ai.system", attempt.Provider),
				attribute.String("localrpg.generation.failure_code", string(attempt.Code)),
				attribute.Int64("duration_ms", attempt.DurationMS),
			))
		}
		if i == 0 {
			continue
		}
		generationMetrics().fallbacks.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.role", attempt.Role),
			attribute.String("localrpg.generation.failure_code", string(attempt.Code)),
		))
		if span != nil {
			span.AddEvent("fallback", oteltrace.WithAttributes(
				attribute.String("localrpg.role", attempts[i-1].Role),
				attribute.String("localrpg.role.next", attempt.Role),
				attribute.String("localrpg.generation.failure_code", string(attempt.Code)),
			))
		}
	}
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestRecordGenerationFailureEmitsSpanAndMetric|TestOutcomeLabel' ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/generation_telemetry.go pkg/gui/generation_telemetry_test.go
git commit -m "feat(gui): add OpenTelemetry instruments for generation failures"
```

---

### Task 5: `GenerateText` returns failures

**Files:**
- Modify: `pkg/gui/text_generate.go:26-29,116-176`
- Test: `pkg/gui/text_generate_failure_test.go`

**Interfaces:**
- Consumes: `harness.GenerationFailure`, `FailureFrom`, `pickFailureCode` (below); `decodeGeneratedValuesChecked`; `(*Service).startGenerationSpan`, `recordGeneration`.
- Produces: `GenerateTextResponse.Warning *harness.GenerationFailure`; `GenerateText` returns `(nil, *GenerationFailure)` when nothing usable was produced; `pickFailureCode(attempts []harness.Attempt) harness.FailureCode`.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/text_generate_failure_test.go`:
```go
package gui

import (
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestPickFailureCode(t *testing.T) {
	if got := pickFailureCode(nil); got != harness.FailureProviderUnavailable {
		t.Fatalf("pickFailureCode(nil) = %q, want provider_unavailable", got)
	}
	attempts := []harness.Attempt{
		{Code: harness.FailureProviderError},
		{Code: harness.FailureParseError},
	}
	if got := pickFailureCode(attempts); got != harness.FailureParseError {
		t.Fatalf("pickFailureCode() = %q, want the last attempt code", got)
	}
}

func TestGenerateTextUnknownFormReturnsNoFailureWithNoFields(t *testing.T) {
	// A form type with no fields requests nothing, so it must not be reported as
	// a provider failure.
	service := &Service{}
	resp, err := service.GenerateText(context.Background(), GenerateTextRequest{FormType: "unknown"})
	if err != nil {
		t.Fatalf("GenerateText returned an error for an empty request: %v", err)
	}
	if len(resp.Fields) != 0 {
		t.Fatalf("fields = %v, want empty", resp.Fields)
	}
	_ = errors.New
}
```

Note: the router is built from config inside `GenerateText`, so the provider-failure paths are exercised end-to-end in Task 7's handler test using a stub config; this task's unit test covers the pure helper and the no-fields case. Keep the stub config helper in `pkg/gui` tests if one exists; otherwise rely on the default (disabled) provider and assert `provider_unavailable`.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestPickFailureCode|TestGenerateTextUnknownFormReturnsNoFailureWithNoFields' ./pkg/gui/`
Expected: FAIL (`undefined: pickFailureCode`).

- [x] **Step 3: Implement**

In `pkg/gui/text_generate.go`, add `time` to the imports and change `GenerateTextResponse`:
```go
type GenerateTextResponse struct {
	Fields      map[string]string         `json:"fields"`
	GeneratedBy string                    `json:"generated_by"`
	Warning     *harness.GenerationFailure `json:"warning,omitempty"`
}
```
Add the helper near the bottom:
```go
// pickFailureCode chooses the most informative code from a fallback chain. An
// empty chain means no role could even be built.
func pickFailureCode(attempts []harness.Attempt) harness.FailureCode {
	if len(attempts) == 0 {
		return harness.FailureProviderUnavailable
	}
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].Code != "" {
			return attempts[i].Code
		}
	}
	return harness.FailureProviderError
}
```
Replace the body of `GenerateText` from the router construction onward (currently lines 146-175) with:
```go
	router, err := harness.RouterFromConfigWithLogger(s.configMgr.Get(), s.logger)
	if err != nil {
		failure := &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: fmt.Sprintf("no model provider is configured: %v", err),
		}
		s.recordGeneration(ctx, nil, req.FormType, started, failure)
		return nil, failure
	}

	request := harness.GenerateRequest{
		System:    systemPrompt,
		Prompt:    buildTextGeneratorPrompt(req, systemFields),
		MaxTokens: 1000,
	}

	roles := []string{role}
	if role != "gm" {
		roles = append(roles, "gm")
	}

	attempts := make([]harness.Attempt, 0, len(roles))
	for _, tryRole := range roles {
		roleStarted := time.Now()
		result, err := router.GenerateForRole(ctx, tryRole, request)
		if err != nil {
			if failure, ok := harness.FailureFrom(err); ok {
				attempts = append(attempts, failure.Attempts...)
				if len(failure.Attempts) == 0 {
					attempts = append(attempts, harness.Attempt{
						Role: tryRole, Provider: router.ProviderIDForRole(tryRole),
						Code: failure.Code, Detail: failure.Message,
						DurationMS: time.Since(roleStarted).Milliseconds(),
					})
				}
				continue
			}
			attempts = append(attempts, harness.Attempt{
				Role: tryRole, Provider: router.ProviderIDForRole(tryRole),
				Code: harness.FailureProviderError, Detail: err.Error(),
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		if result == nil || strings.TrimSpace(result.Text) == "" {
			attempts = append(attempts, harness.Attempt{
				Role: tryRole, Provider: router.ProviderIDForRole(tryRole),
				Code: harness.FailureEmptyResponse, Detail: "model returned no text",
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		values, decodeErr := decodeGeneratedValuesChecked(result.Text)
		if decodeErr != nil {
			attempts = append(attempts, harness.Attempt{
				Role: tryRole, Provider: router.ProviderIDForRole(tryRole),
				Code: harness.FailureParseError, Detail: decodeErr.Error(),
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		resp.GeneratedBy = tryRole
		for k, v := range values {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				resp.Fields[k] = trimmed
			}
		}
		break
	}

	if len(resp.Fields) == 0 {
		resp.GeneratedBy = "none"
		failure := &harness.GenerationFailure{
			Code:        pickFailureCode(attempts),
			Message:     "the model did not return any usable text",
			Attempts:    attempts,
			PromptChars: len([]rune(request.PromptText())),
			ElapsedMS:   time.Since(started).Milliseconds(),
		}
		s.recordGeneration(ctx, nil, req.FormType, started, failure)
		return nil, failure
	}

	if len(attempts) > 0 {
		resp.Warning = &harness.GenerationFailure{
			Code:      attempts[len(attempts)-1].Code,
			Message:   "some requested fields were not generated",
			Attempts:  attempts,
			ElapsedMS: time.Since(started).Milliseconds(),
		}
	}
	s.recordGeneration(ctx, nil, req.FormType, started, nil)
	return resp, nil
```
Make the **first two lines** of `GenerateText` the `started` clock and the span:
```go
	started := time.Now()
	ctx, span := s.startGenerationSpan(ctx, "generate.text", req.FormType, req.FieldName)
	defer span.End()
```
and change the doc comment to describe the new contract:
```go
// GenerateText fills one field or a whole form's worth of values. A failure is
// returned as a *harness.GenerationFailure: an unconfigured provider, a provider
// error, an empty reply, or an unparseable reply. A partial success is a 200 with
// Warning set. The user can always type the answers themselves.
```
The service owns the `generate.text` span: it knows `form_type` and `field_name`, and the HTTP handler has no extra context. Replace every `s.recordGeneration(ctx, nil, ...)` in the new code with `s.recordGeneration(ctx, span, ...)`, and call `s.recordGenerationAttempts(ctx, span, attempts)` immediately before each `recordGeneration` so the fallback chain becomes `attempt`/`fallback` span events and the `localrpg.provider.fallbacks` counter. With telemetry disabled the span is a no-op and all of this is inert.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestPickFailureCode|TestGenerateText' ./pkg/gui/` and `go build ./...`
Expected: PASS and a clean build.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/text_generate.go pkg/gui/text_generate_failure_test.go
git commit -m "feat(gui): report text generation failures instead of an empty success"
```

---

### Task 6: `GenerateCharacter` returns failures

**Files:**
- Modify: `pkg/gui/character_generate.go:15-114`
- Test: `pkg/gui/character_generate_failure_test.go` (extend)

**Interfaces:**
- Consumes: same helpers as Task 5.
- Produces: `GenerateCharacterResponse.Warning *harness.GenerationFailure`; `GenerateCharacter` returns `(nil, *GenerationFailure)` when it requested fields but produced none.

- [x] **Step 1: Write the failing test**

Append to `pkg/gui/character_generate_failure_test.go`:
```go
func TestGenerateCharacterNoGeneratableFieldsSucceeds(t *testing.T) {
	service := &Service{}
	resp, err := service.GenerateCharacter(context.Background(), GenerateCharacterRequest{
		Fields: []core.CharacterCreationField{{ID: "voice", Kind: "voice", Generatable: true}},
	})
	if err != nil {
		t.Fatalf("GenerateCharacter returned an error: %v", err)
	}
	if len(resp.Values) != 0 {
		t.Fatalf("values = %v, want empty", resp.Values)
	}
}
```
Add `"github.com/darkliquid/localrpg/pkg/core"` to that test file's imports.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestGenerateCharacterNoGeneratableFieldsSucceeds ./pkg/gui/`
Expected: PASS already (the guard exists) — this pins current behaviour. If it fails to compile, add the missing `core` import.

- [x] **Step 3: Implement**

In `pkg/gui/character_generate.go`, add `time` to the imports, change the response type, and replace the body from the router construction (`currently lines 79-113`) with:
```go
	started := time.Now()
	router, err := harness.RouterFromConfigWithLogger(s.configMgr.Get(), s.logger)
	if err != nil {
		failure := &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: fmt.Sprintf("no model provider is configured: %v", err),
		}
		s.recordGeneration(ctx, nil, req.FormType, started, failure)
		return nil, failure
	}

	request := harness.GenerateRequest{
		System:    characterGeneratorSystemPrompt,
		Prompt:    characterGeneratorPrompt(req, generatable),
		MaxTokens: 700,
	}

	roles := []string{"character"}
	if req.SystemID == "" {
		roles = []string{"gm"}
	}

	attempts := make([]harness.Attempt, 0, 2)
	var values map[string]string
	for _, role := range roles {
		roleStarted := time.Now()
		result, err := router.GenerateForRole(ctx, role, request)
		if err != nil {
			if failure, ok := harness.FailureFrom(err); ok {
				attempts = append(attempts, failure.Attempts...)
				continue
			}
			attempts = append(attempts, harness.Attempt{
				Role: role, Provider: router.ProviderIDForRole(role),
				Code: harness.FailureProviderError, Detail: err.Error(),
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		if result == nil || strings.TrimSpace(result.Text) == "" {
			attempts = append(attempts, harness.Attempt{
				Role: role, Provider: router.ProviderIDForRole(role),
				Code: harness.FailureEmptyResponse, Detail: "model returned no text",
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		decoded, decodeErr := decodeGeneratedValuesChecked(result.Text)
		if decodeErr != nil {
			attempts = append(attempts, harness.Attempt{
				Role: role, Provider: router.ProviderIDForRole(role),
				Code: harness.FailureParseError, Detail: decodeErr.Error(),
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		values = decoded
		resp.GeneratedBy = role
		break
	}

	for _, field := range generatable {
		if value, ok := values[field.ID]; ok && strings.TrimSpace(value) != "" {
			resp.Values[field.ID] = strings.TrimSpace(value)
		}
	}

	if len(resp.Values) == 0 {
		resp.GeneratedBy = "none"
		failure := &harness.GenerationFailure{
			Code:        pickFailureCode(attempts),
			Message:     "the model did not return any usable character values",
			Attempts:    attempts,
			PromptChars: len([]rune(request.PromptText())),
			ElapsedMS:   time.Since(started).Milliseconds(),
		}
		s.recordGeneration(ctx, nil, req.FormType, started, failure)
		return nil, failure
	}

	if len(attempts) > 0 {
		resp.Warning = &harness.GenerationFailure{
			Code:     attempts[len(attempts)-1].Code,
			Message:  "some requested fields were not generated",
			Attempts: attempts, ElapsedMS: time.Since(started).Milliseconds(),
		}
	}
	s.recordGeneration(ctx, nil, req.FormType, started, nil)
	return resp, nil
```
Change the response type:
```go
type GenerateCharacterResponse struct {
	Values      map[string]string          `json:"values"`
	GeneratedBy string                     `json:"generated_by"`
	Warning     *harness.GenerationFailure `json:"warning,omitempty"`
}
```
`GenerateCharacterRequest` has no `FormType`, so pass the literal `"character"` to `recordGeneration`. Make the first two lines of `GenerateCharacter` the clock and the span (after the `generatable` guard, so a request with nothing to generate still returns early without a span):
```go
	started := time.Now()
	ctx, span := s.startGenerationSpan(ctx, "generate.text", "character", "_all")
	defer span.End()
```
Then replace every `s.recordGeneration(ctx, nil, "character", ...)` in the new code with `s.recordGeneration(ctx, span, ...)` and call `s.recordGenerationAttempts(ctx, span, attempts)` immediately before each `recordGeneration`.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestGenerateCharacter|TestDecodeGeneratedValuesChecked' ./pkg/gui/` and `go build ./...`
Expected: PASS and a clean build.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/character_generate.go pkg/gui/character_generate_failure_test.go
git commit -m "feat(gui): report character generation failures"
```

---

### Task 7: Map failures to HTTP status and JSON body

**Files:**
- Create: `pkg/gui/generation_errors.go`
- Test: `pkg/gui/generation_errors_test.go`
- Modify: `pkg/gui/text_generate.go:178-198`, `pkg/gui/character_generate.go:15-35`, `pkg/gui/server.go:1130-1151`

**Interfaces:**
- Consumes: `harness.GenerationFailure`, `harness.FailureFrom`.
- Produces: `generationStatus(code harness.FailureCode) int`; `writeGenerationFailure(w http.ResponseWriter, err error) bool` (returns false when `err` is not a generation failure); `writeGenerationError(w http.ResponseWriter, failure *harness.GenerationFailure)`.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/generation_errors_test.go`:
```go
package gui

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestGenerationStatus(t *testing.T) {
	tests := map[harness.FailureCode]int{
		harness.FailureProviderUnavailable: 503,
		harness.FailureProviderError:       502,
		harness.FailureEmptyResponse:       502,
		harness.FailureParseError:          422,
		harness.FailureTimeout:             504,
		harness.FailureContextTooLarge:     413,
		harness.FailureInvalidRequest:      400,
		harness.FailureCode("unknown"):     500,
	}
	for code, want := range tests {
		if got := generationStatus(code); got != want {
			t.Fatalf("generationStatus(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestWriteGenerationFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	failure := &harness.GenerationFailure{Code: harness.FailureEmptyResponse, Message: "no text"}
	if !writeGenerationFailure(rec, failure) {
		t.Fatal("writeGenerationFailure did not recognise a failure")
	}
	if rec.Code != 502 {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	var body struct {
		Error harness.GenerationFailure `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Error.Code != harness.FailureEmptyResponse {
		t.Fatalf("error.code = %q, want empty_response", body.Error.Code)
	}
}

func TestWriteGenerationFailureIgnoresPlainErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	if writeGenerationFailure(rec, errPlain{}) {
		t.Fatal("writeGenerationFailure claimed a plain error was a generation failure")
	}
}

type errPlain struct{}

func (errPlain) Error() string { return "plain" }
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestGenerationStatus|TestWriteGenerationFailure' ./pkg/gui/`
Expected: FAIL (`undefined: generationStatus`).

- [x] **Step 3: Implement**

Create `pkg/gui/generation_errors.go`:
```go
package gui

import (
	"encoding/json"
	"net/http"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// generationStatus maps a bounded failure code to the HTTP status that best
// describes it. A hard failure is a non-2xx; a partial success never reaches
// here because it is returned as a 200 with Warning set.
func generationStatus(code harness.FailureCode) int {
	switch code {
	case harness.FailureInvalidRequest:
		return http.StatusBadRequest
	case harness.FailureContextTooLarge:
		return http.StatusRequestEntityTooLarge
	case harness.FailureParseError:
		return http.StatusUnprocessableEntity
	case harness.FailureProviderUnavailable:
		return http.StatusServiceUnavailable
	case harness.FailureTimeout:
		return http.StatusGatewayTimeout
	case harness.FailureProviderError, harness.FailureEmptyResponse:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// writeGenerationError writes a structured failure body and status.
func writeGenerationError(w http.ResponseWriter, failure *harness.GenerationFailure) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(generationStatus(failure.Code))
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": failure})
}

// writeGenerationFailure recognises a generation failure and writes it, so
// handlers can share one line. It reports whether it handled the error; a plain
// error must still go through the caller's existing path.
func writeGenerationFailure(w http.ResponseWriter, err error) bool {
	failure, ok := harness.FailureFrom(err)
	if !ok {
		return false
	}
	writeGenerationError(w, failure)
	return true
}
```
Update `handleGenerateTextRoute` (replace lines 192-197):
```go
	resp, err := s.service.GenerateText(r.Context(), req)
	if err != nil {
		if writeGenerationFailure(w, err) {
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
```
Update `handleCharacterGenerateRoute` identically around its `s.service.GenerateCharacter` call.
Update `handleGenerateAssetPreview` (replace lines 1144-1150):
```go
	data, contentType, err := s.service.GenerateAssetPreview(r.Context(), req)
	if err != nil {
		if writeGenerationFailure(w, err) {
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestGenerationStatus|TestWriteGenerationFailure' ./pkg/gui/` and `go test ./pkg/gui/`
Expected: PASS (existing server tests that expected 200-empty on failure, if any, must be updated — search `generated_by` in `pkg/gui/server_test.go` and adjust them to assert the new status).

- [x] **Step 5: Commit**

```bash
git add pkg/gui/generation_errors.go pkg/gui/generation_errors_test.go pkg/gui/text_generate.go pkg/gui/character_generate.go pkg/gui/server.go
git commit -m "feat(gui): return structured errors for failed generation"
```

---

### Task 8: Turn generation diagnostics

**Files:**
- Modify: `pkg/engine/orchestrator.go:766-774, 780-888, 1051-1077, 573-601` and the deferred block at `373-382`
- Modify: `pkg/gui/types.go:376-389`
- Modify: `pkg/gui/server.go:1063-1065`
- Test: `pkg/engine/orchestrator_failure_test.go`

**Interfaces:**
- Consumes: `harness.GenerationFailure`, `harness.ClassifyProviderError`.
- Produces: `streamResult.Failure *harness.GenerationFailure`, `streamResult.ProviderID string`; the `turn` root span gets `turn.outcome=error`, `SetStatus`, `RecordError`; `provider.generate` gets `localrpg.generation.failure_code`; `TurnEvent` gains `Code`, `Detail`, `Failure`.

- [x] **Step 1: Write the failing test**

Create `pkg/engine/orchestrator_failure_test.go`:
```go
package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

type failingStreamProvider struct{ err error }

func (f failingStreamProvider) ID() string { return "failing" }

func (f failingStreamProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return nil, f.err
}

func (f failingStreamProvider) Stream(_ context.Context, _ harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	close(out)
	return f.err
}

func TestStreamStalledIsTimeoutFailure(t *testing.T) {
	router := harness.NewRouter()
	router.RegisterProvider(failingStreamProvider{err: ErrGenerationStalled})
	router.AssignRole("gm", "failing")

	o := &TurnOrchestrator{router: router, chunkTimeout: 0}
	result, err := o.stream(context.Background(), failingStreamProvider{err: ErrGenerationStalled}, harness.GenerateRequest{}, nil)
	if err == nil {
		t.Fatal("stream returned no error")
	}
	if !errors.Is(err, ErrGenerationStalled) {
		t.Fatalf("err = %v, want ErrGenerationStalled", err)
	}
	if result.Failure == nil || result.Failure.Code != harness.FailureTimeout {
		t.Fatalf("result.Failure = %+v, want timeout", result.Failure)
	}
}

func TestProcessActionFailureRecordsTurnSpanError(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()
	_ = recorder
	// The full orchestrator needs a campaign fixture; this test asserts the
	// streamResult contract and is expanded in the integration task using the
	// existing orchestrator test fixtures.
}
```
(If `TurnOrchestrator` fields are unexported and the test is in package `engine`, direct construction is allowed. If the fields differ, set only `chunkTimeout` and `router`; check `pkg/engine/orchestrator_test.go` for the existing constructor helper and use it.)

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestStreamStalledIsTimeoutFailure' ./pkg/engine/`
Expected: FAIL (`result.Failure` undefined).

- [x] **Step 3: Implement the engine changes**

In `pkg/engine/orchestrator.go`:
1. Extend `streamResult`:
```go
type streamResult struct {
	Text         string
	FinishReason string
	Interrupted  error
	ToolCalls    []harness.ToolCall
	Provenance   []ToolCallRecord
	// Failure is the bounded generation failure when the stream produced no
	// usable text; ProviderID names the provider that failed.
	Failure    *harness.GenerationFailure
	ProviderID string
}
```
2. In `stream`, set the failure on every error return. Replace the `interrupted` closure and the returns:
```go
	interrupted := func(err error) (streamResult, error) {
		result := streamResult{ProviderID: provider.ID()}
		if sawText {
			result.Text = sb.String()
			result.FinishReason = finishReason
			result.Interrupted = err
			return result, nil
		}
		result.Failure = &harness.GenerationFailure{
			Code:      harness.ClassifyProviderError(err),
			Message:   err.Error(),
			ElapsedMS: time.Since(started).Milliseconds(),
		}
		if errors.Is(err, ErrGenerationStalled) {
			result.Failure.Code = harness.FailureTimeout
		}
		return result, err
	}
```
Add `started := time.Now()` at the top of `stream`, and change the clean-close return to:
```go
			if strings.TrimSpace(sb.String()) == "" {
				return streamResult{
					ProviderID: provider.ID(),
					Failure: &harness.GenerationFailure{
						Code:      harness.FailureEmptyResponse,
						Message:   "provider returned no text",
						ElapsedMS: time.Since(started).Milliseconds(),
					},
				}, nil
			}
			return streamResult{Text: sb.String(), FinishReason: finishReason, ToolCalls: toolCalls, ProviderID: provider.ID()}, nil
```
3. In `generateRequest`, preserve the primary failure when the fallback also fails:
```go
	result, err := o.stream(ctx, provider, req, onChunk)
	if err == nil || errors.Is(err, errStreamListener) || ctx.Err() != nil {
		return result, err
	}
	if fallback, ok := o.router.FallbackForRole("gm"); ok {
		fallbackResult, fallbackErr := o.stream(ctx, fallback, req, onChunk)
		if fallbackErr == nil && fallbackResult.Failure == nil {
			return fallbackResult, nil
		}
		if result.Failure != nil && len(result.Failure.Attempts) == 0 {
			result.Failure.Attempts = []harness.Attempt{{
				Role: "gm", Provider: result.ProviderID, Code: result.Failure.Code, Detail: result.Failure.Message,
			}}
		}
		return result, err
	}
	return result, err
```
4. In `runGenerationLoop`, replace the failure block (lines 1065-1075) with:
```go
		if err != nil {
			code := harness.ClassifyProviderError(err)
			if result.Failure != nil && result.Failure.Code != "" {
				code = result.Failure.Code
			}
			engineMetrics().providerErrors.Add(context.Background(), 1, otelmetric.WithAttributes(
				attribute.String("localrpg.role", "gm"),
				attribute.String("error.kind", string(code)),
				attribute.String("gen_ai.system", provider.ID()),
			))
			engineMetrics().providerDuration.Record(context.Background(), roundDuration, roundAttributes)
			roundSpan.SetAttributes(attribute.String("localrpg.generation.failure_code", string(code)))
			roundSpan.RecordError(err)
			roundSpan.SetStatus(codes.Error, string(code))
			roundSpan.End()
			return result, err
		}
```
Also handle an empty-but-no-error result after `roundSpan.End()`: when `result.Failure != nil`, record the counter and status the same way and return `result, result.Failure`. Add immediately after line 1077:
```go
		if result.Failure != nil {
			engineMetrics().providerErrors.Add(context.Background(), 1, otelmetric.WithAttributes(
				attribute.String("localrpg.role", "gm"),
				attribute.String("error.kind", string(result.Failure.Code)),
				attribute.String("gen_ai.system", provider.ID()),
			))
			roundSpan.SetAttributes(attribute.String("localrpg.generation.failure_code", string(result.Failure.Code)))
			roundSpan.RecordError(result.Failure)
			roundSpan.SetStatus(codes.Error, string(result.Failure.Code))
			roundSpan.End()
			return result, result.Failure
		}
```
5. In `ProcessActionStream`: set the turn outcome to `error` on the generation-failure return and enrich the `generation.error` event. Replace lines 573-601:
```go
	result, err := o.runGenerationLoop(ctx, &assembly, gmDirective, onChunk)
	if err != nil {
		outcome = "error"
		failure, _ := harness.FailureFrom(err)
		fields := map[string]interface{}{
			"error":         err.Error(),
			"generation_code": generationCode(failure),
			"provider":      result.ProviderID,
			"prompt_chars":  len([]rune(assembly.Prompt)),
			"elapsed_ms":    time.Since(turnStarted).Milliseconds(),
		}
		if failure != nil {
			fields["attempts"] = len(failure.Attempts)
			turnSpan.SetAttributes(
				attribute.String("localrpg.generation.failure_code", string(failure.Code)),
				attribute.Int("localrpg.generation.attempts", len(failure.Attempts)),
			)
			turnSpan.RecordError(failure)
			turnSpan.SetStatus(codes.Error, string(failure.Code))
		} else {
			turnSpan.SetStatus(codes.Error, err.Error())
			turnSpan.RecordError(err)
		}
		if result.Failure != nil {
			fields["finish_reason"] = result.Failure.FinishReason
		}
		o.logger.Event("generation.error", fields)
		return nil, fmt.Errorf("gm generation failed: %w", err)
	}
```
And the empty-narration branch (currently 595-601):
```go
	if strings.TrimSpace(narration) == "" {
		outcome = "error"
		failure := &harness.GenerationFailure{
			Code:         harness.FailureEmptyResponse,
			Message:      "gm returned no narration",
			FinishReason: result.FinishReason,
			ElapsedMS:    time.Since(turnStarted).Milliseconds(),
		}
		if result.Failure != nil {
			failure = result.Failure
		}
		turnSpan.SetAttributes(attribute.String("localrpg.generation.failure_code", string(failure.Code)))
		turnSpan.RecordError(failure)
		turnSpan.SetStatus(codes.Error, string(failure.Code))
		o.logger.Event("generation.error", map[string]interface{}{
			"error":           failure.Message,
			"generation_code": string(failure.Code),
			"finish_reason":   result.FinishReason,
		})
		return nil, failure
	}
```
6. Update the deferred block (lines 373-382) so failed turns are separable and not counted as completed:
```go
	defer func() {
		turnSpan.SetAttributes(attribute.String("turn.outcome", outcome))
		metrics := engineMetrics()
		attributes := otelmetric.WithAttributes(
			attribute.String("turn.mode", mode),
			attribute.String("turn.outcome", outcome),
		)
		metrics.turnDuration.Record(context.Background(), float64(time.Since(turnStarted).Milliseconds()), attributes)
		metrics.turnCompleted.Add(context.Background(), 1, otelmetric.WithAttributes(
			attribute.String("turn.mode", mode),
			attribute.String("turn.outcome", outcome),
		))
	}()
```
7. Add the helper:
```go
func generationCode(failure *harness.GenerationFailure) string {
	if failure == nil {
		return string(harness.FailureProviderError)
	}
	return string(failure.Code)
}
```
8. Record extractor failures on their span. The extractor block currently only counts when `err == nil` (`orchestrator.go:632-640`). Change it to:
```go
	extractSpan.SetAttributes(attribute.Int("localrpg.entities.extracted", len(entities)))
	if extractErr != nil {
		extractSpan.RecordError(extractErr)
		extractSpan.SetStatus(codes.Error, string(harness.ClassifyProviderError(extractErr)))
		o.logger.Event("extract.error", map[string]interface{}{"error": extractErr.Error()})
	}
	extractSpan.End()
```
Keep the existing behaviour that a failed extractor never loses the turn; only the span and trace gain the reason.
(The orchestrator already imports `harness`, `attribute`, `codes`.)

- [x] **Step 4: Surface the failure in the streamed event**

In `pkg/gui/types.go`, extend `TurnEvent`:
```go
	// Structured generation failure detail, present when Type is "error".
	Code    string                     `json:"code,omitempty"`
	Detail  string                     `json:"detail,omitempty"`
	Failure *harness.GenerationFailure `json:"failure,omitempty"`
```
Add the `harness` import to `types.go`. In `pkg/gui/server.go`, replace line 1064:
```go
	if err := session.Run(r.Context(), req, writeEvent); err != nil {
		event := TurnEvent{Type: "error", Message: err.Error()}
		if failure, ok := harness.FailureFrom(err); ok {
			event.Code = string(failure.Code)
			event.Detail = failure.Message
			event.Failure = failure
		}
		_ = writeEvent(event)
	}
```
Add the `harness` import to `server.go` if absent.

- [x] **Step 5: Run tests**

Run: `go test -run 'TestStreamStalledIsTimeoutFailure' ./pkg/engine/` and `go test ./pkg/engine/ ./pkg/gui/`
Expected: PASS. Existing turn tests that assert a bare `"gm returned no narration"` message may now need the error type; update them to assert `FailureFrom(err)` with code `empty_response`.

- [x] **Step 6: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_failure_test.go pkg/gui/types.go pkg/gui/server.go
git commit -m "feat(engine): record structured turn generation failures"
```

---

### Task 9: Reject empty image bytes

**Files:**
- Modify: `pkg/gui/service.go:2685-2778`
- Test: `pkg/gui/asset_generate_failure_test.go`

**Interfaces:**
- Consumes: `harness.NewFailure`.
- Produces: the three asset methods return a `*harness.GenerationFailure` when the client yields zero bytes or fails.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/asset_generate_failure_test.go`:
```go
package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestGenerateAssetPreviewEmptyBytesIsFailure(t *testing.T) {
	// The real client is built from config; this asserts the shared guard used
	// by all three asset methods.
	if _, err := guardImageBytes(nil); err == nil {
		t.Fatal("guardImageBytes(nil) returned no error")
	}
	failure, ok := harness.FailureFrom(guardImageBytes([]byte{}))
	if !ok || failure.Code != harness.FailureProviderError {
		t.Fatalf("guardImageBytes(empty) = %v, want provider_error", failure)
	}
	if _, err := guardImageBytes([]byte("<svg/>")); err != nil {
		t.Fatalf("guardImageBytes(svg) returned an error: %v", err)
	}
	_ = context.Background
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestGenerateAssetPreviewEmptyBytesIsFailure ./pkg/gui/`
Expected: FAIL (`undefined: guardImageBytes`).

- [x] **Step 3: Implement**

The three asset methods also get a `generate.image` span and telemetry, so image failures are visible in OpenTelemetry. In each method add `started := time.Now()` and, after any validation, `ctx, span := s.startGenerationSpan(ctx, "generate.image", "image", req.Kind); defer span.End()`. Replace the failure returns with a `*harness.GenerationFailure` and call `s.recordGeneration(ctx, span, "image", started, failure)` before returning it; on success call `s.recordGeneration(ctx, span, "image", started, nil)`.

In `pkg/gui/service.go`, add the guard and use it in all three methods:
```go
// guardImageBytes rejects a provider that returned success with no image data,
// so a zero-byte file is never written or served as .webp/octet-stream.
func guardImageBytes(imgBytes []byte) ([]byte, error) {
	if len(imgBytes) == 0 {
		return nil, &harness.GenerationFailure{
			Code:    harness.FailureProviderError,
			Message: "image provider returned no data",
		}
	}
	return imgBytes, nil
}
```
In `GenerateAssetPreview`, replace the image error block:
```go
	imgBytes, err := client.GenerateImage(ctx, prompt)
	if err != nil {
		return nil, "", &harness.GenerationFailure{
			Code:    harness.FailureProviderError,
			Message: fmt.Sprintf("generate image: %v", err),
		}
	}
	imgBytes, err = guardImageBytes(imgBytes)
	if err != nil {
		return nil, "", err
	}
```
Apply the same shape to `GenerateGameAsset` and `GenerateWorldAsset` (they return `string, error`; return `"", failure`). Add the `harness` import.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestGenerateAssetPreviewEmptyBytesIsFailure|TestBuildAssetPrompt' ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/asset_generate_failure_test.go
git commit -m "fix(gui): reject empty image generation results"
```

---

### Task 10: Frontend failure types and client

**Files:**
- Modify: `frontend/src/types.ts:164-185` (near `GenerateTextRequest`/`GenerateTextResponse`)
- Modify: `frontend/src/api/client.ts:40-49,110-130,188-202`

**Interfaces:**
- Consumes: the backend JSON shapes from Tasks 5-9.
- Produces: `GenerationFailureCode`, `GenerationAttempt`, `GenerationFailure`, `GenerateTextResponse.warning`, `GenerateCharacterResponse.warning`, `GenerationError extends HTTPError`.

- [x] **Step 1: Add the types**

In `frontend/src/types.ts`, add:
```ts
export type GenerationFailureCode =
  | 'provider_unavailable' | 'provider_error' | 'empty_response'
  | 'parse_error' | 'timeout' | 'context_too_large' | 'invalid_request';

export interface GenerationAttempt {
  role: string;
  provider: string;
  code: GenerationFailureCode;
  detail?: string;
  duration_ms: number;
}

export interface GenerationFailure {
  code: GenerationFailureCode;
  message: string;
  attempts?: GenerationAttempt[];
  finish_reason?: string;
  prompt_chars?: number;
  context_chars?: number;
  elapsed_ms?: number;
}
```
Add `warning?: GenerationFailure;` to `GenerateTextResponse` and `GenerateCharacterResponse`. (Locate them with `grep -n "interface GenerateTextResponse" frontend/src/types.ts`.)

- [x] **Step 2: Add `GenerationError` and JSON error parsing**

In `frontend/src/api/client.ts`, after `HTTPError`:
```ts
// GenerationError carries the structured failure a generation endpoint returns,
// so a caller can show why nothing was generated instead of guessing.
export class GenerationError extends HTTPError {
  failure: GenerationFailure;
  constructor(status: number, failure: GenerationFailure) {
    super(status, failure.message);
    this.name = 'GenerationError';
    this.failure = failure;
  }
}

async function throwGenerationError(res: Response): Promise<never> {
  const text = await res.text();
  let failure: GenerationFailure = {
    code: 'provider_error',
    message: text || `generation failed with status ${res.status}`,
  };
  try {
    const body = JSON.parse(text) as { error?: GenerationFailure };
    if (body.error && body.error.code) failure = body.error;
  } catch {
    // A non-JSON body is left as the message.
  }
  throw new GenerationError(res.status, failure);
}
```
Import `GenerationFailure` in client.ts. Replace the not-ok paths:
- `generateText`: `if (!res.ok) return throwGenerationError(res);`
- `generateCharacter`: `if (!res.ok) return throwGenerationError(res);`
- `generateAssetPreview`, `generateGameAsset`, `generateWorldAsset`: same.

- [x] **Step 3: Verify it compiles**

Run: `mise run test:frontend`
Expected: PASS (`tsc --noEmit`).

- [x] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): surface structured generation failures"
```

---

### Task 11: Visible generation errors in the UI

**Files:**
- Modify: `frontend/src/components/ui/AIGenerateButton.tsx`
- Modify: `frontend/src/components/launcher/NewCampaignModal.tsx:113-149`
- Modify: `frontend/src/components/WorldsStudio.tsx:252-273,583-605`
- Modify: `frontend/src/components/SystemsStudio.tsx` (auto-fill and `AIGenerateButton` callers)

**Interfaces:**
- Consumes: `GenerationError`, `GenerationFailure` from Task 10.
- Produces: `AIGenerateButtonProps.onError?: (failure: GenerationFailure) => void`; an inline error badge; form toasts on failure.

- [x] **Step 1: Add the error surface to `AIGenerateButton`**

Rewrite `frontend/src/components/ui/AIGenerateButton.tsx` to:
```tsx
import React, { useState } from 'react';
import { Sparkles, Loader2, AlertCircle } from 'lucide-react';
import { APIClient, GenerationError } from '../../api/client';
import { GenerateTextRequest, GenerationFailure } from '../../types';

export interface AIGenerateButtonProps {
  formType: 'character' | 'world' | 'system' | 'campaign';
  fieldName: string;
  getContext: () => Record<string, string>;
  onGenerated: (value: string) => void;
  onError?: (failure: GenerationFailure) => void;
  worldID?: string;
  systemID?: string;
  seed?: string;
  disabled?: boolean;
  className?: string;
  title?: string;
}

const DEFAULT_FAILURE: GenerationFailure = {
  code: 'empty_response',
  message: 'The model returned no text for this field.',
};

export const AIGenerateButton: React.FC<AIGenerateButtonProps> = ({
  formType, fieldName, getContext, onGenerated, onError,
  worldID, systemID, seed, disabled = false, className = '', title,
}) => {
  const [isGenerating, setIsGenerating] = useState(false);
  const [error, setError] = useState<GenerationFailure | null>(null);

  const report = (failure: GenerationFailure) => {
    setError(failure);
    if (onError) onError(failure);
  };

  const handleGenerate = async (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (isGenerating || disabled) return;

    setIsGenerating(true);
    setError(null);
    try {
      const payload: GenerateTextRequest = {
        form_type: formType, field_name: fieldName, context: getContext(),
        world_id: worldID, system_id: systemID, seed: seed || undefined,
      };
      const res = await APIClient.generateText(payload);
      const value = res.fields?.[fieldName];
      if (value) {
        onGenerated(value);
        return;
      }
      report(res.warning ?? DEFAULT_FAILURE);
    } catch (err) {
      report(err instanceof GenerationError ? err.failure : { code: 'provider_error', message: (err as Error).message });
    } finally {
      setIsGenerating(false);
    }
  };

  return (
    <span className="inline-flex items-center gap-1">
      <button
        type="button"
        onClick={handleGenerate}
        disabled={isGenerating || disabled}
        className={`inline-flex items-center justify-center p-1 rounded-md bg-purple-600/15 hover:bg-purple-600/25 border border-purple-500/30 text-purple-300 hover:text-purple-200 transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${className}`}
        title={error ? `${error.code}: ${error.message}` : (title || `AI Generate ${fieldName}`)}
      >
        {isGenerating ? (
          <Loader2 className="w-3 h-3 animate-spin text-purple-400" />
        ) : (
          <Sparkles className={`w-3 h-3 ${error ? 'text-red-400' : 'text-purple-300'}`} />
        )}
      </button>
      {error && (
        <span className="inline-flex items-center gap-1 text-[10px] font-sans text-red-300" role="alert">
          <AlertCircle className="w-3 h-3 text-red-400" />
          <span className="max-w-[18rem] truncate">{error.code}</span>
        </span>
      )}
    </span>
  );
};
```

- [x] **Step 2: Surface generate-all failures**

In `frontend/src/components/launcher/NewCampaignModal.tsx`, in `handleGenerateAll`, after the two `generateText` calls, inspect warnings. Replace the set of `if (campRes.fields...)` lines by capturing the responses and adding:
```tsx
      const warning = campRes.warning ?? charRes.warning;
      if (warning) {
        setGenError(`${warning.code}: ${warning.message}`);
      }
```
and in the `catch (err)` block replace `console.error(...)` with a user-visible message:
```tsx
    } catch (err) {
      setGenError((err as Error).message || 'Generation failed');
    } finally {
```
(Import `GenerationError` is unnecessary here because the message is enough; if richer detail is wanted, use `err instanceof GenerationError ? err.failure.message : ...`.)

- [x] **Step 3: Surface auto-fill and button failures in the studios**

In `WorldsStudio.tsx` `handleGenerateAllWorldFields`, replace the unconditional success toast:
```tsx
      const warning = res.warning;
      if (warning) {
        setToast({ type: 'error', message: `Partial: ${warning.code} — ${warning.message}` });
      } else if (Object.keys(res.fields).length === 0) {
        setToast({ type: 'error', message: 'The model returned nothing to fill.' });
      } else {
        setToast({ type: 'success', message: 'Auto-filled world fields!' });
      }
```
Pass `onError` to each `AIGenerateButton` in the file:
```tsx
onError={(failure) => setToast({ type: 'error', message: `${failure.code}: ${failure.message}` })}
```
Do the same in `SystemsStudio.tsx` for its auto-fill handler and its `AIGenerateButton` instances (`grep -n "AIGenerateButton" frontend/src/components/SystemsStudio.tsx`).

- [x] **Step 4: Verify it compiles**

Run: `mise run test:frontend` and `mise run build:frontend`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/ui/AIGenerateButton.tsx frontend/src/components/launcher/NewCampaignModal.tsx frontend/src/components/WorldsStudio.tsx frontend/src/components/SystemsStudio.tsx
git commit -m "feat(frontend): show generation failures instead of a silent spinner"
```

---

### Task 12: Full verification

- [x] **Step 1: Run the whole suite**

Run: `mise run test` and `mise run lint`
Expected: PASS (`go test -v -count=1 ./...`, `npx tsc --noEmit`, `go vet ./...`).

- [x] **Step 2: Manual verification**

1. Configure the `gm` role as a `cli` provider running a command that prints nothing. Click an AI generate button in Worlds Studio and confirm an inline `empty_response` error and a red toast.
2. Point the role at a failing command and confirm `provider_error` with the underlying detail.
3. Set the role to `disabled` and confirm an `unavailable` error (503).
4. Run a turn with a provider that stalls; confirm the error banner names the timeout and `generate.error` is in the Debug trace with `generation_code`.
5. With telemetry enabled against a local collector, confirm a `generate.text` span with `Status=Error` and `localrpg.generation.failure_code`, and that `localrpg.generation.errors` increments.

- [x] **Step 3: Commit any test fixups**

```bash
git add -A
git commit -m "test: verify generation failure diagnostics"
```
