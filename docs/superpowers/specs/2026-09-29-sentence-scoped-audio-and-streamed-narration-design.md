# Sentence-Scoped Audio and Streamed Narration Design

**Date:** 2026-09-29
**Status:** Proposed
**Scope:** Make a clip and its key the same unit everywhere — one sentence of reduced text, or the whole text when splitting is unsafe — so a segment's audio is an ordered list of clips, streaming playback needs no heuristic, and the concatenation path disappears
**Supersedes:** the concatenation approach in `2026-09-28-sentence-scoped-tts-clips-design.md` (its §3.1 and §3.3), and the hybrid dedupe in `2026-09-29-streamed-narration-audio-design.md` (§3.4)
**Related:** `pkg/media` (`tts.go`, `cache.go`, `sentence.go`), `pkg/gui` (`service.go`, `streaming_tts.go`, `server.go`, `types.go`), `pkg/scene`, `pkg/export`, `frontend/src`

## 1. Overview & Goals

Two designs met in the middle and disagreed about scope. Sentence-scoped synthesis
split a segment, synthesized each sentence (reusing clips the sentence streamer
had already written), then **concatenated** them into a second clip under a
segment key. Streamed playback then had to relate sentence clips to that segment
clip with a heuristic, because one sound had two keys.

**A key must name exactly the audio it produces.** This design picks the unit that
makes that true and keeps it everywhere: the synthesis unit is a sentence of
reduced text (or the whole reduced text when splitting would corrupt Markdown).
A clip is that unit's audio; its key is that unit's key. A segment's audio is the
ordered list of its clips.

**Goals:**

- One key per clip, one clip per unit, everywhere (cache, DTO, playback, export).
- Delete concatenation and every heuristic that existed to relate scopes.
- Play narration while the turn streams, with exact reuse and no repeats.
- Keep dialogue out of the mid-stream path (speakers are unknown until
  `submit_turn`).

**Non-Goals:**

- Provider-side streaming audio.
- Per-sentence UI controls; a segment remains the user-visible beat.
- Changing the turn, timeline, or theater beat model.
- Splitting a Markdown-consuming client's text (unchanged: its segment's list has
  one clip).

**Success Criteria:**

- For any segment, `len(clips) == len(units)` and every clip's file name is the
  key computed from that unit's speaker, voice, and text.
- A segment synthesized at finalise reuses every clip the streamer produced for
  it, for multi-sentence segments as well as single-sentence ones.
- Server-side playback starts on the first streamed sentence and never plays a
  clip twice in a turn.
- Export and the video render consume the same clip lists the app plays.
- `go test ./...` and `npx tsc --noEmit` are green; concatenation code is gone.

## 2. Investigation Findings

Callers of the single-clip contract, all of which change:

| Caller | Today | Under this design |
| --- | --- | --- |
| `TTSPipeline.SynthesizeSegment*` (`pkg/media/tts.go:222,241`) | one path, concatenating multi-sentence segments | `SynthesizeSegmentClips(...) ([]string, error)` |
| `TTSPipeline.SynthesizeSegments` (`:177`) | `[]string`, one per segment | `[][]string`, one list per segment |
| `Service.GetSegmentAudio` (`pkg/gui/service.go:1822`) | one path | `GetSegmentClips` returns the list |
| `PlaySegmentAudio` (`:2032`), `PlayTurnAudio` (`:2015`) | one clip per segment | a clip stream, flattened |
| warm-up loop (`:1474`) | one clip per segment | the same list, ignored |
| `scene.SpeechResolver.SegmentAudio` (`pkg/scene/compile.go:46`) | one path + duration | `([]string, time.Duration, error)` |
| `scene.Beat` (`pkg/scene/scene.go:31`) | `AudioPath string` | `AudioPaths []string` |
| `export` web (`web.go:88`) and video (`video.go:152`) | one file per beat | one file per clip |
| frontend | `segment.audio_url?: string` | `segment.audio_urls?: string[]` |

- `segmentDTOs` already computes a per-segment `AudioKey` with an empty speaker
  reference for narration, while the pipeline namespaces narration as `narrator`
  (`pkg/gui/service.go`): the value token and the served clip disagree today.
  Content-addressed URLs remove the mismatch rather than fix it.
- `sentenceStreamer` workers discard the clip path
  (`pkg/gui/streaming_tts.go`), so the clips streaming produces are never played.
- Audio routes are mounted at `/api/audio/` (`handleAudioRoutes`,
  `pkg/gui/server.go:1103`) with `status` and `stop`; a clip route fits there.
