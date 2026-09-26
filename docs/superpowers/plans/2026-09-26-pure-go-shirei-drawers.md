# Pure-Go shirei GUI — Drawers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the single-active drawer set beside the chronicle: Codex (entities, notes, memories, portraits, merge), Context (turn telemetry), Graph (entity graph), Living World (arcs, clocks, threads, recap), and Character Sheet.

**Architecture:** A drawer is an ordinary fixed-width layout panel in the shell's `Row`, not a popup: shirei's popups self-dismiss on outside click and trap focus, which is wrong for a panel you read while playing. `desktop.State.Drawer` selects one of the five; each reads state loaded through the service and re-renders on `RequestNextFrame`. The graph has no shirei canvas API, so it renders offscreen into an `*image.RGBA` (via `golang.org/x/image/vector`) and is shown with `UseImage`/`ImageView`.

**Tech Stack:** Go 1.27.1, `go.hasen.dev/shirei` (`TextArea`, `TabStrip`/`TabItem`, `VirtualListView`, `Modal`, `UseImage`/`ImageView`), `golang.org/x/image/vector`, `pkg/gui` service + DTOs, standard-library tests.

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md` (Phase 4, drawers)

## Global Constraints

- Desktop only. Do not modify `pkg/gui/server.go`, `socket.go`, `assets.go`, or `middleware.go`.
- Drawers are layout panels (`Container` with `FixWidth`/`Grow`), never `Popup`/`PopupPanel`/`Modal`. `Modal` is only for the merge and delete confirmations.
- All service calls are in-process reads/writes; never fetch `/api/...`.
- Go style: `any`; `go vet ./...` clean. Tests standard-library only.
- Every interactive container gets `NextAccessName(...)` + `AssignAccess()`.
- Commits: Conventional Commits with a scope; end with the attribution block shown in Task 1 Step 4.
- `mise run test` must pass (`pkg/gui` is flaky ~1 in 6; re-run before calling it a regression).

---

### Task 1: The drawer host and tab strip

**Files:**
- Modify: `pkg/desktop/state.go` (add `Drawer string`)
- Create: `pkg/desktop/drawer.go`
- Modify: `pkg/desktop/chronicle.go` (render the drawer beside the turn list)
- Test: `pkg/desktop/drawer_test.go`

**Interfaces:**
- Consumes: shirei `TabStrip(tabs, trailing func())` / `TabItem(key any, label string, selected bool, content func()) bool` (`widgets/tabs.go:20,42`).
- Produces: `State.Drawer string`, `func drawerPanel()`, and per-drawer entry points `codexDrawer()`, `contextDrawer()`, `graphDrawer()`, `worldDrawer()`, `characterDrawer()` (stubs until their tasks).

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/drawer_test.go`:

```go
package desktop

import "testing"

func TestDrawerTabsToggle(t *testing.T) {
	appState = &State{Loaded: true, Drawer: ""}
	toggleDrawer("codex")
	if appState.Drawer != "codex" {
		t.Fatalf("Drawer = %q, want codex", appState.Drawer)
	}
	toggleDrawer("codex")
	if appState.Drawer != "" {
		t.Fatalf("re-toggling the same drawer must close it, got %q", appState.Drawer)
	}
	toggleDrawer("graph")
	if appState.Drawer != "graph" {
		t.Fatalf("Drawer = %q, want graph", appState.Drawer)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestDrawerTabsToggle -v`
Expected: FAIL — `toggleDrawer` undefined.

- [ ] **Step 3: Implement the host**

Add `Drawer string` to `State` (`state.go`). Create `pkg/desktop/drawer.go`:

