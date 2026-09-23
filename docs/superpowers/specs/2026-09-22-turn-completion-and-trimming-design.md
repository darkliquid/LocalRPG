# Design Spec: Recovering a Narrator Reply That Stops Mid-Thought

**Date:** 2026-09-22
**Status:** Draft
**Target:** `pkg/engine`, `pkg/harness`, `pkg/config`, `pkg/gui`, `frontend/src`

---

## 1. Executive Summary

A turn can end in the middle of a word, a clause, or an open quotation. Three things cause it: the model hits its token limit, the provider's stream fails after some text has arrived, or the model simply stops without finishing its sentence. The recorded prose is then a hard cut, and the next turn opens on a scene that trails off. The one signal the codebase acts on today is `finish_reason == "length"`, which sets `Turn.Truncated` so the chronicle can apologise for it (`frontend/src/components/ChronicleView.tsx:120`), but nothing repairs the text.

This spec adds a **reply recovery pass** that runs once, after generation and before anything is segmented or recorded. It first tries to *continue* the cut-off thought with a second, narrowly-scoped call whose prompt says only "finish this". If that fails, it *trims* the reply back to the last place all open constructs (sentences, quotes, emphasis, wikilinks) balance. A turn is never left half-written, and a turn is never lost because the repair attempt failed.

The player's streamed text is unaffected in protocol: the final `turn` event already replaces the streamed buffer (`frontend/src/App.tsx:211-214`), so a trimmed reply simply lands shorter than what was streamed.

---

## 2. Findings

### 2.1 Truncation is detected but not repaired

`Turn.Truncated` records "the model hit its token limit mid-reply" (`pkg/engine/history.go:27-29`), and the orchestrator sets it from the stream's finish reason (`pkg/engine/orchestrator.go:478`). The narration is written as-is. The note in the chronicle asks the player to raise the token limit; the scene is still cut off in the meantime.

### 2.2 A failed stream throws away usable text

`generate` accumulates deltas in a `strings.Builder`, but every failure path returns `""` for the text, discarding what arrived:

- a chunk carrying an error returns `"", "", chunk.Error` (`pkg/engine/orchestrator.go:636-638`);
- the idle watchdog returns `"", "", ErrGenerationStalled` (`pkg/engine/orchestrator.go:618-627`).

`ProcessActionStream` then fails the turn (`pkg/engine/orchestrator.go:459-461`), so the player's action is answered with an error even when 90% of a scene was already on screen. The router only falls back *before* the first chunk, so a mid-stream failure has no second provider either (`pkg/harness/router.go:100-116`).

### 2.3 Finish reason cannot be the only signal

Only the HTTP provider reports a real finish reason, and it defaults a silent stream to `"stop"` (`pkg/harness/http_provider.go:239-251`). The other providers always claim completion:

- the CLI provider emits `FinishReason: "stop"` unconditionally (`pkg/harness/cli_provider.go:146`);
- the narrative oracle emits a `Done` chunk with no finish reason at all (`pkg/harness/oracle_provider.go:58`);
- the echo default in the factory emits `Done: true` and nothing else (`pkg/harness/factory.go:36-40`).

A CLI-backed model that exits 0 one word into a sentence is indistinguishable from one that finished, so recovery must also read the text itself.

### 2.4 Nothing inspects the prose

There is no helper anywhere that answers "does this end at a sentence boundary?", and no notion of balanced quotes, emphasis, or wikilinks. `buildTurnSegments` and `harness.ResolveEntityMentions` consume the narration verbatim (`pkg/engine/orchestrator.go:486-496`), so an unterminated line can be parsed as an attributed speech beat with no closing quote.

### 2.5 Recovery has to happen before segmentation

Segments, entity mentions, speech speakers, and the extractor all read `turn.Narration` (`pkg/engine/orchestrator.go:486-503`). Repairing the prose at any point after line 496 would leave segments and mentions describing text that is no longer recorded. The pass belongs immediately after `generate` returns and before the `Turn` is constructed.

### 2.6 The roles pattern already supports a second provider

`extractor` is an optional role that defaults to `inherit: gm` and is resolved once per turn by `harness.ExtractorFromConfig` (`pkg/harness/factory.go:157-190`). A completion role can follow the same shape, letting a campaign point the repair at a cheaper or faster model without touching the narrator's configuration.

---

## 3. Design

### 3.1 Completeness and boundary detection

Create `pkg/harness/prose.go` with three pure functions.

