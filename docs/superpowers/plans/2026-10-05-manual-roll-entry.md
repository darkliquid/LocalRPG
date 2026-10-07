# Manual Roll Entry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a player enter a physical dice result on the Roll card, resolved like a server roll and recorded as manual.

**Architecture:** IR-1's request gains `ManualDice`; the server sums dice and applies bonuses after; the Roll card gains an input with a range hint and a total preview.

**Tech Stack:** React 19; Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-manual-roll-entry-design.md`
**Depends on:** IR-1, IR-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- A manual entry sets the dice; bonuses apply after.
- A plausible entry is never rejected.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The request fields and the engine handling

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`, `pkg/engine/orchestrator.go` (the forced total)
- Test: `pkg/gui/resolve_check_test.go` (append)

**Interfaces:**
- Consumes: IR-1's forced total.
- Produces: `ManualDice`, summed and recorded.

- [ ] **Step 1: Write the failing tests**

```go
func TestManualDiceAreSummed(t *testing.T) {
	// ManualDice [4,3] -> total 7 plus bonuses, source manual.
}
func TestManualTotalPlusBonuses(t *testing.T) {
	// A manual total of 7 with a +2 stat records 9.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestManual -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `ManualDice []int` to the request; when present, sum it to the forced total; apply the SYS-1
bonuses after; set `Source = "manual"` and record the dice.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestManual -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui pkg/engine
git commit -m "feat(gui): resolve a manually entered roll"
```

---

### Task 2: The input affordance

**Files:**
- Modify: `frontend/src/components/PendingCheckCard.tsx`
- Create: `frontend/src/lib/diceRange.ts`
- Test: `frontend/src/lib/diceRange.test.ts`, `frontend/src/components/PendingCheckCard.test.tsx` (append)

**Interfaces:**
- Consumes: the request's notation and bonuses.
- Produces: an input with a range hint and a total preview.

- [ ] **Step 1: Write the failing tests**

```ts
import { diceRange } from "./diceRange";
test("ranges a simple notation", () => {
  expect(diceRange("2d6")).toEqual({ min: 2, max: 12 });
});
test("returns null for an unknown notation", () => {
  expect(diceRange("5d10")).toBeNull();
});
```
```tsx
test("shows the range and previews the total", () => { /* the hint and preview render */ });
test("blocks a non-number", () => { /* the submit is disabled */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- diceRange` and `npm run test -- PendingCheckCard`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the range helper (simple `NdM`), the input, the range hint, and the total preview (dice + bonuses).

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- diceRange` and `npm run test -- PendingCheckCard`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): enter a manual roll"
```

---

### Task 3: Recording and display

**Files:**
- Modify: `pkg/gui/types.go` (the check DTO), `frontend/src/components/DiceCheckCard.tsx`
- Test: `frontend/src/components/DiceCheckCard.test.tsx` (append)

**Interfaces:**
- Consumes: `Source`, `Roll.Dice`.
- Produces: "rolled 4 3 (manual) + Stealth 2 = 9".

- [ ] **Step 1: Write the failing test**

```tsx
test("shows a manual roll with its source", () => {
  render(<DiceCheckCard check={{ /* … */ source: "manual", roll: { dice: [{ value: 4 }, { value: 3 }] },
    applied: [{ source: "Stealth", value: 2 }], total: 9 }} />);
  expect(screen.getByText(/manual/)).toBeInTheDocument();
  expect(screen.getByText(/4/)).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- DiceCheckCard`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Render the source when it is `manual`, and the dice from `Roll.Dice`.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- DiceCheckCard`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat(frontend): show a manual roll honestly"
```

---

### Task 4: Verification

- [ ] **Step 1: Regression guard**

Add a test that pressing Roll with no entry is unchanged.

- [ ] **Step 2: Property test**

Add a test that for any entered dice, the recorded total equals the dice sum plus the bonuses.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- A manual total and manual dice both resolve.
- Bonuses apply after the entered dice.
- The source is recorded as manual.
- An out-of-range entry warns but is accepted.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the server-roll path"
```
