# Design Specification: GUI Launcher Hub & Campaign Creator

**Date:** 2026-09-20  
**Status:** Approved  
**Topic:** Twintail-Style Launcher Hub Landing Page, Campaign Discovery, and Quick-Start Creation Wizard

---

## 1. Overview & Objectives

Currently, opening the LocalRPG desktop GUI directly mounts the Chronicle view with a hardcoded test game ID (`test-campaign`). If no games exist or if the player has multiple campaigns, there is no landing interface to view saved games or start a new adventure.

This specification designs a **Launcher Hub Landing Page** styled after the Twintail Launcher glassmorphic dashboard:
1. **Atmospheric Landing Hub:** A full-window dashboard displaying saved campaigns with world/system badges, protagonist details, turn count, and a prominent "Resume Adventure" hero panel.
2. **Quick-Start Campaign Wizard:** A modal creation flow allowing the player to name a campaign, pick from discovered systems and worlds, specify their protagonist's name, and immediately initialize and enter the game.
3. **Seamless State Lifecycle:** Dynamic switching between the Launcher Hub and In-Game Chronicle view with `localStorage` persistence and an in-game "Campaigns / Home" header action.
4. **Backend Discovery APIs:** REST endpoints for listing games (`GET /api/games`), systems (`GET /api/systems`), worlds (`GET /api/worlds`), and provisioning new campaigns (`POST /api/games`).

---

## 2. Architecture & Data Flow

```
                      ┌────────────────────────┐
                      │    LocalRPG GUI App    │
                      └───────────┬────────────┘
                                  │
                  ┌───────────────┴───────────────┐
                  ▼                               ▼
       activeGameID == null             activeGameID != null
                  │                               │
                  ▼                               ▼
          <LauncherHub />                  <InGameView />
     ┌────────────────────────┐       ┌────────────────────────┐
     │ • Resume Hero Banner   │       │ • Floating Glass Header│
     │ • Saved Games Grid     │       │   ("Campaigns" Button) │
     │ • "New Campaign" Wizard│       │ • Chronicle & Actions  │
     └────────────┬───────────┘       │ • Codex, Graph, Drawers│
                  │                   └────────────────────────┘
                  │ onSelectGame(id)
                  └───────────────────────────────▲
```

---

## 3. Backend Discovery & Provisioning APIs (`pkg/gui`)

The following endpoints will be added to `pkg/gui/server.go` and serviced by `pkg/gui/service.go`:

### 3.1 `GET /api/games`
Scans the `games/` root directory. For each subdirectory with a `game.yaml`, reads the manifest and checks the SQLite cache for turn history.
**Response (200 OK):**
```json
[
  {
    "id": "shadows-over-arkham",
    "name": "Shadows Over Arkham",
    "system_id": "daggerheart",
    "world_id": "solitary_defiance",
    "player_name": "Elena Nightshade",
    "turn_count": 14,
    "last_played": "2026-09-20T18:00:00Z",
    "thumbnail_url": ""
  }
]
```

### 3.2 `GET /api/systems`
Scans the `systems/` directory for `system.yaml` manifests.
**Response (200 OK):**
```json
[
  {
    "id": "daggerheart",
    "name": "Daggerheart",
    "description": "Narrative fantasy roleplaying engine with dual d12 dice mechanics.",
    "version": "1.0.0"
  }
]
```

### 3.3 `GET /api/worlds`
Scans the `worlds/` directory for `world.yaml` manifests.
**Response (200 OK):**
```json
[
  {
    "id": "solitary_defiance",
    "name": "Solitary Defiance",
    "description": "A gothic fantasy city besieged by twilight horrors.",
    "genre": "Gothic Fantasy",
    "compatible_systems": ["daggerheart"]
  }
]
```

### 3.4 `POST /api/games`
Accepts a JSON payload to initialize a new campaign:
```json
{
  "name": "The Sunken Cathedral",
  "system_id": "daggerheart",
  "world_id": "solitary_defiance",
  "player_name": "Father Lucian"
}
```
- Validates that `system_id` and `world_id` exist.
- Generates a sanitized URL-safe directory slug if `id` is omitted (e.g. `the-sunken-cathedral`).
- Calls `engine.InitGame(paths, id, systemID, worldID, playerName)`.
- Returns `201 Created` with the newly created `GameSummaryDTO`.

---

## 4. Frontend Components & User Experience

### 4.1 `LauncherHub.tsx`
- **Atmospheric Dark Backdrop:** Preserves the Twintail Launcher full-window vignette with dynamic noise overlay.
- **Top Brand Bar:** "LocalRPG" in Cinzel gold font with glowing amber status indicator.
- **Hero "Resume Adventure" Panel:** If at least one campaign exists, the most recently played campaign is highlighted in a frosted glass card with a prominent golden **"Resume Adventure"** button (`Play` icon), current turn count, world name, and character identity.
- **Saved Chronicles Grid:** A responsive grid (`grid-cols-1 md:grid-cols-2 lg:grid-cols-3`) of frosted glass cards displaying:
  - Campaign name.
  - Setting and System pills (e.g. `[Solitary Defiance]`, `[Daggerheart]`).
  - Character name (`User` icon) and turn count (`Clock` icon).
  - Hover glow effect (`hover:border-amber-500/50 hover:scale-[1.01]`).
- **"New Campaign" Button:** Amber pill button triggering the creation wizard.
- **Empty State:** If no campaigns exist, renders a welcoming central card inviting the player to create their first adventure.

### 4.2 Quick-Start Creation Wizard Modal
- Modal dialog rendered over the hub.
- Fields:
  - **Campaign Name:** Text input with dynamic slug preview.
  - **System:** Select dropdown loaded from `/api/systems`.
  - **World:** Select dropdown loaded from `/api/worlds` with lore excerpt.
  - **Player Character Name:** Text input (e.g. "Elena Nightshade").
- Actions: **Cancel** and **Embark on Adventure**.
- On submission: Calls `APIClient.createGame`, updates campaign state, and launches directly into the Chronicle.

### 4.3 In-Game Header Integration (`App.tsx`)
- In the floating glass header of the active game view, adds a **"Campaigns" / Home** button (`Compass` icon).
- Clicking it sets `activeGameID(null)`, returning the player to `<LauncherHub />` without reloading the application.

---

## 5. Verification & Testing Plan

1. **Backend Unit & Integration Tests (`pkg/gui/server_test.go`):**
   - Test `GET /api/games` returns list of campaigns.
   - Test `GET /api/systems` and `GET /api/worlds` returns valid manifests.
   - Test `POST /api/games` provisions new campaign directory and returns `201 Created`.
2. **Frontend Type Checking:**
   - Verify TypeScript type definitions for `GameSummary`, `SystemInfo`, `WorldInfo`, and `CreateGamePayload`.
   - Run `npx tsc --noEmit`.
3. **End-to-End Build & Run Verification:**
   - Execute `mise run test`.
   - Verify bundle build with `mise run build`.
