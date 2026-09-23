# Launcher & UI Coherence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate legacy Cinzel fonts and amber styling in favor of modern sans-serif and dark glass/violet across Studios and Tabletop Gameplay, fix World Flyout horizontal scrolling, fix New Campaign modal header background layout, and implement a full campaign settings & creation suite (Narrator Voice, character attributes, opening prompt, start location).

**Architecture:** Extend backend settings DTOs and routes (`PATCH /api/game/{id}/settings`, `GET /api/game/{id}/state`) with `narrator_voice` and `start_location`. Update frontend API client. Implement cross-browser scrollbar utilities in `index.css` and smooth wheel scrolling in `WorldFlyout.tsx`. Restructure `NewCampaignModal.tsx` header with background banner backdrop and full character/narrator setup. Expand `CampaignSettingsModal.tsx` to read and edit narrator voice, start location, opening prompt, and assets. Refactor `WorldsStudio.tsx`, `SystemsStudio.tsx`, `SettingsStudio.tsx`, `App.tsx`, and Tabletop drawers/views to modern sans-serif and purple accents.

**Tech Stack:** Go 1.27, React 19, TypeScript, Tailwind CSS v4, Lucide React, standard library `net/http`.

---

### File Map

#### Backend:
- Modify: `pkg/gui/types.go` (Add fields to `GameSettingsPatchDTO`, `GameStateDTO`, `CreateGameRequestDTO`)
- Modify: `pkg/gui/server.go` (Update `handleUpdateGameSettings` to handle `narrator_voice` and `start_location`)
- Modify: `pkg/gui/service.go` (Populate `NarratorVoice` and `StartLocation` in `GetGameState`, record `StartLocation` in `CreateGame`)
- Create: `pkg/gui/settings_endpoint_test.go` (Test settings patch & retrieval)

#### Frontend:
- Modify: `frontend/src/index.css` (Add `.no-scrollbar` cross-browser utility)
- Modify: `frontend/src/types.ts` (Update `GameState`, `GameSettingsPatch`, `CreateGameRequest`)
- Modify: `frontend/src/api/client.ts` (Add `updateGameSettings` method)
- Modify: `frontend/src/components/launcher/WorldFlyout.tsx` (Scrollbar suppression & layout fix)
- Modify: `frontend/src/components/launcher/NewCampaignModal.tsx` (Header backdrop layout fix, narrator voice, character fields, start location)
- Modify: `frontend/src/components/launcher/CampaignSettingsModal.tsx` (Read & edit narrator voice, start location, opening prompt)
- Modify: `frontend/src/components/WorldsStudio.tsx` (Remove Cinzel & amber palette -> sans-serif & violet)
- Modify: `frontend/src/components/SystemsStudio.tsx` (Remove Cinzel & amber palette -> sans-serif & violet)
- Modify: `frontend/src/components/SettingsStudio.tsx` (Update buttons and badges to violet/sans-serif)
- Modify: `frontend/src/App.tsx` (Tabletop header bar, active campaign, drawer trigger pills)
- Modify: `frontend/src/components/ActionConsole.tsx` (Update badges & buttons)
- Modify: `frontend/src/components/ChronicleView.tsx` (Update badges & accents)
- Modify: `frontend/src/components/CodexDrawer.tsx` (Remove Cinzel & amber)
- Modify: `frontend/src/components/CharacterSheetDrawer.tsx` (Remove Cinzel & amber)
- Modify: `frontend/src/components/LivingWorldDrawer.tsx` (Remove Cinzel & amber)
- Modify: `frontend/src/components/ProloguePanel.tsx` (Remove Cinzel & amber)
- Modify: `frontend/src/components/StoryTheater.tsx` (Remove Cinzel & amber)
- Modify: `frontend/src/components/Drawers.tsx` (Remove Cinzel & amber)
- Modify: `frontend/src/components/AddEntityModal.tsx` (Remove Cinzel & amber)

---

### Task 1: Backend Settings DTOs, Endpoints & Tests

**Files:**
- Modify: `pkg/gui/types.go:38-50, 204-210`
- Modify: `pkg/gui/server.go:180-205`
- Modify: `pkg/gui/service.go:340-365, 1870-1885`
- Create: `pkg/gui/settings_endpoint_test.go`

- [ ] **Step 1: Write failing test in `pkg/gui/settings_endpoint_test.go`**

