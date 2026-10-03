# Structured Turn Protocol Design

> **SUPERSEDED (2026-10-03).** Replaced by
> `2026-10-03-progressive-turn-stream-design.md`. Kept for history only.

**Date:** 2026-09-25
**Status:** Proposed
**Scope:** Engine turn generation, GM response format, entity personae declaration, speech/narration segmentation, mechanics check interleaving, mode mapping
**Related:** Agentic Turns and Tools Design (2026-09-22), Entity Memories & Memory Tools Design (2026-09-25), Mechanics Engagement & Declarative Schema Design (2026-09-25), `pkg/engine/orchestrator.go`, `pkg/harness/extractor.go`, `pkg/entity`, `pkg/dialogue`

## 1. Overview & Goals

Today the GM's reply is a single prose blob (`engine.Turn.Narration`). Everything
else is reconstructed after the fact: mentions by regex and wikilinks, speech vs
narration by `dialogue.Parse` plus a second model pass (`harness.Extractor`), new
entities by that extractor, and state never at all. There is no typed place for
"who spoke", "this character is new and here is their gender", "this action
needed a roll and here is the result", or "the player's action was impossible".

This specification makes the GM author a structured turn through a terminal
`submit_turn` tool call, with mid-stream `request_check` calls for mechanics and a
single `action_verdict` that encodes feasibility. The extractor becomes a
fallback for providers that cannot call tools or for a malformed submission,
never the primary path.

**Goals:**

- One typed turn payload: ordered `segments[]`, `action_verdict`, `personae[]`,
  `memories[]`, `state_changes[]`, and an optional `player_location`.
- Speech is authored, not reconstructed: a speech segment names its speaker, and a
  speaker that does not exist yet is declared in `personae` as a stub.
- Checks interrupt generation mid-stream; the result is fed back so the rest of
  the narration is consistent with it.
- The player's action feasibility is explicit: `automatic`, `uncertain`, or
  `impossible`, with a reason. "Impossible" is a first-class rejection.
- The engine audits the submission: an `uncertain` verdict must reference a check
  it actually resolved; a segment that narrates a check outcome must reference
  that check.
- The extractor is invoked only for tool-incapable providers or after one failed
  structured attempt.
- Existing turns keep rendering; the stored `Turn` change is additive.

**Non-Goals:**

- Per-entity memory storage and retrieval, beyond accepting and persisting what
  the GM declares (spec 2).
- Declarative mechanics schema and check resolution internals (spec 3). This spec
  defines the `request_check` boundary and the audit, not how a check is scored.
- Progressive per-segment streaming; the raw text stream remains the provisional
  view and `segments[]` is authoritative at submit.
- Rewriting stored history.
- Player-initiated rolls being forced on the GM (they are proposals; see §7).

**Success Criteria:**

- A turn authored through `submit_turn` stores typed segments, the verdict,
  personae, memories, state changes, and a location move without any extractor
  pass on a tool-capable provider.
- A speech segment whose speaker is a `new` persona creates exactly one stub
  entity with the declared name/type/gender/pronouns/tags and a `Speech` mention.
- A `verdict: uncertain` submission with no resolved check is rejected and retried;
  a second failure yields a prose-only turn plus a warning, with no fabricated
  entities or state.
- A `verdict: impossible` turn is stored flagged as rejected and does not resolve
  the player's action.
- A tool-incapable provider still produces a usable turn via the extractor
  fallback, unchanged from today.
- Existing turns render from their stored `narration`.

## 2. Investigation Findings

- The GM reply becomes `Turn.Narration` wholesale; `buildTurnSegments`
  (`pkg/engine/segments.go:17`) re-splits it with `dialogue.Parse`
  (`pkg/dialogue/dialogue.go:31`) and `mergeAttributions`, resolving speakers
  against the entity store and the entities the extractor is about to create.
- `Extractor.Extract` (`pkg/harness/extractor.go:321`) is a second model call
  whose schema is only `{id,name,type,location,faction,appearance,body}` +
  `dialogue[]` + `player_location`; it cannot emit state, tags, aliases, or voice.
  Creation ignores the model id and uses `entity.Slugify(name)`
  (`pkg/engine/timeline.go:139`).
- Tool calls already work for both OpenAI-compatible and Gemini providers, and a
  tool-call round's prose is deliberately discarded (`orchestrator.go:1254`). The
  tool loop caps rounds (`toolRoundCap`, default 4) and budget.
- The single terminal tool surface is `harness.ToolSpecs()` (`pkg/harness/tools.go:31`)
  with four read-only query tools; dispatch is in `pkg/tools/tools.go:38`.
