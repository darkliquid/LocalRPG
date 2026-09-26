# Turn Latency Reduction Design

**Date:** 2026-09-26
**Status:** Proposed
**Scope:** Caching the per-turn wiring, removing redundant hot-path work, pipelining audio synthesis and playback, and phase instrumentation
**Related:** Turn latency research (`docs/proposals/2026-09-26-turn-latency-research.md`), Mechanics Trigger & Cadence Design (2026-09-26), `pkg/gui`, `pkg/engine`, `pkg/media`, `pkg/config`, `frontend/`

## 1. Overview & Goals

A turn is slow in four distinct places: everything `prepareTurn` rebuilds
before the first byte, extra full model calls during and after generation,
post-stream re-parsing before segments appear, and an audio pipeline that
resynthesises clients and waits for every clip before playing any. The research
note quantifies each with file references.

This specification removes the avoidable work and overlaps the rest, without
changing narration quality or the persistence guarantees (Markdown canonical,
`history.jsonl` append-only, index rebuildable).

**Goals:**

- Cut time-to-first-byte by caching the per-turn wiring and rebuilding only when
  configuration or on-disk content actually changes.
- Cut post-stream latency by removing O(N) re-parsing (`Sync` over the whole
  entity dir, full `history.jsonl` loads, repeated `ListEntities` scans).
- Cut time-to-first-audio by sharing one TTS client, synthesising with
  single-flight, and starting playback from the first completed clip.
- Stop a second blocking model call from being the default when it is not
  needed.
- Make each phase measurable so regressions are visible.

**Non-Goals:**

- Changing narrative quality, prompt layering, or what is persisted.
- Provider-level parameters that the current provider implementations do not
  expose.
- Sentence-level streaming TTS across providers (needs a streaming-capable TTS
  backend; recorded as a follow-up).
- Speech-to-speech / Realtime API.

**Success Criteria:**

- Warm `prepareTurn` adds < 50 ms before the first byte (today it rebuilds the
  router, a goja VM + rules, prompts, and a TTS client every turn).
- Time-to-first-audio for an N-beat turn is governed by the **first** beat, not
  the slowest; playback begins as soon as segment 0 is synthesized.
- A turn that writes k entities re-indexes k files, not every file in
  `entities/`.
- `history.jsonl` is parsed at most once per turn, not once per segment.
- Default tool rounds are bounded, and a cut reply defaults to `trim`.
- Per-phase spans exist for pre-first-byte, TTFT, post-stream, first-audio, and
  total-audio.

## 2. Investigation Summary

See the research note for full citations. Load-bearing facts:

- `prepareTurn` (`pkg/gui/service.go:1143-1258`) rebuilds the router, JS engine
  + rules, prompts, and TTS client every turn; `system.yaml` is read three times.
- `ToolRounds` default 0 means unbounded (`pkg/config/types.go:650-655`).
- The extractor is a second full model call after the stream when there is no
  structured submission (`pkg/engine/orchestrator.go:824-836`).
- `Timeline.writeEntities` calls `Sync(dir)`, re-parsing every entity file
  (`pkg/engine/timeline.go:248-273`).
- `GetSegmentAudio` re-parses all of `history.jsonl` per segment and builds a
  new TTS client and pipeline per call (`pkg/gui/service.go:1671-1709`).
- `PlayTurnAudio` `wg.Wait()`s for every segment before `PlayFiles`
  (`pkg/gui/service.go:1831-1856`).
- `Player.PlayFiles` replaces the queue and returns once playback starts
  (`pkg/media/playback/player.go:147`); there is no queue-append API.
- Clips are served `Cache-Control: no-store` (`pkg/gui/server.go:435`) and the
  client discards its prefetch `Audio` (`frontend/src/hooks/useSegmentPlayback.ts:61-66`).
- The theater advances a beat only after the previous finishes, with a 300 ms
  gap and a 500 ms completion poll (`frontend/src/components/StoryTheater.tsx:29-31,142-157`;
  `frontend/src/App.tsx:317-331`).

## 3. Design

### 3.1 Configuration revision

Caching needs a cheap "has anything changed" signal.

Add to `pkg/config`:

```go
// Revision increments on every successful load or save, so callers can key
// caches on the configuration without diffing it.
func (m *ConfigManager) Revision() uint64
```

`Load` and `Save` bump an internal `atomic.Uint64`. `Get` is unchanged.

### 3.2 Phase 1 — hot-path quick wins

Each is independently shippable.

**3.2.1 Targeted entity sync.** In `Timeline.writeEntities`
(`pkg/engine/timeline.go:270`), replace `Sync(dir)` with `SyncFile(path)` for
each file just written (`pkg/storage/sync.go:81`), which is what `SaveEntity`
already uses. Removes O(all entities) parsing per turn.

**3.2.2 Bound tool rounds.** `Config.ToolRounds()` returns a bounded default
(3) when the configured value is 0, keeping a documented ceiling of 100. An
explicit `-1` remains "unbounded" for advanced users.

