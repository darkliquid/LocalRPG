# Share Frontend Constants Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Centralize hardcoded API routes, `localStorage` keys, package file extensions, media defaults, and display mode unions across the frontend codebase as defined in FE-10.

**Architecture:** Route URL formatting is added as static methods on `APIClient` in `frontend/src/api/client.ts`. Shared domain keys, package extensions, and media defaults are centralized in `frontend/src/lib/constants.ts`. The `StageDirectionsDisplayMode` union is exported from `frontend/src/types.ts` and used across story/theatre components.

**Tech Stack:** TypeScript, React, Vite, Vitest.

---

### Task 1: Centralize `StageDirectionsDisplayMode` Type

**Files:**
- Modify: `frontend/src/types.ts:1247-1253`

- [ ] **Step 1: Export `StageDirectionsDisplayMode` and update `SpeechCuesConfig`**

In `frontend/src/types.ts`:
```ts
export type StageDirectionsDisplayMode = 'stage_directions' | 'hidden' | 'raw';

export interface SpeechCuesConfig {
  enabled: boolean;
  audio_tags?: boolean;
  markdown_emphasis?: boolean;
  display_mode?: StageDirectionsDisplayMode;
}
```

- [ ] **Step 2: Run typecheck to verify change compiles**

Run: `mise run lint:frontend`
Expected: Passes with 0 errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/types.ts
git commit -m "refactor(frontend): export StageDirectionsDisplayMode type (LocalRPG-14a.9)"
```

---

### Task 2: Shared Constants Module & Tests

**Files:**
- Create: `frontend/src/lib/constants.ts`
- Create: `frontend/src/lib/constants.test.ts`

- [ ] **Step 1: Write unit tests for constants**

Create `frontend/src/lib/constants.test.ts`:
```ts
import { describe, expect, it } from 'vitest';
import {
  DEFAULT_GEMINI_AGENT_MODELS,
  DEFAULT_KOKORO_MODEL_ID,
  DEFAULT_KOKORO_SIZE_BYTES,
  DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE,
  PACKAGE_ACCEPT,
  PACKAGE_EXTENSIONS,
  STORAGE_KEYS,
} from './constants';

