# System Test Harness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A deterministic scenario runner for systems, exposed as a CLI command and a studio action, that the reference corpus uses as a regression suite.

**Architecture:** `pkg/systemtest` defines scenarios, a seeded `GameHostAPI` test bridge, and a runner that loads a system into a `JSEngine` and asserts outcomes, totals, and state. A CLI command and an endpoint expose it.

**Tech Stack:** Go standard library; goja via `pkg/rules`; `gopkg.in/yaml.v3`.

**Spec:** `docs/superpowers/specs/2026-10-05-system-test-harness-design.md`
**Depends on:** SYS-5, SYS-6.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- The runner must use the same `SchemaResolver` the engine uses; it must not reimplement resolution.
- The same scenario run twice must produce identical failures.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The scenario types

**Files:**
- Create: `pkg/systemtest/scenario.go`
- Test: `pkg/systemtest/scenario_test.go`

**Interfaces:**
- Consumes: `gopkg.in/yaml.v3`.
- Produces: `Scenario`, `SetupSpec`, `SetupEntity`, `Step`, `Expectations`, `Range`, `LoadScenario([]byte) (Scenario, error)`.

- [ ] **Step 1: Write the failing test**

```go
package systemtest

import "testing"

func TestLoadScenario(t *testing.T) {
	in := []byte("name: a test\nseed: 7\nsteps:\n  - action: do\n    expect:\n      outcome: weak\n")
	s, err := LoadScenario(in)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "a test" || s.Seed != 7 || len(s.Steps) != 1 || s.Steps[0].Expect.Outcome != "weak" {
		t.Fatalf("scenario = %+v", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/systemtest/ -run TestLoadScenario -v`
Expected: FAIL, `undefined: LoadScenario`.

- [ ] **Step 3: Write minimal implementation**

