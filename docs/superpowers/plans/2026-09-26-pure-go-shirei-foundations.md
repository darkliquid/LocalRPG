# Pure-Go shirei GUI — Foundations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the dependency, style, tooling, and packaging foundations for a pure-Go `go-shirei` GUI without changing any user-visible behaviour.

**Architecture:** Add shirei as a pinned, vendored dependency and put it behind a thin `pkg/ui` adapter (boot, theme, snapshot helper). Build an empty `pkg/desktop` application shell that renders a single root view, exercised only by a headless golden-PNG test. Add cross-compile and snapshot tasks to `mise.toml`. Nothing in `cmd/localrpg` is wired to the new shell yet; Wails keeps running the app.

**Tech Stack:** Go 1.27.1, `go.hasen.dev/shirei` v0.8.0, `mise` task runner, shirei's software-rendered headless snapshots.

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md`

## Global Constraints

- Desktop only: Linux, Windows, macOS. No mobile, no web/WASM in v1.
- No CGO on desktop; every new build path must work with `CGO_ENABLED=0`.
- Pin shirei to exactly `v0.8.0` and commit `vendor/`.
- Go style: prefer `any` over `interface{}`; use current-Go idioms. `interface{}` must not appear in new code.
- Tests use the standard library only (`testing`, `t.TempDir()`); no testify.
- Errors wrapped with `fmt.Errorf("...: %w", err)`.
- `go vet ./...` and `go build ./...` must stay clean; `mise run test` must pass.
- Fidelity: match the SPA's layouts/styling closely; 1:1 pixel parity is not required.
- Commits: Conventional Commits with a scope (e.g. `build(gui): …`). Every commit ends with the attribution block shown in Task 1, Step 3.
- Do not wire `pkg/desktop` into `cmd/localrpg` in this plan; that is the next plan.

---

### Task 1: Make `cmd/localrpg` trackable

`.gitignore:23` is the bare pattern `localrpg`, which matches the `cmd/localrpg/` directory as well as the root binary, so any new entry-point file there is silently untracked. Anchor the pattern to the repository root.

**Files:**
- Modify: `.gitignore:23`

**Interfaces:**
- Consumes: nothing.
- Produces: a working tree where `cmd/localrpg` is no longer ignored, needed by every later task that adds root-command code.

- [ ] **Step 1: Confirm the current over-match**

Run: `git check-ignore -v cmd/localrpg/main.go`
Expected: prints a rule for `.gitignore` line 23 (`localrpg`) and exits 0, i.e. the path is ignored.

- [ ] **Step 2: Anchor the pattern**

Replace the line:

```gitignore
# Binaries
localrpg
```

with:

```gitignore
# Binaries
/localrpg
```

- [ ] **Step 3: Verify the fix and commit**

Run: `git check-ignore -v cmd/localrpg/main.go; echo "exit=$?"`
Expected: no rule printed, `exit=1`.

Run: `git check-ignore -v localrpg; echo "exit=$?"`
Expected: prints `/localrpg` and `exit=0` (root binary still ignored).

```bash
git add .gitignore
git commit -m "$(cat <<'EOF'
chore(git): only ignore the root localrpg binary

The bare `localrpg` pattern also matched the cmd/localrpg directory, so
new files added there were silently untracked. Anchor it to the repo
root so the entry point stays under version control.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Sweep `interface{}` to `any`

Adopt `any` project-wide in one mechanical commit so it is not entangled with GUI work. `gofmt -r 'interface{} -> any'` rewrites type positions only; it leaves comments and string literals untouched.

**Files:**
- Modify: every `.go` file containing `interface{}` (457 occurrences across 116 files).

**Interfaces:**
- Consumes: Task 1 (so files under `cmd/localrpg` are visible to `rg`).
- Produces: the house style required by the spec's Global Constraints; no API shape changes.

- [ ] **Step 1: Record the baseline**

Run: `rg -l 'interface\{\}' -g '*.go' | wc -l`
Expected: `116` (or close; record the actual number).

- [ ] **Step 2: Apply the rewrite**

```bash
rg -l 'interface\{\}' -g '*.go' | xargs gofmt -w -r 'interface{} -> any'
```

- [ ] **Step 3: Verify no types remain and formatting is clean**

Run: `rg -c 'interface\{\}' -g '*.go' | wc -l`
Expected: `0`.

Run: `gofmt -l .`
Expected: no output.

- [ ] **Step 4: Build, vet, and test**

Run: `go build ./... && go vet ./...`
Expected: clean.