```go
package desktop

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// toggleDrawer opens the named drawer, or closes it when already active.
// One drawer is visible at a time, matching the SPA.
func toggleDrawer(name string) {
	if appState.Drawer == name {
		appState.Drawer = ""
		return
	}
	appState.Drawer = name
}

// drawerPanel renders the active drawer as a fixed-width column beside the
// chronicle. It is an ordinary layout container, not a popup.
func drawerPanel() {
	if appState.Drawer == "" {
		return
	}
	p := ui.DefaultPalette()
	Container(Attrs(FixWidth(420), Expand, Clip, BackgroundVec(p.Panel)), func() {
		Container(Attrs(Row, CrossMid, Gap(4), Pad(8)), func() {
			TabStrip(func() {
				for _, tab := range []struct{ key, label string }{
					{"codex", "Codex"}, {"context", "Context"}, {"graph", "Graph"},
					{"world", "World"}, {"character", "Character"},
				} {
					tab := tab
					NextAccessName("drawer.tab." + tab.key)
					if TabItem(tab.key, tab.label, appState.Drawer == tab.key, func() {}) {
						toggleDrawer(tab.key)
					}
					AssignAccess()
				}
			}, func() {
				Filler(1)
				NextAccessName("drawer.close")
				if Button(NoIcon, "×") {
					appState.Drawer = ""
				}
				AssignAccess()
			})
		})
		Element(Attrs(Expand, FixHeight(1), BackgroundVec(p.Border)))
		Container(Attrs(Viewport, Pad(12)), func() {
			ScrollOnInput()
			ScrollBars()
			switch appState.Drawer {
			case "codex":
				codexDrawer(p)
			case "context":
				contextDrawer(p)
			case "graph":
				graphDrawer(p)
			case "world":
				worldDrawer(p)
			case "character":
				characterDrawer(p)
			}
		})
	})
}

func codexDrawer(p ui.Palette)     { Label("Codex", TextColorVec(p.Muted)) }
func contextDrawer(p ui.Palette)   { Label("Context", TextColorVec(p.Muted)) }
func graphDrawer(p ui.Palette)     { Label("Graph", TextColorVec(p.Muted)) }
func worldDrawer(p ui.Palette)     { Label("Living World", TextColorVec(p.Muted)) }
func characterDrawer(p ui.Palette) { Label("Character Sheet", TextColorVec(p.Muted)) }
```

These stubs are replaced by Tasks 2-6 in order; each task replaces exactly one.

- [ ] **Step 4: Render the panel in the chronicle and commit**

In `chronicleView`, wrap the turn list and drawer in a `Row`: the turn list `Grow(1)`, then `drawerPanel()`. Add five access-named buttons in the shell (or the console) that call `toggleDrawer("codex")` etc.

Run: `go test ./pkg/desktop/ -v`
Expected: PASS.

```bash
git add pkg/desktop
git commit -m "$(cat <<'EOF'
feat(gui): add the drawer host and tab strip

Drawers are layout panels rather than popups, because a panel is meant
to stay open while the player reads the scene behind it.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Codex browser, note view, and save

**Files:**
- Modify: `pkg/desktop/state.go` (codex fields)
- Create: `pkg/desktop/codex.go`
- Modify: `pkg/desktop/data.go` (loaders)
- Modify: `pkg/desktop/drawer.go` (replace the `codexDrawer` stub)
- Test: `pkg/desktop/codex_test.go`

**Interfaces:**
- Consumes: `ListEntities(ctx, gameID) ([]gui.EntitySummaryDTO, error)` (`pkg/gui/service.go:595`), `GetEntity(ctx, gameID, entityID) (*gui.EntityDTO, error)` (`:644`), `SaveEntity(ctx, gameID, entityID, rawMarkdown string) error` (`:688`), `EntityDTO` (`types.go:108`), `EntitySummaryDTO` (`types.go:207`).
- Produces:
  - `State.Entities []gui.EntitySummaryDTO`, `State.Entity *gui.EntityDTO`, `State.EntityMarkdown string`, `State.EntityQuery string`, `State.EntityType string`
  - `func loadEntities(ctx, svc, gameID) []gui.EntitySummaryDTO`, `func loadEntity(ctx, svc, gameID, entityID) (*gui.EntityDTO, string)`
  - injected `saveEntity func(ctx, svc, gameID, entityID, markdown string) error`
  - `func filteredEntities() []int`

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/codex_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestFilteredEntitiesByQueryAndType(t *testing.T) {
	appState = &State{
		Loaded: true,
		Entities: []gui.EntitySummaryDTO{
			{ID: "hero", Name: "Vance", Type: "character", Tags: []string{"player"}},
			{ID: "market", Name: "Old Market", Type: "location"},
			{ID: "guild", Name: "Thieves Guild", Type: "faction"},
		},
		EntityQuery: "vance",
		EntityType:  "all",
	}
	if got := filteredEntities(); len(got) != 1 || appState.Entities[got[0]].ID != "hero" {
		t.Fatalf("query filter = %v", got)
	}

	appState.EntityQuery = ""
	appState.EntityType = "location"
	if got := filteredEntities(); len(got) != 1 || appState.Entities[got[0]].ID != "market" {
		t.Fatalf("type filter = %v", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestFilteredEntitiesByQueryAndType -v`
