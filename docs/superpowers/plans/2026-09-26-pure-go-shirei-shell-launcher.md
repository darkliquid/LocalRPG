# Pure-Go shirei GUI — Shell Boot and Launcher Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the shirei shell reachable behind `LOCALRPG_UI=shirei`, give it an application state loaded from `gui.Service`, and build the launcher's core screens: a campaign list (dock + gallery), a world picker, and a create-campaign form.

**Architecture:** `pkg/desktop` gains a `State` loaded from `*gui.Service` and rendered by screens; `Run` accepts the service. `cmd/localrpg/gui.go` gains an env-gated branch that reuses the already-constructed service and calls `desktop.Run`, leaving the Wails/HTTP/SPA paths untouched. Views read a package-level `appState` and re-render on `RequestNextFrame`, which makes golden snapshots deterministic from a seeded state.

**Tech Stack:** Go 1.27.1, `go.hasen.dev/shirei` (+ `/widgets`), `pkg/gui` (`Service` and its DTOs), standard-library tests.

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md` (Phase 3)

## Global Constraints

- Desktop only; the SPA/Wails paths must keep working. Do not modify `pkg/gui/server.go`, `socket.go`, `assets.go`, or `middleware.go`.
- Go style: `any` over `interface{}`; `go vet ./...` clean.
- Tests use the standard library only; no testify. Snapshots use `ui.Snapshot`; interaction tests use shirei's `RunFrameFn` + `GetInputState`/`GetFrameInput` pattern.
- Artwork is loaded from disk via `GetGameAsset`/`GetWorldAsset`, never by fetching the `/api/...` URL strings on the DTOs.
- All interactive containers get `NextAccessName(...)` + `AssignAccess()` so later drive tests can target them.
- Mono audio, `any`, and the existing service behaviour are unchanged.
- Commits: Conventional Commits with a scope; end with the attribution block shown in Task 1 Step 4.
- `mise run test` must pass (`pkg/gui` is flaky ~1 in 6 on temp-dir cleanup; re-run before calling it a regression).

---

### Task 1: Boot the shirei shell behind `LOCALRPG_UI=shirei`

**Files:**
- Modify: `pkg/desktop/run.go` (add `Service`, `State` to `Config`)
- Modify: `pkg/desktop/root.go` (seed state)
- Modify: `cmd/localrpg/gui.go` (env branch + import)
- Test: `pkg/desktop/run_test.go`

**Interfaces:**
- Consumes: `gui.NewService(rootDir string) *gui.Service` (`pkg/gui/service.go:76`).
- Produces:
  - `type desktop.Config struct { Service *gui.Service; Dir string; PNGPath string; Width int; Height int; State *State }`
  - `type desktop.State struct { Games []gui.GameSummaryDTO; Worlds []gui.WorldSummaryDTO; Systems []gui.SystemSummaryDTO; Loaded bool }` (declared in Task 2)

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/run_test.go`:

```go
package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunWithSeededStateRendersPNG(t *testing.T) {
	out := filepath.Join(t.TempDir(), "shell.png")
	st := &State{Loaded: true}
	err := Run(Config{PNGPath: out, Width: 400, Height: 300, State: st})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The file is written by RenderToPNG; existence is the observable contract.
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected PNG at %s: %v", out, err)
	}
}

func TestRunRequiresStateOrService(t *testing.T) {
	err := Run(Config{PNGPath: filepath.Join(t.TempDir(), "x.png"), Width: 100, Height: 100})
	if err == nil {
		t.Fatal("Run must fail when neither State nor Service is provided")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestRun -v`
Expected: FAIL — `State` and the `Config` fields are undefined.

- [ ] **Step 3: Extend `Config` and `Run`**

In `pkg/desktop/run.go`, replace `Config` and `Run`:

```go
package desktop

import (
	"context"
	"errors"
	"fmt"

	"go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// Config configures the desktop application shell. Exactly one of State or
// Service must be set: State for deterministic snapshots, Service for a live
// app.
type Config struct {
	Service *gui.Service
	Dir     string
	PNGPath string
	Width   int
	Height  int
	State   *State
}

// Run starts the desktop GUI, or renders a single headless frame when
// Config.PNGPath is set.
func Run(cfg Config) error {
	if cfg.Width == 0 {
		cfg.Width = 1280
	}
	if cfg.Height == 0 {
		cfg.Height = 800
	}

	switch {
	case cfg.State != nil:
		appState = cfg.State
	case cfg.Service != nil:
		appState = loadAll(context.Background(), cfg.Service)
	default:
		return errors.New("desktop: Config needs State or Service")
	}

	if cfg.PNGPath != "" {
		return shirei.RenderToPNG(cfg.PNGPath, cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	}
	ui.Run("LocalRPG", cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	return nil
}

var _ = fmt.Sprintf
```

