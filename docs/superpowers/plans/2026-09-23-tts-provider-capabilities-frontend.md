# TTS Provider Capabilities (Frontend and Cost Guardrails) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render a provider's declared options and voice catalog generically in Settings and the codex, persist per-profile options, and make a metered provider visible with an uncached-beat count.

**Architecture:** One debounced `useTTSInspect` hook calls `POST /api/tts/inspect` for the configuration the editor already holds. A single schema-driven `VoiceOptionsControl` renders every provider's tunables, so no component names a provider. The catalog is a separate picker component used by the codex. The backend gains `TTSConfig.Options`, save-time validation, and an uncached-beat count.

**Tech Stack:** React 19 + TypeScript (strict, `noUnusedLocals`, `noUnusedParameters`), Tailwind v4, `lucide-react`; Go 1.27.1 for the backend additions.

**Spec:** `docs/superpowers/specs/2026-09-22-tts-provider-capabilities-design.md`
**Backend plan (already implemented):** `docs/superpowers/plans/2026-09-23-tts-provider-capabilities.md`

## Scope

This is the second half of the spec. It consumes the `POST /api/tts/inspect` endpoint, `media.ValidateVoiceOptions`, and `media.ProviderKey` delivered by the backend plan.

**Acceptance criterion 7 is scoped down.** The spec asks that a bulk operation warn with the uncached-beat count before synthesis. This application has no GUI bulk-synthesis control today (no export-with-audio button and no play-all), so this plan delivers the count as a tested helper and endpoint, and surfaces the metered flag visibly in Settings and the codex. The confirmation dialog lands with the first bulk control, and that is recorded here rather than inventing a button.

## Global Constraints

- Frontend has no test runner; the gate for a frontend task is `npx tsc --noEmit` (and, for the final gate, `npm run build`). Do not add a test framework.
- `tsconfig.json` enables `strict`, `noUnusedLocals`, `noUnusedParameters`: an unused import or parameter fails the build.
- Follow the existing component style: `React.FC` with an explicit props interface, Tailwind classes, `lucide-react` icons, stone/amber palette.
- Go tests use only `testing` and `t.TempDir()`; `interface{}`, never `any`; errors wrapped with `%w`; `go vet ./...` clean.
- Never write em dashes in source code.
- Conventional Commits with a scope, subject under 72 characters.
- Verification: `mise run test` and `mise run lint`.
- Frontend typecheck: `cd frontend && npx tsc --noEmit`.
- Backend single test example: `go test -run TestCountUncached ./pkg/media/`.

### File Map

| Action | Path | Responsibility |
| :--- | :--- | :--- |
| Modify | `frontend/src/types.ts` | `VoiceOption`, `ProviderVoice`, `VoiceCatalog`, `TTSInspect*`, `VoiceProfile.options`, `TTSConfig.options`/`metered`, `TestProviderRequest.voice_id` |
| Modify | `frontend/src/api/client.ts` | `inspectTTS`, `uncachedBeats` |
| Create | `frontend/src/hooks/useTTSInspect.ts` | Debounced inspect hook |
| Create | `frontend/src/components/VoiceOptionsControl.tsx` | Schema-driven option controls |
| Create | `frontend/src/components/VoiceCatalogPicker.tsx` | Catalog search, filter, audition, add-as-profile |
| Modify | `frontend/src/components/SettingsStudio.tsx` | Options block, catalog strip, metered badge, per-profile options |
| Modify | `frontend/src/components/CodexDrawer.tsx` | Options in the voice snippet, provider hint, catalog picker |
| Modify | `frontend/src/App.tsx` | Pass `ttsConfig` and an `onAddProfile` callback to the codex |
| Modify | `pkg/config/types.go` | `TTSConfig.Options` |
| Modify | `pkg/config/types_test.go` | `TTSConfig.Options` round-trip |
| Modify | `pkg/gui/service.go` | Narrator and default-voice options; `CountUncachedBeats`; save-time validation; `TurnSession` narrator options |
| Create | `pkg/gui/tts_validate.go` | `validateVoiceOptionsInConfig` |
| Modify | `pkg/gui/server.go` | `GET /api/game/{id}/tts/uncached` |
| Modify | `pkg/media/tts.go` | `TTSPipeline.CountUncached` |
| Create | `pkg/media/uncached_test.go` | `CountUncached` tests |
| Create | `pkg/gui/tts_validate_test.go` | Save-time validation tests |
| Create | `pkg/gui/uncached_test.go` | `CountUncachedBeats` route test |

---

## Task 1: Types and client methods

**Files:**
- Modify: `frontend/src/types.ts:309-337`, `frontend/src/types.ts:396-400`
- Modify: `frontend/src/api/client.ts` (imports and methods)

**Interfaces:**
- Consumes: the backend `TTSInspectResponseDTO` shape and `GET /api/game/{id}/tts/uncached`.
- Produces: `VoiceOption`, `ProviderVoice`, `VoiceCatalog`, `TTSInspectRequest`, `TTSInspectResponse`; `VoiceProfile.options`, `TTSConfig.options`, `TTSConfig.metered`, `TestProviderRequest.voice_id`; `APIClient.inspectTTS`, `APIClient.uncachedBeats`.

- [ ] **Step 1: Add the types**

In `frontend/src/types.ts`, replace the `VoiceProfile` and `TTSConfig` interfaces with:

```ts
export interface VoiceProfile {
  id: string;
  name: string;
  voice_id: string;
  provider?: string;
  pitch: number;
  speech_rate: number;
  tags?: string[];
  description?: string;
  // Provider-declared tunables keyed by VoiceOption.key. Omitted means the
  // provider's own defaults.
  options?: Record<string, unknown>;
}

export interface VoiceOption {
  key: string;
  label: string;
  kind: 'float' | 'int' | 'bool' | 'string' | 'enum';
  min?: number;
  max?: number;
  step?: number;
  options?: string[];
  default?: unknown;
  help?: string;
}

export interface ProviderVoice {
  id: string;
  name: string;
  language?: string;
  gender?: string;
  accent?: string;
  categories?: string[];
  tags?: string[];
  description?: string;
  preview_url?: string;
  defaults?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface VoiceCatalog {
  available: boolean;
  fetched_at?: string;
  stale: boolean;
  voices: ProviderVoice[];
}

export interface TTSInspectRequest {
  config: TTSConfig;
  refresh?: boolean;
}

export interface TTSInspectResponse {
  provider_key: string;
  metered: boolean;
  options?: VoiceOption[];
  catalog: VoiceCatalog;
  error?: string;
}

export interface TTSConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled';
  builtin_name?: string;
  model_path?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  default_voice?: string;
  pitch?: number;
  speech_rate?: number;
  auto_play: boolean;
  master_volume: number;
  voice_profiles?: VoiceProfile[];
  // How narration Markdown is treated before synthesis. Omitted means auto.
  markdown?: 'auto' | 'strip' | 'keep';
  // Provider-declared tunables for the default voice.
  options?: Record<string, unknown>;
  // Overrides a provider's own metered declaration when set.
  metered?: boolean;
}
```

Add `voice_id?: string;` to `TestProviderRequest`:

```ts
export interface TestProviderRequest {
  category: 'llm' | 'tts' | 'stt' | 'image';
  provider: AgentRoleConfig | TTSConfig | STTConfig | ImageConfig;
  test_prompt?: string;
  voice_id?: string;
}
```

- [ ] **Step 2: Add the client methods**

In `frontend/src/api/client.ts`, add `TTSInspectRequest, TTSInspectResponse` to the type import list, then add after `testProvider`:

```ts
  static async inspectTTS(req: TTSInspectRequest): Promise<TTSInspectResponse> {
    const res = await fetch('/api/tts/inspect', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new Error(`inspectTTS: ${res.statusText}`);
    return res.json();
  }

  static async uncachedBeats(gameID: string): Promise<{ cached: number; uncached: number }> {
    const res = await fetch(`/api/game/${gameID}/tts/uncached`);
    if (!res.ok) throw new Error(`uncachedBeats: ${res.statusText}`);
    return res.json();
  }
```

- [ ] **Step 3: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors (the methods are unused for now, which TypeScript permits).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): type the TTS inspect response and add client calls"
```

---

## Task 2: The inspect hook

**Files:**
- Create: `frontend/src/hooks/useTTSInspect.ts`

**Interfaces:**
- Consumes: `APIClient.inspectTTS` (Task 1), `TTSConfig`, `TTSInspectResponse`.
- Produces: `useTTSInspect(config: TTSConfig | null, enabled?: boolean): { inspect, loading, error, refresh }`.

- [ ] **Step 1: Write the hook**

Create `frontend/src/hooks/useTTSInspect.ts`:

```ts
import { useEffect, useRef, useState } from 'react';
import { APIClient } from '../api/client';
import { TTSConfig, TTSInspectResponse } from '../types';

// INSPECT_DEBOUNCE_MS lets a burst of edits settle before one inspect runs, so
// dragging a slider does not issue a request per pixel.
const INSPECT_DEBOUNCE_MS = 400;

