# Dedicated Providers Tab in Settings Studio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Relocate the shared Google Gemini master credentials into a dedicated "Providers" tab within Settings Studio, providing a clean and extensible central interface for multi-service providers.

**Architecture:** Update `frontend/src/components/SettingsStudio.tsx` to add `'providers'` to the subtab navigation with a `Cloud` icon, render a new Providers view containing a Google Gemini card with status indicators and supported capabilities, and remove the redundant shared key input from the AI Agents tab while maintaining per-role override support.

**Tech Stack:** React 19, TypeScript, Tailwind CSS v4, `lucide-react`.

---

### File Map

| Action | File | Responsibility |
|---|---|---|
| Modify | `frontend/src/components/SettingsStudio.tsx` | Add `'providers'` tab, render Providers panel, remove duplicate key box from AI Agents tab |

---

### Task 1: Add Providers Tab Navigation

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Update imports and subtab state**

Ensure `Cloud` is imported from `lucide-react` in `frontend/src/components/SettingsStudio.tsx`:
```typescript
import {
  Folder,
  Cpu,
  Volume2,
  Sliders,
  Bug,
  Save,
  CheckCircle,
  AlertCircle,
  Sparkles,
  Cloud,
  // ...
} from 'lucide-react';
```

Update `activeSubTab` state:
```typescript
const [activeSubTab, setActiveSubTab] = useState<
  'paths' | 'providers' | 'agents' | 'media' | 'preferences' | 'debug'
>('paths');
```

- [x] **Step 2: Add Providers tab button to the navigation bar**

In `SettingsStudio.tsx`, insert the Providers button right after Paths:
```tsx
          <button
            onClick={() => setActiveSubTab('paths')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeSubTab === 'paths' ? 'bg-amber-600 text-stone-950 font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Folder className="w-3.5 h-3.5" />
            <span>Paths</span>
          </button>
          <button
            onClick={() => setActiveSubTab('providers')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeSubTab === 'providers' ? 'bg-amber-600 text-stone-950 font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Cloud className="w-3.5 h-3.5" />
            <span>Providers</span>
          </button>
```