Delete the trailing `var _ = fmt.Sprintf` and the `fmt` import if unused.

- [ ] **Step 4: Add the env branch in `cmd/localrpg/gui.go`**

Add the import:

```go
	"github.com/darkliquid/localrpg/pkg/desktop"
```

Immediately after `svc := gui.NewService(cfg.Dir)` (`gui.go:63`), before telemetry, insert:

```go
	// Transitional escape hatch: run the in-process shirei GUI instead of the
	// Wails window or the HTTP daemon. Removed at teardown.
	if os.Getenv("LOCALRPG_UI") == "shirei" {
		if err := desktop.Run(desktop.Config{Dir: cfg.Dir, Service: svc}); err != nil {
			fmt.Fprintf(os.Stderr, "shirei GUI failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
```

This branch deliberately skips telemetry and the trace logger, which `desktop.Run`/`ui.Run` do not use yet.

- [ ] **Step 5: Run the test**

Run: `go test ./pkg/desktop/ -run TestRun -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/desktop cmd/localrpg/gui.go
git commit -m "$(cat <<'EOF'
feat(gui): boot the shirei shell behind LOCALRPG_UI=shirei

The in-process GUI needs a way to run while the SPA still ships. Add a
transitional env switch that reuses the existing service and starts the
shirei shell, leaving the Wails and HTTP paths untouched.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Application state and background data loading

**Files:**
- Create: `pkg/desktop/state.go`
- Create: `pkg/desktop/data.go`
- Test: `pkg/desktop/data_test.go`

**Interfaces:**
- Consumes from Task 1: `desktop.Config`, `desktop.Run` referencing `State` and `loadAll`.
- Produces:
  - `type desktop.State struct { Games []gui.GameSummaryDTO; Worlds []gui.WorldSummaryDTO; Systems []gui.SystemSummaryDTO; Loaded bool; Err error }`
  - `type desktop.Screen int` with `ScreenLauncher`, `ScreenNewCampaign`
  - `var appState *State` (package-level)
  - `func loadAll(ctx context.Context, svc *gui.Service) *State`
  - `func reload(ctx context.Context, svc *gui.Service)`

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/data_test.go`:

```go
package desktop

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestLoadAllIgnoresServiceErrors(t *testing.T) {
	// A fresh service with no games/systems/worlds directories must not error.
	svc := gui.NewService(t.TempDir())
	st := loadAll(context.Background(), svc)
	if !st.Loaded {
		t.Fatal("loadAll must mark the state loaded even when directories are empty")
	}
	if st.Games == nil || st.Worlds == nil || st.Systems == nil {
		t.Fatal("loadAll must return non-nil slices so views can range safely")
	}
	if st.Err != nil {
		t.Fatalf("loadAll reported %v", st.Err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestLoadAllIgnoresServiceErrors -v`
Expected: FAIL — `loadAll` undefined.

- [ ] **Step 3: Write the state**

Create `pkg/desktop/state.go`:

```go
package desktop

import "github.com/darkliquid/localrpg/pkg/gui"

// Screen identifies which top-level view the shell renders.
type Screen int

const (
	ScreenLauncher Screen = iota
	ScreenNewCampaign
)

// State is the desktop application's cached data. It is replaced wholesale by
// loadAll/reload and read by every view during a frame.
type State struct {
	Games   []gui.GameSummaryDTO
	Worlds  []gui.WorldSummaryDTO
	Systems []gui.SystemSummaryDTO
	Loaded  bool
	Err     error

	// Selected is the campaign highlighted in the dock/hero.
	Selected string
	// Screen is the active top-level view.
	Screen Screen
	// PendingWorld is the world chosen for a new campaign.
	PendingWorld string
}

// appState is read by views and replaced under the frame lock by the loader.
var appState = &State{}

// SelectedGame returns the currently selected campaign, or nil.
func (s *State) SelectedGame() *gui.GameSummaryDTO {
	for i := range s.Games {
		if s.Games[i].ID == s.Selected {
			return &s.Games[i]
		}
	}
	return nil
}

// WorldName returns the display name for a world id, falling back to the id.
func (s *State) WorldName(id string) string {
	for _, w := range s.Worlds {
		if w.ID == id {
			return w.Name
		}
	}
	return id
}

// SystemName returns the display name for a system id, falling back to the id.
func (s *State) SystemName(id string) string {
	for _, sys := range s.Systems {
		if sys.ID == id {
			return sys.Name
		}
	}
	return id
}
```

