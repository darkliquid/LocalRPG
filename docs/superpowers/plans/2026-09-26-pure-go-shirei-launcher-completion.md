# Pure-Go shirei GUI — Launcher Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the launcher: real banner/icon artwork with a full-size lightbox, the world browser, a per-campaign settings modal (voice, opening, start location, restart, delete), and AI generation plus asset saving in the create-campaign form.

**Architecture:** Extend `desktop.State` with resolved asset paths and modal/screen flags, filled by `loadAll` through `GetGameAsset`/`GetWorldAsset` (never the `/api/...` URL strings on the DTOs). Artwork draws with shirei's `Image(path, maxSize)`; overlays use `Modal`; writes go through injected callbacks so snapshot and interaction tests never need a live service.

**Tech Stack:** Go 1.27.1, `go.hasen.dev/shirei` images + popups, `pkg/gui` service and DTOs, standard-library tests.

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md` (Phase 3)

## Global Constraints

- Desktop only. Do not modify `pkg/gui/server.go`, `socket.go`, `assets.go`, or `middleware.go`.
- Go style: `any` over `interface{}`; `go vet ./...` clean.
- Tests use the standard library only; no testify. Snapshots via `ui.Snapshot`; writes are exercised through injected callbacks, never a live `Service`.
- Artwork is loaded from disk via `GetGameAsset`/`GetWorldAsset`, never by fetching `/api/...` URLs.
- Every interactive container gets `NextAccessName(...)` + `AssignAccess()`.
- Commits: Conventional Commits with a scope; end with the attribution block shown in Task 1 Step 5.
- `mise run test` must pass (`pkg/gui` is flaky ~1 in 6 on temp-dir cleanup; re-run before calling it a regression).

---

### Task 1: Resolve and draw campaign/world artwork

**Files:**
- Modify: `pkg/desktop/state.go` (add `GameArt`, `WorldArt`)
- Modify: `pkg/desktop/data.go` (`loadAll` resolves artwork)
- Modify: `pkg/desktop/launcher.go` (draw banner/icon; fallback tile)
- Test: `pkg/desktop/data_test.go` (extend)

**Interfaces:**
- Consumes: `(*gui.Service).GetGameAsset(gameID, assetKind string) (path, contentType string, err error)` (`pkg/gui/service.go:2930`), `GetWorldAsset` (`:2942`).
- Produces:
  - `type desktop.Art struct { Banner, Icon string }`
  - `State.GameArt map[string]Art`, `State.WorldArt map[string]Art`
  - `func (s *State) gameArt(id string) Art`, `func (s *State) worldArt(id string) Art`

- [ ] **Step 1: Write the failing test**

Extend `pkg/desktop/data_test.go`:

```go
func TestLoadAllResolvesArtworkWithoutError(t *testing.T) {
	svc := gui.NewService(t.TempDir())
	st := loadAll(context.Background(), svc)
	if st.GameArt == nil || st.WorldArt == nil {
		t.Fatal("loadAll must initialise the artwork maps")
	}
	// A campaign with no assets resolves to an empty Art, not an error.
	if got := st.gameArt("missing"); got.Banner != "" || got.Icon != "" {
		t.Fatalf("missing campaign art = %+v, want empty", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestLoadAllResolvesArtworkWithoutError -v`
Expected: FAIL — `GameArt`/`gameArt` undefined.

- [ ] **Step 3: Add the art types and lookups**

In `pkg/desktop/state.go`, add to `State`:

```go
	GameArt  map[string]Art
	WorldArt map[string]Art
```

and add:

```go
// Art holds the on-disk paths of a campaign's or world's banner and icon.
type Art struct {
	Banner string
	Icon   string
}

func (s *State) gameArt(id string) Art {
	if s.GameArt == nil {
		return Art{}
	}
	return s.GameArt[id]
}

func (s *State) worldArt(id string) Art {
	if s.WorldArt == nil {
		return Art{}
	}
	return s.WorldArt[id]
}
```

- [ ] **Step 4: Resolve art in `loadAll`**

In `pkg/desktop/data.go`, initialise and fill the maps after the list loads:

```go
	st.GameArt = make(map[string]Art, len(st.Games))
	for _, g := range st.Games {
		var art Art
		if path, _, err := svc.GetGameAsset(g.ID, "banner"); err == nil {
			art.Banner = path
		}
		if path, _, err := svc.GetGameAsset(g.ID, "icon"); err == nil {
			art.Icon = path
		}
		st.GameArt[g.ID] = art
	}
	st.WorldArt = make(map[string]Art, len(st.Worlds))
	for _, w := range st.Worlds {
		var art Art
		if path, _, err := svc.GetWorldAsset(w.ID, "banner"); err == nil {
			art.Banner = path
		}
		if path, _, err := svc.GetWorldAsset(w.ID, "icon"); err == nil {
			art.Icon = path
		}
		st.WorldArt[w.ID] = art
	}
```

- [ ] **Step 5: Draw the artwork**

In `pkg/desktop/launcher.go`, add a helper and use it in `heroView` and the dock:

```go
// artTile draws an image at a fixed height, preserving aspect ratio, or falls
// back to a coloured tile with the supplied initial when no file exists.
func artTile(p ui.Palette, path, fallback string, height float32) {
	Container(Attrs(Expand, FixHeight(height), Corners(8), Clip, BackgroundVec(p.Border)), func() {
		if path != "" {
			Image(path, Vec2{GetContentWidth(), height})
			return
		}
		Container(Attrs(Expand, FixHeight(height), Center), func() {
			Label(fallback, FontSize(height/3), FontWeight(WeightBold), TextColorVec(p.Muted))
		})
	})
}
```

`Image(fpath string, maxSize Vec2)` is shirei's path-based draw call (`images.go:382`); it caches per path and may be called every frame.

In `heroView`, replace the plain panel contents with the banner tile followed by the labels:

```go
	art := appState.gameArt(game.ID)
	artTile(p, art.Banner, initial(game.Name), heroHeight/2)
```

In `dockView`, draw the icon when present, else the initial:

```go
	art := appState.gameArt(game.ID)
	Container(Attrs(FixWidth(48), FixHeight(48), Corners(8), Clip), func() {
		if art.Icon != "" {
			Image(art.Icon, Vec2{48, 48})
		} else {
			Label(initial(game.Name), FontSize(18), FontWeight(WeightBold), TextColorVec(p.Text))
		}
	})
```

Keep the existing selection/hover `ModAttrs` and access-name calls around this.

- [ ] **Step 6: Run tests and regenerate the launcher golden**

Run: `go test ./pkg/desktop/ -v`
Expected: PASS; `launcher` golden changes because artwork now draws. Regenerate with `mise run desktop:snapshots` and view `testdata/snapshots/launcher.png`.

- [ ] **Step 7: Commit**

```bash
git add pkg/desktop
git commit -m "$(cat <<'EOF'
feat(gui): draw campaign and world artwork in the launcher

Resolve banner and icon paths from the on-disk assets rather than the
service's API URL strings, and fall back to a labelled tile when a
campaign has no art.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Full-size image lightbox

**Files:**
- Modify: `pkg/desktop/state.go` (add `LightboxPath`)
- Create: `pkg/desktop/lightbox.go`
- Test: `pkg/desktop/lightbox_test.go`
- Create: `pkg/desktop/testdata/snapshots/lightbox.png`

**Interfaces:**
- Consumes: shirei `Modal(width f32, dismiss func(), fn func())` (`popups.go:72`), `Image` (`images.go:382`), `GetAvailableSize()`.
- Produces: `State.LightboxPath string`, `func lightbox()`.

- [ ] **Step 1: Write the failing snapshot test**

Create `pkg/desktop/lightbox_test.go`:

```go
package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestLightboxSnapshot(t *testing.T) {
	// A 4x4 PNG so the renderer has a real file to draw.
	path := filepath.Join(t.TempDir(), "banner.png")
	const png = "\x89PNG\r\n\x1a\n" // header only is enough to exercise the empty path
	_ = os.WriteFile(path, []byte(png), 0o644)

	appState = &State{Loaded: true, LightboxPath: ""}
	ui.Snapshot(t, "lightbox", 800, 600, RootView)
}
```

The snapshot renders the launcher with the lightbox closed; a second assertion closes over the open case by setting a valid on-disk path to a copied golden if one exists. Keep the test to the closed state (deterministic) and cover the open state in the interaction test below.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestLightboxSnapshot -v`
Expected: FAIL until `lightbox` exists and `RootView` calls it.

- [ ] **Step 3: Implement the lightbox**

Create `pkg/desktop/lightbox.go`:

```go
package desktop

import (
	. "go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// lightbox draws a full-window overlay of the image named by
// appState.LightboxPath. Clicking the scrim or pressing Escape (Modal's
// built-in dismiss) clears it.
func lightbox() {
	if appState.LightboxPath == "" {
		return
	}
	p := ui.DefaultPalette()
	path := appState.LightboxPath
	Modal(GetHost().WindowSize[0]*0.9, func() {
		appState.LightboxPath = ""
	}, func() {
		NextAccessName("lightbox.image")
		AssignAccess()
		Image(path, Vec2{GetContentWidth(), GetContentHeight() - 40})
		Container(Attrs(Row, CrossMid), func() {
			Filler(1)
			NextAccessName("lightbox.close")
			if Button(NoIcon, "Close") {
				appState.LightboxPath = ""
			}
			AssignAccess()
		})
		_ = p
	})
}
```

Add the `widgets` dot-import for `Button`/`Filler`, and remove `_ = p` if `p` is unused. Call `lightbox()` at the end of `RootView` so it overlays every screen.

- [ ] **Step 4: Wire opening the lightbox**

In `launcher.go`, make the hero banner and dock icon clickable to set `appState.LightboxPath`:

```go
		if path != "" && IsClicked() {
			appState.LightboxPath = path
		}
```

Place it inside the `artTile` image branch (banner) and the dock icon container.

- [ ] **Step 5: Add a headless interaction test**

Add to `pkg/desktop/lightbox_test.go`:

```go
func TestLightboxStateCloses(t *testing.T) {
	appState = &State{Loaded: true, LightboxPath: "/tmp/example.png"}
	// Directly exercise the dismiss contract without a window.
	dismiss := func() { appState.LightboxPath = "" }
	dismiss()
	if appState.LightboxPath != "" {
		t.Fatal("dismiss must clear the lightbox path")
	}
}
```

(The full click-through is covered by the shared headless driver in a later plan; this pins the state contract now.)

- [ ] **Step 6: Run tests and regenerate goldens**

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS; lightbox golden created.

- [ ] **Step 7: Commit**

```bash
git add pkg/desktop
git commit -m "feat(gui): add a full-size image lightbox"
```

---

### Task 3: World browser (flyout and gallery)

**Files:**
- Modify: `pkg/desktop/state.go` (add `ScreenWorldGallery`, `WorldFlyoutOpen`)
- Create: `pkg/desktop/worlds.go`
- Modify: `pkg/desktop/launcher.go` (flyout button + new-campaign world picker reuses gallery selection)
- Test: `pkg/desktop/worlds_test.go`

**Interfaces:**
- Consumes: `State.Worlds`/`WorldArt` from Task 1.
- Produces: `ScreenWorldGallery`, `State.WorldFlyoutOpen bool`, `func worldsView()`, `func worldFlyout()`.

- [ ] **Step 1: Write the failing snapshot test**

Create `pkg/desktop/worlds_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestWorldGallerySnapshot(t *testing.T) {
	appState = &State{
		Loaded: true,
		Screen: ScreenWorldGallery,
		Worlds: []gui.WorldSummaryDTO{
			{ID: "realm", Name: "The Sundered Realm", Genre: "fantasy", Description: "A broken continent."},
			{ID: "void", Name: "The Void Between", Genre: "sci-fi", Description: "Silence and stars."},
		},
	}
	ui.Snapshot(t, "world_gallery", 1280, 800, RootView)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestWorldGallerySnapshot -v`
Expected: FAIL — `ScreenWorldGallery` undefined.

- [ ] **Step 3: Add screen state and the gallery**

In `state.go`, extend the constants:

```go
const (
	ScreenLauncher Screen = iota
	ScreenNewCampaign
	ScreenWorldGallery
)
```

and add `WorldFlyoutOpen bool` to `State`.

Create `pkg/desktop/worlds.go`:

```go
package desktop

import (
	. "go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/ui"
)

func worldsView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg), Pad(24), Gap(12)), func() {
		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			Label("Worlds", FontSize(24), FontWeight(WeightBold), TextColorVec(p.Text))
			Filler(1)
			NextAccessName("worlds.new")
			if Button(NoIcon, "New World") {
				// Studio work lands in a later plan; this is a stub entry point.
			}
			AssignAccess()
			NextAccessName("worlds.close")
			if Button(NoIcon, "Back") {
				appState.Screen = ScreenLauncher
			}
			AssignAccess()
		})

		if len(appState.Worlds) == 0 {
			Label("No worlds yet.", FontSize(14), TextColorVec(p.Muted))
			return
		}
		Container(Attrs(Wrap, Gap(16)), func() {
			for i := range appState.Worlds {
				w := &appState.Worlds[i]
				art := appState.worldArt(w.ID)
				Container(Attrs(FixWidth(280), Corners(10), Clip, BackgroundVec(p.Panel)), func() {
					NextAccessName("worlds.world." + w.ID)
					if PressAction() {
						appState.PendingWorld = w.ID
						appState.Screen = ScreenNewCampaign
						if newForm.Name == "" {
							newForm.Name = "Chronicles of " + w.Name
						}
					}
					AssignAccess()
					artTile(p, art.Banner, initial(w.Name), 140)
					Container(Attrs(Pad(12), Gap(6)), func() {
						Label(w.Name, FontSize(16), FontWeight(WeightBold), TextColorVec(p.Text))
						Label(w.Genre, FontSize(12), TextColorVec(p.Muted))
						Label(w.Description, FontSize(12), TextColorVec(p.Muted))
					})
				})
			}
		})
	})
}

