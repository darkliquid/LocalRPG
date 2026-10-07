# Chronicle Inline Check Results Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render dice checks inline in the Chronicle where they were rolled, with an SVG dice glyph, the numbers, a colour-coded outcome, and the stakes.

**Architecture:** Carry `check_ref` from the GM's authored segments onto stored `TurnSegment`s, persist the check kind and stakes on `CheckResult`, mirror checks into the SQLite index, and merge a new `DiceCheckCard` into the Chronicle's segment stream at the referencing segment.

**Tech Stack:** Go 1.27 (stdlib tests), React 19 + TypeScript, Tailwind v4, inline SVG.

**Spec:** `docs/superpowers/specs/2026-09-26-chronicle-inline-check-results-design.md`

## Global Constraints

- Use `any`, not `interface{}`. Wrap errors with `fmt.Errorf("...: %w", err)`. `go vet` clean.
- Do not AI-generate dice art; inline SVG plus numbers only.
- Migrations are ordered and idempotent; new version is 7.
- TypeScript `strict`, `noUnusedLocals`, `noUnusedParameters`.
- Test gates: `mise run test:backend`, `mise run lint`, `mise run test:frontend`.
- Do not commit unless the user asks.

---

## File Map

- Modify: `pkg/harness/turn.go` (CheckResult fields)
- Modify: `pkg/entity/segment.go` (CheckRef)
- Modify: `pkg/engine/orchestrator.go` (resolveCheck copies kind/stakes)
- Modify: `pkg/engine/submission.go` (buildSegments carries CheckRef)
- Modify: `pkg/engine/submission_test.go` (or new `pkg/engine/segment_checkref_test.go`)
- Modify: `pkg/storage/db.go`, `pkg/storage/migrate.go`, `pkg/storage/turn.go`, `pkg/storage/migrate_test.go`
- Modify: `pkg/engine/timeline.go` (turnRecord)
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go` (SegmentDTO.CheckRef)
- Modify: `pkg/gui/character_portrait_test.go`? no; add to `pkg/gui/segment_check_test.go`
- Modify: `frontend/src/types.ts`
- Create: `frontend/src/components/DiceCheckCard.tsx`
- Modify: `frontend/src/components/TurnSegments.tsx`, `frontend/src/components/ChronicleView.tsx`

---

### Task 1: Carry check identity and stakes through the turn

**Files:**
- Modify: `pkg/harness/turn.go`, `pkg/entity/segment.go`, `pkg/engine/orchestrator.go`, `pkg/engine/submission.go`
- Create: `pkg/engine/segment_checkref_test.go`

**Interfaces:**
- Produces: `CheckResult.CheckKind`, `CheckResult.Stakes`; `entity.TurnSegment.CheckRef`; write-side population.

- [x] **Step 1: Add the fields**

In `pkg/harness/turn.go`, `CheckResult` gains (after `Target`):

```go
	CheckKind string                 `json:"check_kind,omitempty"`
	Stakes    string                 `json:"stakes,omitempty"`
```

In `pkg/entity/segment.go`, `TurnSegment` gains after `Text`:

```go
	// CheckRef names the CheckResult whose roll this segment narrates, so the
	// chronicle can render the dice inline rather than in a detached strip.
	CheckRef string `json:"check_ref,omitempty"`
```

- [x] **Step 2: Copy kind and stakes in `resolveCheck`**

Replace `TurnOrchestrator.resolveCheck` in `pkg/engine/orchestrator.go`:

```go
func (o *TurnOrchestrator) resolveCheck(ctx context.Context, req harness.CheckRequest, actor *entity.Entity) (*harness.CheckResult, error) {
	resolver := o.checkResolver
	if resolver == nil {
		resolver = defaultCheckResolver{}
	}
	resolved, err := resolver.Resolve(ctx, req, actor)
	if err != nil {
		return nil, err
	}
	if resolved.CheckKind == "" {
		resolved.CheckKind = req.CheckKind
	}
	if resolved.Stakes == "" {
		resolved.Stakes = req.Stakes
	}
	return resolved, nil
}
```

- [x] **Step 3: Carry `CheckRef` in `buildSegments`**

In `pkg/engine/submission.go`, set `CheckRef: spec.CheckRef` on every produced `TurnSegment` (the speech branch, the speech-falls-back-to-narration branch, and the narration branch).

- [x] **Step 4: Tests**

Create `pkg/engine/segment_checkref_test.go`:

```go
package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestBuildSegmentsCarriesCheckRef(t *testing.T) {
	sub := &harness.TurnSubmission{
		Segments: []harness.SegmentSpec{
			{Kind: "narration", Text: "One.", CheckRef: "chk_a"},
			{Kind: "speech", Speaker: "Elena", Text: "Two.", CheckRef: "chk_b"},
			{Kind: "speech", Speaker: "Nobody", Text: "Three.", CheckRef: "chk_c"},
		},
	}
	resolve := func(speaker string) (string, bool) {
		if speaker == "Elena" {
			return "elena", true
		}
		return "", false
	}
	_, segments := buildSegments(sub, resolve)
	if len(segments) != 3 {
		t.Fatalf("segments = %d, want 3", len(segments))
	}
	want := []string{"chk_a", "chk_b", "chk_c"}
	for i, ref := range want {
		if segments[i].CheckRef != ref {
			t.Errorf("segment %d CheckRef = %q, want %q", i, segments[i].CheckRef, ref)
		}
	}
}