- `Turn` is `pkg/engine/history.go:17`; `TurnSegment` and `Mention` are in
  `pkg/entity`. `history.jsonl` marshals the whole `Turn` except `Prompt`.

## 3. Architecture

### 3.1 Two tool families

- **Turn tools** (available on narrative turns): `submit_turn` (terminal) and
  `request_check` (mid-stream, repeatable). Defined in a new
  `pkg/harness/turn_tools.go`, offered alongside the query tools.

The `CheckResolver` interface lives in `pkg/harness` (not `pkg/engine`) so that
`pkg/rules` can implement it without importing `pkg/engine`, which imports
`pkg/rules`.
- **Query tools** (available any round): the existing `search_entities`,
  `get_entity`, `graph_neighbours`, `search_timeline`, plus the memory tools from
  spec 2.

`submit_turn` is terminal: the loop stops when it is called and returns the
submission instead of prose. `request_check` is not terminal: the engine resolves
it through the rules layer (spec 3) and appends the result as a tool message so
generation continues.

### 3.2 Payload types

New types in `pkg/harness/turn.go` (transport-neutral; the engine and GUI share
them):

```go
type ActionFeasibility string // "automatic" | "uncertain" | "impossible"

type ActionVerdict struct {
    Feasibility ActionFeasibility `json:"feasibility"`
    Reason      string            `json:"reason,omitempty"`
}

type SegmentSpec struct {
    Kind     string `json:"kind"`               // "narration" | "speech"
    Speaker  string `json:"speaker,omitempty"`  // name or id, speech only
    Text     string `json:"text"`
    CheckRef string `json:"check_ref,omitempty"`
}

type PersonaDecl struct {
    Name        string   `json:"name"`
    Type        string   `json:"type"`
    New         bool     `json:"new,omitempty"`
    Gender      string   `json:"gender,omitempty"`
    Pronouns    string   `json:"pronouns,omitempty"`
    RoleTags    []string `json:"role_tags,omitempty"`
    Description string   `json:"description,omitempty"`
    VoiceHint   string   `json:"voice_hint,omitempty"`
}

type MemoryDecl struct {
    Kind       string   `json:"kind"`         // event|relationship|discovery|dialogue
    EntityRefs []string `json:"entity_refs"`
    Text       string   `json:"text"`
    Importance int      `json:"importance"`   // 1-5
    Tags       []string `json:"tags,omitempty"`
}

type StateChangeDecl struct {
    Entity string      `json:"entity"`
    Path   string      `json:"path"`
    Op     string      `json:"op"`   // set|add|sub
    Value  interface{} `json:"value"`
    Reason string      `json:"reason,omitempty"`
}

type CheckRequest struct {
    Actor      string            `json:"actor"`
    Target     string            `json:"target,omitempty"`
    CheckKind  string            `json:"check_kind"`
    Stat       string            `json:"stat,omitempty"`
    Difficulty string            `json:"difficulty,omitempty"`
    Stakes     string            `json:"stakes"`
    Outcomes   map[string]string `json:"outcomes"`
    Notation   string            `json:"notation,omitempty"`
}

type CheckResult struct {
    CheckID  string         `json:"check_id"`
    Roll     *RollSummary   `json:"roll"`
    Outcome  string         `json:"outcome"`
    Breakdown map[string]interface{} `json:"breakdown,omitempty"`
}

// RollSummary is harness's view of a die roll, so the protocol types never
// import pkg/rules (rules imports harness and would cycle).
type RollSummary struct {
    Notation  string `json:"notation"`
    Total     int    `json:"total"`
    Successes int    `json:"successes"`
    RollCount int    `json:"roll_count"`
}

type TurnSubmission struct {
    Verdict       ActionVerdict     `json:"action_verdict"`
    Segments      []SegmentSpec     `json:"segments"`
    Personae      []PersonaDecl     `json:"personae,omitempty"`
    Memories      []MemoryDecl      `json:"memories,omitempty"`
    StateChanges  []StateChangeDecl `json:"state_changes,omitempty"`
    PlayerLocation string           `json:"player_location,omitempty"`
    DismissedChecks []DismissedCheck `json:"dismissed_checks,omitempty"`
}
```

`submit_turn`'s parameters mirror `TurnSubmission`; `request_check`'s mirror
`CheckRequest`.

### 3.3 `Turn` changes (additive)

```go
type Turn struct {
    // ...existing fields unchanged...
    Verdict   *harness.ActionVerdict `json:"verdict,omitempty"`
    Rejected  bool                   `json:"rejected,omitempty"`
    Checks    []harness.CheckResult  `json:"checks,omitempty"`
    Personae  []string               `json:"personae,omitempty"` // created stub ids
}
```