- [ ] **Step 4: Write the loader**

Create `pkg/desktop/data.go`:

```go
package desktop

import (
	"context"

	"go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/gui"
)

// loadAll reads the launcher data from the service. Directory-level failures
// (missing games/worlds/systems) are treated as empty lists, matching the SPA
// which swallowed each request's error independently.
func loadAll(ctx context.Context, svc *gui.Service) *State {
	st := &State{Loaded: true}

	if games, err := svc.ListGames(ctx); err == nil {
		st.Games = games
	} else {
		st.Games = []gui.GameSummaryDTO{}
	}
	if worlds, err := svc.ListWorlds(ctx); err == nil {
		st.Worlds = worlds
	} else {
		st.Worlds = []gui.WorldSummaryDTO{}
	}
	if systems, err := svc.ListSystems(ctx); err == nil {
		st.Systems = systems
	} else {
		st.Systems = []gui.SystemSummaryDTO{}
	}

	// Keep a selection that still exists; otherwise select the first campaign.
	if st.SelectedGame() == nil {
		st.Selected = ""
		if len(st.Games) > 0 {
			st.Selected = st.Games[0].ID
		}
	}
	return st
}

// reload refreshes the cached state from a background goroutine and asks the
// UI to redraw.
func reload(ctx context.Context, svc *gui.Service) {
	next := loadAll(ctx, svc)
	shirei.WithFrameLock(func() {
		prevScreen := appState.Screen
		prevWorld := appState.PendingWorld
		next.Screen = prevScreen
		next.PendingWorld = prevWorld
		*appState = *next
	})
	shirei.RequestNextFrame()
}
```

- [ ] **Step 5: Verify the signature names against the service**

Run: `rg -n 'func \(s \*Service\) List(Games|Worlds|Systems)\(' pkg/gui/service.go`
Expected: three matches at `service.go:1940/2054/2024`. If names differ, use the actual spellings and update this plan's references.

- [ ] **Step 6: Run the test and vet**

Run: `go test ./pkg/desktop/ -run TestLoadAllIgnoresServiceErrors -v`
Expected: PASS.

Run: `go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add pkg/desktop/state.go pkg/desktop/data.go pkg/desktop/data_test.go
git commit -m "feat(gui): add desktop app state and background data loading"
```

---

### Task 3: Launcher shell — dock, campaign gallery, and hero

**Files:**
- Modify: `pkg/desktop/root.go`
- Create: `pkg/desktop/launcher.go`
- Modify: `pkg/desktop/root_test.go`
- Test: `pkg/desktop/launcher_test.go`
- Create: `pkg/desktop/testdata/snapshots/launcher.png` (written by the test)

**Interfaces:**
- Consumes: `State`, `appState`, `SelectedGame`, `WorldName`, `SystemName` from Task 2; `NextAccessName`/`AssignAccess`.
- Produces:
  - `func desktop.RootView()` (unchanged name; now dispatches on `appState.Screen`)
  - `func launcherView()` (unexported)

- [ ] **Step 1: Write the failing snapshot test**

Create `pkg/desktop/launcher_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestLauncherSnapshot(t *testing.T) {
	appState = &State{
		Loaded:   true,
		Selected: "campaign-01",
		Games: []gui.GameSummaryDTO{
			{ID: "campaign-01", Name: "The Hollow Crown", WorldID: "realm", SystemID: "dnd5e", PlayerName: "Vance", TurnCount: 12},
			{ID: "campaign-02", Name: "Salt and Ash", WorldID: "realm", SystemID: "dnd5e", PlayerName: "Mira", TurnCount: 3},
		},
		Worlds:  []gui.WorldSummaryDTO{{ID: "realm", Name: "The Sundered Realm"}},
		Systems: []gui.SystemSummaryDTO{{ID: "dnd5e", Name: "Dungeons & Dragons 5e"}},
	}
	ui.Snapshot(t, "launcher", 1280, 800, RootView)
}
```

