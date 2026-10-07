# Base System Catalogue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Offer the reference systems as derivable bases and derive a variant from one by instruction.

**Architecture:** `pkg/sysgen.Derive` seeds SG-1's pipeline with a base's schema/script/rules and validates the variant with the smoke gate plus the base's scenarios; a studio catalogue offers Clone and Derive.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-base-system-catalogue-design.md`
**Depends on:** SYS-6, SG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- The catalogue is `refsystems.List()`; no second source.
- A derive with no instruction errors.
- Conventional Commits, subject under 72 chars.

---

### Task 1: `Derive`

**Files:**
- Create: `pkg/sysgen/derive.go`
- Test: `pkg/sysgen/derive_test.go`

**Interfaces:**
- Consumes: `refsystems.ReferenceSystem`, `Generator`, SG-1's `System`.
- Produces: `func Derive(ctx, Generator, refsystems.ReferenceSystem, instruction string) (System, error)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestDeriveRequiresAnInstruction(t *testing.T) {
	if _, err := Derive(context.Background(), &jsonGen{}, refsystems.ReferenceSystem{ID: "b"}, ""); err == nil {
		t.Fatal("an empty instruction should error")
	}
}
func TestDeriveSeedsTheBase(t *testing.T) {
	g := &jsonGen{prompts: []string{}, responses: []string{`{"mechanics":{"stats":[{"id":"sanity"}]},"rules":"x"}`}}
	base := refsystems.ReferenceSystem{ID: "b", Name: "Base", Script: "onAction(...)"}
	s, err := Derive(context.Background(), g, base, "add sanity")
	if err != nil {
		t.Fatal(err)
	}
	if s.Mechanics == nil {
		t.Fatalf("system = %+v", s)
	}
	if !g.sawPromptContaining("Base") {
		t.Fatal("the base should seed the prompt")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sysgen/ -run TestDerive -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Build a prompt containing the base's manifest, mechanics, script, and rules, plus the instruction;
parse the returned `MechanicsSpec`/script/rules into a `System`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sysgen/ -run TestDerive -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/derive.go pkg/sysgen/derive_test.go
git commit -m "feat(sysgen): derive a system from a base"
```

---

### Task 2: Validation against the base's scenarios

**Files:**
- Modify: `pkg/sysgen/derive.go`
- Test: `pkg/sysgen/derive_test.go` (append)

**Interfaces:**
- Consumes: `Gate` (SG-3), the base's scenarios.
- Produces: `func ValidateDerivation(sys System, baseScenarios []systemtest.Scenario) GateResult`.

- [ ] **Step 1: Write the failing test**

```go
func TestValidateDerivationRunsBaseScenarios(t *testing.T) {
	sys := System{Script: `onAction("do", () => ({ outcome: "strong" }))`, Mechanics: &core.MechanicsSpec{}}
	base := []systemtest.Scenario{{Name: "base", Steps: []systemtest.Step{{Action: "do", Expect: systemtest.Expectations{Outcome: "weak"}}}}}
	got := ValidateDerivation(sys, base)
	if got.OK {
		t.Fatal("a variant that breaks a base scenario should be flagged")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sysgen/ -run TestValidateDerivation -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Run the smoke gate and the base's scenarios, returning the combined result. Unlike SG-1's block, a
derivation's base-scenario failure is a warning the caller shows, not a hard stop.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sysgen/ -run TestValidateDerivation -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/derive.go pkg/sysgen/derive_test.go
git commit -m "feat(sysgen): validate a derivation against the base"
```

---

### Task 3: The endpoint

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/gui/types.go`
- Test: `pkg/gui/derive_test.go`

**Interfaces:**
- Consumes: `Derive`, `refsystems.Get`.
- Produces: `POST /api/system/derive`.

- [ ] **Step 1: Write the failing tests**

```go
func TestDeriveEndpointWritesNothing(t *testing.T) { /* the systems dir is unchanged */ }
func TestDeriveEndpointUnknownBase(t *testing.T) { /* 404 for an unknown base id */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestDeriveEndpoint -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Resolve the base via `refsystems.Get`, run `Derive`, and stream/return the draft with its validation
result. An unknown base is a `404`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestDeriveEndpoint -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): derive a system from a base"
```

---

### Task 4: The studio catalogue

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `frontend/src/components/BaseSystemCatalogue.test.tsx`

**Interfaces:**
- Consumes: `/api/reference-systems` (SYS-6), the derive endpoint (Task 3).
- Produces: a catalogue with Clone and Derive per base.

- [ ] **Step 1: Write the failing test**

```tsx
test("offers clone and derive per base", () => {
  render(<BaseSystemCatalogue bases={[{ id: "d20_dc", name: "d20 + DC" }]}
    onClone={() => {}} onDerive={() => {}} />);
  expect(screen.getByRole("button", { name: /clone/i })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /derive/i })).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- BaseSystemCatalogue`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Render the catalogue from the fetched reference systems, with Clone (loads the base into the draft)
and Derive (opens an instruction input and calls the endpoint, then the review).

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- BaseSystemCatalogue`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): offer bases to clone or derive"
```

---

### Task 5: Verification

- [ ] **Step 1: Regression guard**

Add a test that cloning a base yields it unchanged.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- The catalogue is the reference systems.
- Derive seeds the base and validates with the smoke gate plus base scenarios.
- Clone is unchanged behaviour.
- An unknown base is a 404; no instruction errors.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the clone-a-base path"
```
