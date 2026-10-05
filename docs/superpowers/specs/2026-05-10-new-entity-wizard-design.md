# New Entity Wizard Design

**Date:** 2026-05-10
**Status:** Approved
**Scope:** a backend entity-type catalogue, a generated frontmatter scaffold, a
shared wizard component, and the three authoring entry points that use it
**Related:** `2026-10-04-content-editor-core-design.md` (the editor the wizard
hands off to), `2026-10-04-frontmatter-and-wikilink-intelligence-design.md`
(the completion and key descriptions the wizard reuses)

---

## 1. Problem

Every surface that creates a note still asks the user to write YAML. The codex
has no create affordance at all; Content Studio has none; Worlds Studio offers a
single "Entity Slug" text box (`WorldsStudio.tsx:1147`) and then drops a fixed
`STARTER_ENTITY_TEMPLATE` into the editor. The only creation path that fills in
a name and type is the continuity-repair modal (`AddEntityModal.tsx`), and it
writes a three-key template (`id`, `name`, `type`) with an empty body
(`App.tsx:668`).

The autosuggester closes the *syntax* gap: once a user is inside the frontmatter
block it completes keys and known values. It does not close the *discovery* gap,
which is knowing which keys exist, which of them apply to the kind of thing being
described, and what each one is for. A user who does not already know that a
`character` can carry `voice`, `appearance`, `gender` and `age` will never think
to type them.

## 2. Goals and non-goals

**Goals**

- One dialog that scaffolds a valid, type-aware note and hands it to the existing
  editor, so the user edits rather than authors from a blank page.
- Every accepted key for the chosen type present in the generated frontmatter,
  each preceded by a one-line description of what it is for.
- One canonical list of entity types and their fields, so the three lists that
  disagree today stop disagreeing.
- A type-specific option step, with voice selection for `character` as the first
  instance.

**Non-goals**

- No change to the loader, the frontmatter schema, the save path, or the
  `EntityFrontmatter` struct. The wizard only emits a string.
- No new persistence. The editor and `PUT .../entity/{id}` remain the only write
  path, and world templates keep using `PUT .../world/{id}/entity/{id}`.
- No change to `IsCharacterType` or any other engine behaviour keyed on `type`.
  The engine stays schema-agnostic and still accepts any type string.

## 3. The canonical type catalogue

### 3.1 Where it lives

A new file, `pkg/gui/entitycatalog.go`, holds the catalogue. It is the single
source of truth for three things that are currently scattered:

| Concern | Today | After |
| --- | --- | --- |
| Type suggestions | `schema.go:167` (7), `AddEntityModal.tsx:60` (6), `10-codex-yaml.md:19` (5) | the catalogue |
| Per-type field set | nowhere | the catalogue |
| Key descriptions | `schema.go:144` | unchanged; the catalogue references them |

The catalogue reuses `frontmatterKeyDescriptions` (`pkg/gui/schema.go:144`) so a
wizard comment and an editor completion cannot describe the same key
differently. It also reuses the existing `FrontmatterKeySchema` struct
(`schema.go:107`), so a key carries the same `name`, `type`, `required`,
`values` and `description` whether it arrives from the frontmatter schema or the
type catalogue.

### 3.2 Catalogue contents

Offered types, in display order. `npc`, `person` and `creature` remain accepted
by `entity.IsCharacterType` (`pkg/entity/entity.go:321`) but are aliases of
`character` rather than separate choices, so the wizard does not present four
buttons that behave identically.

| Type | Label | Field keys (ordered) |
| --- | --- | --- |
| `character` | Character | id, name, type, appearance, gender, age, voice, tags, aliases, location, faction, state |
| `location` | Location | id, name, type, appearance, tags, aliases, location, faction, state |
| `faction` | Faction | id, name, type, appearance, tags, aliases, location, faction, state |
| `item` | Item | id, name, type, appearance, tags, aliases, location, faction, state |
| `concept` | Concept | id, name, type, tags, aliases, location, faction, state |
| `arc` | Narrative Arc | id, name, type, tags, aliases, location, faction, state |
| `event` | Event | id, name, type, tags, aliases, location, faction, state |
| `quest` | Quest | id, name, type, tags, aliases, location, faction, state |
| `lore` | Lore | id, name, type, tags, aliases, location, faction, state |

