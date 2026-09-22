# Design Specification: The Playable Gameplay Experience

**Date:** 2026-09-22
**Status:** Draft — pending review
**Topic:** Make playing a campaign feel like sitting at a table with a GM: a real prologue, a turn that always ends, prose that reads like prose, speech that is spoken, and a protagonist who actually exists

---

## 1. Problem Statement & Motivation

The app can create a campaign, browse it, and stream a turn. It cannot yet be *played*. Six failures were observed in one sitting, and each maps to a concrete defect rather than to taste.

| Observation | Root cause | Evidence |
| --- | --- | --- |
| A new campaign has no beginning. The player must invent the first move against a blank console. | There is no opening prompt or opening turn. Campaign creation only derives a location entity. | `pkg/engine/startlocation.go:137-188`; no `opening_prompt` anywhere |
| Generation never finishes; the app never returns control. | No deadline, no idle timeout, no `max_tokens`, and `finish_reason` is ignored. A provider that stalls holds the turn and the per-game lock forever. | `pkg/harness/http_provider.go:27,130-141`; `pkg/engine/orchestrator.go:313-343`; `pkg/gui/service.go:411-416` |
| The GM reply is malformed: no paragraphs, no line breaks, NPC speech not delineated and not played, voice metadata leaked into the text. | No markdown/whitespace rendering; speech parsing is one strict regex; the prompt asks the model to *write voice IDs into the prose*. | `frontend/src/components/TurnSegments.tsx:13-28,61-68`; `pkg/dialogue/dialogue.go:22,31-64`; `pkg/harness/context.go:68-69` |
| Stopping a stalled turn erased the whole response. | The stop handler calls `setStreamedProse('')`; an errored stream is logged but never surfaced or preserved. | `frontend/src/App.tsx:103-111,117-120` |
| The Characters tab shows "No character loaded." despite entering a protagonist name. | The manifest stores the display name (`"Elena Nightshade"`) but the note is written to the slug (`elena-nightshade.md`), and both `GetGameState` and the orchestrator treat the manifest value as the entity ID. | `pkg/engine/game.go:58,137-142`; `pkg/gui/service.go:189-191`; `pkg/core/types.go:33` |
| The campaign title entered at setup is lost after a reload. | `InitGame` writes `Name: gameID`. | `pkg/engine/game.go:55` |

Two secondary defects compound the same experience: "Resume Adventure" uses alphabetical list order rather than `last_played` (`frontend/src/components/LauncherHub.tsx:86` vs `pkg/gui/service.go:635-673`), and the `preferences.streaming` setting is dead configuration that no turn code reads (`frontend/src/App.tsx:83-115`).

The goal is not a feature list. It is that a first-time player can press **New Campaign**, be given an opening, type a sentence, and receive a readable, possibly-spoken reply within a bounded time — every time.

---

## 2. Goals & Non-Goals

**Goals**

1. A campaign begins with a scene the player can configure and the GM narrates, before the player is asked for anything.
2. Every turn terminates. A stalled provider fails visibly, within a configured bound, and leaves the campaign untouched.
3. GM prose is rendered as prose: paragraph breaks, emphasis, scene breaks, inline entity links.
4. NPC speech is visually distinct, parsed reliably, optionally spoken, and never contains engine metadata.
5. The protagonist entered at setup exists, is found by the UI, and is playable.
6. A stopped or failed turn preserves the draft and offers a next action instead of destroying it.

**Non-goals**

- A full narrative scripting engine or branching dialogue trees.
- Rich-text authoring of openings beyond a prompt field and optional scene title.
- Changing the schema-agnostic engine model: no HP/mana/classes are introduced.
- Multiplayer, cloud sync, or authentication. The zero-TCP posture is unchanged.
- Rewriting the exporter or the TUI beyond the shared engine changes that keep them honest.

---

## 3. Decisions

