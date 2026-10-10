# Scene Imagery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Describe the scene's characters from their authored appearance, and generate a scene image only on a major scene change, reusing it across the turns of that scene.

**Architecture:** The scene cast gains appearance; a `major` trigger policy replaces `significant` as the default and is a pure `MajorSceneChange` function; scenes become units tracked in the manifest settings and an `assets/scenes/index.json`, so non-major turns inherit the current image with no provider call; the image budget is wired; regeneration is explicit.

**Tech Stack:** Go 1.27, `pkg/engine`, `pkg/media`, `pkg/config`, `pkg/gui`; React 19, TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-10-scene-imagery-design.md`
**Issue:** [#132](https://github.com/darkliquid/LocalRPG/issues/132)

## Global Constraints

- The scene prompt stays deterministic, and the stable prefix (location plus style) is unchanged.
- The cast is capped at four, each description bounded, so the prompt cannot grow into a page.
- `major` is opt-in-reversible: `trigger: significant` restores the old heuristic.
- A scene file is written once and never overwritten; the resolver falls back to legacy `turn-<N>.ext`.

## File Map

| File | Change |
| --- | --- |
| `pkg/engine/scene_worker.go` | `SceneCast`, `presentSceneCast`, `BuildScenePrompt` |
| `pkg/gui/service.go` | manual path builds the cast |
| `pkg/engine/image_trigger.go` | `MajorSceneChange`, `major` branch |
| `pkg/config/types.go` | `major` as the default trigger |
| `pkg/engine/scene_index.go` | new: scene index and settings |
| `pkg/gui/service.go` (DTO) | resolve `image_url` from the index |
| `pkg/engine/scene_worker.go`, `orchestrator.go` | write `scene-<serial>`, dedup, no overwrite |
| `pkg/gui/service.go` | wire `configureSceneBudget` |
| `frontend/src/components/ChronicleView.tsx`, `api/client.ts` | regenerate |
| `pkg/gui/docs/*.md` | docs |

---

### Task 1: The cast, with appearance

**Files:**
- Modify: `pkg/engine/scene_worker.go`, `pkg/gui/service.go`
- Test: `pkg/engine/scene_prompt_test.go`

- [ ] **Step 1: Write failing tests** that a cast member's appearance renders and that an empty appearance falls back to a bounded excerpt; and that the manual builder produces a cast.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Replace `Entities []string` with `Cast []SceneCast` and read `ent.Appearance`.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 2: The `major` trigger

**Files:**
- Modify: `pkg/engine/image_trigger.go`, `pkg/config/types.go`
- Test: `pkg/engine/image_trigger_test.go`

- [ ] **Step 1: Write a failing table test** for `MajorSceneChange`: fires on a location change, a major new character, an extreme outcome, and a scene break; does not fire on a long narration or a new-but-minor speaker.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Implement `MajorSceneChange`, the `major` branch, and the default.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 3: Scenes as units

**Files:**
- Create: `pkg/engine/scene_index.go`
- Modify: `pkg/gui/service.go` (DTO resolution)

- [ ] **Step 1: Write failing tests** that a turn resolves to its scene's image and that a legacy `turn-<N>.ext` still resolves.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the settings accessors, the index, and the resolver.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 4: Generate, dedup, and never overwrite

**Files:**
- Modify: `pkg/engine/orchestrator.go`, `pkg/engine/scene_worker.go`
- Test: `pkg/engine/scene_worker_test.go`, `scene_consistency_test.go`

- [ ] **Step 1: Write failing tests** with a counting generator: a non-major turn calls nothing; an unchanged prompt hash calls nothing; a re-run writes no second file.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the gate, the hash, and the immutable `scene-<serial>` write.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 5: Wire the budget

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/scene_image_test.go`

- [ ] **Step 1: Write a failing test** that an exhausted budget yields the procedural fallback and that `configureSceneBudget` is reached by the turn and manual workers.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Call `configureSceneBudget` at both worker creations.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 6: Regenerate

**Files:**
- Modify: `pkg/gui/service.go`, `frontend/src/api/client.ts`, `frontend/src/components/ChronicleView.tsx`
- Test: `ChronicleView` tests

- [ ] **Step 1: Write failing tests** that a turn with an image shows "Regenerate scene" and sends `force`.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Add the `force` path and the control.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 7: Docs

**Files:**
- Modify: `pkg/gui/docs` and the image trigger spec's default-policy note

- [ ] **Step 1: Describe the cast, the major trigger, scene persistence, and regeneration.**
- [ ] **Step 2: Run `mise run lint:docs`.**
- [ ] **Step 3: Commit.**
