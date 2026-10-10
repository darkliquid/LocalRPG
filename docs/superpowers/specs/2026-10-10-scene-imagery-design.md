# Scene Imagery Design

**Date:** 2026-10-10
**Status:** Proposed
**Issue:** [#132](https://github.com/darkliquid/LocalRPG/issues/132)
**Epic:** [#124 Scene imagery quality and cadence](https://github.com/darkliquid/LocalRPG/issues/124)
**Depends on:** [Turn-Aware Scene Prompt Design](2026-10-05-turn-aware-scene-prompt-design.md), [Scene Consistency Design](2026-10-05-scene-consistency-design.md), [Image Trigger Policy Design](2026-10-05-image-trigger-policy-design.md), [Theatre Effects Design](2026-10-05-theatre-effects-design.md)
**Scope:** `pkg/engine`, `pkg/gui`, `pkg/media`, `pkg/config`, `frontend`, `pkg/gui/docs`

---

## 1. Problem

Two faults in turn-scene illustration.

### 1.1 The scene prompt does not describe the characters

`BuildScenePrompt` (`pkg/engine/scene_worker.go:43-89`) composes the prompt from a cue,
a narration excerpt, the action, the cast, the location, the location's appearance, a
`SceneStyle` clause, the world style, an outcome tone, and a fixed suffix. The cast is
`ScenePromptContext.Entities []string` (`scene_worker.go:21`), and it is **display names
only**: `presentEntityNames` (`scene_worker.go:152-177`) fetches each entity with
`o.store.GetEntity(id)` but reads only `ent.Name`. `ent.Appearance` is never touched.

The only `Appearance` the prompt reads is the **location's** (`orchestrator.go:1593-1594`,
`service.go:2722-2723`). So a character's authored `appearance` reaches the prompt only
through the accident of also being the place. The portrait pipeline does read it
(`BuildPortraitPrompt`, `pkg/engine/portrait_worker.go:18-39`, reads
`ent.Appearance`), which proves the field is there and used elsewhere; the scene prompt
simply never consults it.

The manual path is worse: `Service.GenerateTurnSceneImage` (`pkg/gui/service.go:2688-2735`)
builds a `ScenePromptContext` with no `Entities` at all, so a hand-generated scene image
names no one.

The two design specs confirm this was scoped out, not lost: the turn-aware scene prompt
spec defines `Appearance` as "the location's authored appearance" and the cast as names
(§4.1, §4.2), and the scene consistency spec calls character consistency "a separate
problem" and a non-goal (§3). The gap is deliberate in the specs and a defect in the
result.

### 1.2 Images are generated far more often than the scene changes

The trigger policy defaults to `significant` (`pkg/config/types.go:474`), resolved every
turn (`pkg/gui/service.go:2236`) and applied by `shouldIllustrate`
(`pkg/engine/image_trigger.go:80-105`). The `significant` heuristic fires on **any** of
(`image_trigger.go:24-43`):

- a scene break (`turn.SceneBreak`, from a `---` in the prose or the extractor's
  `scene_break`);
- a **decisive** check outcome, where decisive is any of
  `strong, success, pass, critical, miss, fail, failure` (`DefaultTriggerConfig`,
  `image_trigger.go:15-20`);
- a location change;
- **any new speaker** (`image_trigger.go:60`), which is a character's first line, not a
  major introduction;
- **more than 400 runes of narration**, which ordinary long turns exceed.

The last two fire on nearly every substantial turn, which is what "generating new scenery
images a lot" is. The trigger spec itself notes the default changed from scene-break-only
to `significant` and "produces more images than before" (§7).

There is also no reuse. `SceneWorker.writeScene` (`scene_worker.go:457-489`) writes a
fixed `assets/scenes/turn-<N>.<ext>` unconditionally; the only guard is an in-flight map
keyed `gameID:turnNumber` (`scene_worker.go:351-374`), deleted when the job ends. There is
no prompt hash and no existing-file check, so a re-run or repair of a turn re-bills the
image. The turn-aware spec's §4.4 claim that "the scene image is content-addressed by the
prompt" is not what the code does; the location backdrop is the pipeline that is
content-addressed (`pkg/media/image.go:36-54`, `ComputeArtCacheKey`), and it is a
different artifact.

The budget that could bound this is dead code: `configureSceneBudget`
(`pkg/gui/service.go:2541`), which would call `SetBudget`, `SetApproval`, `SetPrice`, and
`SetProceduralFallback`, is never called. The per-turn worker (`service.go:2188`) and the
manual worker (`service.go:2730`) get no budget, so `ImageBudget.Exhausted()` is always
false (`pkg/engine/scene_worker.go:388`) and the campaign's `max_images` setting is
displayed but never enforced.

Finally, the manual path always regenerates: `POST /scene-image` builds a new worker and
enqueues every time, and the only UI control is "Generate image", shown on turns that have
**no** image (`frontend/src/components/ChronicleView.tsx:133-143`). There is no way to
regenerate deliberately and no way to leave an image alone.

## 2. Goals

- The scene prompt describes each present character from its authored `appearance`, so an
  image can match the character entity.
- A scene image is generated only on a major scene change: a change of location, the
  introduction of a major new character, an extreme consequence, or an explicit scene
  break.
- Turns that are not a major change reuse the current scene image instead of generating a
  new one.
- A regeneration is deliberate, bounded by the campaign's image budget, and never
  overwrites an unchanged scene.
- The existing `max_images` budget and approval mode actually apply.

## 3. Non-goals

- Image conditioning on a character's portrait (a second reference image), which depends
  on provider support; the text appearance is the fix here, and portrait conditioning is a
  follow-up.
- Changing the location backdrop pipeline, which is already cached and correct.
- Theatre effects, which animate existing stills.
- A model tool or record for requesting an image; the trigger stays deterministic, driven
  by the turn's own facts.
- Generating images for turns that had none retroactively.

## 4. Design

### 4.1 The cast, with appearance

`ScenePromptContext`'s `Entities []string` becomes a cast member with a visual
description:

```go
// SceneCast is one character the scene shows, described well enough to draw.
type SceneCast struct {
	Name       string // display name
	Appearance string // authored appearance, or a bounded fallback
}

type ScenePromptContext struct {
	// …
	Cast []SceneCast // present characters, described
	// …
}
```

`presentEntityNames` becomes `presentSceneCast`, reading `ent.Appearance` and falling
back, when it is empty, to the same shape `BuildLocationPrompt` uses
(`pkg/media/image.go:168-195`): the name plus a bounded body excerpt and tags. Each
description is capped (about 120 characters) and the cast stays capped at four
(`sceneEntityCap`), so the prompt does not grow into a page.

`BuildScenePrompt` renders the clause as `characters: Name (appearance); Name2
(appearance)`. `ScenePromptPrefix` (the stable part) is unchanged: it stays
location-plus-style so two turns in one scene share a look; the cast is the per-turn part,
as it already is.

The manual path (`Service.GenerateTurnSceneImage`) builds the same cast by calling the
shared helper, so a hand-generated scene names and describes its characters too.

Docs: `pkg/gui/docs/22-editing-content.md` already says `appearance`, `gender`, `age` are
"fed into image and narration prompts"; the sentence becomes true for scene images, and
the scene prompt specs get a note that the cast now carries appearance.

### 4.2 The major-change trigger

Add a policy value `major` and make it the default in `ImageTrigger()`
(`pkg/config/types.go:474`). The existing `off`, `manual`, `scene_break`, `significant`,
and `every_turn` remain available; `significant` is documented as the older, broader
heuristic.

`major` fires only when one of these is true:

1. **An explicit scene break.** `turn.SceneBreak`, unchanged: a `---` rule break in the
   prose or the extractor's `scene_break` object.
2. **The location changed.** `turn.Location != prev.Location`. The existing comparison
   (`image_trigger.go:38`) is kept; because a move applies from the next turn, the image
   lands on the turn that first happens in the new place, which is correct.
3. **A major new character appeared.** A present speaker that resolves to a stored entity
   of type `character`, was not present in the previous `majorCharacterWindow` turns
   (default 5), and is not tagged with an ambient tag (`minor`, `extra`, `background`,
   `crowd`). This replaces the current "any new speaker" rule, which fires on a returning
   character's first line in a turn.
4. **An extreme consequence.** A check outcome in `ExtremeOutcomes`, a set separate from
   the broad `DecisiveOutcomes`: default
   `critical, critical_success, fumble, critical_failure`. A routine success or miss no
   longer triggers; only a critical result does. The set is configurable so a system whose
   vocabulary names its extremes can list them.

The narration-length rule is **removed** from the default path. A long turn is not a scene
change; it was the single largest source of spurious images. `DefaultTriggerConfig`
(`pkg/engine/image_trigger.go:15-20`) keeps the field so an explicit `significant` policy
still honours it, but `major` does not consult it.

`shouldIllustrate` (`image_trigger.go:80-105`) gains the `major` branch:

```go
case "major":
	return MajorSceneChange(turn, prev, cfg, recentEntityIDs)
```

`MajorSceneChange` is a pure function beside `ShouldIllustrate`, so it is unit-testable
without an orchestrator, exactly as `ShouldIllustrate` is.

### 4.3 Scenes are units, not turns

Today an image is a property of a turn (`turn-<N>.ext`). Under the new policy most turns
have no image, so an image is a property of a **scene**, and every turn in that scene
shows the same one. The campaign manifest's settings map already carries
engine-owned keys (the image budget is stored as `"image_budget"`,
`pkg/engine/image_budget.go:31`), so the scene state rides the same mechanism:

| Setting key | Type | Meaning |
| --- | --- | --- |
| `scene_serial` | int | increments on each generated scene |
| `scene_image` | string | the current scene image's relative path |
| `scene_prompt_hash` | string | the hash of the prompt that produced it |

A new `pkg/engine/scene_index.go` owns reads and writes through the existing settings
accessors, in the shape `ImageBudgetFromManifest`/`SetImageBudget` already use
(`image_budget.go:74-110`), so no manifest struct changes.

An `assets/scenes/index.json` records each scene's serial, the turn it began on, its
prompt hash, and its path. It is the read model for "which image was current on turn N":

```json
{
  "scenes": [
    { "serial": 1, "from_turn": 1, "path": "assets/scenes/scene-1.webp", "prompt_hash": "…" },
    { "serial": 2, "from_turn": 14, "path": "assets/scenes/scene-2.webp", "prompt_hash": "…" }
  ]
}
```

The turn DTO's `image_url` is resolved from the index: the scene whose `from_turn` is the
last one at or before the turn. So every turn in a scene shows that scene's image, and a
turn generated by the old pipeline keeps showing its `turn-<N>.ext` (the resolver falls
back to the per-turn file when the index has no earlier scene). `GetTurnSceneImage`
(`pkg/gui/service.go:2739`) keeps serving the legacy path for old turns.

New images are written as `assets/scenes/scene-<serial>.<ext>`. Scene files are
immutable: a scene is written once and never overwritten, so a re-run cannot re-bill it.

### 4.4 Generation, dedup, and budget

The automatic path (`orchestrator.go:1575-1607`) changes to:

1. Compute the current scene's prompt and its hash (the prompt is already deterministic,
   `TestBuildScenePromptIsDeterministic`).
