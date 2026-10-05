# Roll Mode Interactive Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a player Roll action produce a pending check the player resolves, and delete the dead proposed-check directive.

**Architecture:** Roll mode sets `result.PendingCheck` from the player's input and the system conventions, ending the turn like `propose_check`; `ProposedCheck` and the `[PROPOSED CHECK]` directive are removed; the console disables Roll when mechanics are off.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-roll-mode-interactive-design.md`
**Depends on:** IR-1, IR-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- The `@roll` record path under `auto` and `ask` must be unchanged.
- Conventional Commits, subject under 72 chars.

---

### Task 1: Roll mode pends

**Files:**
- Modify: `pkg/engine/orchestrator.go:708-717`
- Test: `pkg/engine/roll_mode_test.go`

**Interfaces:**
- Consumes: `harness.PendingCheck`, `rollRef` (`pkg/engine/streamsegments.go:97`).
- Produces: Roll mode ends with a pending check.

- [ ] **Step 1: Write the failing test**

```go
func TestRollModeProducesPendingCheck(t *testing.T) {
	o := newTestOrchestrator(t) // existing helper, engagement "auto"
	turn, err := o.ProcessAction(context.Background(), "roll", "pick the lock")
	if err != nil {
		t.Fatal(err)
	}
	if turn.PendingCheck == nil || turn.PendingCheck.ProposedBy != "player" {
		t.Fatalf("pending = %+v", turn.PendingCheck)
	}
	if turn.PendingCheck.Request.Actor == "" {
		t.Fatal("the request should name the actor")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestRollModeProducesPendingCheck -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Replace the Roll branch with a pending check built from `o.playerRollRequest(actionInput)`, set
`result.PendingCheck`, and return without generation, mirroring the `propose_check` handling.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run TestRollModeProducesPendingCheck -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/roll_mode_test.go
git commit -m "fix(engine): make a player roll produce a pending check"
```

---

### Task 2: `playerRollRequest`

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/roll_mode_test.go` (append)

**Interfaces:**
- Consumes: `core.CheckConventions`, the mechanics spec.
- Produces: `func (o *TurnOrchestrator) playerRollRequest(input string) harness.CheckRequest`.

- [ ] **Step 1: Write the failing test**

```go
func TestPlayerRollRequestUsesTheProfileNotation(t *testing.T) {
	o := newTestOrchestratorWithMechanics(t, &core.MechanicsSpec{Checks: core.CheckConventions{
		Notation: "2d6", Profiles: map[string]core.ResolutionProfile{"pbta": {Notation: "2d6"}}}})
	req := o.playerRollRequest("pick the lock")
	if req.Notation != "2d6" || req.Actor != o.playerID {
		t.Fatalf("request = %+v", req)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestPlayerRollRequest -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement `playerRollRequest` per the spec §4.1: actor is the player, notation from the profile or
conventions, `CheckKind` from the system default, and no stat/skill/stakes unless the input names
them.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run TestPlayerRollRequest -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/roll_mode_test.go
git commit -m "feat(engine): build a player roll request from conventions"
```

---

### Task 3: Remove the dead proposed-check path

**Files:**
- Modify: `pkg/engine/orchestrator.go`, `pkg/harness/turn.go`
- Delete: `pkg/engine/proposed_check_test.go`
- Test: `pkg/engine/` (adjust)

**Interfaces:**
- Consumes: nothing.
- Produces: `harness.ProposedCheck` and the directive removed.

- [ ] **Step 1: Grep for every reference**

Run:
```bash
rg -n "ProposedCheck|PROPOSED CHECK|proposedCheck" pkg frontend
```
Expected: the Roll branch, the struct, its test, and the directive builder. Record each.

- [ ] **Step 2: Remove them**

Delete `harness.ProposedCheck`, the `proposedCheck` variable and its use in `runGenerationLoop`'s
signature, the directive builder, and `pkg/engine/proposed_check_test.go`. Update any test that
asserted the directive.

- [ ] **Step 3: Build and test**

Run: `go build ./... && go vet ./... && go test ./pkg/engine/ ./pkg/harness/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/engine pkg/harness
git commit -m "refactor: remove the dead proposed-check directive"
```

---

### Task 4: Handle engagement `off`

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Modify: `frontend/src/components/ActionConsole.tsx`
- Test: `pkg/engine/roll_mode_test.go` (append), `frontend/src/components/ActionConsole.test.tsx` (append)

**Interfaces:**
- Consumes: the engagement setting.
- Produces: no pending check under `off`; the console disables Roll.

- [ ] **Step 1: Write the failing tests**

```go
func TestRollModeUnderOffProducesNoCheck(t *testing.T) {
	o := newTestOrchestratorWithEngagement(t, "off")
	turn, err := o.ProcessAction(context.Background(), "roll", "pick the lock")
	if err != nil {
		t.Fatal(err)
	}
	if turn.PendingCheck != nil {
		t.Fatalf("off should not pend, got %+v", turn.PendingCheck)
	}
}
```

```tsx
test("Roll is disabled when mechanics are off", () => {
  render(<ActionConsole engagement="off" /* … */ />);
  expect(screen.getByRole("button", { name: /roll/i })).toBeDisabled();
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine/ -run TestRollModeUnderOff -v` and `npm run test -- ActionConsole`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In Roll mode, if engagement is `off`, narrate that mechanics are disabled and produce no check. In
`ActionConsole.tsx`, disable the Roll action when engagement is `off` and show a tooltip.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -run TestRollModeUnderOff -v` and `npm run test -- ActionConsole`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/roll_mode_test.go frontend/src/components/ActionConsole.tsx frontend/src/components/ActionConsole.test.tsx
git commit -m "feat: honour mechanics-off for player rolls"
```

---

### Task 5: Verification

- [ ] **Step 1: Regression guard**

Add a test asserting the `@roll` record path under `auto` resolves immediately and under `ask` pends,
unchanged.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- A Roll action pends under `auto` and `ask`.
- No pending check under `off`; the console disables Roll.
- `ProposedCheck` and the directive are gone.
- The `@roll` record path is unchanged.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the record roll path"
```
