# Character Appearance Split Design

**Date:** 2026-10-11
**Status:** Proposed
**Issue:** [#147](https://github.com/darkliquid/LocalRPG/issues/147)
**Scope:** `pkg/entity`, `pkg/harness`, `pkg/engine`, `pkg/media`, `pkg/gui`, `frontend`

---

## 1. Problem

`appearance` is one field doing two jobs. It is the description a world author
writes and character creation collects, and it is also the field gameplay
overwrites. The extractor asks the model for an entity's `appearance` and an
`appearance_changed` flag, which the prompt defines as an *existing* character or
location that "has noticeably altered in appearance or age (e.g. time skip, aging,
scars, transformation, haircuts)" (`pkg/harness/extractor.go:436`), and
`MergeExtractedEntity` copies the value straight over the note's `appearance`
(`pkg/harness/extractor.go:393`). The overwrite is one-way and cumulative: a scar
written at turn 20 stands at turn 200, a later haircut replaces the sentence that
described the scar, and each rewrite loses a little more of what the author wrote.

Nothing recovers the original. A campaign reset restores world *template* notes by
rewriting them from the world, so a template character does come back, but the
player is not a template: `resetEntityNotes` keeps the player's note and
`clearRuntimeFields` (`pkg/engine/restart.go:232`) clears only `history` and
`state`. A player whose appearance drifted keeps the drifted text across a
restart. The existing reset test asserts the appearance is kept, and it passes
only because the test never mutates it. An undo does not restore it either:
`RewindToTurn` prunes the entity's history numbers and leaves the description
alone.

Every consumer reads that one value, so nothing can ask for the character's
identity separately from the character as they are now:

| Consumer | Reads | Wants |
| --- | --- | --- |
| Scene cast (`sceneCastAppearance`, `pkg/engine/scene_worker.go:209`) | `appearance` | both, so the scene shows the wound as well as the face |
| Portrait (`BuildPortraitPrompt`, `pkg/engine/portrait_worker.go:19`) | `appearance` | both, so a regenerated portrait shows the change that prompted it |
| Location art (`AppearanceHash`, `pkg/media/image.go:145`) | `appearance`, else tags and state | both, and the cache has to follow whatever the image shows |
| Character sheet (`CharacterSheetDrawer.tsx:116`) | `appearance` | the identity, with the circumstance called out as temporary |

## 2. Goals

- An entity carries two appearances: the **baseline**, which is who the character
  is, and the **current** one, which is how they look in their present
  circumstances.
- Gameplay writes only the current one. Once an entity exists, the extractor never
  touches the baseline again.
- A reset clears the current one, so a restarted campaign's characters look as
  they were authored.
- Every consumer that composes a prompt or describes a character reads both
  through one shared rule, so the scene, the portrait, the location art, and the
  sheet cannot drift apart.
- An undo puts the current appearances back the way the discarded turns found
  them, so a rewind rewinds what play did to how a character looks as well as what
  it did to the story.
- Whether a place's own art follows the look play has given it is a setting, so a
  campaign that wants location art to stay the authored reference can have that.
- Existing notes, world templates, world packages, and the config file keep
  working. `appearance` keeps its name and becomes the baseline.

## 3. Non-goals

- **Renaming `appearance`.** Every note, template, guide, and type carries it, and
  the field is already the authored one.
- **Recovering a baseline that gameplay has already overwritten.** The original is
  gone from the note, and the world template is the only other copy. There is no
  repair pass and no report: section 4.7 states what that means for a campaign
  that is already running, and the answer is that the drift becomes the baseline
  until a person edits it.
- **Rolling the portrait back on an undo.** The portrait worker runs after the turn
  is recorded, so a turn cannot name the portrait it is about to produce. The
  portrait history a note already keeps is the way back, and section 8 explains
  the asymmetry.
- **Changing the turn database's shape.** The appearance records live in the
  history log, which is canonical and carries them without a schema migration.