| Area | Decision |
| --- | --- |
| Opening | Optional `opening_prompt` captured at creation and editable afterwards; a **Prologue** screen generates turn 1 on demand; generation is a normal turn in a reserved `Opening` mode, recorded like any other turn |
| Opening trigger | Explicit, never automatic. The player presses **Begin the story**. An empty campaign with no prompt still offers **Begin with my first action** |
| Turn deadline | Two server-side bounds: a wall-clock `turn_timeout` and an idle `chunk_timeout` since the last delta. Breach cancels generation and emits an `error` event; nothing is persisted |
| Provider completion | `max_tokens`, `temperature`, and optional `stop` are sent; `finish_reason` is propagated through `StreamChunk`; `length` is a recorded-but-flagged truncation, not a stall |
| Stop button | Stops generation, **keeps** the partial prose as a `stopped` draft, and offers Retry / Discard / Continue. It never silently clears |
| Drafts | A draft is ephemeral UI state keyed to the in-flight turn. It is never written to `history.jsonl` unless a `turn` event arrives |
| Prose rendering | A dependency-free `MarkdownProse` component renders a constrained Markdown subset and preserves soft line breaks; no raw HTML is trusted |
| Speech contract | The model writes `Name: "words"` lines only; it is no longer asked to name voice profiles. Voice assignment stays where it already works: the deterministic extractor |
| Metadata defence | Prompt no longer invites metadata; a server-side sanitizer strips known voice/attribution directives as defence in depth; an optional advanced toggle may reveal resolved voice IDs out of band |
| Playback | Speech segments autoplay sequentially after a turn completes when `auto_play` is on; audio is synthesized on demand from the existing per-segment endpoint |
| Player identity | The manifest stores the **entity ID**; a display name lives on the note. A resolver tolerates legacy display-name values |
| Character presence | Creation always writes a player note with a description; the Characters drawer offers a recovery action if it cannot be resolved |
| Streaming preference | `preferences.streaming` becomes real: off means one blocking request behind a spinner |

---

## 4. Opening the Campaign

### 4.1 What exists today

`InitGame` resolves a start location and pins it in `game.yaml` (`pkg/engine/game.go:101-107`), and creates a location note when the world supplies none (`pkg/engine/startlocation.go:137-188`). Nothing narrates. The first thing the player sees is an input box.

### 4.2 The `Opening` mode and turn

A reserved mode joins the engine's set (`Do`, `Say`, `Story`, `Roll`, `GM`, `System`):

```go
// engine/orchestrator.go — handled beside /gm and /go, before generation.
if strings.EqualFold(mode, "Opening") {
    action = openingDirective(manifest)   // built from settings.opening_prompt
}
```

`openingDirective` produces the instruction the GM receives as the player's action:

```text
[OPENING SCENE]
Establish the opening of this campaign. Describe where the protagonist is, what
they can perceive, and one thing that invites action. Introduce at most one
present character, using their established name. Do not decide the protagonist's
actions, thoughts, or feelings.

<opening_prompt, verbatim, when present>
```

When `opening_prompt` is empty the first paragraph stands alone, which is exactly the "GM creates the scene" default. The resulting turn is recorded with `Mode: "Opening"` and numbered 1, so `/undo` and the chronicle need no special case.

**Guard:** the endpoint accepts `Opening` only while `history.jsonl` is empty; afterwards it is a `400`. The Prologue screen is only shown when the chronicle is empty, so the guard is a safety net, not a workflow.

### 4.3 Storage

`game.yaml` settings gain two keys, both optional strings:

```yaml
settings:
  start_location: aldon-harbour       # existing
  opening_prompt: "Begin in the rain-soaked market at dusk."   # new
```

No new file type. `core.GameManifest.Settings` already carries arbitrary values (`pkg/core/types.go:34`).

### 4.4 The Prologue screen

When `GetChronicle` returns zero turns, `App` renders a Prologue panel instead of an empty turn list:

- The campaign title, world, and system.
- An editable **Opening Prompt** textarea, prefilled from settings; saving it writes `settings.opening_prompt` through a small `PATCH /api/game/{id}/settings` route.
- **Begin the story** — submits `{mode:"Opening", input:""}` through the existing turn stream.
- **Begin with my first action** — focuses the action console; submits a normal first turn.
- A one-line explanation that the opening is turn 1 and can be undone.

The console's normal view replaces the Prologue once a turn exists.

### 4.5 Creation wizard

`CreateGameRequestDTO` (`pkg/gui/types.go:118-124`) gains:

```go
OpeningPrompt        string `json:"opening_prompt,omitempty"`
ProtagonistDetails   string `json:"protagonist_details,omitempty"`
StartingLocation     string `json:"starting_location,omitempty"` // optional entity ID
```

