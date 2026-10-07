# Check Modifiers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a check draw on a governing stat, a skill rating, and named situational modifiers, and show the breakdown.

**Architecture:** `CheckRequest` gains `Skill` and `Modifiers`; a shared summation helper in `pkg/rules` is used by both `SchemaResolver` and the engine's default resolver; `CheckResult` carries an `Applied` breakdown; the `request_check` tool advertises the new parameters and the mechanics instruction lists declared skills.

**Tech Stack:** Go standard library; React 19 + Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-10-05-check-modifiers-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- A request with only `Stat` set must resolve to exactly the pre-change total.
- A missing stat or skill contributes 0 and is not an error.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The request and result vocabulary

**Files:**
- Modify: `pkg/harness/turn.go`
- Test: `pkg/harness/turn_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `CheckModifier`, `CheckRequest.Skill`, `CheckRequest.Modifiers`, `AppliedModifier`, `CheckResult.Applied`.

- [ ] **Step 1: Write the failing test**

```go
func TestCheckRequestCarriesSkillAndModifiers(t *testing.T) {
	raw := `{"actor":"x","check_kind":"do","skill":"stealth",
	         "modifiers":[{"source":"high ground","value":1}]}`
	req, err := ParseCheckRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if req.Skill != "stealth" || len(req.Modifiers) != 1 || req.Modifiers[0].Value != 1 {
		t.Fatalf("decoded %+v", req)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/ -run TestCheckRequestCarries -v`
Expected: FAIL, `req.Skill undefined`.

- [ ] **Step 3: Write minimal implementation**

In `pkg/harness/turn.go`, add to `CheckRequest`:

```go
	Skill     string          `json:"skill,omitempty"`
	Modifiers []CheckModifier `json:"modifiers,omitempty"`
```

and the supporting types:

```go
// CheckModifier is one named adjustment to a check's total.
type CheckModifier struct {
	Source string `json:"source"`
	Value  int    `json:"value"`
	Reason string `json:"reason,omitempty"`
}

// AppliedModifier is one contribution to a resolved check total.
type AppliedModifier struct {
	Source string `json:"source"`
	Value  int    `json:"value"`
}
```

Add `Applied []AppliedModifier` to `CheckResult`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/ -run TestCheckRequestCarries -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/turn.go pkg/harness/turn_test.go
git commit -m "feat(harness): carry skill and modifiers on a check request"
```

---

### Task 2: One summation helper

**Files:**
- Create: `pkg/rules/modifiers.go`
- Test: `pkg/rules/modifiers_test.go`

**Interfaces:**
- Consumes: `harness.CheckRequest`, `harness.AppliedModifier`.
- Produces: `func SumBonuses(value func(name string) (int, bool), req harness.CheckRequest) (int, []harness.AppliedModifier)`.

- [ ] **Step 1: Write the failing test**

```go
package rules

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestSumBonuses(t *testing.T) {
	values := map[string]int{"might": 2, "stealth": 3}
	lookup := func(n string) (int, bool) { v, ok := values[n]; return v, ok }
	req := harness.CheckRequest{
		Stat:  "might",
		Skill: "stealth",
		Modifiers: []harness.CheckModifier{
			{Source: "high ground", Value: 1},
			{Source: "wounded", Value: -2},
		},
	}
	total, applied := SumBonuses(lookup, req)
	if total != 4 {
		t.Fatalf("total = %d, want 4", total)
	}
	if len(applied) != 4 {
		t.Fatalf("applied = %+v, want 4 entries", applied)
	}
}

func TestSumBonusesMissingValuesAddZero(t *testing.T) {
	req := harness.CheckRequest{Stat: "luck", Skill: "nope"}
	total, applied := SumBonuses(func(string) (int, bool) { return 0, false }, req)
	if total != 0 || len(applied) != 0 {
		t.Fatalf("total = %d applied = %+v, want 0 and empty", total, applied)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/rules/ -run TestSumBonuses -v`
Expected: FAIL, `undefined: SumBonuses`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/rules/modifiers.go`:

```go
package rules

import "github.com/darkliquid/localrpg/pkg/harness"

// SumBonuses totals a check's stat, skill, and named modifiers. value resolves a
// declared stat or skill; a miss contributes nothing. The returned slice names
// every contribution so a result can be displayed.
func SumBonuses(value func(name string) (int, bool), req harness.CheckRequest) (int, []harness.AppliedModifier) {
	total := 0
	applied := make([]harness.AppliedModifier, 0, 2+len(req.Modifiers))
	for _, name := range []string{req.Stat, req.Skill} {
		if name == "" {
			continue
		}
		if v, ok := value(name); ok && v != 0 {
			total += v
			applied = append(applied, harness.AppliedModifier{Source: name, Value: v})
		}
	}
	for _, m := range req.Modifiers {
		if m.Value == 0 {
			continue
		}
		total += m.Value
		applied = append(applied, harness.AppliedModifier{Source: m.Source, Value: m.Value})
	}
	return total, applied
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/rules/ -run TestSumBonuses -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/rules/modifiers.go pkg/rules/modifiers_test.go
git commit -m "feat(rules): sum a check's stat, skill, and modifiers"
```

---

### Task 3: `SchemaResolver` uses the helper

**Files:**
- Modify: `pkg/rules/resolver.go:23-78`
- Test: `pkg/rules/resolver_test.go` (append)

**Interfaces:**
- Consumes: `SumBonuses` (Task 2).
- Produces: `CheckResult.Applied` populated.

- [ ] **Step 1: Write the failing test**

```go
func TestSchemaResolverAppliesSkillAndModifiers(t *testing.T) {
	actor := testActorWithState(map[string]interface{}{"might": 2, "stealth": 3})
	r := SchemaResolver{bridge: testBridgeFor(actor), conventions: core.CheckConventions{Notation: "2d6"}}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Stat: "might", Skill: "stealth",
		Modifiers: []harness.CheckModifier{{Source: "high ground", Value: 1}},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	// 2d6 (2..12) + 2 + 3 + 1; assert the bonus is present regardless of the dice.
	if res.Total < 8 {
		t.Fatalf("total %d did not include the +6 bonus", res.Total)
	}
	if len(res.Applied) != 3 {
		t.Fatalf("applied = %+v, want 3", res.Applied)
	}
}
```

Use the existing test helpers in `pkg/rules` for an actor and bridge; add them if missing.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/rules/ -run TestSchemaResolverApplies -v`
Expected: FAIL (the skill and modifier are ignored).

- [ ] **Step 3: Write minimal implementation**

In `pkg/rules/resolver.go`, replace the single-stat bonus with the helper:

```go
	bonus, applied := SumBonuses(func(name string) (int, bool) {
		return statValue(r.bridge, actor, name)
	}, req)
	total := roll.Total + bonus
```

and set `Applied: applied` on the returned `CheckResult`. Rename `statValue` to `stateValue` and
keep its body; update the one caller.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/rules/ -v`
Expected: PASS, including the existing resolver tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/rules/resolver.go pkg/rules/resolver_test.go
git commit -m "feat(rules): apply skills and modifiers in the schema resolver"
```

---

### Task 4: The engine's default resolver matches

**Files:**
- Modify: `pkg/engine/check_resolver.go`
- Test: `pkg/engine/check_resolver_test.go` (append)

**Interfaces:**
- Consumes: `rules.SumBonuses`.
- Produces: the default resolver's `CheckResult.Applied`.

- [ ] **Step 1: Write the failing test**

```go
func TestDefaultResolverAppliesModifiers(t *testing.T) {
	res, err := defaultCheckResolver{}.Resolve(context.Background(), harness.CheckRequest{
		Notation: "2d6", Modifiers: []harness.CheckModifier{{Source: "wounded", Value: -3}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total > 9 {
		t.Fatalf("total %d did not include the -3 modifier", res.Total)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestDefaultResolverAppliesModifiers -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In `pkg/engine/check_resolver.go`, after the roll, add:

```go
	bonus, applied := rules.SumBonuses(func(name string) (int, bool) {
		return engineStateValue(o.bridge, actor, name)
	}, req)
	res.Total = roll.Total + bonus
	res.Applied = applied
```

Reuse whatever state reader the file already has for the actor; if none exists, read
`actor.State.Get(name)` directly.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/check_resolver.go pkg/engine/check_resolver_test.go
git commit -m "feat(engine): apply skills and modifiers in the default resolver"
```

---

### Task 5: The tool and the instruction

**Files:**
- Modify: `pkg/harness/turn_tools.go:34-49`
- Modify: `pkg/harness/mechanics_instructions.go`
- Test: `pkg/harness/turn_tools_test.go`, `pkg/harness/mechanics_instructions_test.go` (append)

**Interfaces:**
- Consumes: Task 1.
- Produces: two new tool parameters; a skill line in the instruction.

- [ ] **Step 1: Write the failing tests**

```go
func TestRequestCheckSpecAdvertisesSkillAndModifiers(t *testing.T) {
	spec := requestCheckSpec()
	props := spec.Parameters["properties"].(map[string]interface{})
	if _, ok := props["skill"]; !ok {
		t.Fatal("skill parameter missing")
	}
	if _, ok := props["modifiers"]; !ok {
		t.Fatal("modifiers parameter missing")
	}
}

func TestMechanicsInstructionListsSkills(t *testing.T) {
	spec := &core.MechanicsSpec{Skills: []core.SkillSpec{{ID: "stealth", Label: "Stealth"}}}
	got := FormatMechanicsInstructions(spec, "auto", nil)
	if !strings.Contains(got, "Stealth") {
		t.Fatalf("instruction did not list the skill: %s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/harness/ -run 'TestRequestCheckSpecAdvertises|TestMechanicsInstructionListsSkills' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In `pkg/harness/turn_tools.go`, add to `requestCheckSpec`'s properties:

```go
	"skill": map[string]interface{}{
		"type":        "string",
		"description": "A declared skill whose rating is added, alongside stat.",
	},
	"modifiers": map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"source": map[string]interface{}{"type": "string"},
				"value":  map[string]interface{}{"type": "integer"},
				"reason": map[string]interface{}{"type": "string"},
			},
			"required": []string{"source", "value"},
		},
		"description": "Situational bonuses and penalties, each named.",
	},
