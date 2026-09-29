# Sentence-Scoped TTS Clips Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-28.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Synthesize every segment sentence by sentence, so the streamed sentence clips are reused by the finaliser and the segment still resolves to one playable clip.

**Architecture:** `SynthesizeSegmentForce` splits the reduced text, synthesizes each sentence through the existing per-utterance path (the cache keys the streaming path already wrote), and concatenates the resulting Ogg/Opus clips with `opus.Decode`/`opus.Encode` into the segment-keyed clip. The single-path contract is unchanged.

**Tech Stack:** Go 1.27 stdlib tests.

**Spec:** `docs/superpowers/specs/2026-09-28-sentence-scoped-tts-clips-design.md`

## Global Constraints

- Use `interface{}`, never `any`; `go vet` clean.
- Standard library only in tests.
- One clip per segment: the GUI, export, and video mux must not change.
- **Deviation from the spec:** a Markdown-consuming client is never split (the
  spec allowed a per-sentence balance scan). This is the conservative rule: it
  can only cost a reuse opportunity, never corrupt markup.

---

### Task 1: Sentence-scoped synthesis

**Files:** `pkg/media/tts.go`; test `pkg/media/sentence_test.go`.

- [x] **Step 1:** In `SynthesizeSegmentForce`, after `prepareSegment`, split the
  reduced text with `sentencesFor`. One sentence takes the existing direct path.
- [x] **Step 2:** For several sentences, check the segment-keyed clip first, then
  synthesize each sentence via `SynthesizeUtteranceForce`, then
  `concatenateSentences`.
- [x] **Step 3:** `concatenateSentences` decodes each clip with `opus.Decode`,
  appends the PCM, encodes once with `opus.Encode`, and stores the result under
  the segment key (single-flight on that key).
- [x] **Step 4:** `sentencesFor`/`markdownPreserved` keep the whole segment when
  the client receives Markdown.
- [x] **Step 5:** Tests: multi-sentence reuse (2 calls, second read cached),
  single-sentence direct path, provisional-to-final reuse, a Markdown-aware client
  not split, and evicting the segment clip re-concatenating from sentence clips
  with no provider call.
- [x] **Step 6:** `go test ./pkg/media/`.

---

### Task 2: Update the streaming spec's limitation note

**Files:** `docs/superpowers/specs/2026-09-28-streaming-tts-design.md`.

- [x] **Step 1:** Record that reuse is now total for plain prose, with the
  Markdown-aware caveat.

---

### Task 3: Full verification

- [x] **Step 1:** `go build ./...`, `go test ./...`, `go vet ./...`.
- [x] **Step 2:** `cd frontend && npx tsc --noEmit` (no frontend change, sanity).
