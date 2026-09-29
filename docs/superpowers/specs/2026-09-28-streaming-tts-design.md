# Sentence-Level Streaming TTS Design

**Date:** 2026-09-28
**Status:** Proposed
**Scope:** Overlap speech synthesis with generation by splitting streamed prose into sentences and synthesizing each as it completes, reusing the existing cache keys so final segments are cache hits, with cost gating for metered providers
**Related:** `pkg/media` (`tts.go`, `cache.go`, `tts.go` policy), `pkg/entity` (`segment.go`), `pkg/dialogue`, `pkg/engine` (`orchestrator.go`, `segments.go`), `pkg/gui` (`service.go`, `types.go`), `frontend/src` (`useSegmentPlayback`, `StoryTheater`); implements the follow-up deferred in `docs/superpowers/specs/2026-09-26-turn-latency-reduction-design.md` and builds on `2026-09-22-tts-markdown-stripping-design.md`, `2026-09-22-voice-speech-cues-and-steering-design.md`, and `2026-09-26-theater-native-audio-and-visual-novel-layout-design.md`

## 1. Overview & Goals

Time-to-first-audio is currently bounded by the end of generation plus the time
to synthesize the first whole segment. TTS is per turn **segment**, sequential,
and whole-clip: `TTSPipeline.SynthesizeSegments` loops segments in order
(`pkg/media/tts.go:177`), each `SynthesizeSegment` returns a file path
(`:222`), and the pipeline never begins until the turn's segments exist.
Segments, in turn, only exist after the structured `submit_turn` tool call at the
end of generation (`pkg/engine/orchestrator.go:1665`, `:908-970`).

Meanwhile the LLM stream is already delivered incrementally: `onChunk func(string)
error` fires for every text delta (`orchestrator.go:1170,1277-1283`), and the GUI
forwards each as a `chunk` event (`pkg/gui/service.go:1399-1401`). The play queue
already accepts clips as they are produced (`Player.PlayQueue`,
`pkg/media/playback/player.go:182`; `PlayTurnAudio`, `service.go:1959`).

The latency design explicitly deferred this: *"Sentence-level streaming TTS across
providers (needs a streaming-capable TTS backend; recorded as a follow-up)"*
(`2026-09-26-turn-latency-reduction-design.md` §Goals) and lists it as an open
question (`§8`). It does **not** require streaming audio from the provider:
batch synthesis of individual sentences, overlapped with generation, is enough.

**Goals:**

- Begin synthesizing readable sentences while the model is still generating.
- Guarantee that a sentence synthesized mid-stream is the **same cache entry** the
  final segment will look up, so reconciliation is free.
- Keep the cache key a pure function of speaker, voice, prosody, and text.
- Never spend money on a metered provider without opt-in.
- Keep the current turn/segment contract and persistence untouched.

**Non-Goals:**

- Provider-streaming (chunked) audio; that is a separate capability and is
  recorded as Phase 2.
- Changing the segmentation contract (`TurnSegment`, `dialogue.Parse`,
  `buildSegments`).
- Changing playback, the theater beat model, or the audio cache format.
- Speech attribution beyond what the stream allows: mid-stream, the authoritative
  speaker map does not exist yet.

**Success Criteria:**

- During a turn, the first narrator sentence is synthesized before the final
  `turn` event, for a provider that streams text.
- A final narration segment whose text is a pre-synthesized sentence is a cache
  hit at finalize, so the common line-per-beat case makes no duplicate provider
  call.
- A metered TTS provider does not pre-synthesize unless
  `media.tts.stream_sentences` is explicitly enabled.
- Clip bytes and cache keys are byte-for-byte identical to the current
  non-streaming result for the same text and voice.

> **Implementation note (2026-09-28):** the follow-up in
> `2026-09-28-sentence-scoped-tts-clips-design.md` shipped, so a finished segment
> is now synthesized sentence by sentence and reuses the streamed clips; the
> concatenated segment clip keeps the one-clip contract. Reuse is total for plain
> prose. A client that consumes Markdown is still synthesized as a whole segment,
> because emphasis or audio tags may span a sentence boundary. The metered
> default remains off.