`Narration` remains, derived by joining narration segments in order. `Segments`
becomes authoritative when present. Old records with only `narration` are
unchanged.

## 4. Engine Flow

`runGenerationLoop` gains a submission path alongside the prose path:

1. Offer turn tools and query tools per round (subject to cap/budget, unchanged).
2. If a round returns `request_check`: resolve via the rules layer (spec 3),
   append a tool message with the `CheckResult`, record it on a running
   `[]CheckResult`, and continue.
3. If a round returns `submit_turn`: validate (§5), stop, and return the
   submission.
4. If a round returns only prose: that prose is the provisional narration. On a
   tool-capable provider this is a protocol deviation; retry once with a system
   reminder. A second prose-only reply is treated as a malformed submission (§6).
5. No calls and no prose in the final round is the existing
   `tool loop ended without an answer` error.

`ProcessActionStream` then:

- builds `Turn.Segments` from `SegmentSpec`, resolving speakers against existing
  entities and the submitted personae; a `new` persona is created by the timeline
  as an entity stub.
- resolves each `SegmentSpec.CheckRef` against the recorded checks; an unknown ref
  is a validation error.
- derives `Turn.Narration` by joining narration segments.
- stores `Verdict`, `Rejected`, `Checks`, `Personae`.
- applies `StateChanges` through the rules/host layer (spec 3) and writes
  `MemoryDecl`s plus the engine's mechanical memories (spec 2).
- applies `player_location` exactly as the extractor's did.

## 5. Validation and Audit

`validateSubmission` (engine) rejects with a bounded reason:

- `verdict == uncertain` and no `CheckResult` recorded → `no_check`.
- `verdict == impossible` and any resolved check → `impossible_with_check`.
- a segment `check_ref` not present in the recorded checks → `unknown_check`.
- a speech segment with neither a resolvable speaker nor a matching persona →
  `unknown_speaker` (retry once, then keep the text as narration and warn).
- `state_changes` referencing an undeclared stat under a system that declares
  stats → `undeclared_stat` (spec 3 owns the rule).
- a `dismissed_checks` entry whose reason is empty → `unjustified_dismissal`.

Validation failures retry once with the reason appended as a system reminder. A
second failure logs `turn.protocol_error` and falls back (§6).

## 6. Fallback

- **Tool-incapable provider:** prose path as today, extractor used for personae
  and dialogue (unchanged), no verdict/check enforcement.
- **Malformed structured turn after retry:** accept the provisional prose as
  `Narration`, skip personae/memories/state (never fabricate), emit
  `turn.protocol_fallback` with the reason, and leave `Verdict` nil.
- The extractor is never invoked when a valid `submit_turn` was accepted.

## 7. Player-Initiated Checks and Mode Mapping

- A `Roll` or skill input is parsed into a `proposed_check {actor, stat_or_skill,
  intent, stakes}` carried in the GM's context (not auto-executed). The GM either
  calls `request_check` referencing it or records a `dismissed_checks` entry with
  a reason. Both are logged; the GM's decision is authoritative.
- `Do`/`Say`/`Story` are one pipeline: player context plus the verdict policy.
- `/gm <directive>` is carried as a directorial system directive and sets
  `verdict: automatic` for that turn (the override is logged as `gm.override`).
- `System` remains a non-narrative command path.
- The opening turn has no player action: no verdict or check is required, but
  personae and memories are allowed.

## 8. Testing

- A scripted provider emits `request_check` then `submit_turn`; assert the check
  result is recorded and the segment `check_ref` resolves.
- `verdict: uncertain` with no check is rejected and retried; a second failure
  yields a prose turn with `turn.protocol_fallback` and no personae.
- `verdict: impossible` stores `Rejected` and no checks.
- A speech segment with a `new` persona creates one stub with the declared fields.
- A tool-incapable provider takes the extractor path and produces the same turn as
  today.
- Modes: a `Roll` input produces a `proposed_check` in context; a dismissed check
  with no reason fails validation.

## 9. Compatibility & Migration

- `Turn` gains optional fields; `history.jsonl` and the SQLite `turns` row remain
  readable. No migration.
- `Narration` is still populated, so export, TTS, and chronicle code that reads it
  keep working; the GUI may adopt segments incrementally.
- A provider that cannot call tools behaves exactly as before.

## 10. Open Questions

- Should the provisional raw stream and the final segments both render during a
  turn, or should the UI replace in place? (Current proposal: replace on submit.)
- Does `submit_turn` need an explicit `narration` convenience field, or is joining
  segments enough? (Current proposal: derive only.)
