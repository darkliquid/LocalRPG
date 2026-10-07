# Opposed Rolls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a check name an opponent, roll for both sides, and resolve by comparison.

**Architecture:** `CheckRequest.Opposed` names the opponent's stat; the resolver rolls both sides with the same notation and bonuses; `CheckResult` carries the opponent's roll; a profile may declare a default opposed stat and tie rule.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-opposed-rolls-design.md`
**Depends on:** SYS-1, SYS-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- A check with no `Opposed` is unchanged.
- An unknown opponent contributes 0.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The request and result fields

**Files:**
- Modify: `pkg/harness/turn.go`
- Test: `pkg/harness/turn_test.go` (append)

**Interfaces:**
- Consumes: `RollSummary`.
- Produces: `CheckRequest.Opposed`, `CheckResult.OpposedRoll`, `.OpposedTotal`, `.OpposedActor`.

- [ ] **Step 1: Write the failing test**

```go
func TestCheckRequestCarriesOpposed(t *testing.T) {
	req, err := ParseCheckRequest(`{"actor":"a","check_kind":"do","target":"ogre","opposed":"might"}`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Opposed != "might" || req.Target != "ogre" {
		t.Fatalf("request = %+v", req)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/ -run TestCheckRequestCarriesOpposed -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the fields from the spec §4.1/§4.2.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/ -run TestCheckRequestCarriesOpposed -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/turn.go pkg/harness/turn_test.go
git commit -m "feat(harness): carry an opposed check"
```

---

### Task 2: The profile's opposed default and ties

**Files:**
- Modify: `pkg/core/mechanics.go` (the profile)
- Test: `pkg/core/profile_test.go` (append)

**Interfaces:**
- Consumes: `ResolutionProfile` (SYS-2).
- Produces: `ResolutionProfile.Opposed`, `.Ties`.

- [ ] **Step 1: Write the failing test**

```go
func TestProfileOpposedFields(t *testing.T) {
	p := ResolutionProfile{Opposed: "might", Ties: "opponent"}
	if p.Opposed != "might" || p.Ties != "opponent" {
		t.Fatalf("profile = %+v", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/core/ -run TestProfileOpposedFields -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the two fields and validate `Ties` is `actor`, `opponent`, or empty in `CheckConventions.Validate`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/core/ -run TestProfileOpposedFields -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/core/mechanics.go pkg/core/profile_test.go
git commit -m "feat(core): declare an opposed profile"
```

---

### Task 3: The resolver's opposed path

**Files:**
- Modify: `pkg/rules/resolver.go`, `pkg/engine/check_resolver.go`
- Test: `pkg/rules/resolver_test.go` (append)

**Interfaces:**
- Consumes: Tasks 1-2, SYS-1's `SumBonuses`.
- Produces: both sides rolled and compared.

- [ ] **Step 1: Write the failing tests**

```go
func TestOpposedRollHigherWins(t *testing.T) {
	// The actor's total beats the opponent's -> the better outcome.
}
func TestOpposedTieFollowsTheProfile(t *testing.T) { /* ties: opponent loses */ }
func TestOpposedUnknownOpponentIsFlat(t *testing.T) { /* an unknown target contributes 0 */ }
func TestNonOpposedUnchanged(t *testing.T) { /* no opposed field -> the fixed path */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/rules/ -run TestOpposed -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

When `req.Opposed != ""` (or the profile declares one), roll for the actor and the opponent with the
same notation, sum each side's bonuses, compare, apply the tie rule, and map the win/loss through the
profile or conventions. Populate the opposed result fields.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/rules/ -run TestOpposed -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/rules pkg/engine
git commit -m "feat(rules): resolve an opposed check"
```

---

### Task 4: The tool and the instruction

**Files:**
- Modify: `pkg/harness/turn_tools.go`, `pkg/harness/mechanics_instructions.go`
- Test: `pkg/harness/turn_tools_test.go`, `pkg/harness/mechanics_instructions_test.go` (append)

**Interfaces:**
- Consumes: Task 1.
- Produces: an `opposed` parameter; the instruction mentions opposed profiles.

- [ ] **Step 1: Write the failing tests**

```go
func TestRequestCheckSpecAdvertisesOpposed(t *testing.T) {
	props := requestCheckSpec().Parameters["properties"].(map[string]interface{})
	if _, ok := props["opposed"]; !ok {
		t.Fatal("opposed parameter missing")
	}
}
func TestInstructionMentionsOpposedProfiles(t *testing.T) { /* the profile's opposed stat is listed */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/harness/ -run 'TestRequestCheckSpecAdvertisesOpposed|TestInstructionMentionsOpposed' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the `opposed` parameter (the opponent's stat) to both specs and mention a profile's opposed stat
in the instruction.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/harness/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness
git commit -m "feat(harness): let the GM name an opposed check"
```

---

### Task 5: Presentation

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`, `frontend/src/types.ts`, `frontend/src/components/DiceCheckCard.tsx`
- Test: `frontend/src/components/DiceCheckCard.test.tsx` (append)

**Interfaces:**
- Consumes: the opposed result fields.
- Produces: the contest rendered.

- [ ] **Step 1: Write the failing test**

```tsx
test("shows both sides of an opposed check", () => {
  render(<DiceCheckCard check={{ /* … */ opposed_total: 7, opposed_actor: "Ogre",
    total: 9, outcome: "strong" }} />);
  expect(screen.getByText(/Ogre/)).toBeInTheDocument();
  expect(screen.getByText(/9/)).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- DiceCheckCard`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the opposed fields to `CheckDTO`, map them, and render "You N vs Them M → outcome".

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- DiceCheckCard`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat(frontend): show an opposed check"
```

---

### Task 6: Verification

- [ ] **Step 1: Symmetry property test**

Add a test that swapping the actor and target swaps the outcome.

- [ ] **Step 2: Regression guard**

Add a test that a check with no `Opposed` is unchanged.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- Both sides roll with the same notation and bonuses.
- The higher total wins; a tie follows the profile.
- An unknown opponent is flat.
- A non-opposed check is unchanged.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the non-opposed path"
```
