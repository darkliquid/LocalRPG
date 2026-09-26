# Turn latency research

Date: 2026-09-26
Status: research + recommendations (no code changed)

## Question

A turn round-trip feels slow: time to the first response, then converting the
reply into segments, then handling each segment, then generating voices and
playing narration. Where does the wall-clock time actually go, and how can each
stage be shortened?

## TL;DR

Latency comes from four serial phases, and most of it is not the model:

1. **Before the first byte**: `prepareTurn` rebuilds the router, the JS
   mechanics VM (re-evaluating `mechanics.js` every turn), the prompt files, and
   a TTS client on *every* request (`pkg/gui/service.go:1143-1258`). None of it
   is cached, and all of it is paid before anything streams.
2. **Generation**: one streaming call plus extra full model calls — unbounded
   tool rounds by default (`pkg/config/types.go:650-655`), a completion
   continuation (`pkg/engine/recovery.go`), and a synchronous extractor call
   after the stream when there is no structured submission
   (`pkg/engine/orchestrator.go:824-836`).
3. **Post-stream before segments appear**: a full-directory `Sync` re-parses
   every entity file (`pkg/engine/timeline.go:248-273`), `history.jsonl` is
   fully re-parsed per turn and again per segment
   (`pkg/gui/service.go:1672-1673`), and `ListEntities` full scans repeat for
   speaker and mention resolution.
4. **Audio**: every segment builds a fresh TTS client (and, for built-in TTS,
   reloads the ONNX model), encoding Ogg/Opus runs inline, and whole-turn
   playback waits for *every* clip before the first one plays
   (`pkg/gui/service.go:1818-1856`). The theater then adds a 500 ms status poll
   and a 300 ms beat gap per line.

External primary sources agree on the shape of the fix: overlap the stages
rather than sum them, cache aggressively, and keep post-processing off the
critical path (Microsoft Speech, Cartesia, Picovoice, OpenAI).

## Method

- Traced the full server path: `pkg/gui/server.go` turn route →
  `Service.BeginTurn`/`TurnSession.Run` → `engine.ProcessActionStream` →
  `runGenerationLoop` → timeline writes → `turnDTO`.
- Traced the audio path: turn play/segment play/segment fetch routes →
  `GetSegmentAudio` → `pkg/media` synthesis/decode/Opus encode → client
  `useSegmentPlayback` and `StoryTheater`.
- Read config defaults in `pkg/config/types.go`.
- Consulted vendor primary docs and research on LLM→TTS latency (sources at the
  end).

## Where the time goes

### Phase 0: pre-first-byte (`prepareTurn`)

`handleTurnSubmit` calls `BeginTurn` → `prepareTurn` and only writes the 200 and
stream headers after it returns (`pkg/gui/server.go:1114-1137`). Per turn,
`prepareTurn` does all of:

- `core.LoadSystemManifest` twice (`pkg/gui/service.go:1151`, `:1226`) and once
  more inside the rules loader (`pkg/rules/loader.go:27`).
- `harness.RouterFromConfigWithLogger` — rebuilds every provider object
  (`pkg/gui/service.go:1180`; `pkg/harness/factory.go:135-183`).
- `rules.NewJSEngine` + `LoadRules` — a new goja VM and a re-read/re-eval of
  `mechanics.js` and world `hooks.js` (`pkg/gui/service.go:1185-1193`;
  `pkg/rules/loader.go:23-50`). This is now every turn by design (see the
  Mechanics Trigger plan), and it is the top candidate for caching.
- `LoadPrompts` — reads `rules.md`, `system.yaml`, `mechanics.js`, `lore.md`
  (`pkg/engine/orchestrator.go:388-412`).
- `ttsClientFor` — builds a TTS client (`pkg/gui/service.go:1251`).

The comment at `pkg/gui/service.go:1140-1142` calls this intentional (to pick up
settings and note edits). That correctness goal can be met by caching keyed on a
config revision plus file mtimes, rebuilding only when something actually
changed.

