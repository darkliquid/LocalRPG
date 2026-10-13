# Response encoding errors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a response that cannot be encoded a 500 the client can parse, instead of a 200 whose body stops mid-object, by marshalling before anything is written.
**Architecture:** Two response helpers in `pkg/gui` marshal first and answer a fault when marshalling fails; ten inline encoder sites become calls to them; `pkg/debugger` gets the same shape locally for its three handlers.
**Tech Stack:** Go 1.27, `encoding/json`, `net/http`, the standard test library.
**Spec:** `docs/superpowers/specs/2026-10-13-response-encoding-errors-design.md`

## Global Constraints

- The bytes of a response that encodes do not change, newline included: `json.Marshal` plus `'\n'` is what `Encoder.Encode` wrote.
- No status, route or body changes for anything that encodes.
- The fault message is fixed and generic. The marshalling error names internal types and does not go to a client.
- The streaming handlers are not touched. Their status is committed before the first event; the spec records what is left discarded and why.

## File Map

| File | Why |
|---|---|
| `pkg/gui/server.go` | `writeJSON`, `writeJSONStatus`, `writeSaveError` and eight inline encoder sites |
| `pkg/gui/registry.go` | one inline encoder site behind a `201` |
| `pkg/gui/response_encoding_test.go` | new: the failure path and the byte-for-byte path |
| `pkg/debugger/server.go` | three handlers with their own server and no shared helper |

---

### Task 1: Fail the test before the fix exists

**Files:**
- Create: `pkg/gui/response_encoding_test.go`

- [ ] **Step 1: Write the failure test**

Drive `writeJSON` through `httptest.NewRecorder` with a value `json.Marshal` rejects (a struct field of a type with no encoding, an unencodable map key, or a `math.Inf` float), and assert the recorder's status is `http.StatusInternalServerError` and its body parses as the standard error envelope.

- [ ] **Step 2: Run it and read the failure**

Run: `go test -run TestWriteJSONUnencodable ./pkg/gui/`
Expected: fail, reporting `200` where the test wants `500`. That failure is the finding: the status is written before the encoder runs.

- [ ] **Step 3: Write the byte-identity test**

Assert a normal value produces exactly the bytes `json.NewEncoder` used to produce, newline included, so the change cannot be observed by a client that receives a well-formed response.

---

### Task 2: Marshal before writing

**Files:**
- Modify: `pkg/gui/server.go`

- [ ] **Step 1: Rewrite `writeJSON`**

Marshal into a buffer, answer `writeEncodeFailure` on error, and otherwise set the content type and write the body with its newline.

- [ ] **Step 2: Add `writeEncodeFailure`**

Answer `500` through `writeJSONError`, whose envelope is a map of strings and cannot fail the way the response just did. The message is fixed: the marshalling error names internal types.

- [ ] **Step 3: Rewrite `writeJSONStatus`**

Marshal before `WriteHeader`, so an explicit status is only sent for a body that exists.

- [ ] **Step 4: Rewrite `writeSaveError`**

Marshal before its `422`, keeping the shape: a frontmatter failure still carries its line and column.

- [ ] **Step 5: Run the tests**

Run: `go test -run 'TestWriteJSON' ./pkg/gui/`
Expected: both pass.

---

### Task 3: Replace the inline sites

**Files:**
- Modify: `pkg/gui/server.go` (eight sites: `:176` in `writeSaveError`, `:405`, `:433`, `:617`, `:1382`, `:1427`, `:1492`, `:1499`)
- Modify: `pkg/gui/registry.go` (`:537`)

- [ ] **Step 1: Batch the `201` and `202` sites**

Each is `w.Header().Set(...)`, `w.WriteHeader(status)`, then an encode. That is `writeJSONStatus(w, status, data)`, which is why the duplication existed and why the result was ignored.

- [ ] **Step 2: Batch the plain sites**

`handleModelsRoutes`' status list is `writeJSON(w, statuses)`.

- [ ] **Step 3: Confirm nothing is left**

Run: `grep -rn '_ = json.NewEncoder(w)' pkg cmd`
Expected: no matches outside the streaming closures, which the plan does not touch.

- [ ] **Step 4: Build and test**

Run: `go build ./... && go test ./pkg/gui/`
Expected: both pass; the route tests exercise every rewritten site.

---

### Task 4: The debugger's three handlers

**Files:**
- Modify: `pkg/debugger/server.go`

- [ ] **Step 1: Add a local helper of the same shape**

`handleActions`, `handleActionDetail` and `handleSpans` each set the content type and discard the encode. The package has its own server and its own error text, and importing `pkg/gui` for a twelve-line function would pull the GUI package in; the helper is local, with a comment pointing at the spec.

- [ ] **Step 2: Use it at all three sites**

- [ ] **Step 3: Test**

Run: `go test ./pkg/debugger/`
Expected: pass.

---

### Task 5: Verify

**Files:** none

- [ ] **Step 1: The whole suite**

Run: `mise run test`
Expected: Go, frontend and e2e pass, and the coverage floor is met.

- [ ] **Step 2: Lint**

Run: `gofmt -l pkg && mise run lint`
Expected: silent, exit 0.

- [ ] **Step 3: Read the diff for stray behaviour changes**

Every changed response site should differ only in that it now marshals first. A site that changed its status, its content type or its payload shape is a mistake.

- [ ] **Step 4: Commit**

Commit as `fix(gui): answer a 500 when a response cannot be encoded`.
