# Generation and World Studio Follow-ups Design

**Date:** 2026-09-25
**Status:** Proposed
**Scope:** OpenTelemetry attribute completeness and image observability, HTTP error-shape consistency, frontend error surfacing at every call site, world draft selection robustness, and the code/test debt left by the first implementation pass
**Related:** Generation Failure Diagnostics Design (2026-09-25), World Creation Draft Entry Design (2026-09-25), their implementation plans, and the two read-only reviews of the shipped code

## 1. Overview & Goals

The first implementation of the generation-failure diagnostics and world
creation draft-entry specs shipped and passed review, but each review left a
list of deferred findings. They are real but non-blocking: attribute gaps in
OpenTelemetry, error-shape inconsistencies, an error surface that was not wired
at every call site, a selection race in the studio, and tests that would have
caught the earlier bugs. None of them change the user-visible contract that
already shipped; this spec closes the gaps so the two subsystems are consistent
and fully observable.

**Goals:**

- Complete the OpenTelemetry surface promised by the diagnostics spec: every
  span and metric carries its documented attributes, and image generation is
  observable on the same footing as text.
- Make every generation failure use one JSON error shape, including malformed
  requests, so `invalid_request` is reachable and clients never receive a
  plain-text error from a generation route.
- Surface generation errors at every `AIGenerateButton` call site and highlight
  the slug field on a world creation conflict.
- Remove the world-draft selection race and the review's code-quality debt in
  the studio.
- Add the tests the first pass skipped, so a regression in any of these areas
  fails a test rather than reaching a user.

**Non-Goals:**

- Changing any user-visible success path or the bounded failure-code enum.
- Adding provider retries or changing fallback ordering.
- Adding a world delete endpoint or persisting drafts.
- Reworking the telemetry provider, config, or the `pkg/trace` bridge.
- Frontend component tests (there is still no frontend test runner); frontend
  work is verified by `tsc --noEmit` and the manual checklist in section 9.

**Success Criteria:**

- `generate.text` carries `localrpg.generation.attempts`, `localrpg.generated_by`,
  `localrpg.field_count`, and `gen_ai.system`; `localrpg.generation.errors` and
  `localrpg.generation.duration` carry `localrpg.role`.
- `generate.image` carries `localrpg.image.kind`, `localrpg.image.provider`, and
  `localrpg.image.bytes`, records `localrpg.media.image.duration`, and emits
  `image.request`/`image.complete`/`image.error` trace events.
- `generation.error` on the turn path logs `code`, `role`, `provider`,
  `prompt_chars`, `context_chars`, `elapsed_ms`, `chunk_count`, and
  `partial_chars`, using the same field names on every path.
- A malformed generation request body returns `400` with
  `{"error":{"code":"invalid_request",...}}`.
- Every `AIGenerateButton` passes `onError`, and a `409` world conflict shows a
  red slug field.
- Rapid clicking between saved worlds settles on the last click.
- The tests in section 8 pass.

## 2. Deferred Items Inventory

Each item names its origin so the plan can trace it back.

