# Sentence-Scoped TTS Clips Design

**Date:** 2026-09-28
**Status:** Implemented (2026-09-28). Deviation: a Markdown-consuming client is
never split, rather than undergoing the per-sentence balance scan; this can only
forgo a reuse opportunity, never corrupt markup.
**Scope:** Make per-sentence synthesis the unit of caching end to end, so streamed sentence clips are reused by the finalised segment and no provider call is wasted
**Related:** `pkg/media` (`tts.go`, `sentence.go`, `opus`), `pkg/gui` (`service.go`, `streaming_tts.go`, `types.go`), `pkg/export` (`script.go`), `pkg/media/playback`; follows `docs/superpowers/specs/2026-09-28-streaming-tts-design.md`

## 1. Overview & Goals

The shipped streaming TTS synthesizes streamed prose per *sentence* but the turn
finaliser synthesizes per *segment* (`TTSPipeline.SynthesizeSegmentForce`,
`pkg/media/tts.go`). Because the audio cache key is a pure function of speaker,
voice, prosody, and text, a provisional sentence clip is reused only when the
finished segment's text is exactly that sentence. A segment containing several
sentences therefore re-synthesizes the whole segment, and the provisional clips
are wasted cache entries. This was recorded as the documented limitation of the
streaming spec.

**Goals:**

- Make the sentence the caching unit for both the provisional and the final path,
  so every provisional clip is reused.
- Keep the external contract unchanged: a segment still resolves to one playable
  clip, so the GUI, export, and video mux need no change.
- Never regress audio for a Markdown-aware provider whose markup spans a
  sentence boundary.
- Keep the concatenation cost off the generation path and cache its result.

**Non-Goals:**

- Streaming (chunked) audio from a provider; still the Phase 2 of the streaming
  spec.
- Changing the segment model (`entity.TurnSegment`) or the chronicle DTOs.
- Per-sentence playback UI.

**Success Criteria:**

- For a multi-sentence narration segment, the finalise path makes no provider
  call for a sentence that was already synthesized provisionally.
- The segment still yields one clip path whose bytes decode and play as the
  concatenation of its sentences, with a correct probed duration.
- A second read of the same segment is served from the segment-level cache.
- A segment that is one sentence takes the existing single-clip path unchanged.
- A provider that consumes Markdown does not have markup split across clips.

## 2. Investigation Findings

- `TTSPipeline.SynthesizeSegmentForce` (`tts.go:227`) resolves the speaker/voice
  and the reduced text through `prepareSegment`, then calls
  `SynthesizeUtteranceForce` (`tts.go:302`) with the **whole segment text**, which
  is the cache key (`ComputeAudioCacheKeyForVoice`, `pkg/media/cache.go:29`).
- The streaming path (`pkg/gui/streaming_tts.go`) calls
  `SynthesizeProvisional` (`tts.go`), which builds a synthetic narration segment
  and delegates to `SynthesizeSegmentForce`; its cache key is the sentence text.
- `media.SplitSentences` / `SplitCompleteSentences` (`pkg/media/sentence.go`)
  already split prose deterministically, skipping inline code spans,
  abbreviations, and decimals.
- Clips are stored as Ogg/Opus only (`audioExtensions`), and the codec is
  available in-process: `opus.Decode(data) ([]int16, rate, channels, error)` and
  `opus.Encode(pcm, rate, channels, bitrate)` (`pkg/media/opus`). The player
  decodes the same way (`pkg/media/playback/player.go:342`).
- A segment's audio is exposed as one path today: `SegmentDTO.AudioURL` is built
  in `pkg/gui/service.go` (`/api/game/{id}/turn/{n}/segment/{i}/audio`),
  `GetSegmentAudio` calls `SynthesizeSegmentForce`, the exporter's
  `speechResolver.SegmentAudio` (`pkg/export/script.go`) returns one path and a
  probed duration, and the video pipeline takes one audio input per beat.
- Markdown-aware clients are sent raw text (`SpeakableTextFor` with
  `TextPolicyKeep` or a `MarkdownAware` client); emphasis or audio tags can span
  what the splitter sees as a sentence boundary.