```go
package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGameSettingsPatchAndState(t *testing.T) {
	tempDir := t.TempDir()

	gameDir := filepath.Join(tempDir, "games", "settings-game")
	if err := os.MkdirAll(filepath.Join(gameDir, "entities"), 0755); err != nil {
		t.Fatalf("mkdir entities: %v", err)
	}

	gameYAML := "id: settings-game\nname: Settings Game\nsystem: test-sys\nworld: test-world\nplayer: hero\n"
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(gameYAML), 0644); err != nil {
		t.Fatalf("write game.yaml: %v", err)
	}

	playerMD := "---\nid: hero\nname: Hero\ntype: character\n---\n"
	if err := os.WriteFile(filepath.Join(gameDir, "entities", "hero.md"), []byte(playerMD), 0644); err != nil {
		t.Fatalf("write hero.md: %v", err)
	}

	svc := NewService(tempDir)
	srv := NewServer(svc, http.NotFoundHandler())

	// Test initial GetGameState has empty narrator_voice and start_location
	state, err := svc.GetGameState(context.Background(), "settings-game")
	if err != nil {
		t.Fatalf("GetGameState: %v", err)
	}
	if state.NarratorVoice != "" {
		t.Errorf("expected empty NarratorVoice, got %q", state.NarratorVoice)
	}
	if state.StartLocation != "" {
		t.Errorf("expected empty StartLocation, got %q", state.StartLocation)
	}

	// Test PATCH /api/game/settings-game/settings
	patchBody := map[string]string{
		"narrator_voice": "en-US-Standard-A",
		"start_location": "tavern-inn",
		"opening_prompt": "You wake up in a quiet inn.",
	}
	bodyBytes, _ := json.Marshal(patchBody)
	req := httptest.NewRequest(http.MethodPatch, "/api/game/settings-game/settings", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("PATCH settings expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify updated GetGameState
	updatedState, err := svc.GetGameState(context.Background(), "settings-game")
	if err != nil {
		t.Fatalf("GetGameState after patch: %v", err)
	}
	if updatedState.NarratorVoice != "en-US-Standard-A" {
		t.Errorf("expected NarratorVoice 'en-US-Standard-A', got %q", updatedState.NarratorVoice)
	}
	if updatedState.StartLocation != "tavern-inn" {
		t.Errorf("expected StartLocation 'tavern-inn', got %q", updatedState.StartLocation)
	}
	if updatedState.OpeningPrompt != "You wake up in a quiet inn." {
		t.Errorf("expected OpeningPrompt 'You wake up in a quiet inn.', got %q", updatedState.OpeningPrompt)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestGameSettingsPatchAndState ./pkg/gui/`
Expected: FAIL (`NarratorVoice undefined` or compilation error)

- [ ] **Step 3: Update `pkg/gui/types.go`**

Update `GameStateDTO`, `GameSettingsPatchDTO`, and `CreateGameRequestDTO` in `pkg/gui/types.go`:
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

type GameSettingsPatchDTO struct {
	OpeningPrompt *string `json:"opening_prompt,omitempty"`
	NarratorVoice *string `json:"narrator_voice,omitempty"`
	StartLocation *string `json:"start_location,omitempty"`
}

type CreateGameRequestDTO struct {
	Name          string             `json:"name"`
	SystemID      string             `json:"system_id"`
	WorldID       string             `json:"world_id"`
	PlayerName    string             `json:"player_name"`
	NarratorVoice string             `json:"narrator_voice,omitempty"`
	StartLocation string             `json:"start_location,omitempty"`
	OpeningPrompt string             `json:"opening_prompt,omitempty"`
	Player        PlayerCharacterDTO `json:"player"`
}
```

- [ ] **Step 4: Update `pkg/gui/service.go` and `pkg/gui/server.go`**

In `pkg/gui/service.go` inside `GetGameState`:
```go
	var narratorVoice, startLocation string
	if gameManifest.Settings != nil {
		if nv, ok := gameManifest.Settings["narrator_voice"].(string); ok {
			narratorVoice = nv
		}
		if sl, ok := gameManifest.Settings[engine.StartLocationSetting].(string); ok {
			startLocation = sl
		}
	}

	return &GameStateDTO{
		GameID:        gameID,
		GameName:      gameManifest.Name,
		Player:        PlayerDTO{...},
		Arcs:          arcs,
		Clocks:        clocks,
		Locations:     locations,
		OpeningPrompt: engine.OpeningPrompt(gameManifest),
		NarratorVoice: narratorVoice,
		StartLocation: startLocation,
	}, nil
```

In `pkg/gui/service.go` inside `CreateGame`:
```go
	if strings.TrimSpace(req.StartLocation) != "" {
		_ = s.UpdateGameSettings(ctx, gameID, map[string]interface{}{
			engine.StartLocationSetting: strings.TrimSpace(req.StartLocation),
		})
	}
