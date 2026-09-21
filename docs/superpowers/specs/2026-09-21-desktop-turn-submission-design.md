# Design Specification: Desktop Turn Submission

**Date:** 2026-09-21  
**Status:** Draft — pending review  
**Topic:** Let the desktop app play a turn: a streaming turn endpoint, a per-request orchestrator, serialised concurrent turns, and an action console that actually submits

---

## 1. Problem Statement & Motivation

The desktop GUI can browse everything about a campaign and cannot play one.

1. **No turn endpoint exists.** `pkg/gui/server.go` registers `/api/game/`, `/api/games`, `/api/systems`, `/api/system/`, `/api/worlds`, `/api/world/`, `/api/settings`, and `/api/settings/test-provider`. Nothing accepts a player action.
2. **The service has no orchestrator.** `pkg/gui.Service` opens stores, reads entities, compiles chronicles, and tests providers; it never constructs an `engine.TurnOrchestrator`, so the engine's turn pipeline is reachable only from `cmd/localrpg/play.go`.
3. **The action console does not submit.** `frontend/src/components/ActionConsole.tsx` offers a mode selector and an input box, and has no `fetch` call at all. The in-app player can therefore only render turns a terminal produced.
4. **Streaming exists but is unused by the turn pipeline.** `harness.ModelProvider` requires both `Generate` and `Stream`, `CLIProvider.Stream` reads the subprocess's stdout incrementally, `HTTPProvider.Stream` posts `stream: true` and reads server-sent events, and `harness.Router.StreamForRole` already implements first-chunk fallback to another provider. `TurnOrchestrator.ProcessAction` calls `GenerateForRole` only, so a minute-long local generation arrives as one silent wait.
5. **Nothing serialises turns.** Each turn loads history, derives the next number from its length, then appends. Two concurrent turns would interleave into `history.jsonl` with duplicate numbers and a corrupted timeline — the exact artefact the timeline work exists to make trustworthy.
6. **The two clients would grow two copies of provider setup.** `cmd/localrpg/play.go` builds a `harness.Router` from the config and resolves the extractor; both live in `package main`, where the GUI cannot reach them.
7. **One behaviour is unverified.** Whether the Wails webview delivers a streamed response body progressively or buffers it until the request completes is unknown. The design must be correct either way rather than betting on it.

Prior work this depends on: the attribution, location, and playback spec, which creates the player note (so a fresh campaign can be played at all), records a location per turn, produces segments, serves per-segment audio, and gives the client playback; and the export spec, which supplies the shared pacing estimate the console reuses while streaming.

---

## 2. Decisions & Non-Goals

Settled by review:

| Area | Decision |
| --- | --- |
| Transport | Newline-delimited JSON over a single `POST`, read incrementally by the client, correct on clients that buffer the body |
| Streaming role | An optimisation, never a requirement: the same request works whether chunks arrive progressively or all at once |
| Orchestrator lifetime | Built per request; the service stays stateless apart from a per-game lock |
| Concurrent turns | A per-game mutex; a second in-flight turn for the same campaign gets `409 Conflict` |
| Interrupted turns | Nothing is persisted until a turn completes; cancellation is a no-op on disk |
| Partial recovery | Out of scope. Drafts are their own feature with their own lifecycle |
| UI scope | The existing action console, covering every mode and the `/gm`, `/undo`, and `/go` input prefixes. No location picker |
| Provider setup | Extracted into `pkg/harness` so the CLI and the GUI share one implementation |
| Turn rendering | The streamed result is the same `TurnDTO` the chronicle returns, so the client renders a live turn and a replayed turn identically |

Non-goals:

- A WebSocket transport. The stream is one-way and finite, and the existing HTTP surface already covers every runtime mode including the in-process webview.
- Draft persistence or crash recovery for partially generated turns.
- A location picker or any new navigation affordance. `/go <location>` typed into the console is the whole feature.
- Multiple simultaneous turns per campaign, from any client.
- Authentication. The zero-TCP posture (in-process webview, `0600` Unix socket, opt-in loopback TCP) is unchanged and is what protects the endpoint.
- Changes to the TUI, which already plays turns, or to the exporter, which renders persisted ones.

---

## 3. The Endpoint

```text
POST /api/game/{id}/turn
Content-Type: application/json

{"mode": "Do", "input": "I search the harbour for Garrick"}
```

```text
200 OK
Content-Type: application/x-ndjson
X-Accel-Buffering: no

{"type":"chunk","text":"The docks "}
{"type":"chunk","text":"reek of brine."}
{"type":"turn","turn":{ … the same TurnDTO the chronicle returns … }}
```

