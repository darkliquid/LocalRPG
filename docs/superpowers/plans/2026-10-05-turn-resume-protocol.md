# Turn Resume Protocol Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An opt-in single-turn mode where a pending check and its adjudication are one turn record, completed in place.

**Architecture:** A config value selects `continuation` (default) or `single-turn`; in single-turn mode the proposing turn is written as a `Draft`, and resolve-check completes it via `Timeline.ReplaceTurn` (a controlled rewind of the final turn).

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-turn-resume-protocol-design.md`
**Depends on:** IR-1, IR-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- The default mode must be byte-for-byte the IR-1 behaviour.
- Only the final turn may be replaced.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The mode configuration

**Files:**
- Modify: `pkg/config/types.go`, `pkg/config/manager.go` (accessor)
- Test: `pkg/config/types_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `Config.Interactive.InteractiveRolls` (yaml `interactive.rolls`), `func (c *Config) InteractiveRolls() string` returning `"continuation"` by default.

- [ ] **Step 1: Write the failing test**

```go
func TestInteractiveRollsDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.InteractiveRolls() != "continuation" {
		t.Fatalf("default = %q", cfg.InteractiveRolls())
	}
	cfg.Interactive.Rolls = "single-turn"
	if cfg.InteractiveRolls() != "single-turn" {
		t.Fatalf("override = %q", cfg.InteractiveRolls())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestInteractiveRollsDefault -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add an `InteractiveConfig{ Rolls string \`yaml:"rolls,omitempty"\` }` field on `Config` and the
accessor that normalises an empty or unknown value to `"continuation"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -run TestInteractiveRollsDefault -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/manager.go pkg/config/types_test.go
git commit -m "feat(config): add the interactive rolls mode"
```

---

### Task 2: The draft flag and its write

**Files:**
- Modify: `pkg/engine/history.go`, `pkg/engine/orchestrator.go`, `pkg/engine/timeline.go`
- Test: `pkg/engine/timeline_test.go` (append)

**Interfaces:**
- Consumes: the mode (Task 1).
- Produces: `Turn.Draft bool`, and a draft write when a turn ends on a pending check in single-turn mode.

- [ ] **Step 1: Write the failing test**

```go
func TestDraftTurnRoundTrips(t *testing.T) {
	turn := Turn{Number: 1, Draft: true, PendingCheck: &harness.PendingCheck{Ref: "r"}}
	got := roundTripTurn(t, turn)
	if !got.Draft {
		t.Fatal("the draft flag did not survive the round trip")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestDraftTurnRoundTrips -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Draft bool \`json:"draft,omitempty"\`` to `Turn`. In the turn assembly, set `Draft = true` when
the mode is `single-turn` and the turn has a pending check. Ensure `RecordTurn` persists it.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run TestDraftTurnRoundTrips -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/history.go pkg/engine/orchestrator.go pkg/engine/timeline.go pkg/engine/timeline_test.go
git commit -m "feat(engine): mark a pending turn as a draft"
```

---

### Task 3: `Timeline.ReplaceTurn`

**Files:**
- Modify: `pkg/engine/timeline.go`
- Test: `pkg/engine/timeline_test.go` (append)

**Interfaces:**
- Consumes: `RewindToTurn`.
- Produces: `func (t *Timeline) ReplaceTurn(ctx context.Context, turn Turn) error`.

- [ ] **Step 1: Write the failing tests**

```go
func TestReplaceTurnOnlyFinal(t *testing.T) {
	// Replacing the final turn trims it and records the replacement.
	// Replacing a non-final turn returns an error.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine/ -run TestReplaceTurn -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement `ReplaceTurn`: load history, verify `turn.Number` is the final turn (else error), call
`RewindToTurn(turn.Number - 1)`, then `RecordTurn(turn)`. Reuse the existing rewind so entity
history pruning is identical.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -run TestReplaceTurn -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/timeline.go pkg/engine/timeline_test.go
git commit -m "feat(engine): complete a draft turn in place"
```

---

### Task 4: Resolve-check branches on the mode

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/resolve_check_test.go` (append)

**Interfaces:**
- Consumes: Tasks 1-3.
- Produces: single-turn completion; continuation otherwise.

- [ ] **Step 1: Write the failing tests**

```go
func TestSingleTurnModeCompletesInPlace(t *testing.T) {
	// In single-turn mode, resolving yields one turn carrying the resolved check
	// and the adjudication; the turn count does not increase.
}

func TestContinuationModeUnchanged(t *testing.T) {
	// In continuation mode, a new turn is appended with ContinuationOf set.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run 'TestSingleTurnMode|TestContinuationMode' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In `Service.ResolveCheck` (IR-1), branch on `cfg.InteractiveRolls()`: in `single-turn` mode, after
generating the adjudication, call `timeline.ReplaceTurn` with a turn that carries the pending check
(now resolved), the resolved checks, and the full segments; in `continuation` mode, keep the
existing path.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run 'TestSingleTurnMode|TestContinuationMode' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/resolve_check_test.go
git commit -m "feat(gui): complete a pending turn in place in single-turn mode"
```

---

### Task 5: The chronicle renders a draft

**Files:**
- Modify: `frontend/src/types.ts`, `frontend/src/components/TurnSegments.tsx`
- Test: `frontend/src/components/TurnSegments.test.tsx` (append)

**Interfaces:**
- Consumes: `Turn.Draft`.
- Produces: a draft renders the pending card only; a completed turn renders normally.

- [ ] **Step 1: Write the failing tests**

```tsx
test("a draft renders only the pending card", () => { /* … */ });
test("a completed turn renders the resolved check", () => { /* … */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- TurnSegments`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `draft?: boolean` to the turn type. When `draft` is true, render only the pending card (IR-2)
and suppress the narration/segment list.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- TurnSegments`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/TurnSegments.tsx frontend/src/components/TurnSegments.test.tsx
git commit -m "feat(frontend): render a draft turn as waiting"
```

---

### Task 6: Verification

- [ ] **Step 1: Default-mode guard**

Add a test asserting `continuation` mode is byte-for-byte the IR-1 behaviour (a new turn appended,
no draft).

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Single-turn mode yields one turn per scene.
- A draft persists and reloads as waiting.
- `ReplaceTurn` refuses a non-final turn.
- The default mode is unchanged.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the continuation-mode default"
```