## 2. Investigation Findings

- **Whole-clip pipeline.** `SynthesizeSegments` (`tts.go:177`) →
  `SynthesizeSegment`/`SynthesizeSegmentForce` (`:222,227`) →
  `SynthesizeUtteranceForce` (`:302`). A clip path comes from `cachedClip`
  (`:422`). `prepareSegment` (`:197`) resolves speaker and voice and applies
  `SpeakableTextFor(policy, client, segment.Text)`; `CountUncached` (`:245`) uses
  the same path so counts and synthesis cannot disagree.
- **Cache keys are pure.** `ComputeAudioCacheKeyForVoice(speakerID, voice, text)`
  (`pkg/media/cache.go:29`) hashes provider/voice/prosody/options and text with a
  `v2:` prefix. A sentence is a hit iff the same speaker, voice, prosody, and
  **spoken** text are used — the reason the provisional path must apply the same
  `SpeakableTextFor` reduction.
- **Segment model.** `entity.TurnSegment` (`pkg/entity/segment.go:10-22`): `Kind`
  (`SegmentNarration`/`SegmentSpeech`, `:4-7`), `Speaker`, `SpeakerID`, `Text`,
  `CheckRef`, `Player`.
- **Heuristic attribution exists.** `dialogue.Parse(text, resolve) []Segment`
  (`pkg/dialogue/dialogue.go:31`) splits per-line narration/speech and leaves an
  unresolved candidate as narration (`:43-60`). It backs `buildTurnSegments`
  (`pkg/engine/segments.go:31`).
- **Streaming shape.** `ProcessActionStream` (`orchestrator.go:469`) forwards
  deltas through `onChunk func(string) error` (`:1170,1277-1283`). Tool calls are
  **not** parsed incrementally: `chunk.ToolCalls` replaces the prior value
  (`:1256-1257`) and arguments are parsed only after the stream completes
  (`ParseSubmission` `:1665`, `ParseCheckRequest` `:1642`). So mid-stream the
  engine knows prose but not segments or speakers.
- **GUI turn seam.** `TurnSession.Run` (`service.go:1387`): chunk events at
  `:1399-1401`; the authoritative `turn` event at `:1408-1411`; TTS trigger at
  `:1430-1440` (AutoPlay → `PlayTurnAudio`, else pre-synthesize each segment).
  Synthesis is intentionally detached from the request (`:1427-1429`).
- **Playback already streams clips.** `Player.PlayQueue(ctx, <-chan string)`
  (`player.go:182`) starts on the first clip; `PlayTurnAudio` feeds it in order
  (`service.go:1974-1991`). `useSegmentPlayback` and `StoryTheater` consume
  per-segment audio after the turn.
- **No TTS streaming capability.** `media.Capabilities` (`pkg/media/capabilities.go:8-15`)
  has no `Streaming` field; `ttsDerivableFeatures` (`pkg/provider/all/all_test.go:27-34`)
  omits it, so a TTS descriptor declaring `provider.FeatureStreaming` would fail
  its drift guard. `media.TTSClient` is whole-clip (`tts.go:94`).
- **Turn latency groundwork shipped.** `turnRuntime` caching, single-flight
  synthesis, and `PlayQueue` are in place (`pkg/gui/runtime.go`,
  `pkg/media/tts.go`, `player.go`), so this change builds on a faster baseline.

## 3. Design

### 3.1 Sentence splitting

Add a deterministic splitter that both the provisional stream and any future
consumer share:

```go
// pkg/media (or pkg/dialogue)
// SplitSentences returns the complete sentences in text, in order, using the
// same boundaries the spoken reduction would respect. A trailing fragment with
// no terminator is not returned until it is complete.
func SplitSentences(text string) []string
```

Rules: split on `. ! ? …` and paragraph breaks; do not split inside code spans,
fenced blocks, or common abbreviations; strip markdown the same way
`SpeakableTextFor` does before hashing. A table test pins the boundaries.

