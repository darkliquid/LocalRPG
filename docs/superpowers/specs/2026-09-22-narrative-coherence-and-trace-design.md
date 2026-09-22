# Design Specification: Narrative Coherence and Operational Trace

**Date:** 2026-09-22
**Status:** Draft — pending review
**Topic:** Make a long campaign hold together, and make the system's own behaviour visible: layered recall with a bounded prompt, canon repair, deterministic continuity checks, and a trace of every prompt, call, and generation

---

This spec builds on `docs/superpowers/specs/2026-09-22-gameplay-experience-design.md`,
which introduced the configurable context budget, the recall window, and the
`ContextNotes` reporting this design extends.

## 1. Problem Statement & Motivation

Coherence across turns is the feature that decides whether a campaign feels like a story or a sequence of unrelated scenes. Today it rests on exactly one mechanism, plus whatever the model happens to remember.

What `AssembleContextWithProfiles` currently sends:

1. rules, lore, formatting and continuity instructions
2. the voice catalogue
3. **recent events**: the last N turns verbatim
4. immediate scene: location name and body, player name and state
5. living world and present characters: entities **one edge** from the current location

Everything else the engine knows is dropped. Concretely, with file references:

| Leak | Where | Consequence |
| --- | --- | --- |
| Entity `state` never reaches the GM | `pkg/harness/context.go:41,51` send `Body` only | A guttering brazier, `fuel_reserves: 3`, an arc's `clock_ticks` are all invisible; the GM contradicts the state the engine is tracking |
| Prior turns *at this location* are not recalled | `formatRecentTurns` | Returning somewhere feels like arriving for the first time |
| Entities carry `history: []int` (turn numbers) | `pkg/entity/entity.go` | Used only by the Codex drawer, never by the prompt |
| Anything older than the window is gone | `recent_turn_window` | Turn 3 is unrecoverable by turn 30 |
| No aliases | `MatchExistingEntity`, `Extractor` | "Guard Kael" and "The Ember Warden" became two characters for one being |
| No verification of what the model produced | — | Drift is only noticed by the player |

The second half of this spec is the reason the first half is hard to debug: there is **no logging at all**. The exact prompt, the bytes sent to the provider, the raw generation before segmentation, the extractor's response, and the media requests are all unobservable. Diagnosing the coherence defects above required reading `history.jsonl` and inferring. Every `host` in this codebase currently reaches the user as a symptom.

## 2. Goals & Non-Goals

**Goals**

1. A name, place, or fact established in turn 1 is still honoured in turn 50.
2. The GM can see the state the engine tracks, not just prose.
3. What the GM is sent is bounded, prioritised, and explainable.
4. Drift is detectable and repairable by the player.
5. Any turn's behaviour can be reconstructed after the fact from a local trace.
6. Coherence strengthens as a campaign grows rather than decaying.

**Non-Goals**

- A vector database or embeddings. Retrieval uses the index already present.
- Rewriting history or "fixing" past turns.
- Vectorising or exporting traces as a product feature.
- Automatic correction of continuity without the player's say-so (v1 reports).
- A general metrics/telemetry system. The trace is for one operator on one machine.
- Changing the schema-agnostic engine: canon is still opaque YAML state.

---

## 3. Decisions

