# Twintail-Inspired Launcher Redesign

**Status:** Approved by User  
**Date:** 2026-09-23  
**Authors:** Antigravity & User  

---

## 1. Overview & Goals

LocalRPG's campaign launcher is being redesigned into a game-first, visually immersive desktop experience inspired by the modern **Twintail Launcher** aesthetic.

### Key Goals
1. **Hero-Driven Interface:** Replace the traditional multi-tab dashboard with an atmospheric, full-window campaign presentation showcasing high-fidelity banner art.
2. **Left Vertical Dock:** Provide quick, thumb-friendly navigation with an Add button (`+`), scrollable campaign avatars by icon, and utility access to Worlds Studio, Systems Studio, and Global Settings.
3. **Streamlined Campaign Creation:** Clicking `+` expands a horizontal flyout of worlds by icon with hover tooltips; clicking a world opens a dedicated, pre-populated creation dialog featuring the world's banner.
4. **Dual-Image Asset Model:** Support 2 images per campaign and world (full-screen/header banner + avatar icon), with instant deterministic procedural fallbacks, on-demand AI art generation, and custom file uploads.
5. **Legible Modern Typography:** Use clean, highly readable sans-serif typography (`font-sans`, `system-ui`) throughout the launcher, cards, badges, and procedural monograms.
6. **Graceful Zero-States:** Intelligently handle fresh installs with no campaigns (prompting world selection) and no worlds/systems (prompting initial world/system creation).

---

## 2. Visual Architecture & Layout

```
+-----------------------------------------------------------------------------------+
| [DOCK]  | [HERO STAGE]                                                            |
|         |                                                                         |
|  (+) -> | [WORLD FLYOUT (when '+' active)]                                        |
|  ---    |  [World 1] [World 2] [World 3] ... [+ Create World]                    |
|  (C1)*  |                                                     [TOP-RIGHT STATS]   |
|  (C2)   |                                                     42 turns | 2h 15m   |
|  (C3)   |                                                     Last played: 21:40  |
|         |                                                                         |
|         |                                                                         |
| [space] |                                                                         |
|  (🌐)   | [BOTTOM-LEFT TITLE CARD]                          [BOTTOM-RIGHT ACTIONS]|
|  (📜)   | [Icon] Chronicles of Eldoria                      [ ⚙️ ]  [ ▶ PLAY ]   |
|  (⚙️)   |        World: Eldoria • System: D20 Fantasy                             |
+-----------------------------------------------------------------------------------+
```

### 2.1. Left Vertical Dock (`LauncherDock`)
- **Dimensions & Style:** Fixed `72px` width, translucent dark glass (`bg-stone-950/85 backdrop-blur-xl border-r border-white/10`).
- **Top Section:**
  - `+` (New Campaign) button (`46x46px` squircle with dashed border). Clicking toggles the horizontal `WorldFlyout`.
- **Middle Section (Scrollable):**
  - Vertical list of campaign icons (`48x48px` rounded squircles with `12px` border radius).
  - Active campaign indicator: accented glow, distinct border, and a vertical pill indicator anchored to the left dock edge.
  - Inactive campaigns: subtle dimming (`opacity-70 hover:opacity-100 transition-opacity`).
  - Native/custom tooltip on hover showing campaign name and last played date.
- **Flex Spacer (`flex: 1`):** Keeps bottom tools anchored cleanly to the bottom.
- **Bottom Section:**
  - `🌐 Worlds Studio` button: Opens Worlds Studio as a full-window overlay.
  - `📜 Systems Studio` button: Opens Systems Studio as a full-window overlay.
  - `⚙️ Global Settings` button: Opens Settings Studio in a centered modal dialog.

### 2.2. Main Hero Stage (`CampaignHeroStage`)
- **Background Banner:** Full-bleed active campaign banner image (or procedural gradient mesh), with cinematic dark gradient vignettes at top and bottom to ensure text and action contrast.
- **Top-Right Stats Card:**
  - Glassmorphic card (`bg-stone-900/70 backdrop-blur-md border border-white/10 rounded-2xl p-3`).
  - Columns:
    - **Turns Played:** e.g. `42 turns`
    - **Play Time:** cumulative elapsed time or turn estimate (e.g. `2h 15m`)
    - **Last Played:** formatted timestamp (e.g. `Today 21:40` or `Sep 23`)
