# Record Repair Layer Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#66 RB-1](https://github.com/darkliquid/LocalRPG/issues/66)
**Epic:** [#23 Malformed output and playback integrity](https://github.com/darkliquid/LocalRPG/issues/23)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §5 (RB-1)
**Scope:** `pkg/turnstream`, new `pkg/jsonrepair`, `pkg/harness/extractor.go`, `pkg/engine`

---

## 1. Problem

The GM's reply is a line-framed stream of narration, speech, and `@`-prefixed control records.
`Parser.record` (`pkg/turnstream/parser.go:216-241`) accepts a record only when its payload is
already valid JSON:

```go
case !json.Valid(rec.Payload):
    rec.Err = fmt.Errorf("record %q payload is not JSON", rec.Type)
```

A record with `Err` is retained in `Parser.records` but produces no event
(`pkg/turnstream/parser.go:233-235`). Downstream, `applyRecords` logs and skips it
(`pkg/engine/streamsegments.go:50-53`) and `pendingRoll` skips it
(`pkg/engine/streamsegments.go:83`).

The consequences are silent and asymmetric:

- A malformed `@roll` means the model's resolution **vanishes**. The turn proceeds as if no roll
  happened, and nothing tells the player or the author why.
- A malformed `@persona` means the following `> Name: "…"` line is reclassified as narration
  (`pkg/turnstream/parser.go:173-177`), so dialogue is misattributed.
- A malformed `@state`/`@memory`/`@move` means the declaration is dropped.

The parser is also strictly line-framed (`Feed` splits on `\n`, `pkg/turnstream/parser.go:58-72`),
so a pretty-printed record whose JSON spans several lines is guaranteed to fail: the first line
carries an unbalanced `{`, and every subsequent line is parsed as narration.

There is a related, already-shipped repair elsewhere: the extractor trims to the outermost braces
before decoding (`pkg/harness/extractor.go:467-494`). It is inline, untested in isolation, and not
shared.

## 2. Goals

- A malformed control record is **repaired when the damage is structural**, and only then used.
- A record that cannot be repaired is **never silently lost**: it stays in `Parser.records` with
  its error and, once RB-5 lands, is surfaced.
- A pretty-printed multi-line record parses as one record.
- Repair is **deterministic** and **never invents content**: it may remove, truncate, or close
  structure, but it may not add meaning the model did not send.
- The repair primitive is a **shared leaf** the extractor and any future JSON-from-a-model path
  can reuse.

## 3. Non-goals

- Re-asking the model for a corrected reply. That is RB-4 (bounded malformed-response retry).
- Rendering diagnostics in the UI. That is RB-5; this spec defines the data RB-5 reads.
- Repairing narration or speech text. Only `@` record payloads are repaired.
- Semantic repair (for example inventing a missing `actor`). A record that parses but is
  semantically incomplete is out of scope; the decoders and validators own that.

## 4. Design

### 4.1 A shared leaf: `pkg/jsonrepair`

New package `pkg/jsonrepair`, importing only the standard library, so it can be used from
`pkg/turnstream`, `pkg/harness`, and `pkg/gui` without a cycle.

```go
package jsonrepair

// Kind names the structural repair that was applied, for diagnostics.
type Kind string

const (
    KindNone        Kind = ""            // payload was already valid
    KindFence       Kind = "fence"       // stripped a Markdown code fence
    KindTrim        Kind = "trim"        // removed prose around the JSON value
    KindClose       Kind = "close"       // appended missing closing braces/brackets
    KindTrailingComma Kind = "trailing_comma" // removed trailing commas
)

// Result reports what a repair did.
type Result struct {
    Payload []byte // the repaired payload; equals the input when KindNone
    Kind    Kind
    OK      bool   // Payload is valid JSON
}

// Repair attempts a bounded, deterministic structural repair of a payload that
// is expected to be a single JSON object or array.
func Repair(payload []byte) Result
```

`Repair` is bounded: it refuses payloads over `maxPayload` (64 KiB) and does no unbounded
scanning. It applies repairs in order and returns at the first that validates:

1. **None.** If `json.Valid(payload)`, return `{payload, KindNone, true}`. A valid payload is never
   altered, so behaviour for well-behaved models is unchanged.
2. **Fence.** If the trimmed payload starts with ``` ``` ``` (optionally ``` ```json ```), drop the
   opening fence line and a trailing fence.