describe('constants', () => {
  it('defines expected storage keys', () => {
    expect(STORAGE_KEYS.ACTIVE_GAME).toBe('localrpg_active_game');
    expect(STORAGE_KEYS.LAST_PLAYED_GAME).toBe('localrpg_last_played_game');
    expect(STORAGE_KEYS.SETTINGS_ADVANCED).toBe('localrpg.settings.advanced');
  });

  it('defines valid package extensions', () => {
    expect(PACKAGE_EXTENSIONS.SYSTEM).toBe('.lrpgsystem');
    expect(PACKAGE_EXTENSIONS.WORLD).toBe('.lrpgworld');
    expect(PACKAGE_EXTENSIONS.PACK).toBe('.lrpgpack');
  });

  it('defines valid package accept filter strings', () => {
    expect(PACKAGE_ACCEPT.SYSTEM).toBe('.lrpgsystem,.lrpgpack');
    expect(PACKAGE_ACCEPT.WORLD).toBe('.lrpgworld,.lrpgpack');
    expect(PACKAGE_ACCEPT.ALL).toBe('.lrpgworld,.lrpgsystem,.lrpgpack');
  });

  it('defines media defaults', () => {
    expect(DEFAULT_KOKORO_MODEL_ID).toBe('kokoro-tts');
    expect(DEFAULT_KOKORO_SIZE_BYTES).toBe(90_177_536);
  });

  it('defines default stage directions display mode', () => {
    expect(DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE).toBe('stage_directions');
  });

  it('defines non-empty fallback Gemini agent models', () => {
    expect(DEFAULT_GEMINI_AGENT_MODELS.length).toBeGreaterThan(0);
    expect(DEFAULT_GEMINI_AGENT_MODELS).toContain('gemini-2.5-flash');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/lib/constants.test.ts` (inside `frontend/`)
Expected: FAIL ("Cannot find module './constants'")

- [ ] **Step 3: Implement `frontend/src/lib/constants.ts`**

Create `frontend/src/lib/constants.ts`:
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

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/lib/constants.test.ts` (inside `frontend/`)
Expected: PASS (6 tests passed)

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/constants.ts frontend/src/lib/constants.test.ts
git commit -m "feat(frontend): create shared constants module (LocalRPG-14a.9)"
```

---

### Task 3: Static Route URL Builders on `APIClient`

**Files:**
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/api/client.test.ts`

- [ ] **Step 1: Add unit tests for route builders in `client.test.ts`**

In `frontend/src/api/client.test.ts`, append:
```ts
describe('APIClient URL builders', () => {
  it('formats character portrait URLs', () => {
    expect(APIClient.characterPortraitURL('game-1', 'char-2')).toBe(
      '/api/game/game-1/character/char-2/portrait',
    );
    expect(APIClient.characterPortraitURL('game 1', 'char/2', '12345')).toBe(
      '/api/game/game%201/character/char%2F2/portrait?t=12345',
    );
    expect(APIClient.characterPortraitURL('game-1', 'char-2', 98765)).toBe(
      '/api/game/game-1/character/char-2/portrait?t=98765',
    );
  });

  it('formats world asset URLs', () => {
    expect(APIClient.worldAssetURL('world-1', 'banner')).toBe('/api/world/world-1/banner');
    expect(APIClient.worldAssetURL('world 1', 'icon', 'ts-1')).toBe('/api/world/world%201/icon?t=ts-1');
    expect(APIClient.worldBannerURL('world-1')).toBe('/api/world/world-1/banner');
    expect(APIClient.worldBannerURL('world 1', 123)).toBe('/api/world/world%201/banner?t=123');
    expect(APIClient.worldIconURL('world-1')).toBe('/api/world/world-1/icon');
    expect(APIClient.worldIconURL('world 1', 456)).toBe('/api/world/world%201/icon?t=456');
  });

  it('formats audio events URL', () => {
    expect(APIClient.audioEventsURL()).toBe('/api/audio/events');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/api/client.test.ts` (inside `frontend/`)
Expected: FAIL ("APIClient.characterPortraitURL is not a function")

- [ ] **Step 3: Implement static URL builders on `APIClient`**

In `frontend/src/api/client.ts`:
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

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/api/client.test.ts` (inside `frontend/`)
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/client.ts frontend/src/api/client.test.ts
git commit -m "feat(frontend): add static route URL builders to APIClient (LocalRPG-14a.9)"
```

---

### Task 4: Migrate Storage Keys in Consumers

**Files:**
- Modify: `frontend/src/context/CampaignContext.tsx:180-181, 243-244, 332-333`
- Modify: `frontend/src/components/LauncherHub.tsx:51-52, 153-154`
- Modify: `frontend/src/hooks/useAdvanced.ts:6-10, 26`
- Modify: `frontend/src/hooks/useAdvanced.test.ts:17, 29, 33, 37`

- [ ] **Step 1: Update `useAdvanced.ts` and `useAdvanced.test.ts`**

In `frontend/src/hooks/useAdvanced.ts`:
Import `STORAGE_KEYS` from `../lib/constants`:
```ts
import { STORAGE_KEYS } from '../lib/constants';
```
Replace `const STORAGE_KEY = 'localrpg.settings.advanced';` with `STORAGE_KEYS.SETTINGS_ADVANCED`.

In `frontend/src/hooks/useAdvanced.test.ts`:
Import `STORAGE_KEYS` from `../lib/constants` and replace all `'localrpg.settings.advanced'` with `STORAGE_KEYS.SETTINGS_ADVANCED`.

- [ ] **Step 2: Update `LauncherHub.tsx`**

In `frontend/src/components/LauncherHub.tsx`:
Import `STORAGE_KEYS` from `../lib/constants`:
Replace:
```ts
return localStorage.getItem('localrpg_last_played_game') || localStorage.getItem('localrpg_active_game') || null;
```
with:
```ts
return localStorage.getItem(STORAGE_KEYS.LAST_PLAYED_GAME) || localStorage.getItem(STORAGE_KEYS.ACTIVE_GAME) || null;
```
And replace the second occurrence at line 153-154 similarly.

- [ ] **Step 3: Update `CampaignContext.tsx`**

In `frontend/src/context/CampaignContext.tsx`:
Import `STORAGE_KEYS` from `../lib/constants`:
Replace `localStorage.removeItem('localrpg_active_game')` and `localStorage.removeItem('localrpg_last_played_game')` with `STORAGE_KEYS.ACTIVE_GAME` and `STORAGE_KEYS.LAST_PLAYED_GAME`.
Replace `localStorage.setItem('localrpg_last_played_game', gameId)` with `STORAGE_KEYS.LAST_PLAYED_GAME`.

- [ ] **Step 4: Run tests to verify**

Run: `npx vitest run src/hooks/useAdvanced.test.ts src/components/LauncherHub.test.tsx src/context/CampaignContext.test.tsx` (inside `frontend/`)
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/hooks/useAdvanced.ts frontend/src/hooks/useAdvanced.test.ts frontend/src/components/LauncherHub.tsx frontend/src/context/CampaignContext.tsx
git commit -m "refactor(frontend): migrate localStorage keys to STORAGE_KEYS (LocalRPG-14a.9)"
```

---

### Task 5: Migrate Route Call-Sites to `APIClient` Helpers

**Files:**
- Modify: `frontend/src/components/CodexDrawer.tsx:258`
- Modify: `frontend/src/components/StoryTheater.tsx:104`
- Modify: `frontend/src/hooks/useCharacterPortraits.ts:93`
- Modify: `frontend/src/components/WorldsStudio.tsx:253-254, 385-387`
- Modify: `frontend/src/context/CampaignContext.tsx:638`

- [ ] **Step 1: Update portrait URL callers**

In `frontend/src/components/CodexDrawer.tsx`:
Replace:
```ts
const portraitURL = entity.has_custom_portrait
  ? `/api/game/${encodeURIComponent(gameID)}/character/${encodeURIComponent(entity.id)}/portrait${
      entity.updated_at ? `?t=${encodeURIComponent(entity.updated_at)}` : ''
    }`
  : null;
```
with:
```ts
const portraitURL = entity.has_custom_portrait
  ? APIClient.characterPortraitURL(gameID, entity.id, entity.updated_at)
  : null;
```

In `frontend/src/components/StoryTheater.tsx`:
Replace:
```ts
return `/api/game/${gameId}/character/${playerId}/portrait`;
```
with:
```ts
return APIClient.characterPortraitURL(gameId, playerId);
```

In `frontend/src/hooks/useCharacterPortraits.ts`:
Ensure `APIClient` is imported and replace:
```ts
const portraitUrl =
  entity.portrait_url ||
  `/api/game/${encodeURIComponent(activeGameID || '')}/character/${encodeURIComponent(entity.id)}/portrait`;
```
with:
```ts
const portraitUrl =
  entity.portrait_url ||
  APIClient.characterPortraitURL(activeGameID || '', entity.id);
```

- [ ] **Step 2: Update world banner/icon callers in `WorldsStudio.tsx`**

In `frontend/src/components/WorldsStudio.tsx`:
Replace lines 253-254:
```ts
setBannerPreview(APIClient.worldBannerURL(id, Date.now()));
setIconPreview(APIClient.worldIconURL(id, Date.now()));
```
And lines 385-387:
```ts
setBannerPreview(APIClient.worldBannerURL(savedID, Date.now()));
setIconPreview(APIClient.worldIconURL(savedID, Date.now()));
```

- [ ] **Step 3: Update audio SSE caller in `CampaignContext.tsx`**

In `frontend/src/context/CampaignContext.tsx`:
Replace line 638:
```ts
const source = new EventSource(APIClient.audioEventsURL());
```

- [ ] **Step 4: Run tests to verify**

Run: `npx vitest run src/components/CodexDrawer.test.tsx src/components/StoryTheater.test.tsx src/hooks/useCharacterPortraits.test.tsx src/components/WorldsStudio.test.tsx src/context/CampaignContext.test.tsx` (inside `frontend/`)
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/CodexDrawer.tsx frontend/src/components/StoryTheater.tsx frontend/src/hooks/useCharacterPortraits.ts frontend/src/components/WorldsStudio.tsx frontend/src/context/CampaignContext.tsx
git commit -m "refactor(frontend): route URL construction through APIClient (LocalRPG-14a.9)"
```

---

### Task 6: Migrate Package Extension Strings & Accept Filters

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx:487, 494`
- Modify: `frontend/src/components/WorldsStudio.tsx:626, 633`
- Modify: `frontend/src/components/studios/system/SystemHeader.tsx:193`
- Modify: `frontend/src/components/studios/system/SystemList.tsx:61, 84`
- Modify: `frontend/src/components/studios/world/WorldHeader.tsx:142`
- Modify: `frontend/src/components/studios/world/WorldList.tsx:47, 61`
- Modify: `frontend/src/components/LauncherHub.tsx:464`

- [ ] **Step 1: Update `SystemsStudio.tsx` and `WorldsStudio.tsx`**

In `frontend/src/components/SystemsStudio.tsx`:
Import `PACKAGE_EXTENSIONS` from `../lib/constants`:
Replace:
```ts
const defaultFilename = `${savedID}-${version || '1.0.0'}.lrpgsystem`;
```
with:
```ts
const defaultFilename = `${savedID}-${version || '1.0.0'}${PACKAGE_EXTENSIONS.SYSTEM}`;
```
And line 494:
```ts
{ display_name: `LocalRPG System (*${PACKAGE_EXTENSIONS.SYSTEM})`, pattern: `*${PACKAGE_EXTENSIONS.SYSTEM}` }
```

In `frontend/src/components/WorldsStudio.tsx`:
Import `PACKAGE_EXTENSIONS` from `../lib/constants`:
Replace:
```ts
const defaultFilename = `${savedID}-1.0.0.lrpgworld`;
```
with:
```ts
const defaultFilename = `${savedID}-1.0.0${PACKAGE_EXTENSIONS.WORLD}`;
```
And line 633:
```ts
{ display_name: `LocalRPG World (*${PACKAGE_EXTENSIONS.WORLD})`, pattern: `*${PACKAGE_EXTENSIONS.WORLD}` }
```

- [ ] **Step 2: Update studio headers, lists, and launcher accept filters**

In `SystemHeader.tsx`: Use `PACKAGE_EXTENSIONS.SYSTEM` in tooltip text.
In `SystemList.tsx`: Import `PACKAGE_ACCEPT` and use `PACKAGE_ACCEPT.SYSTEM` for `accept`.
In `WorldHeader.tsx`: Use `PACKAGE_EXTENSIONS.WORLD` in tooltip text.
In `WorldList.tsx`: Import `PACKAGE_ACCEPT` and use `PACKAGE_ACCEPT.WORLD` for `accept`.
In `LauncherHub.tsx`: Import `PACKAGE_ACCEPT` and use `PACKAGE_ACCEPT.ALL` for `accept`.

- [ ] **Step 3: Run tests to verify**

Run: `npx vitest run src/components/SystemsStudio.test.tsx src/components/WorldsStudio.test.tsx src/components/LauncherHub.test.tsx` (inside `frontend/`)
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/SystemsStudio.tsx frontend/src/components/WorldsStudio.tsx frontend/src/components/studios/system/SystemHeader.tsx frontend/src/components/studios/system/SystemList.tsx frontend/src/components/studios/world/WorldHeader.tsx frontend/src/components/studios/world/WorldList.tsx frontend/src/components/LauncherHub.tsx
git commit -m "refactor(frontend): share package extensions and accept filters (LocalRPG-14a.9)"
```

---

### Task 7: Migrate Media & Model Defaults

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx:208, 211, 213, 230, 232`
- Modify: `frontend/src/components/settings/MediaPanel.tsx:112`
- Modify: `frontend/src/components/providers/TTSProviderEditor.tsx:358`
- Modify: `frontend/src/context/CampaignContext.tsx:449, 451`
- Modify: `frontend/src/components/providers/LLMRoleEditor.tsx:10-25, 284`

- [ ] **Step 1: Replace Kokoro defaults with `DEFAULT_KOKORO_MODEL_ID` and `DEFAULT_KOKORO_SIZE_BYTES`**

In `SettingsStudio.tsx`, `MediaPanel.tsx`, `TTSProviderEditor.tsx`, and `CampaignContext.tsx`:
Import `DEFAULT_KOKORO_MODEL_ID` and `DEFAULT_KOKORO_SIZE_BYTES` from `../lib/constants`.
Replace literal `'kokoro-tts'` and `90177536` with the imported constants.

- [ ] **Step 2: Replace Gemini fallback model list in `LLMRoleEditor.tsx`**

In `frontend/src/components/providers/LLMRoleEditor.tsx`:
Import `DEFAULT_GEMINI_AGENT_MODELS` from `../../lib/constants`.
Remove the local `const GEMINI_AGENT_MODELS = [ ... ]` declaration.
Use `DEFAULT_GEMINI_AGENT_MODELS` at line 284.

- [ ] **Step 3: Run tests to verify**

Run: `npx vitest run src/components/SettingsStudio.test.tsx src/components/providers/LLMRoleEditor.test.tsx src/context/CampaignContext.test.tsx` (inside `frontend/`)
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx frontend/src/components/settings/MediaPanel.tsx frontend/src/components/providers/TTSProviderEditor.tsx frontend/src/context/CampaignContext.tsx frontend/src/components/providers/LLMRoleEditor.tsx
git commit -m "refactor(frontend): centralize media and model defaults (LocalRPG-14a.9)"
```

---

### Task 8: Migrate Stage Directions Display Mode Consumers

**Files:**
- Modify: `frontend/src/components/MarkdownProse.tsx:7, 19, 106`
- Modify: `frontend/src/components/TurnSegments.tsx:40, 64`
- Modify: `frontend/src/components/theater/TheaterDialogue.tsx:13`
- Modify: `frontend/src/components/story/types.ts:40`

- [ ] **Step 1: Migrate `MarkdownProse.tsx` and `TurnSegments.tsx`**

In `frontend/src/components/MarkdownProse.tsx`:
Import `StageDirectionsDisplayMode` from `../types` and `DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE` from `../lib/constants`.
Replace inline `'stage_directions' | 'hidden' | 'raw'` with `StageDirectionsDisplayMode`.
Replace default value `'stage_directions'` with `DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE`.

In `frontend/src/components/TurnSegments.tsx`:
Import `StageDirectionsDisplayMode` from `../types` and `DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE` from `../lib/constants`.
Replace inline `'stage_directions' | 'hidden' | 'raw'` with `StageDirectionsDisplayMode`.
Replace default value `'stage_directions'` with `DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE`.

- [ ] **Step 2: Migrate `TheaterDialogue.tsx` and `story/types.ts`**

In `frontend/src/components/theater/TheaterDialogue.tsx`:
Import `StageDirectionsDisplayMode` from `../../types`.
Replace `displayMode?: 'stage_directions' | 'hidden' | 'raw';` with `displayMode?: StageDirectionsDisplayMode;`.

In `frontend/src/components/story/types.ts`:
Import `StageDirectionsDisplayMode` from `../../types`.
Replace `display_mode?: 'stage_directions' | 'hidden' | 'raw';` with `display_mode?: StageDirectionsDisplayMode;`.

- [ ] **Step 3: Run tests to verify**

Run: `npx vitest run src/components/MarkdownProse.test.tsx src/components/TurnSegments.test.tsx` (inside `frontend/`)
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/MarkdownProse.tsx frontend/src/components/TurnSegments.tsx frontend/src/components/theater/TheaterDialogue.tsx frontend/src/components/story/types.ts
git commit -m "refactor(frontend): use shared StageDirectionsDisplayMode type (LocalRPG-14a.9)"
```

---

### Task 9: Full Quality Gates & Verification

**Files:** None (repository-wide verification)

- [ ] **Step 1: Run frontend linter**

Run: `mise run lint:frontend`
Expected: 0 errors.

- [ ] **Step 2: Run full frontend test suite**

Run: `mise run test:frontend`
Expected: All 112+ test files pass.

- [ ] **Step 3: Run repository-wide linters and backend tests**

Run: `mise run lint`
Run: `CLEAN=1 mise run test:backend`
Expected: All checks pass.