The wizard (`frontend/src/components/LauncherHub.tsx:385-460`) adds an optional **Opening Prompt** textarea and a **Protagonist** description field beneath the name. `InitGame` writes the prompt into settings, uses `starting_location` as the pinned location when it names an existing location entity, and writes `protagonist_details` into the player note body.

### 4.6 TUI parity

`cmd/localrpg/play.go` gains a startup branch: when history is empty, print the opening prompt (if any) and either run the `Opening` turn immediately or prompt for the first action. The engine work is shared, so this is wiring only.

---

## 5. Turns That Always End

### 5.1 Send the generation parameters we already configure

`RouterFromConfig` copies `Temperature` and `MaxTokens` into `ProviderConfig` (`pkg/harness/factory.go:86-87`), and `NewModelProvider` then drops them (`:41-60`). Fix the factory and the HTTP body:

```go
// pkg/harness/http_provider.go — the OpenAI-compatible request body
type httpChatRequest struct {
    Model       string            `json:"model"`
    Messages    []httpMessage     `json:"messages"`
    Stream      bool              `json:"stream"`
    MaxTokens   int               `json:"max_tokens,omitempty"`
    Temperature float64           `json:"temperature,omitempty"`
    Stop        []string          `json:"stop,omitempty"`
}
```

`GenerateRequest` already carries `MaxTokens`/`Temperature` (`pkg/harness/types.go:16-17`); the orchestrator must populate them from config for the `gm` call (`pkg/engine/orchestrator.go:318` passes only `{Prompt: prompt}` today).

### 5.2 Propagate completion

`StreamChunk` gains the provider's terminal signal:

```go
type StreamChunk struct {
    Text         string
    Done         bool
    FinishReason string // "stop", "length", "tool_calls", …
    Error        error
}
```

The HTTP provider parses `finish_reason` from each SSE `choices[].finish_reason` and sets `Done` when it sees `[DONE]` or the first non-empty reason (`pkg/harness/http_provider.go:46-52,123-143`). The CLI provider sets `FinishReason: "stop"` on clean exit. The orchestrator's `generate` loop (`pkg/engine/orchestrator.go:313-343`) records the reason and treats `"length"` as truncation.

### 5.3 Deadlines

Two bounds, both configurable, both defaulting on:

```yaml
agents:
  turn_timeout_seconds: 300     # wall clock for the whole turn
  chunk_timeout_seconds: 60     # silence tolerated between deltas
  max_tokens: 1024              # existing
  temperature: 0.7              # existing
```

`TurnSession.Run` wraps the orchestrator context:

```go
ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Agents.TurnTimeoutSeconds)*time.Second)
defer cancel()
```

The idle bound is enforced inside `generate`: a timer resets on every chunk and, on firing, cancels the turn with `ErrGenerationStalled`:

```go
// pkg/engine/orchestrator.go
idle := time.NewTimer(chunkTimeout)
for {
    select {
    case <-idle.C:
        return "", ErrGenerationStalled
    case chunk, ok := <-chunks:
        if !ok { /* drain streamErr, return accumulated */ }
        if !idle.Stop() { <-idle.C }
        idle.Reset(chunkTimeout)
        // … accumulate and forward …
    }
}
```

Both breaches abort before `RecordTurn`, so a stalled turn writes nothing and releases the per-game lock (`pkg/gui/service.go:411-427`).

### 5.4 Stream observability

`ProcessActionStream` gains a lightweight event callback distinct from text chunks so the UI can show progress without receiving prose:

```go
type TurnProgress struct {
    Stage      string // "loaded", "generating", "extracting", "recording"
    ElapsedMS  int64
    ChunkCount int
    FinishReason string
}
```

Emitted as `{"type":"status",…}` on the existing NDJSON stream. It costs nothing to add and turns "it stalled" into "it was waiting for the first token" or "it was extracting".

### 5.5 Client timeouts

`APIClient.streamTurn` gains an inactivity guard mirroring the server: if no bytes arrive for `chunk_timeout` (plus a small grace), abort the `AbortController` and surface `The narrator stopped responding`. The console shows an elapsed timer while `turnInFlight` is true, so a long-but-live generation is distinguishable from a dead one.

### 5.6 The Stop button preserves the draft

Replace the current handler (`frontend/src/App.tsx:117-120`):

```ts
const handleStopTurn = () => {
  abortRef.current?.abort();
  setDraft({ text: streamedProse, state: 'stopped', input: lastSubmission });
  setTurnInFlight(false);
};
```

