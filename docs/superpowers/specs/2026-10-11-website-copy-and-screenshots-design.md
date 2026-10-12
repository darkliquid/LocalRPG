# Website Copy and Screenshots Update Design

## Overview
As of v0.5.0, LocalRPG has expanded significantly, introducing structured AI World Generation with draft review, Systems Studio mechanics authoring with automated test scenarios, local ONNX embeddings, a restructured Settings Studio with tabbed provider and media management, standalone content package import/export, and dynamic character appearances.

This design updates the showcase website copy (`home.html.tmpl`), the project documentation (`README.md`), the gallery captions (`tools/sitegen/content.go`), and the screenshot suite (`website/screenshots/`) so the public-facing showcase accurately highlights the current state of LocalRPG.

---

## 1. Website Copy Updates

### Hero Section (`tools/sitegen/templates/home.html.tmpl`)
- **Eyebrow:** Keep the punchy branding:
  ```html
  <p class="eyebrow">{{ icon "sparkles" }} Local-First · AI-Powered · Custom Worlds &amp; Rules</p>
  ```
- **Hero Lede:**
  Update to highlight AI world generation, custom mechanics with scenario testing, voice narration with STT dictation, and standalone export:
  ```html
  Craft original worlds with AI-assisted generation, build custom tabletop rules with automated
  test scenarios, and embark on solo campaigns with local AI as a first-class citizen. LocalRPG
  brings your adventures to life with multi-voice narration, speech-to-text dictation, style-pack
  scene imagery, and shareable visual novel exports.
  ```
- **Hero Quick Facts:**
  - **Local AI:** First-class local models &amp; ONNX embeddings
  - **Voices:** Narration, NPC speech &amp; STT dictation
  - **Theatre:** Replay &amp; export web/video bundles

### Feature Cards Grid (`tools/sitegen/templates/home.html.tmpl`)
Realign the 6 glassmorphic capability cards to represent the v0.5.0 system pillars:
1. **First-Class Local AI & Embeddings** (`cpu` icon):
   Connect local LLMs via Ollama, LM Studio, or vLLM, run pure-local ONNX embeddings for semantic memory, or play instantly with zero-GPU built-ins that require no setup.
2. **AI World Generation & Living Lore** (`sparkles` icon):
   Author factions, locations, and starter lore by hand, or generate complete, cohesive settings with schema-first AI pipelines, batch entity expansion, and draft review.
3. **Custom Systems & Scenario Testing** (`shield` icon):
   Design dice mechanics, outcome tiers, and character creation prompts with worked examples in Systems Studio, and prove system balance with automated test scenarios.
4. **Dynamic Scene Art & Style Packs** (`image` icon):
   Bring scenes to life with curated art style packs and procedural vector art, preserving permanent world aesthetics while reflecting dynamic character appearance changes.
5. **TTS Narration & Voice Dictation** (`layers` icon):
   Hear prose narrated aloud while NPCs speak with distinct voice profiles and pitch. Take your turns hands-free using click-to-talk speech-to-text dictation.
6. **Story Theatre, Packages & Export** (`book` icon):
   Relive adventures in visual novel Theatre Mode, share worlds and rules via standalone content packages, and export complete campaigns as web bundles or rendered video.

---

## 2. Project README Alignment (`README.md`)

- **Features List (`## Features`):**
  Mirror the 6 pillars defined above, explicitly mentioning ONNX embeddings, schema-first world generation, automated test scenarios, and content package imports/exports.
- **Workflow Section (`## Creating Worlds & Playing Campaigns`):**
  Update steps to note AI-assisted generation and draft review in Worlds Studio, and automated test scenarios in Systems Studio.
- **Settings & Providers (`## Configuration & Provider Setup`):**
  Document the restructured studio tabs: Providers (LLMs, Embeddings, Shared Keys), AI Agents, Media Engines (TTS, STT, Image Generation), and Advanced preferences.

---

## 3. Gallery Captions (`tools/sitegen/content.go`)

Update `showcaseShots` in `tools/sitegen/content.go`:
- `01-launcher.jpg` (Launcher Hub):
  "Campaigns, worlds, and systems dock with quick-start creation wizards, ready for immediate play."
- `02-worlds-studio.jpg` (Worlds Studio):
  "Author factions, lore, and entity trees by hand or generate complete settings with AI pipelines."
- `03-systems-studio.jpg` (Systems Studio):
  "Craft dice mechanics and action modes with worked examples, and verify balance with test scenarios."
- `04-settings.jpg` (Settings Studio):
  "Configure local LLMs, ONNX embeddings, multi-voice audio, and media engines across structured tabs."
- `05-codex.jpg` (Codex Drawer):
  "Inspect wikilinked entity lore, track dynamic character appearances, and customize voice profiles."
- `06-story-theatre.jpg` (Story Theatre):
  "Replay campaign scenes as an interactive visual novel, spoken in each character's distinct voice."

---

## 4. Screenshot Capture Workflow

### Automated Studio Capture (`website/scenarios/screenshots.yaml`)
- Verify and refine the step sequence for capturing:
  - `01-launcher.jpg`: Initial load at `/`.
  - `02-worlds-studio.jpg`: Navigates to Worlds Studio.
  - `03-systems-studio.jpg`: Navigates to Systems Studio (ensuring the Mechanics and Tests tabs are visible).
  - `04-settings.jpg`: Navigates to Settings Studio (showing the tabbed Providers layout).
- Execute `mise run site:screenshots` to capture the four studio views headlessly at 1280x800 JPEG quality 88.

### Gameplay Captures (`05-codex.jpg` & `06-story-theatre.jpg`)
- Update `05-codex.jpg` and `06-story-theatre.jpg` from a live test campaign instance:
  - `05-codex.jpg`: Shows an entity note displaying dynamic traits, current appearance, and voice options.
  - `06-story-theatre.jpg`: Shows Story Theatre active playback with character portraits and dialogue pane.
- Save as 1280x800 JPEG (quality 88) into `website/screenshots/`.

---

## 5. Verification & Testing

- **Prose & Doc Linting:**
  - Run `mise run lint:prose` (Vale) to verify all updated prose conforms to style rules.
  - Run `mise run lint:docs` (rumdl) to ensure Markdown docs adhere to formatting rules.
- **Site Generation & Local Preview:**
  - Run `mise run site:build` to confirm static rendering completes with all 6 screenshots recognized (`sitegen: wrote 26 pages and 6/6 screenshots to website/dist`).
  - Verify that no broken links or missing asset errors occur.
