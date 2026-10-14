# Explain Panics in JavaScript Host Bindings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Encapsulate the twelve bare `panic` calls in `pkg/rules/js_engine.go` behind a documented `throwJS` helper explaining Goja's exception mechanism, and add test coverage.
**Architecture:** Add `throwJS(format string, args ...any)` on `*JSEngine` in `pkg/rules/js_engine.go`; document `bindHostAPI`; replace all 12 `panic(j.vm.ToValue(...))` sites; add tests for JS `try ... catch` and Go error propagation.
**Tech Stack:** Go 1.27, Goja JavaScript runtime.
**Spec:** `docs/superpowers/specs/2026-10-14-rules-explain-js-panics-design.md`

## File Map

| File | Why |
|---|---|
| `pkg/rules/js_engine.go` | add `throwJS`, document `bindHostAPI`, migrate 12 panic call sites |
| `pkg/rules/js_engine_test.go` | add test verifying JS exception catching and Go error wrapping |

---

### Task 1: Add unit test for JS exception catching and Go error propagation

**Files:**
- Modify: `pkg/rules/js_engine_test.go`

- [ ] **Step 1: Write test for host exception handling in JS and Go**

Write `TestJSEngineHostExceptionPropagation` asserting:
1. When a script runs with a `try ... catch` block calling a failing host method (e.g. invalid roll notation), the error is caught in JS and caught message is accessible.
2. When a script registers an action hook that calls a failing host method without `try ... catch`, `ExecuteAction` returns a Go error wrapping the exception message.
3. When registering a hook with an invalid non-function argument (e.g. `onAction("test", 123)`), loading the script returns an error mentioning `handler must be a function`.

- [ ] **Step 2: Run test and observe it passes or establishes baseline**

Run: `go test -v ./pkg/rules -run TestJSEngineHostExceptionPropagation`

---

### Task 2: Encapsulate and explain panics in `pkg/rules/js_engine.go`

**Files:**
- Modify: `pkg/rules/js_engine.go`

- [ ] **Step 1: Add `throwJS` method and document `bindHostAPI`**

Add `throwJS(format string, args ...any)` with comprehensive doc comment explaining Goja's panic mechanism. Document `bindHostAPI`.

- [ ] **Step 2: Replace all 12 `panic(j.vm.ToValue(...))` call sites**

Replace 12 sites with `j.throwJS(...)`.

- [ ] **Step 3: Run tests and linters**

Run: `go test -v ./pkg/rules/...`
Run: `mise run lint:gofmt && mise run lint:staticcheck`
Run: `mise run test:backend`

---

### Task 3: Verification & Integration

- [ ] **Step 1: Run full verification**

Run: `mise run test` and `mise run lint`

- [ ] **Step 2: Commit, PR, merge, close bead**

Create PR, verify CI checks pass, merge PR, close bead `LocalRPG-92l.7`.
