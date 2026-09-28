# Streaming TTS Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-28.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Overlap speech synthesis with generation by splitting streamed prose into sentences and synthesizing each as it completes, so the common single-sentence segment is already cached when the turn finalises.

**Architecture:** `pkg/media` gains a sentence splitter and `TTSPipeline.SynthesizeProvisional`, which builds the same synthetic segment the finaliser will, so the cache key matches. `TurnSession.Run` feeds streamed chunks into a `sentenceStreamer` with a bounded worker pool. Cost gating rides on a new `media.tts.stream_sentences` option.

**Tech Stack:** Go 1.27 stdlib tests.

**Spec:** `docs/superpowers/specs/2026-09-28-streaming-tts-design.md`

## Global Constraints

- Use `interface{}`, never `any`; `go vet` clean.
- Standard library only in tests.
- Generation is never blocked: the streamer drops a sentence when its queue is full, and the finalise path synthesizes it normally.
- No frontend change: the theatre still starts on the `turn` event.
- **Documented limitation:** final TTS is per turn *segment*, and a segment's text may contain several sentences. A provisional sentence is therefore a cache hit only when the finished segment's text is that same sentence (the common case for line-per-beat narration). A multi-sentence segment re-synthesizes and its provisional clips are unused cache entries; this is why metered providers default the feature off.

---

### Task 1: Sentence splitter

**Files:** `pkg/media/sentence.go`; test `pkg/media/sentence_test.go`.

- [x] **Step 1:** `SplitSentences(text) []string` and
  `SplitCompleteSentences(text) (complete []string, remainder string)`. Boundaries
  are `. ! ? …` followed by whitespace/end (or closing punctuation then
  whitespace/end) and newlines, skipping inline code spans and common
  abbreviations, with the closing punctuation absorbed into the sentence it ends.
- [x] **Step 2:** Table test: terminators, abbreviations (`Dr.`), decimals
  (`3.14`), code spans, newlines, closing quotes, and the held tail.
- [x] **Step 3:** `go test ./pkg/media/`.

---

### Task 2: Provisional synthesis and cache-key parity

**Files:** `pkg/media/tts.go`; test `pkg/media/sentence_test.go`.

- [x] **Step 1:** `SynthesizeProvisional(ctx, kind, speakerID, text, voice)`
  builds a synthetic segment and delegates to `SynthesizeSegmentForce`, so the
  same `SpeakableTextFor` reduction and the same cache key apply.
- [x] **Step 2:** Test: provisional narration then the identical final narration
  segment synthesizes once (the second is a cache hit).

---

### Task 3: Cost gating

**Files:** `pkg/config/types.go`.

- [x] **Step 1:** `TTSConfig.StreamSentences *bool` and
  `Config.TTSStreamSentences()`: nil means enabled, except when the operator
  marked the provider metered.
- [x] **Step 2:** `go test ./pkg/config/`.

---

### Task 4: GUI wiring

**Files:** `pkg/gui/streaming_tts.go`; `pkg/gui/service.go` (`TurnSession.Run`).

- [x] **Step 1:** `sentenceStreamer` with a bounded sentence queue and worker
  pool; `Feed` splits and enqueues without blocking, `Close` drains.
- [x] **Step 2:** `Service.sentenceStreamerFor` returns one only when
  `TTSStreamSentences()` is on and a provider is configured, else nil (nil-safe).
- [x] **Step 3:** `TurnSession.Run` feeds each streamed chunk after emitting it,
  and closes the streamer before the turn returns.
- [x] **Step 4:** Test: complete sentences are synthesized, a partial tail is
  held, and a nil streamer is safe.

---

### Task 5: Full verification

- [x] **Step 1:** `go build ./...`, `go test ./...`, `go vet ./...`.
- [x] **Step 2:** `go test ./pkg/gui -update-docs` (new config key), then re-run
  the suite.