`id`, `name` and `type` are required and always filled. The rest are emitted
empty and documented. `location` and `faction` are in the base set because both
are graph edges the assembler reads for any note, not just for a place or a
group. `appearance` is offered for every type that can be depicted or referenced
visually: `character`, `location`, `faction` and `item`.

Each type also carries a `label`, a one-line `description` for the picker, and
`aliases`. The set of ids is what `frontmatterKeyValues["type"]` is derived from,
so the completion list, the docs and the wizard all read one list.

### 3.3 API

New route `GET /api/schema/entity-types`, served by the existing
`handleSchemaRoutes` (`pkg/gui/server.go:161`) under the already-mounted
`/api/schema/` prefix (`pkg/gui/routes.go:49`). No new mount is needed.

```json
{
  "types": [
    {
      "id": "character",
      "label": "Character",
      "description": "Player characters, NPCs, companions and adversaries.",
      "aliases": ["npc", "person", "creature"],
      "keys": [
        {
          "name": "id",
          "type": "string",
          "required": true,
          "description": "The note's stable identity..."
        }
      ]
    }
  ]
}
```

Each entry of `keys` is a `FrontmatterKeySchema`. The response is generated at
request time from the catalogue, the same way `renderEntityFrontmatterSchema`
generates the frontmatter schema, so there is no second checked-in artefact to
drift.

## 4. Scaffold generation

### 4.1 Format

Live keys, each preceded by its description as a YAML comment. Required keys hold
real values; optional keys are present but empty, so saving a freshly created
note writes no fabricated data. `tags` and `aliases` are empty lists, `state` is
an empty map, and the remaining scalars are empty strings.

A `character` named "Lady Evelyn Vance", with a voice chosen from the provider
catalog, generates:

```markdown
---
# The note's stable identity. It is unique per collection and does not change when the note moves between folders.
id: lady-evelyn
# The display name shown in the codex and used when the engine matches a mention.
name: Lady Evelyn Vance
# What kind of thing the note is. Types that read as a being (character, npc, person, creature) are the only ones the engine treats specially.
type: character
# A physical description used when generating art or describing the note.
appearance: ""
# The note's gender, as free text.
gender: ""
# The note's age, as free text, because a campaign may count in years, seasons or reigns.
age: ""
# The text-to-speech voice this note speaks with, including pitch and speech rate.
voice:
  provider: elevenlabs
  voice_id: "21m00Tcm4TlvDq8ikWAM"
  pitch: 1
  speech_rate: 1
# Free-form labels for grouping and search.
tags: []
# Other names the same thing is known by. They are matched when resolving mentions and when completing wikilinks.
aliases: []
# Where the note is. May contain a [[wikilink]] to a location note.
location: ""
# The group the note belongs to. May contain a [[wikilink]] to a faction note.
faction: ""
# Arbitrary per-note state the mechanics hooks read and write. The engine does not interpret the keys.
state: {}
---

# Lady Evelyn Vance
```

A `location` named "The Ashen Bastion" generates the same shape with the location
key set, and no `voice`, `gender` or `age`:

```markdown
---
# ...id, name, type, appearance as above...
tags: []
aliases: []
location: ""
faction: ""
state: {}
---

# The Ashen Bastion
```

### 4.2 The voice block

The block is emitted from the catalogue's `voice` key plus a chosen
`VoiceSelection`. When a voice is chosen, `provider` and `voice_id` carry the
provider key and voice id from the inspect response, `pitch` and `speech_rate`
default to `1`, and `options` is emitted only when the provider supplied
defaults. When no voice is chosen, or no catalog is available, the block is
emitted with empty `provider` and `voice_id`, `pitch: 1` and `speech_rate: 1`,
and the same description comment, so the keys are still discoverable.

### 4.3 The generator

`frontend/src/lib/entityScaffold.ts` exports one pure function:

```ts
export interface VoiceSelection {
  provider: string;
  voice_id: string;
  pitch?: number;
  speech_rate?: number;
  options?: Record<string, unknown>;
}

export interface ScaffoldInput {
  id: string;
  name: string;
  type: string;
  voice?: VoiceSelection;
}

export function buildEntityMarkdown(
  catalog: EntityTypeCatalog,
  input: ScaffoldInput,
): string;
```

It iterates the chosen type's ordered `keys`, emits the description comment and
the key's line, then closes the frontmatter and writes `# <name>` as the first
body line. It is pure: no React, no fetch, no clock. A type id that is not in the
catalogue falls back to the base key set so an unknown type still produces a
valid note.

## 5. The wizard

### 5.1 Component

`frontend/src/components/NewEntityWizard.tsx`, a dialog in the same visual
language as `AddEntityModal.tsx` and using the same `useMountTransition` hook.

```ts
interface NewEntityWizardProps {
  isOpen: boolean;
  /** Seeds the name field, e.g. a narrator mention or the world studio's slug. */
  initialName?: string;
  /** Existing ids in the target collection, for collision checking. */
  existingIds: string[];
  /** The active TTS config, so the voice step can inspect the provider. */
  ttsConfig?: TTSConfig;
  /** Renders the confirm button, e.g. "Create note" or "Create template". */
  confirmLabel?: string;
  onConfirm: (input: { id: string; name: string; markdown: string }) => void | Promise<void>;
  onOpenExisting?: (id: string) => void;
  onClose: () => void;
}
```

The wizard does not save. It builds the markdown and calls `onConfirm`; the host
decides which endpoint to hit. That is what lets the codex, Content Studio and
Worlds Studio share one component while writing to three different places.

### 5.2 Steps

1. **Name.** A text field seeded from `initialName`. The id is derived live with
   `slugify` (`frontend/src/lib/slug.ts`, which already mirrors `entity.Slugify`)
   and shown in an editable field below the name. An empty or invalid id, or one
   that appears in `existingIds`, disables confirm and shows an inline message;
   a collision offers `onOpenExisting(id)`.
2. **Type.** A grid of the catalogue's types, each with its label and one-line
   description.
3. **Options.** Type-specific inputs. The only one today is a voice picker for
   `character`: a searchable list driven by the live provider catalog
   (`useTTSInspect` plus the search and category UI patterns already in
   `VoiceCatalogPicker.tsx`). Choosing a voice is optional. The step also shows a
   live preview of the markdown that confirm will produce.
4. **Confirm.** Calls `onConfirm` with the id, name and generated markdown.

### 5.3 TTS configuration

The codex already receives `ttsConfig` from App (`App.tsx:1047`). Content Studio
and Worlds Studio do not, so the wizard accepts an optional `ttsConfig` and,
when it is absent, reads `config.media.tts` from `GET /api/settings` once when
the voice step is first shown. If the provider is disabled, unreachable, or its
catalog is unavailable, the step shows the provider error inline and the wizard
proceeds with an empty documented `voice:` block.

### 5.4 Entry points

- **Codex** (`CodexDrawer.tsx`). A "New Note" button in the sidebar header, next
  to the Notes/Memories tabs, and in the "Choose a note from the codex" empty
  state. On confirm it calls `client.saveEntity(id, markdown)` and selects the new
  note.
- **Content Studio** (`ContentStudio.tsx`). A "+ New" button above the tree. On
  confirm it saves and loads the note into the editor, reusing the existing
  `save` path so the entity index is invalidated and refreshed exactly as a normal
  save does.
- **Worlds Studio** (`WorldsStudio.tsx`). The slug-only modal at line 1147 is
  replaced by the wizard. A draft world keeps the in-memory draft path
  (`setEntityDrafts`), and a saved world calls `APIClient.saveWorldEntity`. The
  world studio's underscore slug rule is replaced by the shared kebab `slugify`,
  so a world template id matches what `entity.Slugify` would compute.
- **Continuity repair** (`AddEntityModal.tsx`). The modal keeps its "Quick
  Register" path (a three-key note is the right answer when a narrator invented a
  name mid-turn) but its "Edit in Codex" path opens the wizard instead of writing
  a three-key note, so a user who wants to do it properly is given the form.

## 6. Error handling