Add the types from the spec §4.1 and `LoadScenario` (yaml.Unmarshal plus a non-empty-steps check).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/systemtest/ -run TestLoadScenario -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/systemtest/scenario.go pkg/systemtest/scenario_test.go
git commit -m "feat(systemtest): add the scenario format"
```

---

### Task 2: The deterministic bridge

**Files:**
- Create: `pkg/systemtest/bridge.go`
- Test: `pkg/systemtest/bridge_test.go`

**Interfaces:**
- Consumes: `rules.GameHostAPI`, `rules.RollResult`, `entity.Entity`, `core.MechanicsSpec`.
- Produces: `type Bridge struct`, `NewBridge(mechanics *core.MechanicsSpec, seed int64, player SetupEntity) *Bridge`, implementing `rules.GameHostAPI`.

- [ ] **Step 1: Write the failing test**

```go
func TestBridgeRollIsDeterministic(t *testing.T) {
	b := NewBridge(nil, 42, SetupEntity{})
	a1, err := b.Roll("2d6")
	if err != nil {
		t.Fatal(err)
	}
	b2 := NewBridge(nil, 42, SetupEntity{})
	a2, _ := b2.Roll("2d6")
	if a1.Total != a2.Total {
		t.Fatalf("same seed produced %d and %d", a1.Total, a2.Total)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/systemtest/ -run TestBridgeRollIsDeterministic -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement `Bridge` with a seeded `*rand.Rand`, an in-memory `*entity.Entity` for the player, and the
declared mechanics. `Roll` parses the notation's dice and sums seeded rolls (reuse the roll
library's parser where it accepts an injected source; otherwise parse `NdM±k` directly for the
harness and document the supported subset). `GetStat`/`SetStat` read and write the entity's
`state`. `ListStats`/`ListSkills`/`CheckConventions` return the declared schema. The remaining
methods are in-memory.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/systemtest/ -run TestBridgeRollIsDeterministic -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/systemtest/bridge.go pkg/systemtest/bridge_test.go
git commit -m "feat(systemtest): add a deterministic host bridge"
```

---

### Task 3: The runner

**Files:**
- Create: `pkg/systemtest/run.go`
- Test: `pkg/systemtest/run_test.go`

**Interfaces:**
- Consumes: `Bridge` (Task 2), `rules.NewJSEngine`, `rules.SchemaResolver`.
- Produces: `type System struct { ID, Script string; Mechanics *core.MechanicsSpec }`, `type Failure struct { Scenario string; Step int; Detail string }`, `func Run(system System, scenario Scenario) []Failure`, `func RunAll(system System, scenarios []Scenario) []Failure`.

- [ ] **Step 1: Write the failing test**

```go
func TestRunPassesAndFails(t *testing.T) {
	sys := System{ID: "t", Script: `onAction("do", () => ({ outcome: "weak", message: "past" }))`,
		Mechanics: &core.MechanicsSpec{}}
	ok := Scenario{Name: "ok", Steps: []Step{{Action: "do", Expect: Expectations{Outcome: "weak", MessageContains: "past"}}}}
	if f := Run(sys, ok); len(f) != 0 {
		t.Fatalf("expected pass, got %+v", f)
	}
	bad := Scenario{Name: "bad", Steps: []Step{{Action: "do", Expect: Expectations{Outcome: "strong"}}}}
	if f := Run(sys, bad); len(f) == 0 {
		t.Fatal("expected a failure")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/systemtest/ -run TestRunPassesAndFails -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

`Run` builds a `Bridge` from the scenario's seed and setup, loads the system into a `JSEngine`, and
for each step calls `ExecuteAction(step.Action, {"action": step.Input, "player": "player"})`,
resolving a requested check through `SchemaResolver` (not a reimplementation), then comparing the
result and state to the expectations. `RunAll` concatenates per-scenario failures.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/systemtest/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/systemtest/run.go pkg/systemtest/run_test.go
git commit -m "feat(systemtest): run a scenario against a system"
```

---

### Task 5: The CLI command

**Files:**
- Create: `cmd/localrpg/test_system.go`
- Modify: `cmd/localrpg/main.go` (register the subcommand under `debug`)
- Test: `cmd/localrpg/test_system_test.go`

**Interfaces:**
- Consumes: `systemtest.RunAll`, `refsystems.List`/`Get`.
- Produces: `localrpg debug test-system <id> [--reference] [--scenario name]`.

- [ ] **Step 1: Write the failing test**

```go
func TestTestSystemCLIReportsFailures(t *testing.T) {
	// Run the command against a fixture system with a failing scenario and assert
	// a non-zero exit and a printed failure.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/localrpg/ -run TestTestSystemCLI -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Load the system (from `--reference` via `refsystems`, else from the systems directory), read
`tests/*.yaml`, run them, print each failure, and exit non-zero on any failure. Support
`--scenario` to run one.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/localrpg/ -run TestTestSystemCLI -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/localrpg/test_system.go cmd/localrpg/main.go cmd/localrpg/test_system_test.go
git commit -m "feat(cli): add debug test-system"
```

---

### Task 6: The endpoint and the studio action

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/types.go`, `pkg/gui/service.go`
- Modify: `frontend/src/components/SystemsStudio.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `pkg/gui/system_test_test.go`

**Interfaces:**
- Consumes: `systemtest.RunAll` (Task 3).
- Produces: `POST /api/system/test` → `SystemTestResponseDTO{Failures []SystemTestFailureDTO}`.

- [ ] **Step 1: Write the failing test**

```go
func TestSystemTestEndpoint(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.TestSystem(context.Background(), SystemTestRequestDTO{
		System: SystemTestSystemDTO{ID: "t", Script: `onAction("do", () => ({ outcome: "weak" }))`},
		Scenarios: []systemtest.Scenario{{Name: "ok", Steps: []systemtest.Step{{Action: "do", Expect: systemtest.Expectations{Outcome: "weak"}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Failures) != 0 {
		t.Fatalf("failures = %+v", resp.Failures)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestSystemTestEndpoint -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Service.TestSystem`, mount `POST /api/system/test`, add a "Run tests" button to the Systems
Studio that posts the draft and renders failures, and mirror the types in the frontend.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestSystemTestEndpoint -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat: run system scenarios from the studio"
```

---

### Task 7: Corpus scenarios and verification

**Files:**
- Create: `pkg/refsystems/systems/*/tests/*.yaml`
- Test: `pkg/refsystems/scenarios_test.go`

**Interfaces:**
- Consumes: `refsystems.List`, `systemtest.RunAll`.
- Produces: a scenario per corpus system, run in `mise run test`.

- [ ] **Step 1: Write the scenarios**

Add at least one scenario per system (`narrative_2d6`, `d20_dc`, `dice_pool`) asserting an outcome
and a state change where the script makes one.

- [ ] **Step 2: Write the test**

```go
func TestCorpusScenariosPass(t *testing.T) {
	for _, s := range List() {
		for _, sc := range LoadScenarios(s.ID) {
			if f := systemtest.RunAll(systemFor(s), []systemtest.Scenario{sc}); len(f) > 0 {
				t.Errorf("%s/%s: %+v", s.ID, sc.Name, f)
			}
		}
	}
}
```

- [ ] **Step 3: Run it**

Run: `go test ./pkg/refsystems/ -run TestCorpusScenariosPass -v`
Expected: PASS.

- [ ] **Step 4: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/refsystems
git commit -m "test(refsystems): ship and run scenarios for every corpus system"
```