Expected: FAIL — `filteredEntities` undefined.

- [ ] **Step 3: Implement the codex**

Add the codex fields to `State`. In `pkg/desktop/codex.go`, implement `filteredEntities()` returning indices into `State.Entities`, matching the SPA: substring on `Name`, `ID`, and `Tags`, then the `Type` facet (`all` matches everything).

Implement `codexDrawer(p ui.Palette)`:

- A search `TextInput(&appState.EntityQuery)` and type-facet chips derived from the entity list.
- An entity list using `VirtualListView("codex-list", len(idxs), itemKey, itemHeight, itemView)` where `itemKey := func(i int) any { return appState.Entities[idxs[i]].ID }`, `itemHeight` returns 44, and `itemView` draws name/type/location and calls `openEntity(id)` on `PressAction`.
- When `appState.Entity != nil`, a view/edit mode toggle: a rendered view via `proseBlocks(appState.Entity.Markdown, ...)` and an edit via `TextArea(&appState.EntityMarkdown)` (multiline; `widgets/textinput.go:775`), with a Save button calling the injected `saveEntity` in a goroutine.
- Backlinks (`EntityDTO.Backlinks`) as access-named buttons calling `openEntity`, and a "Turns" chip list from `EntityDTO.History`.

Add `openEntity(id string)`, `loadEntities`, and `loadEntity` to `data.go`; call them when the game opens.

- [ ] **Step 4: Run tests and goldens, then commit**

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS.

```bash
git add pkg/desktop
git commit -m "feat(gui): add the codex browser and note editor"
```

---

### Task 3: Codex memories, portrait, and merge

**Files:**
- Modify: `pkg/desktop/codex.go`
- Modify: `pkg/desktop/state.go` (`Memories`, `MergeOpen`, `MergeTarget`)
- Test: `pkg/desktop/codex_test.go` (extend)

**Interfaces:**
- Consumes: `ListEntityMemories(gameID, entityID string, limit int) ([]gui.MemoryDTO, error)` (`service.go:3223`), `GetCharacterPortrait(ctx, gameID, characterID) ([]byte, string, error)` (`:1398`), `RegenerateCharacterPortrait(ctx, gameID, characterID) (gui.CharacterPortraitDTO, error)` (`:1432`), `MergeEntities(ctx, gameID, sourceID, targetID) (*gui.EntityDTO, error)` (`:722`), `MemoryDTO` (`types.go:100`).
- Produces: `State.Memories []gui.MemoryDTO`, `State.MergeOpen bool`, `State.MergeTarget string`, injected `regeneratePortrait`, `mergeEntities`, `portraitPath`.

- [ ] **Step 1: Write the failing test**

Add to `codex_test.go`:

```go
func TestMergeTargetsExcludeSelf(t *testing.T) {
	appState = &State{
		Loaded: true,
		Entities: []gui.EntitySummaryDTO{
			{ID: "hero", Name: "Vance"},
			{ID: "vance-alias", Name: "The Traveller"},
		},
		Entity: &gui.EntityDTO{ID: "hero", Name: "Vance"},
	}
	targets := mergeTargets()
	if len(targets) != 1 || targets[0].ID != "vance-alias" {
		t.Fatalf("mergeTargets = %+v", targets)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestMergeTargetsExcludeSelf -v`
Expected: FAIL — `mergeTargets` undefined.

- [ ] **Step 3: Implement memories, portrait, merge**

- Sidebar tab between Notes and Memories (`TabItem`); Memories lists `appState.Memories` with `t{turn}` and text.
- Portrait section only when `appState.Entity.Type == "character"`: load bytes once via `GetCharacterPortrait`, write to a temp file, display with `Image(path, Vec2{56, 56})`; a Regenerate button calls the injected `regeneratePortrait` in a goroutine and reloads the portrait.
- Merge: a `Modal` when `appState.MergeOpen`, listing `mergeTargets()` (all entities except the current one) and confirming via injected `mergeEntities`.

Add `mergeTargets()` returning `[]gui.EntitySummaryDTO` excluding `appState.Entity.ID`.

- [ ] **Step 4: Run tests and commit**

Run: `go test ./pkg/desktop/ -v`
Expected: PASS.

```bash
git add pkg/desktop
git commit -m "feat(gui): add codex memories, portraits, and merge"
```

---

### Task 4: Context drawer

**Files:**
- Modify: `pkg/desktop/state.go` (`TurnContext`, `WorkingSet`)
- Modify: `pkg/desktop/drawer.go` (replace the `contextDrawer` stub)
- Modify: `pkg/desktop/data.go`
- Test: `pkg/desktop/context_test.go`

**Interfaces:**
- Consumes: `GetTurnContext(gameID string) (*gui.TurnContextDTO, error)` (`service.go:3113`), `GetWorkingSet(gameID string) ([]gui.WorkingEntryDTO, error)` (`:3136`), `TurnContextDTO` (`types.go:504`), `WorkingEntryDTO` (`types.go:524`).
- Produces: `State.TurnContext *gui.TurnContextDTO`, `State.WorkingSet []gui.WorkingEntryDTO`, `func loadContext(svc, gameID)`.

- [ ] **Step 1: Write the failing test**

```go
package desktop

import "testing"

func TestContextTokenSummary(t *testing.T) {
	appState = &State{TurnContext: nil}
	if got := contextTokenSummary(); got != "" {
		t.Fatalf("nil context summary = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestContextTokenSummary -v`
Expected: FAIL.

- [ ] **Step 3: Implement the drawer**

Read-only rendering of `appState.TurnContext`: turn number/mode, strategy badge, `CachedTokens`, `EstimatedTokens/Budget`, `PromptHash`/`PrefixHash`, provider session; the working set from `appState.WorkingSet` (name/id, kind, role, last turn, weight) with clickable entries opening the codex; the section breakdown; and a collapsible assembled prompt. `contextTokenSummary()` returns `"<est>/<budget> tokens"` or `""`.

Loader `loadContext(svc, gameID)` fills both fields; refresh when the drawer opens and after each turn.

- [ ] **Step 4: Run tests and commit**

```bash
go test ./pkg/desktop/ -v
git add pkg/desktop
git commit -m "feat(gui): add the context drawer"
```

---

### Task 5: Graph drawer (offscreen render)

shirei v0.8.0 has **no canvas or custom-draw API**; the only primitives are axis-aligned rounded rectangles and text. Render the graph offscreen into an `*image.RGBA` with `golang.org/x/image/vector`, then show it with `UseImage` + `ImageView`.