**3.2.3 Default completion to `trim`.** `CompletionMode()` returns `trim` when
unset, so a cut reply is trimmed rather than triggering a second full 45 s call.
`auto` remains available.

**3.2.4 Extractor concurrency.** When a structured submission is absent, run
`extractor.Extract` concurrently with the deterministic timeline work
(`stageEntities`, history append) and merge before indexing, instead of
serializing it between the last chunk and the `turn` event. A failed extractor
still never loses the turn (`pkg/engine/orchestrator.go:824-836`).

**3.2.5 Shared TTS client and pipeline.** `Service` caches one `TTSClient` and
one `TTSPipeline` per `(config revision)`, invalidated by the revision. Built-in
TTS then loads its ONNX model once, not once per segment. `GetSegmentAudio`
uses the cached client (`pkg/gui/service.go:1697`).

**3.2.6 Single-flight synthesis.** Key a per-key mutex (or singleflight) on the
audio cache key in the pipeline so concurrent requests for the same utterance
synthesize once; the post-turn warm-up (`pkg/gui/service.go:1318-1326`) joins
the same lock rather than racing.

**3.2.7 Streaming playback queue.** Add a queue API to the player so playback
starts on the first clip:

```go
// PlayQueue starts playback and pulls clip paths from clips as each previous
// clip drains. It returns once playback has started; the channel closing ends
// the queue. A clip that cannot be decoded is skipped.
func (p *Player) PlayQueue(ctx context.Context, clips <-chan string) error
```

Implementation: a `beep.Streamer` that holds the current decoded streamer and,
when it drains, receives the next path from the channel and decodes it, with one
clip of look-ahead. `PlayTurnAudio` becomes: start `PlayQueue`, synthesize
segments in order into the channel, close it when done. Time-to-first-audio is
then segment 0's synthesis.

**3.2.8 Client cache and prefetch.** Serve clips with `ETag` and
`Cache-Control: private, max-age=…, immutable` keyed by the existing `v=` token
instead of `no-store` (`pkg/gui/server.go:435`), and retain the prefetch `Audio`
object in `useSegmentPlayback` so the next beat is decoded ahead.

**3.2.9 Theater pacing.** Keep `BEAT_GAP_MS` but make it configurable; prefetch
beat N+1 as soon as N starts; replace the 500 ms completion poll with a server
event (see 3.4).

### 3.3 Phase 2 — structural caching

**3.3.1 Turn runtime cache.** Introduce a `turnRuntime` in `Service` holding the
per-turn wiring that does not depend on the turn itself:

- `harness.Router` (from config)
- `rules.JSEngine` with rules already loaded
- prompts (`rules.md`, `lore.md`) and the rendered mechanics instruction
- declared stats and `AllowFreeformState`
- the shared `TTSClient`/pipeline

Key: `config.Revision()` plus mtimes of `game.yaml`, `system.yaml`,
`mechanics.js`, world `hooks.js`, `prompts/rules.md`, and `prompts/lore.md`.
Because the key includes file mtimes, a hand edit still takes effect on the next
turn — preserving the intent of the comment at `pkg/gui/service.go:1140-1142`
while paying the build cost only on change.

The store, timeline, player id, and chronicler stay per turn (they are cheap or
turn-specific).

**3.3.2 In-memory turn cache.** A small per-game cache of parsed
`history.jsonl` keyed by file size + mtime, updated in place by `RecordTurn`.
`GetSegmentAudio` reads from it instead of re-parsing the log per segment
(`pkg/gui/service.go:1672-1673`). The log remains canonical; the cache is a
read-through.

**3.3.3 Once-per-turn entity resolver.** Build the entity summary map once per
turn and pass it to segment building, `MatchExistingEntity`, `ResolveProseMentions`,
and `turnDTO`, replacing the repeated `ListEntities` scans
(`pkg/harness/extractor.go:421,483,159`; `pkg/gui/service.go:942`).

**3.3.4 Cheaper Opus encoding (keep the single-format invariant).** Every clip in
the content cache is Ogg/Opus: `audioExtensions` is exactly `.opus`, a clip whose
bytes are not a valid Ogg/Opus header is dropped and re-synthesized
(`pkg/media/tts.go:28-30,358-397`), and playback decodes Opus only
(`pkg/media/playback/player.go:337-347`). Encoding therefore stays **inline on the
request path** so the cache is never left holding a non-Opus or partial file.
The cost is reduced only by skipping the 48 kHz resample when the provider output
is already 48 kHz, and by lowering the encoder complexity (10 → 5). If a future
change wants encoding off the request path, it must preserve the invariant by
writing `base+".opus.tmp"` and renaming to `base+".opus"` only once the complete
Ogg is on disk, so a concurrent reader misses until the file is ready.

Note: the cache key hashes speaker, provider/voice/prosody/options and text, but
not the encoder parameters (`pkg/media/cache.go:29-67`). Lowering complexity or
changing `OpusBitrate` therefore does not invalidate existing clips; only newly
synthesized clips take the new setting (all still Opus). Add an encoder-version
token to the key if a consistent bitrate across the cache is required.