Run: `go test -count=1 ./...`
Expected: all packages pass (same failures as before the sweep, if any).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: use any instead of interface{} throughout"
```

---

### Task 3: Pin, vendor, and wrap shirei

Add the pinned dependency and a thin `pkg/ui` adapter for boot and theme. Keep it data-and-helpers only; do not build a full facade over shirei's widget API.

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `vendor/` (via `go mod vendor`)
- Create: `pkg/ui/boot.go`
- Create: `pkg/ui/theme.go`
- Test: `pkg/ui/theme_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `ui.Palette` struct with fields `Bg, Panel, Text, Muted, Border, Accent, Danger shirei.Vec4`
  - `func ui.DefaultPalette() Palette`
  - `func ui.Run(title string, w, h int, view shirei.FrameFn)`

- [ ] **Step 1: Write the failing test**

Create `pkg/ui/theme_test.go`:

```go
package ui

import "testing"

func TestDefaultPaletteChannelsAreInRange(t *testing.T) {
	p := DefaultPalette()
	colors := map[string][4]float32{
		"Bg": p.Bg, "Panel": p.Panel, "Text": p.Text, "Muted": p.Muted,
		"Border": p.Border, "Accent": p.Accent, "Danger": p.Danger,
	}
	for name, c := range colors {
		if c[0] < 0 || c[0] > 360 {
			t.Errorf("%s hue out of range: %v", name, c[0])
		}
		for i := 1; i < 4; i++ {
			if c[i] < 0 || c[i] > 100 && i < 3 {
				t.Errorf("%s channel %d out of range: %v", name, i, c[i])
			}
		}
		if c[3] < 0 || c[3] > 1 {
			t.Errorf("%s alpha out of range: %v", name, c[3])
		}
	}
}
```

Note: `shirei.Vec4` is `[4]f32`, so `p.Bg` is assignable to `[4]float32`.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/ui/ -run TestDefaultPaletteChannelsAreInRange -v`
Expected: FAIL — the `ui` package does not exist yet (or `DefaultPalette` undefined).

- [ ] **Step 3: Add the pinned dependency**

```bash
go get go.hasen.dev/shirei@v0.8.0
```

Expected: `go.mod` gains `require go.hasen.dev/shirei v0.8.0`.

- [ ] **Step 4: Write the adapter**

Create `pkg/ui/theme.go`:

```go
package ui

import "go.hasen.dev/shirei"

// Palette is the application colour scheme in shirei's HSLA form:
// hue 0-360, saturation 0-100, lightness 0-100, alpha 0-1.
//
// Values are first-pass matches for the retired SPA's stone/amber styling.
// Tune them against the SPA screenshot while porting screens; this is not a
// pixel-parity exercise.
type Palette struct {
	Bg     shirei.Vec4
	Panel  shirei.Vec4
	Text   shirei.Vec4
	Muted  shirei.Vec4
	Border shirei.Vec4
	Accent shirei.Vec4
	Danger shirei.Vec4
}

// DefaultPalette returns the light-on-dark palette used by the desktop app.
func DefaultPalette() Palette {
	return Palette{
		Bg:     shirei.Vec4{20, 13, 4, 1},   // stone-950
		Panel:  shirei.Vec4{24, 8, 10, 1},   // raised surface
		Text:   shirei.Vec4{20, 6, 92, 1},   // stone-200
		Muted:  shirei.Vec4{25, 5, 45, 1},   // stone-500
		Border: shirei.Vec4{24, 6, 20, 1},   // hairline
		Accent: shirei.Vec4{30, 65, 55, 1},  // amber
		Danger: shirei.Vec4{0, 65, 55, 1},
	}
}
```

Create `pkg/ui/boot.go`:

```go
package ui

import (
	"go.hasen.dev/shirei"
	app "go.hasen.dev/shirei/app"
)

// Run opens the native window and enters the shirei frame loop. It does not
// return; quit paths exit the process. SetupWindow must be called first, which
// Run does, so callers only supply the title and content size.
func Run(title string, w, h int, view shirei.FrameFn) {
	app.SetupWindow(title, w, h)
	app.Run(view)
}
```

- [ ] **Step 5: Run the test and tidy**

Run: `go test ./pkg/ui/ -run TestDefaultPaletteChannelsAreInRange -v`
Expected: PASS.

Run: `go mod tidy`
Expected: no changes to `go.mod` beyond the shirei requirement and its indirect deps.

- [ ] **Step 6: Vendor and verify a CGO-free build**

```bash
go mod vendor
CGO_ENABLED=0 go build ./...
```

Expected: `vendor/go.hasen.dev/shirei/` exists; build succeeds.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum vendor pkg/ui
git commit -m "build(gui): pin and vendor go-shirei behind a thin ui adapter"
```

