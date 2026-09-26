# Design Spec: Provider Error Surfacing (Logs, Traces, and User Messages)

**Date:** 2026-09-26
**Status:** Proposed
**Target:** `pkg/provider/*`, `pkg/media`, `pkg/harness` (`failure.go`, `router.go`), `pkg/gui` (`generation_errors.go`, `generation_telemetry.go`, `service.go`), `frontend` (`api/client.ts`, `ui/AIGenerateButton.tsx`, turn error UI)

---

## 1. Executive Summary

The 2026-09-25 generation-failure-diagnostics work introduced a shared `harness.GenerationFailure` contract (`pkg/harness/failure.go`), HTTP propagation (`pkg/gui/generation_errors.go`), OTel propagation (`pkg/gui/generation_telemetry.go`), and a frontend `GenerationError`. That covers LLM text/character generation and images. It does **not** cover the whole provider surface, and several adapters deliberately throw away the provider's own words. The result: when a TTS clip, an image, or an LLM call fails, the user sees a generic message and the logs/traces lose the actual cause.

This spec is a **follow-up**: it does not re-design the taxonomy, HTTP mapping, or telemetry helper. It closes the remaining gaps so that as much of the underlying provider error as possible reaches the logs, the traces, and the user.

### Gaps addressed

1. **TTS/STT errors are plain `error`s, not `GenerationFailure`s.** `pkg/media/tts.go:285-288` wraps with `synthesize utterance: %w` and emits no error telemetry; `pkg/gui/service.go:2829-2833` logs `provider.error` and returns the raw error from `TranscribeAudio`; the test-provider endpoint returns a 200 with `Success:false` (`service.go:2737-2800`). No failure code, no `Attempts`, no `localrpg.provider.errors`.
2. **Provider detail is discarded in several adapters:**
   - `pkg/provider/openaichat/http.go:279-296`: non-200 response body is thrown away; the error is `http error <status> from <url>`.
   - `pkg/provider/geminillm/provider.go:579-594` and `pkg/provider/imagegemini/client.go:146-161`: 401/403/429 are replaced with friendly text, dropping the original message.
   - `pkg/media/providers.go:95-106` (`fallbackImageClient.GenerateImage`): the primary provider's error is swallowed entirely when the fallback succeeds.
   - `pkg/provider/ttspiper/client.go:29-31` and `pkg/provider/sttwhispercli/client.go:27-29`: CLI stderr is not captured (contrast `pkg/provider/clillm/cli.go:85-88`, which includes stderr).
   - `pkg/provider/imagehttp/client.go:341`: URL-fetch failures do not include the response body.
3. **Top-level `Message` is generic.** `router.go:130-135`, `text_generate.go:189`, and `character_generate.go:122` produce generic top-level messages; the specific detail lives only in `Attempts[].Detail`. The spec'd `GenerationFailure.Summary()` was never implemented.
4. **The frontend shows only the code.** `AIGenerateButton.tsx:105` renders `error.code`; the message is tooltip-only.
5. **No test asserts a full provider message survives to the frontend.**

---

## 2. Architecture & Data Flow

```
provider adapter (HTTP/CLI)
  - capture status + response body / stderr  (bounded)
        |
        v
media / harness wraps as harness.GenerationFailure{Code, Message, Attempts[Detail]}
  - Summary() lifts the most informative attempt detail into Message
        |
        +--------------------------+---------------------------+
        v                          v                           v
generation_telemetry          generation_errors.go        frontend GenerationError
  - trace event + span attr     - {"error": failure}        - message + attempts[]
  - localrpg.provider.errors      with attempts[].detail      - inline message, expandable
```

---

## 3. Detailed Component Designs

### 3.1 Preserve provider detail in adapters

Every adapter must retain the provider's own message, bounded but not replaced.

- **`openaichat/http.go`**: read up to `maxProviderDetailBytes` (new shared const, 8192) of the non-200 body and include it in the error: `http error %s from %s: %s`. Never log auth headers.
- **`geminillm/provider.go` / `imagegemini/client.go`**: keep the friendly first line for the user **and** append the original provider message, e.g. `invalid API key (provider: <original message>)`. Do not discard the original.
- **`media/providers.go` `fallbackImageClient`**: before falling back, record the primary error as an attempt detail (and via the telemetry hook if available) so a fallback success is still traceable as a degraded path. If the fallback also fails, return a `GenerationFailure` whose `Attempts` carry both provider errors.
- **`ttspiper/client.go` / `sttwhispercli/client.go`**: capture stderr with the same bounded helper and include it in the wrapped error, matching `clillm/cli.go`.
- **`imagehttp/client.go`**: include the response body in URL-fetch failures.
- Add a shared bounded-body helper in `pkg/provider` (e.g. `provider.TruncateDetail([]byte) string`) so caps are consistent and no adapter re-implements truncation.

### 3.2 `GenerationFailure.Summary()`

Implement in `pkg/harness/failure.go`:

```go
// Summary returns the most informative message available: the top-level Message
// when set, otherwise the first non-empty attempt detail, otherwise the code.
func (f *GenerationFailure) Summary() string
```