- **Changing the API shape or the provider model.** The config gains one key.
- **Making the current appearance a first-class prompt-layer input** beyond the
  descriptions that already reach the model.

## 4. Design

### 4.1 Two fields, one concept

| Field | Holds | Written by |
| --- | --- | --- |
| `appearance` | The baseline: build, face, marks, dress, bearing. The character's identity. | A world author, character creation, the enricher, a person editing the note |
| `current_appearance` | How they look right now: a fresh scar, soaked clothes, a disguise, a decade of age. A layer on the baseline, never a replacement for it. | The extractor, a person editing the note |

A sibling scalar is chosen over nesting the two under one key:

```yaml
appearance: "A weathered sailor with a storm-grey coat and a white scar above one eye."
current_appearance: "Soaked through, and favouring his left arm."
```

A nested mapping (`appearance: {baseline: ..., current: ...}`) models the concept
more precisely, and is rejected because it changes the shape of a key that every
note, template, character sheet, world package, and guide already uses. It would
need a parser that accepts a scalar or a mapping, and it would break
`entityKeys("appearance")`, the frontend types, and `CharacterSheetDrawer` at
once. The additive scalar keeps one parsing path and one meaning per key.

**Entity side.** `EntityFrontmatter` and `Entity` gain `CurrentAppearance`;
`SerializeMarkdown` writes it; the `ExtraMeta` skip list
(`pkg/entity/entity.go:353`) gains `current_appearance` so a parsed note does not
also carry it as an unknown key; `pkg/storage` adds it to the metadata map it
writes and to the rehydration it does on the way back out
(`pkg/storage/store.go:54`, `:129`), without which an entity read from the index
would lose the field.

### 4.2 Write rules

The split is enforced where the writes happen, not by convention.

**The extractor** keeps one JSON field. The model still returns `appearance` and
`appearance_changed`, because deciding which of two fields to fill is the same
decision it already makes with the flag, and moving that decision into the prompt
would give it two chances to describe a character twice. The merge routes the
value (`MergeExtractedEntity`):

| Case | Baseline | Current |
| --- | --- | --- |
| New entity, no baseline yet | the model's `appearance` | untouched |
| Existing entity, `appearance_changed` | untouched | the model's `appearance` |
| Existing entity, nothing changed | untouched | untouched |

The prompt gains one sentence asking the model to describe only what is different
when it sets `appearance_changed`, so the current reads as a change rather than a
restatement of the whole character. Section 4.3 bounds the damage when it does
restate anyway.

**The current never replaces the baseline.** A reader of the note, a prompt, a
cache key, and a sheet always see the baseline, with the current added to it. The
one exception is an entity with no baseline at all, where the current stands alone
because there is nothing to layer it on.

**The enricher** (`pkg/engine/character_enricher.go:99`) fills the baseline when a
character has none. It exists to give an under-described character an identity, so
it writes `appearance`, and it never writes the current one.

**The GUI editors** may write either. A person correcting a baseline is
legitimate, and is the only way the player's drifted text is ever repaired.

### 4.3 Read rules: one composition function

`entity.DescribeAppearance(ent, limit)` becomes the single rule for turning the two
fields into the text a prompt, a cache key, or a sheet shows:

- Neither set: an empty string.
- Only the baseline: that, truncated to `limit`.
- Only the current, which happens when nothing authored a baseline: that, truncated
  to `limit`, because there is no context to layer it on.
- Both: `<baseline>; now <current>`, truncated to `limit`.

The composition is a layer, not a choice between two descriptions: the baseline
gives the context, and the current is what play has added to it. The order is
deliberate. The baseline comes first so the character reads as themselves, and the
current comes last, where a model weights it as the qualifier. The `; now`
separator says which is which without inventing a sentence.

A caller with a tighter budget than `limit` bounds each part before it composes,
because the rule can only truncate the result. The scene cast is the caller that
needs this: it keeps the 120 characters the baseline has today and gives the
current appearance 80 of its own, so a long circumstance cannot crowd out the
identity.

