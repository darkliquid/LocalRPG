# Generation and World Studio Follow-ups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the deferred review findings: complete the OpenTelemetry surface on one unified `generate.*` event family, make generation routes return one JSON error shape, surface errors at every frontend call site, remove the world-draft selection race, and add the tests the first pass skipped.

**Architecture:** Extract the duplicated role/attempt loop and the image path into shared helpers, then hang the missing span/metric attributes and events off them. The image span stays `generate.image`; text and image share `generate.request`/`generate.attempt`/`generate.complete`/`generate.error`, distinguished by `localrpg.form_type`. A single `writeGenerationError`/`writeInvalidRequest` pair gives every generation route one JSON error shape. The World Studio gains a request token so the last click wins.

**Tech Stack:** Go 1.27.1 (`pkg/harness`, `pkg/gui`, `pkg/engine`, `go.opentelemetry.io/otel`), React 19 + TypeScript + Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-09-25-generation-and-world-studio-followups-design.md`

## Global Constraints

- Go 1.27.1. Standard library only for tests (`testing`, `t.TempDir()`); no testify.
- Use `interface{}`, not `any`; wrap errors with `fmt.Errorf("...: %w", err)`; `go vet ./...` must stay clean.
- The failure-code enum and the success contract do not change: `provider_unavailable`, `provider_error`, `empty_response`, `parse_error`, `timeout`, `context_too_large`, `invalid_request`.
- One event family: `generate.request`, `generate.attempt`, `generate.complete`, `generate.error`. Images use `form_type=image`; there is no `image.*` event family.
- Span name is `generate.image` (no `localrpg.` prefix on span names). Image attributes are `localrpg.image.kind`, `localrpg.image.provider`, `localrpg.image.bytes`.
- Payload text is never a span attribute; `trace.Sanitize` is the only redaction point. `game.id` is never a metric attribute.
- TypeScript: `strict`, `noUnusedLocals`, `noUnusedParameters`; `npx tsc --noEmit` is the frontend gate.
- Conventional Commits with a scope; subject under 72 characters.

---

## File Map

**Create**
- `pkg/gui/generation_attempts.go` — shared role/attempt collection.
- `pkg/gui/generation_attempts_test.go` — failure-path table tests.
- `pkg/gui/image_generation.go` — shared image request (span, client, guard, metrics, events).
- `pkg/gui/image_generation_test.go` — empty-byte and event tests with a stub client.

**Modify**
- `pkg/gui/generation_telemetry.go` — free span helpers, `role` on metrics, `recordImage`, event emission.
- `pkg/gui/generation_telemetry_test.go` — adapt to the new signatures.
- `pkg/gui/text_generate.go`, `pkg/gui/character_generate.go` — use the shared helper; set result span attributes.
- `pkg/gui/service.go` — `GenerateAssetPreview`/`GenerateGameAsset`/`GenerateWorldAsset` delegate to `generateImage`.
- `pkg/gui/generation_errors.go` — `writeInvalidRequest`, `writeJSONError`, `writeJSON` reuse.
- `pkg/gui/text_generate.go`, `pkg/gui/character_generate.go`, `pkg/gui/server.go` — malformed bodies use `writeInvalidRequest`.
- `pkg/engine/orchestrator.go` — `streamResult.ChunkCount`, complete `generation.error` fields.
- `pkg/harness/router.go` — `StreamForRole` attempt tracking and failure type.
- `pkg/harness/router_failure_test.go` — stream parity tests.
- `frontend/src/components/WorldsStudio.tsx` — `onError` wiring, slug highlight, request token, `startMode` ref, typed catch.
- `frontend/src/components/SystemsStudio.tsx`, `frontend/src/components/launcher/NewCampaignModal.tsx`, `frontend/src/components/launcher/CampaignSettingsModal.tsx` — `onError` wiring.
---

### Task 1: Shared attempt collection

**Files:**
- Create: `pkg/gui/generation_attempts.go`
- Create: `pkg/gui/generation_attempts_test.go`
- Modify: `pkg/gui/text_generate.go` (loop in `GenerateText`), `pkg/gui/character_generate.go` (loop in `GenerateCharacter`)

**Interfaces:**
- Consumes: `harness.GenerationFailure`, `harness.FailureFrom`, `harness.FailureCode`; `decodeGeneratedValuesChecked`.
- Produces: `type generationOutcome struct { Values map[string]string; Attempts []harness.Attempt; GeneratedBy string }`; `func collectTextAttempts(ctx context.Context, router *harness.Router, roles []string, request harness.GenerateRequest) generationOutcome`.

- [ ] **Step 1: Write the failing test**

Create `pkg/gui/generation_attempts_test.go`:
```go
package gui

import (
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

type scriptedProvider struct {
	id   string
	text string
	err  error
}

func (p *scriptedProvider) ID() string { return p.id }

func (p *scriptedProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: p.text}, p.err
}

func (p *scriptedProvider) Stream(context.Context, harness.GenerateRequest, chan<- harness.StreamChunk) error {
	return nil
}

func routerWith(primary, fallback *scriptedProvider) *harness.Router {
	router := harness.NewRouter()
	router.RegisterProvider(primary)
	router.AssignRole("gm", primary.id)
	if fallback != nil {
		router.RegisterProvider(fallback)
		router.SetFallback("gm", fallback.id)
	}
	return router
}

