# Add static analysis to lint and CI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `staticcheck` part of `mise run lint` and CI, and resolve its findings so the gate lands on a clean tree.
**Architecture:** A pinned tool in `mise.toml`, one `lint:staticcheck` task wired into `lint`, one CI step, and 25 findings fixed.
**Tech Stack:** Go 1.27, `staticcheck` 2026.2.1, mise tasks, GitHub Actions.
**Spec:** `docs/superpowers/specs/2026-10-12-staticcheck-gate-design.md`

## Global Constraints

- No suppressions, baselines or exclusion lists. The gate is clean or it is not added.
- The command a contributor runs locally is the command CI runs.
- Findings that are wanted-but-unwired are wired up, not deleted. That is RD-13's precedent.

---

### Task 1: Measure before deciding

**Files:**
- Modify: `mise.toml`

- [x] **Step 1: Install and pin the tool**

`mise ls-remote staticcheck` offers 2026.2.1. Add `staticcheck = "2026.2.1"` to `[tools]`.

- [x] **Step 2: Get the finding count**

Run: `mise exec -- staticcheck ./...`
Result: 25 findings, not the hundreds a `golangci-lint`-shaped history suggests. Fixing all
of them and gating on zero is therefore the right call, and no baseline is needed.

- [x] **Step 3: Note what the checks caught**

Five `U1000` in unused helpers, three `SA4000` tautological test assertions, four `SA1019`
deprecated APIs, one `SA4010` discarded `append`, two `ST1005` capitalised error strings,
and ten more `U1000` that turned out to be a feature wired to nothing.

---

### Task 2: Resolve the findings that are safe to resolve

**Files:**
- Modify: `pkg/provider/geminillm/provider.go` and its test
- Modify: `pkg/engine/scene_prompt_test.go`, `pkg/provider/oracle/provider_test.go`, `pkg/scene/genre_test.go`
- Modify: `pkg/content/unpack.go`, `pkg/debugger/collector.go`, `pkg/telemetry/inmemory.go`, `pkg/media/tts.go`
- Delete: `pkg/export/web.go` `replaceTag`, `pkg/gui/generation_errors.go` `writeJSONErrorCode`, `pkg/harness/turn_tools.go` `arrayProperty`, `pkg/ingest/ingest.go` `entityFolderFor`, `pkg/provider/ttselevenlabs/client.go` `voiceOptions`

- [x] **Step 1: Fix the discarded tool calls (`SA4010`)**

`geminillm`'s `Generate` collected function calls and returned only the text, because
`harness.GenerateResponse` has no field for them. Report the condition instead of dropping
it, and handle the `json.Marshal` error in the same block.

- [x] **Step 2: Add the test, and check it fails without the fix**

A fake Gemini response returning a `functionCall` part must make `Generate` error. Run it
against the old behaviour to confirm it fails for the right reason.

- [x] **Step 3: Fix the tautological assertions (`SA4000`)**

`BuildScenePrompt`, `craftProse` and `gradientFor` were each compared against an inline
second call to themselves. Compare named results instead, which is what the assertions
meant.

- [x] **Step 4: Replace the deprecated APIs (`SA1019`)**

`tar.TypeRegA` (the `Reader` normalises it since Go 1.11, so the check was dead as well as
deprecated), `attribute.Value.Emit` twice, and `trace.NewNoopTracerProvider`.

- [x] **Step 5: Lowercase the error strings (`ST1005`)**

Without losing the proper nouns: Ogg and Opus stay capitalised, the sentence start does not.

- [x] **Step 6: Delete the five unambiguously dead symbols**

- [x] **Step 7: Verify**

Run: `go build ./... && staticcheck ./... | wc -l`
Expected: 10, all `U1000`, all accounted for by Task 3.

---

### Task 3: Hand the unwirable findings to their own proposal

**Files:** none

- [x] **Step 1: Recognise what the last ten are**

None is obsolete. `configureSceneBudget` is the only caller of the scene worker's setters
and nothing calls it, so the campaign's image budget is never enforced. `imageProviderMetered`
and `imagePrice` are unreachable for the same reason; the STT registry and the ComfyUI
usage recorder have the same shape.

- [x] **Step 2: File RD-13 rather than deleting them**

`#216`, which wires the scene guardrails and removes the two half-built paths. This
proposal depends on it.

- [x] **Step 3: Rebase once RD-13 lands, and confirm the count is zero**

Run: `git rebase main && staticcheck ./...`
Expected: exit 0, no output. Confirmed.

---

### Task 4: Wire the gate

**Files:**
- Modify: `mise.toml`
- Modify: `.github/workflows/ci.yml`
- Modify: `AGENTS.md`

- [x] **Step 1: Add the task**

`lint:staticcheck` running `staticcheck ./...`, using the default check set.

- [x] **Step 2: Add it to `mise run lint`**

Add to the `lint` task's `depends`.

- [x] **Step 3: Add the CI step**

A "Staticcheck" step in the `go` job, after "Vet" and before "Test".

- [x] **Step 4: Verify the gate fails on a violation**

Add `func unusedProbe() {}` to a file in `pkg/` and run `mise run lint:staticcheck`.
Expected: `U1000`, naming the symbol, and `ERROR task failed`. Then delete it and confirm
exit 0.

- [x] **Step 5: Lint the workflow and the tree**

Run: `actionlint && mise run lint`
Expected: both silent, exit 0.

- [x] **Step 6: Run the suite**

Run: `go test -count=1 ./...`
Expected: 63 packages `ok`.

- [x] **Step 7: Document the task**

Add `lint:staticcheck` to the command list in `AGENTS.md`.

- [x] **Step 8: Commit**

Commit as `chore: add staticcheck to lint and CI`.