| # | Area | Item | Origin |
|---|------|------|--------|
| D1 | OTel | `generate.text` missing `attempts`/`generated_by`/`field_count`/`gen_ai.system` | Review 1 §1.5 |
| D2 | OTel | `localrpg.generation.errors`/`duration` missing `localrpg.role` | Review 1 §1.6 |
| D3 | OTel | Image span uses `form_type`/`field_name`, lacks `localrpg.image.*`, `attempt`/`error` events, `localrpg.media.image.duration`, and `image.*` trace events | Review 1 §1.7 |
| D4 | OTel | Turn `generation.error` renames `code` to `generation_code` and omits `role`, `context_chars`, `chunk_count`, `partial_chars`; empty-narration branch omits `provider`/`prompt_chars`/`elapsed_ms`/`attempts` | Review 1 §1.8 |
| D5 | OTel | `StreamForRole` does not track attempts or return a `*GenerationFailure` on empty close | Review 1 §1.3 |
| D6 | HTTP | `invalid_request` is never produced; malformed bodies still return plain text | Review 1 §1.9 |
| D7 | HTTP | `writeGenerationFailure` does not emit its own trace/logger event, so the game/world asset paths lose the `generate.error` event | Review 1 §1.4 |
| D8 | HTTP | `writeGenerationError` bypasses the shared `writeJSON` helper | Review 1 §3 |
| D9 | Frontend | `AIGenerateButton.onError` is never passed at any call site | Review 1 §1.11 |
| D10 | Frontend | World create `409` does not highlight the slug field | Review 2 §1 |
| D11 | Studio | Rapid saved-world clicks can settle on a stale selection | Review 2 §2 |
| D12 | Studio | `useEffect` mount closure trips `react-hooks/exhaustive-deps`; `catch (err: any)` bypasses strict typing | Review 2 §3 |
| D13 | Docs | `WorldDraft` interface in the design doc lists `name`, but the implementation derives the label from the form | Review 2 §1 |
| D14 | Quality | Duplicated role/attempt loop in `GenerateText` and `GenerateCharacter` | Review 1 §3 |
| D15 | Quality | Duplicated image path in `GenerateGameAsset`/`GenerateWorldAsset` | Review 1 §3 |
| D16 | Quality | `startGenerationSpan` has an unused receiver | Review 1 §3 |
| D17 | Tests | Failure-path, fallback, turn-span, image, and trace-event tests listed in section 8 | Both reviews |

## 3. OpenTelemetry Completeness

### 3.1 Text spans and metrics (D1, D2)

`pkg/gui/generation_telemetry.go`:

- `startGenerationSpan` keeps its current attributes; the callers add the
  result-dependent ones after the request completes, because only they know the
  winning role and field count.
- `recordGeneration` gains a `role string` parameter. It records
  `localrpg.role` on both `localrpg.generation.errors` and
  `localrpg.generation.duration`. On failure the role is the last attempt's
  role; on success it is `resp.GeneratedBy`.
- `GenerateText` and `GenerateCharacter` set the span attributes themselves,
  immediately before `span.End()`:
  - on failure: `localrpg.generation.attempts` = `len(attempts)`, and
    `gen_ai.system` = the last attempt's provider.
  - on success: `localrpg.generated_by` = `resp.GeneratedBy`,
    `localrpg.field_count` = `len(resp.Fields)`/`len(resp.Values)`,
    `localrpg.generation.attempts` = `len(attempts)`, and `gen_ai.system` = the
    winning attempt's provider.

The winning/last provider is derived from `attempts` (which already carries
`Provider`), so no new plumbing is needed. When `attempts` is empty the
attributes are omitted rather than set to `""`.

### 3.2 Image observability (D3)

`pkg/gui/service.go` and `pkg/gui/generation_telemetry.go`:

- `startGenerationSpan(ctx, "generate.image", "image", req.Kind)` is replaced by
  a dedicated `startImageSpan(ctx, kind string) (context.Context, oteltrace.Span)`
  that sets `localrpg.image.kind` instead of `localrpg.form_type`/`localrpg.field_name`.
- On completion set `localrpg.image.provider` (from the image config `type` or
  `builtin_name`; whichever the factory resolves) and `localrpg.image.bytes`.
- Add a `localrpg.media.image.duration` histogram instrument (in the existing
  cached `generationInstruments` struct, or a sibling `mediaInstruments` if that
  reads cleaner) recorded with `localrpg.image.provider`.
- Emit `generate.request`, `generate.complete`, and `generate.error` through the
  same `trace.LogEvent` seam used by text generation, with `form_type` set to
  `image` and `field_name` set to the asset kind. There is one unified event
  family; images do not get their own `image.*` names. On failure also emit an
  `error` span event with the bounded code.