| Area | Decision |
| --- | --- |
| Coherence model | Four layers: **canon** (authoritative state), **recall** (what happened), **repair** (fixing drift), **verification** (noticing drift) |
| Canon | Entity `state` and established names are rendered compactly for scene, present characters, and active arcs |
| Scene recall | Prior turns at the current location, from `turns.location`, newest first, capped |
| Long memory | A rolling `story so far` summary note, regenerated every N turns, injected compactly |
| Retrieval | Past turns sharing entities with the entities in play, ranked by overlap, from `turn_entities` |
| Budget | Every new section participates in the existing budget; the compressed summary is dropped **last** |
| Aliases | `aliases: []` frontmatter, honoured by matching and by the prompt |
| Merge | An explicit player action that folds one note into another and rewrites inbound links |
| Continuity check | Deterministic rules only in v1; no extra model call |
| Trace | Structured JSONL in a **single appended file** in the cache dir, off by default, with levels; the file is the source of truth |
| Trace levels | `off`, `summary` (decisions and sizes), `full` (payloads and per-chunk detail) |
| Redaction | A deny-list of field names; secrets are never written, at any level |
| Trace UI | A Debug drawer reading the file, polled while a turn is in flight; no new streaming protocol |
| Chronicle note | A normal entity (`type: chronicle`), player-visible and editable, excluded from canon, recall, retrieval, and edge scanning by type |
| Summary cadence | Every N turns, plus a location change; never on an idle thread |
| Summary authority | Stated as recollection, explicitly subordinate to canon and the notes |
| Continuity findings | On by default, display-only, with a one-click correction offered on the finding |
| Retrieval query set | Recent speech speakers and recently mentioned characters, **excluding the player and the location**, which other sections already cover |
| Summary in the budget | Summoned last, and only for campaigns whose history exceeds the recall window |
| Trace cost model | Opt-in: `off` by default, so retention bounds guard against a debug session left running, not against normal play |
| Retrieval ranking | Overlap weighted by recency decay, then turn number as a tie-break |
| Chronicle in the graph | Hidden. The Codex is where it is edited; an isolated node is furniture |
| Summary timing | Detached from the turn that triggered it, deliberately not used by that turn, so it can never add latency or damage a turn |
| Findings on record | `history.jsonl` stays append-only; addressed findings are marked in a per-campaign sidecar |
| Corrections feed the summary | A correction is the player stating a fact, which is what the summary is for |
| Untrimmed prompt | Recorded at `full` whenever the budget trimmed, because "what did I lose" is the likeliest reason to be tracing |
| Wire fidelity | Both: parsed chunks as consumed, plus raw provider lines under `provider.wire`, so a parse bug is distinguishable from a model bug |

---

## 4. The Coherence Model

Four distinct jobs, which were previously conflated into "put the last few turns in the prompt":

- **Canon** answers *what is true*. It is authoritative, small, and must be present every turn. State and names live here. It is also the only layer that is never trimmed, because every line of it is a constraint the model would otherwise have to guess.
- **Recall** answers *what happened*. It is bulky, ranked, and can be trimmed.
- **Repair** answers *the model got it wrong*. It is a player action: aliases and merges.
- **Verification** answers *the model is drifting now*. It is a deterministic check whose findings are reported on the turn.

The layers are independent: a campaign with perfect canon still needs recall for texture; perfect recall still needs repair when the model renames someone.

---

## 5. Context Assembly v2

The assembler gains sections. Each has a fixed position in the output, a trim rank, and a share of the budget.

| Order in prompt | Section | Trim rank (1 = dropped first) | Config |
| --- | --- | --- | --- |
| 1 | Rules | never | — |
| 2 | Lore | never | — |
| 3 | Formatting and continuity instructions | never | — |
| 4 | Canon: scene, player, present characters, arcs, open threads | never | — |
| 5 | Established names | never | — |
| 6 | Story so far | 6 | `agents.summary_char_limit` |
| 7 | Recent events | 4 | `agents.recent_turn_window`, `agents.recent_turn_char_limit` |
| 8 | What happened here (scene recall) | 3 | `agents.scene_recall_turns`, `agents.scene_recall_chars` |
| 9 | Relevant history (retrieval) | 2 | `agents.retrieval_turns`, `agents.retrieval_chars` |
| 10 | Voice catalogue | 1 | — |
| 11 | Player action | never | — |

`AssembleContext` is split accordingly: it currently returns one block containing the scene, the world, and the player's action, which is why canon cannot sit before recall today. It becomes `AssembleCanon` (sections 4 and 5) and the player action (section 11), with the action rendered last because it is the request the model is answering and everything above it is context for that request. The voice catalogue moves to just before the action; it is instructions about how to voice NPCs, so it reads better next to the instruction block than between the rules and recall as it does today.

Drop order is by trim rank: the catalogue goes first, retrieval next, then scene recall, then the oldest remembered turn, then shorter excerpts, and the summary is the last thing surrendered. The summary is kept longest because it is the cheapest continuity per token.