```

In `pkg/gui/server.go` inside `case "settings":`:
```go
		values := map[string]interface{}{}
		if patch.OpeningPrompt != nil {
			values[engine.OpeningPromptSetting] = strings.TrimSpace(*patch.OpeningPrompt)
		}
		if patch.NarratorVoice != nil {
			values["narrator_voice"] = strings.TrimSpace(*patch.NarratorVoice)
		}
		if patch.StartLocation != nil {
			values[engine.StartLocationSetting] = strings.TrimSpace(*patch.StartLocation)
		}
		if err := s.service.UpdateGameSettings(r.Context(), gameID, values); err != nil {
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -v -run TestGameSettingsPatchAndState ./pkg/gui/`
Run: `go test -v ./pkg/gui/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/server.go pkg/gui/settings_endpoint_test.go
git commit -m "feat(gui): expand campaign settings with narrator_voice and start_location"
```

---

### Task 2: Frontend Types & API Client

**Files:**
- Modify: `frontend/src/types.ts:60-90, 160-190`
- Modify: `frontend/src/api/client.ts:310-340`

- [ ] **Step 1: Update `frontend/src/types.ts`**

Update `GameState`, `CreateGameRequest`, and `GameSettingsPatch`:
```typescript
export interface GameState {
  game_id: string;
  game_name: string;
  player: PlayerInfo;
  arcs: NarrativeArc[];
  clocks: FactionClock[];
  locations: string[];
  opening_prompt?: string;
  narrator_voice?: string;
  start_location?: string;
}

export interface GameSettingsPatch {
  opening_prompt?: string;
  narrator_voice?: string;
  start_location?: string;
}

export interface CreateGameRequest {
  name: string;
  system_id: string;
  world_id: string;
  player_name: string;
  narrator_voice?: string;
  start_location?: string;
  opening_prompt?: string;
  player?: {
    appearance?: string;
    age?: string;
    gender?: string;
    pronouns?: string;
    background?: string;
    voice_id?: string;
    extra?: Record<string, string>;
  };
}
```

- [ ] **Step 2: Add `updateGameSettings` to `frontend/src/api/client.ts`**

```typescript
  async updateGameSettings(patch: GameSettingsPatch): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/settings`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
    if (!res.ok) {
      throw new HTTPError(res.status, await res.text());
    }
  }

  static async updateGameSettings(gameId: string, patch: GameSettingsPatch): Promise<void> {
    const res = await fetch(`/api/game/${gameId}/settings`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
    if (!res.ok) {
      throw new HTTPError(res.status, await res.text());
    }
  }
```

- [ ] **Step 3: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): add narrator_voice and start_location to types and API client"
```

---

### Task 3: World Selector Popout & Scrollbar Utilities

**Files:**
- Modify: `frontend/src/index.css`
- Modify: `frontend/src/components/launcher/WorldFlyout.tsx`

- [ ] **Step 1: Add `.no-scrollbar` to `frontend/src/index.css`**

Add cross-browser scrollbar hiding rules to `frontend/src/index.css`:
```css
.no-scrollbar::-webkit-scrollbar {
  display: none;
}
.no-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
```

- [ ] **Step 2: Update `frontend/src/components/launcher/WorldFlyout.tsx`**

Refactor `WorldFlyout.tsx`:
- Apply `.no-scrollbar` and horizontal wheel listener (`onWheel={(e) => { e.currentTarget.scrollLeft += e.deltaY; }}`).
- Wrap the items in a container with vertical padding (`py-1.5`) so `hover:scale-105` doesn't clip or create vertical scrollbars.
- Position the tooltips safely with `pointer-events-none absolute left-1/2 -translate-x-1/2 top-full mt-2.5 ... z-50`.
- Use `max-w-[min(720px,calc(100vw-140px))]`.

- [ ] **Step 3: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/index.css frontend/src/components/launcher/WorldFlyout.tsx
git commit -m "fix(frontend): remove forced scrollbars and add smooth wheel scrolling to WorldFlyout"
```

---

### Task 4: New Campaign Modal Header & Form Expansion

**Files:**
- Modify: `frontend/src/components/launcher/NewCampaignModal.tsx`

- [ ] **Step 1: Restructure Modal Header in `NewCampaignModal.tsx`**