The earlier latency design already reduced markdown stripping for TTS
(`2026-09-22-tts-markdown-stripping-design.md`); reuse that reduction rather than
adding a second one.

### 3.2 Provisional synthesis during generation

Do this in the GUI turn session, which already owns the chunk callback, the TTS
pipeline, and the narrator voice. No orchestrator signature change is needed.

In `TurnSession.Run`, wrap the `onChunk` callback:

- Append each delta to a buffer.
- Extract newly complete sentences with `SplitSentences`.
- For each sentence, build a provisional `entity.TurnSegment{Kind: narration,
  SpeakerID: narratorSpeaker, Text: sentence}` and hand it to a bounded worker
  pool that calls a new pipeline entry point:

```go
// pkg/media
// SynthesizeProvisional synthesizes text with the same reduction, speaker, and
// voice the final segment will use, so the resulting cache entry is exactly the
// one SynthesizeSegment will look up. Returns ErrNoSpeakableText for text that
// reduces to nothing.
func (p *TTSPipeline) SynthesizeProvisional(ctx context.Context, kind, speakerID string, voice *entity.VoiceConfig, text string) (string, error)
```

`SynthesizeProvisional` reduces via the same `SpeakableTextFor` policy as
`prepareSegment` (factor that resolution so there is one implementation), then
calls `SynthesizeUtteranceForce` with the resolved voice. Because the cache key
does not encode the segment, only who reads it and what is spoken, a provisional
narration sentence is a hit for the final narration segment.

Mid-stream attribution is deliberately conservative: prose is provisionally
**narration**. Quoted dialogue is not pre-synthesized, because the speaker map
does not exist until `submit_turn` parses. Where `dialogue.Parse` can resolve a
speaker from an entity name already in the working set, a future iteration may
pre-synthesize speech too; the first iteration leaves it to finalize.

Concurrency is bounded (e.g. 2–4 in flight) and ordered by sentence index so the
play queue still receives clips in reading order. A failed sentence is dropped
from the provisional path; finalize retries it through the normal path.

### 3.3 Reconciliation at finalize

At `:1430-1440` today, `SynthesizeSegments` walks the authoritative segments.
Because provisional narration sentences share the cache key:

- A final narration segment whose text matches a provisional sentence is a cache
  hit (`cachedClip`, `tts.go:337,349`) — no provider call.
- A final speech segment or a narration segment not covered provisionally
  synthesizes normally.
- No segment is ever synthesized twice for the same key: single-flight
  (`tts.go` per-key lock) already serialises any overlap.

The turn record is unchanged: audio references stay out of `history.jsonl`, as
today.

### 3.4 Playback and UX

Phase 1 keeps the UI contract: the theater still starts on the `turn` event, but
by then narrator audio is already cached, so first-audio is near-instant and the
prefetch has little to do. This captures most of the latency win with no frontend
change.

Phase 2 (optional, separate): emit a provisional audio event (e.g.
`TurnEvent{Type: "segment_audio"}` or an `audio` event carrying a clip URL) as
each provisional clip lands, so the theater can begin before generation
finishes, then reconcile with the authoritative `turn` event. This needs a
segment index and a stable order, and is deferred.

### 3.5 Cost gating for metered providers

Pre-synthesis can waste spend when a provisional sentence is later re-voiced as
NCP dialogue, or when the reply is regenerated. Gate it:

```go
// config.Media.TTS
StreamSentences bool `yaml:"stream_sentences" json:"stream_sentences"`
```

- Default: **enabled** for providers whose descriptor does not carry
  `provider.FeatureMetered` (local, offline, self-hosted).
- Default: **disabled** for metered providers; enabling it is an explicit
  opt-in with a settings note about possible duplicate charges.
- A global off switch disables the whole path.

