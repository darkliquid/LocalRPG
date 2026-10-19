---
id: 27-desktop-app
title: The Desktop App
category: Using the App
order: 27
description: A tour of the desktop window, from the launcher to the campaign screen, its drawers, and the settings panels.
---

# The Desktop App

`localrpg gui` opens the desktop application. The window has a launcher, where
campaigns are chosen and created, and a campaign screen, where a campaign is
played. The documentation reader, the Content Studio, the settings editor, and the
two studios open above either of them.

This page maps what is on screen and gives each control its name. The guide for a
topic is linked where the topic first appears.

## The launcher

The launcher is a narrow icon dock on the left and a hero area for the selected
campaign.

### The dock

The dock runs down the left edge. From top to bottom:

| Control | Opens |
| --- | --- |
| **New Campaign** (`+`) | The world strip, to choose the world the campaign will use. |
| **Browse Campaigns** | The campaign gallery. |
| Campaign icons | That campaign, as the hero. The selected campaign is highlighted. |
| **Worlds Studio** | The worlds studio. See [Building Worlds: Studio Guide](08-worlds-studio.md). |
| **Systems Studio** | The systems studio. See [Building Custom Systems: Studio Guide](09-systems-studio.md). |
| **Import Content Package** | A file picker for a `.lrpgpack` or `.lrpgworld` package. |
| **Content Registry** | The registry modal, where registry URLs are added and packages are installed. |
| **Settings** | Global settings. |
| **Help & Documentation** | The documentation reader. |

Each icon shows a label on hover, and a campaign icon's label also shows its turn
count.

### The hero stage

The hero area shows the selected campaign: its icon, name, world, system, and
protagonist, with **Play Time**, **Turns**, and **Last Played** beside it. The
**PLAY** button opens the campaign. **Campaign Settings** opens that campaign's own
settings: the narrator voice, the start location and opening directives, the banner
and icon artwork (each with **Upload** and **AI Gen**), and, in the **Danger Zone**,
**Restart Campaign** and **Delete Campaign**.

With no campaigns yet, the hero shows a welcome card offering **Create First
World**, **Browse Systems**, and the [Local-First and Offline
Play](21-local-first.md) guide.

### The galleries

**Browse Campaigns** opens the campaign gallery: a search field
(`Search campaigns, worlds, systems...`), a sort control (**Recently Played**,
**Turn Count**, **Alphabetical**), and a card per campaign. A click on a card plays
the campaign, and the card shows its turn count and a settings gear.

The world strip is the horizontal row of icons behind the dock's `+` button. It
shows one icon per world, a **Create New World** tile, and a button that expands
into the world gallery. In the gallery, a click selects a world, and a world can be
viewed full size or deleted.

### Creating a campaign

Selecting a world opens **New Campaign**. The form has several sections:

- **Campaign Title**, with a generate button beside it.
- **Rules System**, a grid of systems, with compatible ones highlighted.
- **Voice & Narration**: the narrator voice and the protagonist voice.
- **Protagonist Setup**: name, age, gender, pronouns, appearance, and background,
  followed by the creation prompts the chosen system declares.
- **Story & World Seed**: an optional start location and an optional opening scene
  directive.
- **Custom Campaign Artwork**: a banner and an icon, each with **Upload** and
  **AI Gen**.

**Auto-Fill Fields** fills the fields that can be generated from the world and
system; **Create Campaign** writes the campaign and opens it.

## The campaign screen

The header shows the campaign name, a chip for whether mechanics are engaged, any
provider limit chips, and a row of buttons:

| Button | Opens |
| --- | --- |
| **Character** | The character sheet drawer. |
| **Graph** | The lore graph drawer. |
| **Codex** | The Codex drawer, the Markdown editor for the campaign's entities. |
| **World Arcs** | The living-world drawer. |
| **Context** | The turn context and continuity drawer. |
| **Theater** | Story Theatre, the visual-novel replay. See [Story Theatre, Audio & Portraits](28-story-theatre.md). |
| **Export** | The export modal, for a web player or a video. |
| **Settings** | Global settings. |
| **Docs** | The documentation reader. |

**Back**, at the left of the header, returns to the launcher.

The body of the screen is the chronicle: the illustration and narration of each
turn, in order, with the newest at the bottom. A turn that produced a finding, a
world tick, or a health effect shows a notice beside it.