### 5.1 Canon

`AssembleContext` renders each entity as a compact fact line rather than prose alone:

```text
**Current Location:** The Ashen Bastion
A fortified granite sanctuary above the peat bogs.
State: danger_level=2, brazier_lit=true
```

Present characters and arcs gain the same treatment, and the arc's clock is stated in words (`Clock: 2/6`), because a bare `{clock_ticks: 2}` in the prompt is what the model ignores.

A new `## ESTABLISHED NAMES` block lists every character, location, and arc by canonical name plus any alias, so the model has a vocabulary to reuse instead of inventing one.

Rules:

- State is rendered only when non-empty; a note with no state reads exactly as it does today.
- Values are rendered with `%v` in a stable key order, so the same note produces the same prompt (testable, cacheable).
- Canon is never trimmed. It is small and every line is a constraint.

### 5.2 Scene recall: what happened here

New store query `TurnsAtLocation(locationID string, beforeTurn, limit int) ([]TurnRecord, error)`, reading `turns.location`. Rendered as:

```text
## WHAT HAPPENED HERE (this location)
Turn 4: The brazier guttered while the wardens argued.
Turn 7: Kael refused to open the gate.
```

- Newest first, capped at `agents.scene_recall_turns` (default 4) and `agents.scene_recall_chars` (default 800 per turn).
- Excludes turns already in the recent window, so the same text is not sent twice.

### 5.3 Story so far: long memory

Stored as a normal note, so it is readable, editable, and exportable: `games/<id>/entities/chronicle.md` with frontmatter `type: chronicle`, `state: {through_turn: N}`.

- **Regeneration trigger**: `turnNum - through_turn >= agents.summary_every` (default 10).
- **Input**: the previous summary body plus the turns `through_turn+1..turnNum`.
- **Timing**: regenerated **detached** from the turn that triggered it, and never used by that turn. The turn records and returns; the summary updates behind it and the next turn sees it. This keeps a second model call off the critical path and means a failed summary can never damage a turn. Regenerations coalesce: a pending regeneration is a flag, not a queue, so a player turning quickly triggers one catch-up run, not several.
- **Provider**: the **extractor** role (settled). It is already configured, already cheap, and already reads turns. It falls back to `gm` through the existing `inherit` mechanism, and setting `agents.roles.extractor: disabled` disables summaries along with extraction.
- **Instruction**: preserve names, places, promises, unresolved threads, and state changes; drop verbatim dialogue; never invent.
- **Injection**: `## STORY SO FAR`, capped at `agents.summary_char_limit` (default 2000 characters).
- **Failure policy**: a failed summarisation never loses a turn. The note is left alone, `through_turn` is not advanced, and the turn proceeds. The next trigger retries.
- **Repair**: if the note is missing, or `through_turn` exceeds the log length, it is rebuilt from the last `2 * agents.recent_turn_window` turns.
- **Player-visible**: `GET /api/game/{id}/recap` returns the text, and `/recap` prints it (section 8).

The summary is the only place where lossy compression is acceptable, which is why it is a separate note the player can edit rather than something hidden in the database.

### 5.4 Retrieval: relevant history

Query entities = current location ∪ speakers in the last window ∪ entities mentioned in the last window.

New store query `TurnsMentioningEntities(entityIDs []string, excludeFromTurn, limit int) ([]TurnRecord, error)` over `turn_entities` (already indexed by `entity_id`). Ranking:

1. count of query entities mentioned
2. then recency

Rendered as:

```text
## RELEVANT HISTORY
Turn 6: Kael mentioned that the oil reserves were low.
```

Excludes the recent window and the scene recall selection, so nothing appears twice. This is a precision tool: it recovers the turn where a promise was made, which no fixed window can do.

### 5.5 Budget interaction

`AssembleResult.Trimmed` already reports what was dropped, and the turn carries it as `ContextNotes`. Every new section reports there too, by name, so a thin reply is explainable. `AssembleResult` gains per-section token counts, surfaced in the trace (section 9) rather than the UI:

```go
type SectionStat struct {
	Name     string
	Tokens   int
	Included bool
}
type AssembleResult struct {
	Prompt          string
	EstimatedTokens int
	Trimmed         []string
	Sections        []SectionStat
}
```

