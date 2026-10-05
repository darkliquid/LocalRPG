# Named Resolution Profiles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a system declare named resolution profiles (ladder, DC, success pool, position/effect) and the GM select one per check.

**Architecture:** `CheckConventions.Profiles` holds `ResolutionProfile`s; a shared `ResolveProfile` maps a roll to an outcome; the schema and engine resolvers select a profile from the request; the tool advertises the profile and the UI shows the outcome and stakes.

**Tech Stack:** Go standard library; `github.com/darkliquid/roll` for the dice.

**Spec:** `docs/superpowers/specs/2026-10-05-resolution-profiles-design.md`
**Depends on:** SYS-1 (`docs/superpowers/plans/2026-10-05-check-modifiers.md`).

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- A check with no profile resolves exactly as before.
- Ladder steps resolve highest-first; ties are impossible after a defensive sort.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The schema and its validation

**Files:**
- Modify: `pkg/core/mechanics.go`
- Create: `pkg/core/profile.go`
- Test: `pkg/core/profile_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `ResolutionProfile`, `LadderStep`, `SuccessOutcome`, `CheckConventions.Profiles`, `func (c CheckConventions) Validate() []string`.

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestValidateProfiles(t *testing.T) {
	ok := CheckConventions{Profiles: map[string]ResolutionProfile{
		"pbta":  {Ladder: []LadderStep{{Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}}},
		"d20":   {Notation: "1d20", DC: 15},
		"pool":  {SuccessOn: ">=8", Outcomes: []SuccessOutcome{{Min: 1, Max: -1, Outcome: "strong"}}},
		"blades": {Position: []string{"risky"}, Effect: []string{"standard"}, Ladder: []LadderStep{{Min: 7, Outcome: "weak"}}},
	}}
	if p := ok.Validate(); len(p) != 0 {
		t.Fatalf("valid profiles reported %v", p)
	}
	bad := CheckConventions{Profiles: map[string]ResolutionProfile{
		"empty-ladder": {DC: 0, Ladder: nil},
		"bad-pool":     {SuccessOn: ">=8"},
	}}
	if len(bad.Validate()) == 0 {
		t.Fatal("bad profiles should be rejected")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/core/ -run TestValidateProfiles -v`
Expected: FAIL, `undefined: ResolutionProfile`.

- [ ] **Step 3: Write minimal implementation**

Add the types to `pkg/core/mechanics.go` (or `profile.go`) and the map to `CheckConventions`, as in
the spec §4.1. Implement `Validate`:

```go
// Validate reports problems with the declared profiles.
func (c CheckConventions) Validate() []string {
	var problems []string
	for name, p := range c.Profiles {
		switch {
		case p.DC != 0:
			// DC form: nothing else required.
		case len(p.Outcomes) > 0:
			if p.SuccessOn == "" {
				problems = append(problems, "checks.profiles."+name+": outcomes without success_on")
			}
		case len(p.Ladder) > 0:
			// Ladder form.
		default:
			problems = append(problems, "checks.profiles."+name+": needs a dc, a ladder, or a pool")
		}
		for _, step := range p.Ladder {
			if step.Outcome == "" {
				problems = append(problems, "checks.profiles."+name+": a ladder step has no outcome")
			}
		}
	}
	return problems
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/core/ -run TestValidateProfiles -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/core/mechanics.go pkg/core/profile.go pkg/core/profile_test.go
git commit -m "feat(core): declare named resolution profiles"
```

---

### Task 2: `ResolveProfile`

**Files:**
- Create: `pkg/rules/profile.go`
- Test: `pkg/rules/profile_test.go`

**Interfaces:**
- Consumes: `core.ResolutionProfile` (Task 1).
- Produces: `func ResolveProfile(p core.ResolutionProfile, total, successes int) (string, bool)`.

- [ ] **Step 1: Write the failing test**

```go
package rules

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestResolveProfile(t *testing.T) {
	pbta := core.ResolutionProfile{Ladder: []core.LadderStep{
		{Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}, {Min: 0, Outcome: "miss"}}}
	cases := []struct {
		total, succ int
		want        string
	}{
		{12, 0, "strong"}, {10, 0, "strong"}, {9, 0, "weak"}, {7, 0, "weak"}, {6, 0, "miss"},
	}
	for _, c := range cases {
		got, ok := ResolveProfile(pbta, c.total, c.succ)
		if !ok || got != c.want {
			t.Errorf("total %d: got %q ok %v, want %q", c.total, got, ok, c.want)
		}
	}
	d20 := core.ResolutionProfile{DC: 15}
	if got, _ := ResolveProfile(d20, 15, 0); got != "success" {
		t.Errorf("dc met: got %q", got)
	}
	if got, _ := ResolveProfile(d20, 14, 0); got != "fail" {
		t.Errorf("dc missed: got %q", got)
	}
	pool := core.ResolutionProfile{SuccessOn: ">=8", Outcomes: []core.SuccessOutcome{
		{Min: 3, Max: -1, Outcome: "strong"}, {Min: 1, Max: 2, Outcome: "weak"}, {Min: 0, Max: 0, Outcome: "miss"}}}
	if got, _ := ResolveProfile(pool, 0, 3); got != "strong" {
		t.Errorf("pool 3: got %q", got)
	}
	if _, ok := ResolveProfile(core.ResolutionProfile{}, 5, 0); ok {
		t.Error("an empty profile must not decide")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/rules/ -run TestResolveProfile -v`