Update `pkg/desktop/root_test.go` so `TestRootViewSnapshot` seeds `appState` (`&State{Loaded: true}`) before rendering, since `RootView` now dispatches on it.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestLauncherSnapshot -v`
Expected: FAIL — the root view renders the placeholder title, so either the snapshot is created with wrong content or the test does not compile against the new fields.

- [ ] **Step 3: Build the launcher views**

Create `pkg/desktop/launcher.go`. Use the root package's dot-import for layout (`Row`, `Gap`, `Pad`, `BackgroundVec`, `Viewport`, `Expand`, `Grow`, `Clip`, `Label`, `PressAction`, `IsHovered`, `ModAttrs`, `Corners`, `NextAccessName`, `AssignAccess`) and the `widgets` dot-import for `Button`, `Filler`, `Spacer`.

```go
package desktop

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

const (
	dockWidth  = 72.0
	heroHeight = 220.0
)

func launcherView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg)), func() {
		Container(Attrs(Row, Expand, Grow(1), Clip), func() {
			dockView(p)
			Container(Attrs(Grow(1), Expand, Clip, Pad(24)), func() {
				heroView(p)
				Spacer(24)
				campaignListView(p)
			})
		})
	})
}

func dockView(p ui.Palette) {
	Container(Attrs(FixWidth(dockWidth), Expand, Clip, BackgroundVec(p.Panel), Pad(8), Gap(8)), func() {
		NextAccessName("launcher.new-campaign")
		if Button(NoIcon, "+") {
			appState.Screen = ScreenNewCampaign
		}
		AssignAccess()

		for i := range appState.Games {
			game := &appState.Games[i]
			selected := game.ID == appState.Selected
			Container(Attrs(FixWidth(48), FixHeight(48), Corners(8), Clip), func() {
				if selected {
					ModAttrs(BackgroundVec(p.Accent))
				} else if IsHovered() {
					ModAttrs(BackgroundVec(p.Border))
				}
				NextAccessName("launcher.game." + game.ID)
				if PressAction() {
					appState.Selected = game.ID
				}
				AssignAccess()
				Label(initial(game.Name), FontSize(18), FontWeight(WeightBold), TextColorVec(p.Text))
			})
		}
	})
}

func heroView(p ui.Palette) {
	game := appState.SelectedGame()
	Container(Attrs(Expand, FixHeight(heroHeight), Corners(12), Clip, BackgroundVec(p.Panel), Pad(20)), func() {
		if game == nil {
			Label("No campaigns yet", FontSize(20), FontWeight(WeightBold), TextColorVec(p.Text))
			Label("Create one with the + button.", FontSize(13), TextColorVec(p.Muted))
			return
		}
		Label(game.Name, FontSize(28), FontWeight(WeightBold), TextColorVec(p.Text))
		Label(appState.WorldName(game.WorldID)+" · "+appState.SystemName(game.SystemID), FontSize(14), TextColorVec(p.Muted))
		Spacer(8)
		Label(game.PlayerName+" · "+turnLabel(game.TurnCount), FontSize(13), TextColorVec(p.Muted))
	})
}

func campaignListView(p ui.Palette) {
	Label("Campaigns", FontSize(15), FontWeight(WeightBold), TextColorVec(p.Text))
	Spacer(8)
	if len(appState.Games) == 0 {
		Label("No campaigns yet.", FontSize(13), TextColorVec(p.Muted))
		return
	}
	for i := range appState.Games {
		game := &appState.Games[i]
		selected := game.ID == appState.Selected
		Container(Attrs(Expand, FixHeight(56), Corners(8), Pad(12), BackgroundVec(p.Panel)), func() {
			if selected {
				ModAttrs(BackgroundVec(p.Border))
			} else if IsHovered() {
				ModAttrs(BackgroundVec(p.Border))
			}
			NextAccessName("launcher.campaign." + game.ID)
			if PressAction() {
				appState.Selected = game.ID
			}
			AssignAccess()
			Container(Attrs(Row, CrossMid, Gap(10)), func() {
				Label(game.Name, FontSize(15), FontWeight(WeightBold), TextColorVec(p.Text))
				Filler(1)
				Label(appState.WorldName(game.WorldID), FontSize(12), TextColorVec(p.Muted))
			})
		})
		Spacer(6)
	}
}

// initial returns the first rune of a name for the dock tile.
func initial(name string) string {
	for _, r := range name {
		return string(r)
	}
	return "?"
}