```go
// ProseComplete reports whether text ends at a natural boundary: a sentence
// terminator optionally followed by a closing quote or bracket, with every
// construct the text opened (quote, emphasis, wikilink) closed.
func ProseComplete(text string) bool

// LastSentenceBoundary returns the byte offset just past the last sentence
// terminator in text that leaves all open constructs balanced. ok is false when
// no such boundary exists, or when the boundary sits before minChars.
func LastSentenceBoundary(text string, minChars int) (int, bool)

// TrimToLastSentence returns text cut back to its last balanced sentence
// boundary. It returns ("", false) when there is no usable boundary.
func TrimToLastSentence(text string, minChars int) (string, bool)

// StitchContinuation joins a continuation onto the text it resumes, removing a
// repeated overlap and repairing the seam. It is the only place the two halves
// are combined, so the seam rules live in one test.
func StitchContinuation(existing, continuation string) string
```

**Terminators.** `.`, `!`, `?`, the ellipsis character `…`, and three-or-more dots. A terminator counts only when it is followed by whitespace or end-of-text, which rejects `3.5` and `example.com`.

**Abbreviation guard.** A period does not terminate a sentence when the preceding token is a known abbreviation (`mr`, `mrs`, `ms`, `dr`, `prof`, `sr`, `jr`, `st`, `vs`, `etc`, `e.g`, `i.e`, `approx`, `dept`, `no`) or a single letter (`H. P. Lovecraft`). The list is deliberately short; a missed abbreviation costs one sentence of trim, which is cheaper than the reverse.

**Balance check.** `ProseComplete` and the boundary search both require:

- an even number of unescaped `"` in the final paragraph (an open quote in an earlier paragraph was closed by the paragraph break);
- balanced `*`/`**` emphasis runs, using the same no-internal-whitespace rule as `media.SpeakableText`;
- no unterminated `[[` wikilink;
- a non-empty final line after trimming markers.

**Scene-break exception.** A line consisting only of `---`, `***`, or `___` is a complete ending, as is a Markdown heading line.

**Ordering.** `LastSentenceBoundary` scans left to right, records every candidate boundary, and returns the last one at which all constructs are balanced. A reply cut inside a quotation therefore trims to the sentence *before* the quote opened, rather than leaving an unbalanced quote or fabricating a closing one.

### 3.2 The completion prompt

A cut-off reply is continued by a second call whose prompt is the completion instruction plus a bounded tail of the partial text. The whole assembled scene context is **not** resent: the goal is to finish a local thought, and a full context invites a fresh scene or a restatement.

The tail is the last `tail_chars` runes (default 1500), backed up to a paragraph boundary when one falls inside that window, so the model resumes at a clean boundary.

```text
[CONTINUATION]
The narrator's reply was cut off mid-thought. Finish it.

- Continue the text below from exactly where it stops. Do not repeat any of it.
- Do not start a new scene, introduce characters, or resolve anything the
  cut-off text had not already begun.
- End at the first natural boundary: the end of the sentence or paragraph you
  are completing.
- Output only the continuation. No preamble, no headings, no wrapping the whole
  reply in quotation marks.

Cut-off text:
<tail>
```

The instruction is identical for all three causes. The model cannot act on "your stream failed"; it only needs to know the text stops mid-thought.

### 3.3 Stitching the two halves

`StitchContinuation` applies, in order:

1. Drop a leading `[...]`-style artifact or a leading label such as `Continuation:` if the model added one.
2. **Overlap removal.** If the continuation begins by repeating the end of the existing text, find the longest suffix of `existing` that is a prefix of `continuation`, require it to be at least 8 runes, and drop it from the continuation. Shorter candidate overlaps are ignored because they match too readily.
3. **Seam repair.**
   - if `existing` ends with whitespace, drop leading whitespace from the continuation;
   - if both sides of the join are alphanumeric, concatenate directly, because a token-limit cut lands mid-word and the continuation resumes it (`bar` + `red` -> `barred`);
   - if `existing` ends with sentence punctuation or a closing quote/bracket, join with a single space;
   - if the continuation begins with `, . ; : ! ?`, drop trailing whitespace from `existing` and concatenate directly.
4. Collapse a run of three or more newlines to two.
5. Reject the result (return `existing` unchanged) if the continuation is empty or adds nothing.

### 3.4 The recovery pass

Add `pkg/engine/recovery.go`:

```go
// RecoveryOutcome describes what the pass did to a reply, for the trace and the
// recorded turn.
type RecoveryOutcome string

const (
    RecoveryNone      RecoveryOutcome = ""          // the reply was already complete
    RecoveryContinued RecoveryOutcome = "continued" // a second call finished it
    RecoveryTrimmed   RecoveryOutcome = "trimmed"   // the unfinished tail was dropped
    RecoveryKept      RecoveryOutcome = "kept"      // recovery was skipped or failed
)

// RecoveryOutcome is the pass's decision; Truncated reports whether the recorded
// prose is still incomplete.
func (o *TurnOrchestrator) recoverReply(ctx context.Context, partial string, cut CutCause, onChunk func(string) error) (string, RecoveryOutcome, bool)
```

`CutCause` is one of `cutNone`, `cutLength`, `cutInterrupted`, `cutStructural`:

- `cutLength` - the stream declared `finish_reason == "length"`;
- `cutInterrupted` - the stream failed or stalled after producing text;
- `cutStructural` - the stream completed normally but `ProseComplete` is false;
- `cutNone` - `ProseComplete` is true and no truncation was declared.

Decision ladder, driven by `agents.completion.mode`:

| Mode | Behaviour |
| :--- | :--- |
| `auto` (default) | continue when `cutLength`/`cutInterrupted`/`cutStructural`, then trim if the continuation fails or is still incomplete |
| `continue` | continue only; if it fails, keep the partial and mark the turn incomplete |
| `trim` | never call; trim to the last boundary |
| `off` | current behaviour: record what arrived, only mark `Truncated` |

Safeguards, applied in every mode except `off`:

