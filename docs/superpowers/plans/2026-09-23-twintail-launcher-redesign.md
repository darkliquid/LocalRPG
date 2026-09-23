# Twintail-Inspired Launcher Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign LocalRPG's launcher into an atmospheric, game-first desktop experience inspired by the Twintail Launcher, featuring a vertical dock, hero stage, horizontal world flyout, dual-image asset model (banner + icon) with procedural fallbacks, AI generation, and custom file uploads.

**Architecture:** Extend backend DTOs and API routes with asset serving, multipart upload, and AI generation endpoints; build modular React components (`LauncherDock`, `WorldFlyout`, `CampaignHeroStage`, `NewCampaignModal`, `CampaignSettingsModal`, `ProceduralAsset`) with clean modern sans-serif typography; render Worlds/Systems studios as full-window overlays and settings as centered glassmorphic modals.

**Tech Stack:** Go (1.27.1), TypeScript, React 19, Tailwind CSS v4, Lucide React.

---

### File Map

| Action | File | Responsibility |
|---|---|---|
| Modify | `pkg/gui/types.go` | Add `banner_url`, `icon_url`, `play_time_seconds` to `GameSummaryDTO` & `WorldSummaryDTO` |
| Modify | `pkg/gui/service.go` | Asset disk helpers, populated DTOs in `ListGames`/`ListWorlds`, upload & AI asset generation |
| Create | `pkg/gui/assets_endpoint_test.go` | Tests for asset serving, upload, and generation routes |
| Modify | `pkg/gui/server.go` | HTTP routes for `/api/game/{id}/banner`, `icon`, `generate-asset`, and `/api/world/{id}/...` |
| Modify | `frontend/src/types.ts` | Mirror updated `GameSummary` and `WorldInfo` asset fields |
| Modify | `frontend/src/api/client.ts` | Methods for asset upload and AI generation |
| Create | `frontend/src/components/launcher/ProceduralAsset.tsx` | Deterministic FNV-1a mesh gradients and genre/monogram icons |
| Create | `frontend/src/components/launcher/LauncherDock.tsx` | Left vertical navigation rail (`+`, campaigns, studios, settings) |
| Create | `frontend/src/components/launcher/WorldFlyout.tsx` | Horizontal world icon expansion drawer |
| Create | `frontend/src/components/launcher/CampaignHeroStage.tsx` | Main hero presentation (banner, stats card, title card, play button, zero-states) |
| Create | `frontend/src/components/launcher/NewCampaignModal.tsx` | Centered modal with world banner header, system selector, character answers |
| Create | `frontend/src/components/launcher/CampaignSettingsModal.tsx` | Centered modal for artwork management, metadata, restart, and delete |
| Modify | `frontend/src/components/LauncherHub.tsx` | Coordinator wiring new dock, hero stage, flyout, modals, and full-window studios |

---

### Task 1: Backend DTOs, Asset Helpers & Asset Endpoints

**Files:**
- Modify: `pkg/gui/types.go:120-150`
- Modify: `pkg/gui/service.go:1620-1760`
- Modify: `pkg/gui/server.go:60-120`
- Create: `pkg/gui/assets_endpoint_test.go`

- [x] **Step 1: Write failing test in `pkg/gui/assets_endpoint_test.go`**

Create `pkg/gui/assets_endpoint_test.go` testing asset retrieval, upload, and DTO population:

