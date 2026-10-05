# Image Trigger Policy Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#52 IMG-2](https://github.com/darkliquid/LocalRPG/issues/52)
**Epic:** [#20 Scene images from turn content](https://github.com/darkliquid/LocalRPG/issues/20)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §4 (IMG-2)
**Depends on:** [#51 IMG-1](https://github.com/darkliquid/LocalRPG/issues/51)
**Scope:** `pkg/engine`, `pkg/config`, `pkg/gui`, `frontend`

---

## 1. Problem

A turn image is generated on exactly one condition: the narration contains a rule break (`---`) or
the extractor reported a scene break (`pkg/engine/orchestrator.go:1295-1301,1462-1473`). That is a
single, model-dependent trigger. In practice most turns produce no image, and the ones that do are
the ones where the model happened to write a horizontal rule.

With IMG-1 making the prompt genuinely turn-aware, the value of an image is no longer tied to a rule
break, and the cost of one is no longer negligible (a metered provider charges per image). Neither
side of that trade is controllable: you cannot ask for more images, and you cannot turn the feature
off.

## 2. Goals

- A configurable **policy** deciding when a turn image is generated.
- Five levels covering the useful range: never, on a scene break (today), on a significant beat
  (the new default), every turn, and only on request.
- A **significance heuristic** that is deterministic and testable, so "significant" is not a mystery.
- The policy is per-campaign (a setting) with a global default.

## 3. Non-goals

- The prompt (IMG-1), consistency (IMG-3), budgets (IMG-4), export (IMG-5).
- Generating the location backdrop, which is a separate, cached pipeline.

## 4. Design

### 4.1 The policy values

```go
// ImageTrigger selects when a turn scene image is generated.
type ImageTrigger string

const (
	ImageOff        ImageTrigger = "off"        // never
	ImageSceneBreak ImageTrigger = "scene_break" // a rule break or extractor scene break (today)
	ImageSignificant ImageTrigger = "significant" // a significant beat (default)
	ImageEveryTurn  ImageTrigger = "every_turn"
	ImageManual     ImageTrigger = "manual"     // only on an explicit request
)
```

Resolution order: campaign setting (`game.yaml` `settings.image_trigger`) → global config
(`media.image.trigger`) → `significant`.

### 4.2 The significance heuristic

A pure function, so it is testable and its reasons are traceable:

```go
// ShouldIllustrate reports whether a turn is significant enough to illustrate.
func ShouldIllustrate(turn Turn, cfg TriggerConfig) (bool, string)
```

It returns true when any of:

- the turn has a scene break (`turn.SceneBreak`), which is today's trigger and always significant;
- the turn resolved a check with a decisive outcome (a fail or a strong/success), because the
  fiction turned on it;
- the player's location changed (`turn.Location` differs from the previous turn);
- a character entered or left the scene (a new speaker in the turn's segments that was not in the
  previous turn's);
- the narration is longer than a threshold (a beat with substance rather than a one-liner).

The returned reason string is logged with the image job, so a trace explains why an image was made.

`cfg` carries the thresholds (decisive-outcome set, narration-length threshold) with sensible
defaults, so a campaign can tune them later without a schema change.

### 4.3 The gate

The orchestrator's enqueue site (`pkg/engine/orchestrator.go:1462-1473`) becomes:

```go
if o.sceneWorker != nil && o.imageTrigger != ImageOff {
	if ok, reason := o.shouldIllustrate(turn); ok {
		// enqueue with reason
	}
}
```

`scene_break` short-circuits to the existing check; `every_turn` always enqueues; `manual` never
auto-enqueues.

### 4.4 Manual requests

`manual` (and any policy) still allows an explicit request: a `POST /api/game/{id}/turn/{n}/scene-image`
that generates on demand. The turn view offers a "Generate image" action when a turn has no image,
so `manual` is usable and the other policies are not a trap.

### 4.5 Cost awareness

The heuristic is the primary cost control (it does not fire every turn), and IMG-4 adds a budget on
top. The policy itself is not a budget; `every_turn` with a metered provider is the user's explicit
choice.

## 5. Behaviour

| Policy | Turn | Image |
| --- | --- | --- |
| off | any | none |
| scene_break | a rule break | generated |
| scene_break | a plain turn | none |
| significant (default) | a failed check | generated |
| significant | a quiet one-liner | none |
| every_turn | any | generated |
| manual | any | none, until requested |
| any | an explicit request | generated |

## 6. Testing

- `pkg/engine`: `ShouldIllustrate` returns true for each significant condition and false for a quiet
  turn, with the expected reason; `scene_break` and `every_turn` and `off` behave per the table.
- `pkg/config`: the trigger resolves from campaign → global → default.
- `pkg/gui`: the manual request generates an image for a turn without one.
- A regression guard: with `scene_break`, behaviour is identical to today.

## 7. Rollout

Additive. The default changes from "scene break only" to "significant", which produces more images
than before. That is the intended improvement, but it should be called out in the release notes; a
user who wants the old behaviour sets `scene_break`.

## 8. Risks

- **More images, more cost.** The default is more generous than today. The heuristic limits it, the
  release note warns, and IMG-4 adds a budget. Consider defaulting to `scene_break` for a metered
  provider if IMG-4 has not landed.
- **Heuristic disagreement.** "Significant" is a judgement. The reasons are logged and the thresholds
  are configurable, so a user can see and adjust it.
- **Entity-entrance detection.** Comparing speakers across turns needs the previous turn's segments;
  a cheap index query suffices, and a miss only makes the heuristic less sensitive, never wrong.