- **Too short to work with.** If the partial is shorter than `agents.completion.min_incomplete_chars` (default 24 runes), skip recovery entirely. There is too little text to continue coherently and trimming it would leave nothing.
- **Attempt budget.** At most `max_attempts` continuation calls (default 1). A second attempt only runs if the first produced usable text that is still incomplete; it re-reads the newly extended text.
- **Time budget.** The continuation call is bounded by `agents.completion.timeout_seconds` (default 45) and by the same per-chunk silence watchdog the main call uses, so a repair cannot hold a turn open past its own deadline.
- **Disconnect is final.** If `onChunk` returns an error, or `ctx` is cancelled by the client, the pass stops and the turn is discarded exactly as today. Context deadline expiry (`context.DeadlineExceeded`, the turn's own budget) is treated as an interruption and *is* repaired, because the client is still connected.
- **Continuation deltas stream.** Continuation text is forwarded through `onChunk` as it arrives, so the player watches the sentence finish. No new stream event type is introduced.
- **Never fabricate on failure.** If the completion call errors, times out, returns empty text, or returns text that is still incomplete, fall back to trimming. If trimming finds no boundary, keep the partial text and mark the turn incomplete. The turn is always recorded.
- **A repair never turns an empty generation into a turn.** An empty partial is still the existing "gm returned no narration" failure (`pkg/engine/orchestrator.go:482-484`).

### 3.5 Keeping the text that arrived

`generate` currently discards partial text on every failure path. Refactor it into a shared chunk pump used by both the main call and the repair call:

```go
type streamResult struct {
    Text         string
    FinishReason string
    Interrupted  error // set when the stream failed or stalled after producing text
}

func (o *TurnOrchestrator) stream(ctx context.Context, provider harness.ModelProvider, req harness.GenerateRequest, onChunk func(string) error) (streamResult, error)
```

Rules:

- chunk error after text -> return the text and the error as `Interrupted`, not as a hard failure;
- idle watchdog after text -> same;
- chunk error before any text -> hard failure, as today;
- client disconnect or `onChunk` error -> hard failure, as today;
- success -> text plus the provider's finish reason.

`generate` becomes a thin wrapper that resolves the `gm` role through the router and calls `stream`. The repair pass resolves the completion provider directly (never through the role fallback chain, which is already exhausted) and calls the same `stream`.

`ProcessActionStream` then reads:

```go
result, err := o.generate(ctx, contextPrompt, onChunk)
if err != nil {
    return nil, fmt.Errorf("gm generation failed: %w", err)
}

cause := o.classifyCut(result)
narration, recovery, stillIncomplete := o.recoverReply(ctx, result.Text, cause, onChunk)
if strings.TrimSpace(narration) == "" {
    return nil, fmt.Errorf("gm returned no narration")
}
```

### 3.6 Recording the outcome

`Turn` gains one field (`pkg/engine/history.go`):

```go
// Recovery records how a reply that stopped mid-thought was repaired:
// "continued" (a second call finished it), "trimmed" (the unfinished tail was
// dropped), "kept" (recovery was skipped or failed), or empty (nothing was
// wrong). Truncated is true only when the recorded prose is still incomplete.
Recovery string `json:"recovery,omitempty"`
```

`Truncated` is recomputed as `stillIncomplete`, which changes its meaning slightly: it now describes the **recorded** prose rather than the model's exit reason. A reply that was cut off and then continued or trimmed is no longer truncated, because the player is not being shown a hard cut. A reply that could not be repaired keeps `Truncated: true` and the existing chronicle note still applies.

The note's wording should be broadened from "cut off by the model's token limit" to "could not be completed", since a provider failure or a silent stop now reaches it too (`frontend/src/components/ChronicleView.tsx:120-124`).

### 3.7 Configuration

Add to `AgentsConfig` (`pkg/config/types.go`):

```go
// Completion governs how a narrator reply that stops mid-thought is repaired.
Completion CompletionConfig `yaml:"completion" json:"completion"`

type CompletionConfig struct {
    // Mode is "auto", "continue", "trim", or "off". Empty means "auto".
    Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
    // MaxAttempts caps continuation calls per turn. Zero means one.
    MaxAttempts int `yaml:"max_attempts,omitempty" json:"max_attempts,omitempty"`
    // TailChars is how much of the partial reply the continuation call sees.
    TailChars int `yaml:"tail_chars,omitempty" json:"tail_chars,omitempty"`
    // MinIncompleteChars skips recovery for replies shorter than this.
    MinIncompleteChars int `yaml:"min_incomplete_chars,omitempty" json:"min_incomplete_chars"`
    // TimeoutSeconds bounds one continuation call.
    TimeoutSeconds int `yaml:"timeout_seconds,omitempty" json:"timeout_seconds"`
}
```

Accessors follow the existing zero-safe pattern: `CompletionMode() string`, `CompletionAttempts() int`, `CompletionTailChars() int`, `CompletionMinChars() int`, `CompletionTimeout() time.Duration`, with defaults `auto`, 1, 1500, 24, 45s. `DefaultConfig` writes the block explicitly so a fresh config shows the knobs.

Add a role constant (`RoleCompletion = "completion"`) and a default role entry `{Type: "inherit", InheritFrom: RoleGM}`, mirroring `RoleExtractor`. Resolution mirrors the extractor exactly:

```go
// CompletionFromConfig resolves the role that finishes a cut-off reply. It
// inherits gm unless configured otherwise, and a nil result disables the
// continuation half of recovery, leaving trimming.
func CompletionFromConfig(cfg *config.Config, router *Router, logger trace.Logger) ModelProvider
```

`TurnOrchestrator` gains `SetCompletionProvider(ModelProvider)` and is wired in `pkg/gui/service.go`'s `prepareTurn` next to `SetExtractor` (`pkg/gui/service.go:1086`), and in `cmd/localrpg/play.go` for the TUI path.

### 3.8 API and frontend

- `TurnDTO` gains `Recovery string \`json:"recovery,omitempty"\`` and it is copied in `turnDTO` (`pkg/gui/service.go:822-837`).
- `frontend/src/types.ts` gains `recovery?: string`.
- `ChronicleView` shows one terse line when `recovery === 'trimmed'` ("The narrator's reply ended mid-thought; the unfinished tail was dropped.") and keeps the existing note for `truncated`. `continued` renders nothing: the reply reads as complete, which is the point.
- No change to the stream protocol or to `App.tsx`; the final `turn` event already clears `streamedProse`.

### 3.9 Alternatives considered

- **Repair inside each provider.** Rejected: providers are transport. The CLI provider cannot tell whether its process stopped early, and recovery needs the assembled role and configuration, which live above the provider.
- **Always trim, never continue.** Rejected as the sole behaviour: it discards a player's scene whenever a limit is reached. It remains available as `mode: trim`.
- **Always continue.** Rejected as the sole behaviour: it spends a second model call on every reply and invents prose after a genuine failure. It remains available as `mode: continue`.
- **Resend the full context to the continuation call.** Rejected: the model then re-plans the turn and produces a second scene, and the stitch becomes guesswork.

### 3.10 Out of scope

- Repairing the chronicler's summaries, which go through `harness.Summariser` and can be truncated too. The prose helpers are deliberately reusable so this can follow.
- Rewriting or shortening prose that is complete but too long.
- Detecting a reply that is *coherently* complete but thematically cut short; only structure is inspected.
- A UI control for the recovery policy beyond settings.

---

## 4. Data Flow

```text
ProcessActionStream
  └─ assemble context
       └─ generate (gm role)
            └─ stream() ─ accumulated text + finish reason
                 ├─ complete + no truncation ──────► cutNone
                 ├─ finish_reason == "length" ─────► cutLength
                 ├─ stream failed/ stalled ────────► cutInterrupted (text kept)
                 └─ ProseComplete false ───────────► cutStructural
       └─ recoverReply(partial, cause, onChunk)
            ├─ mode "off" ────────────────────────► keep, Truncated = !complete
            ├─ partial shorter than minimum ──────► keep, Recovery = "kept"
            ├─ continuation call (completion role)
            │    └─ StitchContinuation(partial, delta) ─► stream deltas to client
            │         ├─ ProseComplete ──────────► Recovery = "continued"
            │         └─ still incomplete ───────► retry, then trim
            └─ TrimToLastSentence(partial)
                 ├─ boundary found ─────────────► Recovery = "trimmed"
                 └─ no boundary ────────────────► Recovery = "kept", Truncated = true
       └─ build segments, mentions, extraction from the repaired narration
            └─ Timeline.RecordTurn
```

---

## 5. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| Create | `pkg/harness/prose.go` | `ProseComplete`, `LastSentenceBoundary`, `TrimToLastSentence`, `StitchContinuation` |
| Create | `pkg/harness/prose_test.go` | Table tests for terminators, abbreviations, balance, trim, stitching |
| Modify | `pkg/engine/orchestrator.go` | `stream`, `generate` refactor through it, `cut` classification, recovery wiring before segmentation |
| Create | `pkg/engine/recovery.go` | `RecoveryOutcome`, `CutCause`, `recoverReply`, completion prompt builder |
| Create | `pkg/engine/recovery_test.go` | Continuation, trim fallback, short-reply skip, disconnect, mode matrix |
| Modify | `pkg/engine/history.go` | `Turn.Recovery` |
| Modify | `pkg/engine/orchestrator_stream_test.go` | Interrupted stream keeps partial text; no text still fails |
| Modify | `pkg/harness/factory.go` | `CompletionFromConfig`, `RoleCompletion` default inherit |
| Modify | `pkg/harness/factory_test.go` | Completion role resolution and disable |
| Modify | `pkg/config/types.go` | `CompletionConfig`, accessors, `RoleCompletion`, defaults |
| Modify | `pkg/config/types_test.go` | Defaults and zero-safe accessors |
| Modify | `pkg/gui/service.go` | `SetCompletionProvider` wiring, `TurnDTO.Recovery` |
| Modify | `pkg/gui/types.go` | `TurnDTO.Recovery` |
| Modify | `cmd/localrpg/play.go` | Wire the completion provider into the TUI orchestrator |
| Modify | `frontend/src/types.ts` | `recovery?: string` |
| Modify | `frontend/src/components/ChronicleView.tsx` | Trimmed note; broaden the truncated note |

---

## 6. Acceptance Criteria

1. A reply whose `finish_reason` is `length` is continued by exactly one second call by default, and the recorded prose ends at a sentence boundary.
2. A reply that stops mid-sentence with no finish reason (CLI, oracle, or echo provider) is detected structurally and repaired the same way.
3. A provider failure or stall after partial text no longer loses the turn: the text that arrived is repaired and recorded.
4. A provider failure before any text still fails the turn, and a client disconnect still records nothing.
5. When continuation is unavailable, fails, times out, or returns still-incomplete text, the reply is trimmed to the last balanced sentence boundary.
6. A reply cut inside a quotation or an emphasis run trims to a boundary at which the quotation or emphasis is balanced; no unbalanced Markdown or dangling speech segment is ever recorded.
7. A reply that is already complete is recorded byte-for-byte unchanged, with `Recovery` empty.
8. `Truncated` is true only when the recorded prose is still incomplete; `recovery` distinguishes `continued`, `trimmed`, and `kept`.
9. Recovery never introduces more than `max_attempts` extra calls and never exceeds its own time budget.
10. `mode: off` reproduces today's behaviour exactly, including the existing `Truncated` flag.
11. Segments, entity mentions, speech speakers, and the extractor all see the repaired narration, never the raw cut.
12. `mise run test` passes (`go test ./...` and `npx tsc --noEmit`), and `go vet ./...` stays clean.
