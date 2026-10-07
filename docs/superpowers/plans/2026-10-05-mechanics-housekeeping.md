# Mechanics Housekeeping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the dead action-verdict vocabulary, give `newCheckID` one definition, and make the Systems Studio doc match the code.

**Architecture:** Pure deletion and one small move. `harness.NewCheckID` becomes the single check-id definition; the never-assigned `Turn.Verdict` and its `ActionVerdict`/`ActionFeasibility` vocabulary are removed; the studio article is corrected.

**Tech Stack:** Go standard library; React 19 + Tailwind v4; the embedded docs are Vale-linted.

**Spec:** `docs/superpowers/specs/2026-10-05-mechanics-housekeeping-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- Do not add comments beyond the ones specified; the goal is less noise, not more.
- Conventional Commits, subject under 72 chars.

---

### Task 1: Confirm there are no hidden readers

**Files:**
- Read only.

- [ ] **Step 1: Grep every target across the whole tree**

Run:
```bash
rg -n "ActionVerdict|ActionFeasibility|FeasibilityAutomatic|FeasibilityUncertain|FeasibilityImpossible|Turn\.Verdict|\.Verdict|dismissed_checks|rollDice|action_modes" pkg frontend
```

Expected: the hits listed in the spec, plus possibly a frontend `verdict`. Record every path:line;
each must be addressed in a later task. If a hit appears that is not in the spec, stop and update
the spec before deleting.

- [ ] **Step 2: Commit**

No commit (investigation only).

---

### Task 2: Remove the verdict vocabulary from Go

**Files:**
- Modify: `pkg/engine/history.go` (remove `Turn.Verdict`)
- Modify: `pkg/harness/turn.go` (remove `ActionVerdict`, `ActionFeasibility`, the three constants)
- Modify: `pkg/gui/types.go` (remove `TurnDTO.Verdict`)
- Modify: `pkg/gui/service.go` (remove the `Verdict:` mapping)
- Modify: `pkg/engine/mechanics_engagement.go` (remove the `verdict` hook-context block)

**Interfaces:**
- Consumes: nothing.
- Produces: nothing new; removes `Turn.Verdict` and the harness vocabulary.

- [ ] **Step 1: Delete the `Turn.Verdict` field**

In `pkg/engine/history.go`, delete:

```go
	Verdict  *harness.ActionVerdict `json:"verdict,omitempty"`
```

Remove the now-unused `harness` import from `pkg/engine/history.go` if nothing else uses it.

- [ ] **Step 2: Delete the harness vocabulary**

In `pkg/harness/turn.go`, delete lines 5-19 (the `ActionFeasibility` type, its constants, and
`ActionVerdict`). Keep `ProposedCheck` and `PendingCheck`.

- [ ] **Step 3: Delete the DTO field and mapping**

In `pkg/gui/types.go`, delete `Verdict *harness.ActionVerdict \`json:"verdict,omitempty"\`` from
`TurnDTO`. In `pkg/gui/service.go:1525`, delete the `Verdict: turn.Verdict,` line from the `turnDTO`
literal. Remove the `harness` import from `pkg/gui/types.go` if it becomes unused.

- [ ] **Step 4: Delete the hook-context block**

In `pkg/engine/mechanics_engagement.go`, delete:

```go
	if turn.Verdict != nil {
		hookCtx["verdict"] = string(turn.Verdict.Feasibility)
	}
```

- [ ] **Step 5: Build and test**

Run: `go build ./... && go vet ./... && go test ./pkg/engine/ ./pkg/gui/ ./pkg/harness/`
Expected: PASS. The compiler is the test for the deletions.

- [ ] **Step 6: Commit**

```bash
git add pkg/engine/history.go pkg/harness/turn.go pkg/gui/types.go pkg/gui/service.go pkg/engine/mechanics_engagement.go
git commit -m "refactor(engine): remove the never-set action verdict"
```

---

### Task 3: Fix the `ProposedCheck` comment

**Files:**
- Modify: `pkg/harness/turn.go`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing; a comment only.

- [ ] **Step 1: Rewrite the comment**

Replace the `ProposedCheck` doc comment (around `pkg/harness/turn.go:111-118`) so it describes
reality: the Roll mode builds a `ProposedCheck` and the orchestrator turns it into an advisory
`[PROPOSED CHECK]` directive for the GM. Remove the sentence that references `dismissed_checks`.

- [ ] **Step 2: Confirm the build**

