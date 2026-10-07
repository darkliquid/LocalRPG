# Renegotiation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a player counter-propose the stakes or difficulty of a pending check and have the GM re-adjudicate, recorded on the turn.

**Architecture:** A `CounterProposal` and an `Adjudication`; a short adjudication generation; the endpoint updates the pending check in place and appends a `Negotiation`; the Roll card gains an Argue form.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-renegotiation-design.md`
**Depends on:** IR-1, IR-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- A hold leaves the pending check unchanged.
- A resolved check refuses a counter.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The types

**Files:**
- Modify: `pkg/harness/turn.go`, `pkg/engine/history.go`
- Test: `pkg/harness/turn_test.go` (append)

**Interfaces:**
- Consumes: `PendingCheck`.
- Produces: `CounterProposal`, `Adjudication`, `Negotiation`, `Turn.Negotiations`.

- [ ] **Step 1: Write the failing test**

```go
func TestNegotiationTypes(t *testing.T) {
	n := Negotiation{Counter: CounterProposal{Approach: "lift the bar"}, Ruling: Adjudication{Ruling: "accept"}}
	if n.Counter.Approach == "" || n.Ruling.Ruling != "accept" {
		t.Fatalf("negotiation = %+v", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/ -run TestNegotiationTypes -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the types from the spec §4.1/§4.2/§4.4 and `Turn.Negotiations`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/ -run TestNegotiationTypes -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness pkg/engine
git commit -m "feat(harness): add the negotiation types"
```

---

### Task 2: The adjudication generation

**Files:**
- Create: `pkg/engine/negotiate.go`
- Test: `pkg/engine/negotiate_test.go`

**Interfaces:**
- Consumes: the pending check, the counter, the provider.
- Produces: `func (o *TurnOrchestrator) Adjudicate(ctx, pending harness.PendingCheck, counter CounterProposal) (Adjudication, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestAdjudicateAcceptsAReasonableCounter(t *testing.T) {
	o := newTestOrchestratorWithStubProvider(t, `{"ruling":"accept","stakes":"the bar holds","difficulty":"risky"}`)
	got, err := o.Adjudicate(context.Background(), testPending(), CounterProposal{Approach: "lift the bar"})
	if err != nil || got.Ruling != "accept" {
		t.Fatalf("adjudication %+v err %v", got, err)
	}
}
func TestAdjudicateHoldsLeavesTerms(t *testing.T) { /* a hold returns the original terms */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine/ -run TestAdjudicate -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Build a short prompt from the pending check and the counter, ask for a structured adjudication,
parse it (repairing JSON), and return it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -run TestAdjudicate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine
git commit -m "feat(engine): adjudicate a counter-proposal"
```

---

### Task 3: The endpoint and the pending update

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/gui/types.go`
- Test: `pkg/gui/renegotiate_test.go`

**Interfaces:**
- Consumes: `Adjudicate` (Task 2).
- Produces: `POST /api/game/{id}/turn/{n}/renegotiate` and an in-place pending update.

- [ ] **Step 1: Write the failing tests**

```go
func TestRenegotiateUpdatesPending(t *testing.T) { /* accept updates the terms */ }
func TestRenegotiateHoldKeepsTerms(t *testing.T) { /* hold leaves the check unchanged */ }
func TestRenegotiateRefusesResolved(t *testing.T) { /* a resolved check is a 409 */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestRenegotiate -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Load the turn, validate the pending ref, adjudicate, and on accept/adjust update the
`PendingCheck.Request` (stakes, difficulty, profile, notation) and re-save the turn; append the
`Negotiation`; return the adjudication.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestRenegotiate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): renegotiate a pending check"
```

---

### Task 4: The Argue form

**Files:**
- Modify: `frontend/src/components/PendingCheckCard.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `frontend/src/components/PendingCheckCard.test.tsx` (append)

**Interfaces:**
- Consumes: the endpoint (Task 3).
- Produces: an Argue form showing the ruling.

- [ ] **Step 1: Write the failing tests**

```tsx
test("opens the argue form", () => { /* approach, stakes, and difficulty fields appear */ });
test("shows an accept ruling and the agreed terms", () => { /* the card updates */ });
test("shows a hold reason", () => { /* the reason is displayed */ });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test -- PendingCheckCard`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the form, call the endpoint, show the ruling and reason, and re-render the card with the agreed
terms on accept/adjust.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test -- PendingCheckCard`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): argue a pending check"
```

---

### Task 5: Verification

- [ ] **Step 1: Regression guard**

Add a test that a turn with no counter is unchanged.

- [ ] **Step 2: Cap guard**

Add a test that counters per check are capped.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- A counter updates the pending check on accept/adjust.
- A hold leaves it unchanged.
- The negotiation is recorded and shown.
- A resolved check refuses a counter.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the no-counter path"
```
