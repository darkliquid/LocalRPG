# Opening Scene Restatement Design

**Date:** 2026-10-05
**Status:** Proposed
**Amends:** §4.2 of `2026-09-22-gameplay-experience-design.md` (the opening
directive is no longer the first turn's *action*)
**Scope:** the campaign's first turn, `pkg/engine` (opening handling),
`pkg/harness` (context assembly), `pkg/gui` (turn request, Prologue wiring),
`cmd/localrpg/play.go` (TUI wiring), the Prologue panel
**Related:** `pkg/engine/orchestrator.go:63` (`openingDirective`),
`pkg/harness/context.go:203` (`actionEchoInstruction`),
`frontend/src/components/ProloguePanel.tsx`,
`frontend/src/App.tsx:529` (`handleBeginStory`)

---

## 1. Problem

A new campaign's opening scene is a piece of prose the player writes (or the AI
generates) into the **opening directive**, stored as `settings.opening_prompt`
and shown on the Prologue dialog (`frontend/src/components/ProloguePanel.tsx`).
It establishes where the story begins, when, and what is already happening. For
example:

> Fire rains down from above as the smoking wreckage of the airship *Dawnbreaker*
> tumbles past the palace window. Below, the old market burns.

The first turn the GM writes is, in practice, only the **continuation** of that
prompt. It produces the hooks, a figure in the doorway, a sound from the corridor,
a choice presented, but it does not restate the establishing context those hooks
depend on. A player who has scrolled past the Prologue, or who returns to the
campaign the next day, reads a turn about a burning market with no statement that
there was ever a fire.

The mechanical cause is in `openingDirective` (`pkg/engine/orchestrator.go:63`).
Its text is handed to the assembler as the turn's **action**
(`ContextRequest.Action`, `pkg/engine/orchestrator.go:806`), so the prompt renders
it under `## PLAYER ACTION` (`pkg/harness/context.go:277`) with the protagonist's
name prefixed. The GM reads the scene as something the *player said* and responds
to it. The scene is therefore "only responded to": the reply is written as if the
establishing context were already on screen, which it is not, because the only
place it ever appears is the Prologue dialog and the campaign settings panel.

## 2. The insight: this is `actionEcho` for the opening

The engine already solved the identical problem for player actions.
`actionEchoInstruction` (`pkg/harness/context.go:203`) tells the narrator to open
every turn with one short segment that **restates the player's action** before
resolving it, precisely so a turn never starts mid-consequence.

The opening scene needs the same treatment, applied once: **the first turn opens
by restating the opening scene, then continues.** The scene is the turn's frame,
not its prompt.

## 3. Design

### 3.1 The opening scene becomes its own prompt section

`ContextRequest` (`pkg/harness/context.go:79`) gains two fields:

```go
// OpeningScene is the campaign's opening prose, carried on the first turn so
// the narrator can restate it before continuing.
OpeningScene string
// OpeningHooks asks the turn to extend the scene with events that invite the
// protagonist to act. False for the quiet scene turn that precedes the player's
// own first action.
OpeningHooks bool
```

A new section renders them, modelled on `actionEchoSection`:

```go
// openingSceneInstruction tells the narrator to restate the campaign's opening
// scene before continuing, so the first turn never starts mid-consequence.
const openingSceneInstruction = `## OPENING SCENE
This is the campaign's first turn. Open the narration by restating the scene below
in your own words, keeping its facts, imagery, and mood, so the player can tell
where they are and what is already happening from this turn alone. Do not skip
straight to the consequences.

Do not decide the protagonist's actions, thoughts, or feelings.
`

// openingHooksInstruction asks for the events that draw the player in. It is the
// current opening directive's second half, now separable from the scene text.
const openingHooksInstruction = `Once the scene is established, continue with one
thing that invites the protagonist to act, and at most one present character,
named as they are already known. Stop there; do not resolve the protagonist's
response.
`
```

The section is placed beside `action_echo` in `buildSections`
(`pkg/harness/context.go:284`), before `canon`, and is **not droppable**: a turn
whose whole purpose is to establish the scene must not lose it to the token
budget. It is emitted on the first turn whenever there is a scene to restate or
hooks to add.

### 3.2 The opening turn stops passing the scene as the action

`openingDirective` (`pkg/engine/orchestrator.go:63`) currently fuses the scene
text with the framing instruction. The two are separated: the scene text travels
in `OpeningScene`, and the framing moves into the two instructions above.

The opening turn (`pkg/engine/orchestrator.go:697`) therefore sets an **empty**
action and a non-empty scene:

```go
isOpening := strings.EqualFold(mode, OpeningMode)
if isOpening {
    if len(pastTurns) > 0 {
        return nil, fmt.Errorf("the campaign has already begun")
    }
    mode = OpeningMode
    generationPrompt = "" // the scene is the frame, not the player's action
}
```

`openingDirective` is deleted once nothing calls it. The `[OPENING SCENE]` banner
disappears from the prompt; the new section replaces it.

### 3.3 The action section is omitted when there is no action

`buildSections` currently always emits `## PLAYER ACTION`, including the format
reminder and the protagonist's name (`pkg/harness/context.go:277`). An opening
turn has no action, so the section is skipped when both the action and the player
name are empty. This is what stops the GM from responding to a scene nobody said.

The format reminder (`> Speaker:` framing, `@persona` guidance) currently rides
inside the action section. It is hoisted into its own always-present section so
the opening turn still learns the reply format. `turnFramingInstruction`
(`pkg/harness/context.go:227`) already exists for exactly this and simply needs
its `> ` reminder folded in.

### 3.4 Two flavours of the first turn

The scene turn comes in two flavours, distinguished by one new field on the turn
request:

```go
// pkg/gui/types.go — TurnRequest
type TurnRequest struct {
    Mode      string `json:"mode"`
    Input     string `json:"input"`
    SceneOnly bool   `json:"scene_only,omitempty"` // restate the scene, add no hooks
}
```

`SceneOnly` is carried to the orchestrator per turn, using the same shape as
`SetPendingCheckRef` (`pkg/engine/orchestrator.go:515`), and is meaningful only
for `Opening` mode. It never changes the recorded mode: both flavours record as
`Opening`, so the chronicle's `[Opening]` label (`ChronicleView.tsx:108`) is
unchanged.

The full matrix:

| `opening_prompt` | Entry point | Turn 1 | Turn 2 |
|---|---|---|---|
| set | **Begin the story** — `{mode:"Opening"}` | The scene, restated, then hooks that invite action | — |
| set | **I'll take the first step** — `{mode:"Opening", scene_only:true}` | The scene, restated, and nothing else | The player's action |
| blank | **Begin the story** — `{mode:"Opening"}` | A scene invented from world, rules, and lore, then hooks | — |
| blank | **I'll take the first step** — any mode | The player's action | — |

The last row is the current behaviour, unchanged: with no starting prompt there is
nothing to restate, so no scene turn is inserted and the player's action is the
first turn.

### 3.5 The Prologue wiring

`ProloguePanel` (`frontend/src/components/ProloguePanel.tsx`) keeps its two
buttons. Only the second changes:

```ts
// frontend/src/App.tsx — handleBeginWithAction
const handleBeginWithAction = async () => {
  if (gameState?.opening_prompt) {
    // A scene turn establishes the scene quietly; the console then takes the
    // player's own first step as turn 2.
    await handleActionSubmit('Opening', '', { sceneOnly: true });
  }
  document.getElementById('action-console-input')?.focus();
};
```

`handleBeginStory` (`App.tsx:532`) is unchanged: it saves the prompt and submits
`{mode:"Opening"}`. The client's only fetch layer
(`frontend/src/api/client.ts:682`) gains `scene_only?: boolean` on the turn body,
and `frontend/src/types.ts` mirrors it.

### 3.6 What is deliberately unchanged

- **No new storage.** `settings.opening_prompt` is already the scene. There is no
  new file, no new setting, and no persisted "opening narration".
- **The guard stays.** A second `Opening` turn is still refused once history
  exists (`pkg/engine/orchestrator.go:699`).
- **The `opening-scene` location entity is untouched.** `OpeningSceneEntityID`
  (`pkg/engine/startlocation.go:24`) is a synthesised *location* note, unrelated
  to the opening *prose*. The two share a name; the code and this design keep them
  distinct.
- **`/undo` needs no special case.** Rewinding to zero turns re-shows the Prologue,
  and the next scene turn restates the scene again.
- **Restart inherits it.** `RestartGame` (`pkg/gui/service.go:3828`) preserves
  `opening_prompt`, so a restarted campaign's first turn restates it.

## 4. TUI parity

`localrpg play` wires the orchestrator in `cmd/localrpg/play.go` but never calls
`SetOpeningPrompt`, and its mode cycle has no `Opening`
(`pkg/tui/app.go:36`). The TUI has no Prologue, so its first turn is a normal
mode. To match the GUI, the TUI runs the quiet scene turn at startup when the
campaign is empty and the prompt is set:

```go
orchestrator.SetOpeningPrompt(engine.OpeningPrompt(manifest))
// with an empty history and a prompt, establish the scene before the first input
```

## 5. Testing

| Test | Level | Asserts |
|---|---|---|
| `TestOpeningTurnRestatesTheScene` | engine | `ProcessActionStream("Opening", "")` records a turn whose narration contains the scene's distinctive text |
| `TestOpeningTurnWithHooksInvitesAction` | engine | the non-`SceneOnly` opening prompt carries the hooks instruction |
| `TestQuietSceneTurnAddsNoHooks` | engine | `SceneOnly` opening turn carries the restatement and not the hooks instruction |
| `TestOpeningSceneNotCarriedAfterTurnOne` | engine | turn 2's prompt contains no opening scene section |
| `TestEmptyOpeningPromptEstablishesFromWorld` | engine | an absent directive still yields an `Opening` turn, as today |
| `TestOpeningTurnOmitsPlayerAction` | harness | `Assemble` with an empty action emits no `## PLAYER ACTION` section and still emits the framing section |
| `TestOpeningSceneSectionIsNotDroppable` | harness | a tiny budget trims canon before the scene section |
| `TestTurnSessionRestatesTheSceneOnFirstTurn` | gui | the streamed `Opening` turn carries the scene; `scene_only` reaches the orchestrator |
| `TestBeginWithActionRunsAQuietSceneFirst` | gui | a first action submitted after a scene turn is turn 2 |

Existing tests that assert the current shape must be updated:
`TestOpeningModeEstablishesTheFirstTurnOnly`
(`pkg/engine/orchestrator_stream_test.go:329`) and
`TestTurnSessionRunsAnOpeningTurn` (`pkg/gui/turn_test.go:200`).

Commands: `go test ./pkg/engine/ ./pkg/harness/ ./pkg/gui/`, `go vet ./...`, and
`npm run build` in `frontend/` for the type gate.

## 6. Options considered

**Generate and persist a separate opening narration, then restate it.** Rejected:
it costs an extra GM call at campaign open, and it makes the first turn a
restatement of a hidden artefact rather than of the text the player actually
wrote. The directive is already the scene.

**Fold the scene restatement into the player's first action turn.** Rejected:
it contradicts the requested shape, in which "I'll take the first step" produces
the scene alone and leaves the player's own step to the turn after.

**A separate reserved mode for the quiet scene turn.** Rejected in favour of a
request field: the chronicle renders `[turn.mode]`, and a second mode would print
a second label for what is the same thing, the opening scene.

**Echo the directive verbatim in the chronicle.** Rejected: the directive may be
an instruction ("Begin at dusk in the market") rather than prose, and a verbatim
first-person line ("the city gates closing behind me") reads wrong in a third
person turn. "Restate" means narrate faithfully, not reproduce.

## 7. Out of scope

- Changing how the Prologue generates or stores the opening prompt.
- Generating opening art or a title card.
- Any change to `OpeningSceneEntityID` and start-location resolution.

## 8. File map

| File | Change |
|---|---|
| `pkg/harness/context.go` | Add `ContextRequest.OpeningScene` and `OpeningHooks`, `openingSceneInstruction`, `openingHooksInstruction`, `openingSceneSection`; emit the section and drop it from the droppable set; omit `## PLAYER ACTION` when the action and player name are empty; hoist the `> ` format reminder into the framing section |
| `pkg/engine/orchestrator.go` | Set `OpeningScene`/`OpeningHooks` on the first turn; empty the opening turn's action; add `SetSceneOnly`; delete `openingDirective` |
| `pkg/gui/types.go` | Add `TurnRequest.SceneOnly` |
| `pkg/gui/service.go` | Pass `req.SceneOnly` to the orchestrator before the turn |
| `cmd/localrpg/play.go` | Call `SetOpeningPrompt`; run the quiet scene turn at startup when the campaign is empty |
| `frontend/src/types.ts` | Add `scene_only?: boolean` to the turn request |
| `frontend/src/api/client.ts` | Send `scene_only` on the turn body |
| `frontend/src/App.tsx` | `handleBeginWithAction` submits the quiet scene turn when a prompt is set |
| `pkg/engine/orchestrator_stream_test.go` | Update the opening-turn test; add the restatement and hooks tests |
| `pkg/gui/turn_test.go` | Update the opening-turn session test; add the quiet-scene and action-first cases |
| `pkg/harness/context_test.go` | Section presence, absence, and budget tests |
