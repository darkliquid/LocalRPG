# Rewrite interface{} as any Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove every `interface{}` from the tree so the code matches the documented convention.
**Architecture:** One AST-based rewrite with `gofmt -r`, verified by the compiler and the existing suite.
**Tech Stack:** Go 1.27 toolchain (`gofmt -r`).
**Spec:** `docs/superpowers/specs/2026-10-12-interface-to-any-design.md`

## Global Constraints

- No behaviour change: `any` is an alias for `interface{}`.
- Use `gofmt -r`, never a text substitution, so shadowed identifiers are caught.
- Tests are rewritten too; the convention is repository-wide.

---

### Task 1: Rewrite

**Files:**
- Modify: 224 files under `cmd/`, `pkg/` and `tools/`

- [x] **Step 1: Record the baseline**

Run: `rg -o 'interface\{\}' -g '*.go' | wc -l`
Result: 636.

- [x] **Step 2: Rewrite**

Run: `gofmt -r 'interface{} -> any' -w cmd pkg tools`

- [x] **Step 3: Confirm none remain**

Run: `rg -o 'interface\{\}' -g '*.go' | wc -l`
Expected: 0.

- [x] **Step 4: Build**

Run: `go build ./...`
Expected: fails at `pkg/gui/service.go:3314`, `any (local variable) is not a type`. This is
the expected detection, not an accident; continue to Task 2.

- [x] **Step 5: Commit**

Empty commit is not needed; the fix in Task 2 lands in the same commit.

---

### Task 2: Fix the identifier the rewrite exposed

**Files:**
- Modify: `pkg/gui/turn_audio.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/turn_audio_test.go`

- [x] **Step 1: Understand the collision**

`turnAudioPlan.heardState` returns `(all, any bool)`. The result named `any` shadows the
builtin inside the function body, so `map[string]any` at the call site resolves to a bool.

- [x] **Step 2: Rename the result**

Change the signature to `(all, some bool)`, rename the body's uses, and update the doc
comment to say why the name is `some`.

- [x] **Step 3: Update the call sites**

`pkg/gui/service.go:3309` and `pkg/gui/turn_audio_test.go:178`.

- [x] **Step 4: Verify**

Run: `go build ./... && go vet ./... && gofmt -l cmd pkg tools`
Expected: silent, and no unformatted files.

- [x] **Step 5: Run the suite**

Run: `go test -count=1 ./...`
Expected: 63 packages `ok`, exit 0.

- [x] **Step 6: Commit**

Commit as `refactor: use any instead of interface{} throughout`.
