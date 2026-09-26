# Provider Error Surfacing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure the provider's own error text reaches logs, traces, and the user for LLM, image, TTS, and STT failures, instead of being discarded or replaced by a generic message.

**Architecture:** Add a bounded `provider.TruncateDetail` helper, stop adapters from dropping bodies/stderr, add `harness.SummarizeAttempts`/`GenerationFailure.Summary()`, wrap media TTS/STT errors as `harness.GenerationFailure`, route STT/audio failures through `writeGenerationFailure`, and show the message (not just the code) in the frontend.

**Tech Stack:** Go 1.27 (stdlib tests, no testify), OpenTelemetry, React 19 + TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-26-provider-error-surfacing-design.md`

## Global Constraints

- Use `interface{}`, not `any`. Wrap errors with `fmt.Errorf("...: %w", err)`. `go vet` must stay clean.
- Tests use only `testing` and `t.TempDir()`; no testify.
- `pkg/provider` is a leaf package and must stay free of internal imports; the new helper is stdlib-only.
- `pkg/media` may import `pkg/harness` and `pkg/trace`: `pkg/harness` does not import `pkg/media` or any provider adapter, so there is no cycle.
- Bound every captured body/stderr to `provider.MaxProviderDetailBytes`.
- Never include secrets in errors; do not capture `Authorization` headers.
- Test gate: `go test -count=1 ./pkg/...` and `go vet ./...`. Frontend gate: `mise run test:frontend`.
- Do not commit unless the user asks.

---

## File Map

- Create: `pkg/provider/detail.go`, `pkg/provider/detail_test.go`
- Modify: `pkg/provider/openaichat/http.go`, `pkg/provider/openaichat/http_test.go`
- Modify: `pkg/provider/geminillm/provider.go`, `pkg/provider/geminillm/provider_test.go`
- Modify: `pkg/provider/imagegemini/client.go`, `pkg/provider/imagegemini/client_test.go`
- Modify: `pkg/provider/imagehttp/client.go`
- Modify: `pkg/provider/ttshttp/client.go`
- Modify: `pkg/provider/clillm/cli.go`
- Modify: `pkg/provider/ttspiper/client.go`
- Modify: `pkg/provider/sttwhispercli/client.go`
- Modify: `pkg/harness/failure.go`, `pkg/harness/failure_test.go`
- Modify: `pkg/harness/router.go`
- Modify: `pkg/gui/text_generate.go`, `pkg/gui/character_generate.go`
- Modify: `pkg/media/metrics.go`, `pkg/media/providers.go`, `pkg/media/tts.go`, `pkg/media/stt.go`
- Modify: `pkg/media/tts_test.go` if present (else create `pkg/media/providers_test.go`)
- Modify: `pkg/gui/service.go`, `pkg/gui/server.go`, `pkg/gui/types.go`
- Create: `frontend/src/lib/generationError.ts`
- Modify: `frontend/src/api/client.ts`, `frontend/src/App.tsx`, `frontend/src/components/ui/AIGenerateButton.tsx`, `frontend/src/components/WorldsStudio.tsx`, `frontend/src/components/SystemsStudio.tsx`

---

### Task 1: Bounded provider detail helper

**Files:**
- Create: `pkg/provider/detail.go`
- Create: `pkg/provider/detail_test.go`

**Interfaces:**
- Produces: `provider.MaxProviderDetailBytes` (int), `provider.TruncateDetail([]byte) string`, `provider.TruncateDetailString(string) string`.

- [x] **Step 1: Write the failing test**

```go
package provider

import (
	"strings"
	"testing"
)

