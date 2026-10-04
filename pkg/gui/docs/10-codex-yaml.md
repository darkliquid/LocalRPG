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

- **`character`**: Player characters, NPCs, companions, and adversaries. Supports `voice`, `appearance`, and `inventory`.
- **`location`**: Towns, taverns, dungeons, starships, and regions. Can nest inside parent locations via `location: "[[parent-zone]]"`.
- **`faction`**: Guilds, secret societies, governments, and crews. Can track influence, reputation, and rivalries in `state`.
- **`item`**: Relics, weapons, spellbooks, and keys. Can be carried in entity `inventory` arrays.
- **`concept`**: Prophecies, historical events, cultural taboos, and magical phenomena.

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