Run: `go build ./pkg/harness/`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/harness/turn.go
git commit -m "docs(harness): describe ProposedCheck without dismissed_checks"
```

---

### Task 4: One `newCheckID`

**Files:**
- Create: `pkg/harness/check_id.go`
- Create: `pkg/harness/check_id_test.go`
- Modify: `pkg/engine/check_resolver.go`
- Modify: `pkg/rules/resolver.go`
- Modify: `pkg/rules/js_engine.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `func NewCheckID() string`.

- [ ] **Step 1: Write the failing test**

```go
package harness

import (
	"strings"
	"testing"
)

func TestNewCheckIDIsUniqueAndPrefixed(t *testing.T) {
	a, b := NewCheckID(), NewCheckID()
	if !strings.HasPrefix(a, "chk_") {
		t.Fatalf("id %q lacks the chk_ prefix", a)
	}
	if a == b {
		t.Fatalf("two ids collided: %q", a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/ -run TestNewCheckID -v`
Expected: FAIL, `undefined: NewCheckID`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/harness/check_id.go`:

```go
package harness

import (
	"strconv"
	"time"
)

// NewCheckID returns a process-unique check identifier. It is the single
// definition shared by the engine's default resolver and the rules engine.
func NewCheckID() string {
	return "chk_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
```

- [ ] **Step 4: Delete the duplicates and update the call sites**

Delete `newCheckID` from `pkg/engine/check_resolver.go:38` and `pkg/rules/resolver.go:121`. Replace
the three calls (`pkg/engine/check_resolver.go:32`, `pkg/rules/resolver.go:53`,
`pkg/rules/js_engine.go:294`) with `harness.NewCheckID()`. `pkg/engine/check_resolver.go` and
`pkg/rules/*` already import `pkg/harness`; add the import where missing.

- [ ] **Step 5: Run the tests**

Run: `go test ./pkg/harness/ ./pkg/engine/ ./pkg/rules/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/harness/check_id.go pkg/harness/check_id_test.go pkg/engine/check_resolver.go pkg/rules/resolver.go pkg/rules/js_engine.go
git commit -m "refactor: share one check-id definition"
```

---

### Task 5: Remove the frontend verdict field

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: any component that reads `turn.verdict` (found in Task 1).

**Interfaces:**
- Consumes: the DTO change from Task 2.
- Produces: nothing.

- [ ] **Step 1: Remove the field**

Delete the `verdict` property from the turn type in `frontend/src/types.ts`, and remove any
renderer that reads it.

- [ ] **Step 2: Typecheck**

Run: `npx tsc --noEmit` (in `frontend/`)
Expected: PASS. `noUnusedLocals` will catch a now-unused import.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/types.ts
git commit -m "refactor(frontend): drop the unused turn verdict"
```

---

### Task 6: Fix the Systems Studio doc

**Files:**
- Modify: `pkg/gui/docs/09-systems-studio.md`

**Interfaces:**
- Consumes: `SystemManifest` (`pkg/core/types.go:12-23`), `MechanicsSpec` (`pkg/core/mechanics.go`),
  the host API (`pkg/rules/js_engine.go:46-174`).
- Produces: nothing; documentation.

- [ ] **Step 1: Read the current article**

Read `pkg/gui/docs/09-systems-studio.md` and note every inaccurate fragment: the `action_modes`
field and the `rollDice(notation)` global are the known ones.

- [ ] **Step 2: Correct the example**

Replace the `system.yaml` example with one that matches `SystemManifest` plus a small declarative
`mechanics` block (a stat and a check convention). Replace `rollDice(notation)` with `roll(notation)`
and list the real globals and hooks: `roll`, `getStat`, `setStat`, `getLocation`, `setLocation`,
`injectGMDirection`, `log`, `grantXP`, and `onAction`/`onTurnBegin`/`onTurnEnd`/`onWorldTick`/
`onCheck`/`onHealthZero`.

- [ ] **Step 3: Lint the docs**

Run: `mise run lint:docs`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/gui/docs/09-systems-studio.md
git commit -m "docs: correct the systems studio manifest and host API"
```

---

### Task 7: Verification

- [ ] **Step 1: Grep for the removed names**

Run:
```bash
rg -n "ActionVerdict|ActionFeasibility|FeasibilityUncertain|Turn\.Verdict|dismissed_checks|rollDice|action_modes" pkg frontend
```
Expected: no hits (except possibly in superseded spec files under `docs/`, which are historical and
must not be edited).

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Commit any residual fixes**

```bash
git add -A
git commit -m "chore: finish mechanics housekeeping"
```
