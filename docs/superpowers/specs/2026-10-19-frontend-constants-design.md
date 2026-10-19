# Share the Frontend Constants Design

**Date:** 2026-10-19  
**Status:** Proposed  
**Bead:** LocalRPG-14a.9  
**External Ref:** [#188](https://github.com/darkliquid/LocalRPG/issues/188)  
**Epic:** LocalRPG-14a ([#152](https://github.com/darkliquid/LocalRPG/issues/152))  
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (FE-10, finding F-10)  
**Scope:** `frontend/src/lib/constants.ts`, `frontend/src/api/client.ts`, `frontend/src/types.ts`, and consumer components/hooks. No behaviour change.

## 1. Problem

Finding F-10 in the quality review identified several classes of hardcoded strings, magic numbers, and duplicate types scattered across the frontend codebase:

1. **API route strings outside `APIClient`:**
   - Character portrait URLs (`/api/game/.../character/.../portrait`) are constructed ad-hoc across `CodexDrawer.tsx`, `StoryTheater.tsx`, and `useCharacterPortraits.ts`.
   - World artwork URLs (`/api/world/.../banner`, `/api/world/.../icon`) are formatted manually in `WorldsStudio.tsx`.
   - Audio Server-Sent Events endpoint (`/api/audio/events`) is hardcoded in `CampaignContext.tsx`.
   - These bypass the invariant that `frontend/src/api/client.ts` is the sole authority for API routing and endpoints.

2. **Scattered `localStorage` keys:**
   - Keys `'localrpg_active_game'` and `'localrpg_last_played_game'` are hardcoded across `LauncherHub.tsx` and `CampaignContext.tsx`.
   - Key `'localrpg.settings.advanced'` is hardcoded in `useAdvanced.ts` and `useAdvanced.test.ts`.
   - There is no central registry for storage keys.

3. **Repeated package file extension strings:**
   - Package extensions (`.lrpgsystem`, `.lrpgworld`, `.lrpgpack`) and file input filter strings (e.g., `accept=".lrpgworld,.lrpgpack"`) are duplicated across `SystemsStudio.tsx`, `WorldsStudio.tsx`, `LauncherHub.tsx`, `SystemHeader.tsx`, `SystemList.tsx`, `WorldHeader.tsx`, and `WorldList.tsx`.

4. **Magic media defaults:**
   - Model ID `'kokoro-tts'` and Kokoro model size `90177536` bytes are repeated across `SettingsStudio.tsx` and `CampaignContext.tsx`.
   - The offline fallback model list `GEMINI_AGENT_MODELS` is defined locally inside `LLMRoleEditor.tsx`.

5. **Duplicate display-mode union:**
   - The union `'stage_directions' | 'hidden' | 'raw'` is repeated in `frontend/src/types.ts`, `MarkdownProse.tsx`, `TurnSegments.tsx`, `TheaterDialogue.tsx`, and `story/types.ts` instead of referencing a shared named type.

## 2. Goals

- Encapsulate all API route URL generation inside static helper methods on `APIClient`.
- Centralize `localStorage` keys, package extensions, file picker accept filters, media defaults, and model fallbacks in a single shared module: `frontend/src/lib/constants.ts`.
- Export a named type `StageDirectionsDisplayMode` from `frontend/src/types.ts` and use it across all consumers.
- Maintain 100% behavioural backward compatibility.
- Ensure all frontend and backend tests and lint checks pass cleanly.

## 3. Non-goals

- Refactoring the internal HTTP client implementation or converting endpoints to zod/runtime validation (covered by FE-5 / finding F-5).
- Modifying backend routes or API schemas.
- Changing `localStorage` key names (which would discard existing user settings on upgrade).

## 4. Design

### 4.1 Route Helper Methods on `APIClient` (`frontend/src/api/client.ts`)

Add static methods to `APIClient` that encapsulate path formatting and query parameters:

```ts
// characterPortraitURL formats the endpoint URL for an entity portrait image.
static characterPortraitURL(gameID: string, characterID: string, timestamp?: string | number): string {
  const base = `/api/game/${encodeURIComponent(gameID)}/character/${encodeURIComponent(characterID)}/portrait`;
  return timestamp ? `${base}?t=${encodeURIComponent(String(timestamp))}` : base;
}

// worldAssetURL formats the endpoint URL for a world banner or icon image.
static worldAssetURL(worldID: string, kind: 'banner' | 'icon', timestamp?: string | number): string {
  const base = `/api/world/${encodeURIComponent(worldID)}/${kind}`;
  return timestamp ? `${base}?t=${encodeURIComponent(String(timestamp))}` : base;
}

static worldBannerURL(worldID: string, timestamp?: string | number): string {
  return APIClient.worldAssetURL(worldID, 'banner', timestamp);
}

static worldIconURL(worldID: string, timestamp?: string | number): string {
  return APIClient.worldAssetURL(worldID, 'icon', timestamp);
}

// audioEventsURL returns the Server-Sent Events endpoint URL for audio playback.
static audioEventsURL(): string {
  return '/api/audio/events';
}
```

### 4.2 Shared Constants Module (`frontend/src/lib/constants.ts`)

Create `frontend/src/lib/constants.ts` declaring typed constants:

```ts
import type { StageDirectionsDisplayMode } from '../types';

// Storage keys used across local session and settings persistence.
export const STORAGE_KEYS = {
  ACTIVE_GAME: 'localrpg_active_game',
  LAST_PLAYED_GAME: 'localrpg_last_played_game',
  SETTINGS_ADVANCED: 'localrpg.settings.advanced',
} as const;

// Package file extensions and file-picker filter strings.
export const PACKAGE_EXTENSIONS = {
  SYSTEM: '.lrpgsystem',
  WORLD: '.lrpgworld',
  PACK: '.lrpgpack',
} as const;

export const PACKAGE_ACCEPT = {
  SYSTEM: '.lrpgsystem,.lrpgpack',
  WORLD: '.lrpgworld,.lrpgpack',
  ALL: '.lrpgworld,.lrpgsystem,.lrpgpack',
} as const;

// Default media model configurations.
export const DEFAULT_KOKORO_MODEL_ID = 'kokoro-tts';
export const DEFAULT_KOKORO_SIZE_BYTES = 90_177_536;

// Offline fallback model suggestions for Gemini agents.
export const DEFAULT_GEMINI_AGENT_MODELS: readonly string[] = [
  'gemini-3.8-flash',
  'gemini-3.7-flash',
  'gemini-3.5-flash',
  'gemini-3.5-flash-lite',
  'gemini-3.1-flash-lite',
  'gemini-3.1-pro-preview',
  'gemini-3-flash-preview',
  'gemini-2.5-flash',
  'gemini-2.5-pro',
  'gemini-2.5-flash-lite',
  'gemma-4-31b-it',
  'gemma-4-26b-it',
  'gemma-4-12b-it',
  'gemma-3-27b-it',
];

export const DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE: StageDirectionsDisplayMode = 'stage_directions';
```

### 4.3 Type Centralization (`frontend/src/types.ts`)

Export the named union type:
```ts
export type StageDirectionsDisplayMode = 'stage_directions' | 'hidden' | 'raw';
```
Update `SpeechCuesConfig`:
```ts
export interface SpeechCuesConfig {
  enabled: boolean;
  audio_tags?: boolean;
  markdown_emphasis?: boolean;
  display_mode?: StageDirectionsDisplayMode;
}
```

### 4.4 Consumer Migrations

1. **Route Call Sites:**
   - `frontend/src/components/CodexDrawer.tsx`: Replace inline `/api/game/...` template with `APIClient.characterPortraitURL(gameID, entity.id, entity.updated_at)`.
   - `frontend/src/components/StoryTheater.tsx`: Replace inline `/api/game/...` template with `APIClient.characterPortraitURL(gameId, playerId)`.
   - `frontend/src/hooks/useCharacterPortraits.ts`: Replace inline `/api/game/...` template with `APIClient.characterPortraitURL(activeGameID || '', entity.id)`.
   - `frontend/src/components/WorldsStudio.tsx`: Replace `/api/world/${encodeURIComponent(id)}/banner...` with `APIClient.worldBannerURL(id, Date.now())` and `APIClient.worldIconURL(id, Date.now())`.
   - `frontend/src/context/CampaignContext.tsx`: Replace `new EventSource('/api/audio/events')` with `new EventSource(APIClient.audioEventsURL())`.

2. **Storage Key Call Sites:**
   - `frontend/src/components/LauncherHub.tsx`: Replace `'localrpg_active_game'` and `'localrpg_last_played_game'` with `STORAGE_KEYS.ACTIVE_GAME` and `STORAGE_KEYS.LAST_PLAYED_GAME`.
   - `frontend/src/context/CampaignContext.tsx`: Replace `'localrpg_active_game'` and `'localrpg_last_played_game'` with `STORAGE_KEYS.ACTIVE_GAME` and `STORAGE_KEYS.LAST_PLAYED_GAME`.
   - `frontend/src/hooks/useAdvanced.ts` and `frontend/src/hooks/useAdvanced.test.ts`: Replace `'localrpg.settings.advanced'` with `STORAGE_KEYS.SETTINGS_ADVANCED`.

3. **Package Extension Call Sites:**
   - `frontend/src/components/SystemsStudio.tsx`: Use `PACKAGE_EXTENSIONS.SYSTEM` for default filename and filter patterns.
   - `frontend/src/components/WorldsStudio.tsx`: Use `PACKAGE_EXTENSIONS.WORLD` for default filename and filter patterns.
   - `frontend/src/components/studios/system/SystemList.tsx` & `SystemHeader.tsx`: Use `PACKAGE_ACCEPT.SYSTEM`.
   - `frontend/src/components/studios/world/WorldList.tsx` & `WorldHeader.tsx`: Use `PACKAGE_ACCEPT.WORLD`.
   - `frontend/src/components/LauncherHub.tsx`: Use `PACKAGE_ACCEPT.ALL`.

4. **Media and Model Defaults:**
   - `frontend/src/components/SettingsStudio.tsx` & `frontend/src/context/CampaignContext.tsx`: Use `DEFAULT_KOKORO_MODEL_ID` and `DEFAULT_KOKORO_SIZE_BYTES`.
   - `frontend/src/components/providers/LLMRoleEditor.tsx`: Import and use `DEFAULT_GEMINI_AGENT_MODELS`.

5. **Display Mode Union:**
   - `frontend/src/components/MarkdownProse.tsx`: Import `StageDirectionsDisplayMode` and `DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE`.
   - `frontend/src/components/TurnSegments.tsx`: Import `StageDirectionsDisplayMode` and `DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE`.
   - `frontend/src/components/theater/TheaterDialogue.tsx`: Import `StageDirectionsDisplayMode`.
   - `frontend/src/components/story/types.ts`: Import `StageDirectionsDisplayMode`.

## 5. Testing & Verification

- **`frontend/src/lib/constants.test.ts`**:
  - Test `STORAGE_KEYS` values match the expected persistent strings.
  - Test `PACKAGE_EXTENSIONS` start with `.` and match `.lrpg*`.
  - Test `PACKAGE_ACCEPT` strings contain comma-separated extensions.
  - Test `DEFAULT_KOKORO_MODEL_ID`, `DEFAULT_KOKORO_SIZE_BYTES`, and `DEFAULT_GEMINI_AGENT_MODELS` are valid and non-empty.
- **`frontend/src/api/client.test.ts`**:
  - Test `APIClient.characterPortraitURL`:
    - Base URL without timestamp.
    - URL with string or numeric timestamp (`?t=...`).
    - URL-encoding of gameID and characterID containing spaces, slashes, or special symbols.
  - Test `APIClient.worldAssetURL`, `worldBannerURL`, `worldIconURL`:
    - Base URL without timestamp.
    - URL with timestamp.
    - URL-encoding of worldID.
  - Test `APIClient.audioEventsURL`:
    - Returns `/api/audio/events`.
- **Quality Gates:**
  - `mise run lint:frontend` passes with 0 errors.
  - `mise run test:frontend` passes with 100% of test suites passing.
  - `mise run lint` passes across the repository.
  - `CLEAN=1 mise run test:backend` passes.