- Failures continue to increment `localrpg.generation.errors` with
  `localrpg.form_type=image`; success increments `localrpg.generation.duration`
  with the same form type.

If the image provider identity cannot be resolved without extra plumbing, the
provider attribute is omitted and the omission is recorded in the plan; the
byte count and span name are the load-bearing parts.

### 3.3 Turn failure payload (D4)

`pkg/engine/orchestrator.go`:

- Use the field name `code`, not `generation_code`, on every `generation.error`
  event.
- Add `role` (`"gm"`), `context_chars` (`len([]rune(assembly.Prompt))` minus the
  prompt chars, or the assembled context size already available), `chunk_count`
  (a counter added to `streamResult`), and `partial_chars`
  (`len([]rune(result.Text))`).
- The empty-narration branch logs the same base fields (`code`, `provider`,
  `prompt_chars`, `elapsed_ms`, `attempts`) as the hard-failure branch plus
  `finish_reason`.
- `streamResult` gains `ChunkCount int`, incremented per text or tool chunk in
  `stream`, so `chunk_count` is real rather than inferred.

`pkg/gui/generation_telemetry.go` already uses `code`; aligning the engine to it
removes the inconsistency noted in D4 and review 1 §3.

### 3.4 `StreamForRole` parity (D5)

`pkg/harness/router.go`:

- `StreamForRole` records an `Attempt` per provider and, when the primary
  closes with no first chunk and no text, returns a `*GenerationFailure` with
  code `empty_response` (or the first chunk's classified error) instead of the
  bare `<-errCh` / `close(out)`. The fallback path records the attempt it
  skipped.
- This API has no callers today; the change keeps it consistent with
  `GenerateForRole` so a future streaming consumer cannot reintroduce the bug
  the diagnostics spec fixed. A test covers both the fallback and the
  empty-everywhere case.

## 4. HTTP Error-Shape Consistency

### 4.1 `invalid_request` and one JSON shape (D6, D8)

Add to `pkg/gui/generation_errors.go`:

```go
// writeInvalidRequest writes a 400 in the same shape as every other generation
// failure, so clients parse one error type.
func writeInvalidRequest(w http.ResponseWriter, message string) {
    writeGenerationError(w, &harness.GenerationFailure{
        Code:    harness.FailureInvalidRequest,
        Message: message,
    })
}
```

- `handleGenerateTextRoute`, `handleCharacterGenerateRoute`,
  `handleGenerateAssetPreview`, the game/world `generate-asset` handlers, and
  `handleTurnSubmit` call `writeInvalidRequest` for malformed bodies (and for a
  failed turn's pre-stream validation) instead of `http.Error`.
- `writeGenerationError` uses the existing `writeJSON` helper for the body, so
  the encoding/header handling has one implementation.
- Non-generation routes are unaffected; `turn` body-too-large already returns
  400 and only gains the JSON shape.

The world create duplicate `409` response also switches to the JSON error shape
(`{"error":{"code":"invalid_request"?}}` is wrong for a conflict, so a small
`writeConflict`/`writeHTTPError` helper that wraps the same shape but keeps the
status is added; the frontend already parses text and a JSON body is a
strict improvement).

Ruling on the conflict code: `ErrWorldExists` is not a generation failure, so it
gets its own `writeJSONError(w, status, message)` helper rather than being
forced through the generation enum.

### 4.2 `writeGenerationFailure` emits the event (D7)

`writeGenerationFailure` logs a `generate.error` event through the service
logger when it handles a failure, so the game/world asset handlers (which do not
go through `recordGeneration`) still produce a trace event and a nil-safe logger
line. To avoid double logging on the text/character paths, `recordGeneration`
stops emitting `generate.error` on failure and relies on the handler, or the
handler emits a distinct `generate.response` event. Decision: `recordGeneration`
keeps the span/metric/event responsibility (it has `ctx` and the span) and the
handler emits nothing; the game/world asset handlers call `recordGeneration`
themselves (they already create a span) so they are covered. The game/world
asset handlers currently call `recordGeneration` but return the failure to the
handler, which writes the body; verify that combination and remove any
duplicate event.

## 5. Frontend Error Surfacing

### 5.1 `onError` at every call site (D9)

Pass `onError={(failure) => setToast({ type: 'error', message: `${failure.code}: ${failure.message}` })}`
to every `AIGenerateButton`:

- `frontend/src/components/WorldsStudio.tsx` (3 buttons: name, genre, art_style,
  description, lore_prompt).
- `frontend/src/components/SystemsStudio.tsx` (name, description, rules_prompt).
- `frontend/src/components/launcher/NewCampaignModal.tsx` (player and campaign
  fields; it already has `genError`, so wire `setGenError`).
- `frontend/src/components/launcher/CampaignSettingsModal.tsx` (name,
  description).

The inline badge stays as the immediate per-button signal; the toast is the
form-level record. A shared helper on each studio,
`const reportGenerationError = (failure: GenerationFailure) => setToast(...)`,
avoids repeating the message format.

### 5.2 Slug field on conflict (D10)

`WorldsStudio.tsx`:

- Add `const [slugError, setSlugError] = useState(false)`.
- On `WorldExistsError`, set `slugError = true` as well as the toast.
- Clear it when the slug or name input changes.
- Render a red border/ring on the slug input when `slugError` and show a short
  message under it.

### 5.3 Turn banner

Already shipped (`App.tsx`): the error event renders `code: message`. No change.

## 6. World Studio Robustness

### 6.1 Stale selection race (D11)

`WorldsStudio.tsx`:

- Add `const detailRequest = React.useRef(0)`.
- `loadWorldDetail` captures `const token = ++detailRequest.current` and, after
  the awaited fetch, returns early if `token !== detailRequest.current`. This
  makes the last click win regardless of response order.
- `handleNewWorld` also bumps the token so an in-flight saved-world fetch cannot
  later overwrite a fresh draft.

### 6.2 Closure and typing debt (D12)

- Mount effect: capture `startMode` in a ref
  (`const startModeRef = React.useRef(startMode)`) and call
  `loadWorlds(undefined, startModeRef.current)` with `[]` deps, or add an
  explicit `// eslint-disable-next-line react-hooks/exhaustive-deps` with a
  comment; prefer the ref so no lint suppression is needed.
- Replace `catch (err: any)` with `catch (err: unknown)` and a shared
  `errorMessage(err: unknown): string` helper (`err instanceof HTTPError ? err.message : 'Unexpected error'`).

### 6.3 `WorldDraft` doc drift (D13)

Amend the World Creation Draft Entry Design's `WorldDraft` interface to
`{ localId: string; dirty: boolean }` (matching the implementation), since the
sidebar label is derived from the live form `name` and keeping a second copy in
the draft risks drift.

## 7. Code Quality

- **D14:** Extract the shared attempt-collection loop into
  `pkg/gui/generation_attempts.go`:
  ```go
  // collectTextAttempts asks each role in order and returns the first decode
  // result, the attempts recorded, and the generated_by role.
  func collectTextAttempts(ctx context.Context, router *harness.Router, roles []string, request harness.GenerateRequest) (map[string]string, []harness.Attempt, string)
  ```
  `GenerateText` and `GenerateCharacter` call it and keep only their
  prompt/decoding differences. The helper owns the `len(failure.Attempts)==0`
  synthesis and the per-attempt timing.
- **D15:** Extract the shared image path into
  `pkg/gui/image_generation.go`:
  ```go
  func (s *Service) generateImage(ctx context.Context, kind, prompt string) ([]byte, *harness.GenerationFailure)
  ```
  covering span, client construction, `GenerateImage`, `guardImageBytes`, byte/
  provider metric, and the `image.*` events. `GenerateAssetPreview`,
  `GenerateGameAsset`, and `GenerateWorldAsset` become thin wrappers that build
  the prompt and then persist or return the bytes.
- **D16:** `startGenerationSpan` is a method only for `s.logger`; either keep it
  and document why, or make it a free function taking the logger. Prefer a free
  `startGenerationSpan(ctx, logger, name, formType, fieldName)` and a small
  `startImageSpan(ctx, logger, kind)`.

## 8. Testing Strategy

Backend (standard library only, `t.TempDir()`):

- **Router:** `StreamForRole` falls back when the primary produces no chunk and
  returns `empty_response` when every provider is empty; `GenerateForRole`
  fallback code is the *previous* attempt's code (locks in the D4/Review-1-2.3
  fix).
- **One-shot failures:** a table test over `GenerateText` and `GenerateCharacter`
  using a stub router: provider error -> `provider_error`, empty ->
  `empty_response`, unparseable -> `parse_error`, `_all` with one field missing
  -> `200` + `warning`, full success via a fallback -> no warning. This needs a
  test seam for the router (accept an injected `*harness.Router` in a
  `generateTextWithRouter`/`generateCharacterWithRouter` internal helper, with
  the public methods building the router and delegating).
- **Handlers:** each code maps to its status and a JSON body; a malformed body
  returns `400 invalid_request`; a duplicate world create returns the JSON
  conflict shape.
- **Asset handlers:** a zero-byte image client yields a structured error and
  writes no file. Requires injecting the image client behind the existing
  service seam (or testing `generateImage` directly with a stub client).
- **Turn:** a failed generation marks the root `turn` span `Error`, sets
  `turn.outcome=error`, records `localrpg.turn.completed{turn.outcome=error}`,
  and the streamed `error` event carries `Failure`. Use the existing
  orchestrator fixture plus `telemetry.NewInMemory()`.
- **Trace events:** a one-shot failure emits `generate.request`,
  `generate.attempt`, and `generate.error`; an image failure emits
  `generate.request` and `generate.error` with `form_type=image`.
- **OTel attributes:** assert `generate.text` has `attempts`/`field_count`/
  `gen_ai.system` on success, and `generate.image` has `localrpg.image.bytes`.

Frontend: `mise run test:frontend` plus the manual checklist in section 9.

## 9. Manual Verification

1. Configure the `gm`/`character` roles as a CLI provider that prints nothing;
   confirm the inline error, the form toast, and (after D9) the toast firing at
   every call site.
2. Create a world with a name matching an existing world; confirm a red slug
   field and the conflict toast.
3. Click rapidly between two saved worlds; confirm the editor settles on the
   last clicked one after both requests resolve.
4. With telemetry enabled, confirm `generate.text` carries the new attributes
   and `localrpg.generation.errors` carries `localrpg.role`.
5. Run a turn against a stalling provider; confirm the turn banner names the
   timeout and `generation.error` logs `code`, `chunk_count`, and
   `partial_chars`.

## 10. Compatibility & Migration

- Additive to the shipped contract: no route, status, or success-path change.
  Malformed generation bodies change from `text/plain 400` to
  `application/json 400`; clients already parse the structured shape.
- The world duplicate `409` body changes from plain text to JSON; the frontend
  already tolerates both.
- The amended `WorldDraft` design doc drops the unused `name` field.
- `StreamForRole` behaviour changes only for empty streams; it has no callers.

## 11. Resolved Decisions

- **Event family:** text and image generation share one `generate.*` event
  family (`generate.request`, `generate.attempt`, `generate.complete`,
  `generate.error`). Images are identified by `form_type=image`; there is no
  separate `image.*` family.
- **Span name:** the image span stays `generate.image`. The `localrpg.` prefix
  belongs on attributes, not span names, and `generate.image` matches the
  existing `generate.text` and dotted `noun.verb` convention in the
  OpenTelemetry spec. Image-specific attributes keep their `localrpg.image.*`
  prefix.