---

## 6. Canon Repair

### 6.1 Aliases

- Frontmatter gains `aliases: []`.
- `harness.MatchExistingEntity` consults aliases after ID and name, before token overlap, so "The Ember Warden" resolves to `guard-kael`.
- `ResolveSpeakerID` consults aliases too, which fixes speech attribution when the model uses the other name.
- Aliases are listed in `## ESTABLISHED NAMES`, so the model is told both names are one being.

### 6.2 Merge

`POST /api/game/{id}/entity/{source}/merge` with body `{"into": "<target>"}`:

1. append the source body to the target body, separated by a blank line
2. union `tags`, `aliases`, and `history`, and merge `state` (target wins on conflict)
3. rewrite `[[source]]` and `[[source|label]]` to the target in every other note
4. delete the source note, resync the index
5. return the merged entity

This is an explicit player action with no automatic equivalent, because deciding two names are one being is a judgement the engine should not make silently. The Codex drawer offers it as "Merge into..." with a target picker.

---

## 7. Continuity Checking

After generation and before recording, deterministic rules run over the narration and the segments. No model call: the value is in being cheap enough to run every turn and predictable enough to trust.

| Rule | Detection | Note |
| --- | --- | --- |
| Unknown entity | A capitalised multi-word proper noun matching no entity, alias, or stop-word list | "The Ashen Bastion" introduced with a different name |
| Location drift | Narration names a known location other than the current one, and the turn recorded no move | The party is teleported by prose |
| Unresolved speaker | A `Name: "..."` line whose speaker did not resolve | The line silently became narration |
| Reintroduction | An established character introduced with discovery phrasing | "A stranger named Kael..." for a known Kael |
| State contradiction | Narration asserts a state value contrary to a boolean or numeric state the note holds | "The brazier was cold" when `brazier_lit: true` |

Findings become `Turn.ContinuityNotes []string`, rendered under the turn like `ContextNotes`, with a **Correct** action that submits the finding as a `/gm` directive on the next turn. Nothing is auto-rewritten.

Because `history.jsonl` is append-only, addressing a finding does not edit the turn. `games/<id>/findings.json` records `{turn, rule}` pairs the player has addressed, so the UI can render them as handled while the record itself stays intact. Dismissing is per finding, never per rule, so a rule that becomes noisy is tuned rather than muted. The correction turn is ordinary history, so it reaches the next summary without special handling.

Deliberately deferred: an advisory model pass that reads the notes and the narration and reports contradictions in prose. It costs a call per turn and its output is harder to trust; if it is added later it belongs behind its own config flag.

---

## 8. Threads and Recaps

- Arc notes gain `state.status` (`open`, `complicated`, `resolved`) and `state.last_advanced` (a turn number).
- `## OPEN THREADS` lists unresolved arcs with how long they have been idle, so a dropped thread is visible to the model as an omission rather than being silently forgotten.
- The extractor reports which arcs a turn advanced, and the timeline writes `last_advanced`.
- `GET /api/game/{id}/recap` returns the story-so-far text plus open threads; the GUI shows it in a panel, and `/recap` prints it in the TUI.
- A campaign that has an arc idle for more than `agents.thread_idle_turns` (default 10) shows it as a nudge in the UI, not in the prompt.

---

## 9. Operational Trace

### 9.1 Event catalogue

Every event is a name plus structured fields. Values are typed; payloads are truncated at a configurable cap.

