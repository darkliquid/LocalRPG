# Streamed Narration Audio Design

**Date:** 2026-09-29
**Status:** Proposed
**Scope:** Play a turn's narration as it streams: feed the process player the sentence clips the sentence streamer already produces, and publish those clips to the client as `speech` events so a browser-authority client can do the same
**Related:** `pkg/media` (`tts.go`, `cache.go`, `sentence.go`), `pkg/gui` (`service.go`, `streaming_tts.go`, `server.go`, `types.go`), `frontend/src` (`App.tsx`, `api/client.ts`, `hooks/useSegmentPlayback.ts`, `types.ts`); implements Phase 2 of `docs/superpowers/specs/2026-09-28-streaming-tts-design.md` and depends on `2026-09-28-sentence-scoped-tts-clips-design.md`

## 1. Overview & Goals

Synthesis already overlaps generation: `sentenceStreamer` (`pkg/gui/streaming_tts.go`)
synthesizes each completed sentence while the model is still writing, and the
clip paths land in the content cache. Nothing plays them. Playback still starts
only after the `turn` event, so a turn is silent while it is being written and
then speaks at once.

**Goals:**

- Start narration playback while the turn is still streaming, for the process
  player (the authority on a machine with an audio device) and, via events, for a
  browser-authority client.
- Never play the same narration twice when the authoritative turn arrives.
- Never block or fail generation because audio is slow or unavailable.
- Keep dialogue out of the mid-stream path: speakers are only known once
  `submit_turn` is parsed.

**Non-Goals:**

- Provider-side streaming audio.
- Per-sentence playback of dialogue, or a per-sentence UI.
- Changing the theater's beat model or the segment contract.
- Replacing the existing on-demand segment audio endpoints.

**Success Criteria:**

- With `media.tts.auto_play` on and a usable device, the first narration sentence
  is audible before the turn's `turn` event arrives.
- No sentence is heard twice: a narration segment whose sentences were all played
  mid-stream is not played again at finalise.
- A client that owns playback receives one ordered `speech` event per sentence
  with a clip URL that resolves, and reaches parity on the same no-repeat rule.
- `go test ./...` covers the streamer's events, the clip endpoint, and the
  no-repeat selection; `tsc` stays green.

## 2. Investigation Findings

- `sentenceStreamer` (`pkg/gui/streaming_tts.go`) queue-fed workers call
  `pipeline.SynthesizeProvisional(...)` and **discard the returned path**. The
  clip is already in the cache under `ComputeAudioCacheKeyForVoice`.
- `TurnSession.Run` (`pkg/gui/service.go`) starts audio only after the turn is
  recorded: `PlayTurnAudio` (whole turn) or a per-segment pre-synthesis loop, both
  in `goBackground`.
- `sentenceStreamerFor` gates on `Config.TTSStreamSentences()`; with the user's
  config (`type: gemini`, options set, metered unset) it returns a streamer, so
  the clips exist — they are simply never played.
- The process player is `playback.Player`; `PlayQueue(ctx, <-chan string)` starts
  on the first clip and ends when the channel closes (`pkg/media/playback/player.go`).
  `Service.AudioAvailable()` reports whether it opened, and the client uses
  `serverPlayback={serverAudio}` when it did (`frontend/src/App.tsx`), meaning the
  webview does not play turn audio on a machine with a device.
- Turn events travel as newline-delimited JSON on `POST /api/game/{id}/turn`
  (`TurnEvent`, `pkg/gui/types.go`); `streamTurn` on the client dispatches on
  `type` and already handles `chunk`, `tool`, `turn`, `model_missing`, `error`.
- `GetSegmentAudio` resolves a segment to a single clip; the client's
  `useSegmentPlayback` plays `segment.audio_url` in order and dedupes nothing.
- **Key mismatch (pre-existing, found while diagnosing the reuse bug):**
  `segmentDTOs` computes `AudioKey` with an empty speaker reference for narration,
  while the pipeline namespaces narration as `narrator`. The value is only a
  cache-buster today, but exact dedupe needs them to agree.
