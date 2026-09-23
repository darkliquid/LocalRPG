# Design Specification: Launcher & Studios Design Coherence Pass

- **Date:** 2026-09-23
- **Status:** Approved
- **Topic:** Refining Launcher rough edges, unifying typography & color palette across studios and tabletop gameplay, expanding campaign creation & settings, and fixing dialog headers.

---

## 1. Overview & Goals

Following the Twintail-inspired campaign launcher redesign, several areas remain inconsistent or rough:
1. **World Selector Flyout**: Displays an unwanted horizontal scrollbar track and cramped styling.
2. **Studios Theme Mismatch**: `WorldsStudio`, `SystemsStudio`, and `SettingsStudio` still use legacy `font-cinzel` and amber color palettes (`amber-500`, `amber-600`, `amber-950`).
3. **Tabletop Gameplay UI Mismatch**: The active gameplay interface (`App.tsx`, `ActionConsole`, `ChronicleView`, `Drawers`, `CodexDrawer`, `CharacterSheetDrawer`, `LivingWorldDrawer`, `ProloguePanel`, `StoryTheater`) also retains `font-cinzel` and amber accents instead of matching the launcher's modern dark glass and violet aesthetic.
4. **Campaign Settings & Creation Completeness**: `NewCampaignModal` and `CampaignSettingsModal` lack important campaign-level configuration, specifically the **Narrator Voice** selector, player voice, starting location, and opening prompt seed.
5. **New Campaign Dialog Header Layout**: The banner image in `NewCampaignModal` behaves as a flex item pushing content off-screen to the right rather than acting as an absolute background backdrop behind the header scrim.

This design unifies all visual surfaces onto a single modern dark-glass design language, resolves layout and scrolling bugs, and provides a comprehensive settings suite.

---

## 2. Design System & Typography Migration

### 2.1 Complete Removal of Cinzel
- Replace all usages of `font-cinzel` with modern sans-serif typography (`font-sans`, Inter/system default) throughout the entire codebase:
  - `frontend/src/components/WorldsStudio.tsx`
  - `frontend/src/components/SystemsStudio.tsx`
  - `frontend/src/components/SettingsStudio.tsx`
  - `frontend/src/App.tsx`
  - `frontend/src/components/ActionConsole.tsx`
  - `frontend/src/components/ChronicleView.tsx`
  - `frontend/src/components/CodexDrawer.tsx`
  - `frontend/src/components/CharacterSheetDrawer.tsx`
  - `frontend/src/components/LivingWorldDrawer.tsx`
  - `frontend/src/components/ProloguePanel.tsx`
  - `frontend/src/components/StoryTheater.tsx`
  - `frontend/src/components/Drawers.tsx`
  - `frontend/src/components/AddEntityModal.tsx`
  - `frontend/src/components/TurnSegments.tsx`

### 2.2 Global Styling & Palette (`frontend/src/index.css`)
- **Accent Color**: Transition from amber (`#f59e0b`, `amber-500`, `amber-600`) to crisp modern violet/purple:
  - Primary button: `bg-purple-600 hover:bg-purple-500 text-white shadow-[0_0_15px_rgba(168,85,247,0.35)] active:scale-95`
  - Secondary button: `bg-white/[0.05] hover:bg-white/[0.1] text-stone-200 border border-white/10`
  - Focus rings: `focus:border-purple-500/60 focus:ring-1 focus:ring-purple-500/40`
  - Accent text & icons: `text-purple-400`
  - Selected cards: `bg-purple-950/30 border-purple-500/50 shadow-[0_0_15px_rgba(168,85,247,0.15)]`
- **Cross-Browser Scrollbar Hiding**:
  Add utility classes in `index.css`:
  ```css
  .no-scrollbar::-webkit-scrollbar {
    display: none;
  }
  .no-scrollbar {
    -ms-overflow-style: none;
    scrollbar-width: none;
  }
  ```

---

## 3. Component Refinements

### 3.1 World Selector Popout (`WorldFlyout.tsx`)
- Apply `.no-scrollbar` to container elements with `overflow-x-auto`.
- Wrap the items in a container with vertical padding (`py-1.5`) so hover-zoom (`hover:scale-105`) does not cause vertical overflow clipping or trigger scrollbars.
- Position item tooltips using fixed or detached positioning so tooltips cannot inflate container dimensions.
- Support smooth horizontal wheel scrolling using an `onWheel` handler that maps deltaY to deltaX.

