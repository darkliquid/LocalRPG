# Route every API error through one mapper Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One function decides the status, the code and the message a failed handler call produces, an internal failure carries a fixed message with its detail recorded instead of echoed, and every one of the 97 call sites goes through it.
**Architecture:** `apiFaultFor` maps an error to a `apiFault` from a table built out of the sentinels the handlers already distinguish; `(*Server).writeAPIError` writes it and records the detail through the service logger; `invalid(err)` marks an error as the request's fault.
**Tech Stack:** Go 1.27, `net/http`, `errors.Is`/`errors.As`, `pkg/trace` for the record.
**Spec:** `docs/superpowers/specs/2026-10-14-api-error-mapper-design.md`

## Global Constraints

- A 5xx body never contains the error's text. Its message is fixed, and its detail goes to `api.error`.
- A 4xx keeps the message that says what to change; that is the client's only instruction.
- No failure is decided by matching an error's text.
- Every converted site keeps its meaning. Where a handler stated the request was wrong, that statement survives as `invalid(err)` rather than as a status number.

## File Map

| File | Why |
|---|---|
| `pkg/gui/api_errors.go` | new: the fault, the table, the writer, `invalid`, `methodNotAllowed` |
| `pkg/gui/api_errors_test.go` | new: the table test, the leak test, the method test |
| `pkg/gui/*.go` | the 97 sites, the method fallbacks, the string match |

---

### Task 1: The mapper, test first

**Files:**
- Create: `pkg/gui/api_errors_test.go`
- Create: `pkg/gui/api_errors.go`

- [ ] **Step 1: Write the table test**

One case per row of the spec's table, asserting the status and the code, plus a case for an unknown wrapped error asserting `500`/`internal`.

- [ ] **Step 2: Write the leak test**

Drive a handler with an error whose message contains a filesystem path, and assert the body does not contain it, the status is 500, and the code is `internal`.

- [ ] **Step 3: Run both and read the failure**

Run: `go test -run 'TestAPIFault|TestInternalDetailIsNotSent' ./pkg/gui/`
Expected: fail to compile, which is the point of writing them first.

- [ ] **Step 4: Write `api_errors.go`**

`ErrInvalidRequest` and `invalid`, the `apiFault` type, `apiFaultFor`'s table, `(*Server).writeAPIError`, and `methodNotAllowed`.

- [ ] **Step 5: Add the code to the envelope**

`writeErrorEnvelope(w, status, code, message)` writing `{"error":{"code":...,"message":...}}`, which is what `frontend/src/api/client.ts` already reads, and `writeJSONError` delegating to it with no code.

- [ ] **Step 6: Run the tests**

Run: `go test -run 'TestAPIFault|TestInternalDetailIsNotSent' ./pkg/gui/`
Expected: pass.

---

### Task 2: The 97 sites

**Files:**
- Modify: `pkg/gui/*.go`

- [ ] **Step 1: Record the count before**

Run: `grep -rn 'http.Error(w, err.Error()' pkg/gui | grep -v _test | wc -l`
Expected: 97.

- [ ] **Step 2: Convert the mechanical ones**

Every site becomes `s.writeAPIError(w, err)`, except those whose handler is stating that the request was wrong, which become `s.writeAPIError(w, invalid(err))`. Decide per site from what the error is: a `pathutil` sentinel or the handler's own validation text is the request's fault, everything else is not.

- [ ] **Step 3: Convert from the count down**

Run the grep again after each file, so a missed site is visible rather than assumed.

- [ ] **Step 4: Build and test**

Run: `go build ./... && go test ./pkg/gui/`
Expected: both pass. The route tests exercise every converted site.

---

### Task 3: Wrong methods

**Files:**
- Modify: `pkg/gui/*.go`

- [ ] **Step 1: Find the ten that answer 404**

Run: `grep -rn -B3 'http.NotFound(w, r)' pkg/gui | grep -c 'r.Method'`
Expected: 10.

- [ ] **Step 2: Route them through the helper**

`methodNotAllowed(w, r, http.MethodGet, http.MethodPatch)` answers 405 and sets `Allow`.

- [ ] **Step 3: Add the header to the ones that answered 405 already**

Same helper, so the status and the header come from one place.

---

### Task 4: The string match

**Files:**
- Modify: `pkg/gui/server.go`

- [ ] **Step 1: Make the failure a type**

`ExportContent`'s missing-id case returns a sentinel the mapper maps to 404, so the route stops reading the error's text.

- [ ] **Step 2: Replace the match**

`handleContentExportRoute` answers `s.writeAPIError(w, err)`.

---

### Task 5: Verify

**Files:** none

- [ ] **Step 1: Nothing is left**

Run: `grep -rn 'http.Error(w, err.Error()' pkg/gui | grep -v _test`
Expected: no matches.

- [ ] **Step 2: The whole suite and the lint gate**

Run: `mise run test && mise run lint`
Expected: both pass.

- [ ] **Step 3: Read the diff for status changes**

Every converted site should keep its status. A site that changed one is either a mistake or a case for the spec's table, and either way it is written down.

- [ ] **Step 4: Commit**

Commit as `refactor(gui): route every API error through one mapper`.