- **Bottom-Left Title Card:**
  - Translucent glassmorphic pill (`bg-stone-900/80 backdrop-blur-xl border border-white/10 rounded-2xl p-4 flex items-center gap-4`).
  - Left: Campaign icon avatar (`52x52px`).
  - Right: Campaign title (bold sans-serif, `text-xl text-white`), Subtitle (`text-xs text-stone-400`: `World: [Name] • System: [Name] • Hero: [Protagonist]`).
- **Bottom-Right Actions:**
  - **Campaign Settings Button (⚙️):** `52x52px` rounded glass square. Opens `CampaignSettingsModal`.
  - **Primary "▶ PLAY" Button:** Bold gradient pill (`px-9 py-3.5 bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-500 hover:to-indigo-500 text-white font-bold tracking-wide rounded-2xl shadow-xl shadow-purple-600/30`). Clicking transitions into the campaign turn chronicle.

### 2.3. Horizontal World Flyout (`WorldFlyout`)
- Slides out horizontally from the dock at the level of the `+` button when active.
- Translucent dark glass container with subtle border and backdrop blur.
- Contains:
  - Header label: `Select World:`
  - Horizontal scrollable list of world icon tiles (`42x42px` squircles with genre glyphs / icons).
  - Hover tooltip displaying full `World Name (Genre)`.
  - Trailing `+ Create World` tile linking directly to Worlds Studio.
- Clicking any world tile closes the flyout and immediately opens the centered `NewCampaignModal`.

---

## 3. Asset & Image Architecture

Both Campaigns and Worlds use a standardized 2-image structure:
1. **Banner Image:**
   - Campaign: Full-screen background (`16:9` widescreen ratio).
   - World: Header image for the new campaign dialog (`16:9` or `21:9` landscape).
2. **Icon Image:**
   - Campaign: Square avatar emblem (`1:1` ratio, `128x128` to `512x512`).
   - World: Square avatar emblem (`1:1` ratio).

### 3.1. On-Disk Asset Paths
- Campaigns: `games/<id>/assets/banner.png` (or `.webp`/`.jpg`/`.svg`) and `games/<id>/assets/icon.png`.
- Worlds: `worlds/<id>/assets/banner.png` and `worlds/<id>/assets/icon.png`.

### 3.2. Procedural Fallback Engine (`ProceduralAsset`)
When no custom file exists on disk:
- **Hashing:** Computes a deterministic FNV-1a 32-bit hash from the entity's ID, name, and genre/tags.
- **Procedural Banners:**
  - Selects an atmospheric color theme from curated palettes (Arcane Violet, Midnight Obsidian, Ancient Deepwood, Dragon Flame Amber, Eldritch Teal, Cobalt Sky).
  - Renders multi-stop radial and angular CSS gradient mesh with film grain noise.
- **Procedural Icons:**
  - Gradient background matching the theme.
  - Themed Lucide glyph based on genre keyword match (e.g. `Shield` for fantasy, `Rocket` for sci-fi, `Skull` for horror, `Cpu` for cyberpunk) or a clean 2-letter uppercase monogram in modern sans-serif typography.

### 3.3. Asset Ingestion: "Upload Image" & "Generate with AI"
Both `CampaignSettingsModal` and `NewCampaignModal` provide:
- **Upload File Button:** Triggers standard file picker / drag-and-drop (`image/png`, `image/jpeg`, `image/webp`, `image/svg+xml`). Uploads via `POST /api/.../banner` or `POST /api/.../icon`.
- **Generate AI Art Button:** Triggers backend `POST /api/.../generate-asset` with `kind: "banner" | "icon"`. Synthesizes a prompt from the world/campaign metadata and art style, calling the configured `media.SceneImageClient`. Saves directly to `assets/` and refreshes the UI.

---

## 4. Backend Endpoints & Data Model

