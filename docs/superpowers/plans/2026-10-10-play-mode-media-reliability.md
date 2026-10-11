# Play Mode Media Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Never show a broken portrait, and never let an audio failure be silent: every beat either plays or reports why.

**Architecture:** One `EntityAvatar` replaces every bare portrait `<img>` with a deterministic initials fallback, and the server returns a placeholder for an unknown character id. On audio, `AudioStatusDTO` gains `Error`/`Stage`, broadcast wherever synthesis fails, and the client's SSE handler surfaces it instead of treating every end as success.

**Tech Stack:** Go 1.27, `pkg/gui`, `pkg/media`; React 19, TypeScript (strict), Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-10-play-mode-media-reliability-design.md`
**Issue:** [#130](https://github.com/darkliquid/LocalRPG/issues/130)

## Global Constraints

- The `AudioStatusDTO` change is additive; an older client ignores `Error`.
- Only the first failure per beat is broadcast.
- The export player's conditional degradation is unchanged.
- **Scope note:** deferred, and why. (1) The `pkg/media/playback` decode-error capture and the `useSegmentPlayback`/`useStreamedSpeech` reason plumbing: the SSE error channel covers the device path's silence, which is the reported symptom. (2) The ChronicleView scene illustration: it is a scene image, not a character portrait, and an initials fallback would be wrong for it. (3) The StoryTheater fabricated player portrait URL was kept rather than removed: with the server placeholder for an unknown id it now resolves to an image, so it is no longer a broken-image source.

## File Map

| File | Change |
| --- | --- |
| `frontend/src/components/EntityAvatar.tsx`, `.test.tsx` | new |
| `frontend/src/components/TurnSegments.tsx`, `theater/TheaterStage.tsx`, `theater/TheaterDialogue.tsx`, `ImageLightbox.tsx`, `ChronicleView.tsx` | use `EntityAvatar` |
| `frontend/src/components/StoryTheater.tsx` | stop fabricating a player portrait URL |
| `pkg/gui/service.go` | placeholder for an unknown character id |
| `pkg/gui/character_portrait_test.go` | unknown-id placeholder test |
| `pkg/gui/types.go`, `service.go` | `AudioStatusDTO.Error`/`Stage`, broadcasts |
| `frontend/src/types.ts`, `frontend/src/App.tsx` | parse and surface the error |

---

### Task 1: A placeholder for an unknown character id

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/character_portrait_test.go`

- [x] **Step 1: Write the failing test** that `GET /api/game/{id}/character/no-such-note/portrait` returns `200 image/svg+xml`, not a text 404.
- [x] **Step 2: Run it to verify it fails.** `go test -run TestCharacterPortraitUnknownID ./pkg/gui/`
- [x] **Step 3: Implement:** in `GetCharacterPortrait`, an unknown entity becomes a synthetic entity (`ID: cleanID, Name: cleanID, Type: "character"`) that falls through to the procedural SVG, and a store-open failure is non-fatal so the placeholder is still served.
- [x] **Step 4: Run it to verify it passes.**
- [x] **Step 5: Commit.**

### Task 2: `EntityAvatar`

**Files:**
- Create: `frontend/src/components/EntityAvatar.tsx`, `frontend/src/components/EntityAvatar.test.tsx`

- [x] **Step 1: Write the failing tests:** renders `src`; falls back to initials on `onError`; falls back with no `src`; the colour is stable for a name.
- [x] **Step 2: Run them to verify they fail.** `cd frontend && npx vitest run src/components/EntityAvatar.test.tsx`
- [x] **Step 3: Write the minimal implementation:** an `<img onError>` when `src` is set and not failed, else initials over an FNV-1a-derived colour.
- [x] **Step 4: Run them to verify they pass.**
- [x] **Step 5: Commit.**

### Task 3: Replace the portrait sites

**Files:**
- Modify: `TurnSegments.tsx`, `theater/TheaterStage.tsx`, `theater/TheaterDialogue.tsx`, `ImageLightbox.tsx`, `ChronicleView.tsx`, `StoryTheater.tsx`
- Test: `StoryTheater.test.tsx`

- [x] **Step 1: Write the failing test** that the theater does not build a player portrait URL when the player has no portrait, and that a speaker with no portrait renders the fallback rather than an `<img>`.
- [x] **Step 2: Run it to verify it fails.**
- [x] **Step 3: Implement** each site with `EntityAvatar`, and drop the fabricated `/character/{playerId}/portrait` fallback in `StoryTheater`.
- [x] **Step 4: Run them to verify they pass**, plus `npx tsc --noEmit`.
- [x] **Step 5: Commit.**

### Task 4: A failure channel for audio

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`
- Test: `pkg/gui/turn_audio_test.go` or `streaming_tts_test.go`

- [x] **Step 1: Write the failing test** that a segment whose synthesis fails broadcasts an `AudioStatusDTO` with a non-empty `Error` and `Stage: "synthesize"`.
- [x] **Step 2: Run it to verify it fails.**
- [x] **Step 3: Implement:** add `Error string` and `Stage string` to `AudioStatusDTO`; in `emitTurnClips`, replace the two `continue`s on a synthesis error with a single broadcast of the first failure; broadcast `Stage: "synthesize"` for the grouped empty result.
- [x] **Step 4: Run it to verify it passes.**
- [x] **Step 5: Commit.**

### Task 5: The client surfaces it

**Files:**
- Modify: `frontend/src/types.ts`, `frontend/src/App.tsx`
- Test: `frontend/src/App` tests if present, else a focused handler test

- [x] **Step 1: Write the failing test** that an SSE `error` payload sets an error status, and that `onerror` before a terminal message is an error rather than success.
- [x] **Step 2: Run it to verify it fails.**
- [x] **Step 3: Implement:** `AudioStatus` type gains `error`/`stage`; `handlePlayTurnAudio` parses `error` and sets an error state, and its `source.onerror` and safety timeout set an error rather than silently finishing.
- [x] **Step 4: Run it to verify it passes**, plus `npx vitest run` and `npx tsc --noEmit`.
- [x] **Step 5: Commit.**

## Verification

- `go test ./...`, then `cd frontend && npx vitest run && npx tsc --noEmit`.
- `mise run lint`.