func turnLabel(n int) string {
	if n == 1 {
		return "1 turn"
	}
	return strconv.Itoa(n) + " turns"
}
```

Add `"strconv"` to the imports. Use `strconv.Itoa`; do not hand-roll an integer formatter.

- [ ] **Step 4: Dispatch in the root view**

Replace `pkg/desktop/root.go`'s `RootView`:

```go
package desktop

import (
	. "go.hasen.dev/shirei"
)

// RootView renders the application frame for the active screen.
func RootView() {
	switch appState.Screen {
	case ScreenNewCampaign:
		newCampaignView()
	default:
		launcherView()
	}
}
```

- [ ] **Step 5: Run the snapshot and confirm it is stable**

Run: `go test ./pkg/desktop/ -run TestLauncherSnapshot -v`
Expected: PASS, logging `snapshot launcher: created`. Re-run to confirm `match`.

View `pkg/desktop/testdata/snapshots/launcher.png` to confirm the dock, hero, and campaign rows render legibly.

- [ ] **Step 6: Commit**

```bash
git add pkg/desktop
git commit -m "feat(gui): add the launcher shell with a campaign list"
```

---

### Task 4: Create-campaign flow — world picker and form

**Files:**
- Create: `pkg/desktop/newcampaign.go`
- Test: `pkg/desktop/newcampaign_test.go`
- Create: `pkg/desktop/testdata/snapshots/new_campaign.png`

**Interfaces:**
- Consumes: the state and shell from Tasks 2-3; `(*gui.Service).CreateGame` when run live.
- Produces:
  - `func newCampaignView()` (unexported)
  - `type newCampaignForm struct { Name, PlayerName, Opening string; SystemID string }` stored in a package-level `var newForm = newCampaignForm{PlayerName: "Adventurer"}`

- [ ] **Step 1: Write the failing snapshot test**

Create `pkg/desktop/newcampaign_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestNewCampaignSnapshot(t *testing.T) {
	appState = &State{
		Loaded:       true,
		Screen:       ScreenNewCampaign,
		PendingWorld: "realm",
		Worlds:       []gui.WorldSummaryDTO{{ID: "realm", Name: "The Sundered Realm"}},
		Systems:      []gui.SystemSummaryDTO{{ID: "dnd5e", Name: "Dungeons & Dragons 5e"}},
	}
	newForm = newCampaignForm{PlayerName: "Adventurer"}
	ui.Snapshot(t, "new_campaign", 1280, 800, RootView)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestNewCampaignSnapshot -v`
Expected: FAIL — `newCampaignView`/`newCampaignForm` undefined.

- [ ] **Step 3: Implement the flow**

Create `pkg/desktop/newcampaign.go`:

```go
package desktop

import (
	"context"
	"strconv"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

type newCampaignForm struct {
	Name       string
	PlayerName string
	Opening    string
	SystemID   string
}

var newForm = newCampaignForm{PlayerName: "Adventurer"}

// createGame is injected by Run when a live service is present; snapshot tests
// leave it nil and only render.
var createGame func(ctx context.Context, svc *gui.Service, req gui.CreateGameRequestDTO) error

func newCampaignView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg), Pad(24), Gap(12)), func() {
		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			Label("New Campaign", FontSize(24), FontWeight(WeightBold), TextColorVec(p.Text))
			Filler(1)
			NextAccessName("new-campaign.cancel")
			if Button(NoIcon, "Cancel") {
				appState.Screen = ScreenLauncher
			}
			AssignAccess()
		})

		Label("World", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		Container(Attrs(Row, Gap(8), Wrap), func() {
			for _, w := range appState.Worlds {
				world := w
				selected := world.ID == appState.PendingWorld
				Container(Attrs(Pad(10), Corners(8), BackgroundVec(p.Panel)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					} else if IsHovered() {
						ModAttrs(BackgroundVec(p.Border))
					}
					NextAccessName("new-campaign.world." + world.ID)
					if PressAction() {
						appState.PendingWorld = world.ID
						if newForm.Name == "" {
							newForm.Name = "Chronicles of " + world.Name
						}
					}
					AssignAccess()
					Label(world.Name, FontSize(14), TextColorVec(p.Text))
				})
			}
		})

		Label("System", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		Container(Attrs(Row, Gap(8), Wrap), func() {
			for _, s := range appState.Systems {
				system := s
				selected := system.ID == newForm.SystemID
				Container(Attrs(Pad(10), Corners(8), BackgroundVec(p.Panel)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					} else if IsHovered() {
						ModAttrs(BackgroundVec(p.Border))
					}
					NextAccessName("new-campaign.system." + system.ID)
					if PressAction() {
						newForm.SystemID = system.ID
					}
					AssignAccess()
					Label(system.Name, FontSize(14), TextColorVec(p.Text))
				})
			}
		})

		Label("Campaign name", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		TextInput(&newForm.Name)
		Label("Player name", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		TextInput(&newForm.PlayerName)
		Label("Opening prompt", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		TextInput(&newForm.Opening)

		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			Filler(1)
			NextAccessName("new-campaign.create")
			if canCreate() && Button(NoIcon, "Create") {
				submitCreate()
			}
			AssignAccess()
		})
	})
}