```

Do the same in `proposeCheckSpec`.

In `pkg/harness/mechanics_instructions.go`, after the stats line, add a skills line mirroring it:

```go
	if len(spec.Skills) > 0 {
		parts := make([]string, 0, len(spec.Skills))
		for _, skill := range spec.Skills {
			label := skill.Label
			if label == "" {
				label = skill.ID
			}
			parts = append(parts, label)
		}
		sb.WriteString("Skills: " + strings.Join(parts, ", ") + ".\n")
	}
```

and one sentence in the `default` branch: "Name the skill as well as the stat when a check tests a
trained ability, and list situational modifiers."

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/harness/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/turn_tools.go pkg/harness/mechanics_instructions.go pkg/harness/turn_tools_test.go pkg/harness/mechanics_instructions_test.go
git commit -m "feat(harness): ask the GM for a skill and named modifiers"
```

---

### Task 6: Show the breakdown

**Files:**
- Modify: `pkg/gui/types.go` (check DTO)
- Modify: `pkg/gui/service.go` (map `Applied`)
- Modify: `frontend/src/types.ts`, `frontend/src/components/DiceCheckCard.tsx`
- Test: `frontend/src/components/DiceCheckCard.test.tsx` (append)

**Interfaces:**
- Consumes: `harness.CheckResult.Applied`.
- Produces: `CheckDTO.Applied []AppliedModifierDTO`; a rendered breakdown.

