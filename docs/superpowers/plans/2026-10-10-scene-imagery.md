# Scene Imagery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Describe the scene's characters from their authored appearance, and generate a scene image only on a major scene change.

**Architecture:** The scene prompt's cast gains appearance, read from each present entity. A new `major` trigger policy replaces `significant` as the default and is a pure `MajorSceneChange` function: a scene break, a location change, a major new character, or an extreme check outcome.

**Tech Stack:** Go 1.27, `pkg/engine`, `pkg/config`, `pkg/gui`.

**Spec:** `docs/superpowers/specs/2026-10-10-scene-imagery-design.md`
**Issue:** [#132](https://github.com/darkliquid/LocalRPG/issues/132)

## Global Constraints

- The scene prompt stays deterministic and the stable prefix is unchanged.
- The cast is capped at four, each description bounded, so the prompt cannot grow into a page.
- `major` is opt-in-reversible: `trigger: significant` restores the old heuristic.
- **Scope note:** deferred to follow-ups, and why. (1) Scenes as units with an `index.json` and non-major turns inheriting the current image: the trigger change already stops the over-generation, and the inheritance needs a storage model this increment does not add. (2) Prompt-hash deduplication and wiring `configureSceneBudget` into the live worker. (3) The frontend "Regenerate scene" control. The two parts here are the ones the report names as faults.

## File Map

| File | Change |
| --- | --- |
| `pkg/engine/scene_worker.go` | `SceneCastMember`, `SceneCast`, `BuildScenePrompt` |
| `pkg/engine/scene_prompt_test.go` | cast tests |
| `pkg/engine/orchestrator.go` | build the cast at the call site |
| `pkg/gui/service.go` | the manual path builds the cast |
| `pkg/engine/image_trigger.go` | `MajorSceneChange`, the `major` branch, config defaults |
| `pkg/engine/image_trigger_test.go` | the trigger table test |
| `pkg/config/types.go` | `major` as the default `ImageTrigger()` |

---

### Task 1: The cast, with appearance

**Files:**
- Modify: `pkg/engine/scene_worker.go`, `pkg/engine/orchestrator.go`, `pkg/gui/service.go`
- Test: `pkg/engine/scene_prompt_test.go`

- [x] **Step 1: Write the failing test** that a cast member's appearance renders in the prompt, and that an empty appearance falls back to a bounded excerpt.
- [x] **Step 2: Run it to verify it fails.** `go test -run TestBuildScenePrompt ./pkg/engine/`
- [x] **Step 3: Implement** `SceneCastMember{Name, Appearance}` and `SceneCast(source, turn)` (reading `ent.Appearance`, falling back to a body excerpt then tags), replace `ScenePromptContext.Entities` with `Cast`, render `characters: Name (appearance); ...`, and build the cast at both call sites.
- [x] **Step 4: Run it to verify it passes**, including the existing prompt tests updated to the cast.
- [x] **Step 5: Commit.**

### Task 2: The `major` trigger

**Files:**
- Modify: `pkg/engine/image_trigger.go`, `pkg/config/types.go`
- Test: `pkg/engine/image_trigger_test.go`

- [x] **Step 1: Write the failing table test** for `MajorSceneChange`: fires on a location change, a major new character, an extreme outcome, and a scene break; does **not** fire on a long narration or a returning character within the window.
- [x] **Step 2: Run it to verify it fails.**
- [x] **Step 3: Implement** `MajorSceneChange(turn, pastTurns, cfg)`, add `ExtremeOutcomes` (default `critical, critical_success, fumble, critical_failure`) and `MajorCharacterWindow` (default 5) to `TriggerConfig`, add the `major` branch to `shouldIllustrate`, and make `major` the default in `ImageTrigger()`.
- [x] **Step 4: Run it to verify it passes**, plus `go test ./pkg/engine/ ./pkg/config/`.
- [x] **Step 5: Commit.**

## Verification

- `go test ./...` and `mise run lint`.