2. If the turn is not a major change, do nothing: the turn inherits the current scene via
   the index. **No provider call.**
3. If it is a major change but the prompt hash equals the current scene's (nothing visual
   actually changed, for example a new character who was already drawn), do nothing.
4. Otherwise increment `scene_serial`, enqueue the generation, and on success append to
   the index and update `scene_image` and `scene_prompt_hash`.

The worker stops writing `turn-<N>.<ext>` for new scenes and gains the existence guard it
lacks: `writeScene` never overwrites a `scene-<serial>` file.

**The budget is wired.** `configureSceneBudget` (`service.go:2541`) is called by the turn
setup that creates the worker (`service.go:2188`) and by the manual path
(`service.go:2730`), so `SetBudget` (from the campaign's `max_images`), `SetApproval`,
`SetPrice`, and `SetProceduralFallback` take effect. When the budget is exhausted
`plan()` already falls back to the procedural generator or skips
(`scene_worker.go:388-396`); with the budget wired, that path is reachable.

### 4.5 Manual generation and regeneration

The manual path (`Service.GenerateTurnSceneImage`) becomes "generate a scene image for
this turn" and:

- builds the cast (4.1), so it describes the characters;
- resolves the scene the turn belongs to and, when one exists, returns it instead of
  generating a duplicate unless `force` is passed;
- respects the budget.

The UI stops hiding the action when an image exists. `ChronicleView.tsx:133-143` renders
"Generate image" for an image-less turn and **"Regenerate scene"** for a turn that has
one; regenerate is an explicit `POST` with `force`, and the client's existing
`generateTurnSceneImage` gains an optional `force`. Regeneration writes a **new** scene
serial rather than overwriting the old file, so the previous image stays available.

### 4.6 Docs

`pkg/gui/docs` entries on images and the studio are updated: the cast carries appearance,
the default trigger is a major change, a scene image persists across the turns of its
scene, and regeneration is explicit and budgeted. The image trigger spec's default-policy
note is amended to point at this one.

## 5. Behaviour

| Situation | Before | After |
| --- | --- | --- |
| A character with an authored appearance is in the scene | prompt says only the name | prompt says the name and the appearance |
| A long narrated turn (500 runes) | new image | no image; the scene image persists |
| A returning character speaks for the first time this turn | new image | no image |
| A critical success or fumble | new image | new image |
| A routine success or miss | new image | no image |
| The player moves | new image on the next turn | new image on the next turn |
| A new major character appears | new image | new image |
| A turn is re-run or repaired | the image is re-billed | the scene is unchanged, no call |
| The image budget is exhausted | ignored | procedural fallback or skipped |
| A turn in the middle of a scene in the chronicle | no image | the scene's image |
| Regenerate an existing image | impossible | explicit, forced, budgeted |

## 6. Testing

- `pkg/engine`: `BuildScenePrompt` renders a cast member's appearance and falls back to a
  body excerpt when `Appearance` is empty (extend `scene_prompt_test.go`); the manual and
  automatic paths both produce a cast (a test that the manual builder is not the
  names-only shape).
- `pkg/engine`: `MajorSceneChange` fires on a location change, a major new character, an
  extreme outcome, and an explicit scene break, and does **not** fire on a long narration
  or a new-but-minor speaker. This is the core of the fix and is a pure-function table
  test beside `shouldIllustrate`.
- `pkg/engine`: a non-major turn makes no provider call (a counting fake generator);
  a major turn whose prompt hash is unchanged makes no call; a re-run of the same turn
  writes no second file.
- `pkg/engine`: the scene index resolves turn N to the scene whose `from_turn` covers it,
  and falls back to a legacy `turn-<N>.ext`.
- `pkg/gui`: `configureSceneBudget` is reached by the turn worker and the manual path, and
  an exhausted budget yields the procedural fallback (`scene_image_test.go`,
  `turn_audio`-style fixtures).
- `frontend`: `ChronicleView` shows "Generate image" with no image and "Regenerate scene"
  with one; regenerate sends `force`.
- A migration test: a campaign with only `turn-<N>.ext` files still serves them and shows
  them against the right turns.

## 7. Rollout

Three independent slices:

1. **Cast appearance** (4.1): prompt-only, frontend-neutral, ships alone and fixes the
   "nothing like the character" complaint.
2. **Trigger policy** (4.2): a new default plus a pure function; existing campaigns get
   fewer images with no data change.
3. **Scene model, dedup, and budget** (4.3-4.5): the storage change, with the index and
   the legacy fallback so nothing existing breaks.

Slice 1 and 2 can share a release; slice 3 follows. The budget wiring (4.4) is a bug fix
and can ship with slice 2.

## 8. Risks

- **Fewer images may read as "broken".** A user on `significant` who upgrades to `major`
  sees far fewer images. The policy is a config value, so anyone who wants the old
  behaviour sets `trigger: significant`; the docs say so.
- **The scene index is derived state that must not drift.** It is written by the engine
  through the same settings accessors as the image budget, and the resolver falls back to
  the legacy per-turn file when the index has no entry, so a missing or stale index
  degrades to today's behaviour rather than to a blank.
- **A major change with an identical prompt is skipped.** That is the point (no re-bill),
  but it means "a change happened, why is the image the same?" is possible when the change
  is invisible to the prompt, for example a location whose entities were already drawn.
  The hash is of the full prompt, so any visual change produces a new scene.
- **"Major new character" is a heuristic.** Tagging an ambient character `minor` (or
  `extra`, `background`, `crowd`) is how an author says "do not illustrate this", and the
  window bounds a returning face. A wrong guess costs one image, not a broken turn.
- **Portrait conditioning is still absent.** A described appearance improves matching but
  cannot guarantee a likeness the way an image reference would; that is the follow-up
  named in §3, and this spec does not pretend otherwise.
- **The budget now bites.** Wiring `configureSceneBudget` means a campaign with a low
  `max_images` stops generating images where it previously ignored the cap. That is the
  intended behaviour, and the existing `GenerationLimitNotice` surfaces the limit.
