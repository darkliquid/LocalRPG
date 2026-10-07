---
id: 22-editing-content
title: Editing Content Outside the App
category: Codex & Content Reference
order: 22
description: The on-disk and package formats as the interchange contract, so systems, worlds, and campaigns can be edited in any editor.
---

# Editing Content Outside the App

LocalRPG is local-first and file-based. A system is a directory of YAML, JavaScript, and Markdown, and a world is a directory of Markdown, so the studios are a convenience rather than the only way in. Every path LocalRPG reads resolves through `core.PathResolver`, and every file it writes is one you can open in any editor.

This article documents that layout as the interchange contract: the contents of each directory, the meaning of each manifest field, how packing works, and what LocalRPG rewrites behind your back.

## 1. The Content Tiers

LocalRPG keeps content in three tiers, each one a directory of directories named by ID.

| Tier | Directory | Contents |
| --- | --- | --- |
| System | `systems/<id>/` | Mechanics: `system.yaml`, `mechanics.js`, `prompts/rules.md`, `tests/*.yaml`. |
| World | `worlds/<id>/` | Lore: `world.yaml`, `prompts/lore.md`, `entities/`, `system_overrides/`. |
| Campaign | `games/<id>/` | One campaign on disk: `game.yaml`, `history.jsonl`, `entities/`, `assets/`, `cache/index.db`, `content.lock.yaml`. |

An ID is a directory name, and `pathutil.SanitizeID` normalises it before every lookup, so a trailing space or a capital letter does not create a second copy of the same content.