- `PlayQueue(ctx, <-chan string)` already consumes a stream of clips and ends when
  the channel closes (`pkg/media/playback/player.go`), so a segment being several
  clips costs nothing at the player.

## 3. Design

### 3.1 One unit, one clip, one key

`clipUnits(segment)` resolves a segment to its reduced texts:

- `prepareSegment` (as today) yields the speaker, voice, and reduced text.
- The reduced text is split with `SplitSentences` unless the client receives
  Markdown (`markdownPreserved`), in which case the unit is the whole text.
- Each unit is synthesized with `SynthesizeUtteranceForce`, whose key is
  `ComputeAudioCacheKeyForVoice(speakerID, voice, unit)` — a clip per unit.

```go
// pkg/media
// SynthesizeSegmentClips renders a segment as one clip per unit, in order. Each
// clip's key is the key of the unit it reads, so a clip is exactly the audio one
// unit produces.
func (p *TTSPipeline) SynthesizeSegmentClips(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig, force bool) ([]string, error)

// SegmentClipKeys reports the keys those clips will have, without synthesizing.
func (p *TTSPipeline) SegmentClipKeys(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) ([]string, error)

// SynthesizeSegments renders every segment as a list of clips.
func (p *TTSPipeline) SynthesizeSegments(ctx context.Context, segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) ([][]string, error)
```

`SynthesizeProvisional` stays: it is `SynthesizeSegmentClips` for a one-unit
synthetic segment, which is what a streamed sentence is.

**Deleted:** `concatenateSentences` and its `opus.Decode`/`opus.Encode` path.
Nothing concatenates audio any more; multi-clip is the representation.

### 3.2 Clips are content-addressed

A clip is served by its key, so no turn or segment index is needed to name one:

- `GET /api/audio/clip/{key}`: validates 64 lowercase hex characters, resolves the
  file inside the audio cache, and serves it as `audio/ogg` with an immutable
  cache header. Malformed or unknown keys are 404. No client path is accepted.
- `SegmentDTO.audio_urls` is the ordered list of `/api/audio/clip/{key}` URLs for a
  segment. `audio_url` and `audio_key` are removed: one sound, one name.
- Regeneration (the "force" case) re-synthesizes a segment's units:
  `POST /api/game/{id}/turn/{n}/segment/{i}/audio` returns the refreshed
  `audio_urls`. Playback never needs it; the speaker chips use it.

### 3.3 Streaming: the same unit, so reuse is exact

`sentenceStreamer` emits what it synthesizes:

```go
// pkg/gui
type provisionalSpeech struct {
    Index    int    // unit ordinal within the turn, monotonic
    Text     string
    AudioKey string
    AudioURL string
}
```

- With `auto_play` and an available player, the turn session opens a buffered
  queue channel before generation and the streamer appends clip paths in
  emission order. After the turn is recorded, the session appends every clip of
  every segment **whose path is not already in the played set** and closes the
  channel.
- Because a unit is a clip, that set is exact: a unit that was played is skipped,
  a unit that failed mid-stream is synthesized and played at finalise, and no unit
  can be heard twice or missed. No partial-coverage case exists.
- Events are emitted for a client-authority session; the client plays them while
  the prose streams and applies the same played-set rule when the `turn` event
  arrives.

`TurnEvent` gains `index`, `text`, `audio_key`, `audio_url` for `type: "speech"`.

### 3.4 Export

- `scene.SpeechResolver.SegmentAudio` becomes
  `SegmentAudio(ctx, segment) ([]string, time.Duration, error)`, and
  `scene.Beat.AudioPaths []string`; `AudioDuration` is the sum of the clip
  durations (0 when probing fails, as today).
- Web bundle: `webBeat.Audio []string`; the embedded player plays a beat's clips
  in order before advancing.
- Video: `BuildCommand` adds one input per clip (and `anullsrc` for a clip-less
  beat), so `concat` receives one stream per unit — finer pacing than one stream
  per beat.

### 3.5 What this removes

- The concatenated segment clip and its cache entry.
- `sentence_audio_keys` and the partial-coverage rule from the previous design.
- The `AudioKey`/`audio_url` duplication on the DTO.
- The narration `AudioKey` namespace mismatch (keys come from the pipeline).

## 4. Interfaces