Expected: FAIL, `undefined: ResolveProfile`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/rules/profile.go`:

```go
package rules

import (
	"sort"

	"github.com/darkliquid/localrpg/pkg/core"
)

// ResolveProfile maps a roll total and success count through a profile to an
// outcome. It returns ("", false) when the profile cannot decide.
func ResolveProfile(p core.ResolutionProfile, total, successes int) (string, bool) {
	if len(p.Ladder) > 0 {
		steps := append([]core.LadderStep(nil), p.Ladder...)
		sort.SliceStable(steps, func(i, j int) bool { return steps[i].Min > steps[j].Min })
		for _, step := range steps {
			if total >= step.Min {
				return step.Outcome, true
			}
		}
		return steps[len(steps)-1].Outcome, true
	}
	if p.DC != 0 {
		if total >= p.DC {
			return "success", true
		}
		return "fail", true
	}
	if p.SuccessOn != "" && len(p.Outcomes) > 0 {
		for _, o := range p.Outcomes {
			if successes >= o.Min && (o.Max < 0 || successes <= o.Max) {
				return o.Outcome, true
			}
		}
	}
	return "", false
}
```

Note: the pool's `successes` count is computed by the caller from the dice (`RollResult.Successes`,
`pkg/rules/dice.go:10-19`), which the library already provides.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/rules/ -run TestResolveProfile -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/rules/profile.go pkg/rules/profile_test.go
git commit -m "feat(rules): resolve a roll through a named profile"
```

---

### Task 3: The request and result fields