**Files:**
- Create: `pkg/desktop/graph.go`
- Modify: `pkg/desktop/state.go` (`Graph`, `GraphImage *image.RGBA`)
- Modify: `pkg/desktop/drawer.go` (replace the `graphDrawer` stub)
- Modify: `pkg/desktop/data.go` (`loadGraph`)
- Test: `pkg/desktop/graph_test.go`

**Interfaces:**
- Consumes: `GetGraph(ctx, gameID) (*gui.GraphDTO, error)` (`service.go:858`), `GraphDTO`/`GraphNodeDTO`/`GraphLinkDTO` (`types.go:134`), shirei `UseImage(key string, rgba *image.RGBA) ImageId` (`images.go:321`), `ImageView(id ImageId, maxSize Vec2)` (`:343`).
- Produces:
  - `func layoutGraph(g *gui.GraphDTO, size int) map[string][2]float64`
  - `func renderGraph(g *gui.GraphDTO, size int) *image.RGBA`
  - `State.Graph *gui.GraphDTO`, `State.GraphImage *image.RGBA`

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/graph_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestLayoutGraphIsOnTheRing(t *testing.T) {
	g := &gui.GraphDTO{
		Nodes: []gui.GraphNodeDTO{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}, {ID: "c", Label: "C"}},
		Links: []gui.GraphLinkDTO{{Source: "a", Target: "b"}},
	}
	pos := layoutGraph(g, 360)
	if len(pos) != 3 {
		t.Fatalf("positions = %d, want 3", len(pos))
	}
	for id, xy := range pos {
		if xy[0] < 0 || xy[0] > 360 || xy[1] < 0 || xy[1] > 360 {
			t.Errorf("node %s off canvas: %v", id, xy)
		}
	}
}

