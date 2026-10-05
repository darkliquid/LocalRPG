---
id: 10-codex-yaml
title: Codex YAML Frontmatter & Graph Reference
category: Codex & Content Reference
order: 10
description: Complete YAML frontmatter schema, entity types, custom state, and wikilink graph modeling.
---

# Codex YAML Frontmatter & Graph Reference

In LocalRPG, every entity in your campaign (characters, locations, factions, items, and concepts) is represented as a plain Markdown file in `entities/` with YAML frontmatter enclosed in `---` delimiters.

## Full Entity Frontmatter Schema

```yaml
---
id: lady-evelyn
name: Lady Evelyn Vance
type: character # character | location | faction | item | concept
tags: [noble, merchant, ally, secretive]
location: "[[the-iron-bastion]]"
faction: "[[the-gilded-cabal]]"
portrait: "assets/portraits/lady-evelyn.png"
appearance: "A slender woman in midnight-blue velvet holding an engraved silver spyglass."
gender: female
age: "34"
aliases:
  - The Blue Falcon
  - Evelyn Vance
voice:
  provider: elevenlabs
  voice_id: "21m00Tcm4TlvDq8ikWAM"
  pitch: 1.0
  speech_rate: 1.05
state:
  hp: 24
  max_hp: 24
  disposition: friendly
  gold: 140
  skills:
    persuasion: 4
    intrigue: 3
inventory:
  - "[[iron-vault-key]]"
  - "Silver spyglass"
  - "Vial of belladonna"
---

# Lady Evelyn Vance

Evelyn is the second daughter of the Vance merchant dynasty. While outwardly managing shipping manifests, she covertly funds salvage expeditions into the sunken ruins.
```

## Supported Entity Types

The New Note wizard offers these types. Each one scaffolds the frontmatter keys
it uses, every key preceded by a line explaining its purpose. The engine
itself accepts any type string, so a type not listed here still loads.

- **`character`**: Player characters, NPCs, companions, and adversaries. Offers `voice`, `appearance`, `gender`, and `age`.
- **`location`**: Towns, rooms, regions, and any other place a scene happens in. Offers `appearance`, and nests inside a parent location through `location: "[[parent-zone]]"`.
- **`faction`**: Guilds, orders, crews, and governments. Offers `appearance`. Influence, reputation, and rivalries live in `state`.
- **`item`**: Relics, weapons, tools, and keys. Offers `appearance`.
- **`concept`**: Ideas, customs, deities, and forces that define the world.
- **`arc`**: A running storyline or threat, tracked as a progress clock in `state`.
- **`event`**: Something that happened, or something yet to happen.
- **`quest`**: A goal the player can pursue, with a `state` to track it.
- **`lore`**: Background history or a piece of world knowledge.

## Creating a Note

The **+ New** button in the codex, the Content Studio, and the Worlds Studio opens
the same wizard. Give the note a name, pick its type, and answer the options the
type asks for, such as a voice for a character. The wizard writes the frontmatter
with every key for that type present and documented, and opens it in the editor,
where the autosuggester completes keys and known values as you type.

## Graph Modeling with `[[Wikilinks]]`

LocalRPG automatically converts wikilinks into relationship edges in the campaign knowledge graph:

- **Frontmatter references**:

  ```yaml
  location: "[[sunken-spire]]"
  faction: "[[salvage-guild]]"
  ```

- **Prose references with labels**:

  ```markdown
  Evelyn was apprenticed to [[master-corvus|Arch-Mage Corvus]] before the fall.
  ```

Edges are bidirectional in search queries: when the player visits `[[sunken-spire]]`, the context assembler automatically loads all characters whose `location` points to `sunken-spire`.
