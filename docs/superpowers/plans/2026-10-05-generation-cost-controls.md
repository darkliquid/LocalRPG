# Generation Cost Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Estimate, confirm, and cap the cost of world generation, and fall back to the deterministic oracle when no provider is configured.

**Architecture:** `worldgen.EstimatePlan` predicts calls and cost; endpoints accept a dry run; the pipeline enforces a call budget; an oracle path produces a template world offline; actual usage is recorded.

**Tech Stack:** Go standard library; the pricing ledger.

**Spec:** `docs/superpowers/specs/2026-10-05-generation-cost-controls-design.md`
**Depends on:** WG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- A dry run makes no model call.
- The cap is per generation and configurable.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The estimate

**Files:**
- Create: `pkg/worldgen/estimate.go`
- Test: `pkg/worldgen/estimate_test.go`

**Interfaces:**
- Consumes: `Brief`, `Counts`, the pricing table.
- Produces: `Estimate`, `func EstimatePlan(kind string, brief Brief, chunks int, prices pricing.Table) Estimate`.

- [ ] **Step 1: Write the failing test**

```go
func TestEstimatePlan(t *testing.T) {
	if got := EstimatePlan("world", Brief{Counts: Counts{Locations: 2, Factions: 2, Characters: 3}}, 0, nil); got.Calls != 4 {
		t.Fatalf("world calls = %d, want 4", got.Calls)
	}
	if got := EstimatePlan("entities", Brief{Counts: Counts{Characters: 25}}, 0, nil); got.Calls < 2 {
		t.Fatalf("a large batch should span calls: %d", got.Calls)
	}
	if got := EstimatePlan("ingest", Brief{}, 10, nil); got.Calls < 10 {
		t.Fatalf("ingest calls = %d", got.Calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/worldgen/ -run TestEstimatePlan -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Compute the calls per kind (fixed steps plus batches), and fill `CostMicros` from the pricing table
when the provider is priced.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/worldgen/ -run TestEstimatePlan -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/worldgen/estimate.go pkg/worldgen/estimate_test.go
git commit -m "feat(worldgen): estimate a generation's cost"
```

---

### Task 2: The dry run and the cap

**Files:**
- Modify: `pkg/gui/worldgen.go`, `pkg/gui/types.go`, `pkg/config/types.go`
- Modify: `pkg/worldgen/worldgen.go` (a call counter)
- Test: `pkg/gui/worldgen_cost_test.go`

**Interfaces:**
- Consumes: `EstimatePlan` (Task 1).
- Produces: `GenerationConfig{MaxCalls int}`, a `dry_run` request flag, and a per-generation budget.

- [ ] **Step 1: Write the failing tests**

```go
func TestDryRunMakesNoCall(t *testing.T) { /* the stub generator is never called */ }
func TestCapAborts(t *testing.T) { /* exceeding MaxCalls errors and discards the draft */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run 'TestDryRun|TestCap' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Generation.MaxCalls` to the config and an accessor. Thread a call counter through
`worldgen.Generate` that errors past the budget. Handle `dry_run` by returning the estimate before
any call.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run 'TestDryRun|TestCap' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config pkg/worldgen pkg/gui
git commit -m "feat: dry-run and cap world generation"
```

---

### Task 3: The oracle fallback

**Files:**
- Modify: `pkg/worldgen/worldgen.go`
- Test: `pkg/worldgen/oracle_test.go`

**Interfaces:**
- Consumes: the narrative-oracle provider.
- Produces: an oracle `Generator` that produces a template world with zero calls.

- [ ] **Step 1: Write the failing test**

```go
func TestOracleGeneratorProducesATemplate(t *testing.T) {
	g := NewOracleGenerator()
	d, err := Generate(context.Background(), g, Brief{Premise: "a drowned kingdom", Counts: Counts{Characters: 2}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.World.Name == "" || len(d.Entities) == 0 {
		t.Fatalf("draft = %+v", d)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/worldgen/ -run TestOracleGenerator -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement a `Generator` backed by the deterministic oracle that fills the same structured shapes from
templates, and have the GUI select it when no provider is configured.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/worldgen/ -run TestOracleGenerator -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/worldgen pkg/gui
git commit -m "feat(worldgen): generate offline with the oracle"
```

---

### Task 4: Usage reporting and the studio estimate

**Files:**
- Modify: `pkg/gui/worldgen.go` (record usage), `pkg/gui/types.go`
- Modify: `frontend/src/components/WorldGenerateDialog.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `frontend/src/components/WorldGenerateDialog.test.tsx` (append)

**Interfaces:**
- Consumes: the estimate (Task 1), the ledger.
- Produces: actual usage recorded under `generator`, and an estimate shown before Generate.

- [ ] **Step 1: Write the failing tests**

```go
func TestGenerationRecordsUsage(t *testing.T) { /* the ledger gains a generator entry */ }
```
```tsx
test("shows the estimate before generating", () => { /* the dry run's calls are rendered */ });
test("requires confirmation for a large estimate", () => { /* a second click is needed */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestGenerationRecordsUsage -v` and `npm run test -- WorldGenerateDialog`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Record the calls and cost in the ledger after generation. In the dialog, dry-run first, show the
estimate, and gate a large estimate behind a confirmation.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestGenerationRecordsUsage -v` and `npm run test -- WorldGenerateDialog`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat: show the generation estimate and record usage"
```

---

### Task 5: Verification

- [ ] **Step 1: Regression guard**

Add a test that a small generation under the cap and threshold behaves as before.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- A dry run makes no call and returns an estimate.
- The cap aborts a runaway.
- No provider falls back to the oracle.
- Actual usage is recorded and the estimate is shown first.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the small-generation path"
```
