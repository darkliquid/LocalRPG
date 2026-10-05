# Resolve Pending Check Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `POST /turn/{n}/resolve-check` endpoint that rolls a pending check and streams the GM's adjudication as a continuation turn.

**Architecture:** A thin endpoint wrapping the existing turn pipeline with `PendingCheckRef` set; the engine gains an optional forced total for manual rolls; the resulting turn is recorded with `ContinuationOf` and `ResolvesCheckRef`.

**Tech Stack:** Go standard library; the existing NDJSON turn stream.

**Spec:** `docs/superpowers/specs/2026-10-05-resolve-pending-check-design.md`
**Depends on:** SYS-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- The endpoint must reuse the existing resolution; no second resolver.
- `POST /turn` with `PendingCheckRef` must behave exactly as before.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The DTO and the route

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/routes.go`, `pkg/gui/server.go`
- Test: `pkg/gui/resolve_check_test.go`

**Interfaces:**
- Consumes: nothing yet.
- Produces: `ResolveCheckRequestDTO`, the `POST /api/game/{id}/turn/{n}/resolve-check` mount.

- [ ] **Step 1: Write the failing test**

```go
func TestResolveCheckRouteExists(t *testing.T) {
	// Assert the route is registered and rejects a missing body with 400.
	// Use the existing router test helper.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestResolveCheckRouteExists -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add:

```go
type ResolveCheckRequestDTO struct {
	PendingRef   string `json:"pending_check_ref"`
	ManualResult *int   `json:"manual_result,omitempty"`
	Note         string `json:"note,omitempty"`
}
```

Mount the route next to the turn route in `routes.go` and `server.go`, with the same NDJSON headers
as `handleTurnSubmit`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestResolveCheckRouteExists -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/routes.go pkg/gui/server.go pkg/gui/resolve_check_test.go
git commit -m "feat(gui): add the resolve-check route"
```

---

### Task 2: `Service.ResolveCheck`

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/resolve_check_test.go` (append)

**Interfaces:**
- Consumes: the DTO (Task 1), `TurnSession.Run`, the turn lock (`BeginTurn`).
- Produces: `func (s *Service) ResolveCheck(ctx, gameID string, turnNumber int, req ResolveCheckRequestDTO, emit func(TurnEvent) error) error`.

- [ ] **Step 1: Write the failing test**

```go
func TestResolveCheckRejectsMissingPending(t *testing.T) {
	svc := newTestServiceWithGame(t)
	err := svc.ResolveCheck(context.Background(), "game1", 1,
		ResolveCheckRequestDTO{PendingRef: "nope"}, func(TurnEvent) error { return nil })
	if err == nil {
		t.Fatal("a turn with no pending check should error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestResolveCheckRejectsMissingPending -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement `ResolveCheck` per the spec §4.2: load the turn, validate the pending check and ref, acquire
the turn lock, and run the turn pipeline with `Mode: "roll"`, `PendingCheckRef: req.PendingRef`,
`Input: req.Note`, and the forced total when `ManualResult` is set. Return typed errors the handler
maps to `404`/`400`/`409`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestResolveCheck -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/resolve_check_test.go
git commit -m "feat(gui): resolve a pending check through the turn pipeline"
```

---

### Task 3: The forced total

**Files:**
- Modify: `pkg/engine/orchestrator.go` (the resolve path), `pkg/harness/turn.go` if a field is needed
- Test: `pkg/engine/resolve_check_test.go`

**Interfaces:**
- Consumes: `CheckResolver`.
- Produces: a way to force a check's total from the request.

- [ ] **Step 1: Write the failing test**

```go
func TestForcedTotalResolvesToThatTotal(t *testing.T) {
	o := &TurnOrchestrator{}
	o.SetForcedTotal(11)
	res, err := o.resolveCheck(context.Background(), harness.CheckRequest{Notation: "2d6"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 11 || res.Source != "manual" {
		t.Fatalf("result = %+v", res)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestForcedTotal -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add a `forcedTotal *int` to the orchestrator (set per turn, cleared after), and in `resolveCheck`
(`pkg/engine/orchestrator.go:268-284`) use it instead of rolling when set, marking
`CheckResult.Source = "manual"`. Add `Source string` to `CheckResult` if absent. Reset it at the
start of `ProcessActionStream` so a forced total never leaks into a later turn.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run TestForcedTotal -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/resolve_check_test.go pkg/harness/turn.go
git commit -m "feat(engine): allow a forced check total"
```

---

### Task 4: Record the continuation

**Files:**
- Modify: `pkg/engine/orchestrator.go` (turn assembly), `pkg/gui/service.go`
- Test: `pkg/engine/resolve_check_test.go` (append)

**Interfaces:**
- Consumes: `Turn.ContinuationOf`, `Turn.ResolvesCheckRef`.
- Produces: a continuation turn carrying the resolved check.

- [ ] **Step 1: Write the failing test**

```go
func TestResolveRecordsContinuation(t *testing.T) {
	// Resolve a pending check and assert the resulting turn has ContinuationOf set
	// to the proposing turn and ResolvesCheckRef set to the ref.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestResolveRecordsContinuation -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Set `turn.ContinuationOf` to the turn number passed to `ResolveCheck` and `turn.ResolvesCheckRef` to
the pending ref, via the existing turn-assembly fields. The proposing turn keeps its `PendingCheck`;
optionally mark it resolved in the index so a second resolve is `409`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/gui/service.go pkg/engine/resolve_check_test.go
git commit -m "feat(engine): record a resolved check as a continuation"
```

---

### Task 5: Handler and error mapping

**Files:**
- Modify: `pkg/gui/server.go`
- Test: `pkg/gui/resolve_check_test.go` (append)

**Interfaces:**
- Consumes: `Service.ResolveCheck` (Task 2).
- Produces: the NDJSON handler with `404`/`400`/`409` mapping.

- [ ] **Step 1: Write the failing tests**

```go
func TestResolveCheckStatusCodes(t *testing.T) {
	// missing turn -> 404; no pending -> 400; mismatched ref -> 400; in-flight -> 409.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestResolveCheckStatusCodes -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Map the service's typed errors to status codes before the first stream byte, mirroring
`handleTurnSubmit`'s early-status pattern (`pkg/gui/server.go:1521-1523`), then stream the turn
events.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestResolveCheck -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/server.go pkg/gui/resolve_check_test.go
git commit -m "feat(gui): map resolve-check errors to status codes"
```

---

### Task 6: Verification

- [ ] **Step 1: Regression guard**

Add a test asserting `POST /turn` with `PendingCheckRef` still resolves exactly as before.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- The endpoint resolves a pending check and streams a continuation turn.
- A manual total is honoured and marked `manual`.
- Missing/mismatched/resolved checks and concurrency map to the right codes.
- The existing `PendingCheckRef` path is unchanged.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the existing pending-check path"
```