```go
package gui

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
)

func createTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestAssetEndpointsAndSummary(t *testing.T) {
	tempDir := t.TempDir()
	resolver := core.NewPathResolver(tempDir)

	// Create dummy game and world directories
	gameDir := resolver.GameDir("test-game")
	if err := os.MkdirAll(filepath.Join(gameDir, "assets"), 0755); err != nil {
		t.Fatalf("mkdir game: %v", err)
	}
	worldDir := resolver.WorldDir("test-world")
	if err := os.MkdirAll(filepath.Join(worldDir, "assets"), 0755); err != nil {
		t.Fatalf("mkdir world: %v", err)
	}

	// Write manifests
	gameYAML := "id: test-game\nname: Test Game\nsystem: test-sys\nworld: test-world\nplayer: Hero\n"
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(gameYAML), 0644); err != nil {
		t.Fatalf("write game.yaml: %v", err)
	}
	worldYAML := "id: test-world\nname: Test World\ndescription: A world\ngenre: fantasy\n"
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatalf("write world.yaml: %v", err)
	}

	// Write a banner file into game assets
	pngBytes := createTestPNG(t)
	if err := os.WriteFile(filepath.Join(gameDir, "assets", "banner.png"), pngBytes, 0644); err != nil {
		t.Fatalf("write banner.png: %v", err)
	}

	cfgMgr := config.NewManagerFromPath(filepath.Join(tempDir, "config.yaml"))
	svc := NewServiceWithResolver(resolver, cfgMgr)
	srv := NewServer(svc)

	// Test ListGames carries BannerURL
	games, err := svc.ListGames(context.Background())
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}
	if games[0].BannerURL != "/api/game/test-game/banner" {
		t.Errorf("expected BannerURL /api/game/test-game/banner, got %q", games[0].BannerURL)
	}
	if games[0].IconURL != "" {
		t.Errorf("expected empty IconURL, got %q", games[0].IconURL)
	}

	// Test GET /api/game/test-game/banner
	req := httptest.NewRequest(http.MethodGet, "/api/game/test-game/banner", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET banner expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "image/png" {
		t.Errorf("expected Content-Type image/png, got %q", rr.Header().Get("Content-Type"))
	}

	// Test POST /api/game/test-game/icon upload
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "icon.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(pngBytes); err != nil {
		t.Fatalf("write part: %v", err)
	}
	writer.Close()

	uploadReq := httptest.NewRequest(http.MethodPost, "/api/game/test-game/icon", &body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRR := httptest.NewRecorder()
	srv.ServeHTTP(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusOK {
		t.Fatalf("POST icon expected 200, got %d: %s", uploadRR.Code, uploadRR.Body.String())
	}

	// Verify icon file was written to disk
	if _, err := os.Stat(filepath.Join(gameDir, "assets", "icon.png")); err != nil {
		t.Errorf("expected icon.png to exist on disk: %v", err)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestAssetEndpointsAndSummary ./pkg/gui/`
Expected: FAIL (`BannerURL undefined` or compilation error)

- [x] **Step 3: Update `pkg/gui/types.go`**

Update `GameSummaryDTO` and `WorldSummaryDTO` in `pkg/gui/types.go`:

```go
type GameSummaryDTO struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	SystemID        string `json:"system_id"`
	WorldID         string `json:"world_id"`
	PlayerName      string `json:"player_name"`
	TurnCount       int    `json:"turn_count"`
	LastPlayed      string `json:"last_played"`
	ThumbnailURL    string `json:"thumbnail_url"`
	BannerURL       string `json:"banner_url,omitempty"`
	IconURL         string `json:"icon_url,omitempty"`
	PlayTimeSeconds int64  `json:"play_time_seconds,omitempty"`
}

type WorldSummaryDTO struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Genre             string   `json:"genre"`
	CompatibleSystems []string `json:"compatible_systems"`
	BannerURL         string   `json:"banner_url,omitempty"`
	IconURL           string   `json:"icon_url,omitempty"`
}
```

- [x] **Step 4: Implement asset helpers, DTO population & handlers in `pkg/gui/service.go` and `pkg/gui/server.go`**

In `pkg/gui/service.go`:
Add asset resolution helpers:
```go
func findAssetFile(dir string, name string) (string, string) {
	assetsDir := filepath.Join(dir, "assets")
	exts := []string{".png", ".webp", ".jpg", ".jpeg", ".svg"}
	for _, ext := range exts {
		path := filepath.Join(assetsDir, name+ext)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path, ext
		}
	}
	return "", ""
}
```