### 3.2 New Campaign Dialog Header (`NewCampaignModal.tsx`)
- **Layout Architecture**:
  The modal header will be structured as:
  1. Outer container: `relative h-44 w-full overflow-hidden border-b border-white/10 select-none`.
  2. Background layer: `absolute inset-0 z-0` containing the banner image or `ProceduralBanner`, plus a bottom gradient overlay `absolute inset-0 bg-gradient-to-t from-stone-900 via-stone-900/60 to-black/30`.
  3. Content layer: `relative z-10 h-full flex items-end p-6 gap-4`, containing the world icon badge, campaign title, world genre pill, and world description snippet.
  4. Top-right close button: `absolute top-4 right-4 z-20`.
- **Form Controls Expansion**:
  - **Narrator Voice Selector**: Dropdown listing available TTS voice profiles from config / voice catalog, with an option for "Default (Provider Setting)".
  - **Player Voice Selector**: Dropdown for protagonist voice synthesis.
  - **Protagonist Profile**: Player name, plus character appearance, age, gender/pronouns, and background.
  - **Story Seed & Location**: Opening prompt textarea and optional starting location.
  - **Artwork**: Banner & icon upload controls with live preview.

### 3.3 Campaign Settings Modal (`CampaignSettingsModal.tsx`)
- On mount / game load, fetch `APIClient.getGameState(game.id)` to load current `narrator_voice`, `opening_prompt`, and `start_location`.
- Structured sections:
  1. **Narrative & Voices**:
     - Narrator Voice `<select>` with voice catalog profiles.
     - Opening Prompt textarea.
     - Starting Location input.
  2. **Campaign Artwork**:
     - Banner & Icon previews.
     - "Upload Image" file picker buttons.
     - "Generate with AI" button with loading spinner.
  3. **Danger Zone**:
     - Inline Restart Campaign button with confirmation dialog.
     - Inline Delete Campaign button with confirmation dialog.
- Persist settings via `PATCH /api/game/{id}/settings`.

### 3.4 Tabletop Gameplay Interface (`App.tsx` & Drawers)
- Top Navigation Bar:
  - Modern sans-serif text, violet active drawer indicators, clean translucent acrylic background.
- Drawer triggers:
  - Active tabs (`character`, `graph`, `codex`, `world`) use `bg-purple-600 text-white shadow-md`.
- Action Console:
  - Turn counter badge, dice notation triggers, and submit button updated to violet/stone styling.

---

## 4. Backend API & DTO Extensions

### 4.1 Types (`pkg/gui/types.go`)
- **`GameSettingsPatchDTO`**:
  ```go
  type GameSettingsPatchDTO struct {
      OpeningPrompt *string `json:"opening_prompt,omitempty"`
      NarratorVoice *string `json:"narrator_voice,omitempty"`
      StartLocation *string `json:"start_location,omitempty"`
  }
  ```
- **`GameStateDTO`**:
  ```go
  type GameStateDTO struct {
      GameID        string            `json:"game_id"`
      GameName      string            `json:"game_name"`
      Player        PlayerDTO         `json:"player"`
      Arcs          []NarrativeArcDTO `json:"arcs"`
      Clocks        []FactionClockDTO `json:"clocks"`
      Locations     []string          `json:"locations"`
      OpeningPrompt string            `json:"opening_prompt,omitempty"`
      NarratorVoice string            `json:"narrator_voice,omitempty"`
      StartLocation string            `json:"start_location,omitempty"`
  }
  ```

### 4.2 Server & Service (`pkg/gui/server.go` & `pkg/gui/service.go`)
- `handleUpdateGameSettings`:
  Extract `NarratorVoice` and `StartLocation` if non-nil and pass to `s.service.UpdateGameSettings(ctx, gameID, values)`.
- `GetGameState`:
  Extract `manifest.Settings["narrator_voice"]` and `manifest.Settings[engine.StartLocationSetting]` and populate them onto `GameStateDTO`.
- `CreateGameRequestDTO` & `CreateGame`:
  Ensure `narrator_voice` and `start_location` fields are saved to `manifest.Settings` on creation if provided.

---

## 5. Verification & Testing

- **Backend Unit Tests**:
  - Test `PATCH /api/game/{id}/settings` updates `narrator_voice`, `opening_prompt`, and `start_location`.
  - Test `GET /api/game/{id}/state` returns these settings.
  - Test `POST /api/game` persists `narrator_voice` and `start_location`.
- **Frontend Verification**:
  - `mise run test:frontend` (`tsc --noEmit`) passes with zero errors.
  - Full test suite `mise run test` passes.
  - Production build `mise run build` compiles successfully.