Rules:

- An arrived `turn` event commits and clears the draft.
- An `error` event or a thrown transport error also leaves the partial text as a draft, with the failure message shown above it.
- The draft panel offers **Retry** (resubmits the same action, discarding the draft on success), **Discard**, and, when truncation was flagged, **Continue** (`/gm Continue from where you stopped.`).
- Nothing is written to `history.jsonl` for a draft; this already holds because `RecordTurn` is never reached (`pkg/engine/timeline.go:45-66`).

---

## 6. Prose That Reads Like Prose

### 6.1 A constrained Markdown renderer

No markdown dependency exists (`frontend/package.json:11-15`). Add `frontend/src/components/MarkdownProse.tsx`, a dependency-free renderer for the subset the GM actually emits:

| Input | Rendering |
| --- | --- |
| Blank-line-separated blocks | Paragraphs |
| Single newline inside a block | Preserved line break (`whitespace-pre-wrap`), because the model's soft breaks are deliberate beats |
| `**bold**`, `*italic*`, `_italic_` | Inline emphasis |
| `>` line | Blockquote |
| `- ` / `* ` lines | Unordered list |
| `---`, `***`, `___` on their own line | Scene break rule |
| `#`…`######` | Heading |
| `` `code` `` | Inline monospace |
| `[[Target]]`, `[[Target|Label]]` | Existing entity link behaviour (`entity.WikilinkTarget`) |
| Raw HTML | Escaped and shown as text, never executed |

The same component renders narration in `TurnSegments` (`frontend/src/components/TurnSegments.tsx:66-68`), the live stream (`frontend/src/App.tsx:242-250`), the chronicle, and the Story Theater, so one formatting rule applies everywhere.

### 6.2 Prompt instruction

Add to `speechFormattingInstruction` (`pkg/harness/context.go:79-83`):

```text
## PROSE FORMATTING
Separate narration beats with blank lines, one beat per paragraph.
Use plain prose. Do not emit headings, tables, or code fences in narration.
You may use *single asterisks* for emphasis and --- for a scene break.
```

### 6.3 Live streaming view

While chunks arrive, render with `MarkdownProse` too, so paragraphs form as the text streams rather than snapping into shape at the end. Re-rendering a growing string is cheap at paragraph granularity; the renderer memoizes parsed blocks.

---

## 7. Speech: Delineated, Spoken, and Metadata-Free

### 7.1 Remove the incentive to leak metadata

`FormatVoiceProfilesCatalog` currently says "assign an appropriate voice profile ID in their description" (`pkg/harness/context.go:68-69`). This is the source of the leaked IDs. Replace it with an instruction that keeps the helpful part and drops the leak:

```text
## NPC VOICE PROFILES
New characters are voiced automatically from their description. Describe an NPC's
manner of speech (age, temperament, accent, timbre) in their introduction; do not
write voice IDs, tags, or profile names into the narration.
```

The catalog itself may remain in the prompt as context for tone, or be dropped entirely; the decision is to **keep the trait guidance, drop the ID assignment**. `AssignVoiceProfile` (`pkg/harness/extractor.go:64`) already maps tags to profiles deterministically, so nothing is lost.

### 7.2 Sanitize as defence in depth

A `sanitizeProse` step runs before segmentation:

```go
// pkg/dialogue/sanitize.go
// Voice directives that some models volunteer despite instructions.
var metadataPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)\s*[\[\(\{](?:voice|voice[_ ]?id|profile|speaker)\s*[:=]\s*[^\]\)\}]+\s*[\]\)\}]\s*`),
    regexp.MustCompile(`(?im)^\s*voice\s*[:=].*$`),
    regexp.MustCompile(`(?i)\s*\bvoice[_ ]?id\s*[:=]\s*\S+`),
}
```

It strips only recognisable engine vocabulary, never prose. Wikilink labels already resolve through `entity.WikilinkTarget`, so `[[elder-sage|the old woman]]` renders as "the old woman".

### 7.3 Parse speech reliably

Extend `dialogue.Parse` (`pkg/dialogue/dialogue.go:22`) so the shapes models actually produce resolve:

- `Name: "…"` (current)
- `**Name:** "…"` and `*Name:* "…"`
- `Name (any parenthetical): "…"`
- `- Name: "…"` and `> Name: "…"`
- Curly and straight quotes, with the trailing narration after the close quote preserved

Speaker resolution continues to require an entity match, using `harness.MatchExistingEntity` for partial-name and context matching instead of exact equality, so "Evelyn" resolves to "Lady Evelyn". A line whose attribution cannot be resolved stays narration, as today, but the sanitizer guarantees any residual directive is gone.

### 7.4 Playback that actually plays

The audio path already works end to end for the chronicle: `turnDTO` stamps `audio_url` on every segment when TTS is enabled (`pkg/gui/service.go:157-175`) and the endpoint synthesizes on demand (`:555-596`). Three changes make it reliable for a live turn:

1. **Play per completed turn, once.** `useSegmentPlayback` keys autoplay to the turn identity, not to mount, so appending a turn never re-plays earlier ones and a fresh turn does play.
2. **Prefetch the just-finished turn's speech.** After a `turn` event, ask the browser to warm the first speech segment's URL so playback starts without a synthesis pause.
3. **Make speech visually unmistakable.** Keep the amber card (`TurnSegments.tsx:44-64`), add a small play glyph, and render narration through `MarkdownProse`. A speech segment that failed to parse is the real failure mode; §7.3 addresses it.

If TTS is disabled the card remains, with the speaker name plain and no play affordance.

### 7.5 Optional voice debug view

Add `SegmentDTO.VoiceID string json:"voice_id,omitempty"`, populated from the speaker entity's configured voice. The Characters drawer and the turn card can reveal it behind the existing advanced/debug preference. It is never part of the narration text.

---

## 8. The Protagonist Exists

### 8.1 Fix the identity mismatch

Two representations disagree: `game.yaml`'s `player:` holds the display name (`pkg/engine/game.go:58`) while the note is `entities/<slug>.md` (`:137-142`), and `GetGameState` opens `entities/<manifest.Player>.md` (`pkg/gui/service.go:189-191`). The orchestrator, JS bridge, and extractor all pass `manifest.Player` as an entity ID, so a spaced name silently drops the player from every turn.

Resolution, in order:

1. **Write the ID.** `InitGame` stores `Player: entity.Slugify(playerName)` and adds `PlayerName` to the manifest for display.
2. **Resolve legacy values.** A shared helper tolerates old campaigns:

```go
// pkg/engine/player.go
// ResolvePlayerID finds the protagonist's entity ID from a manifest whose Player
// field may be an ID, a legacy display name, or absent.
func ResolvePlayerID(store *storage.Store, manifest *core.GameManifest) (string, error)
```

It tries the manifest value as an ID, then its slug, then a name match over character entities. `GetGameState`, `prepareTurn`, the JS bridge, and the extractor call it instead of reading `manifest.Player` directly (`pkg/gui/service.go:189,471,478`).

3. **Migrate on open.** `localrpg play` and `ensureIndexed` rewrite `game.yaml`'s `player:` to the resolved ID the first time a legacy campaign is opened, so the fix is permanent.

### 8.2 A protagonist worth showing

`ensurePlayerNote` (`pkg/engine/game.go:136-169`) writes `Body: "The player character."`. Creation now passes `protagonist_details` and writes it as the body, or a genre-appropriate default when blank. The note keeps `type: character` and its location link.

The wizard's **Protagonist** field (name plus optional description) is the single place a player describes their character; no stats are required, consistent with the schema-agnostic engine.

### 8.3 The Characters drawer recovers

`CharacterSheetDrawer` currently prints "No character loaded." whenever `gameState.player` is falsy (`frontend/src/components/CharacterSheetDrawer.tsx:9-10`). Two changes:

- `GetGameState` returns the resolved player or a specific error; `App` distinguishes "campaign has no player note" from "no campaign active" (`frontend/src/App.tsx:45-55`).
- The drawer offers **Create protagonist note**, calling a new `POST /api/game/{id}/player` with `{name, details}`, which writes and indexes the note and refreshes state. A campaign is never left in a state where the player cannot be represented.

### 8.4 Campaign metadata

- `InitGame` persists the wizard's title in `manifest.Name` and keeps the slug as `manifest.ID` (`pkg/engine/game.go:53-60; frontend/src/components/LauncherHub.tsx:69-78`).
- `ListGames` sorts by `last_played` and the launcher's "Resume Adventure" uses `games[0]` after that sort (`pkg/gui/service.go:635-673`; `frontend/src/components/LauncherHub.tsx:86`).

---

## 9. API & Data Changes

### 9.1 Endpoints

| Method | Path | Change |
| --- | --- | --- |
| `POST` | `/api/game/{id}/turn` | Accepts `mode: "Opening"`; emits `status` and `warning` events |
| `PATCH` | `/api/game/{id}/settings` | New. Updates `opening_prompt` (and future per-campaign settings) in `game.yaml` |
| `POST` | `/api/game/{id}/player` | New. Creates or updates the protagonist note |

`pkg/gui/server.go`, `pkg/gui/service.go`, `frontend/src/types.ts`, and `frontend/src/api/client.ts` change together, per the project convention.

### 9.2 DTOs

```go
type TurnEvent struct {
    Type      string   `json:"type"`                // "status" | "chunk" | "warning" | "turn" | "error"
    Text      string   `json:"text,omitempty"`
    Stage     string   `json:"stage,omitempty"`
    ElapsedMS int64    `json:"elapsed_ms,omitempty"`
    Message   string   `json:"message,omitempty"`
    Turn      *TurnDTO `json:"turn,omitempty"`
}