- Because finalise is sentence-scoped, a narration segment's audio is the
  concatenation of its sentence clips. Playing the provisional sentences and then
  the segment would repeat multi-sentence narration; a single-sentence segment's
  key is the sentence key, so key dedupe alone is not enough.

## 3. Design

### 3.1 The streamer publishes what it synthesizes

`sentenceStreamer` gains an emitter:

```go
// provisionalSpeech is one sentence whose clip is ready to play.
type provisionalSpeech struct {
    Index    int    // sentence ordinal within the turn, monotonic
    Text     string
    AudioKey string // the cache key the clip is stored under
    AudioURL string // /api/audio/clip/{key}
}

func newSentenceStreamer(ctx, pipeline, voice, logger, workers int, emit func(provisionalSpeech)) *sentenceStreamer
```

A worker emits only on success, and never blocks: the emitter is responsible for
being cheap. Index is assigned when the sentence is dispatched, so consumers can
order events even though two workers race.

### 3.2 Serving a clip by key

Clips are addressed by content key, so no turn or segment index is needed while a
turn is in flight. `GET /api/audio/clip/{key}` (under the existing `/api/audio/`
mount) validates the key is 64 lowercase hex characters, resolves it inside the
audio cache, and serves it with `audio/ogg` and an immutable cache header. A key
that is malformed or absent is a 404. No path is accepted from the client.

### 3.3 Process-player playback starts early

When `auto_play` is on and the player is available, `TurnSession.Run` opens a
queue channel and hands the streamer a sink that appends clip paths to it, in
emission order, before generation begins:

- The streamer's emitter appends each clip path it produces to the channel.
- After the turn is recorded, the session appends the audio that was **not**
  already played (see 3.4) and closes the channel.
- A turn with no audio (disabled TTS, no device, synthesis failures) sends
  nothing and closes an empty channel, which `PlayQueue` treats as an empty
  queue.

The channel is buffered so a fast streamer cannot block on the player.

### 3.4 No-repeat selection at finalise

The rule is per segment, using the sentence keys the streamer played:

- **Speech segment**: always synthesized and played (dialog cannot be streamed).
- **Narration segment**: compute its sentence keys. If every one is in the played
  set, skip it — its audio is exactly what was already heard. If none or only
  some were played, play the segment clip (a partially covered segment can repeat
  a sentence; the alternative is a gap, and this is rare because a sentence is
  usually a whole segment).
- The played set is exactly the keys the streamer emitted, so it costs nothing to
  maintain.

This selection is a pure function of the turn's segments, the played keys, and
the pipeline's key helper, so it is unit-tested without a device:

```go
// pkg/gui
func segmentsToPlay(turn *engine.Turn, played map[string]bool, keys segmentKeyFunc) []entity.TurnSegment
```

### 3.5 Client-authority parity

The same `speech` events drive the browser when the client owns playback:

- `TurnEvent` gains `index`, `audio_key`, and `audio_url` for `type: "speech"`.
- `App.tsx` accumulates them in order and plays them while the prose streams,
  using the existing browser-audio mechanics (autoplay-blocked handling is
  unchanged).
- On `turn`, playback continues from the authoritative segments using the same
  no-repeat rule. To make that exact, `SegmentDTO` gains
  `sentence_audio_keys` for narration segments; the client skips a segment whose
  keys are all in its played set.

### 3.6 Key alignment

`segmentDTOs` uses the pipeline's own key for each segment instead of
recomputing it with a different speaker namespace, so `AudioKey`,
`sentence_audio_keys`, and the `?v=` cache-buster all agree with the clip that is
actually served. The pipeline exposes a small helper:

```go
// pkg/media
func (p *TTSPipeline) SegmentAudioKey(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) (string, error)
func (p *TTSPipeline) SegmentSentenceKeys(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) []string
```

`SegmentAudioKey` returns `ErrNoSpeakableText` for a segment that reduces to
nothing; callers treat that as "no audio", as they already do.