func TestTruncateDetailStringTrims(t *testing.T) {
	if got := TruncateDetailString("  boom  "); got != "boom" {
		t.Fatalf("got %q, want %q", got, "boom")
	}
	if got := TruncateDetailString("   "); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestTruncateDetailStringBounds(t *testing.T) {
	long := strings.Repeat("x", MaxProviderDetailBytes+100)
	got := TruncateDetailString(long)
	if len([]rune(got)) != MaxProviderDetailBytes+3 {
		t.Fatalf("runes = %d, want %d", len([]rune(got)), MaxProviderDetailBytes+3)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("got %q, want a trailing ellipsis", got)
	}
}

func TestTruncateDetailAcceptsBytes(t *testing.T) {
	if got := TruncateDetail([]byte("short")); got != "short" {
		t.Fatalf("got %q", got)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/provider/ -run TestTruncateDetail -v`
Expected: FAIL, `undefined: TruncateDetailString`.

- [x] **Step 3: Write the implementation**

```go
package provider

import "strings"

// MaxProviderDetailBytes bounds how much of a provider's response body or CLI
// stderr is kept in an error, so a large HTML error page cannot bloat a failure
// that is also written to the trace and to the HTTP response.
const MaxProviderDetailBytes = 8192

// TruncateDetail renders a provider payload as a compact, bounded string for an
// error message. It truncates on a rune boundary so the result stays valid UTC-8.
func TruncateDetail(data []byte) string {
	return TruncateDetailString(string(data))
}

// TruncateDetailString is TruncateDetail for an already-stringified payload such
// as captured stderr.
func TruncateDetailString(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) > MaxProviderDetailBytes {
		return string(runes[:MaxProviderDetailBytes]) + "..."
	}
	return trimmed
}
```

- [x] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/provider/ -run TestTruncateDetail -v`
Expected: PASS.

---

### Task 2: Preserve provider body and stderr in adapters

**Files:**
- Modify: `pkg/provider/openaichat/http.go`, `pkg/provider/openaichat/http_test.go`
- Modify: `pkg/provider/geminillm/provider.go`, `pkg/provider/geminillm/provider_test.go`
- Modify: `pkg/provider/imagegemini/client.go`, `pkg/provider/imagegemini/client_test.go`
- Modify: `pkg/provider/imagehttp/client.go`, `pkg/provider/ttshttp/client.go`
- Modify: `pkg/provider/clillm/cli.go`, `pkg/provider/ttspiper/client.go`, `pkg/provider/sttwhispercli/client.go`

**Interfaces:**
- Consumes: `provider.TruncateDetail`/`TruncateDetailString`.

- [x] **Step 1: openaichat — include the non-200 body**

In `pkg/provider/openaichat/http.go`, add `"io"` and `"github.com/darkliquid/localrpg/pkg/provider"` to the imports, then replace the `if resp.StatusCode != http.StatusOK {` block body so the bounded body is read once and included:

```go
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		detail := provider.TruncateDetail(body)
		h.logger.Event("provider.error", map[string]interface{}{
			"role":   h.id,
			"status": resp.Status,
			"url":    url,
			"detail": detail,
		})
		if allowTools && len(req.Tools) > 0 && resp.StatusCode == http.StatusBadRequest {
			h.logger.Event("provider.tools", map[string]interface{}{
				"role":     h.id,
				"offered":  len(req.Tools),
				"rejected": true,
				"reason":   "the provider rejected the tools field",
			})
			return h.streamOnce(ctx, req, out, false)
		}
		if detail != "" {
			return fmt.Errorf("http error %s from %s: %s", resp.Status, url, detail)
		}
		return fmt.Errorf("http error %s from %s", resp.Status, url)
	}
```

- [x] **Step 2: openaichat test**

Add to `http_test.go`:

```go
func TestStreamIncludesProviderBodyInError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model name"}}`))
	}))
	defer srv.Close()

	p := NewHTTPProvider("openai-test", srv.URL, "test-model", "")
	out := make(chan harness.StreamChunk, 1)
	err := p.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, out)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "bad model name") {
		t.Fatalf("error %q does not include the provider body", err.Error())
	}
}
```

Add `net/http`, `net/http/httptest`, `strings` imports if missing.

Run: `go test ./pkg/provider/openaichat/ -run TestStreamIncludesProviderBodyInError -v`
Expected: PASS.

- [x] **Step 3: geminillm — bound the body and keep the original message**

In `pkg/provider/geminillm/provider.go`, change the body read (around line 187) to bound it:

```go
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
```

and replace `mapGeminiError` with (add the `provider` import):

```go
func mapGeminiError(err error) error {
	if err == nil {
		return nil
	}
	errStr := provider.TruncateDetailString(err.Error())
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") || strings.Contains(errStr, "PERMISSION_DENIED") {
		return fmt.Errorf("gemini: invalid API key or permission denied; check providers.gemini.api_key or GEMINI_API_KEY (provider: %s)", errStr)
	}
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "RESOURCE_EXHAUSTED") {
		return fmt.Errorf("gemini: quota exceeded or rate limit reached; check your Google AI Studio plan and credits (provider: %s)", errStr)
	}
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "NOT_FOUND") {
		return fmt.Errorf("gemini: model not found (provider: %s)", errStr)
	}
	return fmt.Errorf("gemini: request failed: %w", err)
}
```

Add a test asserting the original survives:

```go
func TestMapGeminiErrorKeepsOriginal(t *testing.T) {
	err := MapGeminiErrorForTest(errors.New("status 401: API key not valid"))
	if !strings.Contains(err.Error(), "API key not valid") {
		t.Fatalf("error %q dropped the provider message", err.Error())
	}
}
```

Run: `go test ./pkg/provider/geminillm/ -run TestMapGeminiErrorKeepsOriginal -v`
Expected: PASS.

- [x] **Step 4: imagegemini — same treatment**

Replace `mapGeminiImageError` in `pkg/provider/imagegemini/client.go` (add the `provider` import):

```go
func mapGeminiImageError(err error) error {
	if err == nil {
		return nil
	}
	errStr := provider.TruncateDetailString(err.Error())
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") || strings.Contains(errStr, "PERMISSION_DENIED") {
		return fmt.Errorf("gemini image: invalid API key or permission denied; check media.image.api_key, providers.gemini.api_key, or GEMINI_API_KEY (provider: %s)", errStr)
	}
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "RESOURCE_EXHAUSTED") {
		return fmt.Errorf("gemini image: quota exceeded or rate limit reached; check your Google AI Studio plan (provider: %s)", errStr)
	}
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "NOT_FOUND") {
		return fmt.Errorf("gemini image: model not found (provider: %s)", errStr)
	}
	return fmt.Errorf("gemini image: generation failed: %w", err)
}
```

Test:

```go
func TestMapGeminiImageErrorKeepsOriginal(t *testing.T) {
	err := MapGeminiImageErrorForTest(errors.New("status 429: quota exceeded"))
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("error %q dropped the provider message", err.Error())
	}
}
```

Run: `go test ./pkg/provider/imagegemini/ -run TestMapGeminiImageErrorKeepsOriginal -v`
Expected: PASS.

- [x] **Step 5: imagehttp — bound bodies and fetch detail**

In `pkg/provider/imagehttp/client.go` (add `provider` import), replace the three `io.ReadAll(resp.Body)` body echoes with bounded reads:

```go
	b, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
	return nil, fmt.Errorf("comfyui prompt failed (%d): %s", resp.StatusCode, provider.TruncateDetail(b))