### 3.4 Audio completion events (frontend)

Replace per-beat `/api/audio/status` polling with a server-sent stream of audio
events for a game (beat started / finished / queue drained). The theater
advances beats on the event rather than on a 500 ms poll. If SSE is too large a
step, Phase 1 keeps the poll but lengthens the interval and advances on
`playing → idle` as today.

### 3.5 Instrumentation

Add spans (the repo already uses OTel) around:

- `turn.prepare` (pre-first-byte)
- `turn.ttft` (submit → first chunk)
- `turn.finalise` (last chunk → `turn` event, i.e. segment build + timeline)
- `turn.audio.first` and `turn.audio.total`

Emit them as trace attributes on the turn so p95/p99 can be read from the
existing trace tooling.

## 4. Interfaces

```go
// pkg/config
func (m *ConfigManager) Revision() uint64

// pkg/media/playback
func (p *Player) PlayQueue(ctx context.Context, clips <-chan string) error

// pkg/gui (internal)
type turnRuntime struct { /* router, jsEngine, prompts, mechanics prompt, stats, tts */ }
func (s *Service) runtimeFor(gameID string, manifest *core.GameManifest) (*turnRuntime, error)
```

No public HTTP route changes are required except the optional audio events
stream (3.4) and the clip cache headers (3.2.8).

## 5. Error Handling

- Cache misses and stale entries rebuild silently; a failed rebuild falls back
  to the current behaviour (log `runtime.rebuild_error`).
- Invalidating the runtime on revision/mtime change must never drop a turn:
  a rebuild failure uses the per-turn construction path as today.
- `PlayQueue` end-of-queue is normal termination, not an error; undecodable
  clips are skipped as `PlayFiles` does.
- The in-memory turn cache is advisory; a stale or missing entry re-reads
  `history.jsonl`.
- Extractor concurrency keeps the existing rule: a failed extractor never loses
  the turn.

## 6. Testing & Verification

Go (stdlib `testing`, `t.TempDir()`):

- `pkg/engine`: a turn writing one entity re-indexes only that file (instrument
  the syncer or assert unrelated malformed files are untouched), and a turn with
  no entity writes performs no `Sync`.
- `pkg/engine`: with a structured submission the extractor is not called; with
  no submission it runs and its result is merged before indexing.
- `pkg/config`: `ToolRounds()` returns the bounded default for 0 and honours a
  negative as unbounded; `CompletionMode()` defaults to `trim`.
- `pkg/gui`: `runtimeFor` returns the same runtime for unchanged revision+mtimes
  and a new one after a save or a file mtime change.
- `pkg/gui`: `GetSegmentAudio` for two segments of one turn reads history once
  (count reads) and reuses one TTS client.
- `pkg/media/playback`: `PlayQueue` starts playing on the first clip and ends
  when the channel closes; a bad clip is skipped.
- `pkg/harness`/`pkg/gui`: the entity resolver is built once per turn.

Frontend: `tsc` only (no runner); verify prefetch retention and event-driven
advance by inspection.

Manual/measured: before and after numbers for TTFT, first-audio, and total on a
3-beat turn with built-in TTS, recorded in the trace.

## 7. Compatibility & Rollout

- Phase 1 items are behaviour-preserving and independently revertable.
- The runtime cache is the only change that could hide an edit; the mtime key
  makes that explicit and testable. A conservative initial key includes the
  entity directory mtime so hand edits there also invalidate.
- The config revision is additive; existing config files are unaffected.
- `Trim` becoming the completion default changes recovery behaviour: a cut
  reply ends instead of being continued. Documented in config comments.

## 8. Open Questions

- Is the mtime+revision key sufficient, or should we hash the prompt files?
- Should `PlayQueue` live on the player or should the service own the pipeline
  and hand the player a ready channel? (Player is simpler; service knows how to
  synthesize.)
- Do we add `segments_json` to the `turns` table rather than an in-memory cache,
  so `GetSegmentAudio` reads one row?
- Is sentence-level streaming TTS worth a provider capability, and which
  providers could support it?
- SSE audio events vs. keeping the poll with a longer interval.

## 9. References

- Research: `docs/proposals/2026-09-26-turn-latency-research.md`
- Code: `pkg/gui/service.go:1143-1258,1311-1326,1671-1709,1818-1884`;
  `pkg/gui/server.go:435`; `pkg/engine/orchestrator.go:420,824-836,1236,1378,1406`;
  `pkg/engine/timeline.go:248-273`; `pkg/harness/extractor.go:143-186,406-437`;
  `pkg/config/types.go:306-309,595-655`; `pkg/media/playback/player.go:147`;
  `pkg/media/tts.go:263-373`; `pkg/media/opus/opus.go:35-192`;
  `pkg/rules/loader.go:23-50`; `pkg/harness/factory.go:135-183`;
  `frontend/src/components/StoryTheater.tsx:29-31,142-157`;
  `frontend/src/hooks/useSegmentPlayback.ts:52-71`
