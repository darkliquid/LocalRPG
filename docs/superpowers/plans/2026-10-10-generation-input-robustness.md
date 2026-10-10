# Generation Input Robustness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop malformed model replies surfacing as a raw "invalid character" error: repair illegal bytes inside strings, retry once, and present a plain reason.

**Architecture:** `pkg/jsonrepair` gains a string-repair pass tried like its structural passes; `pkg/sysgen` and `pkg/worldgen` route every decode through a `generateJSON` helper that retries once with the parse error appended; a `ErrMalformedReply` sentinel maps to a friendly handler message; enhancement entity ids are slugified client-side.

**Tech Stack:** Go 1.27, `encoding/json`, `gopkg.in/yaml.v3`; React 19, TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-10-generation-input-robustness-design.md`
**Issue:** [#129](https://github.com/darkliquid/LocalRPG/issues/129)

## Global Constraints

- `pkg/jsonrepair` only removes, truncates, closes, or escapes; it never invents content, and a pass is adopted only when the payload validates.
- Exactly one retry per model call, never for the deterministic oracle.
- The raw Go error stays in the trace and the NDJSON `detail`; only the headline changes.

## File Map

| File | Change |
| --- | --- |
| `pkg/jsonrepair/repair.go`, `scan.go` | string pass, BOM, smart quotes |
| `pkg/jsonrepair/repair_test.go` | table tests |
| `pkg/sysgen/sysgen.go`, `steps.go`, `enhance.go`, `derive.go` | `generateJSON`, sentinel |
| `pkg/worldgen/worldgen.go`, `steps.go`, `entities.go`, `enhance.go` | `generateJSON`, sentinel |
| `pkg/gui/system_enhance.go`, `sysgen_routes.go`, `worldgen_entities.go`, `worldgen_routes.go` | friendly mapping |
| `frontend/src/components/WorldEnhanceDialog.tsx` | slugify ids before submit |

---

### Task 1: The string-repair pass

**Files:**
- Modify: `pkg/jsonrepair/repair.go`, `pkg/jsonrepair/scan.go`
- Test: `pkg/jsonrepair/repair_test.go`

- [ ] **Step 1: Write failing table tests** for a literal newline/tab/control byte inside a string, an unterminated string, a BOM, smart-quote delimiters, and a smart quote inside a value that must be left alone.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the passes and the new kinds.**
- [ ] **Step 4: Run them; assert the repaired payload unquotes to the original text.**
- [ ] **Step 5: Commit.**

### Task 2: One bounded retry

**Files:**
- Modify: `pkg/sysgen/sysgen.go`, `pkg/worldgen/worldgen.go` and their step call sites
- Test: `pkg/sysgen/*_test.go`, `pkg/worldgen/*_test.go`

- [ ] **Step 1: Write failing tests** with a fake generator returning malformed then valid JSON (retries once) and valid JSON (no retry).
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Add `generateJSON` and switch the steps to it.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 3: A plain-language error

**Files:**
- Modify: `pkg/sysgen/sysgen.go`, `pkg/worldgen/worldgen.go`, `pkg/gui/system_enhance.go`, `pkg/gui/sysgen_routes.go`, `pkg/gui/worldgen_routes.go`
- Test: `pkg/gui/system_enhance_test.go`, `pkg/gui/worldgen_test.go`

- [ ] **Step 1: Write failing tests** that `ErrMalformedReply` maps to the friendly message and a `502`/`code` value.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Add the sentinel, uniform wrapping, and the mapping.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 4: Slug client-supplied enhancement ids

**Files:**
- Modify: `frontend/src/components/WorldEnhanceDialog.tsx`
- Test: its test file

- [ ] **Step 1: Write a failing test** that a non-slug entity id is slugified before submit.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Slugify with the shared helper.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**
