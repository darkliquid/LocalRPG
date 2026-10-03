# Progressive Turn Stream Design

**Date:** 2026-10-03
**Status:** Proposed
**Supersedes:** `2026-10-01-structured-gm-and-response-schema-design.md`,
`2026-09-25-structured-turn-protocol-design.md`
**Scope:** GM turn generation, response framing, speech attribution, mechanics
checks, turn persistence
**Related:** `pkg/dialogue`, `pkg/engine/orchestrator.go`,
`pkg/engine/segments.go`, `pkg/harness/turn_tools.go`,
`pkg/harness/context.go`, `pkg/gui/streaming_tts.go`

---

## 1. Overview & Problem Statement

The structured-turn work (PR #2) makes the GM author one terminal `submit_turn`
tool call holding the whole turn: segments, personae, memories, state changes,
and the action verdict. That shape has three costs:

1. **Nothing is usable until the call completes.** A single JSON object cannot be
   parsed until its closing brace arrives, so the client shows raw prose and the
   narrator's voice for the entire stream, then swaps to attributed segments at
   the end. Dialogue is only voiced with the right character after the turn ends
   (`pkg/gui/streaming_tts.go:73-75`).
2. **The terminal payload is the wrong shape for the data.** `personae` are
   needed *before* the speech that references them, or a new speaker cannot be
   attributed (or voiced) live. `state_changes` are produced *after* a roll.
   Neither belongs in a trailing block.
3. **It is all-or-nothing.** A malformed submission fails the whole turn and
   burns a retry (`pkg/engine/orchestrator.go:1716-1727`).

This design replaces the terminal payload with a **line-framed turn stream** that
is parsed progressively. Segments and named speakers become available as they
arrive; rolls become explicit terminators that hand control to the engine; and
declarations (personae, state, memories) appear in the stream at the point they
are needed.

## 2. Goals & Non-Goals

**Goals:**

- Parse the GM reply progressively: emit narration and attributed speech segments
  while the model is still writing.
- Attribute speech to a known speaker with a per-character voice during the
  stream, not only at finalise.
- Make a roll request end the response, so the engine (or the player) rolls
  deterministically and the model continues with the result.
- Move personae, memories, and state changes into the stream, at the point they
  are needed.
- Keep prose as the always-valid floor: a model that ignores the framing still
  produces a usable turn.
- Retain the extractor as the fallback for models that do not follow the framing.

**Non-Goals:**

- Migrating or rewriting stored turns. Existing `history.jsonl` records keep
  their stored segments and render unchanged.
- Removing `pkg/dialogue.Parse`. It remains the legacy quoted-speech parser used
  by media and by the fallback segment builder.
- A general templating or scripting language for the GM.
- Perfect model compliance. The framing is best-effort and degrades gracefully.

## 3. The Turn Stream

The GM reply is a sequence of newline-terminated lines. Every line belongs to
exactly one of three classes, decided by its leading characters, so the class is
known the moment the line's prefix arrives.

### 3.1 Line classes

| Class | Recognition | Meaning |
|-------|-------------|---------|
| Speech | Starts with `>`, then `Speaker: utterance` | A character speaks |
| Record | Starts with `@` followed by a record type and a JSON payload | A control message |
| Narration | Everything else, including blank lines | Prose |

Blank lines separate paragraphs. A blank line, a speech line, or a record line
flushes any pending narration paragraph.

The canonical speech form is a Markdown blockquote:

```
The harbour is quiet. Kaelen does not look up from the ledger.

> Kaelen: You didn't see me here.

Outside, a gull cries.
```

The `>` sigil is the discriminator. It is chosen over indentation deliberately:
indentation is whitespace, which models mangle and which collides with Markdown's
four-space code-block rule. A blockquote is a non-whitespace marker that prose
rarely opens a line with, it has a strong model prior from its use in fiction and
documentation, and it reads naturally as speech in the raw stream. The marker is
known at the first character of the line, so the class is decided before the
utterance arrives. The speaker name is complete at the colon, which is also
before the utterance finishes, so a speaker can be resolved and a voice selected
while the line is still streaming.

A `>` line whose prefix is not a resolvable `Speaker: utterance` is treated as
narration with the marker stripped, so an unattributed quote is never lost.

**Legacy compatibility.** A line that is not a blockquote but matches the legacy
quoted form `Name: "…"` (the grammar in `pkg/dialogue/dialogue.go:22`) is also
treated as speech when the speaker resolves. This keeps models that were prompted
for the old convention working, and keeps existing tests meaningful.

### 3.2 Records

A record is one line: `@<type> <json>`. The JSON payload is a single-line object.
Records are parsed best-effort: a malformed record is logged and skipped, and the
surrounding prose still parses. This is the resilience property that the terminal
payload lacked.

| Record | Payload | Effect |
|--------|---------|--------|
| `@persona` | `{"name","type","new","gender","pronouns","role_tags","description","voice_hint"}` | Declare a character. Emitted *before* their first line so the speaker is known in time. |
| `@roll` | `{"actor","target","check_kind","stat","difficulty","stakes","outcomes","notation"}` | Request a check. **Terminates the response.** |
| `@state` | `{"entity","path","op","value","reason"}` | A state change. |
| `@memory` | `{"kind","entity_refs","text","importance","tags"}` | A narrative memory. |
| `@move` | `{"location"}` | Move the player to a location. |

`@roll` reuses the existing `harness.CheckRequest` shape and the existing
`outcomes` map (outcome key -> result text), so the model pre-commits to what
each result means before the dice land.

### 3.3 Attribution and the roster

At turn start the engine builds a **roster**: a case-insensitive map from speaker
name and slug to entity ID and voice profile. It is seeded from the entity store,
the voice-profile catalogue, the player, and any `@persona` records seen so far
in this stream.

Resolution is a map lookup, not a query, so it is cheap enough to run at line
start. A speaker that resolves is attributed speech with that speaker's voice. A
speaker that does not resolve is narration, exactly as today, unless a `@persona`
record declares them first.

## 4. Architecture

### 4.1 The parser (`pkg/turnstream`)

A new package holds the framing. It has no dependency on `pkg/engine` or
`pkg/gui`; it depends on `pkg/entity` and a resolver interface.

```go
// Roster resolves a speaker name to an entity id, and reports the entity's voice.
type Roster interface {
    Resolve(name string) (id string, ok bool)
    Voice(id string) *entity.VoiceConfig
}

// Event is one parsed line: a segment or a record.
type Event struct {
    Kind      string // "narration" | "speech" | "record"
    Speaker   string
    SpeakerID string
    Text      string
    Record    *Record
}

type Parser struct { /* buffers a partial line */ }

func NewParser(roster Roster) *Parser
// Feed adds streamed text and returns every event the new text completed.
func (p *Parser) Feed(text string) []Event
// Flush returns the event for any trailing partial line at end of stream.
func (p *Parser) Flush() []Event
// Records returns the records seen so far, in order, for the turn's declarations.
func (p *Parser) Records() []Record
```

`Feed` buffers until a newline is seen, then classifies the line and emits zero
or more events. Narration lines accumulate into one pending narration event until
a flush boundary.

### 4.2 Orchestrator integration

`TurnOrchestrator` owns a parser for the turn, built with a roster it assembles
from the store. Inside the `onChunk` wrapper it feeds the parser and forwards
each event to a segment observer:

```go
func (o *TurnOrchestrator) SetSegmentObserver(func(turnstream.Event))
```

The GUI session wires the observer to a new `TurnEvent{Type: "segment"}`, so the
client renders segments as they arrive. Provisional speech events also carry the
speaker's voice, so `sentenceStreamer` can synthesize the line in the correct
voice instead of always using the narrator.

### 4.3 Finalise

The parser is the single source of truth for segments. At finalise, the
orchestrator calls `Flush`, resolves any speaker that was deferred (a `@persona`
declared later in the stream, or an extractor-created entity), and builds
`[]entity.TurnSegment` from the events. `buildTurnSegments` and the quoted-only
`dialogue.Parse` path remain only as the fallback when a reply produced no
framing at all.

The existing provisional-audio machinery is unchanged: a streamed sentence is a
cache hit at finalise, and the played-set reconciliation
(`streamedSpeech.playedKeys()`) prevents a line being heard twice.

### 4.4 Rolls as terminators

A `@roll` record ends the model's output for that call. The engine then:

- **auto policy:** rolls immediately via `resolveCheck`, then issues a
  continuation call carrying the result, and appends the continuation's segments
  to the same turn. This loops (bounded, like `toolRoundCap`) until the model
  produces a roll-free ending. One logical turn, one `RecordTurn`.
- **ask policy:** persists the turn with a `PendingCheck` (as today), returns to
  the client, and the player's roll arrives on the next request with
  `pending_check_ref`. The continuation adjudicates the result.

The roll result is persisted on the turn when it resolves, so a retry of the same
continuation never re-rolls. This is a correctness fix over today's behaviour,
where `resolveCheck` recomputes on every call.

**Continuation stitching.** A continuation carries `Turn.ContinuationOf` naming
the turn it continues, so the chronicle can group the halves. Appending an ask
continuation into the same stored turn record is a follow-up task; the auto path
needs no storage change because it records once.

### 4.5 Prompt guidance

`ContextAssembler` replaces the terminal-submission protocol with a framing
protocol: the speech convention, the record vocabulary, the roll-terminates rule,
and one worked example. The instruction is a non-droppable, high-rank section.

### 4.6 Superseding the tool surface

`submit_turn` is removed from the offered tools and from `TurnToolNames`.
`request_check` and `propose_check` are replaced by the `@roll` record. The
`harness.TurnSubmission` type and `TurnSubmissionSchema` are removed once nothing
reads them. Personae, memories, and state changes come from records.

## 5. TTS Streaming and Grouping

### 5.1 The problem

The merged TTS grouping work (`pkg/media/group.go`) plans groups over a
**complete** segment list: `planGroups` walks the segments in order, extends the
current group while the speaker budget and request limits allow, and splits an
oversized segment at sentence boundaries. It is a batch algorithm: it needs the
whole list before it can decide the last group.

The current live path (`pkg/gui/streaming_tts.go`) sidesteps the conflict by
making the two mutually exclusive: `sentenceStreamerFor` returns nil when
`TTSGrouping() == "always"`, because a streamed sentence is a cache miss for a
group. So a turn either groups (no live audio) or streams sentence-by-sentence
(live audio, one request per sentence, narration only).

Progressive parsing removes the reason for that exclusivity. The parser already
emits the exact boundaries a group needs: a group ends when the speaker or kind
changes, when a record arrives, or at end of stream. The task is to make grouping
*incremental* so a group can be flushed the moment its boundary is known.

### 5.2 The incremental group plan is the turn's plan

The decisive change: **groups are decided as the stream arrives and the turn's
clip plan is those groups, not a re-plan over the finished segment list.** If
finalise re-planned over the complete list, the streamed clips would not match
the stored ones and the streamed audio would be wasted. One plan, used by both
the stream and the record, keeps the played set and the stored clips identical.

`planGroups` becomes a streaming fold with the same rules:

```
pending = empty group
on sentence s of speaker p:
    if pending is empty:            pending = {p, [s]}
    elif canJoin(pending, p, s):    pending += s
    else:                           flush(pending); pending = {p, [s]}
    if pending over the budget:     flush a sentence-aligned prefix
on speaker/kind change or record:   flush(pending)
at end of stream:                   flush(pending)
```

The batch `planGroups` and the streaming fold must agree when the flush budget
does not bite, so the fold is the single implementation and the batch path is the
fold run over a complete list.

### 5.3 Two flush thresholds

A group is flushed when either threshold is reached:

- **Hard provider limit.** `MaxCharsPerRequest` / `MaxTokensPerRequest` from
  `ResolveGroupCaps`. A group that would exceed them must be split, exactly as
  `splitLineToFit` does today.
- **Latency budget.** A new, smaller threshold: the maximum speakable text held
  before the first audio is emitted. Without it, a long narration block would not
  be voiced until the block ends. The budget is configurable; a per-sentence
  provider is the degenerate case where the budget is one sentence.

The effective flush point is the smaller of the two, always split at a sentence
boundary so no request begins or ends mid-sentence.

### 5.4 Continuous-streaming providers

`TTSCapabilities.SupportsStreaming` is advertised but unused. A provider that
supports it takes a different path: instead of many one-shot group requests, the
engine opens **one request per speaker run**, pushes sentence text into it as it
arrives, and plays the audio chunks back as they return. The run ends, and the
stream is closed, on a speaker change, a record, or end of stream; a run that
reaches the provider's limit is cycled (close and reopen) rather than truncated.

Grouping and continuous streaming are two implementations of one interface:
"accept a sentence for speaker p, and eventually produce a playable clip". The
scheduler chooses per provider capability, and the turn's clip plan records which
clips resulted, so finalise stays identical.

### 5.5 Multi-speaker groups and live audio

A provider with `MaxSpeakers > 1` can render a narration-and-speech run in one
request, which is a good *offline* optimisation: fewer calls, consistent prosody.
It is a poor *live* one, because it delays the first speaker's audio until the
second speaker's text exists. Live groups are therefore single-speaker by
default, flushed on speaker change. Multi-speaker grouping remains available to
the offline/batch path (`pkg/ttsbatch`) and export, which have the whole turn and
no latency budget. A batch backfill re-renders under its own plan, and its keys
are content-addressed independently, so the two plans do not need to agree.

### 5.6 What this replaces

The mutual exclusion in `sentenceStreamerFor` goes away. The streamer is
generalised from "one sentence per request, narration only" to "grouped
same-speaker sentences, flushed on boundary or budget, with the speaker's voice".
The existing provisional-audio reconciliation (`streamedSpeech.playedKeys()`) and
the content-addressed cache are unchanged.

## 6. Data Flow

```
provider stream
      │  deltas
      ▼
onChunk ──► turnstream.Parser ──► events ──┬──► segment observer ──► GUI "segment" event
      │                                    │
      │                                    └──► grouped streamer (voice) ──► audio clips
      ▼
 finalise: Flush + resolve deferred speakers + build []TurnSegment
      │
      ▼
 Timeline.RecordTurn  (single writer; unchanged)
```

A `@roll` in the stream short-circuits: the call ends, the engine resolves or
persists the pending check, and either loops (auto) or returns (ask).

## 7. Error Handling

- **Malformed record:** logged (`turn.record_error`), skipped, prose unaffected.
- **Unresolvable speaker:** narration, as today. A `@persona` may resolve it
  retroactively at finalise.
- **No framing at all:** fall back to `buildTurnSegments` + the extractor,
  unchanged.
- **Continuation failure:** the half-turn is kept with its resolved roll; the
  turn is marked incomplete and can be continued by a later request.
- **Loop cap:** a turn that keeps requesting rolls stops after a bounded number
  of continuations and records what it has.

## 8. Testing Strategy

- `pkg/turnstream`: table tests over `Feed` split at every byte boundary, proving
  the event stream is identical regardless of chunking. This is the core
  correctness property of progressive parsing.
- `pkg/turnstream`: record parsing, malformed records, legacy quoted speech,
  narration coalescing, roster resolution.
- `pkg/engine`: a turn whose stream contains indented speech produces attributed
  segments with the right voices; a `@roll` terminates and (auto) continues; a
  malformed record does not lose the turn.
- `pkg/gui`: provisional `segment` events reach the client and carry a voice.
- Existing `dialogue` and `buildTurnSegments` tests remain green as the fallback.

## 9. Success Criteria

- A client receives attributed speech segments, with the correct character voice,
  while the model is still writing.
- A new character declared with `@persona` before their first line is voiced live.
- A `@roll` ends the response; the engine rolls once, persists the result, and
  continues deterministically.
- Live audio groups consecutive same-speaker sentences into one request up to the
  latency budget, and never holds a group past it.
- A model that emits no framing still produces a usable turn via the fallback.
- No turn is lost to a malformed record or submission.