Update `ListGames` to populate `BannerURL` and `IconURL`:
```go
		var bannerURL, iconURL string
		if p, _ := findAssetFile(gameDir, "banner"); p != "" {
			bannerURL = fmt.Sprintf("/api/game/%s/banner", gameID)
		}
		if p, _ := findAssetFile(gameDir, "icon"); p != "" {
			iconURL = fmt.Sprintf("/api/game/%s/icon", gameID)
		}
```

Update `ListWorlds` to populate `BannerURL` and `IconURL`:
```go
		var bannerURL, iconURL string
		if p, _ := findAssetFile(worldDir, "banner"); p != "" {
			bannerURL = fmt.Sprintf("/api/world/%s/banner", m.ID)
		}
		if p, _ := findAssetFile(worldDir, "icon"); p != "" {
			iconURL = fmt.Sprintf("/api/world/%s/icon", m.ID)
		}
```

Add methods `GetGameAsset`, `GetWorldAsset`, `SaveGameAsset`, `SaveWorldAsset`, `GenerateGameAsset`, `GenerateWorldAsset` on `Service`.

In `pkg/gui/server.go`:
Mount routes:
```go
	mux.HandleFunc("GET /api/game/{id}/banner", s.handleGetGameAsset("banner"))
	mux.HandleFunc("GET /api/game/{id}/icon", s.handleGetGameAsset("icon"))
	mux.HandleFunc("POST /api/game/{id}/banner", s.handleUploadGameAsset("banner"))
	mux.HandleFunc("POST /api/game/{id}/icon", s.handleUploadGameAsset("icon"))
	mux.HandleFunc("POST /api/game/{id}/generate-asset", s.handleGenerateGameAsset)

	mux.HandleFunc("GET /api/world/{id}/banner", s.handleGetWorldAsset("banner"))
	mux.HandleFunc("GET /api/world/{id}/icon", s.handleGetWorldAsset("icon"))
	mux.HandleFunc("POST /api/world/{id}/banner", s.handleUploadWorldAsset("banner"))
	mux.HandleFunc("POST /api/world/{id}/icon", s.handleUploadWorldAsset("icon"))
	mux.HandleFunc("POST /api/world/{id}/generate-asset", s.handleGenerateWorldAsset)
```

- [x] **Step 5: Run tests to verify they pass**

Run: `go test -v -run TestAssetEndpointsAndSummary ./pkg/gui/`
Run: `go test -v ./pkg/gui/`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/server.go pkg/gui/assets_endpoint_test.go
git commit -m "feat(gui): add campaign and world banner and icon endpoints"
```

---

### Task 2: Frontend Types & API Client Methods

**Files:**
- Modify: `frontend/src/types.ts:140-170`
- Modify: `frontend/src/api/client.ts:300-360`

- [x] **Step 1: Update `frontend/src/types.ts`**

Update `GameSummary` and `WorldInfo`:
```typescript
export interface GameSummary {
  id: string;
  name: string;
  system_id: string;
  world_id: string;
  player_name: string;
  turn_count: number;
  last_played: string;
  thumbnail_url?: string;
  banner_url?: string;
  icon_url?: string;
  play_time_seconds?: number;
}

