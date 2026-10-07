# System Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Propose additive changes to an existing system as an accept/reject diff, and explain a system's mechanics in prose.

**Architecture:** `pkg/sysgen.Propose` reads the system and returns capped additions; `Apply` appends them, refusing duplicates; the smoke gate validates the result; `Explain` writes a plain description.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-system-enhancement-design.md`
**Depends on:** SG-2, SG-3.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- Additions only; never modify an existing declaration.
- Applying no proposals leaves the system unchanged.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The proposal and `Propose`

**Files:**
- Create: `pkg/sysgen/enhance.go`
- Test: `pkg/sysgen/enhance_test.go`

**Interfaces:**
- Consumes: `Generator`, `System`, SG-2's `Template`.
- Produces: `Proposal`, `func Propose(ctx, Generator, System, instruction string, kinds []string) ([]Proposal, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestProposeReturnsAdditions(t *testing.T) {
	g := &jsonGen{responses: []string{`{"proposals":[
	  {"kind":"stat","title":"Sanity","stat":{"id":"sanity"}},
	  {"kind":"skill","title":"Occult","skill":{"id":"occult","stat":"sanity"}}]}`}}
	got, err := Propose(context.Background(), g, System{ID: "s", Mechanics: &core.MechanicsSpec{}}, "add sanity", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Kind != "stat" {
		t.Fatalf("proposals = %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sysgen/ -run TestPropose -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the type and `Propose`: build a prompt from the system's manifest, mechanics, script, and rules;
parse the proposals; cap the list.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sysgen/ -run TestPropose -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/enhance.go pkg/sysgen/enhance_test.go
git commit -m "feat(sysgen): propose system additions"
```

---

### Task 2: Applying additions

**Files:**
- Modify: `pkg/sysgen/enhance.go`
- Test: `pkg/sysgen/enhance_test.go` (append)

**Interfaces:**
- Consumes: `Proposal`, `core.MechanicsSpec`.
- Produces: `func ApplyAdditions(spec *core.MechanicsSpec, accepted []Proposal) (*core.MechanicsSpec, error)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestApplyAdditionsAppends(t *testing.T) {
	spec := &core.MechanicsSpec{Stats: []core.StatSpec{{ID: "might"}}}
	got, err := ApplyAdditions(spec, []Proposal{{Kind: "stat", Stat: &core.StatSpec{ID: "sanity"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Stats) != 2 || got.Stats[0].ID != "might" {
		t.Fatalf("stats = %+v", got.Stats)
	}
}
func TestApplyAdditionsRefusesDuplicates(t *testing.T) {
	spec := &core.MechanicsSpec{Stats: []core.StatSpec{{ID: "might"}}}
	if _, err := ApplyAdditions(spec, []Proposal{{Kind: "stat", Stat: &core.StatSpec{ID: "might"}}}); err == nil {
		t.Fatal("a duplicate id should be refused")
	}
}
func TestApplyAdditionsNoProposalsUnchanged(t *testing.T) { /* identical spec */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sysgen/ -run TestApplyAdditions -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Return a copy of the spec with each accepted addition appended to its list; refuse a duplicate id in
any list.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sysgen/ -run TestApplyAdditions -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/enhance.go pkg/sysgen/enhance_test.go
git commit -m "feat(sysgen): apply system additions"
```

---

### Task 3: Validation with the smoke gate

**Files:**
- Modify: `pkg/sysgen/enhance.go`
- Test: `pkg/sysgen/enhance_test.go` (append)

**Interfaces:**
- Consumes: `Gate` (SG-3).
- Produces: `func ValidateEnhancement(sys System, accepted []Proposal) GateResult`.

- [ ] **Step 1: Write the failing test**

```go
func TestValidateEnhancementCatchesABrokenAddition(t *testing.T) {
	sys := System{Script: `onAction("do", () => ({ outcome: "miss" }))`, Mechanics: &core.MechanicsSpec{}}
	// A skill naming an undeclared stat should fail the gate's smoke scenario.
	got := ValidateEnhancement(sys, []Proposal{{Kind: "skill", Skill: &core.SkillSpec{ID: "occult", Stat: "missing"}}})
	if got.OK {
		t.Fatal("a dangling reference should fail validation")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sysgen/ -run TestValidateEnhancement -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Apply the additions to a copy, build a `System`, and run `Gate`; return its result.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sysgen/ -run TestValidateEnhancement -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/enhance.go pkg/sysgen/enhance_test.go
git commit -m "feat(sysgen): validate an enhancement with the smoke gate"
```

---

### Task 4: `Explain`

**Files:**
- Create: `pkg/sysgen/explain.go`
- Test: `pkg/sysgen/explain_test.go`

**Interfaces:**
- Consumes: `Generator`, `System`.
- Produces: `func Explain(ctx, Generator, System) (string, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestExplainReturnsText(t *testing.T) {
	g := &jsonGen{responses: []string{`{"explanation":"Roll 2d6 and add Edge."}`}}
	got, err := Explain(context.Background(), g, System{ID: "s", Mechanics: &core.MechanicsSpec{}})
	if err != nil || got == "" {
		t.Fatalf("explanation %q err %v", got, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sysgen/ -run TestExplain -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Prompt the model with the mechanics and rules, parse the explanation, and return it.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sysgen/ -run TestExplain -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/explain.go pkg/sysgen/explain_test.go
git commit -m "feat(sysgen): explain a system's mechanics"
```

---

### Task 5: The endpoints and the studio

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/gui/types.go`
- Modify: `frontend/src/components/SystemsStudio.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `pkg/gui/system_enhance_test.go`

**Interfaces:**
- Consumes: `Propose`, `ApplyAdditions`, `ValidateEnhancement`, `Explain`.
- Produces: enhance/apply/explain endpoints and studio actions.

- [ ] **Step 1: Write the failing tests**

```go
func TestEnhanceSystemWritesNothing(t *testing.T) { /* the systems dir is unchanged */ }
func TestEnhanceApplyWritesOnlyAccepted(t *testing.T) { /* a rejected proposal is absent */ }
func TestExplainSystemEndpoint(t *testing.T) { /* returns text */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run 'TestEnhanceSystem|TestExplainSystem' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the endpoints and a studio diff (like WG-3's) plus an Explain action that shows the text.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run 'TestEnhanceSystem|TestExplainSystem' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat: enhance and explain a system from the studio"
```

---

### Task 6: Verification

- [ ] **Step 1: No-op guard**

Add a test that applying no proposals leaves the system unchanged.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Proposals are additive and capped.
- Duplicates and dangling references are caught.
- Only accepted proposals are written.
- Explain returns a plain description.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the no-proposal system path"
```