- [x] **Step 3: Verify TypeScript compiles**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add Providers navigation tab to SettingsStudio"
```

---

### Task 2: Implement Providers Tab View

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Add Providers tab rendering block**

Right after `{activeSubTab === 'paths' && ( ... )}` in `frontend/src/components/SettingsStudio.tsx`, add:

```tsx
      {/* Tab: Ecosystem Providers */}
      {activeSubTab === 'providers' && (
        <div className="space-y-4 flex-1 overflow-y-auto pr-1">
          <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
            <h3 className="font-cinzel text-sm font-bold text-amber-400 flex items-center gap-2">
              <Cloud className="w-4 h-4" />
              <span>Cloud & Ecosystem Providers</span>
            </h3>
            <p className="text-xs text-stone-400">
              Configure shared credentials and master keys for external AI and media providers that power multiple capabilities across LocalRPG.
            </p>

            {/* Google Gemini Provider Card */}
            <div className="p-4 bg-stone-950/80 border border-stone-800/90 rounded-xl space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-2 border-b border-stone-800/60 pb-3">
                <div className="flex items-center gap-2">
                  <div className="p-1.5 rounded-lg bg-amber-500/10 border border-amber-500/30 text-amber-400">
                    <Sparkles className="w-4 h-4" />
                  </div>
                  <div>
                    <h4 className="text-xs font-cinzel font-bold text-stone-200">Google Gemini (GenAI)</h4>
                    <p className="text-[11px] text-stone-400">Multi-modal intelligence: text reasoning, image creation, and vocal performance.</p>
                  </div>
                </div>
                <div>
                  {config.providers?.gemini?.api_key ? (
                    <span className="flex items-center gap-1 text-emerald-400 font-mono text-[11px] bg-emerald-950/40 border border-emerald-800/40 px-2 py-0.5 rounded-md">
                      <CheckCircle className="w-3.5 h-3.5" />
                      <span>Configured in Settings</span>
                    </span>
                  ) : (
                    <span className="text-[11px] font-mono text-stone-500 bg-stone-900 border border-stone-800 px-2 py-0.5 rounded-md">
                      Using env or unconfigured
                    </span>
                  )}
                </div>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Shared Gemini API Key</span>
                  <span className="text-[10px] text-stone-500 font-mono">
                    {config.providers?.gemini?.api_key ? '✓ Custom Key Saved' : 'Optional if GEMINI_API_KEY is set'}
                  </span>
                </label>
                <input
                  type="password"
                  placeholder="AIzaSy... or leave blank for GEMINI_API_KEY / GOOGLE_API_KEY env var"
                  value={config.providers?.gemini?.api_key || ''}
                  onChange={(e) => {
                    setConfig({
                      ...config,
                      providers: {
                        ...config.providers,
                        gemini: {
                          ...config.providers?.gemini,
                          api_key: e.target.value,
                        },
                      },
                    });
                  }}
                  className="w-full bg-stone-900 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  Automatically inherited by Gemini LLM agents, Gemini/Imagen image generators, and Gemini TTS voice synthesis. Individual roles and media engines can still provide an override key.
                </p>
              </div>

              <div className="pt-2 border-t border-stone-800/50">
                <div className="text-[11px] font-cinzel uppercase text-stone-400 font-semibold mb-2">Connected Subsystems</div>
                <div className="flex flex-wrap gap-2">
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-[11px] text-stone-300 font-mono">
                    <Cpu className="w-3 h-3 text-amber-400" />
                    <span>AI Agents (GM, Narrator, Extractor)</span>
                  </span>
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-[11px] text-stone-300 font-mono">
                    <Sparkles className="w-3 h-3 text-amber-400" />
                    <span>Image Generation (Imagen 3, Nano Banana)</span>
                  </span>
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-[11px] text-stone-300 font-mono">
                    <Volume2 className="w-3 h-3 text-amber-400" />
                    <span>Voice Synthesis (Gemini 3.1 & 2.5 Flash/Pro TTS)</span>
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}
```

- [x] **Step 2: Verify TypeScript compiles**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): implement Ecosystem Providers panel in SettingsStudio"
```

---

### Task 3: Clean Up AI Agents Tab & Refine Override References

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Remove duplicate shared key box from AI Agents tab**

In `SettingsStudio.tsx`, remove the block:
```tsx
            {/* Shared Gemini Credentials */}
            <div className="p-3 bg-stone-900/60 border border-stone-800 rounded-xl space-y-1.5">
              ...
            </div>
```
which was located directly above `<div className="space-y-4 pt-2">` in the `activeSubTab === 'agents'` section.

- [x] **Step 2: Update the Role API Key Override hint in AI Agents tab**

Update the hint and placeholder for the per-role API key override:
```tsx
                      <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                        <span>Role API Key Override</span>
                        {config.providers?.gemini?.api_key && (
                          <span className="text-[10px] text-emerald-400 font-mono">Shared key active (from Providers tab)</span>
                        )}
                      </label>
                      <input
                        type="password"
                        placeholder={config.providers?.gemini?.api_key ? 'Using shared key from Providers tab (leave blank)' : 'Optional override or GEMINI_API_KEY env'}
                        value={currentRoleConfig.api_key || ''}
                        onChange={(e) => {
                          const updated = { ...currentRoleConfig, api_key: e.target.value };
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: { ...config.agents.roles, [selectedRole]: updated },
                            },
                          });
                        }}
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                      />
```

- [x] **Step 3: Verify TypeScript compiles**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "refactor(frontend): remove duplicate shared key from AI Agents tab"
```

---

### Task 4: Full Verification and Build

- [x] **Step 1: Run all tests**

Run: `mise run test`
Expected: All backend tests pass and TypeScript check passes.

- [x] **Step 2: Run frontend and backend builds**

Run: `mise run build`
Expected: Vite builds bundle cleanly into `pkg/gui/dist` and Go binary compiles to `bin/localrpg`.

- [x] **Step 3: Verify git status (restore dist/.gitkeep if needed)**

Run: `git status`
Expected: Clean working tree.

- [x] **Step 4: Mark plan complete and commit**

```bash
git add docs/superpowers/plans/2026-09-23-settings-providers-tab.md
git commit -m "docs: mark all settings providers tab plan tasks complete"
```
