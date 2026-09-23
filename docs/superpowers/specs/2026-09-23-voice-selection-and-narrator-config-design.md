# Design Spec: Voice Selection, Provider Catalogs, and Campaign Narrator Voice Configuration

**Date:** 2026-09-23  
**Status:** Approved  
**Target:** `pkg/media`, `pkg/gui`, `frontend`  

---

## 1. Executive Summary

LocalRPG supports voice profiles for turn narration and character speech, and recently introduced the `media.VoiceCatalog` capability for dynamic enumeration of provider voices (such as ElevenLabs). However, users currently face several gaps:
1. In Settings Studio, the `voice_id` for each voice profile is a raw free-text input without any dropdown or references to available catalog voices, making it impossible to know or select the valid voice IDs without inspecting external documentation or APIs.
2. The global TTS `default_voice` (used as the baseline narrator voice) is not editable directly in Settings Studio.
3. Built-in providers like `sherpa-onnx` (which supports 11 Kokoro speaker models) do not implement `VoiceCatalog`, leaving their voices unlisted in catalog inspectors.
4. When creating a new campaign in `LauncherHub`, there is no option to choose the narrator voice for that specific campaign (it always inherits the global default), nor is there an easy way to tune the narrator voice during campaign play.

This spec addresses these gaps by:
- Implementing `media.VoiceCatalog` on `SherpaTTSClient` so built-in Kokoro voices appear alongside dynamic catalogs.
- Building a reusable `VoiceCombobox` component with search, filtering, audition previews, and free-text fallback for non-catalog providers.
- Adding an "Import from Catalog" modal in Settings Studio for auditioning and creating voice profiles in one click.
- Exposing `default_voice` in Settings Studio.
- Allowing campaign-level narrator voice configuration during campaign creation in `LauncherHub` and during play via campaign settings.

---

## 2. Architecture & Backend Changes

### 2.1 Static Voice Catalog for Sherpa-ONNX

`pkg/media/sherpa_tts.go` will implement `media.VoiceCatalog`:
```go
func (s *SherpaTTSClient) ListVoices(ctx context.Context) ([]ProviderVoice, error)
```
- It maps the 11 known Kokoro speakers (`pkg/media/kokoro_voices.go`: `af`, `af_bella`, `af_nicole`, `af_sarah`, `af_sky`, `am_adam`, `am_michael`, `bf_emma`, `bf_isabella`, `bm_george`, `bm_lewis`) into `ProviderVoice` objects.
- Display names, accents, genders, and descriptive tags are derived from `KokoroVoiceProfiles` (`pkg/config/presets.go`).
- Does not require network access or loaded model weights to list voices.

### 2.2 Providers Without Catalogs

Providers that do not implement `media.VoiceCatalog` (such as custom CLI commands or HTTP endpoints) continue returning `catalog.available: false` and `catalog.voices: []`. No errors are raised, and the frontend gracefully renders standard text inputs.

### 2.3 Campaign-Level Narrator Voice Resolution

In `pkg/gui/service.go`:
- `CreateGameRequestDTO` gains an optional `NarratorVoice string json:"narrator_voice,omitempty"` field.
- When set, `CreateGame` saves it into `manifest.Settings["narrator_voice"]`.
- `Service.narratorVoiceFor(gameID string)`:
  - Resolves `cfg.Media.TTS.DefaultVoice` as the baseline.
  - If `gameID` has a `manifest.Settings["narrator_voice"]` string, that voice ID overrides the baseline.
  - Passes this resolved voice to `SynthesizeSegment` and `CountUncached`.
- `Service.UpdateGameSettings` already merges arbitrary keys into `manifest.Settings`, allowing `narrator_voice` to be updated dynamically for an existing campaign.

---

## 3. Frontend Architecture

### 3.1 `VoiceCombobox` Component (`frontend/src/components/VoiceCombobox.tsx`)

A reusable, accessible combobox component for selecting a voice ID:
- **Props**:
  - `value: string`: Current voice ID.
  - `onChange: (voiceID: string) => void`.
  - `voices: ProviderVoice[]`: Available catalog voices.
  - `placeholder?: string`.
  - `disabled?: boolean`.
  - `className?: string`.