| Event | Fields |
| --- | --- |
| `config.load` | path, local_override, keys_present |
| `turn.begin` | game, number, mode, input_chars |
| `context.assembled` | estimated_tokens, budget, sections[{name, tokens, included}], trimmed[], recall_turns, retrieval_turns, untrimmed_prompt (full only, and only when trimming occurred) |
| `provider.request` | role, provider_id, kind, model, temperature, max_tokens, endpoint, prompt_chars, prompt (full only) |
| `provider.response` | role, finish_reason, first_token_ms, total_ms, chunks, bytes, usage{prompt_tokens, completion_tokens} when the provider reports it |
| `provider.error` | role, error |
| `generation.complete` | narration_chars, finish_reason, truncated |
| `segment.build` | count, kinds[], speakers[], unresolved[] |
| `extraction.request` | role, prompt (full only) |
| `extraction.result` | entities[], dialogue[], player_location, matched[], created[] |
| `continuity.check` | findings[] |
| `provider.wire` | role, direction, line, truncated (full only; bounded by the payload cap) |
| `record.turn` | number, location, entities, outcome, bytes |
| `summary.regenerate` | from_turn, to_turn, provider, chars, duration_ms |
| `media.tts.request` | speaker, voice_id, pitch, rate, chars, cache_key |
| `media.tts.result` | cache_hit, bytes, content_type, duration_ms |
| `media.stt.request` / `.result` | bytes, chars, duration_ms |
| `media.image.request` / `.result` | prompt, cache_key, bytes, duration_ms |
| `audio.play` | clips, volume, total_ms |
| `lsp`-style summary lines are **not** traced: this is not a metrics system |

### 9.2 Levels

| Level | Contains | Use |
| --- | --- | --- |
| `off` (default) | nothing | Normal play |
| `summary` | Every event, with payload fields omitted: sizes, decisions, timings, errors | "Why was that reply thin / slow / wrong?" |
| `full` | As `summary`, plus prompts, provider request bodies, generated text, and chunk-level provider events (capped at `agents.trace_chunk_limit`, default 500 per turn) | "What exactly did we ask for?" |

Config: `preferences.trace_level`. A CLI flag `--trace <level>` overrides it for one run, so the TUI can be debugged without editing config.

### 9.3 Sinks

1. **File** (canonical): `<cache>/trace/trace.jsonl`, mode `0600`, one JSON object per line, appended across sessions so `tail -f` works without hunting timestamps. Every line carries its own identity, because the file is no longer per-session:

```json
{"ts":"2026-09-22T09:14:03.221Z","event":"context.assembled","run":"9f3c1a","game":"test-campaign","level":"full","tokens":2610,"budget":0,"sections":[…]}
```

   `run` is a per-process identifier, so one process's events can be separated without a second file. A buffered writer is flushed per event, so a crash keeps what was seen.
2. **stderr** when `--trace` is passed to a CLI command.
3. **GUI** reads the file; nothing is streamed over the turn protocol.

Rotation: when the file passes `agents.trace_max_bytes`, it is renamed to `trace.jsonl.1` (shifting older files), and the newest `agents.trace_max_files` are kept. Rotation is checked on open and every `agents.trace_rotate_check` events, not on every write, so the hot path stays a buffer append. An appended file has no natural age, so size is the only honest bound.

At `full`, the assembled prompt is recorded **once**, on `context.assembled`; `provider.request` carries its hash, its length, and the envelope that actually went on the wire. This is about diagnostic redundancy rather than size: two copies of the same text add no information, and give a reviewer two records that can disagree.

Tracing is opt-in. `off` is the default and writes nothing, so the bounds below exist to stop a debug session left enabled for days from filling a disk, not to ration normal play. They can afford to be generous, and a user who wants a week of traces should raise them without guilt.

A single `trace.Logger` interface is passed down explicitly rather than reached for globally:

```go
package trace

type Level int
const (LevelOff Level = iota; LevelSummary; LevelFull)

// Logger records one structured event. Implementations must never block a turn.
type Logger interface {
	Enabled(Level) bool
	Event(name string, fields map[string]any)
}

// Nop is the logger used when tracing is off, so no caller branches on nil.
func Nop() Logger
```

Providers, the assembler, the orchestrator, the timeline, and the media clients take a `trace.Logger`. `Nop` keeps every call site branch-free and keeps tests trivial.

### 9.4 Redaction and retention