func TestRenderGraphProducesRGBA(t *testing.T) {
	g := &gui.GraphDTO{Nodes: []gui.GraphNodeDTO{{ID: "a", Label: "A"}}, Links: nil}
	img := renderGraph(g, 200)
	if img == nil || img.Bounds().Dx() != 200 {
		t.Fatalf("expected a 200px image, got %+v", img)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run 'TestLayoutGraph|TestRenderGraph' -v`
Expected: FAIL.

- [ ] **Step 3: Implement the layout and renderer**

`layoutGraph` places node `i` at `center + radius*(cos, sin)` of `(i/n)*2π − π/2`, mirroring `GraphDrawer.tsx:35-41`. `renderGraph` allocates `image.NewRGBA(image.Rect(0, 0, size, size))`, fills the background, draws each link as a line and each node as a filled circle plus a label, using `golang.org/x/image/vector`:

```go
var r vector.Rasterizer
r.Reset(size, size)
// A circle: two arcs, then fill.
r.MoveTo(cx+rad, cy)
r.QuadTo(cx+rad, cy+rad, cx, cy+rad)
r.QuadTo(cx-rad, cy+rad, cx-rad, cy)
r.QuadTo(cx-rad, cy-rad, cx, cy-rad)
r.QuadTo(cx+rad, cy-rad, cx+rad, cy)
r.ClosePath()
r.Draw(dst, dst.Bounds(), image.NewUniform(colour), image.Point{})
```

Colour nodes by type exactly as the SPA (`character #f59e0b`, `npc #38bdf8`, `location #a855f7`, else `#78716c`).

`graphDrawer` calls `UseImage("lore-graph", appState.GraphImage)` then `ImageView(id, Vec2{width, width})`, and renders a hit list of node buttons below it that open the codex (the image itself cannot hit-test interactively).

- [ ] **Step 4: Run tests, goldens, commit**

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS.

```bash
git add pkg/desktop go.mod go.sum
git commit -m "feat(gui): render the lore graph offscreen into an image"
```

---

### Task 6: Living World and Character Sheet drawers

**Files:**
- Modify: `pkg/desktop/state.go` (`GameState`, `Recap`)
- Modify: `pkg/desktop/drawer.go` (replace both stubs)
- Modify: `pkg/desktop/data.go` (`loadWorld`)
- Test: `pkg/desktop/world_drawer_test.go`

**Interfaces:**
- Consumes: `GetGameState(ctx, gameID) (*gui.GameStateDTO, error)` (`service.go:375`), `GetRecap(ctx, gameID) (*gui.RecapDTO, error)` (`:1107`), `GameStateDTO`/`PlayerDTO`/`NarrativeArcDTO`/`FactionClockDTO` (`types.go:16-51`), `RecapDTO`/`ThreadDTO` (`types.go:217`).
- Produces: `State.GameState *gui.GameStateDTO`, `State.Recap *gui.RecapDTO`, `func loadWorld(ctx, svc, gameID)`.

- [ ] **Step 1: Write the failing test**

```go
package desktop

import (
	"math"
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestProgressFraction(t *testing.T) {
	if got := progressFraction(2, 4); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("progressFraction(2,4) = %v, want 0.5", got)
	}
	if got := progressFraction(5, 0); got != 0 {
		t.Fatalf("zero max must yield 0, got %v", got)
	}
	appState = &State{Loaded: true, GameState: &gui.GameStateDTO{Arcs: []gui.NarrativeArcDTO{{ID: "a", Name: "A", Progress: 3, MaxProgress: 6}}}}
	if got := progressFraction(3, 6); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("fraction = %v", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestProgressFraction -v`
Expected: FAIL.

- [ ] **Step 3: Implement both drawers**

`progressFraction(value, max int) float64` clamps to `[0,1]` and returns 0 for `max <= 0`; a `bar(p, fraction, colour)` helper draws a rounded track and fill with `Element` + `Container`.

- `worldDrawer`: Story So Far from `appState.Recap` (summary + through-turn) with a Refresh button calling `loadWorld`; Open Threads (name, status, last advanced, idle) from `Recap.Threads`, marking stale rows; Active Arcs (name + progress bar) from `GameState.Arcs`; Faction Clocks (name, faction, ticks bar) from `GameState.Clocks`.
- `characterDrawer`: read-only from `GameState.Player` — name, type/level, HP bar from `State["hp"]`/`State["max_hp"]`, appearance, voice name/id, and a grid of remaining `State` entries (numeric values with bars where sensible).

Both are pure reads; no injected callbacks beyond `loadWorld`.

- [ ] **Step 4: Run tests, goldens, full gate, commit**

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS.

Run: `mise run test`
Expected: PASS (re-run if the `pkg/gui` flake triggers).

```bash
git add -A
git commit -m "feat(gui): add the living world and character sheet drawers"
```

---

## Self-Review

**Spec coverage (Phase 4, drawers):**

| Spec item | Task |
| --- | --- |
| drawer shell + tab switching | Task 1 |
| `CodexDrawer` (browser, editor, backlinks, history) | Task 2 |
| codex memories, portrait, merge (incl. `TurnHistoryList`) | Task 3 |
| `ContextDrawer` (turn context + working set) | Task 4 |
| `GraphDrawer` | Task 5 |
| `LivingWorldDrawer`, `CharacterSheetDrawer` | Task 6 |
| `AddEntityModal` | deferred: it is opened from continuity findings in the chronicle; a small follow-up task once findings are surfaced |

**Placeholder scan:** No TBDs. Task 1 ships explicit stubs that later tasks replace one-for-one. The graph task states the shirei limitation and the mandated workaround.

**Type consistency:** `Drawer`, `toggleDrawer`, `drawerPanel`, `filteredEntities`, `mergeTargets`, `openEntity`, `loadEntities`/`loadEntity`/`loadContext`/`loadGraph`/`loadWorld`, `layoutGraph`, `renderGraph`, `progressFraction` are each defined once and used consistently. Service signatures and DTO field lists come from `pkg/gui/service.go` and `pkg/gui/types.go` at the cited lines.

**Known deferrals (not gaps):** clickable wikilinks in prose, entity art, and `AddEntityModal` are follow-ups; the graph is display-only with a hit list.
