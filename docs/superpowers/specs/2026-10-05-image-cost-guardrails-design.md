# Image Cost Guardrails Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#54 IMG-4](https://github.com/darkliquid/LocalRPG/issues/54)
**Epic:** [#20 Scene images from turn content](https://github.com/darkliquid/LocalRPG/issues/20)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §4 (IMG-4)
**Depends on:** [#52 IMG-2](https://github.com/darkliquid/LocalRPG/issues/52)
**Scope:** `pkg/engine`, `pkg/config`, `pkg/gui`, `frontend`

---

## 1. Problem

IMG-2 makes scene images fire far more often (the `significant` default), and IMG-3 adds a reference
image per generation. With a metered provider, that is real money, and nothing bounds it: a campaign
can generate an image on every significant turn, and a bug in the trigger or a long session can spend
without limit.

The usage ledger records what was spent after the fact (`pkg/pricing`). Images need a **budget**.

## 2. Goals

- A per-campaign **image budget**: a maximum number of images, or a maximum spend, or both.
- A **preview/approve** mode for a metered provider, so a user decides before each spend.
- Automatic **fallback to procedural art** when the budget is spent or a provider fails.
- The budget is visible: the user can see how many images remain.

## 3. Non-goals

- A global budget across all metered features; WG-6 does generation. This is images.
- The trigger policy (IMG-2); the budget is a second, independent limit.
- Pricing itself (the ledger exists).

## 4. Design

### 4.1 The budget

A per-campaign setting in `game.yaml`:

```yaml
settings:
  image_budget:
    max_images: 200        # 0 = unlimited
    max_micros: 5000000    # 0 = unlimited; USD 5 at 1e6 micros
  image_approval: auto     # auto | ask
```

```go
// ImageBudget bounds a campaign's image generation.
type ImageBudget struct {
	MaxImages int   `yaml:"max_images,omitempty"`
	MaxMicros int64 `yaml:"max_micros,omitempty"`
	Used      int   `yaml:"used_images,omitempty"`
	Spent     int64 `yaml:"spent_micros,omitempty"`
}
```

`Used` and `Spent` are updated as images are generated, so the budget survives a reload. The config's
global default is unlimited, so an unconfigured campaign is unchanged.

### 4.2 Enforcement

The scene worker checks the budget **before** calling the provider:

- if `MaxImages > 0 && Used >= MaxImages`, or `MaxMicros > 0 && Spent >= MaxMicros`, the generation is
  skipped and a `turn.image_budget_exhausted` event is traced;
- the beat falls back to procedural art (the existing `BuiltinFallback`), so the turn still has an
  image;
- on success, `Used` and `Spent` increment (the cost from the ledger when priced).

The check is in one place, so every path (IMG-2's auto trigger and IMG-3's reference generation)
respects it.

### 4.3 Preview and approve

When `image_approval: ask` and the provider is metered, the worker does **not** call the provider.
Instead it emits a `scene_image_pending` event with the proposed prompt and the estimated cost; the
client shows an approve/reject prompt. On approve, the generation proceeds and the budget is charged.

This is for a user who wants the feature but wants to see the bill per image. `auto` (the default)
generates without asking, bounded by the budget.

### 4.4 Fallback

The existing `fallbackImageClient` (`pkg/media/providers.go:111-144`) already covers a provider
failure with the procedural generator. IMG-4 extends the trigger: an exhausted budget also falls
back, so the beat is never imageless because of a limit. The fallback is free, so it is not charged.

### 4.5 Visibility

The campaign settings show the budget and the remaining allowance; the turn view shows a small
indicator when the budget is nearly spent or exhausted. A user who wants more raises the limit.

## 5. Behaviour

| State | Result |
| --- | --- |
| under budget, auto | generate and charge |
| at the image limit | skip the provider; procedural art; traced |
| at the spend limit | the same |
| approval `ask`, metered | a pending prompt; generate on approve |
| approval `ask`, local | generate without asking (no cost) |
| a provider failure | procedural art, as today |
| unlimited (default) | unchanged from IMG-2 |

## 6. Testing

- `pkg/engine`: the budget blocks a generation at the limit and falls back; `Used`/`Spent` increment
  and persist; an unpriced provider still counts images; `ask` emits a pending event and does not
  call the provider until approved.
- `pkg/config`: the budget round-trips in `game.yaml`.
- `pkg/gui`: the remaining allowance reaches the client; the approval flow generates on approve.
- A regression guard: an unlimited budget behaves exactly as IMG-2.

## 7. Rollout

Additive, and unlimited by default, so no existing campaign changes. A user opts into a budget.

## 8. Risks

- **A budget that blocks a wanted image.** It is a limit the user set; the indicator and the settings
  make it visible, and the procedural fallback means the beat still has an image.
- **Cost accuracy.** `Spent` uses the ledger's price; an unpriced provider records images only, so a
  `MaxMicros` budget on an unpriced provider does nothing. The UI should say so.
- **Approval fatigue.** `ask` on a metered provider can be tedious; `auto` plus a budget is the
  default, and `ask` is for a cautious user.
