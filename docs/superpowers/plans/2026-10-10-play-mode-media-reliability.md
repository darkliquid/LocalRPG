# Play Mode Media Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Never show a broken portrait, and never let an audio failure be silent: every beat either plays or reports why.

**Architecture:** A shared `EntityAvatar` with a deterministic initials fallback replaces every bare portrait `<img>`; the server returns a placeholder for an unknown character id; `AudioStatusDTO` gains `Error`/`Stage`, broadcast at each failure point and surfaced by the client's SSE handler; the playback package stops discarding decode errors.

**Tech Stack:** Go 1.27, `pkg/media/playback`; React 19, TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-10-play-mode-media-reliability-design.md`
**Issue:** [#130](https://github.com/darkliquid/LocalRPG/issues/130)

## Global Constraints

- The `AudioStatusDTO` change is additive; an older client ignores `Error`.
- Only the first failure per beat is broadcast.
- The export player's no-conditional degradation is unchanged.

## File Map

| File | Change |
| --- | --- |
| `frontend/src/components/EntityAvatar.tsx` | new |
| `frontend/src/components/TurnSegments.tsx`, `theater/TheaterStage.tsx`, `theater/TheaterDialogue.tsx`, `ImageLightbox.tsx`, `ChronicleView.tsx` | use `EntityAvatar` |
| `frontend/src/components/StoryTheater.tsx`, `App.tsx` | stop fabricating URLs, version cache-bust |
| `pkg/gui/server.go` | placeholder for an unknown character id |
| `pkg/gui/types.go`, `service.go`, `turn_audio.go`, `streaming_tts.go` | error/status broadcast |
| `pkg/media/playback/player.go`, `queue.go` | capture decode error |
| `frontend/src/hooks/useSegmentPlayback.ts`, `useStreamedSpeech.ts`, `App.tsx` | surface errors |

---

### Task 1: `EntityAvatar`

**Files:**
- Create: `frontend/src/components/EntityAvatar.tsx`, `frontend/src/components/EntityAvatar.test.tsx`

- [ ] **Step 1: Write failing tests** for a `src` render, an `onError` fallback, a missing `src`, and a stable colour per name.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the component.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 2: Replace the portrait sites and URLs

**Files:**
- Modify: the five render sites, `StoryTheater.tsx`, `App.tsx`
- Test: `StoryTheater.test.tsx`, `TurnSegments` tests

- [ ] **Step 1: Write failing tests** that a speaker with no portrait renders the fallback and that the theater does not build a player URL with no portrait.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Swap in `EntityAvatar`, drop the fabricated URL, and use the portrait version for cache-busting.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 3: Server placeholder for an unknown id

**Files:**
- Modify: `pkg/gui/server.go`
- Test: `pkg/gui/character_portrait_test.go`

- [ ] **Step 1: Write a failing test** that an unknown character id returns `200 image/svg+xml`.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Return the procedural placeholder for unknown/error, log the real error.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 4: The audio error channel

**Files:**
- Modify: `pkg/gui/types.go`, `service.go`, `turn_audio.go`, `streaming_tts.go`
- Test: `pkg/gui/turn_audio_test.go`, `streaming_tts_test.go`

- [ ] **Step 1: Write failing tests** that a synthesis failure, an empty group, a full queue, and an unconfigured TTS each broadcast `Error` with the right `Stage`.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Add the fields and the broadcasts; replace the `continue`s.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 5: Capture a decode failure

**Files:**
- Modify: `pkg/media/playback/player.go`, `queue.go`
- Test: the playback package tests

- [ ] **Step 1: Write a failing test** that `queueStreamer.Err()` reports a captured decode error.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Capture the first error and return it from `Err()`.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 6: The client surfaces it

**Files:**
- Modify: `frontend/src/App.tsx`, `frontend/src/hooks/useSegmentPlayback.ts`, `useStreamedSpeech.ts`, `frontend/src/components/SegmentAudioControls.tsx`
- Test: the hook and control tests

- [ ] **Step 1: Write failing tests** that an SSE `error` sets an error state, `onerror` before a terminal message is an error, the timeout is visible, and a `play()` rejection carries its reason.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the error handling and the retry affordance.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**