Use it for the top-level `Message` in:
- `harness.Router.GenerateForRole` / `StreamForRole` (`router.go:130-135`, `:172-177`)
- `Service.GenerateText` (`text_generate.go`)
- `Service.GenerateCharacter` (`character_generate.go`)

The top-level message becomes specific (e.g. the HTTP status plus body), while `Attempts[].Detail` continues to hold per-provider detail. Existing tests that assert a generic message must be updated to assert the summary.

### 3.3 TTS and STT as first-class failures

- **`pkg/media/tts.go`**: wrap provider errors as `harness.GenerationFailure{Code: ClassifyProviderError(err), Message: "synthesize utterance: " + err.Error(), Cause: err}` (or a media-local constructor that produces the same shape), and emit a `media.tts.error` trace event with the failure code. `pkg/media` already imports `pkg/harness`? Verify; if this would cycle, add the failure construction in the caller (`pkg/gui`) instead and have `pkg/media` return a typed error carrying the provider message.
- **`pkg/gui/service.go`**:
  - `TranscribeAudio` returns a `GenerationFailure` (currently raw error, `:2829-2833`).
  - Any TTS/STT HTTP handler that can fail generation uses `writeGenerationFailure` instead of a bare 500 or a 200 `Success:false`. The provider test endpoint may keep returning 200 with `Success:false` for compatibility, but must include the failure code and full message in its DTO.
- **Telemetry**: record `localrpg.provider.errors` with an `error.kind` attribute (`provider_error`, `timeout`, `empty_response`, `provider_unavailable`) for TTS/STT, matching the metric the 2026-09-25 spec defined for text/image but which has no TTS/STT emitter today.

### 3.4 Logs and traces

- The custom `pkg/trace.Logger` + `pkg/telemetry/bridge.go` already turn events into OTel log records and span events. Ensure the TTS/STT failure events added above flow through `trace.SanitizeFields` like the existing `generate.*` events.
- Keep free-form provider detail as an event attribute (`provider.detail`) on the error event, not scattered across span names.
- Detail is sanitized/capped by `trace.Sanitize` (`pkg/trace/sanitize.go`, default 20000) on the trace path; the HTTP JSON path is not sanitized today, which is acceptable because it is local-only, but the adapter-level `maxProviderDetailBytes` (8192) bounds payload size.

### 3.5 Frontend: show the message, not just the code

- **`AIGenerateButton.tsx`**: the inline badge currently renders `error.code` (`:105`). Render a short form of `error.message` (single line, truncated with `truncate max-w-[...]`), keep the code in the tooltip, and add a details disclosure listing `attempts[].provider`/`code`/`detail` when present.
- **Turn error UI**: confirm the turn stream error path renders `TurnEvent.Failure` (message + attempts). If it only shows `code`/`detail`, render the failure message and an expandable attempt list in the action console / turn error banner.
- **TTS playback errors** (turn audio bar, segment playback): surface `Speech error: <message>` with the full provider message, not a generic failure line, reusing the `GenerationError` message.
- Keep a single shared error-presentation helper (e.g. `frontend/src/lib/generationError.ts`) so the message/attempts formatting is consistent across `AIGenerateButton`, the Codex portrait regenerate button, the action console, and audio controls.

---

## 4. Error-Handling Table

| Situation | Code | HTTP | User sees | Trace/log |
|---|---|---|---|---|
| LLM non-200 | `provider_error` | 502 | status + body (truncated) | error event + attempt detail |
| LLM empty completion | `empty_response` | 502 | "model returned no text" | error event |
| Gemini 401/403/429 | `provider_error` | 401/403/429->502 | friendly line + original message | original in attempt detail |
| Image provider error | `provider_error` | 502 | provider message | `localrpg.media.image.duration` + error event |
| Image fallback used | `provider_error` on primary | 200 (fallback wins) | current image | primary error as attempt/event |
| Image disabled | `provider_unavailable` | 503 | provider id + "disabled/unconfigured" | error event |
| TTS/STT provider error | `provider_error` | 502 | provider message | `media.tts.error` / `provider.error` |
| Timeout | `timeout` | 504 | "timed out calling <provider>" | error event |

---

## 5. Non-Goals

- No change to success-path behavior or response shapes beyond adding failure detail.
- No new provider types, models, or configuration.
- No secrets in errors: never include API keys or `Authorization` headers; body capture must strip auth if a provider echoes it.
- No change to the OTel exporter pipeline itself.

---

## 6. Test Strategy

1. **Adapter unit tests** (`pkg/provider/*`): a stub HTTP server returning a non-200 body asserts the body text appears in the returned error; a stub CLI asserts stderr appears; Gemini mappers assert both the friendly line and original message survive.
2. **`pkg/harness` test**: `GenerationFailure.Summary()` picks the top-level message, else the first attempt detail, else the code; router returns a specific top-level message.
3. **`pkg/gui` handler tests**: a TTS generation failure returns a `GenerationFailure` JSON body with a non-empty message; a `localrpg.provider.errors` metric is recorded with `error.kind`.
4. **Full-survival test**: a test that drives a multi-provider fallback and asserts the final HTTP JSON (or frontend `GenerationError` fixture) contains the specific provider detail, not a generic string.
5. **Frontend build gate**: `npm --prefix frontend run build` passes.