export interface WorldInfo {
  id: string;
  name: string;
  description: string;
  genre: string;
  compatible_systems: string[];
  banner_url?: string;
  icon_url?: string;
}
```

- [x] **Step 2: Add asset upload & generate methods to `frontend/src/api/client.ts`**

Add static methods to `APIClient`:
```typescript
  static async uploadGameAsset(gameId: string, kind: 'banner' | 'icon', file: File): Promise<{ url: string }> {
    const formData = new FormData();
    formData.append('file', file);
    const res = await fetch(`/api/game/${encodeURIComponent(gameId)}/${kind}`, {
      method: 'POST',
      body: formData,
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  static async uploadWorldAsset(worldId: string, kind: 'banner' | 'icon', file: File): Promise<{ url: string }> {
    const formData = new FormData();
    formData.append('file', file);
    const res = await fetch(`/api/world/${encodeURIComponent(worldId)}/${kind}`, {
      method: 'POST',
      body: formData,
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  static async generateGameAsset(gameId: string, kind: 'banner' | 'icon', prompt?: string): Promise<{ url: string }> {
    return this.post(`/api/game/${encodeURIComponent(gameId)}/generate-asset`, { kind, prompt });
  }

  static async generateWorldAsset(worldId: string, kind: 'banner' | 'icon', prompt?: string): Promise<{ url: string }> {
    return this.post(`/api/world/${encodeURIComponent(worldId)}/generate-asset`, { kind, prompt });
  }
```

- [x] **Step 3: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): add asset types and API client upload and generate methods"
```

---

### Task 3: Procedural Asset Generator (`ProceduralAsset.tsx`)

**Files:**
- Create: `frontend/src/components/launcher/ProceduralAsset.tsx`

- [x] **Step 1: Implement `ProceduralAsset.tsx`**

Implement deterministic hashing, themed gradient palettes, `ProceduralBanner`, and `ProceduralIcon`:

```typescript
import React from 'react';
import {
  Shield,
  Sword,
  Compass,
  Rocket,
  Atom,
  Skull,
  Ghost,
  Cpu,
  Flame,
  TreePine,
  Sparkles,
  LucideIcon,
} from 'lucide-react';

export function hashString(str: string): number {
  let hash = 0x811c9dc5;
  for (let i = 0; i < str.length; i++) {
    hash ^= str.charCodeAt(i);
    hash += (hash << 1) + (hash << 4) + (hash << 7) + (hash << 8) + (hash << 24);
  }
  return hash >>> 0;
}

const PALETTES = [
  { name: 'Arcane', bg: '#090a0f', from: '#4c1d95', via: '#1e1b4b', to: '#312e81', accent: '#a855f7' },
  { name: 'Void', bg: '#050811', from: '#1e293b', via: '#0f172a', to: '#0284c7', accent: '#38bdf8' },
  { name: 'Deepwood', bg: '#061009', from: '#14532d', via: '#052e16', to: '#166534', accent: '#22c55e' },
  { name: 'Ember', bg: '#140804', from: '#7c2d12', via: '#431407', to: '#9a3412', accent: '#f97316' },
  { name: 'Crimson', bg: '#130508', from: '#881337', via: '#4c0519', to: '#9f1239', accent: '#f43f5e' },
  { name: 'Eldritch', bg: '#041014', from: '#134e4a', via: '#042f2e', to: '#115e59', accent: '#14b8a6' },
];

export function getPalette(id: string) {
  const hash = hashString(id || 'default');
  return PALETTES[hash % PALETTES.length];
}

const GENRE_ICONS: Array<{ match: RegExp; icon: LucideIcon }> = [
  { match: /fantasy|magic|sorcery|dragon|dnd|rpg/i, icon: Sword },
  { match: /sci-?fi|space|cyber|futur/i, icon: Rocket },
  { match: /cyberpunk|neon|tech|hack/i, icon: Cpu },
  { match: /horror|dark|grim|void|death|undead/i, icon: Skull },
  { match: /nature|wild|forest|beast/i, icon: TreePine },
  { match: /mystery|stealth|shadow|ghost/i, icon: Ghost },
  { match: /war|battle|shield|iron/i, icon: Shield },
  { match: /apocalypse|wasteland|flame|fire/i, icon: Flame },
];

export function getGenreIcon(genreOrTag?: string): LucideIcon | null {
  if (!genreOrTag) return null;
  for (const entry of GENRE_ICONS) {
    if (entry.match.test(genreOrTag)) return entry.icon;
  }
  return null;
}

export function getMonogram(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

export const ProceduralBanner: React.FC<{
  id: string;
  name?: string;
  className?: string;
  style?: React.CSSProperties;
}> = ({ id, className = '', style }) => {
  const p = getPalette(id);
  const hash = hashString(id);
  const angle = (hash % 360);

  return (
    <div
      className={`relative w-full h-full overflow-hidden ${className}`}
      style={{
        backgroundColor: p.bg,
        backgroundImage: `radial-gradient(ellipse at 75% 30%, ${p.from} 0%, ${p.via} 50%, ${p.bg} 100%), linear-gradient(${angle}deg, ${p.to}22, transparent)`,
        ...style,
      }}
    >
      {/* Cinematic tabletop ambient texture */}
      <div className="absolute inset-0 bg-gradient-to-t from-black/85 via-black/20 to-black/60 pointer-events-none" />
      <div className="absolute inset-0 bg-radial-[circle_at_center] from-transparent via-transparent to-black/60 pointer-events-none" />
    </div>
  );
};

export const ProceduralIcon: React.FC<{
  id: string;
  name: string;
  genre?: string;
  size?: number;
  className?: string;
}> = ({ id, name, genre, size = 48, className = '' }) => {
  const p = getPalette(id);
  const IconComponent = getGenreIcon(genre) || getGenreIcon(name);
  const monogram = getMonogram(name);

  return (
    <div
      className={`relative flex items-center justify-center rounded-xl overflow-hidden font-sans font-bold select-none border border-white/15 shadow-md ${className}`}
      style={{
        width: size,
        height: size,
        background: `linear-gradient(135deg, ${p.from} 0%, ${p.via} 100%)`,
        color: '#ffffff',
      }}
    >
      <div className="absolute inset-0 bg-white/5 pointer-events-none" />
      {IconComponent ? (
        <IconComponent style={{ width: size * 0.48, height: size * 0.48, color: p.accent }} />
      ) : (
        <span style={{ fontSize: size * 0.38, letterSpacing: '0.05em' }}>{monogram}</span>
      )}
    </div>
  );
};
```

- [x] **Step 2: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/launcher/ProceduralAsset.tsx
git commit -m "feat(frontend): add ProceduralAsset component for banners and icons"
```

---

### Task 4: Launcher Dock & World Flyout (`LauncherDock.tsx` & `WorldFlyout.tsx`)

**Files:**
- Create: `frontend/src/components/launcher/LauncherDock.tsx`
- Create: `frontend/src/components/launcher/WorldFlyout.tsx`

- [x] **Step 1: Implement `LauncherDock.tsx`**

Build the `72px` left navigation rail:
- `+` button at top with active toggle state.
- Scrollable list of campaigns by icon avatar (`ProceduralIcon` or `img` tag if `icon_url` set).
- Active campaign highlight with glowing vertical indicator pill bar.
- Tooltip on hover showing campaign name and last played timestamp.
- Bottom utility icons: `Globe` (Worlds Studio), `BookOpen` (Systems Studio), `Settings` (Global Settings).

- [x] **Step 2: Implement `WorldFlyout.tsx`**

Build the horizontal expandable world flyout:
- Anchored to the right of the `+` button.
- Horizontal scroll container with world icons (`ProceduralIcon` or `img`).
- Hover tooltip displaying full `World Name (Genre)`.
- Trailing `+ Create World` tile linking to Worlds Studio.
- Clicking a world calls `onSelectWorld(worldId)`.

- [x] **Step 3: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/launcher/LauncherDock.tsx frontend/src/components/launcher/WorldFlyout.tsx
git commit -m "feat(frontend): add LauncherDock and WorldFlyout components"
```

---

### Task 5: Hero Stage & Modals (`CampaignHeroStage.tsx`, `NewCampaignModal.tsx`, `CampaignSettingsModal.tsx`)

**Files:**
- Create: `frontend/src/components/launcher/CampaignHeroStage.tsx`
- Create: `frontend/src/components/launcher/NewCampaignModal.tsx`
- Create: `frontend/src/components/launcher/CampaignSettingsModal.tsx`

- [x] **Step 1: Implement `CampaignHeroStage.tsx`**

Build the main hero presentation:
- Background: `ProceduralBanner` or `img` tag if `banner_url` present, with vignette overlays.
- Top-right stats card: `TurnCount`, `LastPlayed` formatted relative date, and `PlayTimeSeconds`.
- Bottom-left title card: Campaign icon, Title (`font-sans font-bold`), Subtitle (`World: [Name] • System: [Name] • Hero: [Protagonist]`).
- Bottom-right: ⚙️ campaign settings button and prominent gradient "▶ PLAY" button.
- Zero-states:
  - If no campaigns exist, but worlds exist: display atmospheric prompt to choose a world, automatically keeping flyout open.
  - If no campaigns AND no worlds exist: display prominent onboarding card to create first world or browse systems.

- [x] **Step 2: Implement `NewCampaignModal.tsx`**

Build the centered new campaign creation dialog:
- Header: World banner image (or `ProceduralBanner`), world icon badge, world title, description snippet, close button.
- Body: Campaign name input, compatible systems selector pills, character fields (name, appearance, background, voice), art buttons (Upload / Generate AI).
- Footer: Cancel and "Create Campaign" action buttons.

- [x] **Step 3: Implement `CampaignSettingsModal.tsx`**

Build the centered campaign settings modal:
- Artwork card: Banner and Icon previews, Upload button (file picker), Generate AI button.
- General info: Campaign name, protagonist name.
- Danger zone: Inline Restart Campaign and Delete Campaign actions with confirmation.

- [x] **Step 4: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/launcher/CampaignHeroStage.tsx frontend/src/components/launcher/NewCampaignModal.tsx frontend/src/components/launcher/CampaignSettingsModal.tsx
git commit -m "feat(frontend): add CampaignHeroStage, NewCampaignModal, and CampaignSettingsModal"
```

---

### Task 6: LauncherHub Integration & Full-Window Studios

**Files:**
- Modify: `frontend/src/components/LauncherHub.tsx`

- [x] **Step 1: Refactor `LauncherHub.tsx`**

Integrate the new components:
- Replace old tab-based layout with:
  - `LauncherDock` pinned to the left.
  - `WorldFlyout` triggered by `+`.
  - `CampaignHeroStage` as the default main screen.
  - `NewCampaignModal` shown when a world is selected from flyout.
  - `CampaignSettingsModal` shown when clicking ⚙️ on the hero stage.
  - `SettingsStudio` shown as a centered modal when clicking ⚙️ in the dock.
  - `WorldsStudio` and `SystemsStudio` rendered as full-window overlays with a prominent "← Back to Launcher" top button when activated from the dock.

- [x] **Step 2: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 3: Run backend and frontend tests**

Run: `mise run test`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/LauncherHub.tsx
git commit -m "feat(frontend): integrate Twintail-inspired launcher layout into LauncherHub"
```

---

### Task 7: Full Verification & Build

- [x] **Step 1: Run complete test suite**

Run: `mise run test`
Expected: PASS

- [x] **Step 2: Run production build**

Run: `mise run build`
Expected: SUCCESS

- [x] **Step 3: Restore `.gitkeep` and check git status**

Run: `git checkout pkg/gui/dist/.gitkeep`
Run: `git status`
Expected: Clean working tree

- [x] **Step 4: Mark plan complete and commit**

```bash
git add docs/superpowers/plans/2026-09-23-twintail-launcher-redesign.md
git commit -m "docs: mark twintail launcher redesign plan complete"
```