- A deny-list of field names (`api_key`, `authorization`, `token`, `secret`, `password`) is replaced with `[redacted]` before writing, at every level. The HTTP provider logs `auth_set: true`, never the header value.
- Prompts and generated prose **are** captured at `full`, because that is the point. They are the player's own story, they stay in the local cache directory, and the file is `0600`.
- Retention: rotate at `agents.trace_max_bytes` (default 256 MiB, roughly 10 000 turns at `full`) and keep `agents.trace_max_files` (default 3), so the ceiling is 768 MiB. These are deliberately generous: tracing is opt-in, and a ceiling nobody reaches costs nothing.
- A per-event payload cap (`agents.trace_payload_chars`, default 20000) truncates with a marker, so one runaway prompt cannot fill the disk.
- Tracing failure is never fatal: a trace that cannot be written is dropped, and the turn continues.

### 9.5 API and UI

| Method | Path | Result |
| --- | --- | --- |
| `GET` | `/api/trace` | `?limit=N&game=id&level=` recent events, newest last |
| `DELETE` | `/api/trace` | clears the trace, optionally filtered to one campaign by rewrite |
| `GET` | `/api/game/{id}/recap` | story so far and open threads |

A **Debug** drawer shows the most recent turn's events as a timeline: context sections and their token cost, the prompt (collapsible, copyable), the provider request and timings, the raw generation next to the parsed segments, the extraction result, continuity findings, and media calls. It polls `/api/trace` once a second while a turn is in flight and otherwise reads on open.

Settings Studio gains, under Preferences: trace level, payload cap, and retention.

---

## 10. Data & API Changes

| Change | Shape |
| --- | --- |
| `Turn.ContinuityNotes` | `[]string`, persisted like `ContextNotes` |
| `Turn.ContextNotes` | already shipped; gains the new section names |
| Arc state | `status`, `last_advanced` |
| Entity frontmatter | `aliases: []` |
| `chronicle.md` | `type: chronicle`, `state.through_turn` |
| `harness.AssembleResult` | gains `Sections []SectionStat` |
| `TurnDTO` | gains `continuity_notes` |
| `storage.Store` | `TurnsAtLocation`, `TurnsMentioningEntities` |

## 11. Config Changes

| Key | Default | Meaning |
| --- | --- | --- |
| `agents.scene_recall_turns` | 4 | Turns recalled at the current location |
| `agents.scene_recall_chars` | 800 | Excerpt cap for one recalled turn |
| `agents.retrieval_turns` | 3 | Turns retrieved by entity overlap |
| `agents.retrieval_chars` | 800 | Excerpt cap for one retrieved turn |
| `agents.summary_every` | 10 | Turns between summary regenerations |
| `agents.summary_char_limit` | 2000 | Injected summary cap |
| `agents.thread_idle_turns` | 10 | When an open thread is considered idle |
| `agents.continuity_checks` | `true` | Run the deterministic checks |
| `preferences.trace_level` | `off` | `off`, `summary`, or `full`; `--trace` defaults to `full` |
| `agents.trace_payload_chars` | 20000 | Per-event payload cap |
| `agents.trace_max_bytes` | 268435456 | Rotate the trace at this size |
| `agents.trace_max_files` | 3 | Rotated trace files retained |
| `agents.trace_rotate_check` | 200 | Events between rotation checks |

All defaulted, so existing configuration is unchanged. Every key is also exposed in the Settings Studio.

## 12. Testing Strategy

- **Prompt composition**: each section appears when enabled and is absent when its data is missing; drop order is asserted by shrinking a budget until each section disappears in turn.
- **Canon**: an entity with state renders that state; an entity without state renders exactly as before; key order is stable across runs.
- **Scene recall**: returns only turns at that location, newest first, excluding the recent window.
- **Retrieval**: ranks by overlap then recency, and never repeats a turn already present.
- **Summary**: regenerates at the cadence, survives a provider failure without losing a turn, rebuilds when `through_turn` is invalid, and is never injected past its cap.
- **Aliases and merge**: matching resolves an alias; merging unions history and aliases, rewrites inbound links, and removes the source note.
- **Continuity rules**: one table-driven test per rule, including the false-positive fixtures that must stay silent.
- **Coherence regression**: a scripted ten-turn provider that must reuse a name introduced in turn 1, honour a state change made in turn 3, and recall a promise made in turn 2 while that turn is outside the window.
- **Trace**: a turn emits the expected event sequence; `api_key` never appears in any file; `off` writes nothing; a payload over the cap is truncated with a marker; a write failure does not fail the turn.