```

```go
	b, _ := io.ReadAll(io.LimitReader(viewResp.Body, provider.MaxProviderDetailBytes))
	return nil, fmt.Errorf("comfyui view failed (%d): %s", viewResp.StatusCode, provider.TruncateDetail(b))
```

```go
	b, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
	return nil, fmt.Errorf("http image failed (%d): %s", resp.StatusCode, provider.TruncateDetail(b))
```

And the URL fetch:

```go
			if getResp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(io.LimitReader(getResp.Body, provider.MaxProviderDetailBytes))
				return nil, fmt.Errorf("fetch image url failed (%d): %s", getResp.StatusCode, provider.TruncateDetail(body))
			}
```

- [x] **Step 6: ttshttp — bound bodies**

Apply the same `io.LimitReader(..., provider.MaxProviderDetailBytes)` + `provider.TruncateDetail` pattern at `pkg/provider/ttshttp/client.go:112-115` and `:152-155`.

- [x] **Step 7: clillm — bound stderr**

In `pkg/provider/clillm/cli.go` (add `provider` import), replace the two `stderr.String()` arguments:

```go
		return nil, fmt.Errorf("cli provider %q failed: %w (stderr: %s)", c.id, err, provider.TruncateDetailString(stderr.String()))
```

```go
		return fmt.Errorf("cli process finished with error: %w (stderr: %s)", err, provider.TruncateDetailString(stderr.String()))
```

- [x] **Step 8: ttspiper / sttwhispercli — capture stderr**

Replace the body of `Synthesize` in `pkg/provider/ttspiper/client.go` (add `provider` import):

```go
	cmd := exec.CommandContext(ctx, c.command, c.args...)
	cmd.Stdin = bytes.NewBufferString(text)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cli tts error: %w (stderr: %s)", err, provider.TruncateDetailString(stderr.String()))
	}
	return out.Bytes(), nil
