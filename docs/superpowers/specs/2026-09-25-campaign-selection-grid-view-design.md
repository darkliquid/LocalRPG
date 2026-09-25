# Campaign Selection Grid View Design

- **Date:** 2026-09-25
- **Status:** Approved
- **Scope:** Launcher UI (`frontend/src/components/launcher/`, `LauncherHub.tsx`)
- **Related:** `docs/superpowers/specs/2026-09-23-twintail-launcher-redesign-design.md`, `LauncherDock.tsx`, `WorldGallery.tsx`, `CampaignHeroStage.tsx`

---

## 1. Overview & Goals

In the current desktop launcher, worlds can be browsed either through a horizontal flyout (`WorldFlyout`) or expanded into a full-window visual grid (`WorldGallery`). In contrast, campaigns are only accessible as a compact vertical column of 46x46 square icons on the left dock (`LauncherDock`). As players accumulate multiple campaigns across different worlds and systems, selecting a campaign from a column of small icons lacks visibility into campaign progress, world/system context, and last-played activity.

This specification adds an expanded **Campaign Gallery** view to the launcher hub, directly mirroring the UX, visual fidelity, and conventions of the existing `WorldGallery`.

### 1.1 Goals

1. **Dedicated Entry Point**: Add an anchored grid button (`LayoutGrid`) to the launcher dock immediately below the New Campaign (`+`) button and divider.
2. **Full-Window Campaign Gallery**: Provide a responsive visual grid overlay displaying rich campaign cards with banner art, icon avatar, campaign name, world & system badges, turn count, and last played time.
3. **Instant Launch & Direct Actions**: Clicking anywhere on a campaign card instantly launches the campaign (`onPlay`), with dedicated direct buttons for Settings (`onOpenSettings`) and a shortcut to create a new campaign.
4. **Search & Sorting**: Provide client-side instant search (by campaign name, player name, world name, and system name) and sorting (Most Recently Played, Alphabetical, Turn Count).
5. **Procedural Fallbacks**: Gracefully handle missing banner and icon assets using `ProceduralBanner` and `ProceduralIcon`.

### 1.2 Non-Goals

1. Modifying backend data structures: `GameSummary` already contains `id`, `name`, `system_id`, `world_id`, `player_name`, `turn_count`, `last_played`, `banner_url`, and `icon_url`. No backend API changes are required.
2. Altering game launch or campaign deletion semantics: existing `onSelectGame` and `onOpenCampaignSettings` callbacks in `LauncherHub` are reused directly.

---

## 2. Component Architecture

### 2.1 Component Hierarchy

```
LauncherHub (manages isCampaignGalleryOpen state)
├── LauncherDock
│   ├── New Campaign Button (+) -> onToggleFlyout (opens WorldFlyout)
│   ├── Campaign Grid Button (LayoutGrid) -> onToggleCampaignGallery (opens CampaignGallery)
│   ├── Divider
│   ├── Scrollable Campaign Icons List
│   └── Bottom Utility Icons (Worlds Studio, Systems Studio, Settings)
├── CampaignGallery (Full-window overlay when isCampaignGalleryOpen is true)
│   ├── Header (Title, Count badge, Search input, Sort select, New Campaign button, Close button)
│   ├── Cards Grid
│   │   ├── CampaignCard (repeated for each matching game)
│   │   │   ├── Banner (img or ProceduralBanner)
│   │   │   ├── Icon (img or ProceduralIcon)
│   │   │   ├── Title, World & System badges
│   │   │   ├── Turn count & relative last played timestamp
│   │   │   └── Actions: Instant launch on click, Settings gear button
│   │   └── Create New Campaign Card (triggers world selection)
│   └── Empty State (when 0 campaigns exist or search returns 0 matches)
├── WorldFlyout & WorldGallery
├── CampaignHeroStage
└── Modals (NewCampaignModal, CampaignSettingsModal, Global Settings)
```

### 2.2 File Modifications & Additions