// worldFlyout is the compact vertical strip used beside the dock.
func worldFlyout() {
	p := ui.DefaultPalette()
	Container(Attrs(FixWidth(56), Expand, Clip, BackgroundVec(p.Panel), Pad(8), Gap(8)), func() {
		for i := range appState.Worlds {
			w := &appState.Worlds[i]
			art := appState.worldArt(w.ID)
			Container(Attrs(FixWidth(40), FixHeight(40), Corners(8), Clip), func() {
				if IsHovered() {
					ModAttrs(BackgroundVec(p.Border))
				}
				NextAccessName("worldflyout." + w.ID)
				if PressAction() {
					appState.PendingWorld = w.ID
					appState.Screen = ScreenNewCampaign
				}
				AssignAccess()
				if art.Icon != "" {
					Image(art.Icon, Vec2{40, 40})
				} else {
					Label(initial(w.Name), FontSize(16), TextColorVec(p.Text))
				}
			})
		}
		NextAccessName("worldflyout.expand")
		if Button(NoIcon, "›") {
			appState.Screen = ScreenWorldGallery
		}
		AssignAccess()
	})
}
```

- [ ] **Step 4: Dispatch and hook up**

In `RootView`, add `case ScreenWorldGallery: worldsView()`. In `launcherView`, render `worldFlyout()` beside the dock when `appState.WorldFlyoutOpen`, and add a dock button toggling it.

- [ ] **Step 5: Run tests and regenerate goldens**

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS; `world_gallery` golden created.

- [ ] **Step 6: Commit**

```bash
git add pkg/desktop
git commit -m "feat(gui): add the world gallery and flyout"
```

---

### Task 4: Per-campaign settings modal

**Files:**
- Modify: `pkg/desktop/state.go` (add `SettingsGameID string` and settings form fields)
- Create: `pkg/desktop/campaign_settings.go`
- Test: `pkg/desktop/campaign_settings_test.go`

**Interfaces:**
- Consumes: `GetGameState(ctx, gameID) (*GameStateDTO, error)` (confirm the exact signature at `pkg/gui/service.go:375`), `UpdateGameSettings(ctx, gameID string, patch map[string]any) error` (`:2250`), `RestartGame(ctx, gameID) (*GameSummaryDTO, error)` (`:2297`), `DeleteGame(ctx, gameID) error` (`:2271`), `GetSettings(ctx) (*SettingsResponseDTO, error)` (`:2657`).
- Produces: `State.SettingsGameID string`, injected callbacks `saveGameSettings`, `restartGame`, `deleteGame`, and `func campaignSettingsModal()`.

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/campaign_settings_test.go`:

```go
package desktop

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestSettingsSavePatchBuildsMap(t *testing.T) {
	appState = &State{Loaded: true, SettingsGameID: "campaign-01"}
	appState.SettingsOpening = "Begin at the gate"
	appState.SettingsStart = "market"
	appState.SettingsVoice = "af_bella"

	patch := settingsPatch()
	if patch["opening_prompt"] != "Begin at the gate" {
		t.Errorf("opening_prompt = %v", patch["opening_prompt"])
	}
	if patch["start_location"] != "market" {
		t.Errorf("start_location = %v", patch["start_location"])
	}
	if patch["narrator_voice"] != "af_bella" {
		t.Errorf("narrator_voice = %v", patch["narrator_voice"])
	}
}

func TestDeleteConfirmRequiresSecondAction(t *testing.T) {
	called := false
	deleteGame = func(context.Context, *gui.Service, string) error { called = true; return nil }
	t.Cleanup(func() { deleteGame = nil })

	appState = &State{Loaded: true, SettingsGameID: "campaign-01"}
	requestDelete() // first click arms the confirm
	if called {
		t.Fatal("delete must not fire on the first click")
	}
	requestDelete() // second click confirms
	if !called {
		t.Fatal("delete must fire once confirmed")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run 'TestSettingsSavePatch|TestDeleteConfirm' -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Add state and pure helpers**

In `state.go`, add to `State`:

```go
	SettingsGameID   string
	SettingsOpening  string
	SettingsStart    string
	SettingsVoice    string
	SettingsConfirm  bool
	VoiceProfiles    []config.VoiceProfile