export interface TTSInspectState {
  inspect: TTSInspectResponse | null;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

// useTTSInspect describes the configuration the editor currently holds, not the
// saved one, so a provider is described before it is applied. It debounces edits
// and ignores a response that a newer edit has superseded.
export function useTTSInspect(config: TTSConfig | null, enabled = true): TTSInspectState {
  const [inspect, setInspect] = useState<TTSInspectResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const requestRef = useRef(0);
  const refreshRef = useRef(false);
  const signature = config ? JSON.stringify(config) : '';

  useEffect(() => {
    if (!enabled || !config) {
      setInspect(null);
      setError(null);
      return;
    }

    const requestID = ++requestRef.current;
    setLoading(true);
    const timer = window.setTimeout(async () => {
      try {
        const refresh = refreshRef.current;
        refreshRef.current = false;
        const res = await APIClient.inspectTTS({ config, refresh });
        if (requestRef.current !== requestID) return;
        setInspect(res);
        setError(res.error ?? null);
      } catch (err) {
        if (requestRef.current !== requestID) return;
        setError(err instanceof Error ? err.message : 'Inspect failed');
      } finally {
        if (requestRef.current === requestID) setLoading(false);
      }
    }, INSPECT_DEBOUNCE_MS);

    return () => window.clearTimeout(timer);
    // The signature stands in for config so an equal config object does not
    // re-trigger the effect on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [signature, nonce, enabled]);

  return {
    inspect,
    loading,
    error,
    refresh: () => {
      refreshRef.current = true;
      setNonce((value) => value + 1);
    },
  };
}
```

- [ ] **Step 2: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/hooks/useTTSInspect.ts
git commit -m "feat(frontend): add a debounced TTS inspect hook"
```

---

## Task 3: The schema-driven options control

**Files:**
- Create: `frontend/src/components/VoiceOptionsControl.tsx`

**Interfaces:**
- Consumes: `VoiceOption` (Task 1).
- Produces: `<VoiceOptionsControl schema values onChange />`, where `onChange(key: string, value: unknown)`.

- [ ] **Step 1: Write the component**

Create `frontend/src/components/VoiceOptionsControl.tsx`:

```tsx
import React from 'react';
import { VoiceOption } from '../types';

interface VoiceOptionsControlProps {
  schema: VoiceOption[];
  values: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
}

// VoiceOptionsControl renders whatever a provider declares. No provider name
// appears here; a new provider needs no frontend change.
export const VoiceOptionsControl: React.FC<VoiceOptionsControlProps> = ({ schema, values, onChange }) => {
  if (schema.length === 0) return null;

  return (
    <div className="space-y-2.5">
      {schema.map((option) => {
        const current = values[option.key];
        return (
          <div key={option.key} className="space-y-1">
            {option.kind !== 'bool' && (
              <div className="flex justify-between text-[11px] text-stone-400">
                <span>{option.label}</span>
                <span className="font-mono text-amber-400">{describeValue(option, current)}</span>
              </div>
            )}
            {renderControl(option, current, onChange)}
            {option.help && <p className="text-[10px] text-stone-500">{option.help}</p>}
          </div>
        );
      })}
    </div>
  );
};

function numericValue(option: VoiceOption, current: unknown): number {
  if (typeof current === 'number') return current;
  if (typeof option.default === 'number') return option.default;
  return option.min ?? 0;
}

function describeValue(option: VoiceOption, current: unknown): string {
  if (option.kind === 'float') return numericValue(option, current).toFixed(2);
  if (option.kind === 'int') return String(Math.round(numericValue(option, current)));
  if (option.kind === 'enum' || option.kind === 'string') {
    const value = typeof current === 'string' ? current : String(option.default ?? '');
    return value || '(provider default)';
  }
  return '';
}

function renderControl(
  option: VoiceOption,
  current: unknown,
  onChange: (key: string, value: unknown) => void,
): React.ReactNode {
  const rangeClass = 'w-full accent-amber-500';
  const inputClass =
    'w-full bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none focus:border-amber-500/60';

  switch (option.kind) {
    case 'float':
    case 'int':
      return (
        <input
          type="range"
          min={option.min ?? 0}
          max={option.max ?? 1}
          step={option.step ?? (option.kind === 'int' ? 1 : 0.01)}
          value={numericValue(option, current)}
          onChange={(e) => onChange(option.key, parseFloat(e.target.value))}
          className={rangeClass}
        />
      );
    case 'bool':
      return (
        <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
          <input
            type="checkbox"
            checked={typeof current === 'boolean' ? current : Boolean(option.default)}
            onChange={(e) => onChange(option.key, e.target.checked)}
            className="accent-amber-500"
          />
          <span>{option.label}</span>
        </label>
      );
    case 'enum':
      return (
        <select
          value={typeof current === 'string' ? current : String(option.default ?? '')}
          onChange={(e) => onChange(option.key, e.target.value)}
          className={inputClass + ' cursor-pointer'}
        >
          {(option.options ?? []).map((choice) => (
            <option key={choice} value={choice}>
              {choice}
            </option>
          ))}
        </select>
      );
    default:
      return (
        <input
          type="text"
          value={typeof current === 'string' ? current : ''}
          placeholder={option.default !== undefined ? String(option.default) : ''}
          onChange={(e) => onChange(option.key, e.target.value)}
          className={inputClass + ' font-mono'}
        />
      );
  }
}
```

- [ ] **Step 2: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors. If the unused `eslint-disable` comment or an unused import is flagged, remove it; TypeScript does not require the comment.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/VoiceOptionsControl.tsx
git commit -m "feat(frontend): render provider options from their declaration"
```

---

## Task 4: Settings options, catalog strip, and metered badge

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx` (imports, hook call, TTS panel insertion)

**Interfaces:**
- Consumes: `useTTSInspect` (Task 2), `VoiceOptionsControl` (Task 3), `TTSConfig.options` (Task 1).
- Produces: the TTS panel renders provider controls, catalog freshness, and a metered badge.

- [ ] **Step 1: Add imports and the hook call**

At the top of `SettingsStudio.tsx`, add to the existing imports:

```tsx
import { VoiceOptionsControl } from './VoiceOptionsControl';
import { useTTSInspect } from '../hooks/useTTSInspect';
```

Inside the component body, immediately after the existing `useState` declarations (near line 67, after `ttsPreviewText`), add:

```tsx
  // The inspect describes the configuration in hand, so a provider is described
  // before it is saved. The fallback keeps the hook unconditional during load.
  const inspectConfig = config?.media.tts ?? { type: 'disabled' as const, auto_play: false, master_volume: 1 };
  const { inspect, loading: inspecting, refresh: refreshInspect } = useTTSInspect(inspectConfig, Boolean(config));
```

- [ ] **Step 2: Insert the options block, catalog strip, and metered badge**

In the TTS section, immediately before the preview block that begins `{config.media.tts.type !== 'disabled' && (`, insert:

```tsx
            {inspect?.metered && (
              <div className="flex items-center gap-2 text-[11px] font-mono text-amber-400/90">
                <span className="px-1.5 py-0.5 rounded border border-amber-500/40 bg-amber-500/10">METERED</span>
                <span>This provider charges per request. Cached clips are reused.</span>
              </div>
            )}

            {inspect && inspect.options && inspect.options.length > 0 && (
              <div className="p-3 rounded-lg bg-stone-950/70 border border-stone-800/80 space-y-2">
                <label className="text-xs font-cinzel uppercase text-stone-300">Provider Tuning</label>
                <VoiceOptionsControl
                  schema={inspect.options}
                  values={config.media.tts.options ?? {}}
                  onChange={(key, value) =>
                    setConfig({
                      ...config,
                      media: {
                        ...config.media,
                        tts: { ...config.media.tts, options: { ...(config.media.tts.options ?? {}), [key]: value } },
                      },
                    })
                  }
                />
              </div>
            )}

            {inspect?.catalog.available && (
              <div className="flex flex-wrap items-center justify-between gap-2 text-[11px] font-mono text-stone-400">
                <span>
                  {inspect.catalog.voices.length} voices
                  {inspect.catalog.fetched_at
                    ? ` - last checked ${new Date(inspect.catalog.fetched_at).toLocaleString()}`
                    : ''}
                  {inspect.catalog.stale ? ' (catalog unavailable, showing the last copy)' : ''}
                </span>
                <button
                  type="button"
                  onClick={refreshInspect}
                  disabled={inspecting}
                  className="px-2 py-1 rounded bg-stone-900 border border-stone-800 text-amber-400 hover:text-amber-300 hover:border-amber-500/40 cursor-pointer disabled:opacity-50"
                >
                  {inspecting ? 'Refreshing...' : 'Refresh Catalog'}
                </button>
              </div>
            )}
```

- [ ] **Step 3: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors. Remove any unused import the compiler flags.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): render provider tuning and catalog state in Settings"
```

---

## Task 5: Per-profile options editor and profile preview

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx` (profile preview call and per-profile card)

**Interfaces:**
- Consumes: `VoiceOptionsControl` (Task 3), `VoiceProfile.options` (Task 1), `inspect.options` (Task 4).
- Produces: a profile carries its own provider options, and a profile preview auditions them.

- [ ] **Step 1: Pass profile options to the preview probe**

In the per-profile Play button, the provider object passed to `handleTestProvider` currently ends with `speech_rate: profile.speech_rate`. Add one line after it:

```tsx
                                options: profile.options,
```

- [ ] **Step 2: Insert the per-profile options editor**

In the profile card, between the closing `</div>` of the pitch/rate/tags grid and the card's closing `</div>`, insert:

```tsx
                    {inspect && inspect.options && inspect.options.length > 0 && (
                      <details className="text-xs">
                        <summary className="cursor-pointer text-[11px] font-cinzel uppercase text-stone-400">
                          Provider Options
                        </summary>
                        <div className="pt-2">
                          <VoiceOptionsControl
                            schema={inspect.options}
                            values={profile.options ?? {}}
                            onChange={(key, value) => {
                              const updated = [...(config.media.tts.voice_profiles || [])];
                              updated[idx] = {
                                ...updated[idx],
                                options: { ...(updated[idx].options ?? {}), [key]: value },
                              };
                              setConfig({
                                ...config,
                                media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                              });
                            }}
                          />
                        </div>
                      </details>
                    )}
```

Anchor the insertion on the end of the description input plus its two closing `</div>` lines and the `))}` that ends the profile map.

- [ ] **Step 3: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): edit and audition per-profile provider options"
```

---

## Task 6: Codex voice picker and provider-aware snippets

**Files:**
- Create: `frontend/src/components/VoiceCatalogPicker.tsx`
- Modify: `frontend/src/components/CodexDrawer.tsx`
- Modify: `frontend/src/App.tsx`

**Interfaces:**
- Consumes: `useTTSInspect` (Task 2), `APIClient.inspectTTS`, `VoiceProfile`/`ProviderVoice` (Task 1).
- Produces: `VoiceCatalogPicker` with props `{ ttsConfig, onAddProfile }`; `CodexDrawer` gains `ttsConfig?: TTSConfig` and `onAddProfile?: (profile: VoiceProfile) => void`; `App` supplies both from its existing config state.

- [ ] **Step 1: Write the catalog picker**

Create `frontend/src/components/VoiceCatalogPicker.tsx`:

```tsx
import React, { useMemo, useState } from 'react';
import { Play, Plus } from 'lucide-react';
import { TTSConfig, VoiceProfile } from '../types';
import { useTTSInspect } from '../hooks/useTTSInspect';
import { playVoicePreview } from '../lib/audioPreview';

interface VoiceCatalogPickerProps {
  ttsConfig: TTSConfig;
  onAddProfile: (profile: VoiceProfile) => void;
}

// VoiceCatalogPicker lists the voices the configured provider offers, so a voice
// that was never authored in Settings can still be auditioned and imported.
export const VoiceCatalogPicker: React.FC<VoiceCatalogPickerProps> = ({ ttsConfig, onAddProfile }) => {
  const { inspect, loading } = useTTSInspect(ttsConfig);
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('');

  const categories = useMemo(() => {
    const seen = new Set<string>();
    (inspect?.catalog.voices ?? []).forEach((voice) => voice.categories?.forEach((value) => seen.add(value)));
    return Array.from(seen).sort();
  }, [inspect]);

  const voices = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return (inspect?.catalog.voices ?? []).filter((voice) => {
      if (category && !voice.categories?.includes(category)) return false;
      if (!needle) return true;
      return voice.name.toLowerCase().includes(needle) || voice.id.toLowerCase().includes(needle);
    });
  }, [inspect, query, category]);

  if (!inspect?.catalog.available) return null;

  const addProfile = (id: string) => {
    const voice = voices.find((candidate) => candidate.id === id);
    if (!voice) return;
    onAddProfile({
      id: voice.id,
      name: voice.name,
      voice_id: voice.id,
      provider: inspect.provider_key,
      pitch: 1,
      speech_rate: 1,
      tags: voice.tags,
      description: voice.description,
      options: voice.defaults,
    });
  };

  return (
    <div className="space-y-2">
      <label className="text-xs font-cinzel uppercase text-stone-400">Provider Catalog</label>
      <div className="flex flex-wrap gap-2">
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search voices..."
          className="flex-1 min-w-[140px] bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
        />
        <select
          value={category}
          onChange={(e) => setCategory(e.target.value)}
          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none cursor-pointer"
        >
          <option value="">All categories</option>
          {categories.map((value) => (
            <option key={value} value={value}>
              {value}
            </option>
          ))}
        </select>
      </div>

      <div className="max-h-48 overflow-y-auto space-y-1.5 pr-1">
        {loading && voices.length === 0 && <p className="text-[11px] text-stone-500">Loading catalog...</p>}
        {!loading && voices.length === 0 && <p className="text-[11px] text-stone-500">No voices match.</p>}
        {voices.map((voice) => (
          <div key={voice.id} className="flex items-center justify-between gap-2 p-2 rounded bg-stone-950/70 border border-stone-800/80">
            <div className="min-w-0">
              <div className="text-xs text-stone-200 truncate">{voice.name}</div>
              <div className="text-[10px] font-mono text-stone-500 truncate">{voice.id}</div>
            </div>
            <div className="flex items-center gap-1.5 shrink-0">
              {voice.preview_url && (
                <button
                  type="button"
                  onClick={() => playVoicePreview(voice.preview_url as string)}
                  className="p-1.5 rounded bg-stone-900 border border-stone-800 text-amber-400 hover:text-amber-300 cursor-pointer"
                  title="Audition"
                >
                  <Play className="w-3.5 h-3.5" />
                </button>
              )}
              <button
                type="button"
                onClick={() => addProfile(voice.id)}
                className="p-1.5 rounded bg-stone-900 border border-stone-800 text-amber-400 hover:text-amber-300 cursor-pointer"
                title="Add as profile"
              >
                <Plus className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
```

Create `frontend/src/lib/audioPreview.ts` for the picker's audition button:

```ts
// playVoicePreview plays a catalog preview without touching the narration queue,
// so auditioning a voice never interrupts the scene.
export function playVoicePreview(url: string, volume = 1): void {
  const audio = new Audio(url);
  audio.volume = Math.min(1, Math.max(0, volume));
  audio.play().catch(() => undefined);
}
```

`SettingsStudio.tsx` keeps its own preview helper because it stops the previous preview and reports a blocked-playback message; the picker has no such feedback surface, so it uses this simpler one. Do not refactor `SettingsStudio`.

- [ ] **Step 2: Extend CodexDrawer**

In `CodexDrawer.tsx`:

Add to the props interface:

```tsx
  ttsConfig?: TTSConfig;
  activeProvider?: string;
  onAddProfile?: (profile: VoiceProfile) => void;
```

Import `TTSConfig`, `VoiceCatalogPicker`, and the hook. In `applyVoiceArchetype`, after pushing `speech_rate`, add:

```ts
    if (profile.options && Object.keys(profile.options).length > 0) {
      const entries = Object.entries(profile.options).map(
        ([key, value]) => `${key}: ${inlineYaml(value)}`,
      );
      lines.push(`options:\n    ${entries.join('\n    ')}`);
    }
```

Add a module-level helper:

```ts
// inlineYaml renders a canonical option value so an imported profile's tunables
// survive into the character's frontmatter.
function inlineYaml(value: unknown): string {
  return typeof value === 'string' ? JSON.stringify(value) : String(value);
}
```

In the archetype select, add a mismatch hint to each option label:

```tsx
                {profiles.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} ({p.voice_id}){p.provider && p.provider !== activeProvider ? ` - belongs to ${p.provider}` : ''}
                  </option>
                ))}
```

After the archetype select row closes, render the catalog picker when a config is available and a callback was supplied:

```tsx
            {ttsConfig && onAddProfile && (
              <VoiceCatalogPicker ttsConfig={ttsConfig} onAddProfile={onAddProfile} />
            )}
```

- [ ] **Step 3: Wire App**

In `App.tsx`'s `<CodexDrawer ... />`, add:

```tsx
                ttsConfig={config?.media.tts}
                onAddProfile={(profile) => {
                  if (!config) return;
                  const existing = config.media.tts.voice_profiles ?? [];
                  if (existing.some((p) => p.id === profile.id)) return;
                  const next = {
                    ...config,
                    media: {
                      ...config.media,
                      tts: { ...config.media.tts, voice_profiles: [...existing, profile] },
                    },
                  };
                  setConfig(next);
                  APIClient.saveSettings(next).catch(() => undefined);
                }}
```

`activeProvider` can be left unset here; the hint simply does not render. If the inspect already runs elsewhere in `App`, pass `activeProvider={...}` from it rather than adding a second call.

- [ ] **Step 4: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/VoiceCatalogPicker.tsx frontend/src/components/CodexDrawer.tsx frontend/src/App.tsx frontend/src/lib/audioPreview.ts
git commit -m "feat(frontend): pick, audition, and import provider catalog voices"
```

(Adjust the file list if the preview helper was not extracted.)

---

## Task 7: `TTSConfig.Options` for the default voice

**Files:**
- Modify: `pkg/config/types.go` (`TTSConfig`)
- Modify: `pkg/config/types_test.go`
- Modify: `pkg/gui/service.go` (narrator voice construction, `TestProvider`)

**Interfaces:**
- Produces: `config.TTSConfig.Options map[string]interface{}`; the narrator voice and the test probe carry it.

- [ ] **Step 1: Write the failing test**

Append to `pkg/config/types_test.go`:

```go
func TestTTSConfigOptionsRoundTrip(t *testing.T) {
	cfg := TTSConfig{Options: map[string]interface{}{"stability": 0.4, "model": "eleven_multilingual_v2"}}
	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded TTSConfig
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Options["stability"] != 0.4 || decoded.Options["model"] != "eleven_multilingual_v2" {
		t.Errorf("options did not round-trip: %v", decoded.Options)
	}

	plain, err := yaml.Marshal(TTSConfig{})
	if err != nil {
		t.Fatalf("marshal plain: %v", err)
	}
	if strings.Contains(string(plain), "options") {
		t.Errorf("an unset options map must be omitted, got %s", plain)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestTTSConfigOptionsRoundTrip ./pkg/config/ -v`
Expected: FAIL to compile with "unknown field Options".

- [ ] **Step 3: Add the field**

In `pkg/config/types.go`, add to `TTSConfig` after `Metered`:

```go
	// Options holds provider-declared tunables for the default voice, keyed by
	// VoiceOption.Key. Absent means the provider's own defaults.
	Options map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
```

- [ ] **Step 4: Carry it into the narrator voice and the probe**

In `pkg/gui/service.go`, find the narrator voice literal in the per-segment synthesis path (`narratorVoice := &entity.VoiceConfig{...}`) and add:

```go
		Options:    cfg.Media.TTS.Options,
```

Do the same in `cmd/localrpg/play.go` if it constructs a narrator voice, and in `pkg/export/script.go` where narration is synthesised, so exports honour the tuning too. Run `grep -rn "narratorVoice :=" pkg cmd` to find every site.

In `TestProvider`'s `"tts"` case, extend the probe voice:

```go
		voice := &entity.VoiceConfig{
			VoiceID:    voiceID,
			Pitch:      ttsCfg.Pitch,
			SpeechRate: ttsCfg.SpeechRate,
			Options:    ttsCfg.Options,
		}
```

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/config/ ./pkg/gui/ ./pkg/export/ ./cmd/... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go pkg/gui/service.go pkg/export/script.go cmd/localrpg/play.go
git commit -m "feat: tune the default narrator voice with provider options"
```

(Adjust the file list to the narrator-voice sites that actually exist.)

---

## Task 8: Validate options when settings are saved

**Files:**
- Create: `pkg/gui/tts_validate.go`
- Create: `pkg/gui/tts_validate_test.go`
- Modify: `pkg/gui/service.go` (`SaveSettings`)

**Interfaces:**
- Consumes: `media.ValidateVoiceOptions`, `media.NewTTSClient`/`Service.ttsClientFor`, `media.VoiceOptions`.
- Produces: `(*Service).validateVoiceOptionsInConfig(ctx, cfg *config.Config) error`.

- [ ] **Step 1: Write the failing test**

Create `pkg/gui/tts_validate_test.go`:

```go
package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestSaveSettingsClampsProfileOptions(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &inspectingClient{}, nil
	}

	cfg := config.DefaultConfig()
	cfg.Media.TTS = config.TTSConfig{
		Type:         "http",
		Endpoint:     "http://localhost:8880/v1/audio/speech",
		AutoPlay:     true,
		MasterVolume: 1,
		VoiceProfiles: []config.VoiceProfile{
			{ID: "hushed", Name: "Hushed", VoiceID: "bf_emma", Options: map[string]interface{}{"stability": 2.5, "banana": 1}},
		},
		Options: map[string]interface{}{"stability": -1.0},
	}

	if err := svc.validateVoiceOptionsInConfig(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}

	profile := cfg.Media.TTS.VoiceProfiles[0]
	if profile.Options["stability"] != 1.0 {
		t.Errorf("profile stability = %v, want the clamped 1.0", profile.Options["stability"])
	}
	if _, ok := profile.Options["banana"]; ok {
		t.Errorf("an unknown option must be dropped, got %v", profile.Options)
	}
	if cfg.Media.TTS.Options["stability"] != 0.0 {
		t.Errorf("config stability = %v, want the clamped 0.0", cfg.Media.TTS.Options["stability"])
	}
}