Replace header layout with absolute background banner layer and floating content layer:
```tsx
        {/* Modal Banner Header */}
        <div className="relative h-44 w-full overflow-hidden border-b border-white/10 select-none shrink-0">
          {/* Background Banner Backdrop */}
          <div className="absolute inset-0 z-0">
            {world.banner_url ? (
              <img src={world.banner_url} alt={world.name} className="w-full h-full object-cover" />
            ) : (
              <ProceduralBanner id={world.id} name={world.name} className="w-full h-full" />
            )}
            {/* Scrim gradient */}
            <div className="absolute inset-0 bg-gradient-to-t from-stone-900 via-stone-900/60 to-black/30" />
          </div>

          {/* Floating Header Content */}
          <div className="relative z-10 h-full flex items-end p-6">
            <div className="flex items-center gap-4">
              <div className="w-16 h-16 rounded-2xl overflow-hidden shadow-2xl border-2 border-white/20 shrink-0">
                {world.icon_url ? (
                  <img src={world.icon_url} alt={world.name} className="w-full h-full object-cover" />
                ) : (
                  <ProceduralIcon id={world.id} name={world.name} genre={world.genre} size={64} className="w-full h-full rounded-none" />
                )}
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <h2 className="text-xl font-sans font-extrabold text-white tracking-tight">
                    New Campaign
                  </h2>
                  <span className="text-[11px] font-sans font-semibold bg-purple-500/20 text-purple-300 border border-purple-500/30 px-2 py-0.5 rounded-full">
                    {world.name}
                  </span>
                </div>
                <p className="text-xs font-sans text-stone-300 mt-1 line-clamp-2 max-w-lg">
                  {world.description || 'Explore uncharted territory and shape the fate of this realm.'}
                </p>
              </div>
            </div>
          </div>

          {/* Close button */}
          <button
            onClick={onClose}
            className="absolute top-4 right-4 z-20 w-8 h-8 rounded-full bg-black/50 border border-white/20 flex items-center justify-center text-white/80 hover:text-white hover:bg-black/70 transition-all cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
```

- [ ] **Step 2: Add Voice Catalog & Character Fields to Form**

- Fetch voice profiles using `APIClient.getSettings()` or pass voice profiles prop.
- Add Narrator Voice selector:
  ```tsx
  <div className="space-y-1.5">
    <label className="text-xs font-sans font-semibold text-stone-300 uppercase tracking-wider flex items-center gap-1.5">
      <Volume2 className="w-3.5 h-3.5 text-purple-400" />
      <span>Narrator Voice</span>
    </label>
    <select
      value={narratorVoice}
      onChange={(e) => setNarratorVoice(e.target.value)}
      className="w-full bg-stone-950 border border-white/10 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
    >
      <option value="">Default (Provider Setting)</option>
      {voiceProfiles.map((p) => (
        <option key={p.id} value={p.voice_id}>
          {p.name} ({p.voice_id})
        </option>
      ))}
    </select>
  </div>
  ```
- Add Character fields (appearance, age, gender, pronouns, background, character voice).
- Add Start Location input (optional).
- Wire all values into `onCreateGame` call.

- [ ] **Step 3: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/launcher/NewCampaignModal.tsx
git commit -m "feat(frontend): fix NewCampaignModal header layout and add narrator voice, character, and location fields"
```

---

### Task 5: Campaign Settings Modal Expansion

**Files:**
- Modify: `frontend/src/components/launcher/CampaignSettingsModal.tsx`

- [ ] **Step 1: Load and Edit Settings in `CampaignSettingsModal.tsx`**

- On open, call `APIClient.getGameState(game.id)` and `APIClient.getSettings()` to load current `narrator_voice`, `opening_prompt`, `start_location`, and voice profiles.
- Add structured tabs or sections:
  1. **Narrative & Voices**:
     - Narrator Voice `<select>` with voice catalog profiles.
     - Opening Prompt `<textarea>` for GM narrative seed.
     - Starting Location `<input>`.
     - "Save Settings" button calling `APIClient.updateGameSettings(game.id, { narrator_voice, opening_prompt, start_location })`.
  2. **Campaign Artwork**:
     - Existing Banner and Icon previews with Upload & AI Generate buttons.
  3. **Danger Zone**:
     - Restart Campaign and Delete Campaign.

- [ ] **Step 2: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/launcher/CampaignSettingsModal.tsx
git commit -m "feat(frontend): expand CampaignSettingsModal with narrator voice, opening prompt, and start location"
```

---

