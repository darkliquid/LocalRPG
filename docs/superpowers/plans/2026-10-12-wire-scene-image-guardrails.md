# Wire the code that nothing calls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the campaign's image budget and approval policy actually apply, and resolve the ten `U1000` findings that are not obsolete code.
**Architecture:** One missing call at the scene worker's creation site, plus one missing callback; then delete the three groups that are genuinely dead.
**Tech Stack:** Go 1.27, `staticcheck` 2026.2.1 (added by TQ-4).
**Spec:** `docs/superpowers/specs/2026-10-12-wire-scene-image-guardrails-design.md`

## Global Constraints

- No change to `ImageBudget`, `ImageApprovalMode` or `scenePlan`. The defect is that nothing configures them, not that they are wrong.
- No change for a campaign that has set neither a budget nor an approval mode.
- No production API added for a test.
- `staticcheck` must report none of the ten `U1000` findings this plan owns when it is done.

---

### Task 1: Wire the scene-image guardrails

**Files:**
- Modify: `pkg/gui/service.go`

- [ ] **Step 1: Record the baseline**

Run: `staticcheck ./... | grep -c U1000`
Result: 15 across the tree; 10 of them are the ones this plan owns.

- [ ] **Step 2: Call the configuration at the creation site**

In `prepareTurn`, the scene worker is built and given only `SetOnReady`. Add the call that
gives it the budget, approval mode, price and procedural fallback, immediately after
`SetOnReady` and before `orchestrator.SetSceneWorker`.

- [ ] **Step 3: Report an approval-gated generation**

`configureSceneBudget` sets `SetOnSkipped` but not `SetOnPending`, so `ask` mode returns
`scenePlan{pending: true}` and reports nothing. Add a `SetOnPending` that traces
`turn.image_pending` and calls `broadcastSceneImagePending` with `"awaiting_approval"`.

- [ ] **Step 4: Verify it builds and the call is no longer dead**

Run: `go build ./... && staticcheck ./... | grep U1000`
Expected: `configureSceneBudget`, `saveImageBudget`, `broadcastSceneImagePending`,
`imageProviderMetered` and `imagePrice` are all absent from the output, because each is
now reachable.

---

### Task 2: Delete the two half-built paths

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/provider/imagehttp/client.go`

- [ ] **Step 1: Remove the STT registry**

Delete `Service.sttRegistry`, the `sttReg` and `sttRegMu` fields, and its clause in
`invalidateRegistries`. `TranscribeAudio` resolves its client directly through
`media.NewSTTClientWithSharedKey`, so nothing ever built this registry and no caller can:
the getter is half of a feature that was never written.

- [ ] **Step 2: Remove the ComfyUI usage recorder**

Delete `comfyUIImageClient.setLastUsage` and the field it writes, and have `LastUsage()`
return the zero value, which its own comment already says is what ComfyUI reports.

- [ ] **Step 3: Verify**

Run: `go build ./... && staticcheck ./... | grep U1000`
Expected: neither symbol appears.

---

### Task 3: Delete the three genuinely dead symbols

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/export/script.go`
- Modify: `pkg/harness/context.go`

- [ ] **Step 1: `TurnOrchestrator.generate`**

A one-line wrapper over `generateRequest` with no caller. Delete it and its comment; keep
`generateRequest`, which is the real implementation and is used.

- [ ] **Step 2: `ScriptCompiler.rootDir`**

The struct field is never read. `NewScriptCompiler`'s parameter of the same name is a
different thing and stays.

- [ ] **Step 3: `speechFormattingInstruction`**

Delete the constant, its comment and the blank line after it.
`FormatSpeechFormattingInstructions` superseded it, and R-10 already named it.

- [ ] **Step 4: Verify the ten are gone**

Run: `staticcheck ./...`
Expected: 15 findings, none of them the ten this plan owns. The five `U1000` that remain
(`replaceTag`, `writeJSONErrorCode`, `arrayProperty`, `entityFolderFor`, `voiceOptions`)
belong to TQ-4, which deletes them on its own branch.

---

### Task 4: Verify the whole change

**Files:** none

- [ ] **Step 1: Formatting, vet and build**

Run: `gofmt -l cmd pkg tools && go build ./... && go vet ./...`
Expected: all silent.

- [ ] **Step 2: The full suite**

Run: `go test -count=1 ./...`
Expected: 63 packages `ok`, exit 0.

- [ ] **Step 3: Lint**

Run: `mise run lint`
Expected: exit 0. Note that until TQ-4 lands, `lint:staticcheck` does not exist yet, so
the staticcheck run in Task 3 is the gate for this change.

- [ ] **Step 4: Commit**

Commit as `fix(gui): enforce the campaign's image budget and approval policy`.