## 3. Design

### 3.1 Split at synthesis, concatenate at the boundary

`SynthesizeSegmentForce` becomes:

1. Resolve speaker/voice and the reduced text exactly as today.
2. If the segment reduces to one sentence, use the existing single-clip path
   (no concatenation, no extra cache entry).
3. Otherwise synthesize each sentence through `SynthesizeUtteranceForce` with the
   same speaker and voice — the keys the provisional path already wrote — and
   concatenate the resulting clips into one segment clip stored under the
   segment key.

```go
// pkg/media/tts.go (shape)
func (p *TTSPipeline) SynthesizeSegmentForce(ctx, segment, narratorVoice, voiceFor, force) (string, error) {
    speakerID, voice, spoken := p.prepareSegment(segment, narratorVoice, voiceFor)
    if strings.TrimSpace(spoken) == "" { return "", ErrNoSpeakableText }

    if base, ok := p.cachedClip(ComputeAudioCacheKeyForVoice(speakerID, voice, segment.Text)); ok && !force {
        return base, nil // the concatenated segment clip is cached
    }

    sentences := p.sentencesFor(spoken)          // see 3.2
    if len(sentences) <= 1 {
        return p.SynthesizeUtteranceForce(ctx, speakerID, voice, spoken, force)
    }

    clips := make([]string, 0, len(sentences))
    for _, sentence := range sentences {
        clip, err := p.SynthesizeUtteranceForce(ctx, speakerID, voice, sentence, force)
        if err != nil { return "", err }
        clips = append(clips, clip)
    }
    return p.concatenate(ctx, speakerID, voice, segment.Text, clips)
}
```

`concatenate` reads each Opus clip, `opus.Decode`s it, appends the PCM, encodes
once with `opus.Encode`, and writes the result under the segment key via
`p.cache.Put("audio", base+".opus", bytes)`.

### 3.2 Splitting rules that preserve reuse and markup

`sentencesFor(reduced string)` returns `media.SplitSentences(reduced)` when it is
safe to split, and `[]string{reduced}` otherwise:

- **Safe** when the provider is not Markdown-aware under the active policy, i.e.
  the text is already plain. Splitting plain prose is loss-free.