By default the three tiers default to the platform data directory (`$XDG_DATA_HOME/localrpg/` on Linux and BSD, `~/Library/Application Support/localrpg/` on macOS, `%LOCALAPPDATA%\localrpg\` on Windows), and the campaign cache defaults to the cache directory. The `paths.systems`, `paths.worlds`, `paths.games`, and `paths.cache` configuration keys override each one: an absolute value applies verbatim, and a relative value resolves against the platform data directory. Launch LocalRPG inside a directory containing `./localrpg.yaml`, or pass `--dir <path>`, and relative values resolve against that project root instead, which is what makes a content tree portable in Git. See [Storage & Directories](07-storage-paths) for the full resolution order.

## 2. A System

A system declares identity, the rules text the narrator reads, an optional mechanics schema, and optional deterministic tests.

```text
systems/classic-d20/
├── system.yaml          # identity, character creation, mechanics schema
├── mechanics.js         # sandboxed JavaScript hooks
├── prompts/
│   └── rules.md         # rules text handed to the Game Master agent
└── tests/
    └── checks.yaml      # deterministic mechanics scenarios
```

### `system.yaml`

Every key except `id` and `name` is optional, and a system without a `mechanics` block is schema-agnostic: `mechanics.js` then owns everything.

```yaml
id: classic-d20
name: Classic d20
version: 1.0.0
description: Attributes, skills, and a d20 roll against a target number.
character_creation:
  preamble: Answer these to build your character.
  fields:
    - id: name
      label: Name
      kind: text
      required: true
    - id: concept
      label: Concept
      kind: long
mechanics:
  stats:
    - id: might
      label: Might
      type: number
      default: 1
      min: 0
      max: 6
  skills:
    - id: athletics
      label: Athletics
      stat: might
  health:
    stat: hp
    max_stat: max_hp
    zero_effect: down
  checks:
    notation: 1d20
    outcome: [strong, weak, miss]
    difficulty:
      - id: easy
        label: Easy
        target: 8
      - id: hard
        label: Hard
        target: 15
```

The `mechanics` block maps onto `core.MechanicsSpec`:

| Key | Meaning |
| --- | --- |
| `stats` | Declared stats, each with `id`, optional `label`, `type` (`number`, `string`, or `bool`), `default`, `min`, and `max`. |
| `skills` | Named skills, each pointing at the `stat` it uses. |
| `health` | The health `stat`, the `max_stat` that holds its ceiling, and the `zero_effect` label applied at zero. |
| `checks` | Default `notation`, the `outcome` vocabulary, named `difficulty` targets, and `profiles` for alternative resolution models. |
| `allow_freeform_state` | Permits state changes to paths the schema does not declare. |
| `engagement` | Default mechanics policy: `off`, `auto`, or `ask`. |
| `advancement` | Progression: a currency stat, how a character gains it, and what it buys. |

The `checks.profiles` map declares named resolution profiles. A profile can be a threshold `ladder`, a difficulty class in `dc`, a success-count pool with `success_on` and `outcomes`, or a `position` and `effect` vocabulary. See [Systems & Mechanics](03-systems) for worked examples of each kind.

### `mechanics.js`

LocalRPG loads the file into a sandboxed Goja runtime, and the host bridge receives the `mechanics` block, so a script can read the stats and conventions the schema declares. A world can extend the system with `worlds/<world>/system_overrides/<system-id>/hooks.js`, which loads after the base script, so it can replace a handler.

The hooks are `onAction`, `onTurnBegin`, `onTurnEnd`, `onWorldTick`, `onCheck`, and `onHealthZero`, alongside helpers such as `roll`, `getStat`, `setStat`, and `injectGMDirection`. [Systems & Mechanics](03-systems) documents them in full.

### `tests/`

Every `*.yaml` under `systems/<id>/tests/` is a deterministic scenario: a name, an optional `seed`, a `setup` with the player's starting stats and tags, and a list of `steps` with the action to run and what it must produce. Run them without a provider, a store, or a network:

```bash
localrpg debug test-system classic-d20
```

A step asserts any combination of `outcome`, a `total` range, resulting `state`, and `message_contains`. Scenarios run in filename order, so a prefix is the way to sequence them.

## 3. A World

A world supplies setting, tone, starter entities, and per-system rule overrides.

```text
worlds/eldoria/
├── world.yaml
├── prompts/
│   └── lore.md
├── entities/
│   ├── the-iron-bastion.md
│   └── characters/
│       └── lord-aldous.md
└── system_overrides/
    └── classic-d20/
        └── hooks.js
```

### `world.yaml`

```yaml
id: eldoria
name: The Sunken Reach
version: 1.0.0
description: A mist-shrouded archipelago of submerged ruins and arcane salvage.
genre: nautical-fantasy
art_style: ink-wash, muted teal, low horizon
default_system: classic-d20
tags: [mysterious, grim, salvage]
requires:
  - type: system
    id: classic-d20
    version: ">=1.0.0 <2.0.0"
```

| Key | Meaning |
| --- | --- |
| `id` | The directory name and the ID a campaign refers to. |
| `name` | Display name. |
| `version` | Semantic version, checked against a world's own `requires` constraints. |
| `description` | Prose summary, used in listings and as the seed for a generated opening scene. |
| `genre` | Free-form genre label. |
| `art_style` | Style text applied to generated scene and portrait images. |
| `default_system` | The system this world is built for. The world listing reports it as the compatible system. |
| `tags` | Free-form labels for browsing. |
| `requires` | Content dependencies, each a `type` (`system` or `world`), an `id`, and an optional `version` constraint in semantic version form. |

LocalRPG enforces a `requires` entry when a campaign is created and again on every open: a world whose constraint is `>=1.0.0 <2.0.0` refuses a system at `0.9.0` rather than playing with mismatched rules.

### `entities/`

Each note is a template. Creating a campaign copies the files directly inside `entities/` into `games/<id>/entities/`, renames each to `<frontmatter-id>.md`, and indexes them. A subdirectory is left untouched, so a world can keep notes it does not want in every campaign. See [Worlds & Lore](02-worlds) for the opening-location resolution that runs after the copy.

## 4. Entities

An entity is one Markdown file: YAML frontmatter between `---` delimiters, then prose. The body is the description for the reader, and a model extends only that part.

```markdown
---
id: lady-evelyn
name: Lady Evelyn Vance
type: character
tags: [noble, merchant, ally]
location: "[[the-iron-bastion]]"
faction: "[[the-gilded-cabal]]"
portrait: "assets/portraits/lady-evelyn.png"
appearance: A slender woman in midnight-blue velvet.
gender: female
age: "34"
aliases:
  - The Blue Falcon
voice:
  provider: elevenlabs
  voice_id: "21m00Tcm4TlvDq8ikWAM"
  pitch: 1.0
  speech_rate: 1.05
state:
  hp: 24
  max_hp: 24
  disposition: friendly
---

Evelyn is the second daughter of the Vance merchant dynasty.
```

- **Name the file after `id`.** The graph derives node IDs from file names, so `lady-evelyn.md` and `id: lady-evelyn` must agree. The indexer falls back to the file name when `id` is absent, which keeps a hand-written note working but hides a typo.
- **Keep the frontmatter valid.** The indexer skips a note whose YAML does not parse, in silence, and the entity is then missing from the codex while the file is on disk.

| Field | Meaning |
| --- | --- |
| `id` | Stable identifier, matching the file name. |
| `name` | Display name. |
| `type` | `character`, `location`, `faction`, `item`, `concept`, `arc`, `event`, `quest`, or `lore`. Any string loads, and the listed types are the ones the note wizard scaffolds. |
| `tags` | Free-form labels, used for filtering and voice-profile matching. |
| `location` | Where the entity is, in most cases a `[[wikilink]]`. |
| `faction` | The faction it belongs to, in most cases a `[[wikilink]]`. |
| `voice` | TTS settings for a character: `provider`, `voice_id`, `pitch`, and `speech_rate`. |
| `portrait` | Records that the entity has artwork. LocalRPG reads the image from `assets/portraits/<id>` with a `.png`, `.jpg`, `.jpeg`, `.webp`, or `.svg` extension, and a regenerated portrait adds a `-v<n>` version suffix. |
| `appearance`, `gender`, `age` | Prose and labels fed into image and narration prompts. |
| `aliases` | Other names for the same entity. |
| `history` | Turn numbers the entity appeared in, maintained by the engine. |
| `state` | Free-form character sheet. |

`state` is what makes the engine schema-agnostic: nothing in LocalRPG knows what `hp` or `mana` means, so LocalRPG stores, shows, and passes any key you add to `mechanics.js` unchanged. A system that declares `stats` may restrict which paths a hook can write, and `allow_freeform_state` lifts that restriction.

Notes may sit in subdirectories, and the walk is recursive: `entities/characters/lord-aldous.md` is the same note as `entities/lord-aldous.md`, with the folder recorded as its location rather than as a field. The walk skips hidden directories and `assets/`.

Links written as `[[wikilinks]]` in `location`, `faction`, and the prose become graph edges, which is how the context assembler loads the characters in the player's current scene. [Codex YAML Frontmatter & Graph Reference](10-codex-yaml) documents the field list and the link forms.

## 5. The Package Format

A `.lrpgpack` is a gzip-compressed tar archive containing a content tree plus a manifest, and optionally a signature. This is the interchange format for sharing a system or a world.

```text
classic-d20.lrpgpack          # gzip-compressed tar
├── package.yaml              # first member: the manifest
├── system.yaml               # the content tree, relative to the content root
├── mechanics.js
├── prompts/rules.md
└── package.sig               # last member, present only when signed
```

### `package.yaml`

```yaml
id: classic-d20
name: Classic d20
version: 1.0.0
type: system
description: Attributes, skills, and a d20 roll against a target number.
author: The Classic d20 authors
license: CC-BY-4.0
min_app_version: 1.0.0
dependencies:
  - type: world
    id: eldoria
    version: ">=1.0.0"
files:
  - path: system.yaml
    sha256: 0d2b...c1
    size: 412
```

| Key | Meaning |
| --- | --- |
| `id`, `name`, `version` | Identity, taken from the system or world manifest unless overridden at export. |
| `type` | `system` or `world`. Detected from `system.yaml` or `world.yaml` when not given. |
| `description`, `author`, `license` | Descriptive metadata. |
| `min_app_version` | Declared minimum app version. Recorded in the manifest for readers and tooling; import does not enforce it. |
| `dependencies` | Other packages this one needs, each with `type`, `id`, and an optional version constraint. |
| `files` | Every payload file with its SHA-256 and size. Generated at pack time. |

LocalRPG computes `files`, and you never author it. The manifest itself and `package.sig` stay out of it, and the archive omits hidden files, directories named `cache`, and `*.legacy` files.

### `package.sig`

A signature covers the content digest, which is the SHA-256 of the sorted `path`, `sha256`, and `size` lines of `files`. Changing one byte of any payload file invalidates it.

```yaml
algorithm: ed25519
key: 9f86d081884c7d65...   # SHA-256 fingerprint of the public key
public_key: MCowBQYDK2Vw... # base64-encoded Ed25519 public key
signature: MEUCIQDx...      # base64-encoded signature
publisher: The Classic d20 authors
```

Verification yields one of four trust states: `verified` when the signature matches and the key is trusted, `unknown_key` when it matches but the key is not, `unsigned` when there is no `package.sig`, and `invalid` when the signature or a payload checksum does not match.

### Command line

```bash
localrpg content export <world|system> <id> [--out <file>]
localrpg content import <file> [--on-conflict refuse|rename|overwrite] [--yes]
localrpg content sign <file|dir> --key <private-key> [--publisher <name>]
localrpg content verify <file|dir>
localrpg publisher add <key-file|fingerprint> --name <name>
localrpg publisher list
localrpg publisher remove <fingerprint>
```

`content export` writes to standard output unless `--out` names a file, and `content import` refuses a clash by default: `rename` installs under a free ID instead, and `overwrite` replaces what is there. Import verifies every member against the manifest before writing anything.

`content sign` accepts either a `.lrpgpack` or an unpacked content directory, which is how a content author signs a tree in place. The `publishers` configuration key maps a key fingerprint to a display name, so `publisher add` is the whole trust decision. Signed content that arrives from a registry without a trusted key reports `unknown_key` rather than `verified`.

Packages can also come from a registry index:

```bash
localrpg registry add <url>
localrpg registry search <query>
localrpg registry install <id>
localrpg registry update
```

Registry URLs live under the `registries.urls` configuration key, and LocalRPG checks every installed package against the checksum its index declares.

## 6. The Edit-Outside Workflow

1. Edit any file in `systems/`, `worlds/`, or `games/` in your editor of choice, and save it.
2. Return to the app. An open campaign rebuilds its per-campaign wiring when the modification time of `game.yaml`, `system.yaml`, `mechanics.js`, the world's `hooks.js`, `prompts/rules.md`, or `prompts/lore.md` changes, so an edit to any of those takes effect on the next turn with no restart.
3. Start a session with `localrpg play <game-id>`, which rebuilds the campaign's entity index and repairs `cache/index.db` before the first turn.

Content is the source of truth, and LocalRPG derives the index from it. Deleting `cache/index.db` costs a rebuild and nothing else, which is what makes the three tiers safe to keep in a Git repository and safe to generate with a script.

The stable parts of the contract are the tier layout, the manifest fields, and the package layout. The fields inside `state`, the prose in every body, and the contents of `assets/` are yours.

## 7. What Not to Edit

| Path | Why |
| --- | --- |
| `games/<id>/cache/index.db` | A disposable SQLite index. Delete it rather than edit it, and LocalRPG rebuilds it. |
| `games/<id>/history.jsonl` | The canonical timeline, append-only and written only by the engine. `/undo` is the one operation that rewrites it, trimming the log below the target turn. |
| `games/<id>/content.lock.yaml` | Written by LocalRPG. It records each dependency's version and a behavioural digest, and editing it produces spurious version and digest warnings. |
| `games/<id>/assets/` | Binary media, addressed by an entity's `portrait` field. The indexer skips the directory. |

A hand edit that appears to do nothing is almost always one of three failure modes.

**A malformed note disappears rather than fails.** The indexer skips a file whose frontmatter has no closing `---`, or whose YAML does not parse, so that one bad file cannot stop the rest of the campaign from indexing. The symptom is an entity missing from the codex while the file is plainly on disk, and the delimiters are the first thing to check.

**A behavioural digest change surfaces a warning.** The campaign's lock file hashes `system.yaml` and `mechanics.js` for a system, and `world.yaml` plus every `system_overrides/*/hooks.js` for a world. Editing those files surfaces a digest change on the next open, because the rules may have moved since the campaign started. Editing prose, entities, or a rules prompt does not, so a documentation pass is silent.

**Editing a signed package invalidates it.** Sign again with `localrpg content sign` after any change to a packed tree, or the next `content verify` reports `invalid`.
