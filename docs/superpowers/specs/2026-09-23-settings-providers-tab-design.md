# Dedicated Providers Tab in Settings Studio Design

**Date:** 2026-09-23  
**Status:** Approved

---

## Goal

Create a dedicated **Providers** tab in the Settings Studio to centrally manage shared credentials and configuration for multi-service providers (such as Google Gemini), rather than placing the shared key within the AI Agents tab.

---

## Background

Previously, the shared Google Gemini API key was located in the **AI Agents** tab of `SettingsStudio.tsx`. However, Google Gemini now powers three distinct subsystems in LocalRPG:
1. **AI Agents:** Text generation across all roles (GM, Narrator, Extractor, Assistant, etc.).
2. **Image Generation:** Scene art generation using Imagen 3 and Nano Banana (Gemini Image) models.
3. **Voice Synthesis (TTS):** Speech generation using Gemini 3.1 & 2.5 Flash/Pro TTS preview models.

Housing the shared key in the AI Agents tab was confusing and unintuitive for users looking to configure media capabilities. Moving shared ecosystem keys to a dedicated **Providers** tab provides a clean, extensible home for multi-capability credentials.

---

## UI & Architecture

### 1. Navigation & Tab Bar

In `frontend/src/components/SettingsStudio.tsx`:
- Extend `activeSubTab` state:
  ```typescript
  const [activeSubTab, setActiveSubTab] = useState<
    'paths' | 'providers' | 'agents' | 'media' | 'preferences' | 'debug'
  >('paths');
  ```
- Insert a **Providers** tab button between `Paths` and `AI Agents`:
  ```tsx
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

### 2. Providers Tab Content

When `activeSubTab === 'providers'`, render:
- **Header:**
  - Icon: `Cloud`
  - Title: `Cloud & Ecosystem Providers`
  - Description: "Configure shared credentials and master keys for ecosystem providers that power multiple capabilities across LocalRPG."
- **Google Gemini Card:**
  - Header: `Sparkles` icon with "Google Gemini (GenAI)"
  - Status indicator:
    - If `config.providers?.gemini?.api_key` is non-empty: `✓ Configured in settings` (emerald badge)
    - If empty: `Using env or unconfigured` (stone badge)
  - API Key Input:
    - Bound to `config.providers?.gemini?.api_key`
    - Placeholder: `AIzaSy... or leave blank for GEMINI_API_KEY / GOOGLE_API_KEY`
    - Password type with clear styling matching LocalRPG design system
    - Explanatory note: "This master key is automatically inherited by Gemini LLM agents, image generators, and voice synthesis. Individual roles and media engines can still provide an override key."
  - Connected Services Indicators:
    - Badges highlighting supported capabilities:
      - `AI Agents (GM, Narrator, Extractor)`
      - `Image Generation (Imagen 3, Nano Banana)`
      - `Speech Synthesis (Gemini 3.1 & 2.5 TTS)`
- **Extensible Card Architecture:**
  - Designed as a card list so additional providers (such as OpenAI or Anthropic) can be added as modular sibling cards in the future.

### 3. Clean-Up in Other Tabs

- **AI Agents Tab:**
  - Remove the standalone shared Gemini key input block (lines 428–460 of `SettingsStudio.tsx`).
  - Keep the per-role "Role API Key Override" field. When `config.providers?.gemini?.api_key` is set, display a subtle indicator `Shared key active (from Providers tab)` and placeholder `Using shared key (leave blank)`.
- **Media Engines Tab (Image & TTS):**
  - Keep existing override inputs and shared key indicators, which already link to `config.providers?.gemini?.api_key`.

---

## Verification & Testing

1. **Frontend typecheck:** Run `mise run test:frontend` (`tsc --noEmit`) to verify strict typing with no unused variables or types.
2. **Frontend build:** Run `mise run build:frontend` (`npm run build`) to ensure Vite bundle builds cleanly.
3. **Full verification:** Run backend tests `go test ./...` and `go vet ./...` to guarantee no regressions.