## 4. Interfaces

```go
// pkg/gui
type provisionalSpeech struct{ Index int; Text, AudioKey, AudioURL string }
func newSentenceStreamer(ctx context.Context, pipeline *media.TTSPipeline, voice *entity.VoiceConfig, logger trace.Logger, workers int, emit func(provisionalSpeech)) *sentenceStreamer
func segmentsToPlay(turn *engine.Turn, played map[string]bool, keys segmentKeyFunc) []entity.TurnSegment
// TurnEvent gains: Index int, AudioKey string, AudioURL string
// SegmentDTO gains: SentenceAudioKeys []string

// pkg/media
func (p *TTSPipeline) SegmentAudioKey(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) (string, error)
func (p *TTSPipeline) SegmentSentenceKeys(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) []string
```

HTTP: `GET /api/audio/clip/{key}`.

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| Provisional sentence fails | No event; finalise synthesizes and plays it |
| Clip key unknown or malformed | 404; client skips the item |
| Player unavailable | No queue is opened; events still emitted for a client authority |
| Client disconnects mid-turn | The turn is cancelled as today; the queue drains its remaining clips |
| Emit from a worker races the chunk writer | Serialised by a mutex owned by the turn session |
| Partially covered narration segment | Segment clip plays; a sentence may repeat (documented) |
| Turn cancelled or failed | Nothing is persisted, as today; the queue is closed and stops |

## 6. Testing & Verification

Go (stdlib `testing`):

- `pkg/media`: `SegmentAudioKey` matches the key the pipeline stores a clip
  under; `SegmentSentenceKeys` returns one key per sentence and none for
  unspeakable text.
- `pkg/gui`: the streamer emits one ordered item per completed sentence with a
  resolvable key, and none for a partial tail.
- `pkg/gui`: `segmentsToPlay` — a fully covered narration segment is skipped, a
  partially covered one is played, a speech segment is always played, and an
  unplayed narration segment is played.
- `pkg/gui`: `GET /api/audio/clip/{key}` serves a stored clip, 404s an unknown
  key, and 404s a malformed one (path traversal attempt included).
- `pkg/gui`: a `speech` event appears on the turn stream when the streamer
  synthesizes (fake TTS client, temp cache).
- Frontend: `tsc` only.

Manual: replay the Pallid Court opening and confirm narration begins while the
text is still arriving and no line is repeated.

## 7. Compatibility & Rollout

- Additive: new event type, new optional DTO fields, one new read-only route.
- A client that ignores `speech` behaves exactly as today.
- Server-side playback changes when audio starts, not what it sounds like; the
  no-repeat rule keeps the heard sequence identical to the old one.
- Existing cached clips remain valid; the key namespace fix changes the `?v=`
  token, which only busts a browser cache.
- The `media.tts.stream_sentences` gate still decides whether sentences are
  pre-synthesized at all; with it off there is nothing to stream and playback
  falls back to today's behaviour.

## 8. Open Questions

- Should a partially covered narration segment play only its unplayed sentences
  rather than the whole segment (complete, but needs sentence-level playback)?
- Should the theater advance beats on streamed audio, or keep advancing on the
  authoritative segments once the turn lands?
- Does the client need a visible indicator that audio is streaming rather than
  buffering?
- Should provisional playback respect a per-sentence timeout so a slow provider
  cannot make audio lag the text indefinitely?

## 9. References

- Code: `pkg/gui/streaming_tts.go`, `pkg/gui/service.go` (`TurnSession.Run`,
  `PlayTurnAudio`, `segmentDTOs`), `pkg/media/tts.go`,
  `pkg/media/playback/player.go`, `pkg/gui/server.go` (`handleAudioRoutes`),
  `frontend/src/App.tsx`, `frontend/src/api/client.ts`,
  `frontend/src/hooks/useSegmentPlayback.ts`.
- Specs: `2026-09-28-streaming-tts-design.md` (Phase 2),
  `2026-09-28-sentence-scoped-tts-clips-design.md`.
