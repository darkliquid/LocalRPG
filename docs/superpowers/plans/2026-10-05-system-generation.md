# System Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate a complete, verified system from a description, schema-first, as a reviewed draft.

**Architecture:** `pkg/sysgen` runs structured steps (shape, schema, hooks, rules) behind WG-1's generator seam, then verifies the result with SYS-7's harness before offering it; an endpoint and studio flow expose it.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-system-generation-design.md`
**Depends on:** SYS-5, SYS-7.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- A generated system must pass verification before it is offered.
- JavaScript is generated only for behaviour the schema cannot express.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The types and the pipeline

**Files:**
- Create: `pkg/sysgen/sysgen.go`
- Test: `pkg/sysgen/sysgen_test.go`

**Interfaces:**
- Consumes: WG-1's `Generator`, `core.MechanicsSpec`.
- Produces: `Brief`, `System`, `VerifyResult`, `func Generate(ctx, Generator, Brief) (System, error)`.

- [ ] **Step 1: Write the failing test**

```go
package sysgen

import (
	"context"
	"testing"
)

type stubGen struct{}

func (stubGen) GenerateJSON(context.Context, string, string) ([]byte, error) {
	return []byte(`{"name":"Gritty","stats":[{"id":"sanity"}]}`), nil
}

func TestGenerateReturnsASystem(t *testing.T) {
	s, err := Generate(context.Background(), stubGen{}, Brief{Description: "gritty d20"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name == "" || s.Mechanics == nil {
		t.Fatalf("system = %+v", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sysgen/ -run TestGenerateReturnsASystem -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the types and a `Generate` that runs the steps, parses each response (repairing JSON), and
assembles a `System`. Verification is wired in Task 4; for now leave `Verify` zero and tolerate a
stub.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sysgen/ -run TestGenerateReturnsASystem -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/sysgen.go pkg/sysgen/sysgen_test.go
git commit -m "feat(sysgen): add the system generation pipeline"
```

---

### Task 2: Shape and schema

**Files:**
- Create: `pkg/sysgen/steps.go`
- Test: `pkg/sysgen/steps_test.go`

**Interfaces:**
- Consumes: `core.MechanicsSpec`, `core.ResolutionProfile` (SYS-2).
- Produces: `runShape`, `runSchema`.

- [ ] **Step 1: Write the failing test**

```go
func TestSchemaStepParsesMechanics(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"ladder","advancement":false}`,
		`{"stats":[{"id":"sanity"}],"checks":{"notation":"2d6","outcome":["strong","weak","miss"],
		  "profiles":{"pbta":{"ladder":[{"min":10,"outcome":"strong"},{"min":7,"outcome":"weak"},{"min":0,"outcome":"miss"}]}}}}`,
	}}
	s, err := Generate(context.Background(), g, Brief{Description: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mechanics == nil || len(s.Mechanics.Checks.Profiles) != 1 {
		t.Fatalf("mechanics = %+v", s.Mechanics)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sysgen/ -run TestSchemaStep -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Prompt for the shape, then for the schema as `core.MechanicsSpec` JSON; parse into the struct and
validate the profiles with SYS-2's `CheckConventions.Validate`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sysgen/ -run TestSchemaStep -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/steps.go pkg/sysgen/steps_test.go
git commit -m "feat(sysgen): generate the system shape and schema"
```

---

### Task 3: Hooks and rules

**Files:**
- Modify: `pkg/sysgen/steps.go`
- Test: `pkg/sysgen/steps_test.go` (append)

**Interfaces:**
- Consumes: the schema.
- Produces: `runHooks`, `runRules`.

- [ ] **Step 1: Write the failing test**

```go
func TestHooksStepIsSkippedWhenUnneeded(t *testing.T) {
	g := &jsonGen{responses: []string{`{}`, `{"stats":[]}`, `{"hooks":[]}`, `{"rules":"Roll 2d6."}`}}
	s, err := Generate(context.Background(), g, Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(s.Script) != "" {
		t.Fatalf("expected no script, got %q", s.Script)
	}
	if s.RulesPrompt == "" {
		t.Fatal("rules should be generated")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sysgen/ -run TestHooksStep -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

The hooks step is asked to return an empty list when the schema suffices; when it returns hooks,
assemble them into a `mechanics.js` registering the relevant `on*` handlers. The rules step produces
`prompts/rules.md` consistent with the schema (it is given the schema in the prompt).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sysgen/ -run TestHooksStep -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/steps.go pkg/sysgen/steps_test.go
git commit -m "feat(sysgen): generate hooks and rules"
```

---

### Task 5: Mandatory verification

**Files:**
- Modify: `pkg/sysgen/sysgen.go`
- Test: `pkg/sysgen/verify_test.go`

**Interfaces:**
- Consumes: SYS-7's `systemtest.Run`.
- Produces: `VerifyResult`, verification inside `Generate`.

- [ ] **Step 1: Write the failing tests**

```go
func TestVerificationCatchesABrokenScript(t *testing.T) {
	g := &jsonGen{responses: []string{`{}`, `{"stats":[]}`, `{"hooks":[{"raw":"onAction(\"do\", ("}]}`, `{"rules":"x"}`}}
	s, err := Generate(context.Background(), g, Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Verify.OK {
		t.Fatal("a broken script should fail verification")
	}
}
func TestVerificationPassesAGoodSystem(t *testing.T) { /* the corpus-like system passes */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sysgen/ -run TestVerification -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Build a `systemtest.System` from the generated `System` and a generated smoke scenario (roll a check
through the first profile, apply a state change), run it, and record the result in `Verify`. Never
panic on a load error; capture it as a failure.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sysgen/ -run TestVerification -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/sysgen.go pkg/sysgen/verify_test.go
git commit -m "feat(sysgen): verify a generated system before offering it"
```

---

### Task 6: The endpoint and the studio flow

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/gui/types.go`
- Modify: `frontend/src/components/SystemsStudio.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `pkg/gui/sysgen_test.go`

**Interfaces:**
- Consumes: `sysgen.Generate`.
- Produces: `POST /api/system/generate` and a studio flow with a review.

- [ ] **Step 1: Write the failing tests**

```go
func TestGenerateSystemWritesNothing(t *testing.T) { /* the systems dir is unchanged */ }
func TestGenerateSystemReportsVerification(t *testing.T) { /* the result carries Verify */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestGenerateSystem -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the endpoint (dry-run estimate, then steps and a draft with the verify result) and a
"Generate a system" flow in the studio that shows the mechanics (SYS-5's editor), the script, the
rules, and the verification outcome, with save/regenerate.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestGenerateSystem -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat: generate and review a system"
```

---

### Task 7: Verification

- [ ] **Step 1: Regression guard**

Add a test that a generated system with no hooks has an empty script.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- A description yields a complete system.
- The schema is declarative; JS only for escape hatches.
- A broken system fails verification and is not offered.
- Nothing is written until accept.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the no-hooks system path"
```
