# Mechanics Result Presentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the full resolution on the check card and summarise a turn's mechanics in a strip.

**Architecture:** `DiceCheckCard` gains progressive sections (dice, modifiers, outcome, stakes, profile); `outcomeTone` derives tone from the system vocabulary; `MechanicsStrip` becomes the turn summary. Two small additive DTO fields carry stakes and the engagement level.

**Tech Stack:** React 19 + Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-10-05-mechanics-result-presentation-design.md`
**Depends on:** SYS-1, SYS-2.

## Global Constraints

- `noUnusedLocals`/`noUnusedParameters` are on.
- A turn with no checks renders exactly as today.
- Tone derives from the vocabulary, not hardcoded pass/fail.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The DTO additions

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`, `frontend/src/types.ts`
- Test: `pkg/gui/service_test.go` (append)

**Interfaces:**
- Consumes: `harness.CheckResult.Stakes`, the campaign engagement setting.
- Produces: `CheckDTO.Stakes`, `TurnDTO.Engagement`.

- [ ] **Step 1: Write the failing test**

```go
func TestTurnDTOCarriesEngagementAndStakes(t *testing.T) {
	turn := engine.Turn{Number: 1, Checks: []harness.CheckResult{{CheckID: "c1", Stakes: "the bridge holds"}}}
	dto := turnDTOForTestWithEngagement(t, turn, "auto")
	if dto.Engagement != "auto" || dto.Checks[0].Stakes != "the bridge holds" {
		t.Fatalf("dto = %+v", dto)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestTurnDTOCarriesEngagementAndStakes -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Stakes string` to `CheckDTO` and map it from `CheckResult.Stakes`. Add `Engagement string` to
`TurnDTO`, set from the resolved campaign engagement (system → campaign → config). Mirror both in
`frontend/src/types.ts`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestTurnDTOCarriesEngagementAndStakes -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/service_test.go frontend/src/types.ts
git commit -m "feat(gui): expose check stakes and turn engagement"
```

---

### Task 2: `outcomeTone`

**Files:**
- Create: `frontend/src/lib/checkTone.ts`
- Test: `frontend/src/lib/checkTone.test.ts`

**Interfaces:**
- Consumes: an outcome label and the vocabulary.
- Produces: `outcomeTone(outcome: string, vocabulary: string[]): "best" | "neutral" | "worst"`.

- [ ] **Step 1: Write the failing test**

```ts
import { outcomeTone } from "./checkTone";

test("maps vocabulary position to tone", () => {
  const v = ["strong", "weak", "miss"];
  expect(outcomeTone("strong", v)).toBe("best");
  expect(outcomeTone("weak", v)).toBe("neutral");
  expect(outcomeTone("miss", v)).toBe("worst");
  expect(outcomeTone("unknown", v)).toBe("neutral");
  expect(outcomeTone("pass", [])).toBe("best");
  expect(outcomeTone("fail", [])).toBe("worst");
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- checkTone`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```ts
export type Tone = "best" | "neutral" | "worst";

export function outcomeTone(outcome: string, vocabulary: string[]): Tone {
  const vocab = vocabulary.length > 0 ? vocabulary : ["pass", "fail"];
  const i = vocab.indexOf(outcome);
  if (i === 0) return "best";
  if (i === vocab.length - 1) return "worst";
  return "neutral";
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- checkTone`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/checkTone.ts frontend/src/lib/checkTone.test.ts
git commit -m "feat(frontend): derive check tone from the outcome vocabulary"
```

---

### Task 3: The check card sections

**Files:**
- Modify: `frontend/src/components/DiceCheckCard.tsx`
- Test: `frontend/src/components/DiceCheckCard.test.tsx` (append)

**Interfaces:**
- Consumes: `outcomeTone` (Task 2), the check DTO fields (SYS-1, SYS-2, Task 1).
- Produces: progressive sections.

- [ ] **Step 1: Write the failing tests**

```tsx
test("shows the modifier breakdown", () => {
  render(<DiceCheckCard check={check({ applied: [{ source: "Stealth", value: 3 }, { source: "wounded", value: -2 }] })} />);
  expect(screen.getByText(/Stealth/)).toBeInTheDocument();
  expect(screen.getByText(/-2/)).toBeInTheDocument();
});

test("shows stakes and outcome text", () => {
  render(<DiceCheckCard check={check({ stakes: "the bridge holds", outcome: "weak",
    outcome_text: "you succeed, at a cost" })} />);
  expect(screen.getByText(/the bridge holds/)).toBeInTheDocument();
  expect(screen.getByText(/at a cost/)).toBeInTheDocument();
});

test("simple check renders compactly", () => {
  const { container } = render(<DiceCheckCard check={check({ outcome: "strong" })} />);
  expect(container.querySelectorAll("[data-section]").length).toBeLessThanOrEqual(2);
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- DiceCheckCard`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Compose the card with the sections from the spec §4.1, each wrapped in a `data-section` attribute and
rendered only when it has content. Use `outcomeTone` for the card's tone and the vocabulary from the
check's system metadata.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- DiceCheckCard`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/DiceCheckCard.tsx frontend/src/components/DiceCheckCard.test.tsx
git commit -m "feat(frontend): show the full check resolution"
```

---

### Task 4: The mechanics strip

**Files:**
- Modify: `frontend/src/components/MechanicsStrip.tsx`, the turn view that mounts it
- Test: `frontend/src/components/MechanicsStrip.test.tsx`

**Interfaces:**
- Consumes: `TurnDTO.Checks`, `TurnDTO.Engagement`.
- Produces: a one-line summary, hidden when mechanics are off.

- [ ] **Step 1: Write the failing test**

```tsx
test("summarises checks and engagement", () => {
  render(<MechanicsStrip turn={turn({ engagement: "auto", checks: [check({ outcome: "weak" })] })} />);
  expect(screen.getByText(/checks: 1/i)).toBeInTheDocument();
  expect(screen.getByText(/auto/)).toBeInTheDocument();
});

test("says none on a quiet turn", () => {
  render(<MechanicsStrip turn={turn({ engagement: "auto", checks: [] })} />);
  expect(screen.getByText(/checks: none/i)).toBeInTheDocument();
});

test("renders nothing when mechanics are off", () => {
  const { container } = render(<MechanicsStrip turn={turn({ engagement: "off" })} />);
  expect(container).toBeEmptyDOMElement();
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- MechanicsStrip`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Render the summary line from the spec §4.3, with outcome chips. Return null when `engagement` is
empty or `off`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- MechanicsStrip`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/MechanicsStrip.tsx frontend/src/components/MechanicsStrip.test.tsx
git commit -m "feat(frontend): summarise a turn's mechanics"
```

---

### Task 5: Verification

- [ ] **Step 1: Regression guard**

Add a test asserting a turn with no checks renders identically to the pre-change output.

- [ ] **Step 2: Typecheck and full suite**

Run: `npx tsc --noEmit`, `npm run test` (in `frontend/`), and `mise run test:backend`
Expected: PASS.

- [ ] **Step 3: Confirm the acceptance criteria**

- The card shows dice, modifiers, outcome, stakes, and profile fields when present.
- Tone follows the vocabulary.
- The strip summarises checks and engagement and hides when mechanics are off.
- A check-free turn is unchanged.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the check-free turn rendering"
```