| Case | Response |
| --- | --- |
| Valid request | `200` with an NDJSON body, one JSON object per line |
| Malformed body, unknown mode, or empty input | `400` before any bytes are written |
| Unknown game | `404` |
| A turn is already in flight for this campaign | `409` with `Retry-After: 1` |
| The campaign cannot be prepared (missing game manifest, system, or world) | `503` with the reason |
| Failure after streaming began | `200` continues, terminated by `{"type":"error","message":"…"}` |

Status codes can only be chosen before the first byte, so any failure after that is an `error` event rather than a status change. The client treats a closed stream without a `turn` event as a failure.

Modes are validated against the engine's set (`Do`, `Say`, `Story`, `Roll`, `GM`, `System`), case-insensitively, and normalised to the engine's capitalisation. Input is passed through verbatim: `/gm`, `/undo`, and `/go` are input prefixes the engine already interprets, and the endpoint adds no interpretation of its own. Those three short-circuit before generation, so a client must tolerate a stream with zero `chunk` events.

```go
// TurnRequest is a player action as submitted from a client.
type TurnRequest struct {
	Mode  string `json:"mode"`
	Input string `json:"input"`
}

// TurnEvent is one NDJSON line sent while a turn runs.
type TurnEvent struct {
	Type    string   `json:"type"`              // "chunk", "turn", or "error"
	Text    string   `json:"text,omitempty"`    // narration delta
	Turn    *TurnDTO `json:"turn,omitempty"`    // the persisted turn
	Message string   `json:"message,omitempty"` // failure detail
}
```

---

## 4. Streaming the Turn Pipeline

`TurnOrchestrator` gains a streaming entry point, and the non-streaming one becomes a wrapper over it, so there is exactly one copy of the turn pipeline:

```go
// ProcessActionStream runs a turn, reporting narration deltas as they arrive. It
// is the implementation; ProcessAction is the same pipeline without a listener.
// A non-nil error from onChunk aborts the turn before it is recorded, which is how
// a client that has disconnected stops generation instead of letting it finish
// into nothing.
func (o *TurnOrchestrator) ProcessActionStream(ctx context.Context, mode, actionInput string, onChunk func(text string) error) (*Turn, error) {
	// … identical to ProcessAction up to and including context assembly …
}
```

The pipeline is unchanged apart from generation:

1. History load, `/undo`, `/gm`, `/go`, roll evaluation, and mechanics hooks behave exactly as they do today. The `/undo`, `/go`, and `/gm` paths return before generation and therefore emit no chunks.
2. Generation uses `router.StreamForRole(ctx, "gm", req, chunks)`. The orchestrator drains the channel to completion, forwarding each chunk's text to `onChunk` (when non-nil) while accumulating it, and treats a `StreamChunk.Error` as the turn's failure. A non-nil error from `onChunk` aborts generation immediately, so a disconnected client does not leave a model running to no purpose. `StreamForRole` already falls back to another provider when the first chunk errors, so the orchestrator does not need to know about fallbacks.
3. Cancellation is checked before the timeline write, so a cancelled or failed stream leaves history, the index, and every entity note untouched:

```go
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("turn cancelled: %w", err)
	}

	if err := o.timeline.RecordTurn(&turn, extraction.Entities); err != nil {
		return nil, fmt.Errorf("record turn: %w", err)
	}
```

4. Everything after accumulation — entity mentions, extraction, segment building, `RecordTurn` — is what the existing non-streaming path already does, unchanged.

`ProcessAction` keeps its signature for the TUI and its existing tests:

```go
func (o *TurnOrchestrator) ProcessAction(ctx context.Context, mode, actionInput string) (*Turn, error) {
	return o.ProcessActionStream(ctx, mode, actionInput, nil)
}
```

---

## 5. Shared Provider Setup

Two helpers move out of `package main` into `pkg/harness/factory.go`, the file that already builds providers, where both clients can use them:

```go
// RouterFromConfig builds the role-routed provider registry a turn needs.
func RouterFromConfig(cfg *config.Config) (*Router, error)

// ExtractorFromConfig resolves the per-turn extractor role, honouring `inherit`
// and `disabled`, and returning nil when extraction is off.
func ExtractorFromConfig(cfg *config.Config, router *Router) *Extractor
```

`RouterFromConfig` is `cmd/localrpg/play.go`'s current loop: build a provider per configured role, assign each role to its own provider, apply the configured fallbacks, and register a default echo provider for `gm` when nothing else is configured. `ExtractorFromConfig` is the `resolveExtractor` helper the attribution spec introduced, moved verbatim.

`play.go` then becomes wiring rather than duplication, and `gui.Service` builds the same router for a turn.

---

## 6. The Service

