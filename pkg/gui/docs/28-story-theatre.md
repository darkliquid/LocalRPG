---
id: 28-story-theatre
title: Story Theatre, Audio & Portraits
category: Using the App
order: 28
description: Replay a campaign as a visual novel, control the spoken lines, and give characters portraits.
---

# Story Theatre, Audio & Portraits

The theatre replays a campaign as a visual novel, the audio controls speak its
lines, and portrait generation gives every character a face. The theatre, the
audio controls, and the portraits share one pipeline, so what the theatre plays is
what a line plays when you click it during normal play.

## Story Theatre

**Theater** in the campaign header opens the replay. The theatre reads the turns
already in the campaign and presents them as scenes, one beat at a time, and it
generates nothing.

### The stage

The stage fills the window with the current turn's picture. A layered scene is
drawn with a slow drift, and the stage is tinted for the turn's outcome, with an
overlay for the location's weather. The protagonist's portrait is drawn on the
left and the active character's on the right, mirrored to face them. The speaker is
outlined, and each portrait has a label underneath.

A turn with no illustration falls back to the campaign's own colours. A character
with no portrait gets a procedural bust, so a face is always drawn.

### The dialogue

The header shows the theatre's own title, the turn's location, and a counter such
as `Turn 3 of 12`. Below the stage, one beat is shown at a time: the speaker's
name, their portrait, and the line. A narrated beat is attributed to the narrator.
Clicking the panel, or pressing Enter or Space while it has focus, moves to the next
beat.

A roll is drawn above the line as a dice card with the notation, the result, and the
outcome. A beat with no speech clip shows a **no audio** badge, so a silent line is
visibly silent.

### Controls

| Control | What it does |
| --- | --- |
| **Previous line** | Step back one beat. |
| **Play** / **Pause** | Start or stop playback. |
| **Next line** | Step forward one beat. |
| **1x** | Cycle the playback speed: 1x, 1.5x, 2x. |
| **Captions** | Show or hide the speech captions. Off by default. |
| **Enable audio** | Unblock playback when the browser refused to start it. |

A progress rail under the controls shows the position through the campaign. While
speech is being rendered, the transport reports **Rendering speech…**, so a pause is
not mistaken for a hang.

Playback advances a group at a time rather than a line at a time, and the lines of
a group can share one clip; the next section describes groups. With no voice
configured for a campaign, the theatre paces the text on its own, at a reading
speed scaled by the chosen rate. Escape closes the theatre.

## Line and group audio

Every spoken line during normal play shows a control on hover, and the theatre's
beats show the same control: **Play this line**, **Stop this line**, and
**Regenerate this line**.

### Grouped speech

A group is a run of adjacent lines by one speaker, rendered in one request and
stored as one clip. The engine merges them because a provider charges and
rate-limits per request, and because one request keeps the performance continuous
across sentence boundaries.

When a line belongs to a group, its control reads **this group** rather than
**this line**, and it appears once, on the group's first line. Regenerating any line
of a group regenerates the group's clip, because the clip is the unit the provider
and the cache use.

| Setting | Values | Effect |
| --- | --- | --- |
| `media.tts.grouping` | `auto`, `off`, `always` | Whether adjacent lines are merged. `auto` merges, `off` gives a clip per line. |
| `media.tts.stream_sentences` | `true`, `false` | Whether lines are synthesized as they stream. On by default, off for a metered provider. |
| `media.tts.multi_speaker` | `true`, `false` | Whether a provider may render more than one voice in one request. |

`always` renders the whole turn as one batch and streams nothing, which suits a
provider with a cheap batch API. `off` with streaming on gives the lowest latency
for a local model. The default, `auto`, merges as it streams.

A clip's identity is a hash of the lines, the voice, the prosody, and the provider,
so two characters that share a voice never share a clip, and the same speech is
never rendered twice.

### Batch speech for a campaign

Rendering speech line by line spends interactive quota and holds the turn while it
runs. The provider's batch API renders the missing speech for a campaign, at a
discount, in the background.

**Batch Jobs** in settings starts a job or reports the jobs already running. Each
job lists its campaign, provider, model, phase, progress, and failed clips, and can
be downloaded now, cancelled, or cleared once finished. The same jobs are available
from the command line, which is the way to run one on a machine with no window
open:

```bash
localrpg tts batch my-campaign --wait
```

See [Command-Line Reference](25-cli-reference.md) for the flags. A job continues
across a restart: the clip cache is the record of what is done, and the job row
records only what is in flight.

## Character portraits

A portrait is generated for a character, stored in the campaign, and served to the
app from there. The workflow is the same whether the character came from the
campaign creator, the note wizard, or an imported world.

### The portrait workflow

1. The campaign is scanned for characters. A character with no portrait of its own
   is queued, and one whose attributes are missing is enriched first, so the prompt
   has something to work with.
2. The prompt is built from the world's art style and genre plus the character's
   traits: a three-quarter bust, looking slightly to the right, on a plain studio
   backdrop, at the world's style.
3. The image is written to the campaign's `assets/portraits/`, named for the
   character and a version number, and the note's frontmatter records the path.
4. The app is told the portrait is ready and shows it at once, in the chronicle, in
   the Codex, and on the theatre stage.

A portrait that no longer suits the character is regenerated from the Codex drawer's
**Regenerate portrait** button, which writes a new version and leaves the old one on
disk. Until a generation succeeds, the character shows a procedural bust, so a
portrait is never a broken image.

### Requirements and settings

Portrait generation needs an image provider. With none configured, **Regenerate
portrait** reports that generation is disabled, and enrichment still fills in the
character's attributes.

| Setting | Effect on portraits |
| --- | --- |
| `media.image.type` | The provider. Empty or `disabled` turns generation off. |
| `media.image.builtin_name` | The built-in generator, when one is selected. |
| `media.image.model`, `api_key`, `endpoint`, `command`, `args` | The provider's endpoint, credential, or command. |
| `media.image.builtin_fallback` | Whether the procedural bust stands in when a provider call fails. |
| `media.image.aspect_ratio`, `media.image.person_generation` | Provider parameters for a portrait. |
| `styles.pack` | The style pack, whose portrait species and archetypes are selected by tag. |
| The world's art style and genre | The strongest stylistic input to the prompt. |

Automatic *scene* imagery is separate, controlled by `media.image.auto_generate` and
`media.image.trigger`; see [Configuration
Reference](13-configuration-reference.md). The per-turn **Generate image** button
asks the provider for a scene picture when you want one.

## Exporting the performance

The theatre is also the exporter's player. **Export** in the campaign header, or
`localrpg export web`, writes one self-contained HTML file with the same stage,
dialogue, and transport, plus chapter navigation; `export video` renders the same
beats to a video file. Both are described in [Campaigns & Turns](04-campaigns.md)
and in the [Command-Line Reference](25-cli-reference.md).
