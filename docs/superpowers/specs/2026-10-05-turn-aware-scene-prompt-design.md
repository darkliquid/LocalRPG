# Turn-Aware Scene Prompt Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#51 IMG-1](https://github.com/darkliquid/LocalRPG/issues/51)
**Epic:** [#20 Scene images from turn content](https://github.com/darkliquid/LocalRPG/issues/20)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §4 (IMG-1)
**Scope:** `pkg/engine`, `pkg/media`

---

## 1. Problem

The image a turn produces barely knows what happened. `BuildScenePrompt` composes the prompt from a
cue, the location's name and appearance, and the world style:

```go
scenePrompt := BuildScenePrompt(cue, locEntity, o.worldArtStyle)
```

(`pkg/engine/orchestrator.go:1462-1473`, `pkg/engine/scene_worker.go:17-35`), where the cue is the
paragraph after a `---` rule or an extractor `VisualCue`
(`pkg/engine/scene_worker.go:38-67`). The narration, the player's action, the characters present,
and whether the roll succeeded never reach the prompt.

So a turn where the player fails to pick a lock in a room of silent guards can produce the same
image as the turn they walked in, and the only thing that varies is whether the model happened to
write a rule break.

## 2. Goals

- The prompt reflects the turn: what was narrated, what the player did, who was present, and the
  outcome tone.
- It stays bounded and deterministic, so the art cache key remains meaningful.
- The location and world style still anchor the image; turn content refines it.
- The change is additive: a turn with none of the new inputs produces today's prompt.

## 3. Non-goals

- When to generate (IMG-2) and consistency across scenes (IMG-3).
- Cost guardrails (IMG-4) and export parity (IMG-5).
- Any change to the location backdrop pipeline, which stays description-based.

## 4. Design

### 4.1 A structured context

`pkg/engine` (or `pkg/media`, where the prompt builder lives) gains:

```go
// ScenePromptContext is everything a scene prompt is composed from.
type ScenePromptContext struct {
	Cue       string   // the extractor cue or rule-break paragraph, if any
	Narration string   // the turn's narration, for an excerpt
	Action    string   // the player's raw action
	Entities  []string // present characters, by display name
	Location  string   // the location's name
	Appearance string  // the location's authored appearance, if any
	Style     string   // the world art style
	Outcome   string   // the resolved check's outcome, if any
}
```

`BuildScenePrompt(ctx ScenePromptContext) string` replaces the three-argument form. The old form is
kept as a thin wrapper that fills only cue/location/appearance/style, so existing callers and tests
are unchanged.

### 4.2 Composition

The prompt is assembled in a fixed order, each part bounded:

1. **Subject** — the cue when present, else a short narration excerpt (the first sentence, capped).
2. **Action** — the player's action, capped.
3. **Cast** — the present entities, up to a small limit, so the image can show them.
4. **Place** — the location name and its authored appearance.
5. **Mood** — the world style, plus outcome-tone words derived from the resolved check:
   success → "triumphant, bright", weak → "tense, uncertain", miss → "ominous, shadowed". An
   unknown or absent outcome adds nothing.
6. **Quality suffix** — the existing fixed suffix.

Caps: narration excerpt ≤ 200 characters, action ≤ 160, entities ≤ 4 names, so the prompt stays
short and the provider is not asked to reconcile a page of prose.

### 4.3 Where the inputs come from

At the enqueue site (`pkg/engine/orchestrator.go:1462-1473`), the orchestrator already has:

- `turn.Narration` (the recorded prose),
- the player's action (the mode input),
- the resolved checks (`turn.Checks`, so the outcome is the first check's outcome, if any),
- the location entity and the world style,
- and the present entities from the turn's mentions (`turn.Segments` speakers, or
  `ResolveEntityMentions`).

The enqueue builds a `ScenePromptContext` from these and passes it to the worker.

### 4.4 Determinism and caching

The scene image is content-addressed by the prompt, so a richer prompt changes the cache key: a turn
that produces a new prompt generates a new image, which is correct (the image is turn-specific and
already keyed by turn). The location backdrop pipeline is untouched, so its cache is unaffected.

The context is deterministic given the turn, so re-running a turn (a rare repair) produces the same
prompt.

## 5. Behaviour

| Turn | Prompt contains |
| --- | --- |
| a scene break with a cue | the cue as the subject |
| a plain turn | a short narration excerpt as the subject |
| a failed check | the action, the cast, and "ominous, shadowed" |
| no present entities | no cast section |
| no outcome | no mood words beyond the world style |
| the old three-argument call | cue + location + appearance + style, as today |

## 6. Testing

- `pkg/media`/`pkg/engine`: `BuildScenePrompt` includes each part when present and omits it when
  absent; the caps are enforced; the outcome tone maps success/weak/miss; the old wrapper produces
  the previous prompt for the same inputs (a regression guard).
- `pkg/engine`: the enqueue site builds a context with the turn's narration, action, entities, and
  outcome.
- A determinism test: the same turn produces the same prompt twice.

## 7. Rollout

Additive. The old prompt builder signature is kept as a wrapper, so no caller breaks. Existing
cached scene images remain valid (they are keyed by turn and prompt).

## 8. Risks

- **Prompt bloat.** Too much prose confuses an image model and costs tokens. The caps are the guard;
  the subject is one sentence, not the paragraph.
- **Sensitive content.** Narration and actions can contain anything; the image provider's own policy
  applies. The prompt is not shown to the user, so this is unchanged from today.
- **Provider variance.** A provider may ignore the mood words; the prompt is best-effort, not a
  contract.