Also on this path: `history.LoadHistory()` re-parses the whole log
(`pkg/engine/orchestrator.go:420`), and prompt `Assemble` does many SQLite reads
(`:652`).

### Phase 1: generation

`runGenerationLoop` (`pkg/engine/orchestrator.go:1236`) is a bounded
conversation of full model calls:

- **gm stream** round 0 (`:1406` → `stream:1048`) — the narration itself.
- **tool rounds**: each round is another full model call (`:1406`), executed
  serially. The cap is `ToolRounds`, whose **default 0 means unbounded**, with a
  runaway ceiling of 100 (`pkg/config/types.go:650-655`; `orchestrator.go:190`).
- **completion continuation**: when the reply is cut, another full model call,
  bounded by `CompletionTimeout` (default 45 s), mode `auto`
  (`pkg/engine/orchestrator.go:752-754`; `pkg/engine/recovery.go:112-188`).
- **extractor**: only when there is *no* structured submission, a second full
  model call runs synchronously after the stream (`orchestrator.go:824-836`;
  `pkg/harness/extractor.go:323-336`), inheriting the `gm` provider by default
  (`pkg/config/types.go:306-309`).
- **summariser**: detached (`pkg/gui/service.go:1066`) — not on the path, but it
  competes for the provider.

### Phase 2: post-stream, before the `turn` event

All awaited before `ProcessActionStream` returns:

- `ResolveEntityMentions` (`orchestrator.go:810`), extractor (`:831`), segment
  building whose speaker resolver falls back to `store.ListEntities()` on a slug
  miss (`pkg/engine/segments.go:17-45`; `pkg/harness/extractor.go:406-437`).
- `MatchExistingEntity` loops calling `GetEntity` + `ListEntities` per extracted
  entity (`orchestrator.go:909-920`; `pkg/harness/extractor.go:143-186`).
- `Timeline.RecordTurnContextStructured` (`pkg/engine/timeline.go:76`):
  `ResolveProseMentions` (another `ListEntities`, `:86`), `stageEntities`, then
  **`writeEntities` calls `Sync(dir)` which re-reads and re-parses every
  `.md`** (`timeline.go:248-273`; `pkg/storage/sync.go:26-79`), then history
  append and `indexTurn`.
- `turnDTO`/`segmentDTOs` re-resolve speaker id and voice per segment
  (`pkg/gui/service.go:285-331`, `:923-946`).

### Phase 3: audio

- **`GetSegmentAudio` rebuilds everything per call**: full `history.jsonl`
  parse (`pkg/gui/service.go:1672-1673`), a fresh TTS client (`:1697`), the
  narrator voice read from `game.yaml` (`:1702`), a new `TTSPipeline` and
  `ContentCache` (`:1704-1706`), and a store open (`:1877`).
- **Built-in TTS reloads the model per segment**: `BuildTTS` returns a new
  `SherpaTTSClient` whose ONNX model loads lazily on first synthesis
  (`pkg/media/ttssherpa.go:32-40`; `pkg/media/ttssherpa/client.go:41-80`).
  `PlayTurnAudio` spawns one `GetSegmentAudio` per segment concurrently
  (`pkg/gui/service.go:1831-1843`), so an N-beat turn can construct and load N
  clients.
- **Whole-turn playback waits for all clips** before playing any: `wg.Wait()`
  then `PlayFiles` (`pkg/gui/service.go:1843-1856`). Time-to-first-audio is the
  slowest segment, not the first.
- **Ogg/Opus encode is inline and expensive**: decode, mono downmix, 48 kHz
  resample, per-20 ms frame encode at complexity 10, Ogg pages with Go CRC
  (`pkg/media/tts.go:324-332`; `pkg/media/opus/opus.go:35-91,168-192`;
  `pkg/media/opus/mux.go:102-165`).
- **No single-flight**: concurrent misses for the same key synthesize twice;
  the post-turn warm-up (`pkg/gui/service.go:1318-1326`) races the first play.
- **`Cache-Control: no-store`** on clips (`pkg/gui/server.go:435`) defeats the
  browser's one-ahead prefetch (`frontend/src/hooks/useSegmentPlayback.ts:61-66`).
