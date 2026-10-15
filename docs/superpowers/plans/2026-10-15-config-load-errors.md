# Report a Malformed Config File Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a config file that does not parse report itself, apply nothing, and reach the user through the error channel or the warnings list, instead of silently becoming defaults.
**Architecture:** One helper parses a file before merging it; `Load` joins the per-file failures, returns them with the still-usable configuration, and appends them to `Warnings()`. The five command sites that discard the error report it and exit non-zero.
**Tech Stack:** Go 1.27, `gopkg.in/yaml.v3`, `errors.Join`.
**Spec:** `docs/superpowers/specs/2026-10-15-config-load-errors-design.md`

## File Map

| File | Why |
|---|---|
| `pkg/config/manager.go` | `mergeConfigFile`; `Load` reports and records the failures |
| `pkg/config/manager_test.go` | the malformed-file cases |
| `cmd/localrpg/registry.go` | three sites report and exit 1 |
| `cmd/localrpg/provenance.go` | one site reports and exits 1 |
| `cmd/localrpg/styles.go` | one site reports and exits 1 |

## Global Constraints

- **The returned configuration is never nil.** Eight call sites depend on it, and the error is the
  new information, not the absence of a configuration.
- **A file that does not parse applies nothing**, so a broken file cannot leak a prefix of itself.
- **A missing or empty file is not an error**, and a well-formed one loads exactly as it did.
- **The commands keep their exit codes**: 1 for a configuration that cannot be loaded, unchanged
  from the commands that already report it.

---

### Task 1: The load contract

- [x] **Step 1: `mergeConfigFile`**

Read, parse into a throwaway `Config`, then merge into the caller's. The error names the path and
wraps the YAML failure.

- [x] **Step 2: `Load` reports and records**

Collect each file's failure, `errors.Join` them, append the same text to `warnings`, and return the
merged configuration with the error. The doc comment states the contract: usable configuration,
error to report.

- [x] **Step 3: Test the four cases**

A malformed user config; a malformed local override (which must not set `isOverride`); a file whose
second field is malformed, leaving the first at its default; and a well-formed pair that still
layers.

---

### Task 2: The command sites

- [x] **Step 1: `cmd/localrpg/registry.go`**

Three sites: the search, and the two that list and install from a registry.

- [x] **Step 2: `cmd/localrpg/provenance.go` and `cmd/localrpg/styles.go`**

One site each. Both answer from the configuration (which publishers are trusted, which style packs
apply), so defaults are the wrong answer.

- [x] **Step 3: Verify the commands**

Run: `go test ./cmd/localrpg/`
Expected: pass, with the `LOCALRPG_CONFIG_DIR` guard still refusing to resolve the real config.

---

### Task 3: Verification

- [x] **Step 1: The malformed file names itself**

A test-written file with a `tab` in the wrong place: the error names the path, and `Warnings()`
carries the same text.

- [x] **Step 2: Vet, test and lint**

Run: `go vet ./... && mise run test && mise run lint`
Expected: pass.

- [x] **Step 3: Close the bead**

`bd close LocalRPG-4kz --reason "..."`, naming the spec and the plan.