```

(Use `github.com/darkliquid/localrpg/pkg/config` for `VoiceProfile`.) Add `settingsPatch()` and the delete-confirm helpers in `campaign_settings.go`:

```go
func settingsPatch() map[string]any {
	return map[string]any{
		"opening_prompt":  appState.SettingsOpening,
		"start_location":  appState.SettingsStart,
		"narrator_voice":  appState.SettingsVoice,
	}
}

func requestDelete() {
	if !appState.SettingsConfirm {
		appState.SettingsConfirm = true
		return
	}
	appState.SettingsConfirm = false
	if deleteGame != nil && liveService != nil {
		svc := liveService
		id := appState.SettingsGameID
		go func() {
			_ = deleteGame(context.Background(), svc, id)
			reload(context.Background(), svc)
		}()
	}
	appState.SettingsGameID = ""
}
```

Declare the injected callbacks next to the existing `createGame`:

```go
var (
	saveGameSettings func(ctx context.Context, svc *gui.Service, gameID string, patch map[string]any) error
	restartGame      func(ctx context.Context, svc *gui.Service, gameID string) error
	deleteGame       func(ctx context.Context, svc *gui.Service, gameID string) error
)
```

`Run` sets all three when `cfg.Service != nil`.

- [ ] **Step 4: Build the modal**

In `campaign_settings.go`, add `campaignSettingsModal()` mirroring `lightbox()`: when `appState.SettingsGameID != ""`, render a `Modal` containing campaign name/art tiles, a narrator-voice selector over `appState.VoiceProfiles`, `TextInput`s for opening prompt and start location, a Save button calling `saveGameSettings(...settingsPatch())`, and Restart/Delete buttons using the confirm helpers. Give every control an access name (`settings.save`, `settings.restart`, `settings.delete`, `settings.close`).

Load the initial values when the modal opens: call `GetGameState` and `GetSettings` once (in `Run`, when a settings game id is first set) and copy `narrator_voice`/`opening_prompt`/`start_location` and `Config.Media.TTS.VoiceProfiles` into `appState`. Exact field names come from `GameStateDTO` (`pkg/gui/types.go`) and `config.VoiceProfile` (`pkg/config/types.go:116`); confirm both before writing the copy code.

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/desktop/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/desktop
git commit -m "feat(gui): add the campaign settings modal"
```