1. **`frontend/src/components/launcher/CampaignGallery.tsx`** (New File):
   - Implements full-window gallery overlay and `CampaignCard`.
   - Prop interface:
     ```typescript
     interface CampaignGalleryProps {
       isOpen: boolean;
       games: GameSummary[];
       worlds: WorldInfo[];
       systems: SystemInfo[];
       onPlayGame: (gameId: string) => void;
       onOpenSettings: (gameId: string) => void;
       onCreateCampaign: () => void;
       onClose: () => void;
     }
     ```
2. **`frontend/src/components/launcher/LauncherDock.tsx`**:
   - Add props:
     ```typescript
     isCampaignGalleryOpen: boolean;
     onToggleCampaignGallery: () => void;
     ```
   - Render `LayoutGrid` icon button right under `+` button and divider.
   - Active highlight styling when `isCampaignGalleryOpen` is true.
3. **`frontend/src/components/LauncherHub.tsx`**:
   - Add state: `const [isCampaignGalleryOpen, setIsCampaignGalleryOpen] = useState(false);`
   - Wire `isCampaignGalleryOpen` and toggle handler to `LauncherDock`.
   - Render `CampaignGallery` with `onPlayGame={(id) => { setIsCampaignGalleryOpen(false); onSelectGame(id); }}`, `onOpenSettings={(id) => { setIsCampaignGalleryOpen(false); setSettingsGameID(id); }}`, and `onCreateCampaign={() => { setIsCampaignGalleryOpen(false); setIsFlyoutOpen(true); }}`.

---

## 3. Data Flow & User Interaction

### 3.1 Filtering & Sorting

- **Search**:
  Filters games case-insensitively where query matches:
  - `game.name`
  - `game.player_name`
  - `world.name` (resolved from `worlds` by `game.world_id`)
  - `system.name` (resolved from `systems` by `game.system_id`)
- **Sort Options**:
  - `recent` (default): `new Date(b.last_played).getTime() - new Date(a.last_played).getTime()`
  - `name`: `a.name.localeCompare(b.name)`
  - `turns`: `b.turn_count - a.turn_count`

### 3.2 Card Layout & Actions

Each card displays:
- **Banner Area**:
  - 16:9 or fixed height (e.g. 140px) banner with gradient overlay.
  - Falls back to `<ProceduralBanner id={game.id} name={game.name} />`.
  - Icon avatar (48x48 rounded-xl) overlay at bottom left, with `<ProceduralIcon id={game.id} name={game.name} />` fallback.
  - Top right badges: `Turn X` badge and `⚙` Settings icon button.
- **Card Body**:
  - Campaign name (bold, truncate).
  - World name & System name tags with subtle accent color.
  - Player character name (if present).
  - Bottom row: relative time since last played (e.g., "2 hours ago", "Yesterday", "3 days ago") and hover pill `"▶ Play"`.
- **Interaction**:
  - Clicking the card invokes `onPlayGame(game.id)`.
  - Clicking the Settings gear icon stops propagation and invokes `onOpenSettings(game.id)`.
  - Hover states: scale-102 transition, purple border glow (`hover:border-purple-400/70`), shadow elevation.

### 3.3 Keyboard Navigation & Dismissal

- Pressing `Escape` or clicking the top-right `X` button closes the gallery without changing the selected campaign.
- Clicking the backdrop or clicking "Back to Launcher" closes the gallery.

---

## 4. Verification & Testing

1. **Component Verification**:
   - `npm run build:frontend` / `npx tsc --noEmit` verifies strict TypeScript types.
2. **Behavioral Scenarios**:
   - With 0 campaigns, opening gallery shows empty state with button to create a campaign.
   - With multiple campaigns, opening gallery displays all campaigns ordered by most recently played.
   - Searching by campaign name, character name, world, or system narrows the grid in real time.
   - Clicking a card launches the game into play mode.
   - Clicking the settings gear opens `CampaignSettingsModal`.