type stubCheckResolver struct{}

func (stubCheckResolver) Resolve(context.Context, harness.CheckRequest, *entity.Entity) (*harness.CheckResult, error) {
	return &harness.CheckResult{CheckID: "chk_x", Outcome: "pass"}, nil
}

func TestResolveCheckCopiesKindAndStakes(t *testing.T) {
	o := &TurnOrchestrator{checkResolver: stubCheckResolver{}}
	got, err := o.resolveCheck(context.Background(), harness.CheckRequest{
		CheckKind: "stealth", Stakes: "The guard wakes",
	}, nil)
	if err != nil {
		t.Fatalf("resolveCheck failed: %v", err)
	}
	if got.CheckKind != "stealth" || got.Stakes != "The guard wakes" {
		t.Fatalf("kind=%q stakes=%q", got.CheckKind, got.Stakes)
	}
}
```

Add the `entity` import to the test.

- [x] **Step 5: Run**

Run: `go test -count=1 ./pkg/engine/ -run 'TestBuildSegmentsCarriesCheckRef|TestResolveCheckCopiesKindAndStakes' -v`
Expected: PASS.

---

### Task 2: Mirror checks into the index

**Files:**
- Modify: `pkg/storage/db.go`, `pkg/storage/migrate.go`, `pkg/storage/turn.go`, `pkg/storage/migrate_test.go`
- Modify: `pkg/engine/timeline.go`

**Interfaces:**
- Produces: `turns.checks_json`; `storage.TurnRecord.ChecksJSON`.

- [x] **Step 1: Baseline column and migration**

In `pkg/storage/db.go`, add `checks_json TEXT` to the `turns` baseline `CREATE TABLE` after `roll_json`.

In `pkg/storage/migrate.go`, add `{version: 7, apply: addChecksColumn}` to `migrations`, and:

```go
// addChecksColumn stores the checks a turn resolved so the index mirrors
// history.jsonl and a rebuilt database regains them.
func addChecksColumn(db *sql.DB) error {
	exists, err := columnExists(db, "turns", "checks_json")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE turns ADD COLUMN checks_json TEXT"); err != nil {
		return fmt.Errorf("add turns.checks_json: %w", err)
	}
	return nil
}
```

- [x] **Step 2: Record field and SQL**

In `pkg/storage/turn.go`, add `ChecksJSON string` to `TurnRecord`, extend the `SaveTurn` upsert (`checks_json` column and `excluded.checks_json`), and add `COALESCE(checks_json, '')` to the `GetTurn` and `ListTurns` selects and scans.

- [x] **Step 3: Marshal checks in `turnRecord`**

In `pkg/engine/timeline.go` `turnRecord`, after the roll block:

```go
	if len(turn.Checks) > 0 {
		if data, err := json.Marshal(turn.Checks); err == nil {
			rec.ChecksJSON = string(data)
		}
	}
```

- [x] **Step 4: Test**

Add to `pkg/storage/migrate_test.go` (or a new `pkg/storage/checks_test.go`):

```go
func TestTurnsChecksJSONRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	exists, err := columnExists(store.db, "turns", "checks_json")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected turns.checks_json after migration")
	}

	rec := TurnRecord{
		Number: 1, Timestamp: time.Now().UTC(), Mode: "Do", Input: "look",
		Narration: "You look.", ChecksJSON: `[{"check_id":"chk_1","outcome":"pass"}]`,
	}
	if err := store.SaveTurn(rec); err != nil {
		t.Fatalf("SaveTurn: %v", err)
	}
	got, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if got.ChecksJSON != rec.ChecksJSON {
		t.Fatalf("ChecksJSON = %q, want %q", got.ChecksJSON, rec.ChecksJSON)
	}
}
```

- [x] **Step 5: Run**

Run: `go test -count=1 ./pkg/storage/ ./pkg/engine/`
Expected: PASS.

---

### Task 3: Expose `check_ref` on the segment DTO

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`
- Create: `pkg/gui/segment_check_test.go`

- [x] **Step 1: DTO field**

In `pkg/gui/types.go`, add to `SegmentDTO`:

