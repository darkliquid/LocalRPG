# Campaign Restart Scope Design

**Date:** 2026-10-05
**Status:** Approved
**Scope:** `pkg/gui` (restart, list, asset routes), `pkg/engine` (campaign
reset, world initial set), `pkg/storage` (derived-state reset), the campaign
settings modal copy
**Related:** `2026-09-20-localrpg-design.md`,
`2026-09-21-canonical-db-and-turn-timeline-design.md`,
`2026-09-25-campaign-selection-grid-view-design.md`
**Feature branch:** `fix/campaign-restart-scope`

---

## 1. Problem

`Service.RestartGame` (`pkg/gui/service.go:3832`) returns a campaign to its
opening state by **deleting the whole campaign directory** and recreating it
through `engine.InitGame`. It carries forward only the protagonist's name,
authored fields, portrait file, start location and opening prompt. Everything
else is lost:

- **Campaign artwork.** `assets/banner.*` and `assets/icon.*` are deleted, so a
  campaign that had an uploaded or generated banner and icon comes back bare.
- **The narrator voice.** `settings.narrator_voice` is a campaign setting, and it
  is not among the keys the recreate step writes back, so the narrator falls back
  to the global default.
- **Every other setting.** Any key the player pinned in the settings panel is
  dropped.
- **The spend ledger.** `usage_records` lives in `games/<id>/cache/index.db`,
  which is inside the deleted directory, so the campaign's cost history is gone.
- **Character and scene art.** NPC portraits, scene images and synthesized audio
  clips under `assets/` are deleted.

The user's requirement is the opposite: a restart should reset the **story** and
the **world's mutable state**, and nothing else.

> The image, icon and narrator voice shouldn't be reset. In fact _nothing_ about
> the campaign should be reset except the turn history and any entities
> added/modified after the initial set imported from the world being used.

Separately, a campaign that has never had its own artwork should borrow the
world's:

> If a campaign lacks an image/icon but is using a world that _does_ have an
> image/icon, then it should fall back to using the ones from the world.

## 2. Design

### 2.1 What a restart resets, and what it keeps

A restart returns the campaign to the state it had immediately after creation.
The set of things that change is small and explicit:

**Reset:**

1. The timeline: `history.jsonl`, and every index table derived from it
   (`turns`, `turn_entities`, `turn_contexts`, `memories`, `memory_entities`,
   `memory_tags`, `memories_fts`, `working_set`, and turn embeddings).
2. The world's mutable cast: any note whose id matches a template in the world's
   `entities/` is overwritten with that template, discarding everything play did
   to it (state, history, voice, portrait reference, prose edits).
3. Entities created during play: any note that is neither from the world, nor the
   protagonist, nor the opening scene is deleted.
4. The protagonist's and the opening scene's runtime fields: `history` and
   `state` are cleared; the authored content (name, appearance, age, gender,
   pronouns, voice, portrait, background, prose, extra frontmatter) is kept.
5. The campaign's batch synthesis jobs (`tts_jobs`). They exist to speak
   narration the reset has just discarded, so an in-flight job is cancelled at
   the provider first, best-effort, and every job row is then removed.

**Kept, untouched:**

- The whole `game.yaml` manifest, including every setting: `narrator_voice`,
  `start_location`, `opening_prompt`, mechanics engagement, and anything else.
- The whole `assets/` directory: banner, icon, NPC portraits, scene images, and
  audio clips.
- `usage_records`, the spend ledger, which records what was bought rather than
  what was said.
- The protagonist note file itself, apart from the runtime fields above.
- The opening-scene location note, which is the campaign's pinned start and is
  derived from the world manifest, not from play. It keeps its authored content
  and loses only its runtime fields.

The campaign directory is therefore **never deleted**. Restart becomes an
in-place reset.

### 2.2 Identifying "the initial set imported from the world"

The world is the source of truth for the starting cast. The initial set is:

- every note under `worlds/<world-id>/entities/`, keyed by its frontmatter `id`
  (falling back to the filename, matching `InitGame`), and
- the protagonist note, whose id is `manifest.Player` (or `ResolvePlayerID`), and
- the opening-scene location, whose id is the constant
  `engine.OpeningSceneEntityID`.

Restart walks `games/<id>/entities/` recursively (notes may sit in folders) and
classifies each note by id:

| Note | Action |
| --- | --- |
| id is the protagonist or the opening scene | Keep file; clear `history` and `state` |
| id matches a world template | Overwrite with the world template at `entities/<id>.md`; remove any nested copy |
| anything else | Delete the file |