---

### Task 4: Add the `pkg/desktop` shell with a golden snapshot

Create the application shell and its first headless test. It is not reachable from the CLI yet.

**Files:**
- Create: `pkg/ui/snapshot.go`
- Create: `pkg/desktop/run.go`
- Create: `pkg/desktop/root.go`
- Test: `pkg/desktop/root_test.go`
- Create: `pkg/desktop/testdata/snapshots/root.png` (written by the test on first run)

**Interfaces:**
- Consumes from Task 3: `ui.Palette`, `ui.DefaultPalette()`, `ui.Run(...)`.
- Produces:
  - `func ui.Snapshot(t *testing.T, name string, w, h int, view func())`
  - `type desktop.Config struct { Dir string; PNGPath string; Width int; Height int }`
  - `func desktop.Run(cfg Config) error`
  - `func desktop.RootView()`

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/root_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestRootViewSnapshot(t *testing.T) {
	ui.Snapshot(t, "root", 800, 600, RootView)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestRootViewSnapshot -v`
Expected: FAIL — `desktop` package does not exist / `ui.Snapshot` undefined.

- [ ] **Step 3: Add the snapshot helper**

Create `pkg/ui/snapshot.go`:

```go
package ui

import (
	"testing"

	"go.hasen.dev/shirei"
)

// Snapshot renders view headlessly at w×h and compares it against the golden
// at testdata/snapshots/<name>.png. On a missing golden it writes one and
// passes; set UPDATE_SNAPSHOTS=1 to regenerate. It skips when the host has no
// usable system fonts, because shirei's text shaping cannot run there.
func Snapshot(t *testing.T, name string, w, h int, view func()) {
	t.Helper()
	res := shirei.Snapshot(t.Name(), name, w, h, shirei.FrameFn(view))
	switch res.Status {
	case shirei.SnapMatch, shirei.SnapCreated, shirei.SnapUpdated:
		if res.Status != shirei.SnapMatch {
			t.Logf("snapshot %s: %s (golden=%s)", name, res.Status, res.Golden)
		}
	case shirei.SnapSkip:
		t.Skipf("snapshot %s skipped: %s", name, res.Reason)
	default:
		t.Fatalf("snapshot %s: %s (golden=%s actual=%s) err=%v",
			name, res.Status, res.Golden, res.Actual, res.Err)
	}
}
```

- [ ] **Step 4: Add the shell and root view**

Create `pkg/desktop/run.go`:

```go
package desktop

import (
	"go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// Config configures the desktop application shell.
type Config struct {
	Dir     string // project root directory; unused until screens land
	PNGPath string // when set, render one frame to this path and return
	Width   int
	Height  int
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
	if cfg.PNGPath != "" {
		return shirei.RenderToPNG(cfg.PNGPath, cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	}
	ui.Run("LocalRPG", cfg.Width, cfg.Height, shirei.FrameFn(RootView))
	return nil
}
```

Create `pkg/desktop/root.go`:

```go
package desktop

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// RootView renders the application frame. It currently shows only a title;
// screens are added by later plans.
func RootView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg), Pad(24)), func() {
		Label("LocalRPG", FontSize(22), FontWeight(WeightBold), TextColorVec(p.Text))
		Label("GUI foundations", FontSize(13), TextColorVec(p.Muted))
	})
}
```

If `Pad`, `FontWeight`, or `WeightBold` are not found by the compiler, check the exact names in `vendor/go.hasen.dev/shirei/attrs.go` and `vendor/go.hasen.dev/shirei/widgets/` and use the vendored spellings. Do not leave a guessed identifier in place.

- [ ] **Step 5: Run the test to create the golden**

Run: `go test ./pkg/desktop/ -run TestRootViewSnapshot -v`
Expected: PASS, logging `snapshot root: created`. `pkg/desktop/testdata/snapshots/root.png` now exists. If the test instead reports `skip`, the machine lacks system fonts; install a font package on the CI/dev host before relying on snapshots.

- [ ] **Step 6: Confirm the golden is stable**

Run: `go test ./pkg/desktop/ -run TestRootViewSnapshot -v`
Expected: PASS with status `match` (no `created` log line).

- [ ] **Step 7: Verify build, vet, and the `--png` render path**

Run: `go build ./... && go vet ./...`
Expected: clean.

Run: `go run ./cmd/localrpg --help` is not expected to change in this plan; instead verify the shell's PNG path compiles by building the test binary above. (The `--png` flag is wired to `desktop.Run` in the next plan.)

- [ ] **Step 8: Commit**

```bash
git add pkg/ui/snapshot.go pkg/desktop
git commit -m "feat(gui): add the desktop shell with a golden snapshot test"
```

---

### Task 5: Add cross-compile and snapshot mise tasks

Add the desktop build targets and a snapshot-regeneration helper, and record them in `AGENTS.md`.

**Files:**
- Modify: `mise.toml`
- Modify: `AGENTS.md` (Commands section)

**Interfaces:**
- Consumes: `pkg/desktop` from Task 4.
- Produces: mise tasks `desktop:build`, `desktop:png`, `desktop:snapshots`.

- [ ] **Step 1: Add the tasks**

Append to `mise.toml`:

```toml
[tasks."desktop:build"]
description = "Cross-compile the desktop GUI for Linux, Windows, and macOS (CGO-free)"
run = """
mkdir -p bin
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o bin/localrpg-linux-amd64   ./cmd/localrpg
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -o bin/localrpg-linux-arm64   ./cmd/localrpg
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/localrpg-windows-amd64.exe ./cmd/localrpg
CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -o bin/localrpg-darwin-amd64  ./cmd/localrpg
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o bin/localrpg-darwin-arm64  ./cmd/localrpg
"""
sources = ["cmd/**/*", "pkg/**/*", "go.mod"]

