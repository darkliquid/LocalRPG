# Draft World Review Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#82 WG-5](https://github.com/darkliquid/LocalRPG/issues/82)
**Epic:** [#25 AI world generation](https://github.com/darkliquid/LocalRPG/issues/25)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §8 (WG-5)
**Depends on:** [#78 WG-1](https://github.com/darkliquid/LocalRPG/issues/78)
**Scope:** `pkg/gui`, `pkg/worldgen`, `frontend`

---

## 1. Problem

WG-1, WG-2, WG-3, and WG-4 all produce a **draft** and none of them commits it. Nothing renders a
draft: the generation endpoints stream a `Draft` that the client has nowhere to put. Without a review
surface, generation either writes directly (which every one of those specs forbids) or the draft is
lost.

The review surface is also the **quality control** for every generation feature. A generated world is
only as good as the model, and the only thing that makes it usable is the ability to keep the good
parts and drop the rest.

## 2. Goals

- Render a draft world: its identity, lore, and entities.
- Accept or reject each entity and each lore section independently.
- Edit an entity or the lore before accepting it.
- Commit the accepted set to `worlds/<id>/` (or merge into an existing world) in one action.
- Persist a draft so a reload resumes review.
- Work for every producer: WG-1, WG-2, WG-3, WG-4.

## 3. Non-goals

- Generating anything (WG-1..4).
- Editing an already-committed world beyond the existing studio.
- A three-way merge; committing into an existing world is additive.

## 4. Design

### 4.1 The draft

`pkg/worldgen.Draft` is the shared type WG-1 produces. It gains a **selection state** for review,
held by the client and echoed on commit:

```go
// Draft is a generated world, before it is committed.
type Draft struct {
	ID       string
	World    core.WorldManifest
	Lore     string
	Sections []DraftSection   // lore split into accept/reject units
	Entities []DraftEntity
}

type DraftSection struct {
	Title string
	Body  string
}
```

Splitting the lore into sections (by heading) lets the user keep one section and drop another, which
is the same granularity the entities get.

### 4.2 Persistence

A draft is written to `worlds/.drafts/<id>.yaml` (a dot-directory the syncer skips) when it is
generated, so a reload resumes review. Committing or discarding deletes it. A draft never lives in
`worlds/<id>/`.

### 4.3 The review surface

A `WorldDraftReview` component (mounted by `WorldsStudio` and the launcher's new-world flow):

- **Header**: the proposed id, name, genre, art style — editable inline.
- **Lore**: each section as a card with accept/reject and an edit toggle; rejected sections are
  dimmed.
- **Entities**: a list, each a card with its type, name, a summary, its links, and accept/reject and
  edit. The entity editor is the existing content editor (SYS-5's sibling), so editing is not a new
  surface.
- **Counts**: "12 of 15 accepted" so the user knows what will be written.
- **Commit**: writes the accepted set; **Discard**: deletes the draft.

### 4.4 Committing

Commit resolves the accepted set:

- **New world**: create `worlds/<id>/` with `world.yaml`, `prompts/lore.md` (the accepted sections
  joined), and the accepted entities via `writeWorld`/`SaveWorldEntity`.
- **Existing world** (a WG-2/3 draft): append the accepted lore sections, write the accepted
  entities, and refuse an id clash as WG-2 does.

An empty accepted set disables Commit.

### 4.5 Where drafts come from

Every generation endpoint returns a draft id; the client fetches it (or uses the streamed draft) and
opens the review. The draft store is the single source, so a WG-1 world, a WG-2 batch, and a WG-4
ingestion all review the same way.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| a generated draft | the review opens with everything accepted by default |
| reject an entity | it is not written |
| edit an entity | the edit is written, not the original |
| reject all | Commit is disabled |
| commit a new world | `worlds/<id>/` is created |
| commit into an existing world | the accepted set is appended; a clash is refused |
| reload mid-review | the draft is restored |
| discard | the draft file is deleted |

## 6. Testing

- `pkg/gui`: a draft persists and reloads; commit writes only the accepted set; an id clash on an
  existing world is refused; discard deletes the draft; an empty set disables commit.
- `pkg/worldgen`: a draft round-trips through the store; the lore splits into sections by heading.
- `frontend`: the review renders sections and entities; accept/reject updates the counts; edit updates
  the item; commit sends the accepted set.
- A regression guard: committing a fully-accepted WG-1 draft reproduces the world WG-1 intended.

## 7. Rollout

Additive: a store, a component, and commit/discard endpoints. No generation feature writes a world
without it.

## 8. Risks

- **A draft left behind.** A user who abandons a review leaves `worlds/.drafts/<id>.yaml`. It is a dot
  file the syncer skips and a cleanup on open can prune a stale one.
- **Editing surface duplication.** The entity editor already exists; the review must reuse it, not
  grow a second one, or the two will drift.
- **Commit atomicity.** Writing a world is several files; a failure midway should not leave a
  half-world. Write to a temp directory and rename, as PKG-2 does.