Consumers change from reading `ent.Appearance` to calling the rule:

| Consumer | Change |
| --- | --- |
| `sceneCastAppearance` | Bounds each part as above, then composes both |
| `BuildPortraitPrompt` | Composes both, so a portrait regenerated for a change shows the change |
| `BuildLocationPrompt` and `media.AppearanceHash` | Compose both, and hash the composed text, so location art invalidates when the place changes, unless section 4.6 pins the authored look |
| The character sheet DTO and drawer | Baseline as the description, the current shown as a temporary note when it is set |

Hashing the composed text is what preserves today's invalidation. Gameplay already
changes `appearance`, so the location art key already changes with it; hashing the
baseline alone would silently stop that. Section 4.6 turns that from a certainty
into a choice.

### 4.4 Reset, undo, and the timeline

`clearRuntimeFields` (`pkg/engine/restart.go:232`) clears `history`, `state`, and
`current_appearance`, and its comment gains the field. Template notes need no
change: `resetEntityNotes` rewrites them from the world, which restores both fields
at once, so a world author who wrote a character with a starting injury gets that
injury back.

This is what makes the reset's existing promise true for appearances. The story is
discarded, so the scars the story left are discarded with it. The scene images
are already cleared on the same principle (the illustrations belong to the
timeline), so a restart now clears every trace of what play did to how a character
looks.

**An undo restores what the discarded turns left behind.** A note on its own
cannot say what a character looked like three turns ago, so the turn that changed
an appearance records the value it found. `Turn` gains one advisory record, in the
shape the log already carries for `ContextNotes` and `ContinuityNotes`:

```go
// EntityRevert is what a turn rewrote on one entity, so an undo can put it back.
// Only a field play owns is recorded here; an authored field never appears.
type EntityRevert struct {
	EntityID         string `json:"id"`
	AppearanceBefore string `json:"appearance_before,omitempty"`
}
```

`stageEntities` (`pkg/engine/timeline.go:325`) is where an extraction is merged
into an existing note, so it is the one place that sees the value before the
write. It records a revert for each entity whose current appearance it is about to
change, and the record rides into `history.jsonl` with the turn.

`RewindToTurn` applies them. It already collects the entities the discarded turns
touched so it can prune their history numbers and write each note once, and the
same pass applies the reverts: it walks the discarded turns newest first, so an
entity changed by several of them ends up with the value from before the oldest
change, and each note is written once. `ReplaceTurn` is covered without further
work, because it rewinds to the turn before the draft and then records the
replacement under the same number.

A turn recorded before this change carries no revert, so undoing it restores
nothing. That is today's behaviour, and it needs no migration.

### 4.5 Editing and display

- `pkg/gui/schema.go` gains a description for `current_appearance`, and the
  `appearance` description says it is the baseline.
- `pkg/gui/entitycatalog.go` offers `current_appearance` for `character` and
  `location`, beside `appearance`, with a label that says it is how the note looks
  right now and that play writes it.
- `pkg/gui/text_generate.go` keeps generating the baseline fields it generates
  today; the current one is not generated, because it describes a moment rather
  than a person.
- The character sheet shows the baseline as the description and the current as a
  separate line, so a player can tell what is authored from what happened.
- `pkg/gui/docs/10-codex-yaml.md` documents the new key in the full schema, in the
  per-type list, and where it explains what a reset keeps.

### 4.6 Whether a place's own art follows the current look

A place's own picture is a reference picture, and a place can change: a tavern
burns, a forest is clear-swept, a room is ransacked. Section 4.3 gives location art
the composed appearance, so it redraws when the place changes. That is what
happens today, because the change lands in `appearance`. Some campaigns want the
reference pinned instead, so a scene that changes how a place looks does not spend
an image on the place's own picture.

`ImageConfig` gains an option beside the `trigger` policy it already carries:

```yaml
media:
  image:
    # What a place's own art is drawn from. original is the authored appearance
    # alone. original_and_current adds the current appearance as a layer on top
    # of it. Empty means original_and_current.
    location_art_appearance: original_and_current
```

| Value | The place's own art | Cache key |
| --- | --- | --- |
| `original_and_current` (the default, and what an empty value means) | The authored appearance with the current one layered on it, so the picture follows what play did to the place without losing what the place is | The layered text |
| `original` | The authored appearance alone, so the picture stays the reference a campaign can rely on | The authored appearance alone |

Neither value ever puts the current appearance in place of the authored one; the
option decides whether the layer is added, not which of the two is used.

The option selects one text for both the prompt and the key, so the picture and
the thing that decides whether it redraws can never disagree. A forced redraw
still works in either mode.

The turn scene illustration is not affected. A scene shows the place as it is in
that moment, whatever the option says; the option is about the place's own picture,
which is the one a campaign may want to keep stable.

`trigger` is not exposed in the UI today, and the new option is, because it is a
choice about cost rather than a policy about cadence: it goes in the image editor
under **Media Engines → Images**, beside **Auto-generate Scene Art**.

### 4.7 What happens to a campaign that is already drifting

Nothing is migrated. The consequence is stated rather than papered over:

- A **world template character** recovers its baseline on the next reset, because
  the template is still the world's copy.
- A **player or hand-authored character** keeps the drifted text, which becomes
  the baseline. A person can correct it in the editor, and from then on it stops
  drifting.
- A **new campaign** never has the problem: the baseline is written at creation,
  and the extractor only ever fills the current one.

Any automatic recovery would have to guess which sentences in a drifted
description are identity and which are circumstance. Guessing wrong writes the
guess into the one field that is supposed to be stable.

## 5. Behaviour

| Action | Before | After |
| --- | --- | --- |
| A character is scarred in play | `appearance` is replaced with the scarred description | `current_appearance` is set, `appearance` is untouched |
| The same character is scarred again, then disguised | Each rewrite replaces the last, losing the earlier detail | The current is replaced; the baseline still has the face |
| Reset a campaign | The player keeps the drifted `appearance` | The current is cleared, so the player looks authored again |
| Generate a scene the character is in | The prompt carries whatever the last writer left | The prompt carries the identity, then the present circumstance |
| Regenerate a portrait after a change | Shows the mutated text | Shows the identity with the change that prompted it |
| Generate location art after a place changes | Redraws, because the key moved with `appearance` | Redraws from the layered text, unless the option in 4.6 is `original` |
| Undo a turn that scarred a character | The scar stays | The current appearance returns to what the turn found |
| A place changes, with location art set to `original_and_current` | The place's picture redraws | The same, from the authored look with the change layered on |
| A place changes, with location art set to `original` | The place's picture redraws | The picture stays the authored reference |

## 6. Testing

- `pkg/entity`: a note round-trips `current_appearance` through parse and
  serialize; a note that carries it does not also carry it in `ExtraMeta`;
  `DescribeAppearance` covers neither, either, both, and truncation.
- `pkg/harness`: a new entity's description becomes the baseline; an
  `appearance_changed` extraction writes the current and leaves the baseline
  byte-identical; an unchanged extraction writes neither.
- `pkg/engine`: the reset clears the current appearance and keeps the baseline,
  extending `TestResetCampaignRestoresWorldCastAndKeepsConfiguration`; a template
  note returns to the template's own current appearance; the scene prompt carries
  both, in order, within the caps; the portrait prompt carries both.
- `pkg/engine`, the undo: a turn that changes a current appearance records the
  value it found; a rewind restores it; several discarded turns that changed one
  entity leave the value from before the oldest; the record survives a round trip
  through `history.jsonl`; `ReplaceTurn` restores the draft's appearance before it
  records the replacement.
- `pkg/config`: the location-art option resolves an empty value to
  `original_and_current`.
