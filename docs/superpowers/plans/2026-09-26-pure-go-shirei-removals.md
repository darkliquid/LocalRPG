# Pure-Go shirei GUI — Non-GUI Removals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the unused TUI, the chromedp scenario driver, and the in-app debugger, and preserve the play-session startup behaviour before the code that owns it is deleted.

**Architecture:** Extract the one-time campaign preparation that `cmd/localrpg/play.go` performs into a reusable `engine.PrepareCampaign`, cover it with a test, then delete the TUI and `play` command. Separately delete `pkg/driver`, `pkg/debugger`, the `debug` command, and `scenarios/`, rewriting `docs/debugging.md` around external OTel collectors. The SPA, its HTTP daemon, and the Wails window are untouched and keep shipping.

**Tech Stack:** Go 1.27.1, `pkg/engine`, `pkg/storage`, `pkg/telemetry`, `pkg/trace`, mise.

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md` (Phase 1)

## Global Constraints

- Desktop only; no CGO regression (Wails still requires cgo until Phase 8).
- Go style: `any` over `interface{}`, current-Go idioms. `go vet ./...` clean.
- Tests use the standard library only; no testify.
- Errors wrapped with `fmt.Errorf("...: %w", err)`.
- The SPA must keep working: do not touch `pkg/gui/server.go`, `socket.go`, `assets.go`, `middleware.go`, or the Wails path in `cmd/localrpg/gui.go`.
- Commits: Conventional Commits with a scope; end with the attribution block shown in Task 1 Step 4.
- `mise run test` must pass. Note `pkg/gui` is flaky (~1 in 6 runs fails on temp-dir cleanup); re-run before treating a `pkg/gui` failure as a regression.

---

### Task 1: Extract `engine.PrepareCampaign`

Before the TUI is deleted, preserve its one-time session setup as a tested library function so the desktop game-open path can reuse it. `cmd/localrpg/play.go:64-101` currently does: sync entities from Markdown, `EnsureIndexed`, `RepairPlayerIdentity`, `ResolveStartLocation`.

**Files:**
- Create: `pkg/engine/prepare.go`
- Test: `pkg/engine/prepare_test.go`

**Interfaces:**
- Consumes: `storage.NewSyncer`, `(*storage.Store).Sync`, `(*Timeline).EnsureIndexed`, `RepairPlayerIdentity`, `ResolveStartLocation`.
- Produces:
  - `type engine.Prepared struct { PlayerID string; StartLocation string }`
  - `func engine.PrepareCampaign(paths core.PathResolver, store *storage.Store, timeline *Timeline, manifest *core.GameManifest, entitiesDir string) (Prepared, error)`

- [ ] **Step 1: Write the failing test**

Create `pkg/engine/prepare_test.go`. Mirror the fixture setup in `pkg/engine/player_test.go:1-50` for the path resolver and manifest, and reuse `newTestStore`/`saveTestEntity` from `pkg/engine/startlocation_test.go:14-31`.

```go
package engine

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestPrepareCampaignResolvesStartLocationAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	paths := core.NewCustomPathResolver(
		filepath.Join(root, "systems"),
		filepath.Join(root, "worlds"),
		filepath.Join(root, "games"),
		filepath.Join(root, "cache"),
	)
	entitiesDir := filepath.Join(paths.GameDir("campaign"), "entities")
	if err := os.MkdirAll(entitiesDir, 0o755); err != nil {
		t.Fatalf("mkdir entities: %v", err)
	}

	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "market", Name: "Old Market", Type: "location", Hash: "hash-market"})

	manifest := &core.GameManifest{
		ID:       "campaign",
		WorldID:  "realm",
		Player:   "hero",
		Settings: map[string]any{StartLocationSetting: "market"},
	}
	history := NewHistoryLogger(filepath.Join(paths.GameDir("campaign"), "history.jsonl"))
	timeline := NewTimeline(paths, store, history, "campaign")

	got, err := PrepareCampaign(paths, store, timeline, manifest, entitiesDir)
	if err != nil {
		t.Fatalf("PrepareCampaign failed: %v", err)
	}
	if got.StartLocation != "market" {
		t.Errorf("StartLocation = %q, want market", got.StartLocation)
	}
	if got.PlayerID == "" {
		t.Errorf("PlayerID is empty")
	}

	if _, err := PrepareCampaign(paths, store, timeline, manifest, entitiesDir); err != nil {
		t.Fatalf("second PrepareCampaign failed (must be idempotent): %v", err)
	}
}
```

Add the `os` and `entity` imports to the block above. If `NewTimeline` or `NewHistoryLogger` signatures differ, copy the exact call from `cmd/localrpg/play.go:71-74` (they are used there verbatim).

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/engine/ -run TestPrepareCampaignResolvesStartLocationAndIsIdempotent -v`
Expected: FAIL — `undefined: PrepareCampaign`.