func TestSaveSettingsKeepsOptionsWhenProviderIsDisabled(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &bareClient{}, nil
	}

	cfg := config.DefaultConfig()
	cfg.Media.TTS = config.TTSConfig{
		Type:     "builtin",
		AutoPlay: false,
		Options:  map[string]interface{}{"stability": 0.5},
	}

	// A provider with no declaration cannot validate, so nothing is dropped.
	if err := svc.validateVoiceOptionsInConfig(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cfg.Media.TTS.Options["stability"] != 0.5 {
		t.Errorf("options = %v, want the value kept", cfg.Media.TTS.Options)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestSaveSettings ./pkg/gui/ -v`
Expected: FAIL with "undefined: validateVoiceOptionsInConfig".

- [ ] **Step 3: Write the validator**

Create `pkg/gui/tts_validate.go`:

```go
package gui

import (
	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

// validateVoiceOptionsInConfig clamps and drops provider options against the
// active provider's own declaration. A provider that declares nothing cannot
// validate, so its values are left untouched rather than discarded.
func (s *Service) validateVoiceOptionsInConfig(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	tts := cfg.Media.TTS
	client, err := s.ttsClientFor(tts)
	if err != nil {
		// The provider cannot be built, so there is no schema to validate against;
		// the save should still succeed and let the provider report its own error.
		return nil
	}
	schema, ok := client.(media.VoiceOptions)
	if !ok {
		return nil
	}
	declared := schema.VoiceOptions()

	if len(tts.Options) > 0 {
		canonical, _ := media.ValidateVoiceOptions(declared, tts.Options)
		cfg.Media.TTS.Options = canonical
	}
	for i := range cfg.Media.TTS.VoiceProfiles {
		profile := &cfg.Media.TTS.VoiceProfiles[i]
		if len(profile.Options) == 0 {
			continue
		}
		canonical, _ := media.ValidateVoiceOptions(declared, profile.Options)
		profile.Options = canonical
	}
	return nil
}
```

- [ ] **Step 4: Call it from SaveSettings**

In `pkg/gui/service.go`, inside `SaveSettings`, before the config is written:

```go
	if err := s.validateVoiceOptionsInConfig(&cfg); err != nil {
		return nil, fmt.Errorf("validate tts options: %w", err)
	}
```

Match the surrounding code's parameter type and error style.

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/gui/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/tts_validate.go pkg/gui/tts_validate_test.go pkg/gui/service.go
git commit -m "feat(gui): clamp provider options when settings are saved"
```

---

## Task 9: Count uncached beats

**Files:**
- Modify: `pkg/media/tts.go`
- Create: `pkg/media/uncached_test.go`
- Modify: `pkg/gui/service.go` (`CountUncachedBeats`)
- Modify: `pkg/gui/server.go` (route)
- Create: `pkg/gui/uncached_test.go`

**Interfaces:**
- Consumes: `ComputeAudioCacheKeyForVoice`, `TTSPipeline`, `Service.voiceFor`, `engine.HistoryLogger`.
- Produces: `(*TTSPipeline).CountUncached(segments, narratorVoice, voiceFor) (cached, uncached int)`; `(*Service).CountUncachedBeats(ctx, gameID) (cached, uncached int, err error)`; `GET /api/game/{id}/tts/uncached`.

- [ ] **Step 1: Write the failing media test**

Create `pkg/media/uncached_test.go`:

```go
package media

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestCountUncached(t *testing.T) {
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(&echoTTSClient{}, cache)

	narrator := &entity.VoiceConfig{VoiceID: "af_bella"}
	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The gate stands open."},
		{Kind: entity.SegmentSpeech, SpeakerID: "aldric", Text: "Hold the line."},
		{Kind: entity.SegmentNarration, Text: "***"},
	}

	cached, uncached := pipeline.CountUncached(segments, narrator, nil)
	if cached != 0 || uncached != 2 {
		t.Fatalf("cached = %d, uncached = %d, want 0 and 2", cached, uncached)
	}

	// Synthesising one segment brings the count down by exactly one.
	if _, err := pipeline.SynthesizeSegment(context.Background(), segments[0], narrator, nil); err != nil {
		t.Fatalf("SynthesizeSegment: %v", err)
	}
	cached, uncached = pipeline.CountUncached(segments, narrator, nil)
	if cached != 1 || uncached != 1 {
		t.Errorf("cached = %d, uncached = %d, want 1 and 1", cached, uncached)
	}
}
```

The `***` segment is a scene break that reduces to no speakable text, so it is counted in neither total.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestCountUncached ./pkg/media/ -v`
Expected: FAIL with "pipeline.CountUncached undefined".

- [ ] **Step 3: Implement `CountUncached` and share the resolver**

In `pkg/media/tts.go`, factor the speaker/voice resolution out of `SynthesizeSegment`:

```go
// prepareSegment resolves who reads a segment and in what voice, without
// synthesising. Both synthesis and the uncached count use it, so the two can
// never disagree about which clip a segment needs.
func (p *TTSPipeline) prepareSegment(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (speakerID string, voice *entity.VoiceConfig, spoken string) {
	spoken = SpeakableTextFor(p.policy, p.client, segment.Text)
	voice = narratorVoice
	speakerID = narratorSpeaker

	if segment.Kind == entity.SegmentSpeech {
		speakerID = segment.SpeakerID
		if speakerID == "" {
			speakerID = segment.Speaker
		}
		if speakerID == "" {
			speakerID = narratorSpeaker
		}
		if voiceFor != nil {
			if resolved := voiceFor(speakerID); resolved != nil {
				voice = resolved
			}
		}
	}
	return speakerID, voice, spoken
}
```

Rewrite `SynthesizeSegment` to use it, keeping the existing reduction trace event:

```go
func (p *TTSPipeline) SynthesizeSegment(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (string, error) {
	speakerID, voice, spoken := p.prepareSegment(segment, narratorVoice, voiceFor)
	if strings.TrimSpace(spoken) == "" {
		return "", ErrNoSpeakableText
	}
	if spoken != segment.Text {
		p.logger = trace.OrNil(p.logger)
		p.logger.Event("media.tts.reduced", map[string]interface{}{
			"chars_raw":    len([]rune(segment.Text)),
			"chars_spoken": len([]rune(spoken)),
		})
	}
	return p.SynthesizeUtterance(ctx, speakerID, voice, spoken)
}
```

Add:

```go
// CountUncached reports how many speakable segments already have a clip and how
// many would need synthesis, so a bulk operation can warn before spending money
// on a metered provider. It mutates nothing.
func (p *TTSPipeline) CountUncached(segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (cached, uncached int) {
	for _, segment := range segments {
		speakerID, voice, spoken := p.prepareSegment(segment, narratorVoice, voiceFor)
		if strings.TrimSpace(spoken) == "" {
			continue
		}
		if _, ok := p.cachedClip(ComputeAudioCacheKeyForVoice(speakerID, voice, spoken)); ok {
			cached++
		} else {
			uncached++
		}
	}
	return cached, uncached
}
```

- [ ] **Step 4: Add the service method and route**

In `pkg/gui/service.go`, add:

```go
// CountUncachedBeats reports how much of a campaign's speech is already cached,
// so a bulk synthesis can warn before spending money on a metered provider.
func (s *Service) CountUncachedBeats(gameID string) (cached, uncached int, err error) {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return 0, 0, fmt.Errorf("build tts client: %w", err)
	}

	historyPath := filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")
	turns, err := engine.NewHistoryLogger(historyPath).LoadHistory()
	if err != nil {
		return 0, 0, fmt.Errorf("load history: %w", err)
	}

	narratorVoice := &entity.VoiceConfig{
		VoiceID:    cfg.Media.TTS.DefaultVoice,
		Pitch:      cfg.Media.TTS.Pitch,
		SpeechRate: cfg.Media.TTS.SpeechRate,
		Options:    cfg.Media.TTS.Options,
	}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(s.resolver.CacheDir()))
	pipeline.SetTextPolicy(media.TextPolicyFromConfig(cfg.Media.TTS))

	voiceFor := s.voiceFor(gameID)
	for _, turn := range turns {
		if len(turn.Segments) == 0 {
			continue
		}
		turnCached, turnUncached := pipeline.CountUncached(turn.Segments, narratorVoice, voiceFor)
		cached += turnCached
		uncached += turnUncached
	}
	return cached, uncached, nil
}
```

In `pkg/gui/server.go`, the game routes are dispatched by `handleGameRoutes`, which splits `/api/game/{id}/{action}/...` into `gameID`, `action := parts[1]`, and the rest. Add a case to that switch:

```go
	case "tts":
		if len(parts) < 3 || parts[2] != "uncached" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		cached, uncached, err := s.service.CountUncachedBeats(gameID)
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, map[string]int{"cached": cached, "uncached": uncached})
```

- [ ] **Step 5: Write the route test**

Create `pkg/gui/uncached_test.go`:

```go
package gui

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestUncachedBeatsRoute(t *testing.T) {
	gameID, svc := setupTestGame(t)
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &inspectingClient{}, nil
	}
	server := NewServer(svc, AssetHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/tts/uncached", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "uncached") {
		t.Errorf("body = %s", rec.Body.String())
	}
}
```

`setupTestGame(t) (string, *Service)` already exists in `pkg/gui/service_test.go`. Being a read-only count over an empty history, the expected body is `{"cached":0,"uncached":0}`.

- [ ] **Step 6: Run tests**

Run: `go test ./pkg/media/ ./pkg/gui/ -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/media/tts.go pkg/media/uncached_test.go pkg/gui/service.go pkg/gui/server.go pkg/gui/uncached_test.go
git commit -m "feat: count uncached speech beats before a bulk synthesis"
```

---

## Task 10: Full gate

**Files:** none (verification only).

- [ ] **Step 1: Run the whole suite**

Run: `go test ./... -count=1`
Expected: every package ok.

- [ ] **Step 2: Vet and frontend gate**

Run: `mise run lint` and `cd frontend && npm run build`
Expected: `go vet ./...` clean and the frontend build (tsc plus vite) succeeds.

- [ ] **Step 3: Confirm the acceptance criteria this plan owns**

- 1: a provider's options render generically through `VoiceOptionsControl`; a provider without them shows nothing extra (Tasks 3-4).
- 2: its catalog lists, searches, filters, and auditions, and a stale snapshot still renders (Tasks 6 and the backend plan's `CachedVoiceCatalog`).
- 3: profile options persist to `config.yaml` and reach an entity's frontmatter (Task 5 and the codex snippet in Task 6).
- 5: a mismatched profile is labelled "belongs to X" (Task 6).
- 7: partial by design. The metered flag is visible and the uncached count is a tested helper plus endpoint (Tasks 4 and 9); the pre-flight dialog is deferred until a bulk control exists.

---

## Self-Review

**Spec coverage (frontend scope):**

- 3.8 Settings Studio: Tasks 2-5 (`useTTSInspect`, generic options, catalog strip, per-profile options, metered badge).
- 3.8 Codex voice picker: Task 6 (catalog section, search, category filter, audition, "Add as profile", mismatch hint).
- 3.9 cost guardrails: the metered flag (Task 4) and `CountUncachedBeats` with its endpoint (Task 9); the interactive warning is deliberately deferred and stated in Scope.
- 3.3 validation on save: Task 8.

**Placeholder scan:** no "TBD"/"implement later" text; every code step carries real code and the one cross-file reuse (the preview helper) is spelled out as a new file rather than left to discovery. `setupTestGame`'s signature is given explicitly.

**Type consistency:** `VoiceOption`, `ProviderVoice`, `VoiceCatalog`, `TTSInspectRequest`, `TTSInspectResponse` are defined in Task 1 and used in Tasks 2, 3, 4, 6. `useTTSInspect` is defined in Task 2 and called in Tasks 4 and 6. `VoiceOptionsControl` is defined in Task 3 and used in Tasks 4 and 5. `VoiceCatalogPicker` is defined in Task 6 and used by `CodexDrawer`. `config.TTSConfig.Options` is defined in Task 7 and consumed in Tasks 4, 7, and 9. `CountUncached` is defined and used in Task 9; `validateVoiceOptionsInConfig` is defined and used in Task 8.
