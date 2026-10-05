# Malformed Response Retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Re-ask the model with a specific repair instruction when a reply is malformed, bounded, before aborting the turn.

**Architecture:** A classifier distinguishes malformed from cut and provider errors; a `repairInstruction` names the problem; the generation loop retries up to a cap, recording each attempt in the failure taxonomy.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-malformed-response-retry-design.md`
**Depends on:** RB-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- A valid reply is never retried.
- A cut reply goes to completion recovery, not the repair.
- The cap is small and configurable.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The classifier

**Files:**
- Create: `pkg/engine/repair.go`
- Test: `pkg/engine/repair_test.go`

**Interfaces:**
- Consumes: the stream result, RB-1's report.
- Produces: `type ReplyProblem int` with `ProblemNone`, `ProblemMalformed`, `ProblemCut`, `ProblemProvider`; `func classifyReply(res streamResult, report turnstream.RepairReport) ReplyProblem`.

- [ ] **Step 1: Write the failing test**

```go
func TestClassifyReply(t *testing.T) {
	if classifyReply(streamResult{text: "A valid reply."}, turnstream.RepairReport{}) != ProblemNone {
		t.Fatal("a valid reply is not a problem")
	}
	if classifyReply(streamResult{text: ""}, turnstream.RepairReport{}) != ProblemMalformed {
		t.Fatal("an empty reply is malformed")
	}
	if classifyReply(streamResult{text: "Half a sent", finishReason: "length"}, turnstream.RepairReport{}) != ProblemCut {
		t.Fatal("a length finish is a cut")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestClassifyReply -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Classify: no usable text and no valid record → malformed; a length finish or an incomplete sentence →
cut; a provider error is not passed here (the fallback chain handles it); otherwise none.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run TestClassifyReply -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/repair.go pkg/engine/repair_test.go
git commit -m "feat(engine): classify a malformed reply"
```

---

### Task 2: `repairInstruction`

**Files:**
- Modify: `pkg/engine/repair.go`
- Test: `pkg/engine/repair_test.go` (append)

**Interfaces:**
- Consumes: the problem and a detail.
- Produces: `func repairInstruction(p ReplyProblem, detail string) string`.

- [ ] **Step 1: Write the failing tests**

```go
func TestRepairInstructionIsSpecific(t *testing.T) {
	if !strings.Contains(repairInstruction(ProblemMalformed, "no roll"), "@roll") {
		t.Fatal("a missing roll should name the record")
	}
	if !strings.Contains(repairInstruction(ProblemMalformed, ""), "empty") {
		t.Fatal("a generic malformed reply should say so")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestRepairInstruction -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Build a short instruction from the detail, with variants for a missing roll, unparseable tool
arguments, and a generic malformed reply.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run TestRepairInstruction -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/repair.go pkg/engine/repair_test.go
git commit -m "feat(engine): build a specific repair instruction"
```

---

### Task 3: The retry in the generation loop

**Files:**
- Modify: `pkg/engine/orchestrator.go` (the generation loop and the post-stream check)
- Test: `pkg/engine/repair_test.go` (append)

**Interfaces:**
- Consumes: the classifier and the instruction (Tasks 1-2).
- Produces: a bounded retry that replaces a malformed reply.

- [ ] **Step 1: Write the failing tests**

```go
func TestEmptyReplyIsRetriedOnce(t *testing.T) {
	// The stub provider returns empty then a valid reply; the turn uses the second.
}
func TestMalformedTwiceAborts(t *testing.T) {
	// Two malformed replies abort, recording two attempts.
}
func TestValidReplyIsNotRetried(t *testing.T) {
	// Exactly one generation call.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine/ -run 'TestEmptyReplyIsRetried|TestMalformedTwice|TestValidReplyIsNotRetried' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

After the stream and RB-1's repair, classify; on `ProblemMalformed`, append the repair instruction and
re-run the generation, up to the cap; discard the malformed reply. On `ProblemCut`, hand to the
existing completion recovery.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -run 'TestEmptyReply|TestMalformedTwice|TestValidReply' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/repair_test.go
git commit -m "feat(engine): retry a malformed reply with a repair instruction"
```

---

### Task 4: The cap, the taxonomy, and the config

**Files:**
- Modify: `pkg/config/types.go`, `pkg/harness/failure.go`, `pkg/engine/orchestrator.go`
- Test: `pkg/engine/repair_test.go` (append), `pkg/config/types_test.go` (append)

**Interfaces:**
- Consumes: `harness.Attempt`, `FailureParseError`.
- Produces: `Config.CompletionMaxRepairAttempts()` (default 2); an attempt per retry.

- [ ] **Step 1: Write the failing tests**

```go
func TestMaxRepairAttemptsDefault(t *testing.T) {
	if DefaultConfig().CompletionMaxRepairAttempts() != 2 {
		t.Fatal("the default cap should be 2")
	}
}
func TestRepairRecordsAttempts(t *testing.T) {
	// A malformed-then-valid turn records one parse_error attempt.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/config/ -run TestMaxRepairAttempts -v` and `go test ./pkg/engine/ -run TestRepairRecordsAttempts -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the config accessor and record an `Attempt{Kind: FailureParseError, Detail: …}` per retry, so the
failure surface shows it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/config/ -run TestMaxRepairAttempts -v` and `go test ./pkg/engine/ -run TestRepairRecordsAttempts -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config pkg/harness pkg/engine
git commit -m "feat: cap and record malformed-reply retries"
```

---

### Task 5: Verification

- [ ] **Step 1: Regression guard**

Add a test that a valid turn makes exactly one generation call.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Malformed replies are retried with a specific instruction.
- Cut replies use completion recovery; provider errors use the fallback.
- The cap aborts with recorded attempts.
- A valid turn is untouched.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the single-call valid turn"
```