```go
	CheckRef string `json:"check_ref,omitempty"`
```

- [x] **Step 2: Copy it**

In `pkg/gui/service.go` `segmentDTOs`, add `CheckRef: segment.CheckRef,` to the `SegmentDTO` literal.

- [x] **Step 3: Test**

Create `pkg/gui/segment_check_test.go`:

```go
package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestSegmentDTOsCarryCheckRef(t *testing.T) {
	segments := []entity.TurnSegment{
		{Kind: "narration", Text: "A roll.", CheckRef: "chk_1"},
		{Kind: "narration", Text: "No roll."},
	}
	dtos := segmentDTOs(segments, "test-game", 1, false, nil, nil)
	if dtos[0].CheckRef != "chk_1" {
		t.Fatalf("dtos[0].CheckRef = %q, want chk_1", dtos[0].CheckRef)
	}
	if dtos[1].CheckRef != "" {
		t.Fatalf("dtos[1].CheckRef = %q, want empty", dtos[1].CheckRef)
	}
}
```

`TurnDTO.Checks` already serializes `harness.CheckResult`, so `check_kind`/`stakes` flow with no further change.

- [x] **Step 4: Run**

Run: `go test -count=1 ./pkg/gui/ -run TestSegmentDTOsCarryCheckRef -v`
Expected: PASS.

---

### Task 4: Frontend types and the dice card

**Files:**
- Modify: `frontend/src/types.ts`
- Create: `frontend/src/components/DiceCheckCard.tsx`

- [x] **Step 1: Types**

In `frontend/src/types.ts`, add `check_ref?: string;` to `TurnSegment`, and replace `TurnCheck`:

```ts
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

- [x] **Step 2: The dice card**

Create `frontend/src/components/DiceCheckCard.tsx`:

```tsx
import React from 'react';
import { TurnCheck } from '../types';

export type CheckTone = 'success' | 'partial' | 'failure' | 'neutral';

// classifyCheckOutcome maps a resolver's arbitrary outcome word to a tone. The
// order matters: a costed success is partial, and a critical failure is a
// failure before it is anything else.
export function classifyCheckOutcome(outcome: string): CheckTone {
  const value = (outcome || '').trim().toLowerCase();
  if (!value) return 'neutral';
  if (/(partial|mixed|success_with_cost|complication)/.test(value)) return 'partial';
  if (/(fail|failure)/.test(value)) return 'failure';
  if (/(success|pass|critical|succeed)/.test(value)) return 'success';
  return 'neutral';
}

function diceCount(notation?: string): number {
  if (!notation) return 1;
  const match = notation.match(/(\d*)\s*d\s*(\d+)/i);
  if (!match) return 1;
  const count = match[1] ? parseInt(match[1], 10) : 1;
  return Number.isFinite(count) && count > 0 ? count : 1;
}

function dieSize(notation?: string): string {
  const match = notation?.match(/d\s*(\d+)/i);
  return match ? match[1] : '?';
}

const TONE_STYLES: Record<CheckTone, { border: string; chip: string }> = {
  success: { border: 'border-emerald-500/60', chip: 'text-emerald-300' },
  partial: { border: 'border-amber-500/60', chip: 'text-amber-300' },
  failure: { border: 'border-rose-500/60', chip: 'text-rose-400' },
  neutral: { border: 'border-stone-500/50', chip: 'text-stone-300' },
};