### Task 6: Design System Migration: Studios (`WorldsStudio`, `SystemsStudio`, `SettingsStudio`)

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`
- Modify: `frontend/src/components/SystemsStudio.tsx`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [ ] **Step 1: Migrate `WorldsStudio.tsx`**

- Replace all `font-cinzel` with `font-sans font-bold`.
- Replace amber accents with purple:
  - `text-amber-400` -> `text-purple-400`
  - `bg-amber-600` / `hover:bg-amber-500` -> `bg-purple-600 hover:bg-purple-500 text-white`
  - `border-amber-500/60` -> `border-purple-500/50`
  - Selected world card: `bg-purple-950/30 border-purple-500/50 shadow-[0_0_15px_rgba(168,85,247,0.15)]`
  - Tab active state: `bg-purple-600 text-white font-bold`

- [ ] **Step 2: Migrate `SystemsStudio.tsx`**

- Replace all `font-cinzel` with `font-sans font-bold`.
- Replace amber accents with purple:
  - `text-amber-400` -> `text-purple-400`
  - `bg-amber-600` / `hover:bg-amber-500` -> `bg-purple-600 hover:bg-purple-500 text-white`
  - Selected system card: `bg-purple-950/30 border-purple-500/50`
  - Tab active state: `bg-purple-600 text-white font-bold`

- [ ] **Step 3: Migrate `SettingsStudio.tsx`**

- Replace amber save buttons and active indicators with `bg-purple-600 hover:bg-purple-500 text-white shadow-md`.
- Replace `border-amber-*` and `text-amber-400` with `border-purple-500/40` and `text-purple-400`.

- [ ] **Step 4: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx frontend/src/components/SystemsStudio.tsx frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): migrate Studios from Cinzel and amber to sans-serif and violet"
```

---

### Task 7: Design System Migration: Tabletop Gameplay & Drawers

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/components/ActionConsole.tsx`
- Modify: `frontend/src/components/ChronicleView.tsx`
- Modify: `frontend/src/components/CodexDrawer.tsx`
- Modify: `frontend/src/components/CharacterSheetDrawer.tsx`
- Modify: `frontend/src/components/LivingWorldDrawer.tsx`
- Modify: `frontend/src/components/ProloguePanel.tsx`
- Modify: `frontend/src/components/StoryTheater.tsx`
- Modify: `frontend/src/components/Drawers.tsx`
- Modify: `frontend/src/components/AddEntityModal.tsx`

- [ ] **Step 1: Migrate `App.tsx` header and controls**

- Change `font-cinzel` in header to `font-sans font-bold`.
- Change Campaigns button: `bg-white/5 hover:bg-white/10 text-stone-200 border border-white/10`.
- Change active game dot: `bg-purple-500 shadow-[0_0_8px_rgba(168,85,247,0.8)]`.
- Change drawer pills active state: `bg-purple-600 text-white font-bold shadow-md`.

- [ ] **Step 2: Migrate Drawers (`CodexDrawer.tsx`, `CharacterSheetDrawer.tsx`, `LivingWorldDrawer.tsx`, `Drawers.tsx`)**

- Replace `font-cinzel` with `font-sans`.
- Replace `text-amber-400` / `bg-amber-600` with `text-purple-400` / `bg-purple-600`.

- [ ] **Step 3: Migrate Action Console, Chronicle & Views (`ActionConsole.tsx`, `ChronicleView.tsx`, `ProloguePanel.tsx`, `StoryTheater.tsx`, `AddEntityModal.tsx`)**

- Replace `font-cinzel` with `font-sans`.
- Replace `text-amber-400` / `bg-amber-600` with `text-purple-400` / `bg-purple-600`.

- [ ] **Step 4: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/App.tsx frontend/src/components/ActionConsole.tsx frontend/src/components/ChronicleView.tsx frontend/src/components/CodexDrawer.tsx frontend/src/components/CharacterSheetDrawer.tsx frontend/src/components/LivingWorldDrawer.tsx frontend/src/components/ProloguePanel.tsx frontend/src/components/StoryTheater.tsx frontend/src/components/Drawers.tsx frontend/src/components/AddEntityModal.tsx
git commit -m "feat(frontend): migrate Tabletop gameplay shell and drawers to sans-serif and violet"
```

---

### Task 8: Full Verification & Build

- [ ] **Step 1: Run complete test suite**

Run: `mise run test`
Expected: PASS

- [ ] **Step 2: Run production build**

Run: `mise run build`
Expected: SUCCESS

- [ ] **Step 3: Restore `.gitkeep` and check git status**

Run: `git checkout pkg/gui/dist/.gitkeep`
Run: `git status`
Expected: Clean working tree

- [ ] **Step 4: Mark plan complete and commit**

```bash
git add docs/superpowers/plans/2026-09-23-launcher-and-ui-coherence.md
git commit -m "docs: mark launcher and ui coherence plan complete"
```
