# Design Specification: Systems Workshop & Worlds Studio

**Date:** 2026-09-20  
**Status:** Approved  
**Topic:** Dedicated In-App Authoring for Rule Systems, World Settings, and Starter Lore Entities

---

## 1. Overview & Objectives

LocalRPG is built around decoupled, modular components:
- **Systems** define rule mechanics, dice formulas, and calculations (`system.yaml` and `mechanics.js`).
- **Worlds** define setting lore, genre, art style prompts, and starter entity templates (`world.yaml` and `entities/*.md`).
- **Games** combine a system, a world, and a protagonist into an active campaign.

This specification designs full creative studios within the Launcher Hub for authoring rule systems and world settings from scratch:
1. **Rule Systems Workshop:** Master-detail UI for creating and editing systems, setting versioning, and editing `mechanics.js` code.
2. **Worlds Studio:** Master-detail UI for crafting settings, defining genres and art styles, and drafting starter Markdown lore entities (`entities/*.md`) with frontmatter.
3. **End-to-End Game Creation:** Complete seamless integration between authoring a system/world and immediately selecting them in the "New Campaign" wizard to play.
4. **Backend Authoring APIs:** REST endpoints for creating, updating, and fetching system and world files, scripts, and entities.

---

## 2. Architecture & Data Flow

```
                      ┌─────────────────────────────────┐
                      │    Launcher Hub Navigation      │
                      └───────────────┬─────────────────┘
                                      │
              ┌───────────────────────┼───────────────────────┐
              ▼                       ▼                       ▼
       [Campaigns Tab]        [Rule Systems Tab]      [Worlds Studio Tab]
     ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
     │ • Saved Games   │     │ • Systems List  │     │ • Worlds List   │
     │ • Resume Banner │     │ • Manifest Info │     │ • Setting Lore  │
     │ • New Game Modal│     │ • mechanics.js  │     │ • Starter Entity│
     └────────▲────────┘     │   Code Editor   │     │   MD Editor     │
              │              └────────┬────────┘     └────────┬────────┘
              │                       │                       │
              └─────────────── Discovered for Play ───────────┘
```

---

## 3. Backend APIs (`pkg/gui`)

### 3.1 Rule Systems Endpoints
- **`GET /api/system/:id`**:
  - Returns `SystemDetailDTO`: `id`, `name`, `version`, `description`, `script` (contents of `mechanics.js`).
- **`POST /api/systems`** & **`PUT /api/system/:id`**:
  - Accepts `CreateSystemRequestDTO`: `id`, `name`, `version`, `description`, `script`.
  - Writes `systems/<id>/system.yaml` and `systems/<id>/mechanics.js`.
  - Default starter script provided if omitted.

### 3.2 Worlds Studio Endpoints
- **`GET /api/world/:id`**:
  - Returns `WorldDetailDTO`: `id`, `name`, `description`, `genre`, `default_system`, `art_style`, `tags`, and list of `entities` (`id`, `name`, `type`).
- **`POST /api/worlds`** & **`PUT /api/world/:id`**:
  - Accepts `CreateWorldRequestDTO`: `id`, `name`, `description`, `genre`, `default_system`, `art_style`, `tags`.
  - Writes `worlds/<id>/world.yaml` and creates `worlds/<id>/entities/` directory.
- **`GET /api/world/:id/entity/:entity_id`**:
  - Returns `{ "id": "...", "markdown": "..." }`.
- **`PUT /api/world/:id/entity/:entity_id`**:
  - Writes `worlds/<id>/entities/<entity_id>.md`.
- **`DELETE /api/world/:id/entity/:entity_id`**:
  - Deletes `worlds/<id>/entities/<entity_id>.md`.

---

## 4. Frontend UI Components

### 4.1 Top Navigation Tabs in `LauncherHub.tsx`
The Launcher Hub header features three tab pills:
1. **Campaigns:** The game launcher grid and resume hero card.
2. **Rule Systems:** Mounts `<SystemsStudio />`.
3. **Worlds Studio:** Mounts `<WorldsStudio />`.

### 4.2 `SystemsStudio.tsx`
- **Left Panel:** List of all discovered systems, version tags, description preview, and **"+ New System"** button.
- **Right Panel:**
  - **Manifest Sub-Tab:** Form fields for System Name, Slug ID, Version, and Description.
  - **Mechanics Script Sub-Tab:** Monospace script editor (`JetBrains Mono`) for editing `mechanics.js` with syntax formatting and starter boilerplate.
  - **Save Action:** Golden amber pill button saving changes to disk.

### 4.3 `WorldsStudio.tsx`
- **Left Panel:** List of all discovered worlds with genre tags, entity counts, and **"+ New World"** button.
- **Right Panel:**
  - **Lore & Atmosphere Sub-Tab:** Form fields for Title, Slug ID, Genre, Default System dropdown, Visual Art Style prompt guide, and World Lore synopsis.
  - **Starter Entities Sub-Tab:**
    - List of template entities with **"+ Add Starter Entity"** button.
    - Markdown editor with YAML frontmatter for drafting locations, factions, and artifacts.
  - **Save Action:** Golden amber pill button saving changes to disk.

---

## 5. Verification & Testing Plan

1. **Backend Unit Tests (`pkg/gui/server_test.go`):**
   - Test `GET /api/system/:id` and `PUT /api/system/:id` (writes and reads `system.yaml` and `mechanics.js`).
   - Test `GET /api/world/:id`, `PUT /api/world/:id`, `PUT /api/world/:id/entity/:name`, and `GET /api/world/:id/entity/:name`.
   - Test end-to-end: Create custom system, create custom world with entity, create game using them via `POST /api/games`, verify game initialized with world entity templates.
2. **Frontend Type Checking & Build:**
   - Verify TypeScript compiles with 0 errors via `mise run test:frontend`.
   - Verify bundle builds via `mise run build:frontend`.
3. **Full Suite Regression:**
   - Execute `mise run test` across all 12 Go packages and frontend checks.