- [ ] **Step 3: Write the implementation**

Create `pkg/engine/prepare.go`:

```go
package engine

import (
	"fmt"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// Prepared holds the per-campaign facts a play session needs before its first
// turn: the authoritative player ID and the resolved opening location.
type Prepared struct {
	PlayerID      string
	StartLocation string
}

// PrepareCampaign reflects on-disk entity edits into the index, ensures the
// turn log is indexed, reconciles the campaign player identity, and resolves
// the opening location. It is the one-time setup a play session used to do
// inline; the desktop game-open path calls it per campaign.
//
// A failed player-identity repair is not fatal, matching the previous
// play-session behaviour: the manifest value is kept.
func PrepareCampaign(paths core.PathResolver, store *storage.Store, timeline *Timeline, manifest *core.GameManifest, entitiesDir string) (Prepared, error) {
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		return Prepared{}, fmt.Errorf("sync entities: %w", err)
	}
	if err := timeline.EnsureIndexed(); err != nil {
		return Prepared{}, fmt.Errorf("ensure indexed: %w", err)
	}

	playerID := manifest.Player
	if resolved, err := RepairPlayerIdentity(paths, store, manifest); err == nil && resolved != "" {
		playerID = resolved
	}

	startLocation, err := ResolveStartLocation(paths, store, manifest)
	if err != nil {
		return Prepared{}, fmt.Errorf("resolve start location: %w", err)
	}

	return Prepared{PlayerID: playerID, StartLocation: startLocation}, nil
}
```

- [ ] **Step 4: Run the test and the full engine suite**

Run: `go test ./pkg/engine/ -run TestPrepareCampaignResolvesStartLocationAndIsIdempotent -v`
Expected: PASS.

Run: `go test ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/prepare.go pkg/engine/prepare_test.go
git commit -m "$(cat <<'EOF'
refactor(engine): extract reusable campaign preparation

The play session's one-time setup (reindex, identity repair, start
location) lived inline in the command that is about to be deleted. Move
it into the engine so the desktop open path can reuse it.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Delete the TUI and the `play` command

`pkg/tui` (three source files plus tests) is only reachable from `cmd/localrpg/play.go`, which is only reachable from the `play` subcommand.

**Files:**
- Delete: `pkg/tui/` (app.go, render.go, styles.go, app_test.go, render_test.go)
- Delete: `cmd/localrpg/play.go`, `cmd/localrpg/play_test.go`
- Modify: `cmd/localrpg/main.go:39-40` (dispatch), `cmd/localrpg/main.go:67` (usage line)
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Consumes: Task 1 (`PrepareCampaign` is the only thing worth keeping from the TUI path).
- Produces: a binary with no `play` command and no terminal UI dependencies.

- [ ] **Step 1: Confirm the removal set**

Run: `rg --no-ignore -l 'pkg/tui|charmbracelet/bubbletea|charmbracelet/lipgloss|charmbracelet/glamour' --glob '*.go'`
Expected: matches only under `pkg/tui/` and in `cmd/localrpg/play.go`. If any other file appears, stop and re-scope.

- [ ] **Step 2: Delete the files and the command wiring**

```bash
git rm -r pkg/tui cmd/localrpg/play.go cmd/localrpg/play_test.go
```

In `cmd/localrpg/main.go`, remove the dispatch case:

```go
	case "play":
		handlePlayCommand(args[1:])
```

and the usage line:

```go
	fmt.Println("  play <game-id>     Launch terminal TUI play mode")
```

- [ ] **Step 3: Drop the now-unused dependencies**

```bash
go mod tidy
```

Run: `rg -n 'bubbletea|lipgloss|glamour' go.mod go.sum`
Expected: no matches.

- [ ] **Step 4: Verify build, vet, and tests**

Run: `go build ./... && go vet ./...`
Expected: clean.

Run: `go test ./... 2>&1 | rg -v '^ok|no test files' | tail -20`
Expected: no failures other than the known flaky `pkg/gui` cleanup errors; re-run those tests to confirm.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(cli): remove the unused terminal TUI and play command"
```

---

### Task 3: Delete the scenario driver, in-app debugger, and `debug` command

`pkg/driver` (chromedp) and `pkg/debugger` are only used by `cmd/localrpg/debug.go`; `pkg/debugger` is also used by `pkg/driver`. With the SPA being retired, both go, and OTel inspection moves to external collectors (already supported by `pkg/telemetry`).

**Files:**
- Delete: `pkg/driver/`, `pkg/debugger/`, `cmd/localrpg/debug.go`, `cmd/localrpg/debug_test.go`, `scenarios/`
- Modify: `cmd/localrpg/main.go` (remove `debug` dispatch + usage line)
- Modify: `docs/debugging.md`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: no `debug` subcommand; OTel export via `pkg/telemetry` remains the debugging path.

