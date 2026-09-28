---
id: 04-campaigns
title: Campaigns & Turns
category: Core Concepts
order: 4
description: The turn lifecycle, canonical history log, non-destructive rewinding, and living-world progression.
---

# Campaigns & Turns

A **Campaign** is an active instance of a World played under a System. It preserves full continuity through an immutable timeline, dynamic relationship graphs, and evolving character sheets.

## The Turn Lifecycle

Every turn passes through a disciplined multi-step pipeline:

```text
[Player Action] ──> [GM Directives / Undo Check]
                          │
                          ▼
            [Dice / Mechanics Evaluation]
                          │
                          ▼
            [4-Layer Context Prompt Assembly]
                          │
                          ▼
              [Narrator Prose Generation]
                          │
                          ▼
           [Entity Mention & Dialogue Parsing]
                          │
                          ▼
       [Entity Extraction & Graph Reconciliation]
                          │
                          ▼
        [history.jsonl Append & Index Update]
```

1. **Directive Handling**: The engine detects slash commands like `/undo` or `/gm <directive>` (which injects steering instructions to the GM).
2. **Mechanics Resolution**: If the action requires dice or triggers a JS hook, the outcome is computed before the GM narrates.
3. **Context Layering**: The engine builds a focused prompt assembling:
   - System rules and mechanics cues.
   - World lore and tone constraints.
   - Voice profile catalog.
   - Scene scope: reachable entities from current location via knowledge graph edges.
   - Active narrative arcs and faction clocks.
4. **Narration & Speech**: The narrator produces prose with inline speaker tags and stage directions.
5. **Timeline Recording**: Turn segments are written to `history.jsonl` and mirrored to `cache/index.db`.

## The `history.jsonl` Canonical Log

Campaign history is stored as append-only newline-delimited JSON (`games/<id>/history.jsonl`). Each record captures:

- Sequential turn number.
- Raw player prompt.
- Narrator's rewritten output.
- Turn segments with identified speakers.
- Mentioned entity IDs and newly discovered concepts.
- Dice roll results and mechanics state patches.

## Non-Destructive Rewind (`/undo`)

Because `history.jsonl` is the source of truth, invoking `/undo` rewinds the campaign cleanly:

- The log is truncated back to the chosen turn number.
- `cache/index.db` turn entries are pruned.
- Entity history turn markers are rolled back, while authored entity lore and character sheets remain intact.