3. **Trim.** Find the first `{` or `[`. Scan forward tracking brace/bracket depth, respecting
   string literals and `\` escapes, and take the substring up to the matching close. Leading prose
   and trailing prose are discarded. If the scan reaches the end without balance, take to the end
   and fall through to Close.
4. **Close.** Append the missing `}`/`]` characters (in reverse nesting order) to close any open
   structure.
5. **Trailing comma.** Remove commas that sit immediately before a `}` or `]` (ignoring commas
   inside strings).

Each step is followed by `json.Valid`. `OK` is true only when the result validates. A payload that
fails every step returns `{payload, KindNone, false}` so the caller keeps the original bytes for
the diagnostic.

The extractor's inline trim (`pkg/harness/extractor.go:467-494`) is rewritten to call
`jsonrepair.Repair` and keeps its current fallback (return the parse error).

### 4.2 Parser integration

`Parser.record` (`pkg/turnstream/parser.go:216`) changes from "valid or errored" to:

```go
switch {
case !validRecordType(rec.Type):
    rec.Err = fmt.Errorf("unknown record type %q", rec.Type)
case len(rec.Payload) == 0:
    rec.Err = fmt.Errorf("record %q has no payload", rec.Type)
default:
    if res := jsonrepair.Repair(rec.Payload); res.OK {
        if res.Kind != jsonrepair.KindNone {
            rec.Repaired = res.Kind
            rec.Payload = res.Payload
        }
    } else {
        rec.Err = fmt.Errorf("record %q payload is not JSON", rec.Type)
    }
}
```

`Record` gains one field (`pkg/turnstream/records.go:21-26`):

```go
// Repaired names the structural repair applied to Payload, or "" when the
// payload arrived valid. A repaired record still emits its event and still
// counts as usable; the field exists so a trace can say a repair happened.
Repaired jsonrepair.Kind
```

A repaired record is treated exactly like a valid one from here on: it produces its `KindRecord`
event, `declarePersona` runs for `@persona`, and `applyRecords`/`pendingRoll` accept it. The only
difference is the non-empty `Repaired` field.

### 4.3 Multi-line record accumulation

A record whose JSON spans lines is the most likely real breakage, because it is the one the model
cannot see it is producing. `consume` (`pkg/turnstream/parser.go:105`) gains a small accumulator:

- When a line starts with `@` and, after the type, the remainder has an **unbalanced** `{`/`[`
  (depth > 0 at end of line), the line is not parsed immediately. It is stored as
  `p.pendingRecord` (the raw text after `@`) and `Feed` returns no events for it.
- Each subsequent line is appended to `p.pendingRecord` with a `\n`, and depth is recomputed over
  the whole accumulated text. When depth returns to zero, the accumulated text is parsed as one
  record and the accumulator is cleared.
- The accumulator is bounded: it gives up after `maxRecordLines` (32) lines or `maxPayload`
  bytes, at which point the accumulated text is parsed as a (likely malformed) record, so nothing
  is lost and the failure is reported rather than hanging.
- `Flush` (`pkg/turnstream/parser.go:75`) parses any still-open accumulator as a record before
  flushing narration, so an unterminated record at end of stream is still reported.
- `Reset` (`pkg/turnstream/parser.go:97`) clears the accumulator.

Depth counting must respect strings and escapes, so a `{` inside a quoted string does not open a
structure. The same scanner as `jsonrepair` is reused via a small exported helper
`jsonrepair.BraceDepth(payload []byte) int` so the two cannot disagree.

This changes only how lines are grouped; the classification of non-record lines is unchanged.

### 4.4 Diagnostics data (for RB-5)

`Parser` gains a read-only summary so the turn can report degradation without RB-5's UI existing
yet:

```go
// RepairReport summarises how many records were repaired and how many failed.
type RepairReport struct {
    Total    int
    Repaired int
    Failed   int
    Kinds    map[jsonrepair.Kind]int // count per repair kind
}

func (p *Parser) RepairReport() RepairReport
```

`Records()` already exposes the per-record `Err` and the new `Repaired` field, so RB-5 can render
either the summary or the individual records. This spec only produces the data.

## 5. Behaviour

| Input | Result |
| --- | --- |
| `@roll {"actor":"x","check_kind":"do"}` | unchanged, `Repaired == ""` |
| ```@roll ```json {"actor":"x"}``` ``` | fence stripped, `Repaired == "fence"` |
| `@roll here you go: {"actor":"x"}` | prose trimmed, `Repaired == "trim"` |
| `@roll {"actor":"x"` | closed, `Repaired == "close"` |
| `@roll {"actor":"x",}` | comma removed, `Repaired == "trailing_comma"` |
| `@roll {` / `  "actor": "x"` / `}` | accumulated, parses as one record, `Repaired == ""` |
| `@roll not json at all` | `Err` set, no event, counted in `Failed` |
| `@bogus {"x":1}` | `Err` (unknown type), no event |
| `@roll {` never closed, 40 lines | parsed at the 32-line bound, `Err` set, reported |
| `@persona {"name":"Vex"}` malformed then repaired | roster declared, speech attributed |

## 6. Testing

- `pkg/jsonrepair/repair_test.go`: a table over every case in §5's first six rows, plus:
  - a valid payload is returned byte-identical (`KindNone`);
  - a payload over `maxPayload` is refused;
  - a `{` inside a string does not affect brace counting;
  - a repair that would require inventing a key is not attempted (it returns `OK == false`).
- `pkg/turnstream/parser_test.go` additions:
  - each repaired case produces a `KindRecord` event with the expected `Repaired` kind;
  - a multi-line `@roll` yields a record whose `DecodeRoll` succeeds;
  - an unrepairable record keeps `Err` and produces no event;
  - `Flush` reports an unterminated accumulator;
  - `Reset` clears the accumulator;
  - `RepairReport` counts repaired and failed records.
- `pkg/engine` regression: a turn stream containing a repaired `@roll` still ends on the roll
  (`pendingRoll` returns it) and the check resolves.
- `pkg/harness` regression: the extractor still parses the fenced/prose-wrapped cases it handled
  before, now through `jsonrepair`.
- A fuzz target `FuzzRepair` (seeded from the corpus) is added here and wired into RB-6's suite.

## 7. Rollout

No configuration. Repair is always on because it only ever makes a previously-dropped record
usable; a valid payload is never touched. The `Repaired` field and `RepairReport` are additive to
the DTO/trace surface and need no migration.

## 8. Risks

- **Over-repair.** A structural repair could turn a narration line that merely starts with `@`
  into a record. Mitigation: repair only runs on lines that already claim to be a record
  (`validRecordType`), and Trim only ever takes a substring that validates as JSON; anything else
  keeps the original `Err`.
- **Multi-line swallowing.** The accumulator could consume narration that follows an unterminated
  record. Mitigation: the line and byte bounds, and the depth check; a follow-up line that would
  not balance is eventually reported rather than silently merged. Covered by tests.
- **Divergent brace counting.** Two implementations of depth would drift. Mitigation: one exported
  `jsonrepair.BraceDepth` used by both.
- **Performance.** Repair runs only on the invalid path, so the happy path is one `json.Valid`
  call, as today.