```go
// pkg/media
func (p *TTSPipeline) SynthesizeSegmentClips(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig, force bool) ([]string, error)
func (p *TTSPipeline) SegmentClipKeys(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) ([]string, error)
func (p *TTSPipeline) SynthesizeSegments(ctx context.Context, segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) ([][]string, error)

// pkg/gui
type provisionalSpeech struct{ Index int; Text, AudioKey, AudioURL string }
func newSentenceStreamer(ctx context.Context, pipeline *media.TTSPipeline, voice *entity.VoiceConfig, logger trace.Logger, workers int, emit func(provisionalSpeech)) *sentenceStreamer
func (s *Service) GetSegmentClips(ctx context.Context, gameID string, turnNumber, segmentIndex int, force ...bool) ([]string, error)
// SegmentDTO: AudioURLs []string `json:"audio_urls,omitempty"`
// TurnEvent:  Index int, AudioKey, AudioURL string

// pkg/scene
type SpeechResolver interface { SegmentAudio(ctx context.Context, segment entity.TurnSegment) ([]string, time.Duration, error) }
// Beat: AudioPaths []string

// pkg/export
// webBeat: Audio []string
```

HTTP: `GET /api/audio/clip/{key}`,
`POST /api/game/{id}/turn/{n}/segment/{i}/audio` (regenerate; returns URLs).

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| A unit reduces to nothing | Skipped; its neighbours still play |
| One unit fails to synthesize | That clip is absent; others play; finalise retries it |
| Provisional sentence fails | No event, no queue entry; finalise synthesizes it |
| Clip key unknown or malformed | 404; the client skips that URL |
| Probing a clip's duration fails | Duration 0; the beat falls back to the reading estimate |
| Player unavailable | No queue is opened; events still let a client authority play |
| Client disconnects | Turn cancelled as today; queue closed |
| Emit from a worker | Serialised by a mutex owned by the turn session |

## 6. Testing & Verification

Go (stdlib `testing`):

- `pkg/media`: `SegmentClipKeys` names the files `SynthesizeSegmentClips`
  produces, one per unit, in order; a multi-sentence segment yields several clips
  and a Markdown-preserving client yields one.
- `pkg/media`: streaming a sentence then finalising the segment that contains it
  reuses the clip (no second provider call), for a multi-sentence segment.
- `pkg/media`: no concatenated clip appears; every produced file's basename is the
  key of its unit.
- `pkg/gui`: the streamer emits one ordered item per completed sentence and none
  for a partial tail.
- `pkg/gui`: `GET /api/audio/clip/{key}` serves a stored clip and 404s unknown and
  malformed keys (including traversal attempts).
- `pkg/gui`: the played-set skips exactly the clips already sent to the queue.
- `pkg/scene`/`pkg/export`: a two-clip beat renders two video inputs, sums its
  durations, and the web payload lists both.
- Frontend: `tsc`; `audio_urls` consumed by the chronicle and theater.

Manual: replay the Pallid Court opening; narration begins while text arrives and
no line repeats.

## 7. Compatibility & Rollout

- `index.db` and Markdown are untouched; only the audio cache shape changes.
- Single-sentence segments keep their exact keys, so their clips stay valid.
  Concatenated clips become orphans; the cache is disposable.
- `history.jsonl` is untouched: no audio paths are recorded there.
- A client that ignores `speech` events still plays on `turn`, now from
  `audio_urls`.
- Export artefacts change layout (one audio file per unit); nothing reads them
  back.

## 8. Open Questions

- Should the client show a "streaming audio" state distinct from buffering?
- Should the theater advance beats on streamed audio, or keep advancing on the
  authoritative segments after `turn`?
- Does the dialogue path want the same treatment once speaker attribution can be
  streamed (a tool-call segment stream)?
- Should clip URLs carry the voice/prosody as a version token, or is the content
  key alone enough (it hashes them)?

## 9. References

- Code: `pkg/media/tts.go`, `pkg/media/cache.go`, `pkg/media/sentence.go`,
  `pkg/gui/service.go`, `pkg/gui/streaming_tts.go`, `pkg/gui/server.go`,
  `pkg/gui/types.go`, `pkg/scene/{scene,compile,timing}.go`,
  `pkg/export/{script,web,video}.go`, `frontend/src/{App.tsx,types.ts}`,
  `frontend/src/hooks/useSegmentPlayback.ts`,
  `frontend/src/components/{TurnSegments,StoryTheater}.tsx`.
- Specs: `2026-09-28-streaming-tts-design.md`,
  `2026-09-28-sentence-scoped-tts-clips-design.md`,
  `2026-09-29-streamed-narration-audio-design.md`.