Deriving the set from the world rather than snapshotting it at creation keeps
`games/<id>/` free of a second source of truth, and means a world author's
correction reaches a restarted campaign. The trade-off is documented in
section 4.

### 2.3 Clearing the derived index

The index is disposable and rebuildable, but it also holds the usage ledger,
which is not. Deleting `cache/index.db` would lose the ledger, so the reset is
surgical: a single `Store.ResetDerivedState` clears the timeline, memories, the
working set, the entity graph and embeddings in one transaction, and leaves
`usage_records` alone. The batch job table is cleared separately by
`Store.DeleteTTSJobs`, because its rows are removed after the provider has been
asked to cancel them, not as part of the derived-state transaction. The caller
then re-syncs the notes from disk, so entities, edges and embeddings are rebuilt
from the Markdown that survived.

### 2.4 World artwork fallback

A campaign that has no `assets/banner.*` or `assets/icon.*` of its own borrows
the world's, for reading only:

- `Service.GetGameAsset` (`pkg/gui/service.go:4630`) falls back to
  `Service.GetWorldAsset` for the campaign's world when the campaign has no such
  asset. The route is unchanged, so `/api/game/<id>/icon` transparently serves
  the world's icon.
- `Service.ListGames` and `Service.GetGameState` set `IconURL` and `BannerURL`
  when **either** the campaign or its world has the asset, so the launcher grid
  and the campaign modal render the borrowed art.
- Uploading or generating a campaign asset writes into the campaign directory as
  today, so a campaign asset always wins over the borrowed world asset.

The fallback never copies files: it is resolved at read time, so a world asset
changed later is reflected immediately and a campaign that never had its own art
still has none to reset.

### 2.5 Code shape

- `pkg/engine/restart.go` (new) owns the domain operation
  `ResetCampaign(paths, store, manifest)`: classify and rewrite notes, clear the
  timeline, re-sync the index. It is engine-level because it is a statement about
  what a campaign is, not about HTTP.
- `pkg/storage` gains `Store.ResetDerivedState` and `Store.DeleteTTSJobs`, the
  index-level primitives the engine composes.
- `pkg/gui/service.go` keeps `RestartGame` as a thin, lock-holding wrapper that
  loads the manifest, cancels any in-flight batch job, calls the engine, and
  returns the summary.
- `pkg/gui` gains a small helper for the world asset fallback so `GetGameAsset`,
  `ListGames` and `GetGameState` agree.

## 3. Testing

- Engine: a world template modified during play is restored; a play-created
  entity is deleted; the protagonist survives with authored fields intact and
  `history`/`state` cleared; the opening scene survives; the manifest and
  `assets/` are byte-identical after a reset; batch jobs are removed.
- Storage: `ResetDerivedState` empties the timeline and memories but leaves
  `usage_records` intact; `DeleteTTSJobs` removes one campaign's jobs and no
  other's.
- GUI: `RestartGame` preserves the banner, icon, narrator voice and usage ledger,
  removes the batch jobs, and reports `TurnCount` 0 with an empty chronicle (the
  existing `TestRestartGameClearsHistoryAndKeepsTheCampaign` and
  `TestRestartGamePreservesPlayerMetadata` must keep passing).
- GUI: a campaign with no icon of its own reports and serves the world's icon;
  a campaign with its own icon ignores the world's.

## 4. Trade-offs and non-goals

- **A world edit reaches a restarted campaign.** Because the initial set is
  derived from the world rather than snapshotted, restarting after a world
  author renames a template restores the new template and deletes a template the
  world no longer ships. This is the intended "reset to the world's current
  opening cast" behaviour and matches what the old delete-and-recreate did; it is
  recorded here so it is not mistaken for a bug.
- **Player-authored entities created before play are reset.** The requirement
  names "entities added/modified after the initial set imported from the world",
  and a note added through the Codex after creation is such an entity. It is
  deleted on restart. The protagonist and the opening scene are the two
  exceptions, and both are called out in section 2.1.
- **Embeddings are rebuilt lazily.** Clearing the entity graph drops entity
  embeddings; the embedding worker regenerates them on demand. This is the
  existing behaviour for any rebuilt index.
- **Batch cancellation is best-effort.** A job whose provider cannot be reached,
  or that belongs to a provider no longer selected, is not cancelled remotely;
  its row is still deleted, so the campaign forgets work for narration it no
  longer has. A provider that keeps such a job running may still bill for it,
  which is the one thing a restart cannot prevent.
- **Not a time machine.** Restart is not `/undo`; it does not preserve any turn.
  It is a full return to turn 0 with the campaign's configuration intact.