## 13. Migration & Compatibility

- New config keys default to today's behaviour; a campaign with no `chronicle.md` simply has no summary block.
- `aliases` is optional metadata; notes without it behave as now.
- The immediate-scene layer renders identically for notes with no state, so existing prompts stay recognisable.
- Trace files live under the disposable cache directory; deleting them costs nothing.
- `TurnDTO` additions are additive; a client that ignores them is unaffected.

## 14. Open Questions

Settled in review:

- **First round**: summary provider is the extractor; the trace is a single appended file; the chronicle is an entity; cadence includes a location change; the summary is subordinate; continuity checks are on by default, display-only plus a correction; retrieval excludes the player and the location; the summary surrenders last and only past the window.
- **Second round**: retrieval ranks by recency-weighted overlap; the chronicle is hidden from the graph; summary regeneration is detached and never used by its triggering turn; addressed findings live in a per-campaign sidecar; corrections feed the summary; the untrimmed prompt is recorded when trimming occurs; raw provider lines are recorded alongside parsed chunks; trace rotation is 256 MiB across 3 files and `--trace` means `full`.

Queued for the third round:

1. **Finding dismissal scope.** Per finding, or a per-rule mute, and where the sidecar lives.
2. **Debug drawer shape.** Interleaved wire lines, or a separate panel, and how their volume is bounded in the UI.
3. **Graph filter.** Hide `chronicle` only, or arcs too.
4. **Recency decay shape.** Linear, or a half-life knob, and its default.
5. **Coalescing.** Confirm one pending regeneration rather than a queue.

## 15. File Map

**Create**

- `pkg/trace/trace.go` (+ test) — `Logger`, levels, JSONL sink, redaction, retention
- `pkg/harness/recall.go` (+ test) — scene recall, retrieval, summary rendering
- `pkg/engine/continuity.go` (+ test) — the deterministic rules
- `pkg/engine/summary.go` (+ test) — summary regeneration and the `chronicle` note
- `frontend/src/components/DebugDrawer.tsx` — the trace timeline
- `frontend/src/components/RecapPanel.tsx` — story so far and open threads
- `docs/superpowers/plans/2026-09-22-narrative-coherence.md` — the task plan

**Modify**

- `pkg/harness/context.go` — `AssembleCanon` split, sections, canon rendering, `Sections` stats
- `pkg/harness/extractor.go` — aliases in matching and `ResolveSpeakerID`
- `pkg/engine/orchestrator.go` — recall wiring, continuity checks, trace events, summary trigger
- `pkg/engine/history.go` — `ContinuityNotes`
- `pkg/engine/timeline.go` — arc `last_advanced`
- `pkg/storage/turn.go` — `TurnsAtLocation`, `TurnsMentioningEntities`
- `pkg/storage/db.go` — no schema change required
- `pkg/gui/service.go`, `server.go`, `types.go` — recap, trace, merge endpoints
- `pkg/config/types.go` — the new keys and accessors
- `frontend/src/components/SettingsStudio.tsx` — Preferences: trace; AI Agents: recall and summary
- `frontend/src/components/CodexDrawer.tsx` — merge action
- `frontend/src/components/ChronicleView.tsx` — continuity notes
- `cmd/localrpg/play.go`, `gui.go` — `--trace` flag, TUI `/recap`

## 16. Delivery Increments

1. **Trace** — `pkg/trace`, providers, assembler, orchestrator, media, file sink, `--trace`, Debug drawer. First because it makes every later increment diagnosable.
2. **Canon** — state rendering, established names, `SectionStat` and the budget interaction.
3. **Recall** — scene recall and retrieval, with their store queries and config.
4. **Summary** — the `chronicle` note, cadence, `/recap`, recap panel.
5. **Repair** — aliases, merge, Codex action.
6. **Verification** — continuity rules and their UI; open threads and idle nudges.

Each increment is independently testable and leaves a coherent product: increment 1 pays for itself immediately, 2 and 3 sharpen what is already sent, 4 extends the horizon, 5 and 6 keep it healthy.