### 4.1. DTO Updates (`pkg/gui/types.go` & `frontend/src/types.ts`)
```go
type GameSummaryDTO struct {
    ID              string `json:"id"`
    Name            string `json:"name"`
    SystemID        string `json:"system_id"`
    WorldID         string `json:"world_id"`
    PlayerName      string `json:"player_name"`
    TurnCount       int    `json:"turn_count"`
    LastPlayed      string `json:"last_played"`
    BannerURL       string `json:"banner_url,omitempty"`
    IconURL         string `json:"icon_url,omitempty"`
    PlayTimeSeconds int64  `json:"play_time_seconds,omitempty"`
}

type WorldSummaryDTO struct {
    ID                string   `json:"id"`
    Name              string   `json:"name"`
    Description       string   `json:"description"`
    Genre             string   `json:"genre"`
    CompatibleSystems []string `json:"compatible_systems"`
    BannerURL         string   `json:"banner_url,omitempty"`
    IconURL           string   `json:"icon_url,omitempty"`
}
```

### 4.2. API Routes (`pkg/gui/server.go`)
- `GET /api/game/{id}/banner` & `GET /api/game/{id}/icon`
- `POST /api/game/{id}/banner` & `POST /api/game/{id}/icon` (multipart/form-data upload)
- `POST /api/game/{id}/generate-asset` (JSON `{ kind: "banner" | "icon", prompt?: string }`)
- `GET /api/world/{id}/banner` & `GET /api/world/{id}/icon`
- `POST /api/world/{id}/banner` & `POST /api/world/{id}/icon` (multipart/form-data upload)
- `POST /api/world/{id}/generate-asset` (JSON `{ kind: "banner" | "icon", prompt?: string }`)

---

## 5. Modal Dialogs vs Full-Window Overlays

1. **Full-Window Overlays:**
   - **Worlds Studio** & **Systems Studio**: Given their complex multi-column layouts, markdown previewers, entity graphs, and rules hook editors, clicking them in the bottom dock renders them as full-window views with a prominent "← Back to Launcher" top navigation button.
2. **Centered Glassmorphic Modals:**
   - **New Campaign Dialog (`NewCampaignModal`):** Shows world banner header, world icon badge, campaign name, system selector, character answers, and art generation.
   - **Campaign Settings Dialog (`CampaignSettingsModal`):** Shows banner/icon upload and AI generation, general name/protagonist info, and danger zone (restart/delete).
   - **Global Settings Dialog (`SettingsStudio`):** Centered modal covering audio, LLM models, and media providers.

---

## 6. Zero-State Handling

- **Case 1: No Campaigns, but Worlds Exist:**
  - Hero stage displays a clean atmospheric gradient with a centered call-to-action: *"Choose a world to begin your adventure"*.
  - The `WorldFlyout` automatically opens and highlights available worlds.
- **Case 2: No Campaigns AND No Worlds Exist (Fresh Install):**
  - Hero stage displays a prominent onboarding card: *"Welcome to LocalRPG. Create your first world or import a rules system to begin."*
  - Provides quick-action buttons: `[ + Create First World ]` (opens Worlds Studio) and `[ 📜 Browse Systems ]` (opens Systems Studio).

---

## 7. In-Game Transition & Navigation

- In-Game gameplay (`App.tsx` mounting `ChronicleView`, `ActionConsole`, and drawers) remains a **dedicated full-bleed tabletop experience**.
- The left dock is hidden during gameplay to maximize focus on narrative text, scene art, and action input.
- In the top header of the gameplay interface, the "← Campaigns" button returns smoothly to the launcher at any time, saving all game state.

---

## 8. Verification Plan

1. **Backend Tests:**
   - Unit tests for asset resolution (`GameBannerPath`, `WorldBannerPath`, `GameIconPath`, `WorldIconPath`).
   - Unit tests for `ListGames` and `ListWorlds` populating `BannerURL` and `IconURL` when files exist.
   - Route tests for `/api/game/{id}/banner`, `/api/world/{id}/banner`, upload handling, and AI asset generation endpoints.
2. **Frontend Tests & Typechecks:**
   - `npx tsc --noEmit` verifies strict types across all new launcher components and DTOs.
   - Verify deterministic procedural gradient and icon generation for various ID strings.
3. **End-to-End Verification:**
   - Verify smooth switching between campaigns via left dock.
   - Verify `+` toggles world flyout with tooltips.
   - Verify clicking a world opens `NewCampaignModal` with world banner and compatible systems.
   - Verify custom upload and AI generation save files to disk and update UI.
   - Verify zero states for empty campaigns and empty worlds.
   - Full build verification via `mise run test` and `mise run build`.