func TestCollectTextAttempts(t *testing.T) {
	tests := []struct {
		name        string
		primary     *scriptedProvider
		fallback    *scriptedProvider
		wantValue   string
		wantBy      string
		wantCodes   []harness.FailureCode
	}{
		{
			name:      "fallback succeeds after a provider error",
			primary:   &scriptedProvider{id: "p1", err: errors.New("boom")},
			fallback:  &scriptedProvider{id: "p2", text: `{"name":"Vela"}`},
			wantValue: "Vela",
			wantBy:    "p2",
			wantCodes: []harness.FailureCode{harness.FailureProviderError},
		},
		{
			name:      "fallback succeeds after an empty reply",
			primary:   &scriptedProvider{id: "p1", text: "   "},
			fallback:  &scriptedProvider{id: "p2", text: `{"name":"Vela"}`},
			wantValue: "Vela",
			wantBy:    "p2",
			wantCodes: []harness.FailureCode{harness.FailureEmptyResponse},
		},
		{
			name:      "fallback succeeds after unparseable text",
			primary:   &scriptedProvider{id: "p1", text: "no json here"},
			fallback:  &scriptedProvider{id: "p2", text: `{"name":"Vela"}`},
			wantValue: "Vela",
			wantBy:    "p2",
			wantCodes: []harness.FailureCode{harness.FailureParseError},
		},
		{
			name:      "both empty",
			primary:   &scriptedProvider{id: "p1", text: ""},
			wantCodes: []harness.FailureCode{harness.FailureEmptyResponse, harness.FailureEmptyResponse},
		},
		{
			name:      "primary succeeds",
			primary:   &scriptedProvider{id: "p1", text: `{"name":"Vela"}`},
			wantValue: "Vela",
			wantBy:    "p1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := routerWith(tt.primary, tt.fallback)
			out := collectTextAttempts(context.Background(), router, []string{"gm"}, harness.GenerateRequest{Prompt: "hi"})
			if out.GeneratedBy != tt.wantBy {
				t.Fatalf("GeneratedBy = %q, want %q", out.GeneratedBy, tt.wantBy)
			}
			if tt.wantValue != "" && out.Values["name"] != tt.wantValue {
				t.Fatalf("Values = %v, want name=%q", out.Values, tt.wantValue)
			}
			if len(out.Attempts) != len(tt.wantCodes) {
				t.Fatalf("attempts = %d (%v), want %d", len(out.Attempts), out.Attempts, len(tt.wantCodes))
			}
			for i, code := range tt.wantCodes {
				if out.Attempts[i].Code != code {
					t.Fatalf("attempt %d code = %q, want %q", i, out.Attempts[i].Code, code)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestCollectTextAttempts ./pkg/gui/`
Expected: FAIL (`undefined: collectTextAttempts`).

- [ ] **Step 3: Implement the helper**

Create `pkg/gui/generation_attempts.go`:
```go
package gui

import (
	"context"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// generationOutcome is the result of walking a role fallback chain.
type generationOutcome struct {
	Values      map[string]string
	Attempts    []harness.Attempt
	GeneratedBy string
}

// collectTextAttempts asks each role in order and returns the first decodable
// result. It owns per-attempt timing and the empty-attempt synthesis so the text
// and character generators share one fallback implementation.
func collectTextAttempts(ctx context.Context, router *harness.Router, roles []string, request harness.GenerateRequest) generationOutcome {
	out := generationOutcome{Attempts: make([]harness.Attempt, 0, len(roles))}
	for _, role := range roles {
		roleStarted := time.Now()
		resp, err := router.GenerateForRole(ctx, role, request)
		if err != nil {
			if failure, ok := harness.FailureFrom(err); ok {
				out.Attempts = append(out.Attempts, failure.Attempts...)
				if len(failure.Attempts) == 0 {
					out.Attempts = append(out.Attempts, harness.Attempt{
						Role: role, Provider: router.ProviderIDForRole(role),
						Code: failure.Code, Detail: failure.Message,
						DurationMS: time.Since(roleStarted).Milliseconds(),
					})
				}
				continue
			}
			out.Attempts = append(out.Attempts, harness.Attempt{
				Role: role, Provider: router.ProviderIDForRole(role),
				Code: harness.FailureProviderError, Detail: err.Error(),
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		if resp == nil || strings.TrimSpace(resp.Text) == "" {
			out.Attempts = append(out.Attempts, harness.Attempt{
				Role: role, Provider: router.ProviderIDForRole(role),
				Code: harness.FailureEmptyResponse, Detail: "model returned no text",
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		values, decodeErr := decodeGeneratedValuesChecked(resp.Text)
		if decodeErr != nil {
			out.Attempts = append(out.Attempts, harness.Attempt{
				Role: role, Provider: router.ProviderIDForRole(role),
				Code: harness.FailureParseError, Detail: decodeErr.Error(),
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		out.Values = values
		out.GeneratedBy = role
		break
	}
	return out
}
```

- [ ] **Step 4: Refactor `GenerateText` and `GenerateCharacter` to use it**

In `pkg/gui/text_generate.go`, replace the `roles`/`attempts`/`for` loop in `GenerateText` with:
```go
	roles := []string{role}
	if role != "gm" {
		roles = append(roles, "gm")
	}
	outcome := collectTextAttempts(ctx, router, roles, request)
	attempts := outcome.Attempts
	resp.GeneratedBy = outcome.GeneratedBy
	for k, v := range outcome.Values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			resp.Fields[k] = trimmed
		}
	}
	s.recordGenerationAttempts(ctx, span, attempts)
```
Keep the existing `if len(resp.Fields) == 0 { ... }` failure block and the `Warning` block below it unchanged.

In `pkg/gui/character_generate.go`, replace the `roles`/`attempts`/`values` loop with:
```go
	outcome := collectTextAttempts(ctx, router, []string{"character", "gm"}, request)
	attempts := outcome.Attempts
	resp.GeneratedBy = outcome.GeneratedBy
	values := outcome.Values
```
Keep the `for _, field := range generatable` copy, `recordGenerationAttempts`, failure, and warning blocks.

- [ ] **Step 5: Run tests**

Run: `go test -run 'TestCollectTextAttempts|TestGenerateText|TestGenerateCharacter|TestPickFailureCode' ./pkg/gui/` and `go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/generation_attempts.go pkg/gui/generation_attempts_test.go pkg/gui/text_generate.go pkg/gui/character_generate.go
git commit -m "refactor(gui): share the role fallback loop between generators"
```

---

### Task 2: Text span attributes, role metric, free span helpers

**Files:**
- Modify: `pkg/gui/generation_telemetry.go`
- Modify: `pkg/gui/generation_telemetry_test.go`
- Modify: `pkg/gui/text_generate.go`, `pkg/gui/character_generate.go`

**Interfaces:**
- Consumes: `generationOutcome` (Task 1).
- Produces: `startGenerationSpan(ctx context.Context, logger trace.Logger, name, formType, fieldName string) (context.Context, oteltrace.Span)`; `startImageSpan(ctx context.Context, logger trace.Logger, kind string) (context.Context, oteltrace.Span)`; `(*Service).recordGeneration(ctx, span, formType, role string, started time.Time, failure *harness.GenerationFailure)`; `(*Service).setTextOutcome(span oteltrace.Span, outcome generationOutcome, fieldCount int)`.

- [ ] **Step 1: Write the failing test**

Replace the direct-method calls in `pkg/gui/generation_telemetry_test.go` and add:
```go
func TestRecordGenerationCarriesRole(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	service := &Service{}
	ctx, span := startGenerationSpan(context.Background(), service.logger, "generate.text", "world", "name")
	service.recordGeneration(ctx, span, "world", "gm", time.Now(), &harness.GenerationFailure{Code: harness.FailureEmptyResponse, Message: "x"})
	span.End()

	spans := recorder.Spans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	rm, err := recorder.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	var roleSeen bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
				if v, ok := dp.Attributes.Value(attribute.Key("localrpg.role")); ok && v.AsString() == "gm" {
					roleSeen = true
				}
			}
		}
	}
	if !roleSeen {
		t.Fatal("localrpg.role was not recorded on a generation metric")
	}
}

func TestSetTextOutcomeAttributes(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	service := &Service{}
	ctx, span := startGenerationSpan(context.Background(), service.logger, "generate.text", "world", "_all")
	service.setTextOutcome(span, generationOutcome{
		Attempts:    []harness.Attempt{{Role: "gm", Provider: "p1"}},
		GeneratedBy: "gm",
	}, 5)
	span.End()

	var attempts, fields bool
	for _, attr := range recorder.Spans()[0].Attributes() {
		switch attr.Key {
		case attribute.Key("localrpg.generation.attempts"):
			attempts = attr.Value.AsInt64() == 1
		case attribute.Key("localrpg.field_count"):
			fields = attr.Value.AsInt64() == 5
		}
	}
	if !attempts || !fields {
		t.Fatal("setTextOutcome did not set attempts and field_count")
	}
	_ = ctx
}
```
Add imports `go.opentelemetry.io/otel/sdk/metric/metricdata` and `go.opentelemetry.io/otel/attribute` to the test file. Note: the metric assertion assumes a `Sum[int64]`; if `localrpg.generation.errors` is the first metric, adjust the type switch to handle histograms by skipping them. If the type assertion is brittle, instead assert by iterating `m.Data` with a type switch and only checking `Sum[int64]`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestRecordGenerationCarriesRole|TestSetTextOutcomeAttributes' ./pkg/gui/`
Expected: FAIL (`undefined: startGenerationSpan` as a free function, `undefined: setTextOutcome`, wrong `recordGeneration` arity).

- [ ] **Step 3: Update `generation_telemetry.go`**

Replace `startGenerationSpan` with free functions and add the role parameter and outcome setter:
```go
// startGenerationSpan opens a span for one generation request and logs the
// request event. The caller must end the span.
func startGenerationSpan(ctx context.Context, logger trace.Logger, name, formType, fieldName string) (context.Context, oteltrace.Span) {
	trace.LogEvent(ctx, trace.OrNil(logger), "generate.request", map[string]interface{}{
		"span":       name,
		"form_type":  formType,
		"field_name": fieldName,
	})
	return telemetry.Tracer("github.com/darkliquid/localrpg/pkg/gui").Start(ctx, name,
		oteltrace.WithAttributes(
			attribute.String("localrpg.form_type", formType),
			attribute.String("localrpg.field_name", fieldName),
		),
	)
}

// startImageSpan opens the image span. The image attributes are set on
// completion, when the provider identity and byte count are known.
func startImageSpan(ctx context.Context, logger trace.Logger, kind string) (context.Context, oteltrace.Span) {
	trace.LogEvent(ctx, trace.OrNil(logger), "generate.request", map[string]interface{}{
		"span":       "generate.image",
		"form_type":  "image",
		"field_name": kind,
	})
	return telemetry.Tracer("github.com/darkliquid/localrpg/pkg/gui").Start(ctx, "generate.image",
		oteltrace.WithAttributes(attribute.String("localrpg.image.kind", kind)),
	)
}
```
Change `recordGeneration` to accept `role string` and add it to both metric attribute sets:
```go
func (s *Service) recordGeneration(ctx context.Context, span oteltrace.Span, formType, role string, started time.Time, failure *harness.GenerationFailure) {
	// ...existing body...
	generationMetrics().errors.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("localrpg.form_type", formType),
		attribute.String("localrpg.role", role),
		attribute.String("localrpg.generation.failure_code", string(failure.Code)),
	))
	// ...existing failure branch...
	generationMetrics().duration.Record(ctx, float64(elapsed.Milliseconds()), otelmetric.WithAttributes(
		attribute.String("localrpg.form_type", formType),
		attribute.String("localrpg.role", role),
		attribute.String("localrpg.generation.outcome", outcomeLabel(failure)),
	))
}
```
Add:
```go
// setTextOutcome records the result-dependent span attributes. An empty attempt
// list (a request that never reached a provider) omits them.
func (s *Service) setTextOutcome(span oteltrace.Span, outcome generationOutcome, fieldCount int) {
	if span == nil {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 4)
	if len(outcome.Attempts) > 0 {
		last := outcome.Attempts[len(outcome.Attempts)-1]
		attrs = append(attrs,
			attribute.Int("localrpg.generation.attempts", len(outcome.Attempts)),
			attribute.String("gen_ai.system", last.Provider),
		)
	}
	if outcome.GeneratedBy != "" {
		attrs = append(attrs,
			attribute.String("localrpg.generated_by", outcome.GeneratedBy),
			attribute.Int("localrpg.field_count", fieldCount),
		)
	}
	span.SetAttributes(attrs...)
}
```

- [ ] **Step 4: Update the callers**

In `GenerateText`: `ctx, span := startGenerationSpan(ctx, s.logger, "generate.text", req.FormType, req.FieldName)`; after building `resp.Fields`, call `s.setTextOutcome(span, outcome, len(resp.Fields))`; change every `s.recordGeneration(ctx, span, req.FormType, started, ...)` to pass a role (`outcome.GeneratedBy` on success, `roleForAttempts(attempts)` on failure).
Add to `generation_telemetry.go`:
```go
// roleForAttempts names the role that produced the last attempt, for the
// failure metric.
func roleForAttempts(attempts []harness.Attempt) string {
	if len(attempts) == 0 {
		return ""
	}
	return attempts[len(attempts)-1].Role
}
```
In `GenerateCharacter`: same, with `startGenerationSpan(ctx, s.logger, "generate.text", "character", "_all")`, `s.setTextOutcome(span, outcome, len(resp.Values))`, and role `outcome.GeneratedBy`/`roleForAttempts(attempts)`.

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/gui/` and `go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/generation_telemetry.go pkg/gui/generation_telemetry_test.go pkg/gui/text_generate.go pkg/gui/character_generate.go
git commit -m "feat(gui): complete text generation span and metric attributes"
```

---

### Task 3: Unified events and image observability

**Files:**
- Create: `pkg/gui/image_generation.go`
- Create: `pkg/gui/image_generation_test.go`
- Modify: `pkg/gui/service.go` (three asset methods), `pkg/gui/generation_telemetry.go` (image instruments and `recordImage`)

**Interfaces:**
- Consumes: `startImageSpan`, `guardImageBytes`.
- Produces: `(*Service).generateImage(ctx context.Context, kind, prompt string) ([]byte, *harness.GenerationFailure)`; `(*Service).recordImage(ctx context.Context, span oteltrace.Span, kind, provider string, bytes int, started time.Time, failure *harness.GenerationFailure)`; a `media.image.duration` instrument.

- [ ] **Step 1: Write the failing test**

Create `pkg/gui/image_generation_test.go`:
```go
package gui

import (
	"context"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

func TestRecordImageEmitsEventsAndDuration(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	mem := trace.NewMemory(trace.LevelSummary)
	service := &Service{logger: mem}
	ctx, span := startImageSpan(context.Background(), service.logger, "banner")
	service.recordImage(ctx, span, "banner", "gemini", 1024, time.Now(), nil)
	span.End()

	var sawBytes bool
	for _, attr := range recorder.Spans()[0].Attributes() {
		if attr.Key == attribute.Key("localrpg.image.bytes") && attr.Value.AsInt64() == 1024 {
			sawBytes = true
		}
	}
	if !sawBytes {
		t.Fatal("localrpg.image.bytes was not set")
	}
	if _, ok := mem.Find("generate.complete"); !ok {
		t.Fatalf("events = %v, want generate.complete", mem.Names())
	}
}

func TestRecordImageFailureEmitsGenerateError(t *testing.T) {
	mem := trace.NewMemory(trace.LevelSummary)
	service := &Service{logger: mem}
	ctx, span := startImageSpan(context.Background(), service.logger, "icon")
	service.recordImage(ctx, span, "icon", "", 0, time.Now(), &harness.GenerationFailure{Code: harness.FailureProviderError, Message: "no data"})
	span.End()
	if _, ok := mem.Find("generate.error"); !ok {
		t.Fatalf("events = %v, want generate.error", mem.Names())
	}
}
```
Add the `trace` import. This tests the new `recordImage` directly; `generateImage` (which needs a real client) is covered by `guardImageBytes` and by inspection.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestRecordImage' ./pkg/gui/`
Expected: FAIL (`undefined: recordImage`).

- [ ] **Step 3: Add image instruments and `recordImage`**

In `pkg/gui/generation_telemetry.go`, add `imageDuration otelmetric.Float64Histogram` to `generationInstruments` and build it:
```go
imageDuration: telemetry.Float64Histogram(meter, "localrpg.media.image.duration", "ms", "Duration of one image generation."),
```
Add:
```go
// recordImage writes the outcome of one image request: span attributes, the
// image duration, the shared generation metrics, and the unified generate.*
// events.
func (s *Service) recordImage(ctx context.Context, span oteltrace.Span, kind, provider string, size int, started time.Time, failure *harness.GenerationFailure) {
	elapsed := time.Since(started)
	fields := map[string]interface{}{
		"form_type":   "image",
		"image_kind":  kind,
		"provider":    provider,
		"bytes":       size,
		"duration_ms": elapsed.Milliseconds(),
	}
	if span != nil {
		span.SetAttributes(
			attribute.String("localrpg.image.provider", provider),
			attribute.Int("localrpg.image.bytes", size),
		)
	}
	generationMetrics().imageDuration.Record(ctx, float64(elapsed.Milliseconds()), otelmetric.WithAttributes(
		attribute.String("localrpg.image.provider", provider),
	))
	if failure != nil {
		fields["code"] = string(failure.Code)
		fields["message"] = failure.Message
		generationMetrics().errors.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.form_type", "image"),
			attribute.String("localrpg.role", "image"),
			attribute.String("localrpg.generation.failure_code", string(failure.Code)),
		))
		if span != nil {
			span.SetAttributes(attribute.String("localrpg.generation.failure_code", string(failure.Code)))
			span.SetStatus(codes.Error, string(failure.Code))
			span.RecordError(failure)
			span.AddEvent("error", oteltrace.WithAttributes(attribute.String("localrpg.generation.failure_code", string(failure.Code))))
		}
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.error", fields)
	} else {
		trace.LogEvent(ctx, trace.OrNil(s.logger), "generate.complete", fields)
	}
	generationMetrics().duration.Record(ctx, float64(elapsed.Milliseconds()), otelmetric.WithAttributes(
		attribute.String("localrpg.form_type", "image"),
		attribute.String("localrpg.role", "image"),
		attribute.String("localrpg.generation.outcome", outcomeLabel(failure)),
	))
}
```

- [ ] **Step 4: Implement `generateImage` and delegate**

Create `pkg/gui/image_generation.go`:
```go
package gui

import (
	"context"
	"fmt"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
)

// generateImage runs one image request: span, provider call, byte guard, and
// observability. It never persists.
func (s *Service) generateImage(ctx context.Context, kind, prompt string) ([]byte, *harness.GenerationFailure) {
	started := time.Now()
	ctx, span := startImageSpan(ctx, s.logger, kind)
	defer span.End()

	cfg := s.configMgr.Get()
	client, err := media.NewImageClientWithSharedKey(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
	if err != nil {
		failure := &harness.GenerationFailure{Code: harness.FailureProviderUnavailable, Message: fmt.Sprintf("image provider: %v", err)}
		s.recordImage(ctx, span, kind, "", 0, started, failure)
		return nil, failure
	}

	provider := cfg.Media.Image.BuiltinName
	if provider == "" {
		provider = cfg.Media.Image.Type
	}

	imgBytes, err := client.GenerateImage(ctx, prompt)
	if err != nil {
		failure := &harness.GenerationFailure{Code: harness.ClassifyProviderError(err), Message: fmt.Sprintf("generate image: %v", err)}
		s.recordImage(ctx, span, kind, provider, 0, started, failure)
		return nil, failure
	}
	checked, failure := guardImageBytes(imgBytes)
	if failure != nil {
		s.recordImage(ctx, span, kind, provider, 0, started, failure)
		return nil, failure
	}
	s.recordImage(ctx, span, kind, provider, len(checked), started, nil)
	return checked, nil
}
```
In `pkg/gui/service.go`, rewrite the three methods to delegate:
```go
func (s *Service) GenerateAssetPreview(ctx context.Context, req GenerateAssetPreviewRequestDTO) ([]byte, string, error) {
	imgBytes, failure := s.generateImage(ctx, req.Kind, buildAssetPrompt(req.Kind, req.Name, req.Description, req.ArtStyle, req.Genre))
	if failure != nil {
		return nil, "", failure
	}
	return imgBytes, imageContentType(imgBytes), nil
}
```
Add `imageContentType([]byte) string` (move the existing `switch media.ArtExtension` block into it), and update `GenerateGameAsset`/`GenerateWorldAsset` to build their prompt, call `s.generateImage`, and either `SaveGameAsset`/`SaveWorldAsset` or return the failure. Remove the now-duplicate span/record generation code from those two methods.

This also closes D7: because every asset path records through `recordImage`, the `generate.error` event is emitted once by the service, and `writeGenerationFailure` stays body-only (no double event). A test asserts exactly one `generate.error` for a text failure in Task 6.

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/gui/` and `go build ./...`
Expected: PASS. Existing asset tests (`TestBuildAssetPrompt`, `TestGuardImageBytes`) still pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/image_generation.go pkg/gui/image_generation_test.go pkg/gui/generation_telemetry.go pkg/gui/service.go
git commit -m "feat(gui): unify generation events and instrument image generation"
```

---

### Task 4: Complete turn failure diagnostics

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/orchestrator_failure_test.go` (extend)

**Interfaces:**
- Consumes: `harness.GenerationFailure`.
- Produces: `streamResult.ChunkCount int`; `generation.error` events carrying `code`, `role`, `provider`, `prompt_chars`, `context_chars`, `elapsed_ms`, `chunk_count`, `partial_chars`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/engine/orchestrator_failure_test.go`:
```go
func TestStreamCountsChunks(t *testing.T) {
	o := &TurnOrchestrator{chunkTimeout: time.Second}
	chunks := &countingProvider{chunks: []string{"a", "b", "c"}}
	result, err := o.stream(context.Background(), chunks, harness.GenerateRequest{}, nil)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if result.ChunkCount != 3 {
		t.Fatalf("ChunkCount = %d, want 3", result.ChunkCount)
	}
}

type countingProvider struct{ chunks []string }

func (c *countingProvider) ID() string { return "counting" }

func (c *countingProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{}, nil
}

func (c *countingProvider) Stream(_ context.Context, _ harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	for _, text := range c.chunks {
		out <- harness.StreamChunk{Text: text}
	}
	close(out)
	return nil
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestStreamCountsChunks ./pkg/engine/`
Expected: FAIL (`undefined: ChunkCount`).

- [ ] **Step 3: Implement**

In `pkg/engine/orchestrator.go`:
- Add `ChunkCount int` to `streamResult`.
- In `stream`, declare `chunkCount := 0`, increment it in the loop when `chunk.Text != ""` or `len(chunk.ToolCalls) > 0`, and include it in every returned `streamResult` (both the success and `interrupted` paths).
- In the `ProcessActionStream` failure branch, replace the fields map with:
```go
		fields := map[string]interface{}{
			"code":          generationCode(failure),
			"error":         err.Error(),
			"role":          "gm",
			"provider":      result.ProviderID,
			"prompt_chars":  len([]rune(actionInput)),
			"context_chars": len([]rune(assembly.Prompt)),
			"elapsed_ms":    time.Since(turnStarted).Milliseconds(),
			"chunk_count":   result.ChunkCount,
			"partial_chars": len([]rune(result.Text)),
		}
```
- In the empty-narration branch, log `code`, `error`, `role`, `provider`, `prompt_chars`, `elapsed_ms`, `attempts`, `finish_reason`:
```go
		o.logger.Event("generation.error", map[string]interface{}{
			"code":         string(failure.Code),
			"error":        failure.Message,
			"role":         "gm",
			"provider":     result.ProviderID,
			"prompt_chars": len([]rune(actionInput)),
			"elapsed_ms":   time.Since(turnStarted).Milliseconds(),
			"attempts":     len(failure.Attempts),
			"finish_reason": result.FinishReason,
		})
```

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_failure_test.go
git commit -m "feat(engine): complete turn generation failure diagnostics"
```

---

### Task 5: `StreamForRole` parity

**Files:**
- Modify: `pkg/harness/router.go`
- Test: `pkg/harness/router_failure_test.go` (extend)

**Interfaces:**
- Produces: `StreamForRole` returns a `*GenerationFailure` when every provider closes without a chunk and records an `Attempt` for each.

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/router_failure_test.go`:
```go
type chunkProvider struct {
	id     string
	chunks []StreamChunk
}

func (p *chunkProvider) ID() string { return p.id }

func (p *chunkProvider) Generate(context.Context, GenerateRequest) (*GenerateResponse, error) {
	return &GenerateResponse{}, nil
}

func (p *chunkProvider) Stream(_ context.Context, _ GenerateRequest, out chan<- StreamChunk) error {
	for _, chunk := range p.chunks {
		out <- chunk
	}
	close(out)
	return nil
}

func TestStreamForRoleFallsBackOnNoChunks(t *testing.T) {
	router := NewRouter()
	router.RegisterProvider(&chunkProvider{id: "primary"})
	router.RegisterProvider(&chunkProvider{id: "fallback", chunks: []StreamChunk{{Text: "hello"}}})
	router.AssignRole("gm", "primary")
	router.SetFallback("gm", "fallback")

	out := make(chan StreamChunk, 8)
	if err := router.StreamForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"}, out); err != nil {
		t.Fatalf("StreamForRole: %v", err)
	}
	var text string
	for chunk := range out {
		text += chunk.Text
	}
	if text != "hello" {
		t.Fatalf("text = %q, want the fallback text", text)
	}
}

func TestStreamForRoleEmptyEverywhereIsFailure(t *testing.T) {
	router := NewRouter()
	router.RegisterProvider(&chunkProvider{id: "primary"})
	router.RegisterProvider(&chunkProvider{id: "fallback"})
	router.AssignRole("gm", "primary")
	router.SetFallback("gm", "fallback")

	out := make(chan StreamChunk, 8)
	err := router.StreamForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"}, out)
	failure, ok := FailureFrom(err)
	if !ok || failure.Code != FailureEmptyResponse {
		t.Fatalf("error = %v, want empty_response", err)
	}
	if len(failure.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(failure.Attempts))
	}
}
```
Add `streamsSend` handling: the current `StreamForRole` closes `out` itself in each path; the tests range over it, so that contract must be preserved.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestStreamForRole ./pkg/harness/`
Expected: FAIL (empty-everywhere returns a nil/raw error, not a typed failure).

- [ ] **Step 3: Implement**

Rewrite `StreamForRole` in `pkg/harness/router.go`:
```go
func (r *Router) StreamForRole(ctx context.Context, role string, req GenerateRequest, out chan<- StreamChunk) error {
	primary, err := r.GetProviderForRole(role)
	if err != nil {
		close(out)
		return &GenerationFailure{
			Code:    FailureProviderUnavailable,
			Message: fmt.Sprintf("no provider available for role %q: %v", role, err),
		}
	}

	attempts := make([]Attempt, 0, 2)
	started := time.Now()

	streamed, streamErr := r.forwardStream(ctx, role, primary, req, out, false)
	if streamed {
		return streamErr
	}
	attempts = append(attempts, Attempt{
		Role: role, Provider: primary.ID(),
		Code: FailureEmptyResponse, Detail: "provider streamed no text",
		DurationMS: time.Since(started).Milliseconds(),
	})

	if fallback, ok := r.FallbackForRole(role); ok && fallback != nil {
		streamed, streamErr = r.forwardStream(ctx, role, fallback, req, out, true)
		if streamed {
			return streamErr
		}
		attempts = append(attempts, Attempt{
			Role: role, Provider: fallback.ID(),
			Code: FailureEmptyResponse, Detail: "provider streamed no text",
			DurationMS: time.Since(started).Milliseconds(),
		})
	}

	close(out)
	return &GenerationFailure{
		Code:      FailureEmptyResponse,
		Message:   fmt.Sprintf("role %q streamed no text", role),
		Attempts:  attempts,
		ElapsedMS: time.Since(started).Milliseconds(),
	}
}

// forwardStream pushes a provider's chunks to out, but only once it has seen a
// usable first chunk. It returns whether anything was forwarded. A provider that
// reports its first-chunk error is treated as having streamed nothing, so the
// caller can fall back; if first is true the channel is left open for the caller.
func (r *Router) forwardStream(ctx context.Context, role string, provider ModelProvider, req GenerateRequest, out chan<- StreamChunk, first bool) (bool, error) {
	tempOut := make(chan StreamChunk, 20)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(ctx, req, tempOut)
	}()

	chunk, ok := <-tempOut
	if !ok || chunk.Error != nil {
		go func() {
			for range tempOut {
			}
		}()
		<-errCh
		return false, nil
	}
	go func() {
		defer close(out)
		out <- chunk
		for rest := range tempOut {
			out <- rest
		}
	}()
	return true, <-errCh
}
```
Note: `forwardStream` is called for the primary with `first=false`, and for the fallback with `first=true`; the `first` flag is documentation only here (both paths close `out` on success). Simplify by dropping the parameter if `go vet` flags it, and keep the closing contract consistent: on fallback success the goroutine closes `out`.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/harness/` and `go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/router.go pkg/harness/router_failure_test.go
git commit -m "fix(harness): report stream failures with the shared failure type"
```

---

### Task 6: One JSON error shape for generation routes

**Files:**
- Modify: `pkg/gui/generation_errors.go`, `pkg/gui/text_generate.go`, `pkg/gui/character_generate.go`, `pkg/gui/server.go`
- Test: `pkg/gui/generation_errors_test.go`, `pkg/gui/world_crud_test.go`

**Interfaces:**
- Produces: `writeInvalidRequest(w http.ResponseWriter, message string)`; `writeJSONError(w http.ResponseWriter, status int, message string)`; `writeGenerationError` uses `writeJSON`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/generation_errors_test.go`:
```go
func TestWriteInvalidRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	writeInvalidRequest(rec, "invalid request body")
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body struct {
		Error harness.GenerationFailure `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Error.Code != harness.FailureInvalidRequest {
		t.Fatalf("code = %q, want invalid_request", body.Error.Code)
	}
}

func TestWriteJSONError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSONError(rec, 409, "world already exists")
	if rec.Code != 409 {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "world already exists") {
		t.Fatalf("body = %q, want the message", rec.Body.String())
	}
}
```
Add `strings` to the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestWriteInvalidRequest|TestWriteJSONError' ./pkg/gui/`
Expected: FAIL (`undefined: writeInvalidRequest`, `undefined: writeJSONError`).

- [ ] **Step 3: Implement**

In `pkg/gui/generation_errors.go`:
```go
// writeJSONError writes a JSON error envelope with an explicit status, for
// non-generation failures such as a world conflict.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"message": message},
	})
}

// writeInvalidRequest writes a 400 in the same shape as every other generation
// failure, so clients parse one error type.
func writeInvalidRequest(w http.ResponseWriter, message string) {
	writeGenerationError(w, &harness.GenerationFailure{
		Code:    harness.FailureInvalidRequest,
		Message: message,
	})
}
```
Change `writeGenerationError` to use `writeJSON`:
```go
func writeGenerationError(w http.ResponseWriter, failure *harness.GenerationFailure) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(generationStatus(failure.Code))
	writeJSON(w, map[string]interface{}{"error": failure})
}
```
(`writeJSON` writes the header itself; drop the duplicate `WriteHeader` and keep only the status line — verify `writeJSON` sets `Content-Type` and calls `WriteHeader`. If `writeJSON` always writes 200, keep the explicit `w.WriteHeader` before calling it.)

In `pkg/gui/text_generate.go`, `pkg/gui/character_generate.go`, and `pkg/gui/server.go`, replace every malformed-body `http.Error(w, ..., http.StatusBadRequest)` on a generation route with `writeInvalidRequest(w, ...)`, including the two `generate-asset` handlers, the preview handler, and `handleTurnSubmit`'s decode and `validate` errors.

In `pkg/gui/server.go`, replace the world create duplicate `http.Error(w, err.Error(), http.StatusConflict)` with `writeJSONError(w, http.StatusConflict, err.Error())`; leave the update `404` as-is or convert it to `writeJSONError` for consistency.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/gui/` and `go build ./...`
Expected: PASS. Update the turn test `TestHandleGenerateTextRoute_FailureIsStructured` only if it asserts a text body. Extend `TestGenerateTextEmitsTraceEvents` to assert exactly one `generate.error` event (closes D7 by proving no duplicate).

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/generation_errors.go pkg/gui/generation_errors_test.go pkg/gui/text_generate.go pkg/gui/character_generate.go pkg/gui/server.go pkg/gui/world_crud_test.go
git commit -m "feat(gui): return one JSON error shape for generation routes"
```

---

### Task 7: Wire `onError` and highlight the slug field

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`, `frontend/src/components/SystemsStudio.tsx`, `frontend/src/components/launcher/NewCampaignModal.tsx`, `frontend/src/components/launcher/CampaignSettingsModal.tsx`

**Interfaces:**
- Consumes: `AIGenerateButtonProps.onError`, `GenerationFailure`.
- Produces: a toast on every button failure and a red slug field on conflict.

- [ ] **Step 1: Wire `onError` in WorldsStudio and SystemsStudio**

Add a shared reporter beside each `setToast`:
```tsx
const reportGenerationError = (failure: GenerationFailure) =>
  setToast({ type: 'error', message: `${failure.code}: ${failure.message}` });
```
Import `GenerationFailure` from `../types`. Add `onError={reportGenerationError}` to every `<AIGenerateButton>` (WorldsStudio: name, genre, art_style, description, lore_prompt; SystemsStudio: name, description, rules_prompt).

- [ ] **Step 2: Wire `onError` in the launcher modals**

`NewCampaignModal.tsx` already has `genError`; add
`onError={(failure) => setGenError(`${failure.code}: ${failure.message}`)}` to its buttons and import `GenerationFailure` if the inline type is needed. `CampaignSettingsModal.tsx` has its own error state; wire the same way (inspect it first to confirm the state setter name).

- [ ] **Step 3: Highlight the slug field**

In `WorldsStudio.tsx`: add `const [slugError, setSlugError] = useState(false)`; set `setSlugError(true)` in the `WorldExistsError` catch; clear it in the slug and name `onChange` handlers and in `handleNewWorld`. On the slug input add:
```tsx
className={`... ${slugError ? 'border-red-500/70 focus:border-red-500' : 'border-stone-800 focus:border-purple-500/50'}`}
```
and below it:
```tsx
{slugError && (
  <p className="text-[11px] text-red-400">That id already exists. Change the name or slug.</p>
)}
```

- [ ] **Step 4: Verify**

Run: `mise run test:frontend && mise run build:frontend`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx frontend/src/components/SystemsStudio.tsx frontend/src/components/launcher/NewCampaignModal.tsx frontend/src/components/launcher/CampaignSettingsModal.tsx
git commit -m "feat(frontend): surface generation errors at every AI button"
```

---

### Task 8: World Studio selection robustness

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`

**Interfaces:**
- Produces: a last-click-wins `loadWorldDetail`; a `startMode` ref; typed `catch`.

- [ ] **Step 1: Add the request token**

```tsx
const detailRequest = React.useRef(0);
```
In `loadWorldDetail`, after `const detail = await APIClient.getWorld(id);`, add:
```tsx
const token = ++detailRequest.current;
```
and after every await that mutates state, guard:
```tsx
if (token !== detailRequest.current) return;
```
Bump `detailRequest.current += 1` at the start of `handleNewWorld`.

- [ ] **Step 2: Capture `startMode` in a ref**

```tsx
const startModeRef = React.useRef(startMode);
useEffect(() => {
  loadWorlds(undefined, startModeRef.current);
  // eslint-disable-next-line react-hooks/exhaustive-deps
}, []);
```
(If the repo's lint does not run `react-hooks`, the disable comment is harmless; keep it.)

- [ ] **Step 3: Type the catches**

Add:
```tsx
const errorMessage = (err: unknown): string =>
  err instanceof Error ? err.message : 'Unexpected error';
```
Replace `catch (err: any) { ... err.message ... }` with `catch (err) { ... errorMessage(err) ... }` throughout the file. Keep the `WorldExistsError` branch (it is an `Error`).

- [ ] **Step 4: Verify**

Run: `mise run test:frontend && mise run build:frontend`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx
git commit -m "fix(frontend): make world selection last-click-wins"
```

---

### Task 9: Full verification

- [ ] **Step 1: Run everything**

Run: `mise run test` and `mise run lint`
Expected: PASS (`go test -v -count=1 ./...`, `npx tsc --noEmit`, `go vet ./...`).

- [ ] **Step 2: Manual checklist**

1. Configure the `gm`/`character` roles as a CLI provider that prints nothing; confirm the inline error and the form toast fire at every call site.
2. Create a world with a duplicate name; confirm the red slug field and the conflict toast.
3. Click rapidly between two saved worlds; confirm the editor settles on the last click.
4. With telemetry enabled, confirm `generate.text` has `attempts`/`field_count`/`gen_ai.system`, `generate.image` has `localrpg.image.bytes`, and `localrpg.generation.errors` has `localrpg.role`.
5. Run a turn against a stalling provider; confirm `generation.error` logs `code`, `chunk_count`, and `partial_chars`, and the banner names the timeout.
6. Confirm a malformed `POST /api/generate-text` returns `400` with `{"error":{"code":"invalid_request"}}`.

- [ ] **Step 3: Commit any fixups**

```bash
git add -A
git commit -m "test: verify generation and studio follow-ups"
```
