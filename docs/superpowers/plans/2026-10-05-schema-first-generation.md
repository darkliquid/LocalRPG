# Schema-First Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate a system's schema by selecting a known-good template and filling parameters, so the result is valid by construction, with JavaScript only for a closed list of escape hatches.

**Architecture:** `pkg/sysgen` gains a template library with `Build`, a choose step, a fill step, and a closed escape-hatch list; SG-1's schema step is replaced.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-schema-first-generation-design.md`
**Depends on:** SG-1, SYS-6.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- `Build` returns an error rather than an invalid spec.
- JavaScript is generated only for a listed escape hatch.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The template library

**Files:**
- Create: `pkg/sysgen/templates.go`
- Test: `pkg/sysgen/templates_test.go`

**Interfaces:**
- Consumes: `core.MechanicsSpec`, `core.LadderStep`, `core.SuccessOutcome`.
- Produces: `Template`, `Params`, `func (t Template) Build(Params) (*core.MechanicsSpec, error)`, `func Templates() []Template`.

- [ ] **Step 1: Write the failing tests**

```go
func TestBuildLadderTemplate(t *testing.T) {
	spec, err := Template{Resolution: "ladder", Health: "single"}.Build(Params{
		Stats: []core.StatSpec{{ID: "might"}},
		Ladder: []core.LadderStep{{Min: 10, Outcome: "strong"}, {Min: 0, Outcome: "miss"}},
		HealthStat: "might",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Checks.Profiles) != 1 {
		t.Fatalf("spec = %+v", spec)
	}
}

func TestBuildRejectsBadParams(t *testing.T) {
	if _, err := (Template{Resolution: "ladder"}).Build(Params{}); err == nil {
		t.Fatal("an empty ladder should error")
	}
	if _, err := (Template{Resolution: "dc"}).Build(Params{}); err == nil {
		t.Fatal("a zero DC should error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sysgen/ -run TestBuild -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the template types and `Build`, assembling a `MechanicsSpec` from the template's fixed structure
and the parameters, validating the parameters and returning an error otherwise.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sysgen/ -run TestBuild -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/templates.go pkg/sysgen/templates_test.go
git commit -m "feat(sysgen): add the schema template library"
```

---

### Task 2: The choose step

**Files:**
- Modify: `pkg/sysgen/steps.go`
- Test: `pkg/sysgen/steps_test.go` (append)

**Interfaces:**
- Consumes: `Template` (Task 1), the `Generator`.
- Produces: `func chooseTemplate(ctx, Generator, brief Brief) (Template, string, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestChooseTemplateMapsADescription(t *testing.T) {
	g := &jsonGen{responses: []string{`{"resolution":"ladder","health":"single","advancement":"none","reason":"PbtA"}`}}
	tpl, reason, err := chooseTemplate(context.Background(), g, Brief{Description: "PbtA with three stats"})
	if err != nil {
		t.Fatal(err)
	}
	if tpl.Resolution != "ladder" || reason == "" {
		t.Fatalf("template %+v reason %q", tpl, reason)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sysgen/ -run TestChooseTemplate -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Prompt the model with the available template dimensions (a closed set), parse the choice, validate it
against `Templates()`, and return the template plus its reason.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sysgen/ -run TestChooseTemplate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/steps.go pkg/sysgen/steps_test.go
git commit -m "feat(sysgen): choose a schema template"
```

---

### Task 3: The fill step and the escape hatches

**Files:**
- Modify: `pkg/sysgen/steps.go`
- Test: `pkg/sysgen/steps_test.go` (append)

**Interfaces:**
- Consumes: `Params` (Task 1).
- Produces: `func fillParams(ctx, Generator, Template, Brief) (Params, error)`, `escapeHatches`, `func requestedHatches(brief Brief) []string`.

- [ ] **Step 1: Write the failing tests**

```go
func TestFillParamsProducesABuiltSpec(t *testing.T) {
	g := &jsonGen{responses: []string{`{"stats":[{"id":"might"}],"ladder":[{"min":7,"outcome":"weak"}],"health_stat":"might"}`}}
	tpl := Template{Resolution: "ladder", Health: "single"}
	params, err := fillParams(context.Background(), g, tpl, Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tpl.Build(params); err != nil {
		t.Fatal(err)
	}
}

func TestRequestedHatches(t *testing.T) {
	if got := requestedHatches(Brief{Description: "a spend economy"}); len(got) == 0 {
		t.Fatal("a spend economy should request the resource_spend hatch")
	}
	if got := requestedHatches(Brief{Description: "plain d20"}); len(got) != 0 {
		t.Fatal("a plain d20 should request no hatch")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sysgen/ -run 'TestFillParams|TestRequestedHatches' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Prompt for the parameters of the chosen template and parse them. Recognise escape hatches from the
closed list against the description.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sysgen/ -run 'TestFillParams|TestRequestedHatches' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/steps.go pkg/sysgen/steps_test.go
git commit -m "feat(sysgen): fill template parameters and detect escape hatches"
```

---

### Task 4: Wire it into SG-1's pipeline

**Files:**
- Modify: `pkg/sysgen/sysgen.go`
- Test: `pkg/sysgen/sysgen_test.go` (append)

**Interfaces:**
- Consumes: Tasks 1-3.
- Produces: `Generate` uses choose + fill + `Build` instead of free-form schema.

- [ ] **Step 1: Write the failing test**

```go
func TestGenerateUsesTemplates(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"ladder","health":"none","advancement":"none"}`,
		`{"stats":[{"id":"might"}],"ladder":[{"min":7,"outcome":"weak"}]}`,
		`{"hooks":[]}`,
		`{"rules":"Roll 2d6."}`,
	}}
	s, err := Generate(context.Background(), g, Brief{Description: "PbtA"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mechanics == nil || len(s.Mechanics.Checks.Profiles) != 1 {
		t.Fatalf("mechanics = %+v", s.Mechanics)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sysgen/ -run TestGenerateUsesTemplates -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Replace the schema step with choose + fill + `Build`, and request JavaScript only for a detected
hatch. Record a note in the draft when the description exceeds the templates and hatches.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sysgen/ -run TestGenerateUsesTemplates -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sysgen/sysgen.go pkg/sysgen/sysgen_test.go
git commit -m "feat(sysgen): generate the schema from templates"
```

---

### Task 5: Verification

- [ ] **Step 1: Corpus parity guard**

Add a test that a generated PbtA system's profile resolves through SYS-7's harness like the corpus's.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Every template builds a valid spec; bad parameters error.
- The choose step maps a description to a template.
- JS is generated only for a listed hatch.
- A generated system verifies.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard template-generated systems"
```