```

Replace the body of `Transcribe` in `pkg/provider/sttwhispercli/client.go` the same way:

```go
	cmd := exec.CommandContext(ctx, c.command, c.args...)
	cmd.Stdin = bytes.NewReader(audioData)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cli stt error: %w (stderr: %s)", err, provider.TruncateDetailString(stderr.String()))
	}
	return out.String(), nil
```

- [x] **Step 9: Run the package tests and vet**

Run: `go test -count=1 ./pkg/provider/... && go vet ./pkg/provider/...`
Expected: PASS, clean.

---

### Task 3: `SummarizeAttempts` / `Summary()`, used by router and text generators

**Files:**
- Modify: `pkg/harness/failure.go`, `pkg/harness/failure_test.go`
- Modify: `pkg/harness/router.go`
- Modify: `pkg/gui/text_generate.go`, `pkg/gui/character_generate.go`

**Interfaces:**
- Produces: `harness.SummarizeAttempts(attempts []Attempt, fallback string) string`, `(*GenerationFailure).Summary() string`.

- [x] **Step 1: Write the failing test**

Add to `pkg/harness/failure_test.go`:

```go
func TestSummarizeAttemptsPrefersLastDetail(t *testing.T) {
	attempts := []Attempt{
		{Role: "gm", Provider: "a", Code: FailureProviderError, Detail: "first"},
		{Role: "gm", Provider: "b", Code: FailureEmptyResponse, Detail: "second"},
	}
	if got := SummarizeAttempts(attempts, "generic"); got != "second" {
		t.Fatalf("got %q, want %q", got, "second")
	}
	if got := SummarizeAttempts(nil, "generic"); got != "generic" {
		t.Fatalf("got %q, want %q", got, "generic")
	}
}

func TestGenerationFailureSummary(t *testing.T) {
	if got := (&GenerationFailure{Code: FailureTimeout}).Summary(); got != "timeout" {
		t.Fatalf("got %q, want %q", got, "timeout")
	}
	f := &GenerationFailure{Message: "boom"}
	if got := f.Summary(); got != "boom" {
		t.Fatalf("got %q, want %q", got, "boom")
	}
}
```

- [x] **Step 2: Run to verify it fails**

Run: `go test ./pkg/harness/ -run 'TestSummarize|TestGenerationFailureSummary' -v`
Expected: FAIL, undefined.

- [x] **Step 3: Implement**

Add to `pkg/harness/failure.go`:

```go
// SummarizeAttempts picks the most informative detail from an attempt chain,
// falling back to a caller-provided sentence when every attempt is silent. It is
// what lifts a provider's own words into the top-level failure message.
func SummarizeAttempts(attempts []Attempt, fallback string) string {
	for i := len(attempts) - 1; i >= 0; i-- {
		if detail := strings.TrimSpace(attempts[i].Detail); detail != "" {
			return detail
		}
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "generation failed"
}

// Summary returns the most informative description of the failure: its message
// when set, otherwise the last non-empty attempt detail, otherwise the code.
func (f *GenerationFailure) Summary() string {
	if f == nil {
		return ""
	}
	if strings.TrimSpace(f.Message) != "" {
		return f.Message
	}
	return SummarizeAttempts(f.Attempts, string(f.Code))
}
```

- [x] **Step 4: Use it at the generic top-level messages**

`pkg/harness/router.go`, both places (`GenerateForRole` ~132, `StreamForRole` ~174):

```go
	return nil, &GenerationFailure{
		Code:      attempts[len(attempts)-1].Code,
		Message:   SummarizeAttempts(attempts, fmt.Sprintf("role %q produced no usable response", role)),
		Attempts:  attempts,
		ElapsedMS: time.Since(started).Milliseconds(),
	}
```

```go
	return &GenerationFailure{
		Code:      attempts[len(attempts)-1].Code,
		Message:   SummarizeAttempts(attempts, fmt.Sprintf("role %q streamed no text", role)),
		Attempts:  attempts,
		ElapsedMS: time.Since(started).Milliseconds(),
	}
```

`pkg/gui/text_generate.go`, the hard-failure block (~line 185):

```go
		failure := &harness.GenerationFailure{
			Code:        pickFailureCode(attempts),
			Message:     harness.SummarizeAttempts(attempts, "the model did not return any usable text"),
			Attempts:    attempts,
			PromptChars: len([]rune(request.PromptText())),
			ElapsedMS:   time.Since(started).Milliseconds(),
		}
```

`pkg/gui/character_generate.go`, the hard-failure block (~line 124): identical with the fallback `"the model did not return any usable character values"`.

- [x] **Step 5: Run tests and vet**

Run: `go test -count=1 ./pkg/harness/... ./pkg/gui/... && go vet ./pkg/harness/... ./pkg/gui/...`
Expected: PASS. Existing router tests that assert a generic message must be updated to assert the detail (for example `router_failure_test.go`).

---

### Task 4: Media TTS/STT failures and provider-error telemetry

**Files:**
- Modify: `pkg/media/metrics.go`, `pkg/media/providers.go`, `pkg/media/tts.go`, `pkg/media/stt.go`
- Create: `pkg/media/providers_test.go`

**Interfaces:**
- Produces: `media.providerErrors` counter (`localrpg.provider.errors`); `fallbackImageClient` returns `*harness.GenerationFailure` on double failure; `NewSceneImageClientWithSharedKey` gains a variadic `...trace.Logger`.

- [x] **Step 1: metrics instrument**

In `pkg/media/metrics.go`, add to `mediaInstruments`:

```go
	providerErrors otelmetric.Int64Counter
```

and in `mediaMetrics()`:

```go
		providerErrors: telemetry.Int64Counter(meter, "localrpg.provider.errors", "1", "Provider rounds that failed."),
```

- [x] **Step 2: TTS failure**

In `pkg/media/tts.go` (add `harness` import and the `attribute`/`otelmetric` imports are already present), replace:

```go
	audioBytes, err := p.client.Synthesize(ctx, text, voice)
	if err != nil {
		code := harness.ClassifyProviderError(err)
		p.logger.Event("media.tts.error", map[string]interface{}{
			"speaker":  speakerID,
			"provider": provider,
			"code":     string(code),
			"error":    err.Error(),
		})
		mediaMetrics().providerErrors.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.role", "tts"),
			attribute.String("error.kind", string(code)),
			attribute.String("gen_ai.system", provider),
		))
		return "", &harness.GenerationFailure{
			Code:    code,
			Message: fmt.Sprintf("synthesize utterance: %v", err),
			Cause:   err,
		}
	}