Below the chronicle are the **Mechanics** strip, which reports whether mechanics
are engaged, how many checks are pending, and how much image budget is left, and a
pending check card when a check is waiting.

At the bottom is the action console.

### The drawers

Each header button opens one drawer, and every drawer has the same frame: a title, a
button that widens it, and a close button.

- **Character Sheet** shows the player's name, level and type, a health bar,
  advancement currency with its costs and the reason one cannot be spent, the
  description, and the sheet's own attributes. The campaign's system defines the
  sheet, and the fields differ per game.
- **Lore Graph** draws the campaign's entity graph, links and all, with a legend
  for characters, non-player characters, locations, and lore. Clicking a node opens
  the note behind it.
- **Codex Markdown Editor** is the full note editor, and it shows a list of notes
  on the left, the Markdown, the frontmatter, the notes that link here, and the
  controls to save, merge a duplicate note, apply a voice archetype, or regenerate
  the note's portrait. See [Codex YAML Frontmatter & Graph
  Reference](10-codex-yaml.md).
- **Turn Context & Continuity** shows what the last turn's prompt was assembled
  from: the mode and strategy, the continuity working set, the prompt sections with
  the share of the budget each took, and the assembled prompt itself.
- **Living World Arcs & Clocks** shows the story so far, the open threads, the
  active narrative arcs, and the faction clocks.

### Playing a turn

The action console has four tabs, and each one sets the mode for a turn:

| Tab | Read as |
| --- | --- |
| **DO** | Something the character does. |
| **SAY** | Something the character says. |
| **STORY** | A narration or a scene-setting statement. |
| **ROLL** | A request for a check, shown only when the campaign has mechanics engaged. |

Typing `/do`, `/say`, `/story`, or `/roll` at the start of a line sets the mode for
that turn without moving the tab. The microphone button dictates input through the
configured speech-to-text provider. **Submit** plays the turn; while the narrator is
writing, the button becomes **STOP**.

A turn that ends on a check shows a **Roll required** card with the stakes, the
notation, the modifiers, and the outcomes. The card offers **Roll**, a manual entry
field, and **Argue**, which puts your case for a different check to the narrator.

A scene with no illustration yet offers **Generate image** on the turn. Speech
controls appear per line on narrated turns; both are covered in [Story Theatre,
Audio & Portraits](28-story-theatre.md).

### The prologue

A campaign with no turns yet shows the **Prologue** instead of the chronicle: the
campaign's name, the optional opening prompt, and two buttons, **Begin the story**
and **I'll take the first step**.

## Settings

Settings opens from the dock or from the campaign header, and it is one editor with
eight tabs.

| Tab | Configures |
| --- | --- |
| **Paths** | The directories for systems, worlds, campaigns, and the media cache. |
| **Providers** | Cloud API keys, the embeddings provider, and the offline preset, in three sub-tabs. |
| **AI Agents** | Which provider serves each agent role, the routing and fallback order, and the context and generation limits. |
| **Media Engines** | Speech synthesis, speech recognition, and image generation providers, in three sub-tabs. |
| **Batch Jobs** | Speech backfill jobs, their progress, and their downloads. |
| **Preferences** | Cinematic overlays, type scale, the style pack, and story token streaming. |
| **Usage** | The spend ledger, by provider and by role, for this campaign or all of them. |
| **Debug** | The trace level and the trace file's bounds, plus the recorded events and raw provider lines. |

A **Show advanced settings** toggle reveals the less common fields, and **Save
Settings** writes the configuration. Problems the loader found are listed above the
form rather than failing the save.

## The menu bar and the reader

Press **Alt** to reveal the application menu bar, and **Alt** again to hide it. It
has two menus: **View** (reload, full screen, zoom) and **Help** (Content Studio,
Documentation, Report an Issue, Project Repository, and About LocalRPG).

**Documentation** opens the reader where this guide is read: a search box, a
sidebar grouped by category, and the article itself. **Content Studio** is the raw
editor for entities, prompts, and manifests, for editing outside a campaign.

## Keyboard

The desktop app binds few keys.

| Key | What it does |
| --- | --- |
| `Alt` | Show or hide the menu bar. |
| `Esc` | Close the menu bar, the reader, a drawer, or a modal. |
| `Enter` or `Space` | Advance the Story Theatre dialogue when it has focus. |

The View menu lists the conventional shortcuts for reload, full screen, and zoom.
The desktop shell handles those rather than the application.
