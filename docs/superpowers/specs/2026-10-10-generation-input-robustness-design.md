# Generation Input Robustness Design

**Date:** 2026-10-10
**Status:** Proposed
**Issue:** [#129](https://github.com/darkliquid/LocalRPG/issues/129)
**Epic:** [#123 Generation, playback, and roll resilience](https://github.com/darkliquid/LocalRPG/issues/123)
**Depends on:** [System Enhancement Design](2026-10-05-system-enhancement-design.md), [World Enhancement Design](2026-10-05-world-enhancement-design.md), [System Generation Design](2026-10-05-system-generation-design.md), [World Generation Pipeline Design](2026-10-05-world-generation-pipeline-design.md)
**Scope:** `pkg/jsonrepair`, `pkg/sysgen`, `pkg/worldgen`, `pkg/gui`, `frontend`

---

## 1. Problem

The generate and enhance flows (system and world) occasionally fail with an error the
user sees as some variant of **"invalid character"**, and the message is a raw Go JSON
error with no guidance. Nothing about it is actionable.

The content is model-authored. Every flow funnels through one seam:

- `sysgen.Generator.GenerateJSON(ctx, prompt, schema) ([]byte, error)`
  (`pkg/sysgen/sysgen.go:79-81`), producing the raw reply.
- `worldgen.Generator` is identical (`pkg/worldgen/worldgen.go:109`).
- `pkg/gui/worldgen.go:92-118`'s `routerGenerator.GenerateJSON` hands the model's reply
  back **raw**: "A reply is handed back raw: the pipeline repairs and parses it."

Parsing is `decodeJSON` in each package, which first runs the structural repairer:

```go
// pkg/sysgen/sysgen.go:83-89 (worldgen.go:219-230 wraps it with a byte count and tail)
func decodeJSON(raw []byte, v any) error {
	payload := raw
	if res := jsonrepair.Repair(raw); res.OK {
		payload = res.Payload
	}
	return json.Unmarshal(payload, v)
}
```

`pkg/jsonrepair` repairs four **structural** faults and nothing else: it strips a code
fence, trims prose, closes open brackets/braces, and removes trailing commas
(`repair.go:37-70`). Its own scan package documents the scope: "It never invents
content: it only removes, truncates, or closes structure." It does **not** touch
malformed bytes inside a string value. So the common failure classes pass through
unchanged and `json.Unmarshal` fails:

- a literal newline, tab, or control character inside a string value
  (`invalid character '\n' in string literal`), which a model produces whenever it
  writes multi-line prose into a `description` or `rules_prompt`;
- a reply cut off mid-string, which `closeStructures` refuses to finish because it bails
  when the scan ends inside a string (`repair.go:171-172`);
- a smart quote used as a structural delimiter (`invalid character '“' after object
  key`);
- a leading UTF-8 BOM.

The error then propagates verbatim and is shown to the user:

- Enhance: `fmt.Errorf("enhance decode: %w", err)` (`sysgen/enhance.go:90`) returned by
  `EnhanceSystem` (`pkg/gui/system_enhance.go:41-47`) and written as a `400`
  (`pkg/gui/sysgen_routes.go:148-152`); the dialog shows `err.message`
  (`SystemEnhanceDialog.tsx:83-84`). World enhance is the same
  (`pkg/gui/worldgen_entities.go:306-312`).
- Generate: an `error` NDJSON event carrying `err.Error()`
  (`pkg/gui/sysgen_routes.go:32-35`, `pkg/gui/worldgen_routes.go:33-36`), shown by
  `SystemGenerateDialog.tsx:113-116` and `WorldGenerateDialog.tsx:155-157`.

There is **no retry**. A model that returns one malformed reply fails the whole run.

A smaller, second source of "invalid characters" is reachable: `ApplyWorldEnhancements`
validates a client-supplied entity id and returns `fmt.Errorf("invalid entity id %q: %w",
…)` (`pkg/gui/worldgen_entities.go:247-249`) wrapping `pathutil.ErrInvalidID`
("identifier contains invalid characters", `pkg/pathutil/pathutil.go:16-36`). That fires
only when a caller sends a non-slug id, never from generated content, but it is the same
sentence and the same confusion.

## 2. Goals

- The repairer fixes malformed bytes inside strings, not just structure.
- One malformed reply gets one bounded retry before the run fails.
- The user sees a plain-language reason and a next step, never `invalid character`.
- Client-supplied enhancement ids cannot produce the plural "invalid characters" error.

## 3. Non-goals

- Making the model deterministic, or catching every possible malformation. The repairer
  fixes the common classes; an unfixable reply still fails, with a better message.
- Changing the prompt's requested schema or the schemas themselves.
- Changing the turn orchestrator's own `@`-record repair loop (`pkg/engine/repair.go`),
  which is a separate path.

## 4. Design

### 4.1 Extend `pkg/jsonrepair`

Add a string-repair pass, tried after the structural passes and only adopted when the
result validates, exactly like the existing four. It does two things:

1. **Escape illegal bytes inside string literals.** Walk the payload tracking
   in-string/escaped state (the same scan the existing passes use) and replace any raw
   byte below `0x20` with its JSON escape: `\n`, `\r`, `\t`, and `\u00XX` for the rest.
   Bytes outside strings are untouched. This is content-preserving: a real newline in a
   string becomes a legal `\n`.
2. **Terminate an unterminated string.** When the scan ends still inside a string, append
   a closing `"`. This composes with `closeStructures`, which is re-run after, so a reply
   cut off mid-string becomes parseable instead of refused.

Also add a leading-BOM strip, and a conservative smart-quote normalisation: a `“` or `”`
is replaced with `"` **only when it sits in a structural position** (immediately after
`{`, `,`, or `[`, or immediately before `:`, `}`, or `]`), leaving a smart quote inside a
string value alone. The normalisation is best-effort and documented as such.

New kinds:

```go
const (
    KindEscapeControl Kind = "escape_control"
    KindCloseString   Kind = "close_string"
    KindBOM           Kind = "bom"
    KindSmartQuote    Kind = "smart_quote"
)
```

`Repair` gains the passes in this order: BOM strip, then the existing fence/trim/close/
trailing-comma chain, then the string pass (escape, close string), re-running
`closeStructures` after the string pass. Each candidate is adopted only if `json.Valid`.
The pass stays inside the 64 KB `maxPayload` bound.

### 4.2 One bounded retry per model call

Add a small helper shared by both pipelines, so the retry policy lives in one place:

```go
// pkg/sysgen (and pkg/worldgen, same shape)
// generateJSON calls the generator, and on an unparseable reply calls it once more
// with the parse failure appended to the prompt, then decodes the second reply.
func generateJSON(ctx context.Context, gen Generator, prompt, schema string, v any) error
```

The steps (`pkg/sysgen/steps.go:44,95,215,256`, `pkg/sysgen/enhance.go:82`,
`pkg/sysgen/derive.go:44`, `pkg/worldgen/steps.go:63,101,130,157`,
`pkg/worldgen/entities.go:74`, `pkg/worldgen/enhance.go:73`) call `generateJSON` instead
of pairing `gen.GenerateJSON` with `decodeJSON` directly. The retry prompt appends:

```text
Your previous reply could not be parsed as JSON (reason: <err>). Reply with one JSON
object only, with every newline inside a string escaped as \n.
```

One retry, no loop. A second failure returns the parse error. The deterministic
`OracleGenerator` never triggers a retry because it emits valid JSON; the retry only
costs a metered call when a real provider misbehaves.

### 4.3 A plain-language error

Both `decodeJSON` functions wrap the failure in a sentinel:

```go
// ErrMalformedReply reports a model reply that could not be parsed after repair.
var ErrMalformedReply = errors.New("the model returned a reply that could not be read")
```

`sysgen.decodeJSON` gains the byte count and reply tail that `worldgen.decodeJSON`
already includes (`worldgen.go:224-227`, `replyTail`), so the trace keeps the diagnostic
detail. The GUI maps `errors.Is(err, ErrMalformedReply)` to a `502` (or an NDJSON
`error` event with `code: "malformed_reply"`) whose message is: *"The model's reply could
not be read. This is usually a one-off; try again, or switch to a different model."* The
raw Go error stays in the trace/`detail` field for debugging, not in the headline.

### 4.4 Client-supplied enhancement ids

`ApplyWorldEnhancements` validates each entity id with `pathutil.ValidateID`. The
frontend builds those ids from entity names in the enhancement draft
(`worldgen_entities.go`'s request). Before submitting, the client slugifies each id with
the same rule the backend enforces (`entity.Slugify`, `pkg/entity/entity.go:303-319`) or
disables submit for an invalid id, so the plural "invalid characters" error is
unreachable from the UI. The backend check stays as defence in depth.

## 5. Behaviour

| Input | Before | After |
| --- | --- | --- |
| `{"description":"line one\nline two"}` with a literal newline | decode fails | escaped to `\n`, parses |
| a reply cut off mid-string | `closeStructures` refuses | string terminated, structures closed |
| a leading UTF-8 BOM | decode fails | stripped |
| `“key”: …` with smart quotes as delimiters | decode fails | normalised to `"` |
| one malformed reply from a provider | the run fails | one retry, then a plain message |
| an enhancement draft with a non-slug id | "identifier contains invalid characters" | client slugifies; unreachable |

## 6. Testing

- `pkg/jsonrepair`: table tests for a literal newline/tab/control byte inside a string,
  an unterminated string, a BOM, smart-quote delimiters, and a smart quote inside a
  string value that must be **left alone**; each asserts the repaired payload is valid
  and content-preserving (the escaped string unquotes to the original text).
- `pkg/sysgen` / `pkg/worldgen`: `generateJSON` retries once and succeeds when a fake
  generator returns malformed then valid JSON, and does not retry when the first reply is
  valid; a second bad reply returns `ErrMalformedReply`.
- `pkg/gui`: the malformed-reply sentinel maps to the friendly message and a `502`
  (extend `system_enhance_test.go` / `worldgen_test.go`).
- `frontend`: the enhancement draft slugifies a non-slug id before submit
  (a small vitest around the enhancement dialog).

## 7. Rollout

Bottom-up and additive: the repairer gains passes (existing callers unchanged), the
pipelines adopt `generateJSON`, the handlers gain a friendly mapping. No schema or API
shape change; the error `code` gains one value.

## 8. Risks

- **A repair that changes meaning.** The string pass only escapes bytes that are illegal
  in a JSON string and terminates a truncated one; it never removes or reorders content.
  The tests unquote the repaired payload and compare it to the intended text, so a silent
  content change fails the suite.
- **A retry that doubles cost.** Exactly one retry, only after a parse failure, and never
  for the deterministic oracle. A metered provider that consistently emits bad JSON costs
  two calls for that step, then fails.
- **Smart-quote normalisation is heuristic.** Restricting it to structural positions
  means a reply whose smart quotes are genuinely inside strings is left untouched. It is
  best-effort and never required for correctness.
- **Sentinel wrapping can hide the detail.** The raw error remains in the trace and the
  NDJSON `detail`, so a developer can still see `invalid character …`; only the headline
  changes.