| Case | Behaviour |
| --- | --- |
| Empty or invalid id | Confirm disabled; inline message. |
| Id already exists | Confirm disabled; inline message with an "Open existing note" action. |
| Catalog unavailable | Voice step shows the provider error; creation proceeds with an empty `voice:` block. |
| Save rejected (422) | Error shown, dialog stays open, no input lost. |
| Unknown type id | Generator falls back to the base key set. |

## 7. Testing

**Go** — `pkg/gui/entitycatalog_test.go`:

- Every key named by any type exists in `frontmatterKeyDescriptions`, so a
  catalogue entry cannot reference an undocumented key.
- Type ids are unique, non-empty, and carry a non-empty label and description.
- `character` offers `voice`, and `location` and `item` offer `appearance`.
- `frontmatterKeyValues["type"]` equals the catalogue ids in order, so the
  completion list cannot drift from the wizard.
- `EntityTypeCatalog()` output round-trips through `json.Marshal`.

Regenerate the checked-in schema after changing the catalogue:
`go test ./pkg/gui -update-docs`.

**Frontend** — `frontend/scripts/checkEntityScaffold.mjs`, a node check in the
style of `checkTreeModel.mjs`, added to `test:frontend` in `mise.toml` and to a
`check:entity-scaffold` script in `frontend/package.json`:

- Required keys are always emitted and filled.
- Every emitted key is immediately preceded by a comment line.
- A `character` with a voice emits a `voice:` block with the chosen values; a
  `character` without one emits the empty documented block.
- A `location` emits `appearance` but not `voice`, `gender` or `age`.
- An unknown type emits the base key set only.
- The generated markdown parses with the `yaml` package (its frontmatter block),
  proving the scaffold is loadable.

`tsc --noEmit` remains the gate for the component and client changes.

## 8. Documentation

`pkg/gui/docs/10-codex-yaml.md` is the reader-facing reference for the
frontmatter. Its "Supported Entity Types" list is updated to match the catalogue,
and a short "Creating a note" section describes the wizard and the fact that
every key it emits is documented inline. The page is already inside Vale's
21-file scope and is rendered into the showcase site by `tools/sitegen`, so
neither changes.

## 9. File map

| File | Change |
| --- | --- |
| `pkg/gui/entitycatalog.go` | New: the catalogue, `EntityTypeCatalog()`, and the DTO |
| `pkg/gui/entitycatalog_test.go` | New: catalogue invariants |
| `pkg/gui/schema.go` | Derive `frontmatterKeyValues["type"]` from the catalogue |
| `pkg/gui/server.go` | Serve `/api/schema/entity-types` from `handleSchemaRoutes` |
| `pkg/gui/schema/entity-frontmatter.json` | Regenerated by `-update-docs` |
| `frontend/src/types.ts` | `EntityTypeSpec`, `EntityTypeCatalog` |
| `frontend/src/api/client.ts` | `getEntityTypes()` |
| `frontend/src/lib/entityScaffold.ts` | New: `buildEntityMarkdown` and its input types |
| `frontend/src/components/NewEntityWizard.tsx` | New: the dialog |
| `frontend/src/components/CodexDrawer.tsx` | "New Note" entry point |
| `frontend/src/components/ContentStudio.tsx` | "+ New" entry point |
| `frontend/src/components/WorldsStudio.tsx` | Replace the slug modal with the wizard |
| `frontend/src/components/AddEntityModal.tsx` | "Edit in Codex" opens the wizard |
| `frontend/scripts/checkEntityScaffold.mjs` | New: pure-function check |
| `frontend/package.json` | `check:entity-scaffold` script |
| `mise.toml` | Add the check to `test:frontend` |
| `pkg/gui/docs/10-codex-yaml.md` | Types list and a "Creating a note" section |

## 10. Out of scope

- Editing an existing note's type through the wizard.
- Generating an `appearance` value with a model. The key is scaffolded empty; the
  existing art-generation paths still own producing it.
- Adding `inventory` (used in the docs example but not a struct field) as a
  catalogue key. It round-trips through `ExtraMeta` today and stays that way.
- Per-type bodies. Every type gets the same `# <name>` heading and an otherwise
  empty body.