- [ ] **Step 1: Write the failing test**

```tsx
test("renders the modifier breakdown", () => {
  render(<DiceCheckCard check={{ /* ... */ applied: [
    { source: "Stealth", value: 3 }, { source: "wounded", value: -2 }] }} />);
  expect(screen.getByText(/Stealth/)).toBeInTheDocument();
  expect(screen.getByText(/-2/)).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- DiceCheckCard`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `AppliedModifierDTO{Source, Value}` and `CheckDTO.Applied []AppliedModifierDTO` in
`pkg/gui/types.go`, map it in the check DTO builder in `pkg/gui/service.go`, add the matching TS
type, and render a compact list in `DiceCheckCard.tsx`: the dice total, then each `source (+n/−n)`,
then the final total.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- DiceCheckCard` and `npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go frontend/src/types.ts frontend/src/components/DiceCheckCard.tsx frontend/src/components/DiceCheckCard.test.tsx
git commit -m "feat(frontend): show a check's modifier breakdown"
```

---

### Task 7: Verification

- [ ] **Step 1: Regression guard**

Add a test asserting that a request with only `Stat` set resolves to the same total as before this
change (compute the dice, assert `Total == dice + stat`), so the change cannot silently alter
existing behaviour.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "test: guard check totals against the pre-modifier behaviour"
```