```

- [x] **Step 3: STT failure**

Replace `pkg/media/stt.go` (add `attribute`, `otelmetric`, `harness` imports):

```go
func (s *STTProvider) TranscribeAudio(ctx context.Context, audioData []byte) (string, error) {
	if len(audioData) == 0 {
		return "", &harness.GenerationFailure{Code: harness.FailureInvalidRequest, Message: "empty audio data"}
	}
	text, err := s.client.Transcribe(ctx, audioData)
	if err != nil {
		code := harness.ClassifyProviderError(err)
		mediaMetrics().providerErrors.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.role", "stt"),
			attribute.String("error.kind", string(code)),
		))
		return "", &harness.GenerationFailure{Code: code, Message: err.Error(), Cause: err}
	}
	return text, nil
}
```

- [x] **Step 4: image fallback**

In `pkg/media/providers.go` (add `harness` and `trace` imports), replace the fallback client and constructor:

```go
type fallbackImageClient struct {
	primary  ImageClient
	fallback ImageClient
	logger   trace.Logger
}

// GenerateImage covers a failing primary with the built-in generator. A fallback
// success is logged as a degraded path; a double failure reports both errors.
func (c *fallbackImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	data, err := c.primary.GenerateImage(ctx, prompt)
	if err == nil {
		return data, nil
	}
	if c.logger != nil {
		c.logger.Event("provider.error", map[string]interface{}{
			"role":     "image",
			"fallback": true,
			"error":    err.Error(),
		})
	}
	fbData, fbErr := c.fallback.GenerateImage(ctx, prompt)
	if fbErr == nil {
		return fbData, nil
	}
	return nil, &harness.GenerationFailure{
		Code:    harness.FailureProviderError,
		Message: fmt.Sprintf("image provider failed: %v; built-in fallback failed: %v", err, fbErr),
		Cause:   err,
		Attempts: []harness.Attempt{
			{Role: "image", Provider: "primary", Code: harness.ClassifyProviderError(err), Detail: err.Error()},
			{Role: "image", Provider: "builtin", Code: harness.ClassifyProviderError(fbErr), Detail: fbErr.Error()},
		},
	}
}
```

Constructor (variadic keeps existing callers compiling):

```go
func NewSceneImageClientWithSharedKey(cfg config.ImageConfig, sharedKey string, logger ...trace.Logger) (ImageClient, error) {
	primary, err := NewImageClientWithSharedKey(cfg, sharedKey)
	if err != nil {
		return nil, err
	}
	if !cfg.BuiltinFallback {
		return primary, nil
	}

	fallback, err := NewImageClient(config.ImageConfig{Type: "builtin", BuiltinName: "procedural-art"})
	if err != nil {
		return nil, err
	}
	var sink trace.Logger
	if len(logger) > 0 {
		sink = trace.OrNil(logger[0])
	}
	return &fallbackImageClient{primary: primary, fallback: fallback, logger: sink}, nil
}
```

Pass the service logger at `pkg/gui/service.go:1362`:

```go
	client, err := media.NewSceneImageClientWithSharedKey(cfg.Media.Image, cfg.Providers.Gemini.APIKey, s.logger)