- **Client serialization**: the theater requests beat N+1 only after beat N
  finishes (`frontend/src/components/StoryTheater.tsx:142-157`), with
  `BEAT_GAP_MS = 300` (`:29-31`) and a 500 ms `/api/audio/status` poll per beat
  (`frontend/src/App.tsx:317-331`).

## What the primary sources say

- **Instrument TTFT separately from total latency.** For streaming UIs,
  time-to-first-token is what users perceive as "stuck"; post-processing drives
  the rest. Microsoft defines TTS first-byte vs finish latency and notes
  first-byte is independent of text length. Measure p95/p99 (Microsoft Speech;
  board.itsueblog).
- **Overlap the stages; do not sum them.** Stream text into the TTS engine as it
  is produced — buffer to sentence boundaries and flush, keeping the connection
  open — rather than synthesizing after the full reply (Microsoft "input text
  streaming"; Picovoice; RealtimeTTS; LLMVoX; SpeakStream).
- **A second blocking model call is the classic latency bug.** "Never block the
  critical path on an LLM judge"; move scoring/extraction after the turn commits
  (futureagi; board.itsueblog).
- **Reuse warm clients.** Re-creating a TTS client per request pays connection
  and load cost; pre-connect and pool (Microsoft; Speechify).
- **Cache synthesized audio** keyed by text+voice+model, and interleave cached
  PCM with live streams (Speechify; Cartesia).
- **Parallelize independent segments and prefetch.** "Run synthesis in parallel
  when you have many short segments"; prefetch the next beat (Speechify;
  Picovoice).
- **Budget against human turn-taking**: median ~200 ms; beyond ~800 ms feels
  broken (Stivers et al. 2009 via Picovoice).

## Recommendations

### Quick wins (low risk, high impact)

1. **Cap tool rounds.** Treat `ToolRounds = 0` as a small default (2-3), not
   unbounded (`pkg/config/types.go:650-655`).
2. **Stop the redundant full-directory `Sync`.** Call `SyncFile` for the files
   just written (`pkg/engine/timeline.go:270`), as `SaveEntity` already does
   (`pkg/storage/sync.go:81`).
3. **Cache the TTS client/pipeline per config revision** in `Service` and reuse
   it across segments and turns (`pkg/gui/service.go:1697`,
   `pkg/media/ttssherpa.go:32-40`). This alone removes repeated model loads.
4. **Start playback on the first clip.** Synthesize segment 0, begin playback,
   and stream the rest in order instead of `wg.Wait()` for all
   (`pkg/gui/service.go:1843-1856`).
5. **Serve clips with an ETag/immutable cache** and keep the prefetch `Audio`
   object (`pkg/gui/server.go:435`;
   `frontend/src/hooks/useSegmentPlayback.ts:61-66`).
6. **Replace the 500 ms poll with a completion event** (SSE) and retune
   `BEAT_GAP_MS` (`frontend/src/App.tsx:317-331`;
   `frontend/src/components/StoryTheater.tsx:29-31`).
7. **Skip the extractor when a structured submission exists** — it already is
   skipped in that case; make the non-tool path cheaper by defaulting the
   extractor role to a faster model or `disabled` for deterministic mentions.
8. **Default completion mode to `trim`** rather than `auto` to avoid a 45 s
   continuation call (`pkg/config/types.go:595-636`).

### Structural

9. **Cache the per-turn wiring** (router, JS engine, prompts, TTS client) keyed
   by a config revision plus manifest/system/world mtimes, rebuilding only on
   change (`pkg/gui/service.go:1143-1258`). This preserves the "edits take
   effect" intent without paying it every turn.
10. **Add an in-memory turn cache** so `history.jsonl` is not fully re-parsed per
    turn and per segment (`pkg/engine/orchestrator.go:420`;
    `pkg/gui/service.go:1672-1673`). The SQLite `turns` table can answer
    "turn N" directly.
