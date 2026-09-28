# Rebuildability & Documentation Integrity Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-28.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove that deleting a campaign's index and replaying its Markdown and history reconstructs exactly the same derived state, and remove the documentation that points contributors at symbols and behaviour that no longer exist.

**Architecture:** A delete-and-replay equivalence test in `pkg/engine` snapshots every derived table before and after the rebuild. Live contributor docs are fixed and guarded by a scanner test. The unwired `WasmEngine` is removed as dead code.

**Tech Stack:** Go 1.27 stdlib tests.

**Spec:** `docs/superpowers/specs/2026-09-28-rebuildability-and-documentation-integrity-design.md`

## Global Constraints

- Use `interface{}`, never `any`; `go vet` clean.
- Standard library only in tests.
- Historical, dated documents under `docs/superpowers` are records and are not rewritten; a correction is an erratum.
- `go mod tidy` drops the wazero dependency when `WasmEngine` is deleted.

---

### Task 1: Delete-and-replay equivalence test

**Files:** `pkg/engine/rebuild_invariant_test.go`.

- [x] **Step 1:** Build a campaign with two entity notes (wikilink, location,
  faction) and three turns carrying segments, mentions, a check, and a memory.
- [x] **Step 2:** Snapshot the derived state: turn count, max turn, entity
  summaries, per-entity turns, per-entity memories, and the working set.
- [x] **Step 3:** Close the store, delete `index.db` (and its `-wal`/`-shm`),
  reopen, run `Syncer.Sync` + `EnsureIndexed`, and assert the snapshot is equal.
- [x] **Step 4:** Assert a second `EnsureIndexed` is a no-op (idempotence).
- [x] **Step 5:** `go test ./pkg/engine/`.

---

### Task 2: Fix the live documentation

**Files:** `AGENTS.md`, `docs/debugging.md`.

- [x] **Step 1:** `AGENTS.md`: `AssembleContextWithProfiles` →
  `ContextAssembler.Assemble(ContextRequest)`.
- [x] **Step 2:** `docs/debugging.md`: same correction.

---

### Task 3: Guard against removed symbols

**Files:** `pkg/gui/docs_symbols_test.go`.

- [x] **Step 1:** Scan the live docs (`AGENTS.md`, `README.md`,
  `docs/debugging.md`, `docs/architecture/*.md`) for a denylist of removed
  symbols and fail with file:line. `docs/superpowers` is exempt as dated records.
- [x] **Step 2:** A seeded fixture proves the scanner flags a reference.
- [x] **Step 3:** `go test ./pkg/gui/`.

---

### Task 4: Correct the stale video claims

**Files:** the canonical-db design spec and its plan.

- [x] **Step 1:** Add a dated erratum noting that `pkg/export/video.go` now
  animates and muxes per-beat audio.

---

### Task 5: Remove the unwired wasm engine

**Files:** delete `pkg/rules/wasm_engine.go` and `pkg/rules/wasm_engine_test.go`;
`go.mod`/`go.sum`.

- [x] **Step 1:** Delete both files (no production caller; `JSEngine` is the
  single execution path).
- [x] **Step 2:** `go mod tidy`, then `go build ./...`.

---

### Task 6: Full verification

- [x] **Step 1:** `go build ./...`, `go test ./...`, `go vet ./...`.
