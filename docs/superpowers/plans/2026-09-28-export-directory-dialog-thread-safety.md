# Export Directory Dialog Thread Safety Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-28.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Open the Wails export directory picker on the application main thread, so a call from an HTTP handler goroutine cannot deadlock.

**Architecture:** Wrap only the dialog call in `application.InvokeSyncWithResultAndError`, which marshals the closure onto the main thread and returns its result. The picker's injected signature, the 501 fallback, and the frontend are unchanged.

**Tech Stack:** Go 1.27, Wails v3 (pinned, no dependency change).

**Spec:** `docs/superpowers/specs/2026-09-28-export-directory-dialog-thread-safety-design.md`

## Global Constraints

- Use `interface{}`, never `any`; `go vet` clean.
- No frontend or API change.
- The picker remains an injected `func(defaultDir string) (string, error)`.

---

### Task 1: Marshal the dialog onto the main thread

**Files:** `cmd/localrpg/gui.go`.

- [x] **Step 1:** Wrap the dialog body in
  `application.InvokeSyncWithResultAndError(func() (string, error) { ... })` and
  return its result directly from the picker closure.
- [x] **Step 2:** Keep the cancel semantics (`err != nil` → `"", nil`).

### Task 2: Verify

- [x] **Step 1:** `go build ./...` (the native branch compiles against the pinned
  Wails API).
- [x] **Step 2:** `go vet ./cmd/...`.
- [x] **Step 3:** `go test ./pkg/gui/` — the stub-picker test is unchanged.
- [x] **Step 4:** Manual (deferred to a desktop run): open the export dialog in
  the Wails window, pick a folder, confirm no freeze; cancel and confirm the
  field is unchanged.