**Files:**
- Modify: `pkg/harness/turn.go`
- Test: `pkg/harness/turn_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `CheckRequest.Profile/Position/Effect`, `CheckResult.Profile/Position/Effect/Successes`.

- [ ] **Step 1: Write the failing test**

```go
func TestCheckRequestCarriesProfile(t *testing.T) {
	req, err := ParseCheckRequest(`{"actor":"x","check_kind":"do","profile":"pbta","position":"risky"}`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Profile != "pbta" || req.Position != "risky" {
		t.Fatalf("decoded %+v", req)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/ -run TestCheckRequestCarriesProfile -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the fields from the spec §4.2 to `CheckRequest` and `CheckResult`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/ -run TestCheckRequestCarriesProfile -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/turn.go pkg/harness/turn_test.go
git commit -m "feat(harness): carry a resolution profile on a check"
```

---

### Task 4: `SchemaResolver` selects and applies a profile

**Files:**
- Modify: `pkg/rules/resolver.go:23-59`
- Test: `pkg/rules/resolver_test.go` (append)

**Interfaces:**
- Consumes: `ResolveProfile` (Task 2), the request fields (Task 3).
- Produces: `SchemaResolver` sets `Profile`, `Position`, `Effect`, `Successes`, and the profile outcome.

- [ ] **Step 1: Write the failing test**

```go
func TestSchemaResolverUsesProfile(t *testing.T) {
	conventions := core.CheckConventions{Notation: "2d6", Profiles: map[string]core.ResolutionProfile{
		"pbta": {Ladder: []core.LadderStep{{Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}, {Min: 0, Outcome: "miss"}}},
	}}
	r := SchemaResolver{bridge: testBridgeFor(testActorWithState(nil)), conventions: conventions}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{Profile: "pbta"}, testActorWithState(nil))
	if err != nil {
		t.Fatal(err)
	}
	if res.Profile != "pbta" || res.Outcome == "" {
		t.Fatalf("result = %+v", res)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/rules/ -run TestSchemaResolverUsesProfile -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In `Resolve`, after the roll and the SYS-1 bonus sum:

```go
	profile := req.Profile
	if profile == "" {
		profile = "default"
	}
	if p, ok := r.conventions.Profiles[profile]; ok {
		if outcome, decided := ResolveProfile(p, res.Total, roll.Successes); decided {
			res.Outcome = outcome
			res.Profile = profile
		}
		res.Position = clampTo(p.Position, req.Position)
		res.Effect = clampTo(p.Effect, req.Effect)
		res.Successes = roll.Successes
	}
```

When no profile decides, keep the existing `outcomeFor` path. `clampTo` returns the request value
when it is in the allowed list, else the first allowed value (or empty).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/rules/ -v`
Expected: PASS, including the existing resolver tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/rules/resolver.go pkg/rules/resolver_test.go
git commit -m "feat(rules): apply a resolution profile in the schema resolver"
```

---

### Task 5: The engine default resolver matches

**Files:**
- Modify: `pkg/engine/check_resolver.go`
- Test: `pkg/engine/check_resolver_test.go` (append)

**Interfaces:**
- Consumes: `rules.ResolveProfile`.
- Produces: the default resolver honours a profile.

- [ ] **Step 1: Write the failing test**

```go
func TestDefaultResolverUsesProfile(t *testing.T) {
	o := &TurnOrchestrator{mechanics: &core.MechanicsSpec{Checks: core.CheckConventions{
		Profiles: map[string]core.ResolutionProfile{"pbta": {Ladder: []core.LadderStep{{Min: 0, Outcome: "miss"}}}},
	}}}
	res, err := defaultCheckResolver{mechanics: o.mechanics}.Resolve(context.Background(),
		harness.CheckRequest{Notation: "2d6", Profile: "pbta"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != "miss" {
		t.Fatalf("outcome = %q", res.Outcome)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestDefaultResolverUsesProfile -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Give `defaultCheckResolver` access to the mechanics spec (it is already built from the orchestrator's
`mechanics`) and apply the same profile logic as Task 4, sharing a helper if practical.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/check_resolver.go pkg/engine/check_resolver_test.go
git commit -m "feat(engine): apply a resolution profile in the default resolver"
```

---

### Task 6: The tool and the instruction

**Files:**
- Modify: `pkg/harness/turn_tools.go`, `pkg/harness/mechanics_instructions.go`
- Test: `pkg/harness/turn_tools_test.go`, `pkg/harness/mechanics_instructions_test.go` (append)

**Interfaces:**
- Consumes: the request fields (Task 3), `MechanicsSpec.Checks.Profiles`.
- Produces: `profile`/`position`/`effect` parameters; a profile list in the instruction.

- [ ] **Step 1: Write the failing tests**

```go
func TestRequestCheckSpecAdvertisesProfile(t *testing.T) {
	props := requestCheckSpec().Parameters["properties"].(map[string]interface{})
	for _, key := range []string{"profile", "position", "effect"} {
		if _, ok := props[key]; !ok {
			t.Fatalf("missing %s parameter", key)
		}
	}
}

func TestMechanicsInstructionListsProfiles(t *testing.T) {
	spec := &core.MechanicsSpec{Checks: core.CheckConventions{Profiles: map[string]core.ResolutionProfile{
		"pbta": {Label: "PbtA ladder"}}}}
	got := FormatMechanicsInstructions(spec, "auto", nil)
	if !strings.Contains(got, "PbtA ladder") {
		t.Fatalf("instruction did not list the profile: %s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/harness/ -run 'TestRequestCheckSpecAdvertisesProfile|TestMechanicsInstructionListsProfiles' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the three string parameters to `requestCheckSpec` and `proposeCheckSpec`. In
`FormatMechanicsInstructions`, after the difficulties, list each profile's name (and label) so the
model knows what it may name.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/harness/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/turn_tools.go pkg/harness/mechanics_instructions.go pkg/harness/turn_tools_test.go pkg/harness/mechanics_instructions_test.go
git commit -m "feat(harness): let the GM name a resolution profile"
```

---

### Task 7: Show the profile outcome

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`, `frontend/src/types.ts`, `frontend/src/components/DiceCheckCard.tsx`
- Test: `frontend/src/components/DiceCheckCard.test.tsx` (append)

**Interfaces:**
- Consumes: `CheckResult.Profile/Position/Effect/Successes`.
- Produces: the fields on the check DTO and a rendered line.

- [ ] **Step 1: Write the failing test**

```tsx
test("shows the profile and stakes", () => {
  render(<DiceCheckCard check={{ /* … */ profile: "blades", position: "risky",
    effect: "limited", outcome: "weak", successes: 2 }} />);
  expect(screen.getByText(/risky/)).toBeInTheDocument();
  expect(screen.getByText(/limited/)).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- DiceCheckCard`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the fields to `CheckDTO` and map them in the check DTO builder; add the TS type and render a
compact "position/effect" line and the success count where present.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- DiceCheckCard` and `npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go frontend/src/types.ts frontend/src/components/DiceCheckCard.tsx frontend/src/components/DiceCheckCard.test.tsx
git commit -m "feat(frontend): show the resolution profile and stakes"
```

---

### Task 8: Verification

- [ ] **Step 1: Regression guard**

Add a test asserting that a check with no profile resolves to the same outcome as before this change.

- [ ] **Step 2: Property test**

Add a test that a ladder covering all totals maps every total to exactly one outcome.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard resolution profiles against the conventions path"
```