func canCreate() bool {
	return appState.PendingWorld != "" && newForm.SystemID != "" && newForm.Name != ""
}

func submitCreate() {
	req := gui.CreateGameRequestDTO{
		Name:          newForm.Name,
		WorldID:       appState.PendingWorld,
		SystemID:      newForm.SystemID,
		PlayerName:    newForm.PlayerName,
		OpeningPrompt: newForm.Opening,
	}
	if createGame == nil {
		appState.Screen = ScreenLauncher
		return
	}
	svc := liveService
	if svc == nil {
		appState.Screen = ScreenLauncher
		return
	}
	go func() {
		_ = createGame(context.Background(), svc, req)
		reload(context.Background(), svc)
	}()
	appState.Screen = ScreenLauncher
}
```

Add `var liveService *gui.Service` in `state.go` or `run.go`, set by `Run` when `cfg.Service != nil`. In `Run`, when a live service is present, also set `createGame` so `submitCreate` can call it. For snapshot tests both stay nil, so rendering is deterministic.

- [ ] **Step 4: Run the snapshot and a headless interaction test**

Add to `pkg/desktop/newcampaign_test.go`:

```go
func TestNewCampaignCreateInvokesCallback(t *testing.T) {
	appState = &State{Loaded: true, Screen: ScreenNewCampaign, PendingWorld: "realm"}
	liveService = nil
	newForm = newCampaignForm{Name: "Test", PlayerName: "Hero", SystemID: "dnd5e"}
	called := false
	createGame = func(context.Context, *gui.Service, gui.CreateGameRequestDTO) error {
		called = true
		return nil
	}
	t.Cleanup(func() { createGame = nil })

	submitCreate()
	if !called {
		t.Fatal("submitCreate must invoke the injected createGame callback")
	}
	if appState.Screen != ScreenLauncher {
		t.Fatal("submitCreate must return to the launcher")
	}
}
```

Run: `go test ./pkg/desktop/ -v`
Expected: PASS, with `new_campaign` golden created then matching on re-run.

- [ ] **Step 5: Verify the live path manually**

Run: `LOCALRPG_UI=shirei go run ./cmd/localrpg gui --dir <a-dir-with-a-campaign>`
Expected: the shirei window opens on the launcher; the `+` opens the new-campaign form; selecting a world/system enables Create. Close the window afterwards.

- [ ] **Step 6: Full gate**

Run: `mise run test`
Expected: PASS (re-run if the `pkg/gui` flake triggers).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(gui): add the shirei create-campaign flow"
```

---

## Self-Review

**Spec coverage (Phase 3):**

| Spec item | Task |
| --- | --- |
| `LOCALRPG_UI=shirei` boots the shell | Task 1 |
| drawer/shell frame | Tasks 2-3 (top-level shell; drawers come with the core loop in the next plan) |
| launcher/hub campaign gallery + hero | Task 3 |
| new-campaign modal (world selection + create) | Task 4 |

**Deferred within the launcher (tracked, not gaps):** banner/icon artwork and lightbox, procedural art fallbacks, world gallery/flyout expansions, AI text/asset generation in the form, per-campaign settings modal (restart/delete/voice), and sort/search in the gallery. These go into the next plan ("launcher completion") so Tasks 3-4 stay right-sized.

**Placeholder scan:** No TBDs. Task 2 Step 5 and Task 3 Step 3 instruct the implementer to confirm service method and shirei identifier spellings against the source; that is factual verification, not a placeholder.

**Type consistency:** `State`, `Screen`, `appState`, `loadAll`, `reload`, `liveService`, `newCampaignForm`, `createGame`, `RootView`, `launcherView`, `newCampaignView` are each defined once and referenced consistently. GUI DTO field names come from `pkg/gui/types.go:150/164/171/183`.