```

- [x] **Step 5: Write the media tests**

Create `pkg/media/providers_test.go`:

```go
package media

import (
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

type stubImageClient struct {
	data []byte
	err  error
}

func (s stubImageClient) GenerateImage(context.Context, string) ([]byte, error) {
	return s.data, s.err
}

func TestFallbackReportsBothFailures(t *testing.T) {
	c := &fallbackImageClient{
		primary:  stubImageClient{err: errors.New("primary down")},
		fallback: stubImageClient{err: errors.New("builtin down")},
	}
	_, err := c.GenerateImage(context.Background(), "prompt")
	failure, ok := harness.FailureFrom(err)
	if !ok {
		t.Fatalf("err = %v, want a GenerationFailure", err)
	}
	if len(failure.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(failure.Attempts))
	}
}

func TestFallbackSucceedsAfterPrimaryFailure(t *testing.T) {
	c := &fallbackImageClient{
		primary:  stubImageClient{err: errors.New("primary down")},
		fallback: stubImageClient{data: []byte("art")},
	}
	data, err := c.GenerateImage(context.Background(), "prompt")
	if err != nil || string(data) != "art" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}
```

Run: `go test ./pkg/media/ -run TestFallback -v`
Expected: PASS.

- [x] **Step 6: Run media tests and vet**

Run: `go test -count=1 ./pkg/media/... && go vet ./pkg/media/...`
Expected: PASS.

---

### Task 5: GUI routes surface STT/audio failures

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/server.go`, `pkg/gui/types.go`
- Modify: `pkg/gui/server_test.go` (or a new `pkg/gui/stt_failure_test.go`)

**Interfaces:**
- Consumes: media TTS/STT failures.
- Produces: `TestProviderResponseDTO.Failure *harness.GenerationFailure`.

- [x] **Step 1: TranscribeAudio wraps failures**

Replace `TranscribeAudio` in `pkg/gui/service.go`:

```go
func (s *Service) TranscribeAudio(ctx context.Context, audioData []byte) (string, error) {
	cfg := s.configMgr.Get()
	if cfg.Media.STT.Type == "" || cfg.Media.STT.Type == "disabled" {
		return "", &harness.GenerationFailure{Code: harness.FailureProviderUnavailable, Message: "STT engine is disabled or unconfigured"}
	}

	client, err := media.NewSTTClient(cfg.Media.STT)
	if err != nil {
		return "", &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: fmt.Sprintf("initialize STT client: %v", err),
			Cause:   err,
		}
	}

	start := time.Now()
	s.logger = trace.OrNil(s.logger)
	s.logger.Event("media.stt.request", map[string]interface{}{
		"provider": cfg.Media.STT.Type,
		"bytes":    len(audioData),
	})

	text, err := media.NewSTTProvider(client).TranscribeAudio(ctx, audioData)
	if err != nil {
		s.logger.Event("provider.error", map[string]interface{}{"role": "stt", "error": err.Error()})
		return "", err
	}

	s.logger.Event("media.stt.result", map[string]interface{}{
		"chars":       len([]rune(text)),
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return text, nil
}
```

- [x] **Step 2: STT route uses the failure shape**

In `pkg/gui/server.go` `handleSTTRoute`:

```go
	text, err := s.service.TranscribeAudio(r.Context(), audioData)
	if err != nil {
		if writeGenerationFailure(w, err) {
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
```

- [x] **Step 3: Audio play routes use the failure shape**

