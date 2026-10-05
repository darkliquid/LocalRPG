# Batch Entity Generation Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#79 WG-2](https://github.com/darkliquid/LocalRPG/issues/79)
**Epic:** [#25 AI world generation](https://github.com/darkliquid/LocalRPG/issues/25)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §8 (WG-2)
**Depends on:** [#78 WG-1](https://github.com/darkliquid/LocalRPG/issues/78)
**Scope:** `pkg/worldgen`, `pkg/gui`, `frontend`

---

## 1. Problem

WG-1 generates a whole world from nothing. The common case is different: a world already exists, with
lore and a handful of entities, and the user wants to **fill it out** — "add three more factions in
the south", "give Saltmarch some locals", "add a rival crew".

The studio's entity creation is one entity at a time, hand-written or field-generated, and nothing
seeds a new entity with the world's existing lore and links it to the rest.

## 2. Goals

- Generate a **batch** of entities for an existing world from a short instruction.
- Seed the generation with the world's lore and its existing entities, so the new ones fit.
- Resolve `[[wikilinks]]` between the new entities and the existing ones.
- Preview the batch before it is written; the user accepts or discards.
- Reuse WG-1's generator, entity format, and link validation.

## 3. Non-goals

- Whole-world generation (WG-1) and enhancing lore (WG-3).
- Editing an existing entity; the batch is additive.
- Ingestion (WG-4).

## 4. Design

### 4.1 The request

```
POST /api/world/{id}/generate-entities
{ "instruction": "three rival factions in the south",
  "kinds": ["faction"], "count": 3, "focus": "Saltmarch" }
```

`focus` optionally anchors the batch to an existing entity or location.

### 4.2 The pipeline

`pkg/worldgen` gains a `GenerateEntities` that reuses WG-1's steps in a smaller form:

1. **Context** — assemble the world's manifest, lore, and the existing entities' ids, names, types,
   and one-line descriptions into the prompt (bounded, so a large world does not overflow).
2. **Generate** — one structured call producing the batch, seeded with the context and the
   instruction.
3. **Link** — resolve every wikilink against the union of the batch and the existing entities
   (WG-1's `linkDraft`, extended with the existing set), dropping an unresolved link.

```go
// EntityRequest asks for a batch of entities in an existing world.
type EntityRequest struct {
	WorldID     string
	Instruction string
	Kinds       []string
	Count       int
	Focus       string
}

// GenerateEntities produces a batch seeded with the world's lore and entities.
func GenerateEntities(ctx context.Context, gen Generator, world WorldContext, req EntityRequest) ([]DraftEntity, error)
```

`WorldContext` carries the manifest, the lore, and the existing entities (id, name, type, summary).

### 4.3 Preview and accept

The batch is returned as a **preview** (the draft entities, with their resolved links and any
dropped-link notes). Nothing is written. An accept call writes them into `worlds/<id>/entities/`
using the existing writer (`SaveWorldEntity`), which renames each note to its frontmatter id and
indexes it.

This mirrors WG-1's "never write without review" and reuses WG-5's review surface if it exists;
otherwise a lightweight preview list.

### 4.4 Consistency with existing entities

The link step validates against existing entities, so a new faction can link to an old location. It
does **not** modify existing entities (no back-links); a follow-up could add them, but that is a
different, riskier operation and is out of scope.

### 4.5 Bounds

- `count` is capped (for example 10) so one request cannot generate an unbounded batch.
- The context is bounded (the lore truncated, the entity list capped) so a large world still fits a
  prompt.
- Each generated entity is bounded in length like any entity note.

## 5. Behaviour

| Input | Result |
| --- | --- |
| "three factions in the south" | three faction entities, linked to existing places |
| a link to a missing entity | dropped, noted in the preview |
| a count over the cap | clamped to the cap |
| a world with no lore | generated from the manifest alone |
| discard | nothing written |
| accept | the entities are written and indexed |

## 6. Testing

- `pkg/worldgen`: a stub generator produces a batch of the requested kind and count; links resolve
  against existing entities; an unresolved link is dropped and noted; the context includes the
  existing entities.
- `pkg/gui`: the preview writes nothing; accept writes and indexes; the count is clamped.
- `frontend`: the preview lists the entities with their links; accept and discard work.
- A regression guard: an accepted entity matches what `SaveWorldEntity` would write by hand.

## 7. Rollout

Additive: new endpoints and a studio action on a world. No existing entity is touched until accept.

## 8. Risks

- **Tonal mismatch.** New entities may not match the world's voice. Seeding with the lore and existing
  entities mitigates it; the preview is the control.
- **Duplicate names.** A generated entity may duplicate an existing one. The accept path checks ids
  (`entity.Slugify`) and refuses or renames a clash.
- **Link explosion.** A batch that links to everything is noise. The prompt asks for a few meaningful
  links; the preview shows them.