- **Behavior**:
  - If `voices.length === 0`:
    - Renders a styled text `<input>` allowing direct manual input of any voice ID.
  - If `voices.length > 0`:
    - Renders an input with an interactive search dropdown.
    - Matches query against voice `name`, `id`, `gender`, `accent`, and `tags`.
    - Dropdown shows:
      - Display name (e.g. `Bella (American Female)`).
      - Monospace voice ID (e.g. `af_bella` or `EXAVITQu4vr4xnSDxMaL`).
      - Category / accent badge.
    - Selecting an item sets `value` to the selected voice ID.
    - Allows free-text entry so users can still type an unlisted ID if desired.
    - If the selected voice has a `preview_url` (or if test synthesis is available), renders an inline preview button.

### 3.2 `VoiceCatalogModal` Component (`frontend/src/components/VoiceCatalogModal.tsx`)

A modal dialog opened from Settings Studio:
- **Header**: "Voice Catalog Browser", with search input and category filter.
- **List**:
  - Audition button (`Play`) using `playVoicePreview`.
  - Display name, ID, tags, and description.
  - "Add as Profile" button (`Plus`).
- **Action**:
  - Clicking "Add as Profile" appends a new `VoiceProfile` to `config.media.tts.voice_profiles`:
    - `id`: slugified from name (e.g. `sarah_premade`)
    - `name`: voice display name
    - `voice_id`: catalog ID
    - `tags`: catalog tags
    - `description`: catalog description
    - `pitch`: 1.0, `speech_rate`: 1.0
    - `options`: catalog defaults (if any)
  - Closes modal or provides toast feedback that the profile was added.

### 3.3 Settings Studio Updates (`frontend/src/components/SettingsStudio.tsx`)

1. **Default Voice Field**:
   - Added in the TTS Provider configuration section (near Preview Phrase and Options).
   - Uses `VoiceCombobox` bound to `config.media.tts.default_voice`.
2. **Profile Cards**:
   - Replaces the raw `voice_id` text input with `VoiceCombobox`.
   - Displays the friendly voice name for known IDs.
3. **Voice Profiles Header**:
   - Adds an **"Import from Catalog"** button next to "Load Fantasy Defaults" and "Add Profile", enabled when `inspect?.catalog?.voices?.length > 0`.

### 3.4 Campaign Creation & In-Game Settings

1. **Campaign Creation Wizard (`frontend/src/components/LauncherHub.tsx`)**:
   - In Step 3 (or alongside character creation), adds a **Narrator Voice** field:
     - Allows selecting a voice profile or catalog voice for the campaign's narrator.
     - Defaults to "Provider Default Voice".
     - Included in `CreateGameRequest`.
2. **In-Game Campaign Settings**:
   - In the game drawer / settings view, exposes the campaign's Narrator Voice setting, saving via `APIClient.updateGameSettings(gameID, { narrator_voice: selectedVoiceID })`.

---

## 4. Error Handling & Edge Cases

1. **Unlisted / Deleted Voice ID**:
   - If a voice profile references an ID no longer present in the catalog, `VoiceCombobox` displays the raw ID in the input without throwing errors or clearing the value.
2. **Offline / Disabled TTS**:
   - When TTS is disabled or fails to inspect, `voices` is empty; all inputs function as regular text fields.
3. **Cache Invalidation on Narrator Voice Change**:
   - The media cache key already includes `voiceID` (`ComputeAudioCacheKeyWithRate`). Changing the campaign narrator voice automatically produces new cache keys without collisions or stale playback.

---

## 5. Verification Plan

1. **Unit Tests (Backend)**:
   - `pkg/media/sherpa_tts_test.go`: Test that `SherpaTTSClient.ListVoices(ctx)` returns 11 voices with correct IDs and attributes.
   - `pkg/gui/tts_inspect_test.go`: Test that `InspectTTS` for `sherpa-onnx` reports `available: true` and 11 voices.
   - `pkg/gui/service_test.go`: Test that `CreateGame` persists `narrator_voice` in `manifest.Settings` and that `Service.SynthesizeSegment` honors it.
2. **Frontend Verification**:
   - `npx tsc --noEmit`: Strict TypeScript compilation passes with zero errors.
   - `mise run build`: Bundles into `pkg/gui/dist` without build errors.
   - `mise run lint`: `go vet ./...` clean.