In both `POST .../play` blocks, replace `writeGameError(w, err)` with:

```go
			if err := s.service.PlaySegmentAudio(r.Context(), gameID, turnNumber, segmentIndex, force); err != nil {
				if writeGenerationFailure(w, err) {
					return
				}
				writeGameError(w, err)
				return
			}
```

and likewise for `PlayTurnAudio`.

- [x] **Step 4: Test-provider DTO carries the failure**

In `pkg/gui/types.go`, add to `TestProviderResponseDTO`:

```go
	// Failure carries the structured reason a probe failed, so a client can show
	// the provider's own message rather than only Success=false.
	Failure *harness.GenerationFailure `json:"failure,omitempty"`
```

In `pkg/gui/service.go` `TestProvider`, for the TTS error branch and the STT error branch, set the failure:

```go
		if err != nil {
			return &TestProviderResponseDTO{
				Success:   false,
				LatencyMS: latency,
				Message:   err.Error(),
				Failure:   &harness.GenerationFailure{Code: harness.ClassifyProviderError(err), Message: err.Error(), Cause: err},
			}, nil
		}
```

- [x] **Step 5: Test**

Add `pkg/gui/stt_failure_test.go`:

```go
package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestTranscribeAudioUnconfiguredIsFailure(t *testing.T) {
	svc := &Service{configMgr: config.NewManagerForTest(config.Default())}
	_, err := svc.TranscribeAudio(context.Background(), []byte("audio"))
	if _, ok := failureCodeOf(err); !ok {
		t.Fatalf("err = %v, want a structured failure", err)
	}
}
```

Use the real test helper if one exists; otherwise assert with `harness.FailureFrom`. Adjust to the actual service construction pattern used by existing `pkg/gui` tests (check `service_test.go` for the constructor before writing). If `config.NewManagerForTest`/`config.Default` do not exist, build the service with the same helper the neighbouring tests use.

- [x] **Step 6: Run tests and vet**

Run: `go test -count=1 ./pkg/gui/... && go vet ./pkg/gui/...`
Expected: PASS.

---

### Task 6: Frontend shows the message, not just the code

**Files:**
- Create: `frontend/src/lib/generationError.ts`
- Modify: `frontend/src/api/client.ts`, `frontend/src/App.tsx`
- Modify: `frontend/src/components/ui/AIGenerateButton.tsx`
- Modify: `frontend/src/components/WorldsStudio.tsx`, `frontend/src/components/SystemsStudio.tsx`

**Interfaces:**
- Produces: `formatGenerationError(failure: GenerationFailure): string`, `generationAttemptLines(failure: GenerationFailure): string[]`.
- Consumes: `GenerationError` from `api/client.ts`.

- [x] **Step 1: Shared formatter**

Create `frontend/src/lib/generationError.ts`:

```ts
import type { GenerationFailure } from '../types';

// formatGenerationError leads with the provider's own message and keeps the
// bounded code as a suffix, so a user sees why a generation failed.
export function formatGenerationError(failure: GenerationFailure): string {
  const message = failure.message?.trim();
  if (message) return `${message} (${failure.code})`;
  return failure.code;
}

// generationAttemptLines renders the per-provider fallback chain for a details
// disclosure.
export function generationAttemptLines(failure: GenerationFailure): string[] {
  return (failure.attempts ?? []).map((attempt) => {
    const detail = attempt.detail?.trim() ? `: ${attempt.detail}` : '';
    return `${attempt.role}/${attempt.provider} [${attempt.code}]${detail}`;
  });
}
```

- [x] **Step 2: AIGenerateButton shows the message and a disclosure**

In `frontend/src/components/ui/AIGenerateButton.tsx`, import the helpers and add `showDetails` state; render the message (truncated), make the error span a toggle when attempts exist, and put `formatGenerationError` plus attempt lines in the `title`:

```tsx
import { formatGenerationError, generationAttemptLines } from '../../lib/generationError';
```

```tsx
  const [showDetails, setShowDetails] = useState(false);
```

```tsx
        title={error ? [formatGenerationError(error), ...generationAttemptLines(error)].join('\n') : (title || `AI Generate ${fieldName}`)}
```