11. **Resolve entities once per turn.** Load the entity summary list once and
    pass a resolver/map through segment build, extraction matching, timeline
    record, and DTO build, replacing the repeated `ListEntities` scans.
12. **Overlap extraction with deterministic timeline work.** Run the extractor
    concurrently with `stageEntities`/history append and merge before indexing,
    or emit it as a follow-up event after the turn is shown.
13. **Stream TTS during generation.** When a tool-capable provider is in use,
    start synthesizing completed sentences while the model is still streaming,
    instead of waiting for the final segments.
14. **Single-flight synthesis per cache key** and serialize the warm-up behind
    the same lock (`pkg/media/tts.go:263-299`).
15. **Move Opus encoding off the request path** once a clip is playable (encode
    to a temp file asynchronously, or lower complexity / skip the 48 kHz
    resample when the source already matches) (`pkg/media/tts.go:324-332`;
    `pkg/media/opus/opus.go:35-91`).
16. **Prefetch the next beat** in the theater: request segment N+1 as soon as N
    starts playing (`frontend/src/components/StoryTheater.tsx:142-157`).

### Measurement

17. **Instrument the phases with spans and p95/p99**: pre-first-byte
    (`prepareTurn`), TTFT (first chunk), post-stream (segment build → `turn`
    event), and first-audio / total-audio. The repo already has OTel tracing;
    add explicit turn-phase spans so regressions are visible.

## Open questions

- Which provider is dominant in practice, and can the extractor be a cheaper
  model than the GM by default?
- Is the per-turn wiring cache safe given hand-edited notes, or should it key on
  entity-dir mtime too?
- Do we want sentence-level TTS streaming, which needs a streaming-capable TTS
  provider (built-in sherpa is batch-only today)?
- Should the theater use server-push completion so beats no longer poll?

## Sources

Repo (primary):

- `pkg/gui/server.go:345,1103-1148,435`; `pkg/gui/service.go:1143-1258,1311-1326,1671-1709,1818-1884`
- `pkg/engine/orchestrator.go:420,652,824-836,909-920,1236,1378,1406`; `pkg/engine/timeline.go:76,240-273`; `pkg/engine/segments.go:17-45`
- `pkg/engine/recovery.go:112-188`; `pkg/harness/extractor.go:143-186,323-437`
- `pkg/rules/loader.go:23-50`; `pkg/harness/factory.go:135-183`
- `pkg/config/types.go:306-309,595-655`
- `pkg/media/tts.go:263-373`; `pkg/media/opus/opus.go:35-192`; `pkg/media/opus/mux.go:102-165`; `pkg/media/ttssherpa.go:32-40`; `pkg/media/ttssherpa/client.go:41-80`
- `frontend/src/App.tsx:293-339`; `frontend/src/components/StoryTheater.tsx:29-31,142-157`; `frontend/src/hooks/useSegmentPlayback.ts:52-71`

External (primary):

- Microsoft Speech, lower speech synthesis latency — https://learn.microsoft.com/en-us/azure/ai-services/speech-service/how-to-lower-speech-synthesis-latency
- Cartesia, TTS caching — https://docs.cartesia.ai/build-with-cartesia/capability-guides/tts-caching
- Picovoice, voice UX latency and turn-taking — https://picovoice.ai/guide/voice-agents/voice-ux-latency-turn-taking/
- OpenAI, voice agents — https://developers.openai.com/api/docs/guides/voice-agents
- OpenAI, Realtime — https://developers.openai.com/api/docs/guides/realtime
- Speechify, reduce TTS latency — https://speechify.com/blog/reduce-tts-latency-production/
- RealtimeTTS — https://github.com/KoljaB/RealtimeTTS
- LLMVoX — https://mbzuai-oryx.github.io/LLMVoX/
- SpeakStream — https://arxiv.org/abs/2505.19206
- futureagi, voice agent latency — https://futureagi.com/blog/how-to-optimize-voice-agent-latency-2026/
- TTFT vs total latency — https://board.itsueblog.com/llm-streaming-ttft-vs-total-latency/
