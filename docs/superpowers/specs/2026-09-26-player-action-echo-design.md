# Design Spec: Player Action Echo (Narrator Restatement)

**Date:** 2026-09-26
**Status:** Proposed
**Target:** `pkg/harness` (`context.go`, `turn_tools.go`), `pkg/engine` (`orchestrator.go`, optionally `submission.go`/`continuity.go`), `pkg/config` (`types.go`), `frontend` (settings toggle)

---

## 1. Executive Summary

The narrator currently receives the player's action as a bare block (`## PLAYER ACTION\n<text>`, `pkg/harness/context.go:238`) with no name attached, and nothing tells it to acknowledge that action before continuing. In play this reads as the narrator answering out of nowhere:

> **player (Stretch Layabout):** I jump into my ship, blasting my pursuers as the hatch closes.
>
> **narrator:** The canopy seals with a hiss. Somewhere behind you, a pursuer peels off. *(no acknowledgement of what Stretch actually did)*

The desired behaviour is a short, third-person restatement of the player's action as the opening beat of the narrator's reply, after which the response continues normally:

> **narrator:** Stretch jumps into their ship, firing blaster shots at their pursuers until the canopy seals shut. *(the response to those actions now follows as normal)*

This spec designs a **player action echo** convention: the narrator opens each action turn by restating what the player did, in third person, before resolving it.

---

## 2. Current Behaviour

- `ContextRequest` (`pkg/harness/context.go:76-104`) carries `PlayerID` and `Action`, but **not the player's name**. `buildSections` renders the action as `"\n## PLAYER ACTION\n" + req.Action + "\n"` with a single `Ref` to `PlayerID` (`context.go:222-238`).
- The GM's system prompt is the system's `prompts/rules.md` (`o.rulesPrompt`, sent as `GenerateRequest.System` in `pkg/engine/orchestrator.go:1254,1308`); the context is the user message. Narration-style guidance therefore lives in per-system content, not in shared code, so a convention added only there would not apply to every system.
- There is already a precedent for anchoring the player's own words: `attachPlayerSegment` (`pkg/engine/player_segment.go:51`) turns a `Say` input into a leading `speech` segment with `Player: true`. The echo is the **action** analogue of that idea, but narrated in third person by the GM rather than quoted.
- Two action turns deliberately change the prompt and must **not** echo: `/gm <directive>` (a director correction, `orchestrator.go:534-537`), `Roll` (`[PROPOSED CHECK: ...]`, `orchestrator.go:538-546`), and the opening turn (`openingDirective`, `orchestrator.go:522-529`). A player `System` move is also not an action (`orchestrator.go:511-527`).
- The player's display name is already resolved by `(*TurnOrchestrator).playerDisplayName()` (`player_segment.go:71`), falling back to the id.

---

## 3. Design

### 3.1 Name the actor in the action block

Add `PlayerName string` to `harness.ContextRequest` and populate it from `o.playerDisplayName()` at the assembly call site (`orchestrator.go:620-638`).

Render the action section as:

```
## PLAYER ACTION
Stretch Layabout: I jump into my ship, blasting my pursuers as the hatch closes.
```

When `PlayerName` is empty the current single-line form is kept. This is a small, safe change that gives the narrator the name it needs to restate the action.

### 3.2 A shared, non-droppable instruction section

Add an instruction block assembled in `buildSections` (next to the existing `instructions`/speech-cues section, `context.go:230`) so it applies to every system without editing each `rules.md`:

```
## PLAYER ACTION ECHO
Open every turn with one short narration segment that restates the player's action
in the third person, using the character's name or pronoun, before anything else
happens. Do not change what they did, add intent they did not state, or narrate the
outcome in that sentence; just re-anchor the scene on their action, then continue.

Example:
  Player (Stretch Layabout): I jump into my ship, blasting my pursuers as the hatch closes.
  Opening narration: Stretch jumps into their ship, firing blaster shots at their
  pursuers until the canopy seals shut.
```

The section is **not droppable** (like `instructions`), and is omitted entirely when:

- the mode is `Say` (the player's quoted line already leads via `attachPlayerSegment`);
- the turn is the opening turn, a `/gm` correction, a `Roll` proposal, or a `System` move;
- the action text is empty;
- the feature is disabled by configuration (3.3).

The GM tool description for `submit_turn` (`turn_tools.go:22`) gains one sentence: *"Begin with a short third-person restatement of the player's action before resolving it."*

### 3.3 Configuration

Add an `Agents` pointer-bool toggle, mirroring `ContinuityChecks` (`pkg/config/types.go:84`, accessor at `:561`):

```go
// ActionEcho prepends the narrator's third-person restatement of the player's
// action to each turn. A pointer distinguishes "unset" (on) from "off".
ActionEcho *bool `yaml:"action_echo" json:"action_echo,omitempty"`
```

Accessor `func (c *Config) ActionEcho() bool` returning `c.Agents.ActionEcho == nil || *c.Agents.ActionEcho` so the convention is on by default and existing configs are unaffected. Surface it as a toggle in the Settings studio alongside the continuity-checks toggle; `gui.Service.GetSettings`/`SaveSettings` already round-trip the `agents` block, so no endpoint change is needed, only the frontend field and control.

### 3.4 Enforcement (advisory first)

A pure prompt convention is the smallest change and is what this spec recommends shipping first. If the model still skips the echo, add a deterministic check in the engine after `submit_turn` is parsed (`pkg/engine/submission.go`):

- Consider the echo present when the **first narration segment** (before stripping wikilinks) contains the player's display name (case-insensitive) or a pronoun from the player entity plus a verb matching the action.
- When it is missing, record a note rather than rewrite the turn: reuse the existing advisory channel (`Turn.ContinuityNotes`, rendered by `ChronicleView`) with a message such as *"The narrator did not restate your action; the scene may be harder to follow."*
- A future `strict` mode may instead issue one repair instruction to the GM (the turn loop already supports bounded retries), but that is out of scope for the first cut because it adds a generation round-trip to every drifting turn.

### 3.5 Rendering, playback, and the chronicle

The echo is an ordinary `narration` segment, so it flows through the existing pipeline with no client change: it is shown inline by `TurnSegments`, narrated by TTS like any narration, included in `history.jsonl` and the index, and paced in the Story Theater like every other beat. The player's input block in `ChronicleView` stays; the echo is the narrator's in-fiction acknowledgement, not a replacement for the input.

---

## 4. Data Flow

```
player action (Do/Story/...)
        |
        v
orchestrator builds generateRequest
  - PlayerName = playerDisplayName()
  - Action = generationPrompt
  - exclude echo for Say / opening / /gm / Roll / System
        |
        v
harness.AssembleContext -> sections:
  rules, lore, instructions, [PLAYER ACTION ECHO], canon, ..., PLAYER ACTION (Name: text)
        |
        v
GM stream -> submit_turn { segments: [ {narration: "<echo>"}, ... ] }
        |
        v
buildSegments -> Turn.Segments (echo first) -> history.jsonl + index
        |
        v
Chronicle / TTS / Story Theater (ordinary narration beat)
```

---

## 5. Edge Cases

| Case | Behaviour |
|---|---|
| `Say` mode | No echo; the quoted player line already leads (`attachPlayerSegment`). |
| Opening turn | No echo; the opening directive establishes the scene. |
| `/gm <directive>` | No echo; directives are corrections, not player actions. |
| `Roll` mode | No echo; it is a proposed check, not an action. |
| `System` move | No echo. |
| Empty or whitespace action | No echo section. |
| Player name missing | Section still emitted using pronouns; action block keeps the unnamed form. |
| Action is itself dialogue ("I shout, get down!") | Echo restates the shout in third person; the quoted words may appear in the same segment. |
| Feature disabled | Section omitted; prompts and behaviour are exactly as today. |
| Long action | Echo is instructed to be one short sentence; the full action remains in the input block and the action section. |

---

## 6. Non-Goals

- Rewriting or paraphrasing the player's action as a deterministic transform; the restatement is generated, not string-munged.
- Quoting the player verbatim in narration (that is `Say` behaviour).
- Changing check resolution, memories, or the structured turn schema.
- Forcing the echo by spending an extra model round-trip (deferred to a possible `strict` mode).

---

## 7. Test Strategy

1. **`pkg/harness` context tests**: with `PlayerName` set, the assembled prompt contains `## PLAYER ACTION ECHO`, the example, and `Stretch Layabout: <action>`; with the toggle off, or for `Say`/opening/`/gm`/`Roll`/`System`, it does not. Extend `context_test.go:221`'s required-sections list.
2. **`pkg/config` test**: `ActionEcho()` defaults true, is true when the pointer is true, false when explicitly false, and is preserved through a save/load round-trip.
3. **`pkg/engine` test**: the orchestrator passes `PlayerName` into `ContextRequest` and omits the echo mode set; an enforcement test (if added) asserts a continuity note when the first narration segment lacks the player's name.
4. **Frontend build gate**: `mise run test:frontend` passes with the new settings field.
5. **Manual**: play an action turn and confirm the narrator opens by restating it in third person, then resolves it; confirm a `Say` turn and the opening turn are unchanged; toggle the setting off and confirm the echo stops.