```tsx
      {error && (
        <span className="inline-flex flex-col items-start gap-0.5 text-[10px] font-sans text-red-300" role="alert">
          <span className="inline-flex items-center gap-1">
            <AlertCircle className="w-3 h-3 text-red-400" />
            <button
              type="button"
              onClick={() => (error.attempts?.length ? setShowDetails((v) => !v) : undefined)}
              className={`max-w-[18rem] truncate text-left ${error.attempts?.length ? 'cursor-pointer underline decoration-dotted' : ''}`}
            >
              {formatGenerationError(error)}
            </button>
          </span>
          {showDetails &&
            generationAttemptLines(error).map((line, index) => (
              <span key={index} className="max-w-[18rem] truncate text-[10px] text-red-400/80">
                {line}
              </span>
            ))}
        </span>
      )}
```

- [x] **Step 3: App turn error uses the formatter and surfaces transport failures**

In `frontend/src/App.tsx`, import `formatGenerationError`, then:

```tsx
          else if (event.type === 'error') {
            const reason = event.failure
              ? formatGenerationError(event.failure)
              : (event.detail || event.message || 'The turn failed.');
            setTurnError(reason);
            console.error('turn failed:', event.message);
          }
```

and in the `catch` block around `streamTurn`, surface it:

```tsx
    } catch (err) {
      console.error('turn failed:', err);
      setTurnError(err instanceof Error ? err.message : String(err));
    } finally {
```

- [x] **Step 4: Studio helpers use the formatter**

In `frontend/src/components/WorldsStudio.tsx` and `frontend/src/components/SystemsStudio.tsx`, replace the local `reportGenerationError` body with:

```tsx
const reportGenerationError = (failure: GenerationFailure) =>
  setToast({ type: 'error', message: formatGenerationError(failure) });
```

(add the `formatGenerationError` import to each).

- [x] **Step 5: Audio client parses failures**

In `frontend/src/api/client.ts`, change `playTurnAudio` and `playSegmentAudio` to reuse the structured parser:

```ts
  static async playTurnAudio(gameID: string, turnNumber: number, force = false): Promise<void> {
    const url = `/api/game/${encodeURIComponent(gameID)}/turn/${turnNumber}/play${force ? '?force=1' : ''}`;
    const res = await fetch(url, { method: 'POST' });
    if (!res.ok) return throwGenerationError(res);
  }

  static async playSegmentAudio(gameID: string, turnNumber: number, segmentIndex: number, force = false): Promise<void> {
    const url = `/api/game/${encodeURIComponent(gameID)}/turn/${turnNumber}/segment/${segmentIndex}/play${force ? '?force=1' : ''}`;
    const res = await fetch(url, { method: 'POST' });
    if (!res.ok) return throwGenerationError(res);
  }
```

In `App.tsx` `handlePlayTurnAudio`'s `.catch`, prefer the structured message:

```tsx
      .catch((err: unknown) => {
        const message = err instanceof GenerationError ? err.failure.message : err instanceof Error ? err.message : String(err);
        setStatus({ state: 'error', message });
      });
```

Import `GenerationError` from `./api/client` in `App.tsx`.

- [x] **Step 6: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 7: Verification

- [x] **Step 1: Backend tests**

Run: `mise run test:backend`
Expected: PASS.

- [x] **Step 2: Backend vet**

Run: `mise run lint`
Expected: clean.

- [x] **Step 3: Frontend typecheck and build**

Run: `mise run test:frontend && mise run build:frontend`
Expected: exit 0.

- [x] **Step 4: Manual**

Trigger a provider failure (wrong API key) for text generation, an image, TTS, and STT; confirm the trace log and OTel event carry the provider body/stderr, the HTTP JSON `message`/`attempts` carry it, and the UI shows the provider message rather than only a code.

---

## Self-Review

**Spec coverage:** adapter detail (Task 2), `Summary()`/message lifting (Task 3), TTS/STT failures + `localrpg.provider.errors` with `error.kind` (Tasks 4-5), image fallback primary error (Task 4), frontend message + attempts (Task 6), logs/traces via existing `trace.LogEvent`/`logger.Event` and span `RecordError` (Tasks 4-5).

**Placeholder scan:** none.

**Type consistency:** `TruncateDetail`/`TruncateDetailString`/`MaxProviderDetailBytes` (Task 1) are used verbatim in Task 2; `SummarizeAttempts`/`Summary` (Task 3); `media.providerErrors` (Task 4); `TestProviderResponseDTO.Failure` (Task 5); `formatGenerationError`/`generationAttemptLines` (Task 6).