- `pkg/media`: `AppearanceHash` changes when either field changes, and does not
  change when neither does; with the option set to `original`, the key ignores the
  current appearance and the prompt carries only the authored one.
- `pkg/gui`: `ListEntities` and `GetEntity` return the new field; the sheet DTO
  carries it.
- `frontend`: the sheet shows the baseline and the current as separate lines, and
  shows only the baseline when there is no current.
- `go test ./pkg/gui -update-docs` is run if the config reference or catalogue
  output changes, and `mise run lint:docs` and `mise run lint:prose` gate the
  documentation.

## 7. Rollout

Each step is independently reviewable and leaves the tree green:

1. **The field and the rule.** `CurrentAppearance` on the entity, the serializer
   and skip list, the storage metadata, and `DescribeAppearance`, with its tests.
   Nothing writes the field yet, so behaviour is unchanged.
2. **The writes.** The extractor's merge rule, the enricher, and the one sentence
   of prompt guidance. From here the baseline stops moving.
3. **The reset.** `clearRuntimeFields` clears the current appearance, with the
   extended reset test.
4. **The undo.** `Turn.EntityReverts`, captured where the merge happens and applied
   in the rewind pass that already prunes history numbers. It needs the writes to
   have something to revert, so it follows them.
5. **The readers.** The scene cast, the portrait prompt, the location art and the
   hash, the sheet, and the location-art option with its resolver.
6. **The editing surface and the guides.** The schema description, the field
   catalogue, the sheet drawer, the image editor's new option, `10-codex-yaml.md`,
   and the regenerated configuration reference (`go test ./pkg/gui -update-docs`,
   because the config struct changed).

## 8. Risks

- **The extractor may restate the whole character into the current appearance.**
  The composed prompt would then say the same thing twice. Bounded by the prompt
  guidance and by the 80-character cap on the current appearance, and visible in
  the scene prompt test.
- **A current appearance that never clears** accumulates the same way the baseline
  did, only one field over. Today's behaviour replaces it on every change, which
  is what the design keeps: the current describes the present, so replacing it is
  correct. A campaign that wants the history of a character's appearance has the
  narration and the chronicle for it.
- **The portrait does not revert with the appearance.** A character scarred and
  then un-scarred keeps the portrait the scar produced, so the picture and the
  current appearance disagree until a portrait is regenerated. The worker runs
  after the turn is recorded, so a turn cannot name the portrait it will produce;
  making it do so means a second write to the note, or carrying the portrait onto
  the next turn's record. The portrait history the note already keeps is the
  documented way back.
- **The history log grows a little.** One revert record per changed appearance: an
  entity id and the text it replaced. The log is append-only and already carries
  per-turn advisory records of the same size.
- **Art churn.** Location art composes the appearance, so a scene that changes how
  a place looks redraws the place's picture. That is today's behaviour, and the
  location-art option is how a campaign opts out of it.
- **A revert record is only as good as the write it describes.** If an appearance
  is changed somewhere that does not go through the merge, the undo cannot put it
  back. Every write goes through `stageEntities` today, and a test covers the
  merge path.
- **Two fields can disagree.** A note can carry a current appearance that reads
  as a different person. Nothing enforces coherence, and the guides should say the
  current one is a qualifier on the baseline rather than a replacement.

## 9. Open questions

- **Should an undo revert the portrait as well?** It does not, and section 8
  explains what makes that awkward. The cheapest version is for the portrait worker
  to record the version it replaced where the rewind can read it, which would make
  the picture follow the appearance exactly.
- **Should the location-art option have a third value?** `original` and
  `original_and_current` answer the report. A third, "redraw only when the place
  changes materially", would need a notion of material that nothing has today, and
  `original` plus a forced redraw covers the same ground with less machinery.
- **Should the current appearance fade?** Nothing clears it as a campaign moves on,
  so a scar from a hundred turns ago is still described as present. A rule that
  ages it out would need to know how long a circumstance lasts, which the
  narration does not state.