A provisional sentence that is never used still occupies one cache entry (free to
keep, bounded by the content cache's existing eviction).

### 3.6 Phase 2 — a real streaming capability (recorded, not required)

For true chunked audio, add `media.Capabilities.Streaming`, a drift-map entry in
`ttsDerivableFeatures`, and an optional
`StreamingTTSClient interface { SynthesizeStream(...) (io.ReadCloser, error) }`
implemented only where the provider supports it (Gemini, ElevenLabs). This is
explicitly out of scope for the first iteration; §3.1–3.5 deliver the win without
it.

## 4. Interfaces

```go
// pkg/media
func SplitSentences(text string) []string
func (p *TTSPipeline) SynthesizeProvisional(ctx context.Context, kind, speakerID string, voice *entity.VoiceConfig, text string) (string, error)

// pkg/config (media.tts)
StreamSentences bool
```

No HTTP route or `TurnEvent` change in Phase 1.

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| Splitter yields an empty sentence | Skipped (`ErrNoSpeakableText` path) |
| Provisional synthesis fails | Sentence dropped from the provisional path; finalize retries normally |
| Metered provider, streaming disabled | No provisional calls; unchanged behaviour |
| Text regenerated/recovered | Provisional clips may become dead cache entries; harmless, bounded |
| Turn cancelled or disconnected | Provisional work is cancelled with the turn context; nothing is persisted |
| Duplicate provisional and final keys | Single-flight returns the same clip; no double call |

## 6. Testing & Verification

Go (stdlib `testing`):

- `pkg/media`: `SplitSentences` table tests (terminators, abbreviations, fenced
  code, markdown, trailing fragment).
- `pkg/media`: `SynthesizeProvisional` for narration text produces the identical
  cache key a `SynthesizeSegment` with the same kind/speaker/text produces
  (assert via the fake client's call count: one call, then a cache hit).
- `pkg/gui`: a fake provider that streams N sentences and a fake TTS client that
  records call times; assert the first synthesis begins before the stream is
  complete and that finalize makes no duplicate call for narration.
- `pkg/gui`: a metered provider does not pre-synthesize unless
  `StreamSentences` is enabled.
- `pkg/media`: mid-stream failure does not fail the turn and finalize still
  produces all clips.

Frontend: `tsc` only (Phase 1 has no frontend change).

Manual/measured: time-to-first-audio before and after for a multi-sentence turn
with a local HTTP TTS provider, recorded in the trace.

## 7. Compatibility & Rollout

- Phase 1 is behaviour-preserving for audio output: same cache keys, same bytes.
- `StreamSentences` defaults preserve current behaviour for metered providers.
- The splitter is new shared code; the reduction used for hashing must match
  `SpeakableTextFor` exactly, or tokens already cached will miss. The
  cache-key-equality test is the guard.
- Removing/avoiding a frontend change keeps the rollout small; Phase 2 is
  independently specified when wanted.

## 8. Open Questions

- Should quoted dialogue be pre-synthesized when the speaker resolves from the
  working set, accepting occasional waste?
- Is a bounded worker pool the right concurrency, or should it be one in-flight
  sentence to keep ordering trivial and cost predictable?
- Do we need a per-provider override for `StreamSentences`, or is the metered
  feature flag enough?
- Should the provisional path also drive the play queue directly, so narration
  can start before `submit_turn`?
- Does the content cache need an explicit "unused provisional clips" sweep, or
  is its existing eviction sufficient?

## 9. References

- Code: `pkg/media/tts.go:177-422`, `pkg/media/cache.go:14-64`,
  `pkg/entity/segment.go:4-22`, `pkg/dialogue/dialogue.go:31-60`,
  `pkg/engine/orchestrator.go:469,1170-1283,1358-1693`,
  `pkg/engine/segments.go:31`, `pkg/gui/service.go:1387-1440,1959-1991`,
  `pkg/media/playback/player.go:182-193`,
  `pkg/media/capabilities.go:8-15`, `pkg/provider/all/all_test.go:27-34`.
- Specs: `2026-09-26-turn-latency-reduction-design.md` (§Goals, §8),
  `2026-09-22-tts-markdown-stripping-design.md`,
  `2026-09-22-voice-speech-cues-and-steering-design.md`,
  `2026-09-26-theater-native-audio-and-visual-novel-layout-design.md`.
- Research: `docs/proposals/2026-09-26-turn-latency-research.md` (recommendation
  13, open questions).