- [ ] **Step 1: Confirm the removal set**

Run: `rg --no-ignore -l 'pkg/debugger|pkg/driver|chromedp' --glob '*.go'`
Expected: matches only under `pkg/debugger/`, `pkg/driver/`, and `cmd/localrpg/debug.go`. Anything else means a hidden dependency; stop and re-scope.

- [ ] **Step 2: Delete**

```bash
git rm -r pkg/driver pkg/debugger scenarios
git rm cmd/localrpg/debug.go cmd/localrpg/debug_test.go
```

Confirm `pkg/driver/`, `pkg/debugger/`, and `scenarios/` no longer exist.

In `cmd/localrpg/main.go`, remove:

```go
	case "debug":
		handleDebugCommand(args[1:])
```

and:

```go
	fmt.Println("  debug <cmd>        Run automated scenario tests or debug server")
```

- [ ] **Step 3: Drop the chromedp dependencies**

```bash
go mod tidy
```

Run: `rg -n 'chromedp' go.mod go.sum`
Expected: no matches.

- [ ] **Step 4: Rewrite the debugging docs**

Replace the body of `docs/debugging.md` with a short guide to OTel-based debugging:

- How the app exports OTLP/gRPC, and that the endpoint comes from `OTEL_EXPORTER_OTLP_ENDPOINT` (default `localhost:4317`) or the telemetry config.
- Pointing the app at an external viewer, e.g. `otel-desktop-viewer`, and at `otel-tui`/Jaeger as alternatives.
- The file-based trace logger (`pkg/trace`, `--trace`), still available.
- A one-line note that the previous chromedp scenario runner and in-app debug dashboard were removed.

Do not leave references to `localrpg debug test-run`, `localrpg debug server`, or YAML scenarios.

- [ ] **Step 5: Verify build, vet, docs, and tests**

Run: `go build ./... && go vet ./...`
Expected: clean.

Run: `rg -n 'debug test-run|debug server|chromedp|/api/trace' docs/debugging.md README.md`
Expected: no matches.

Run: `go test ./... 2>&1 | rg -v '^ok|no test files' | tail -20`
Expected: only the known flaky `pkg/gui` cleanup failures, which pass on re-run.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor(debug): replace the in-app debugger with external OTel tools"
```

---

### Task 4: Final verification of the removals

**Files:** none (verification only).

**Interfaces:**
- Consumes: Tasks 1-3.
- Produces: a green build with the removals in place, and a clean `git status`.

- [ ] **Step 1: Verify the CLI surface**

Run: `go run ./cmd/localrpg --help`
Expected: `play` and `debug` are absent; `roll`, `prompt`, `tts`, `image`, `gui`, `export`, `version` remain, and `gui` still launches the Wails window.

- [ ] **Step 2: Verify the SPA still works**

Run: `mise run dev:gui` in one shell and `mise run dev:frontend` in another; load `http://localhost:3000`.
Expected: the SPA loads and a campaign opens. This confirms the daemon and Wails paths were untouched. Stop both processes afterwards.

- [ ] **Step 3: Full gate**

Run: `mise run test`
Expected: PASS (re-run if the flaky `pkg/gui` cleanup triggers).

Run: `git status --short`
Expected: empty.

- [ ] **Step 4: No commit** (verification task; the work was committed in Tasks 1-3).

---

## Self-Review

**Spec coverage (Phase 1 of the spec):**

| Spec item | Task |
| --- | --- |
| port `play.go` reindex/re-resolve into a reusable path | Task 1 (extraction); wiring into the desktop open path happens in the Phase 3 plan |
| delete TUI (`pkg/tui`, `play.go`, `play` cmd, deps) | Task 2 |
| delete `pkg/driver`, `pkg/debugger`, `debug server`/`test-run` | Task 3 |
| SPA + Wails keep shipping | explicitly out of scope; verified untouched in Task 4 |
| `docs/debugging.md` external-collector rewrite | Task 3 Step 4 (this supersedes the Phase 6 doc item) |

**Placeholder scan:** No TBDs. Task 1 references `pkg/engine/player_test.go` and `play.go:71-74` as the source of exact fixture/signature spellings; that is a factual fallback, not a placeholder.

**Type consistency:** `engine.Prepared` and `engine.PrepareCampaign` are defined once in Task 1 and referenced with the same names in the self-review. `newTestStore`/`saveTestEntity` are existing helpers reused verbatim.

**Known deferrals (not gaps):** wiring `PrepareCampaign` into the desktop open path, the portability changes, the screen ports, and the teardown are later plans.
