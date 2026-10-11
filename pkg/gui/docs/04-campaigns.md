---
id: 04-campaigns
title: Campaigns & Turns
category: Core Concepts
order: 4
description: The turn lifecycle, canonical history log, non-destructive rewinding, and living-world progression.
---

# Campaigns & Turns

A **Campaign** is an active instance of a World played under a System. It preserves full continuity through an immutable timeline, a relationship graph, and per-entity character sheets.

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

## Scene Illustrations

A campaign can illustrate its turns as well as its locations. The image policy is
`media.image.trigger`, and the illustrations live in `games/<id>/assets/scenes/`
as `turn-<N><ext>`.

The default policy is `major`: a turn is illustrated when the scene changes in a
way the player would notice, which is a scene break, a change of location, a newly
introduced character, or an extreme check outcome. An ordinary hit or miss, a
character who was already present, and a long passage of narration are none of
these. Set `trigger` to `significant` for the older, broader heuristic (any
decisive check, any new speaker, or a long turn), `scene_break` for explicit
breaks only, `every_turn`, `manual`, or `off`.

### Consistency

Successive illustrations of one place share a look. A scene's **palette** and
**lighting** are derived from the location and the world's art style, and the same
values are named in every prompt for that place. The action, the cast, and the
outcome tone still vary between turns. A room reads differently at dawn and at
night.

Where the image provider can take a **reference image**, the previous illustration
of the same location is passed with the request. The provider then sees the look it
is continuing. A provider that cannot take one gets the stable prompt instead.

### Budget and approval

A campaign can bound what its images cost, in `game.yaml`:

```yaml
settings:
  image_budget:
    max_images: 200        # 0 = unlimited
    max_micros: 5000000    # 0 = unlimited
  image_approval: auto     # auto | ask
```

An image is counted and charged as it is generated, and the counters are saved with
the campaign. When the budget is spent the provider is not called: the free
procedural generator draws the beat instead, so a turn is never left without an
image, and the skip is traced. With `image_approval: ask` a metered provider waits
for you to ask for the image rather than generating one automatically; a local
provider is never asked, because it does not charge. The turn readout shows how
many images remain.

### Exports

An export shows a turn's illustration where one exists and the location backdrop
otherwise, in both the web and the video export. The exported video and page show
the same moments the app showed.

The stage animates. A beat's image drifts slowly (a Ken Burns zoom and pan), a
layered scene moves its background less than its foreground, the turn's outcome
tints the picture, and rain, snow, or fog draws a light overlay. The app and both
exports compute the effects from the same rules, and a story looks the same in
each. A `prefers-reduced-motion` preference disables the motion.

Captions are off by default. Turn them on from the transport to read the current
spoken line as an overlay. The exported page includes a WebVTT track for the same
lines, and a video export writes that track as a `.vtt` sidecar beside the
`.webm`.

An export is navigable by chapter. Each scene is a chapter, the web player lists
them beside the transport and seeks when one is chosen, and a video export writes
an ffmpeg chapters sidecar (`.chapters.txt`) next to the video. A scene boundary
uses a longer transition than a beat change. That makes a location change read
distinctly.

The export scales each embedded illustration down to the size the player shows. A
bundle does not grow with the source images. The campaign's own files are never
changed.
