# Design Spec: Inline Check Results in the Chronicle

**Date:** 2026-09-26
**Status:** Proposed
**Target:** `pkg/harness` (`turn.go`), `pkg/entity` (`segment.go`), `pkg/engine` (`submission.go`, `orchestrator.go`, `check_resolver.go`, `timeline.go`), `pkg/rules` (`resolver.go`, `js_engine.go`), `pkg/storage` (`db.go`, `migrate.go`, `turn.go`), `pkg/gui` (`types.go`), `frontend` (`TurnSegments.tsx`, `ChronicleView.tsx`, `DiceCheckCard.tsx`, `types.ts`)

---

## 1. Executive Summary

When the GM resolves a check via the `request_check` tool, the engine records a `harness.CheckResult` on the turn (`Turn.Checks`, `pkg/engine/history.go:56`). The Chronicle currently renders these as a small detached monospace strip **after** the prose (`ChronicleView.tsx:132-141`):

```
2d6=9 pass
```

This is easy to miss, gives no sense of *where* in the narration the roll happened, and no sense of *why* it was rolled. The narration already renders inline as ordered segments (`TurnSegments.tsx`), with speech shown as distinct cards. Checks should be first-class inline elements too, interleaved at the point in the narration where they occurred, with:
- a small dice glyph set (inline SVG + numbers, never AI-generated),
- the notation, total (and successes where relevant),
- a colour-coded outcome (green success, yellow partial, red failure),
- a one-line note explaining the stakes.

Two structural gaps block this today:
1. `SegmentSpec` carries `check_ref` (`pkg/harness/turn.go:29`) and validation checks it (`pkg/engine/submission.go:122-124`), but `buildSegments` (`submission.go:14-37`) **drops it**, so a rendered segment cannot know which check it narrates.
2. `CheckResult` does not carry the request's stakes or check kind, so there is nothing to explain the roll.

---

## 2. Architecture & Data Flow

```
GM: request_check {actor, target, check_kind, stakes, notation, ...}
        |
        v
resolveCheck -> harness.CheckResult{CheckID, Actor, Target, CheckKind, Stakes, Roll, Outcome}
        |
        v
GM: submit_turn {segments:[{kind,text,check_ref}], ...}
        |
        v
buildSegments -> entity.TurnSegment{..., CheckRef}   (carry the ref)
        |
        v
Turn{Segments, Checks} -> history.jsonl + turns.checks_json (index)
        |
        v
TurnDTO{Segments, Checks} -> frontend
        |
        v
TurnSegments: merge stream = segments with a DiceCheckCard inserted
              before every segment whose check_ref matches a check
```

---

## 3. Detailed Component Designs

### 3.1 Data model

**`pkg/harness/turn.go`**
```go
type CheckResult struct {
    CheckID   string                 `json:"check_id"`
    Actor     string                 `json:"actor,omitempty"`
    Target    string                 `json:"target,omitempty"`
    CheckKind string                 `json:"check_kind,omitempty"`
    Stakes    string                 `json:"stakes,omitempty"`
    Roll      *RollSummary           `json:"roll"`
    Outcome   string                 `json:"outcome"`
    Breakdown map[string]interface{} `json:"breakdown,omitempty"`
}
```
- `CheckKind` and `Stakes` are copied from the `CheckRequest` when resolving, so every resolver (default, schema, JS) gets them uniformly. Set them in `TurnOrchestrator.resolveCheck` alongside the existing `CheckID`/`Actor`/`Target` assignment (`orchestrator.go:1467-1472`) rather than in each resolver.
- Keep `RollSummary` as-is. Rendering one die glyph per `RollCount` (falling back to the die count parsed from `Notation`) plus total/successes is sufficient; per-face values are an optional future extension.

**`pkg/entity/segment.go`**
```go
type TurnSegment struct {
    Kind      string `json:"kind"`
    Speaker   string `json:"speaker,omitempty"`
    SpeakerID string `json:"speaker_id,omitempty"`
    Text      string `json:"text"`
    CheckRef  string `json:"check_ref,omitempty"` // matches a CheckResult.CheckID
    Portrait  string `json:"-"`
    Player    bool   `json:"player,omitempty"`
}
```
(Field placement/JSON tags must match the existing struct; only add `CheckRef`.)

**`pkg/engine/submission.go`**: `buildSegments` copies `spec.CheckRef` onto every produced `TurnSegment` (including the speech-falls-back-to-narration branch), preserving the validation guarantee that the ref names a real check.

### 3.2 Persistence

Checks currently survive only in `history.jsonl` (`Turn.Checks`); the SQLite index stores only `roll_json` (`pkg/storage/turn.go:19-29`, `timeline.go:491-515`). Because the database is disposable and rebuilt by `EnsureIndexed`, this is not data loss, but the mirror query path (`GetEntityTurns`) cannot see checks.

- Add `ChecksJSON string` to `storage.TurnRecord` and a `checks_json TEXT` column to the `turns` table via a new idempotent migration in `pkg/storage/migrate.go` (use the existing `addColumns` helper, `migrate.go:151`; baseline table is `pkg/storage/db.go:31-40`).
- Update `SaveTurn`, `GetTurn`, and `ListTurns` (`pkg/storage/turn.go`) to read/write `COALESCE(checks_json, '')`.
- Update `Timeline.turnRecord` (`timeline.go:491-515`) to marshal `turn.Checks` into `ChecksJSON`.
- `EnsureIndexed` replay already re-saves turns from `history.jsonl`; because it calls the same `turnRecord`, checks are repopulated automatically.
- Add a migration test in `pkg/storage/migrate_test.go` asserting `turns.checks_json` exists after migrating a legacy DB and that `SaveTurn`/`GetTurn` round-trip checks.