type SegmentDTO struct {
    // existing fields…
    VoiceID string `json:"voice_id,omitempty"` // advanced view only
}

type GameSettingsPatchDTO struct {
    OpeningPrompt *string `json:"opening_prompt,omitempty"`
}

type CreatePlayerNoteDTO struct {
    Name    string `json:"name"`
    Details string `json:"details,omitempty"`
}
```

`CreateGameRequestDTO` and `CreateGameResponseDTO` gain `opening_prompt`, `protagonist_details`, and `starting_location`.

### 9.3 Config

```go
type AgentTuningConfig struct {
    TurnTimeoutSeconds  int `yaml:"turn_timeout_seconds" json:"turn_timeout_seconds"`
    ChunkTimeoutSeconds int `yaml:"chunk_timeout_seconds" json:"chunk_timeout_seconds"`
    MaxTokens           int `yaml:"max_tokens" json:"max_tokens"`
    Temperature         float64 `yaml:"temperature" json:"temperature"`
}
```

Defaults: 300s, 60s, 1024, 0.7. Exposed in Settings Studio under AI Agents. `preferences.streaming` is wired: when false, `streamTurn` uses a non-streaming request and the console shows a spinner plus elapsed timer.

---

## 10. UX Improvements Beyond the Fixes

These are deliberate additions that make the turn feel like a table, ordered by value.

1. **Prologue first.** The player is never dropped into an empty console. The GM establishes the scene; the player responds to something. (§4.4)
2. **A visible clock while thinking.** Elapsed time and the latest `status` stage ("waiting for the narrator…", "writing…", "updating the world…"). (§5.4, §5.5)
3. **Drafts are first-class.** Stop, failure, and truncation all leave text you can retry, continue, or discard. History only records completed turns. (§5.6)
4. **Speech as speaker cards.** Distinct amber cards with a play glyph, rendered prose narration around them, no metadata. (§6, §7)
5. **A character you can see.** The protagonist appears in the Characters drawer and in the graph from turn 1. (§8)
6. **Readable prose.** Paragraphs, emphasis, scene breaks, and entity links that open the Codex. (§6)
7. **Honest empty states.** When there is no player note, offer to create one; when the world has no locations, say so in the Prologue. (§8.3)
8. **Resume the actual latest campaign.** Sorted by `last_played`. (§8.4)

---

## 11. Testing Strategy

Backend (standard library only, per project conventions):

- `openingDirective` includes the configured prompt and the no-self-action clause; an empty prompt still produces a coherent instruction.
- `Opening` mode is rejected once history is non-empty (`400`).
- `generate` returns `ErrGenerationStalled` when no chunk arrives within the idle bound, and records nothing (extend `pkg/engine/orchestrator_stream_test.go`).
- A `finish_reason` of `length` is surfaced as a warning and recorded, not treated as a stall.
- `RouterFromConfig` → `NewModelProvider` preserves `MaxTokens`/`Temperature`; the HTTP provider body contains them (extend `pkg/harness` tests).
- `ResolvePlayerID` finds a slugged player from a legacy display-name manifest, and `GetGameState` returns it (regression for the observed bug).
- `sanitizeProse` removes voice directives and leaves ordinary parentheses and colons untouched.
- `dialogue.Parse` resolves each documented speech shape and keeps unresolved attributions as narration.
- `InitGame` persists the title and the slugged player ID.
- `PATCH settings` and `POST player` round-trip through the store.

Frontend:

- `MarkdownProse` renders paragraphs, preserved soft breaks, emphasis, lists, blockquotes, scene breaks, and wikilinks; raw HTML is escaped.
- Stop keeps the draft; retry resubmits; a `turn` event commits and clears it.
- The Characters drawer shows a recovery action when the player is absent.

Manual verification: create a campaign with an opening prompt and a spaced protagonist name, generate the opening, then a `Say` turn containing NPC speech, and confirm prose formatting, speech cards, playback, and a visible protagonist.

---

## 12. Migration & Compatibility

- Legacy `game.yaml` files with a display-name `player:` are repaired on first open (§8.1). No content is rewritten except that field.
- `settings.opening_prompt` is additive; absent means "GM creates the scene".
- New `status`/`warning` events are additive; a client that ignores them still sees `chunk` and `turn`.
- `preferences.streaming: false` switches the request shape, not the endpoint.
- The `.gitkeep` build gotcha is unaffected; no new embedded assets are introduced.

---

## 13. Open Questions

1. Should the Prologue allow a **regenerate** of the opening before the player acts? Proposal: yes, by treating it as `/undo` plus a fresh `Opening` turn; confirm the UX.
2. Should `opening_prompt` be per-world as a default in addition to per-campaign? Proposal: per-campaign only for now; worlds can document a suggested prompt in `lore.md`.
3. Is a separate `Continue` action desirable, or should truncation simply raise `max_tokens`? Proposal: ship the warning and a manual continue; revisit if truncation is common.
4. Should voice-profile assignment remain in the prompt at all? Proposal: keep the catalog for tone but never ask for IDs.
5. Does the Wails webview deliver NDJSON progressively? Assumed yes, and the design is correct either way because the non-streaming path also works.

---

## 14. File Map

**Create**

- `frontend/src/components/MarkdownProse.tsx` — constrained Markdown renderer
- `frontend/src/components/ProloguePanel.tsx` — opening configuration and Begin actions
- `frontend/src/components/TurnDraftPanel.tsx` — stopped/failed draft with Retry/Discard/Continue
- `pkg/dialogue/sanitize.go` (+ test) — metadata stripping
- `pkg/engine/player.go` (+ test) — `ResolvePlayerID`
- `docs/superpowers/plans/2026-09-22-gameplay-unblock-play.md` — the increment 1 plan

**Modify**

- `pkg/engine/orchestrator.go` — `Opening` mode, deadlines, `finish_reason`, progress events
- `pkg/engine/game.go` — title, slugged player ID, opening prompt, protagonist body
- `pkg/engine/startlocation.go` — honour a chosen starting location
- `pkg/harness/context.go` — prose and voice instructions
- `pkg/harness/types.go`, `factory.go`, `http_provider.go`, `cli_provider.go` — generation parameters and completion
- `pkg/config/types.go` — agent tuning fields and defaults
- `pkg/gui/service.go` — player resolution, settings patch, player route, opening turn, progress forwarding
- `pkg/gui/server.go`, `pkg/gui/types.go` — routes and DTOs
- `frontend/src/App.tsx` — draft state, Prologue routing, status display, resolved player
- `frontend/src/components/TurnSegments.tsx` — `MarkdownProse`, speech affordances
- `frontend/src/components/CharacterSheetDrawer.tsx` — recovery action
- `frontend/src/components/ActionConsole.tsx` — elapsed timer and stop semantics
- `frontend/src/components/LauncherHub.tsx` — opening prompt, protagonist details, latest-first resume
- `frontend/src/api/client.ts`, `frontend/src/types.ts` — new endpoints and fields
- `cmd/localrpg/play.go` — TUI opening parity

---

## 15. Delivery Increments

1. **Unblock play** — player identity fix, campaign title, deadlines and generation parameters. Smallest change that makes the observed failures stop.
2. **Opening** — `Opening` mode, settings patch, Prologue screen, wizard fields.
3. **Prose and speech** — `MarkdownProse`, prompt changes, sanitizer, parser, playback.
4. **Draft UX** — stop/retry/continue, status events, elapsed timer, streaming preference.
5. **Polish** — character recovery, latest-first resume, advanced voice view, TUI parity.