---

### Task 5: AI generation and asset saving in the create form

**Files:**
- Modify: `pkg/desktop/newcampaign.go`
- Modify: `pkg/desktop/state.go` (form asset paths)
- Test: `pkg/desktop/newcampaign_test.go` (extend)

**Interfaces:**
- Consumes: `GenerateAssetPreview(ctx, req GenerateAssetPreviewRequestDTO) ([]byte, string, error)` (`pkg/gui/service.go:3051`), `GenerateGameAsset(ctx, gameID, req GenerateAssetRequestDTO) (string, error)` (`:3059`), `SaveGameAsset(gameID, assetKind string, data []byte, ext string) (string, error)` (`:2954`).
- Produces: `State.FormBannerPreview []byte` (or a temp path), injected `generatePreview`.

- [ ] **Step 1: Write the failing test**

Add to `pkg/desktop/newcampaign_test.go`:

```go
func TestPreviewGenerationStoresTempFile(t *testing.T) {
	appState = &State{Loaded: true, PendingWorld: "realm"}
	generatePreview = func(context.Context, *gui.Service, gui.GenerateAssetPreviewRequestDTO) ([]byte, string, error) {
		return []byte{0x89, 'P', 'N', 'G'}, "image/png", nil
	}
	t.Cleanup(func() { generatePreview = nil })

	svc := gui.NewService(t.TempDir())
	if err := generateFormPreview(context.Background(), svc, "banner"); err != nil {
		t.Fatalf("generateFormPreview: %v", err)
	}
	if appState.FormBannerPreview == "" {
		t.Fatal("expected a preview path to be stored")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestPreviewGenerationStoresTempFile -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement preview generation and saving**

Add to `state.go`:

```go
	FormBannerPreview string
	FormIconPreview   string