### 3.3 API DTO

`pkg/gui/types.go` `TurnDTO.Checks []harness.CheckResult` already serializes the struct, so the new `check_kind`/`stakes` fields flow through automatically. `turnDTO` (`service.go:927`) needs no change beyond the struct. `SegmentDTO` must gain `CheckRef`:
```go
CheckRef string `json:"check_ref,omitempty"`
```
and the segment builders (`segmentDTOs`, `service.go:304-315`) must copy `entity.TurnSegment.CheckRef`.

### 3.4 Frontend types

`frontend/src/types.ts`:
```ts
export interface TurnSegment {
  kind: 'narration' | 'speech';
  speaker?: string;
  speaker_id?: string;
  text: string;
  check_ref?: string;        // new
  audio_url?: string;
  portrait_url?: string;
  audio_key?: string;
  player?: boolean;
  duration?: number;
}

export interface TurnCheck {
  check_id: string;
  actor?: string;
  target?: string;
  check_kind?: string;
  stakes?: string;
  outcome: string;
  roll?: { notation: string; total: number; successes?: number; roll_count?: number };
}
```

### 3.5 `DiceCheckCard` component

New `frontend/src/components/DiceCheckCard.tsx`:

```ts
type CheckTone = 'success' | 'partial' | 'failure' | 'neutral';
function classifyCheckOutcome(outcome: string): CheckTone;
```

Classification (case-insensitive, substring match, first hit wins):
- success: `success`, `pass`, `passed`, `critical` (unless `fail`/`failure` present), `crit_success`
- partial: `partial`, `mixed`, `success_with_cost`, `complication`
- failure: `fail`, `failure`, `crit_fail`, `critical_failure`
- anything else: `neutral`, rendered green if the outcome is truthy/permissive, red otherwise (fallback: green for `pass`, red for `fail`, else neutral)

Visual design (no AI images):
- A compact inline card, visually distinct from narration but lighter than a speech card. Left border tinted by tone; background `bg-black/30`.
- Dice glyphs: inline SVG. Render a pip-style `d6` glyph for each die when the notation is `Nd6`; for other dice render a generic polygonal die glyph with the notation label. Count = `roll.roll_count` when > 0, else the die count parsed from `notation`, capped at a small display max (e.g. 6) with a `+N` overflow badge. Each glyph is decorative (`aria-hidden`); the accessible label carries the numbers.
- Numbers: notation, `total`, and `successes` (when the check uses a success-count system).
- Outcome chip: tone-coloured text (`text-emerald-300`, `text-amber-300`, `text-rose-400`) and matching border.
- Stakes note: `check.stakes` when present; otherwise a fallback built from `actor`/`target`/`check_kind`, e.g. `"Kael vs Locked Door"` or `"Stealth check"`. Omit if nothing is available.
- `title` / `aria-label`: `"<actor> <notation> = <total>, <outcome>: <stakes>"`.

### 3.6 Inline merging

`TurnSegments.tsx` gains an optional `checks?: TurnCheck[]` prop. Build a render stream once:

1. Index checks by `check_id`.
2. Walk segments in order; before emitting a segment whose `check_ref` matches a check, emit that check once and mark it consumed.
3. After the walk, append any unconsumed checks in their array order.

This preserves authored order, places each roll immediately before the narration it produced, and never drops an unreferenced check (older turns, or checks the GM resolved but did not attach). `ChronicleView.tsx` passes `turn.checks` into `TurnSegments` and **removes** the detached text strip at `:132-141`. Speech/narration rendering is unchanged.

The Story Theater also renders `TurnSegments`; it may pass `checks` to show rolls inline there too, but this is optional and not required by this spec.

### 3.7 `Roll`-mode proposal

`Roll` mode is intentionally a proposal, not an executed roll (`orchestrator.go:530-538`), so no card is shown until the GM adopts it via `request_check`. No change.

---

## 4. Non-Goals

- No new dice-rolling engine and no change to the `rules` resolvers' outcome vocabulary.
- No AI-generated dice art (SVG/numbers only, per requirement).
- No interactive re-roll from the Chronicle; the card is read-only.
- No change to mechanical check memories (`timeline.go:552-582`) beyond what they already record.

---

## 5. Test Strategy

1. **`pkg/engine` tests**:
   - `buildSegments` copies `CheckRef` from `SegmentSpec` to `TurnSegment` for narration and speech, and for the speech-falls-back-to-narration branch.
   - `resolveCheck` copies `CheckKind` and `Stakes` from the request into the result.
2. **`pkg/storage` tests**:
   - Migration adds `checks_json` to a legacy database and `SaveTurn`/`GetTurn` round-trips checks.
3. **`pkg/gui` tests**:
   - `TurnDTO` JSON includes `checks[].stakes`, `checks[].check_kind`, and `segments[].check_ref`.
4. **Frontend build gate**: `npm --prefix frontend run build` passes.
5. **Manual verification**: roll a check in play and confirm the dice card appears before the narration it belongs to, with the correct tone colour, notation/total, and a stakes note; confirm old turns without `check_ref` still show their checks (appended); confirm success/partial/failure tone mapping.