[tasks."desktop:png"]
description = "Render one frame of the desktop root view to PNG"
run = "go run ./cmd/localrpg --png /tmp/localrpg-root.png"

[tasks."desktop:snapshots"]
description = "Regenerate desktop golden snapshots"
run = "UPDATE_SNAPSHOTS=1 go test ./pkg/desktop/... -run Snapshot -count=1"
```

Note: `desktop:png` documents the intended entry flag; it will only work once the next plan wires `--png` into the root command. Until then it is documentation of the target surface.

- [ ] **Step 2: Verify the cross-compile task**

Run: `mise run desktop:build`
Expected: five binaries written under `bin/` (which is gitignored), no CGO errors.

Run: `ls -1 bin/localrpg-*`
Expected: the five binaries listed.

- [ ] **Step 3: Verify the snapshot task**

Run: `mise run desktop:snapshots`
Expected: `pkg/desktop` snapshot tests pass; regenerated golden committed in Task 4 remains valid.

- [ ] **Step 4: Record the tasks in AGENTS.md**

In `AGENTS.md`, in the `## Commands` fenced block, after the existing `mise run clean` line, add:

```
mise run desktop:build  # cross-compile localrpg for linux/windows/darwin (CGO-free)
mise run desktop:snapshots # regenerate shirei golden snapshots in pkg/desktop
```

- [ ] **Step 5: Verify the full test gate**

Run: `mise run test`
Expected: `go test -v -count=1 ./...` passes; the frontend type check still passes (the SPA is untouched in this plan).

- [ ] **Step 6: Commit**

```bash
git add mise.toml AGENTS.md
git commit -m "build(gui): add desktop cross-compile and snapshot tasks"
```

---

## Self-Review

**Spec coverage (Phase 0 of the spec):**

| Spec deliverable | Task |
| --- | --- |
| vendor+pin shirei | Task 3 |
| `.gitignore` fix | Task 1 |
| `pkg/ui` adapter | Task 3 (theme + boot), Task 4 (snapshot) |
| `pkg/desktop` shell with a `--png` golden test | Task 4 (shell + golden); `--png` CLI wiring deferred to the next plan by design |
| cross-compile mise tasks | Task 5 |
| Go-style sweep | Task 2 |
| "no user-visible change" | No `cmd/localrpg` behaviour is touched; Wails keeps running the app |

**Placeholder scan:** No TBDs. Task 4 Step 4 includes an explicit instruction to fix identifier spellings against the vendored source rather than guess, which is a factual fallback, not a placeholder.

**Type consistency:** `ui.Palette`/`ui.DefaultPalette`/`ui.Run`/`ui.Snapshot` are defined once and used with the same names in Tasks 3–4. `desktop.Config`/`desktop.Run`/`desktop.RootView` are defined in Task 4 and referenced consistently in Task 5. `shirei.Vec4`, `shirei.FrameFn`, `shirei.Snapshot`, and the `Snap*` status constants are taken verbatim from the vendored `v0.8.0` source.

**Known deferrals (not gaps):** wiring `desktop.Run` into the root command, deleting Wails, the core split, and screen ports are the next four plans.