- **Safe** when the Markdown-aware text has balanced inline markup within every
  sentence — no unclosed `*`, `_`, `` ` ``, or audio tag spans a boundary.
- **Unsafe** otherwise, so a markup-aware provider keeps the whole-segment path
  and never receives half an emphasis span.

The safety check is a small scan of the reduced text: track fenced/inline code
and emphasis nesting; a candidate split is only allowed where the stack is empty.

The provisional path must use the same rule. Because it only ever splits plain
streamed prose (narration before segmentation), it can keep using
`SplitCompleteSentences`; the finalise path reaching a different decision only
means that segment falls back to whole-segment synthesis, which is correct, just
not cached — the same failure mode as today, never worse.

### 3.3 Cache layers

- **Sentence clips** — one per sentence, keyed by `(speaker, voice, sentence)`.
  These are the provisional outputs and the concatenation inputs.
- **Segment clip** — the concatenation, keyed by `(speaker, voice, segment
  text)`. Written once so repeat reads and later exports do not re-concatenate.

Both live in the existing content cache, so eviction is unchanged. A segment clip
is additive: deleting it re-concatenates from the sentence clips, which may
themselves have been evicted, in which case sentences are re-synthesized.

### 3.4 Cost and concurrency

- Concatenation is CPU only (decode N clips, encode once) and happens once per
  segment, on the background finalise path, not inside the model stream.
- Single-flight already serialises per key (`tts.go` `keyLock`); the segment key
  gets its own lock, and per-sentence keys keep theirs, so a concurrent segment
  request and a provisional sentence request cannot race.
- Metered gating is unchanged: `media.tts.stream_sentences` still governs whether
  provisional (and therefore multi-sentence) synthesis happens ahead of the turn.

### 3.5 Compatible fallbacks

- Export and the video pipeline keep taking one path per beat and probe its
  duration; a concatenated clip probes correctly because it is a normal Ogg/Opus
  file.
- Legacy turns with no segments are unaffected (they are parsed into segments
  before synthesis).

## 4. Interfaces

```go
// pkg/media
func (p *TTSPipeline) concatenate(ctx context.Context, speakerID string, voice *entity.VoiceConfig, segmentText string, clips []string) (string, error)
func (p *TTSPipeline) sentencesFor(reduced string) []string
// optional, exported for callers that want the parts:
func (p *TTSPipeline) SynthesizeSegmentSentences(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) ([]string, error)
// pkg/media/opus already provides Decode/Encode; no new surface needed.
```

`SynthesizeSegment*` keep returning a single path, so `pkg/gui`, `pkg/export`,
and playback do not change.

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| One sentence | Existing single-clip path; no concatenation |
| A sentence clip fails | Segment fails as today; the turn is not lost (TTS degrade path) |
| Decode of a stored sentence clip fails | Drop that clip and re-synthesize the sentence once |
| Encode fails | Return a `GenerationFailure`; nothing is published under the segment key |
| Markup spans a boundary | No split; whole-segment synthesis |
| Segment clip evicted | Re-concatenate from sentence clips, or re-synthesize the missing ones |

## 6. Testing & Verification

Go (stdlib `testing`, `t.TempDir()`):

- `pkg/media`: a two-sentence segment with a recording client synthesizes **two**
  sentence clips; a second call to `SynthesizeSegment` makes **no** new provider
  call (segment cache hit).
- `pkg/media`: a provisional sentence followed by the matching multi-sentence
  segment reuses the provisional clip (one fewer provider call than sentences).
- `pkg/media`: a one-sentence segment produces exactly one clip and no
  concatenated duplicate.
- `pkg/media`: a Markdown-aware client with markup spanning a boundary is not
  split; a balanced-markup text is split.
- `pkg/media`: the concatenated clip's probed duration is within a frame of the
  summed sentence durations, and its bytes decode as Ogg/Opus.
- `pkg/media`: a decode failure on one stored clip re-synthesizes only that
  sentence.
- `pkg/gui`/`pkg/export`: existing segment-audio tests still pass unchanged
  (single-path contract preserved).

## 7. Compatibility & Rollout

- No API, DTO, or frontend change; the contract stays one clip per segment.
- Existing single-sentence segments are byte-identical to today.
- Multi-sentence segments gain a concatenation step the first time; steady-state
  reads hit the segment cache.
- Cache keys are unchanged, so existing caches remain valid; the extra segment
  entry is additive.
- The streaming spec's limitation note can be removed once this lands.

## 8. Open Questions

- Is decode-and-re-encode the right concatenation, or should the player accept an
  ordered clip list (no re-encode, more consumer changes)?
- Should the concatenated segment clip be written at all, or only the sentence
  clips (saving disk, costing a concat per read)?
- Is the markup-safety scan worth its complexity, or should a Markdown-aware
  provider always take the whole-segment path?
- Should sentence clips be namespaced so a segment-level eviction can cascade to
  them?
- Does the video mux benefit from per-sentence inputs (finer timing) or is the
  concatenated clip sufficient?

## 9. References

- Code: `pkg/media/tts.go` (`SynthesizeSegmentForce`, `SynthesizeUtteranceForce`,
  `prepareSegment`, `keyLock`), `pkg/media/sentence.go`,
  `pkg/media/opus/opus.go` (`Decode`, `Encode`), `pkg/media/cache.go`,
  `pkg/gui/streaming_tts.go`, `pkg/gui/service.go` (`GetSegmentAudio`,
  `SegmentDTO.AudioURL`), `pkg/export/script.go` (`speechResolver`),
  `pkg/media/playback/player.go`.
- Specs: `2026-09-28-streaming-tts-design.md` (limitation note),
  `2026-09-22-tts-markdown-stripping-design.md`,
  `2026-09-23-voice-speech-cues-and-steering-design.md`.