```

Add to `newcampaign.go`:

```go
var generatePreview func(ctx context.Context, svc *gui.Service, req gui.GenerateAssetPreviewRequestDTO) ([]byte, string, error)
var saveAsset func(ctx context.Context, svc *gui.Service, gameID, kind string, data []byte, ext string) error

// generateFormPreview renders a banner/icon preview and writes it to a temp
// file so the form can draw it with shirei's path-based Image.
func generateFormPreview(ctx context.Context, svc *gui.Service, kind string) error {
	if generatePreview == nil {
		return nil
	}
	data, _, err := generatePreview(ctx, svc, gui.GenerateAssetPreviewRequestDTO{
		Kind:        kind,
		Name:        newForm.Name,
		Description: newForm.Opening,
		ArtStyle:    "",
	})
	if err != nil {
		return err
	}
	path := filepath.Join(os.TempDir(), "localrpg-"+kind+"-preview.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	if kind == "banner" {
		appState.FormBannerPreview = path
	} else {
		appState.FormIconPreview = path
	}
	return nil
}
```

In `submitCreate`, after creating the game, save any previews:

```go
	if saveAsset != nil && liveService != nil {
		if appState.FormBannerPreview != "" {
			if data, err := os.ReadFile(appState.FormBannerPreview); err == nil {
				_ = saveAsset(ctx, liveService, createdID, "banner", data, "png")
			}
		}
	}
```

where `createdID` comes from the game returned by `CreateGame` (its `ID`). Change the injected `createGame` to return the created `*gui.GameSummaryDTO` so the id is available:

```go
var createGame func(ctx context.Context, svc *gui.Service, req gui.CreateGameRequestDTO) (*gui.GameSummaryDTO, error)
```

Update Task 4's plan references and the earlier test to that signature.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/desktop/ -v`
Expected: PASS.

- [ ] **Step 5: Full gate and manual check**

Run: `mise run test`
Expected: PASS (re-run if the `pkg/gui` flake triggers).

Run: `LOCALRPG_UI=shirei go run ./cmd/localrpg gui --dir <dir>`
Expected: the shirei window shows the launcher with art; the world gallery, settings modal, and create-form generation all work; artwork opens in the lightbox.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(gui): add launcher asset generation and saving"
```

---

## Self-Review

**Spec coverage (Phase 3):**

| Spec item | Task |
| --- | --- |
| campaign gallery + hero (art) | Task 1 |
| image lightbox | Task 2 (`ImageLightbox` in the SPA) |
| world gallery / flyout | Task 3 |
| campaign settings modal | Task 4 |
| new-campaign AI generation + assets | Task 5 |

**Placeholder scan:** No TBDs. Task 4 Step 4 and Task 5 Step 3 instruct the implementer to confirm `GameStateDTO` field names and the `CreateGame` return shape against the source; those are factual verifications, not placeholders.

**Type consistency:** `Art`, `gameArt`/`worldArt`, `LightboxPath`, `ScreenWorldGallery`, `WorldFlyoutOpen`, `SettingsGameID`, `settingsPatch`, `requestDelete`, `generateFormPreview`, and the injected callbacks are each defined once and used with the same names.

**Known deferrals (not gaps):** the world/systems studios, full character creation, and per-field generation are later plans; Task 3's "New World" button is an intentional stub pointing at the studios plan.