```go
// RunTurn plays one turn, emitting events as they happen. It is the only place
// that writes a turn from the GUI.
func (s *Service) RunTurn(ctx context.Context, gameID string, req TurnRequest, emit func(TurnEvent) error) error
```

- **Serialisation.** `Service` keeps `locks map[string]*sync.Mutex` beside its existing `indexed` map. A non-blocking `TryLock` that fails returns `ErrTurnInFlight`, which the route maps to `409`. The lock is process-wide for this service, which is what makes the guarantee real for one client; a TUI playing the same campaign at the same moment is out of its reach and is documented rather than pretended about.
- **Construction per request.** The service repairs the index once per process (`ensureIndexed`), loads the game manifest, opens the pooled store, builds a `Timeline`, and wires it exactly as the CLI does: voice profiles from `cfg.Media.TTS.VoiceProfiles` so newly extracted NPCs get archetype voices, the router from the current config, the extractor through `ExtractorFromConfig`, and the orchestrator with the manifest's pinned `start_location` as its fallback. Skipping any of that would make a turn played in the desktop app behave differently from the same turn played in a terminal, which is the kind of divergence a second client is expected to avoid. A handful of file reads buys the guarantee that a settings change or a note edit takes effect on the next turn rather than never — the failure mode a cached orchestrator has already produced twice in this codebase.
- **The final DTO is the chronicle's DTO.** `GetChronicle`'s per-turn mapping is extracted into a helper both callers use, so a freshly played turn and the same turn read back an hour later are byte-identical in shape. The client therefore has one rendering path, not two:

```go
// turnDTO maps a persisted turn for the API. GetChronicle and RunTurn share it so
// a live turn and a replayed one are the same shape.
func (s *Service) turnDTO(turn engine.Turn, store *storage.Store, cfg *config.Config) TurnDTO
```

- **Preflight and failure classification.** `BeginTurn` takes the per-game lock and prepares the campaign (manifest, system, world, rules script), so `ErrTurnInFlight` (409) and an unprepareable campaign (503) are known before the first byte, along with the route's own `400`/`404`. Everything after the first byte becomes an `error` event — including a `gm` provider that is configured as `disabled`, because `RouterFromConfig` always registers an echo fallback for `gm` and a disabled provider is a turn-time failure, not a startup one.
- **Event framing.** Events are written as one JSON object plus `\n` and flushed; a framing failure (the client disconnected) cancels the context, which cancels generation, which means no turn is recorded.

---

## 7. The Client

`frontend/src/api/client.ts` gains one method:

```typescript
export interface TurnEvent {
  type: 'chunk' | 'turn' | 'error';
  text?: string;
  turn?: Turn;
  message?: string;
}

static async streamTurn(
  gameID: string,
  body: { mode: string; input: string },
  onEvent: (event: TurnEvent) => void,
  signal?: AbortSignal
): Promise<void>
```

It posts the request, reads `response.body` through a `TextDecoder` and a `getReader()`, buffers by newline, and parses each complete line. It must not assume chunks arrive progressively: a buffered body simply produces every line at once, and the caller sees the same events in the same order. Non-2xx responses are read as text and thrown with their status, so `409` and `503` surface the server's explanation.

`ActionConsole` becomes the console that submits:

- Enter submits with the current mode and input; the input is disabled and the mode is locked while a turn is in flight.
- The `chunk` events stream into a transient narration block rendered with the existing `TurnSegments` (a single narration segment), so the prose appears as it is written.
- The `turn` event replaces the transient block with the persisted turn, which brings its real segments, speaker labels, entity chips, location, outcome, and audio controls — all of which the chronicle already renders.
- `error` events and request failures show the message and restore the input, so a failed turn is retryable without retyping.
- A Stop control aborts through an `AbortController`. The server sees the disconnect, cancels generation, and records nothing.
- On success the turn is also folded into the chronicle's cached list, and the console runs the same playback the chronicle does, so a played turn sounds the same as a replayed one.

---

## 8. Component & File Map

| File | Change |
| --- | --- |
| `pkg/engine/orchestrator.go` | `ProcessActionStream`, `ProcessAction` as a wrapper, cancellation before the timeline write |
| `pkg/harness/factory.go` | `RouterFromConfig`, `ExtractorFromConfig` moved from `cmd/localrpg` |
| `cmd/localrpg/play.go` | Uses the shared factories; `resolveExtractor` removed |
| `cmd/localrpg/play_resolver_test.go` | Moves to `pkg/harness` with the helper it tests |
| `pkg/gui/service.go` | `RunTurn`, per-game locks, `turnDTO` extracted from `GetChronicle`, `ErrTurnInFlight` |
| `pkg/gui/server.go` | `POST /api/game/{id}/turn` with NDJSON framing and status mapping |
| `pkg/gui/types.go` | `TurnRequest`, `TurnEvent` |
| `frontend/src/api/client.ts` | `streamTurn` with incremental line parsing |
| `frontend/src/components/ActionConsole.tsx` | Submit, stream, Stop, error handling, fold into the chronicle |
| `frontend/src/types.ts` | `TurnEvent` |
| `AGENTS.md` | The GUI can play turns; the endpoint and its serialisation rule |

