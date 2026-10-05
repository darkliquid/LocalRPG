# World Enhancement Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#80 WG-3](https://github.com/darkliquid/LocalRPG/issues/80)
**Epic:** [#25 AI world generation](https://github.com/darkliquid/LocalRPG/issues/25)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §8 (WG-3)
**Depends on:** [#78 WG-1](https://github.com/darkliquid/LocalRPG/issues/78), [#79 WG-2](https://github.com/darkliquid/LocalRPG/issues/79)
**Scope:** `pkg/worldgen`, `pkg/gui`, `frontend`

---

## 1. Problem

WG-1 creates a world and WG-2 adds a batch of entities, but neither touches what the world already
says. A world's `prompts/lore.md` is hand-written and static: nothing proposes a new section, a
deeper history, or a plot hook that follows from what is there. The user must write every expansion
themselves, and the model that could read the whole world and suggest the next thing is never asked.

## 2. Goals

- Given an existing world, propose **enhancements**: lore additions, new entities, and story hooks.
- Present them as a **diff**: each proposal is accepted or rejected individually.
- Seeded with the whole world (manifest, lore, entities), so proposals fit.
- Never modify an existing entity without an explicit accept.
- Reuse WG-1's generator, WG-2's entity writing, and the review surface.

## 3. Non-goals

- Rewriting existing lore (only additions are proposed; a rewrite is a manual edit).
- Whole-world generation (WG-1) and batch entities (WG-2).
- Ingestion (WG-4).

## 4. Design

### 4.1 The proposals

```go
// Enhancement is one proposed change to a world.
type Enhancement struct {
	Kind    string // "lore" | "entity" | "hook"
	Title   string
	Body    string // Markdown for lore/hook, a note body for an entity
	Entity  *DraftEntity
	Target  string // for lore: the section to append under; empty = a new section
	Reason  string // why the model proposes it, shown in the diff
}

// Enhance reads a world and proposes enhancements.
func Enhance(ctx context.Context, gen Generator, world WorldContext, instruction string, kinds []string) ([]Enhancement, error)
```

- **lore**: a new or appended Markdown section for `prompts/lore.md`.
- **entity**: a new entity, written like WG-2's batch (links resolved against the world).
- **hook**: a story seed (a short paragraph) added to a `## Hooks` section of the lore.

### 4.2 The diff

The proposals are returned as a list; the studio renders each as a card with its title, body, reason,
and accept/reject toggles. Applying:

- **lore**: append the accepted sections to `prompts/lore.md` (under the named heading, or at the
  end).
- **entity**: write via `SaveWorldEntity`, like WG-2.
- **hook**: append to the `## Hooks` section.

Rejected proposals are discarded. Applying is a single operation over the accepted set, so a partial
application is not left behind.

### 4.3 Why a diff, not a rewrite

A rewrite of `lore.md` would replace the user's prose with the model's. A diff keeps the user in
control and makes the change reviewable, which is the same reason WG-1 and WG-2 never write without
review. The lore file is the user's voice; the model adds to it, never overwrites it.

### 4.4 Bounds

- The number of proposals is capped (for example 12).
- The world context is bounded as in WG-2.
- Each proposal's body is bounded.

### 4.5 Surfaces

- **API**: `POST /api/world/{id}/enhance` (proposals) and `POST /api/world/{id}/enhance/apply`.
- **Studio**: an "Enhance" action on a world that shows the diff.

## 5. Behaviour

| Input | Result |
| --- | --- |
| "deepen the history" | lore proposals, accepted or rejected individually |
| "add a rival for the captain" | an entity proposal, linked to the captain |
| accept some, reject others | only the accepted changes are written |
| reject all | nothing changes |
| a proposal linking to a missing entity | the link is dropped, as in WG-2 |

## 6. Testing

- `pkg/worldgen`: a stub generator produces the three kinds; the context includes the lore and
  entities; a proposal's links resolve; the cap applies.
- `pkg/gui`: enhance writes nothing; apply writes only the accepted proposals; a lore proposal
  appends rather than replaces; a hook lands under `## Hooks`.
- `frontend`: the diff renders each proposal with accept/reject; applying sends only the accepted
  ones.
- A regression guard: applying no proposals leaves every file unchanged.

## 7. Rollout

Additive: new endpoints and a studio action. No existing content changes without an accept.

## 8. Risks

- **Unwanted lore.** A diff with per-item control is the mitigation; the reason line helps the user
  judge.
- **Lore growth.** Repeated enhancement grows `lore.md` without bound. A future pass could suggest
  consolidation; for now the user owns the file and can prune.
- **Hook quality.** Hooks are the softest output. They are cheap and easily rejected; a low-quality
  hook costs a glance.