export const DiceCheckCard: React.FC<{ check: TurnCheck }> = ({ check }) => {
  const tone = classifyCheckOutcome(check.outcome);
  const style = TONE_STYLES[tone];
  const roll = check.roll;
  const notation = roll?.notation ?? 'check';
  const count = Math.max(1, roll?.roll_count && roll.roll_count > 0 ? roll.roll_count : diceCount(roll?.notation));
  const shown = Math.min(count, 6);
  const size = dieSize(roll?.notation);
  const stakes =
    (check.stakes ?? '').trim() ||
    [check.actor, check.target].filter(Boolean).join(' vs ') ||
    (check.check_kind ?? '').trim();
  const label = `${check.actor ? `${check.actor} ` : ''}${notation}${roll ? ` = ${roll.total}` : ''}, ${check.outcome}${
    stakes ? `: ${stakes}` : ''
  }`;

  return (
    <div className={`my-3 rounded-xl border-l-4 ${style.border} bg-black/30 px-3 py-2 space-y-1`} aria-label={label}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="flex items-center gap-1" aria-hidden="true">
          {Array.from({ length: shown }).map((_, index) => (
            <svg key={index} viewBox="0 0 24 24" className="w-5 h-5 text-stone-300">
              <rect
                x="2"
                y="2"
                width="20"
                height="20"
                rx="5"
                fill="currentColor"
                opacity="0.12"
                stroke="currentColor"
                strokeWidth="1.5"
              />
              <text x="12" y="15.5" textAnchor="middle" fontSize="9" fill="currentColor" fontFamily="monospace">
                {size}
              </text>
            </svg>
          ))}
          {count > shown && <span className="text-[10px] font-mono text-stone-400">+{count - shown}</span>}
        </span>
        <span className="text-xs font-mono text-stone-300">{notation}</span>
        {roll && <span className="text-xs font-mono text-stone-400">&rarr; {roll.total}</span>}
        {roll && roll.successes !== undefined && roll.successes > 0 && (
          <span className="text-xs font-mono text-stone-400">{roll.successes} successes</span>
        )}
        <span className={`text-[11px] font-sans font-bold uppercase tracking-wider ${style.chip}`}>{check.outcome}</span>
      </div>
      {stakes && <div className="text-[11px] font-sans text-stone-400">{stakes}</div>}
    </div>
  );
};
```

- [x] **Step 3: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0 (the component is not yet imported, so no unused errors).

---

### Task 5: Merge checks into the segment stream

**Files:**
- Modify: `frontend/src/components/TurnSegments.tsx`, `frontend/src/components/ChronicleView.tsx`

- [x] **Step 1: TurnSegments prop and stream**

In `TurnSegments.tsx`, import `TurnCheck` from `../types` and `DiceCheckCard` from `./DiceCheckCard`; add `checks?: TurnCheck[];` to `TurnSegmentsProps` and destructure `checks`.

Before `return`, build the stream:

```tsx
  const checkByID = new Map((checks ?? []).map((check) => [check.check_id, check]));
  const usedChecks = new Set<string>();
  const stream: Array<{ segment?: TurnSegment; check?: TurnCheck; index?: number }> = [];
  ordered.forEach((segment, index) => {
    if (segment.check_ref) {
      const check = checkByID.get(segment.check_ref);
      if (check && !usedChecks.has(check.check_id)) {
        usedChecks.add(check.check_id);
        stream.push({ check });
      }
    }
    stream.push({ segment, index });
  });
  for (const check of checks ?? []) {
    if (!usedChecks.has(check.check_id)) stream.push({ check });
  }
```

- [x] **Step 2: Render the stream**

Replace `{ordered.map((segment, i) => ...)}` with:

```tsx
      {stream.map((item, streamIndex) => {
        if (item.check) {
          return <DiceCheckCard key={`check-${item.check.check_id}`} check={item.check} />;
        }
        const segment = item.segment as TurnSegment;
        const i = item.index as number;
        return segment.kind === 'speech' ? (
          <div
            key={streamIndex}
            className={`group relative bg-glass-card border-l-4 pl-4 py-3 pr-4 rounded-r-xl shadow-lg space-y-2 ${
              segment.player ? 'border-sky-400/90' : 'border-purple-500/90'
            }`}
          >
            {/* ...the existing speech body unchanged... */}
          </div>
        ) : (
          <div key={streamIndex} className="group relative">
            {/* ...the existing narration body unchanged... */}
          </div>
        );
      })}
```

Preserve the existing speech and narration bodies exactly; only the map callback shape and keys change.

- [x] **Step 3: ChronicleView passes checks and drops the strip**

Add `checks={turn.checks}` to the `TurnSegments` render, and delete the detached block:

```tsx
              {turn.checks && turn.checks.length > 0 && (
                <div className="text-xs font-mono text-stone-400">
                  ...
                </div>
              )}
```

- [x] **Step 4: Typecheck and build**

Run: `mise run test:frontend && mise run build:frontend`
Expected: exit 0.

---

### Task 6: Verification

- [x] **Step 1: Backend**

Run: `mise run test:backend && mise run lint`
Expected: PASS, clean.

- [x] **Step 2: Frontend**

Run: `mise run test:frontend`
Expected: exit 0.

- [x] **Step 3: Manual**

Play a turn that resolves a check and confirm the dice card appears before the narration it belongs to, with the correct tone colour, notation/total, and stakes; confirm an old turn still shows checks (appended) and that `pass`/`partial`/`fail` map to green/amber/red.

---

## Self-Review

**Spec coverage:** check identity carried (Task 1), kind/stakes persisted (Task 1), index mirror (Task 2), DTO (Task 3), dice card + tone (Task 4), inline merge + strip removal (Task 5), verification (Task 6).

**Placeholder scan:** none (the "existing body unchanged" comments refer to already-present code in the same file, not new code to invent).

**Type consistency:** `CheckRef`/`CheckKind`/`Stakes` names match across harness/entity/engine/gui; `TurnCheck` fields match `DiceCheckCard` and `TurnSegments`; `classifyCheckOutcome` is used only in the card.