---

## 9. Compatibility & Edge Cases

- **The endpoint is additive.** Existing routes and DTOs are unchanged; `turnDTO` extraction is a refactor with the same output, pinned by the chronicle's existing tests.
- **A buffered client** sees one batched burst of events instead of a progressive one. Nothing in the design depends on timing, so this is a latency difference, not a behaviour difference.
- **A client that disconnects mid-stream** cancels generation and records nothing. The per-game lock is released on every path, including cancellation and panic-free error returns, through `defer`.
- **A campaign played from a terminal at the same time** is unguarded; the last writer wins on turn numbering. Documented, not fixed here, because the fix belongs in the timeline's own locking and applies to both clients.
- **Empty `input` with mode `System`** is rejected by the same `400` as any empty input; `/undo` is not a special case at the API layer.
- **Request bodies are capped** with `http.MaxBytesReader` (64KiB), so a runaway paste cannot make the handler allocate without bound. The cap is far above any plausible player action.
- **A `gm` provider that is disabled**, or any provider failure, streams an `error` event rather than a status, because the response has already begun. The client shows the message and keeps the input, so the turn is retryable once a provider is configured.

---

## 10. Verification & Testing Plan

**`pkg/engine`**
- `ProcessActionStream` and `ProcessAction` produce the same turn for the same mock provider, across every mode (`Do`, `Say`, `Story`, `Roll`, `GM`, and a `/gm` directive).
- Chunks are delivered in order and concatenate to exactly the narration stored on the turn.
- A cancelled context leaves `history.jsonl` byte-identical, the index unchanged, and entity notes untouched.
- A provider that fails mid-stream (a mock emitting one chunk then an error) records nothing.
- `/undo`, `/go`, and a system turn complete without emitting any chunk.

**`pkg/harness`**
- `RouterFromConfig` registers a provider per configured role, applies fallbacks, and falls back to the echo provider for `gm` when the config has none.
- `ExtractorFromConfig` honours `inherit`, `disabled`, a missing role, and a missing inherit target — the tests relocated from `cmd/localrpg`.

**`pkg/gui`**
- A turn posted to the endpoint returns NDJSON whose lines all parse, ending with a `turn` event whose DTO carries the narration, location, segments, and entities.
- The same turn is readable from `GetChronicle` and is identical in shape, which is what makes the shared `turnDTO` observable.
- A second concurrent request for the same campaign returns `409` and does not block the first; the first completes and records its turn.
- `400` for an unknown mode and for empty input, `404` for an unknown game, `503` when the campaign cannot be prepared (its system or world is missing).
- A client that stops reading the body cancels the turn and leaves nothing recorded.
- The per-game lock is released after success, after failure, and after cancellation.
- Streaming does not break the existing routes: `go test ./pkg/gui/` covers both.

**Frontend**
- `cd frontend && npx tsc --noEmit` and `npm run build` pass.
- Manual check recorded in the plan: play a turn in the desktop window with a local provider, confirm prose appears, the turn lands in the chronicle with its audio controls, and Stop leaves no turn behind.

**Project**
- `go vet ./...`, `mise run test`, and every commit builds standalone in a scratch worktree.

---

## 11. Phased Delivery

1. **Streaming pipeline** — `ProcessActionStream` with `ProcessAction` as its wrapper, cancellation before persistence, and the equivalence and cancellation tests.
2. **Shared factories** — `RouterFromConfig` and `ExtractorFromConfig` in `pkg/harness`, with `play.go` and its tests repointed.
3. **The endpoint** — `RunTurn`, per-game locks, the shared `turnDTO`, NDJSON framing, and status mapping.
4. **The console** — `streamTurn`, submit, stream, Stop, error handling, and folding the new turn into the chronicle.
5. **Docs** — `AGENTS.md`, and the manual desktop check recorded.

---

## 12. Open Questions

None outstanding. One fact to confirm during Phase 4 rather than assume: whether the Wails webview delivers a streamed body progressively. The design does not depend on the answer — a buffered body produces the same events in the same order, just later — but the manual check records which it is, because it decides whether the desktop app shows prose as it is written or in one burst.
