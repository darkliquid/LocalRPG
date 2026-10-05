# System Smoke Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One smoke scenario every system must pass, gating generated saves and warning hand-authored ones.

**Architecture:** `systemtest.SmokeScenario` defines the minimum; `sysgen.Gate` runs it; the save path enforces it for generated systems and warns for hand-authored ones.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-system-smoke-gate-design.md`
**Depends on:** SG-1, SYS-7.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- A system with no mechanics passes trivially.
- The gate makes no model call.
- Conventional Commits, subject under 72 chars.

---

### Task 1: `SmokeScenario`

**Files:**
- Create: `pkg/systemtest/smoke.go`
- Test: `pkg/systemtest/smoke_test.go`

**Interfaces:**
- Consumes: `core.MechanicsSpec`, `Scenario`.
- Produces: `func SmokeScenario(mechanics *core.MechanicsSpec) Scenario`.

- [ ] **Step 1: Write the failing tests**

```go
func TestSmokeScenarioNamesTheProfile(t *testing.T) {
	spec := &core.MechanicsSpec{Checks: core.CheckConventions{
		Profiles: map[string]core.ResolutionProfile{"pbta": {Ladder: []core.LadderStep{{Min: 0, Outcome: "miss"}}}}}}
	s := SmokeScenario(spec)
	if len(s.Steps) == 0 || s.Steps[0].Expect.Outcome == "" {
		t.Fatalf("scenario = %+v", s)
	}
}

func TestSmokeScenarioEmptyForSchemaAgnostic(t *testing.T) {
	if s := SmokeScenario(nil); len(s.Steps) != 0 {
		t.Fatalf("scenario = %+v", s)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/systemtest/ -run TestSmokeScenario -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Generate the scenario from the mechanics: a `do` step naming the first profile (or the conventions
outcome), and a state step when a stat is declared. Return an empty scenario for a nil spec.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/systemtest/ -run TestSmokeScenario -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/systemtest/smoke.go pkg/systemtest/smoke_test.go
git commit -m "feat(systemtest): define the smoke scenario"
```

---

### Task 2: `Gate`

**Files:**
- Create: `pkg/sysgen/gate.go`
- Test: `pkg/sysgen/gate_test.go`

**Interfaces:**
- Consumes: `systemtest.Run`, `SmokeScenario`.
- Produces: `GateResult`, `func Gate(sys System) GateResult`.

- [ ] **Step 1: Write the failing tests**

```go
func TestGatePassesAGoodSystem(t *testing.T) {
	sys := System{Script: `onAction("do", () => ({ outcome: "miss" }))`, Mechanics: &core.MechanicsSpec{}}
	if got := Gate(sys); !got.OK {
		t.Fatalf("gate = %+v", got)
	}
}
func TestGateFailsABrokenScript(t *testing.T) {
	sys := System{Script: `onAction("do", (`}
	if got := Gate(sys); got.OK {
		t.Fatal("a broken script should fail the gate")
	}
}
func TestGateTriviallyPassesSchemaAgnostic(t *testing.T) {
	if got := Gate(System{}); !got.OK {
		t.Fatalf("gate = %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sysgen/ -run TestGate -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Build a `systemtest.System` from the `System`, run `SmokeScenario`, and return the failures and
whether a script is present. Never panic on a load error.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sysgen/ -run TestGate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/gate.go pkg/sysgen/gate_test.go
git commit -m "feat(sysgen): gate a system on the smoke test"
```

---

### Task 3: The save path warns, strict blocks

**Files:**
- Modify: `pkg/gui/service.go` (`SaveSystem`), `pkg/gui/types.go`
- Test: `pkg/gui/system_gate_test.go`

**Interfaces:**
- Consumes: `sysgen.Gate` (Task 2).
- Produces: a gate result on the save response; a `strict` flag on the request.

- [ ] **Step 1: Write the failing tests**

```go
func TestSaveSystemWarnsOnFailedSmoke(t *testing.T) { /* a broken script yields a warning but saves */ }
func TestSaveSystemStrictBlocks(t *testing.T) { /* strict refuses a broken script */ }
func TestSaveSystemGoodIsSilent(t *testing.T) { /* no warning for a good system */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestSaveSystem -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In `SaveSystem`, run `Gate` on the incoming system; append the failures to `Warnings`, and when
`strict` is set and the gate fails, return an error before writing.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestSaveSystem -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): gate system saves on the smoke test"
```

---

### Task 4: The generated accept blocks

**Files:**
- Modify: `pkg/gui/sysgen.go`
- Test: `pkg/gui/sysgen_test.go` (append)

**Interfaces:**
- Consumes: `sysgen.Gate` (Task 2).
- Produces: the accept path refuses a failing generated system.

- [ ] **Step 1: Write the failing test**

```go
func TestGeneratedSystemAcceptBlocksOnFailure(t *testing.T) { /* a failing draft cannot be accepted */ }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestGeneratedSystemAccept -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In the generated-system accept path, run `Gate` and refuse to save when it fails, returning the
failure detail.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestGeneratedSystemAccept -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): never accept a failing generated system"
```

---

### Task 5: The corpus and the studio

**Files:**
- Test: `pkg/refsystems/smoke_test.go`
- Modify: `frontend/src/components/SystemsStudio.tsx`, `frontend/src/components/SystemGenerateDialog.tsx`
- Test: `frontend/src/components/SystemsStudio.test.tsx` (append)

**Interfaces:**
- Consumes: `SmokeScenario`, the save response's gate result.
- Produces: the smoke test run over the corpus; the studio showing a warning or block.

- [ ] **Step 1: Write the corpus test**

```go
func TestCorpusPassesTheSmokeScenario(t *testing.T) {
	for _, s := range List() {
		sys := systemtest.System{ID: s.ID, Script: s.Script, Mechanics: s.Mechanics}
		if f := systemtest.Run(sys, systemtest.SmokeScenario(s.Mechanics)); len(f) > 0 {
			t.Errorf("%s failed the smoke scenario: %+v", s.ID, f)
		}
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./pkg/refsystems/ -run TestCorpusPassesTheSmokeScenario -v`
Expected: PASS (fix a corpus system if it fails, since the smoke bar is the minimum).

- [ ] **Step 3: Show the gate in the studio**

Render the save response's gate warnings as a dismissible notice, and a failed generated system's
failure inline with a regenerate action.

- [ ] **Step 4: Typecheck and test**

Run: `npm run test -- SystemsStudio` and `npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/refsystems frontend/src
git commit -m "feat: run the smoke gate over the corpus and in the studio"
```

---

### Task 6: Verification

- [ ] **Step 1: Regression guard**

Add a test that a schema-agnostic system saves silently.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- The smoke scenario is one definition, generated from the mechanics.
- A generated system cannot be saved broken.
- A hand-authored system warns, and strict blocks.
- The corpus passes the smoke scenario.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the schema-agnostic save"
```
