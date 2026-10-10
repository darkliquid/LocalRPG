# Generation Input Robustness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Stop malformed model replies surfacing as a raw "invalid character" error: repair illegal bytes inside strings, retry once, and present a plain reason.

**Architecture:** `pkg/jsonrepair` gains a string-repair pass tried like its structural passes (escape illegal control bytes, terminate an unterminated string, strip a BOM, normalise structural smart quotes). `pkg/sysgen` and `pkg/worldgen` route every decode through a `generateJSON` helper that retries once with the parse failure appended to the prompt. A `ErrMalformedReply` sentinel maps to a friendly handler message, and enhancement entity ids are slugified client-side.

**Tech Stack:** Go 1.27, `encoding/json`, `gopkg.in/yaml.v3`; React 19, TypeScript (strict), Vitest.

**Spec:** `docs/superpowers/specs/2026-10-10-generation-input-robustness-design.md`
**Issue:** [#129](https://github.com/darkliquid/LocalRPG/issues/129)

## Global Constraints

- `pkg/jsonrepair` only removes, truncates, closes, or escapes; a pass is adopted only when the payload validates, and the `maxPayload` bound holds.
- Exactly one retry per model call, and only after a reply was received but failed to parse (a generate error does not retry). The deterministic oracle never retries.
- The raw error stays in the trace and the NDJSON `detail`; only the headline changes.

## File Map

| File | Change |
| --- | --- |
| `pkg/jsonrepair/repair.go`, `scan.go` | string pass, BOM, smart quotes, new kinds |
| `pkg/jsonrepair/repair_test.go` | table tests |
| `pkg/sysgen/sysgen.go` | `generateJSON`, `ErrMalformedReply`, uniform wrapping |
| `pkg/sysgen/steps.go`, `enhance.go`, `derive.go`, `explain.go` | call `generateJSON` |
| `pkg/worldgen/worldgen.go` | `generateJSON`, `ErrMalformedReply`, uniform wrapping |
| `pkg/worldgen/steps.go`, `entities.go`, `enhance.go` | call `generateJSON` |
| `pkg/gui/sysgen_routes.go`, `worldgen_routes.go`, `system_enhance.go`, `worldgen_entities.go` | friendly mapping |
| `frontend/src/components/WorldEnhanceDialog.tsx` | slugify ids before submit |

---

### Task 1: The string-repair pass

**Files:**
- Modify: `pkg/jsonrepair/repair.go`, `pkg/jsonrepair/scan.go`
- Test: `pkg/jsonrepair/repair_test.go`

- [x] **Step 1: Write the failing table tests**

Cases that must repair and validate: a literal newline/tab inside a string; an unterminated string (`{"a":"b`); a leading UTF-8 BOM; smart quotes used as delimiters (`{“a”:1}`). A case that must be **left alone**: a smart quote inside a value (`{"a":"it’s"}`), which stays byte-identical. For each repair, `json.Unmarshal` of the result must yield the original text.

- [x] **Step 2: Run them to verify they fail**

Run: `go test -run TestRepair ./pkg/jsonrepair/`
Expected: FAIL on the new cases.

- [x] **Step 3: Write the minimal implementation**

New kinds `KindEscapeControl`, `KindBOM`, `KindSmartQuote`. In `Repair`, after the existing fence/trim/close/trailing-comma chain: strip a leading BOM, then run a string pass that walks the payload in-string/escaped, escaping bytes below `0x20` (`\n`, `\r`, `\t`, else `\u00XX`). Normalise a structural smart quote (one immediately after `{`, `,`, or `[`, or before `:`, `}`, or `]`) to `"`. The spec's "terminate an unterminated string" is dropped: `TestRepairStripsFenceAndKeepsCompleteElements` guards that a reply cut mid-string stays unclosed, and the worldgen salvage path depends on that contract, so a truncated reply must not be reported as valid.

- [x] **Step 4: Run them to verify they pass**

Run: `go test -run TestRepair ./pkg/jsonrepair/`
Expected: PASS, including the smart-quote-inside-a-value negative.

- [x] **Step 5: Commit**

```bash
git add pkg/jsonrepair/repair.go pkg/jsonrepair/scan.go pkg/jsonrepair/repair_test.go
git commit -m "fix(jsonrepair): repair illegal bytes inside strings"
```

### Task 2: One bounded retry

**Files:**
- Modify: `pkg/sysgen/sysgen.go`, `pkg/sysgen/steps.go`, `pkg/sysgen/enhance.go`, `pkg/sysgen/derive.go`, `pkg/sysgen/explain.go`
- Modify: `pkg/worldgen/worldgen.go`, `pkg/worldgen/steps.go`, `pkg/worldgen/entities.go`, `pkg/worldgen/enhance.go`

- [x] **Step 1: Write the failing test**

```go
func TestGenerateJSONRetriesOnceOnAMalformedReply(t *testing.T) {
	gen := &scriptedGen{responses: []string{`{"a": not json}`, `{"a":1}`}}
	var out struct{ A int `json:"a"` }
	if err := generateJSON(context.Background(), gen, "p", "s", &out); err != nil {
		t.Fatalf("generateJSON: %v", err)
	}
	if out.A != 1 || gen.calls != 2 {
		t.Fatalf("out = %+v, calls = %d; want a decoded reply after a retry", out, gen.calls)
	}
}
```

A second case asserts a valid first reply makes exactly one call, and a third asserts two bad replies return an error after two calls.

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestGenerateJSON ./pkg/sysgen/`
Expected: FAIL, undefined.

- [x] **Step 3: Write the minimal implementation**

```go
// generateJSON calls the generator and decodes the reply, retrying once with the
// parse failure appended to the prompt before giving up. A generate error is
// returned as-is: there is no reply to re-ask about.
func generateJSON(ctx context.Context, gen Generator, prompt, schema string, v any) error
```

The retry prompt appends: "Your previous reply could not be parsed as JSON (reason: …). Reply with one JSON object only, escaping every newline inside a string as \n." Update the step call sites to `generateJSON(ctx, gen, prompt, schema, &out)`, dropping the separate `GenerateJSON` + `decodeJSON` pair, and keep the surrounding `fmt.Errorf("step: %w", err)` wrapping.

- [x] **Step 4: Run it to verify it passes**

Run: `go test ./pkg/sysgen/ ./pkg/worldgen/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/sysgen pkg/worldgen
git commit -m "feat(generation): retry a malformed model reply once"
```

### Task 3: A plain-language error

**Files:**
- Modify: `pkg/sysgen/sysgen.go`, `pkg/worldgen/worldgen.go`
- Modify: `pkg/gui/system_enhance.go`, `pkg/gui/sysgen_routes.go`, `pkg/gui/worldgen_entities.go`, `pkg/gui/worldgen_routes.go`
- Test: `pkg/gui/system_enhance_test.go`, `pkg/gui/worldgen_test.go`

- [x] **Step 1: Write the failing test** that a `decodeJSON` failure wraps `ErrMalformedReply`, and that the handler maps it to the friendly message (not `invalid character`).

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestMalformed ./pkg/gui/`
Expected: FAIL.

- [x] **Step 3: Write the minimal implementation**

`var ErrMalformedReply = errors.New("the model returned a reply that could not be read")`. Both `decodeJSON` functions wrap the unmarshal failure with it (sysgen gains the byte count and tail worldgen already has). The enhance and generate handlers map `errors.Is(err, ErrMalformedReply)` to a `502`/`code: "malformed_reply"` carrying "The model's reply could not be read. This is usually a one-off; try again, or switch to a different model." and keep the raw error in the `detail`.

- [x] **Step 4: Run it to verify it passes**

Run: `go test -run TestMalformed ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/sysgen pkg/worldgen pkg/gui
git commit -m "feat(generation): report an unreadable reply in plain language"
```

### Task 4: Slug client-supplied enhancement ids

**Files:**
- Modify: `frontend/src/components/WorldEnhanceDialog.tsx`
- Test: its test file

- [x] **Step 1: Write the failing test** that a non-slug entity id in an enhancement draft is slugified before submit.

- [x] **Step 2: Run it to verify it fails**

Run: `cd frontend && npx vitest run src/components/WorldEnhanceDialog.test.tsx`
Expected: FAIL.

- [x] **Step 3: Write the minimal implementation**

Slugify each entity id with the same rule the backend enforces before sending the apply request.

- [x] **Step 4: Run it to verify it passes**

Run: `cd frontend && npx vitest run src/components/WorldEnhanceDialog.test.tsx`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/WorldEnhanceDialog.tsx frontend/src/components/WorldEnhanceDialog.test.tsx
git commit -m "fix(frontend): slug enhancement entity ids before submit"
```

## Verification

- `go test ./...`, then `cd frontend && npx vitest run && npx tsc --noEmit`.
- `mise run lint`.
