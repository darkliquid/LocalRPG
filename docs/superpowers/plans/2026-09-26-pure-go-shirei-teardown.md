# Pure-Go shirei GUI — Teardown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make shirei the only UI: boot it from the root command, delete the Wails window, the HTTP/socket daemon, the React SPA, and the Node toolchain, and turn on the CGO-free cross-compile gate.

**Architecture:** `cmd/localrpg` boots the shirei shell from the root command (no `gui` subcommand, no `LOCALRPG_UI` switch). The HTTP serving layer goes, but the service DTOs in `pkg/gui/types.go` stay because the desktop views consume them. After Wails, `oto`, and sherpa are all gone from the default build, `CGO_ENABLED=0 go build ./...` and the cross-compile matrix become the standing gate.

**Tech Stack:** Go 1.27.1, `pkg/desktop`, `pkg/gui` (transport-free core), mise.

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md` (Phase 8)

## Global Constraints

- The service DTOs in `pkg/gui/types.go` are **kept**: they are the in-process model the desktop views use. Only the HTTP serving layer is deleted. (This is a deliberate correction to the spec's file map.)
- `pkg/gui` keeps the name for now; the optional rename to `pkg/session` stays an open question.
- Go style: `any`; `go vet ./...` clean. Tests standard-library only.
- The CGO-free gate must pass at the end: `CGO_ENABLED=0 go build ./...` and `mise run desktop:build`.
- Commits: Conventional Commits with a scope; end with the attribution block shown in Task 1 Step 5.
- `mise run test` must pass (the `pkg/gui` handler tests are deleted in Task 2, so the flake goes with them).

---

### Task 1: Boot shirei from the root command

**Files:**
- Modify: `cmd/localrpg/main.go`
- Modify: `cmd/localrpg/gui.go` (rename to `desktop.go`; shirei boot only)
- Modify: `cmd/localrpg/main_test.go`, `cmd/localrpg/gui_test.go`
- Modify: `pkg/desktop/run.go` (drop `State`-only test affordances if they complicate boot)

**Interfaces:**
- Consumes: `desktop.Run(ctx, cfg)`.
- Produces: `localrpg` with no args boots the GUI; `localrpg --dir X --png out.png` renders one frame and exits.

- [ ] **Step 1: Write the failing test**

In `cmd/localrpg/main_test.go`:

```go
func TestNoArgsBootsGUI(t *testing.T) {
	// A PNG boot must render and return without opening a window.
	out := filepath.Join(t.TempDir(), "boot.png")
	code := runMain([]string{"--png", out})
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected a rendered frame: %v", err)
	}
}
```

This requires `main` to be testable as `runMain(args []string) int`. If `main` is currently a monolithic `func main()`, extract `runMain` in Step 3 and have `main` call `os.Exit(runMain(os.Args[1:]))`.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/localrpg/ -run TestNoArgsBootsGUI -v`
Expected: FAIL — `runMain` undefined, and `--png` is not a root flag.

- [ ] **Step 3: Rework the entry point**

- Extract `runMain(args []string) int` from `main`, keeping the provider validation, `--version`, and subcommand dispatch.
- Register root flags `--dir` (default `.`) and `--png` (default ``) on the root flag set, and add `--version`.
- With no subcommand, call `desktop.Run(context.Background(), desktop.Config{Dir: dir, PNGPath: png, Service: gui.NewService(dir)...})` and return its status. The shirei shell needs the service: construct `svc := gui.NewService(dir)`, set up telemetry and the trace logger exactly as `handleGUICommand` does today, then call `desktop.Run`.
- Delete the `gui` subcommand dispatch and its usage line, and delete the `LOCALRPG_UI` branch added in Plan 4.
- Rename `handleGUICommand`/`parseGUIConfig` into a shared `bootDesktop(dir, png string) int` in `cmd/localrpg/desktop.go`, removing `--port`, `--socket`, and `--headless`.
- Update `printUsage` to describe the root GUI boot and the surviving subcommands.

`desktop.Config` currently takes `State` or `Service`; the boot path passes `Service`. Ensure `Run` leaves `PNGPath` working with a live service (it must render without opening a window), so the test above passes.

- [ ] **Step 4: Run the test and the CLI**

Run: `go test ./cmd/localrpg/ -run TestNoArgsBootsGUI -v`
Expected: PASS.

Run: `go run ./cmd/localrpg --help`
Expected: usage lists the root GUI boot and `roll`, `prompt`, `tts`, `image`, `export`, `version`; no `gui`, `play`, or `debug`.

- [ ] **Step 5: Commit**

```bash
git add cmd/localrpg pkg/desktop
git commit -m "$(cat <<'EOF'
feat(cli): boot the GUI from the root command

The separate gui subcommand and the transitional env switch are gone;
running the binary with no arguments opens the shirei window, and
--png renders a single frame for tests.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Delete Wails and the HTTP daemon

**Files:**
- Delete: `pkg/gui/server.go`, `pkg/gui/socket.go`, `pkg/gui/assets.go`, `pkg/gui/middleware.go`, `pkg/gui/dist/`
- Delete: every `pkg/gui/*_test.go` that constructs `NewServer`, `httptest`, or a socket
- Delete: `cmd/localrpg/desktop.go`'s Wails block (may already be gone after Task 1)
- Modify: `go.mod`, `go.sum`
- Modify: `pkg/gui/service.go` (remove HTTP-only helpers if they are unreferenced)

**Interfaces:**
- Consumes: nothing.
- Produces: no `net/http` in the GUI path; no Wails.

- [ ] **Step 1: Confirm the removal set**

Run: `rg -n 'wailsapp|gui\.NewServer|ProtectCrossOrigin|AssetHandler|ListenUnix|DefaultSocketPath|http\.NewCrossOriginProtection' --glob '*.go'`
Expected: matches only in the files listed above (plus their tests). Anything else means a hidden dependency; stop and re-scope.

Run: `rg -ln 'httptest|net/http' pkg/gui --glob '*_test.go'`
Expected: the set of handler tests to delete.

- [ ] **Step 2: Delete**

```bash
git rm pkg/gui/server.go pkg/gui/socket.go pkg/gui/assets.go pkg/gui/middleware.go
git rm -r pkg/gui/dist
git rm $(rg -l 'httptest|NewServer|ListenUnix|ProtectCrossOrigin' pkg/gui --glob '*_test.go')
```

Then delete any now-unreferenced helpers in `pkg/gui/service.go` (e.g. response writers) and the `otelhttp`/`http` imports.

- [ ] **Step 3: Drop the dependencies and verify**

```bash
go mod tidy
```

Run: `rg -n 'wailsapp|otelhttp|chromedp' go.mod`
Expected: no matches.

Run: `go build ./... && go vet ./...`
Expected: clean.

Run: `go test ./... 2>&1 | rg -v '^ok|no test files' | tail -20`
Expected: no failures.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(gui): remove the Wails window and the HTTP daemon"
```

---

### Task 3: Delete the SPA and the Node toolchain

**Files:**
- Delete: `frontend/`
- Modify: `mise.toml` (remove frontend tasks)
- Modify: `.gitignore` (remove frontend/dist entries)
- Modify: `README.md`, `AGENTS.md` (build instructions, architecture text)

**Interfaces:**
- Consumes: nothing.
- Produces: a Node-free build.

- [ ] **Step 1: Delete the SPA**

```bash
git rm -r frontend
```

- [ ] **Step 2: Remove the Node tasks**

In `mise.toml`, remove `setup`'s `cd frontend && npm install`, `build:frontend`, `build:backend`'s dependency on it, `test:frontend`, `test`'s dependency on it, and `dev:frontend`. `build:backend` becomes:

```toml
[tasks."build:backend"]
description = "Build the localrpg CLI binary"
run = "go build -o bin/localrpg ./cmd/localrpg"
sources = ["cmd/**/*", "pkg/**/*", "go.mod"]
outputs = ["bin/localrpg"]
```

Remove the staging copy of `pkg/gui/dist` from any task and the `touch .gitkeep` step.

- [ ] **Step 3: Clean ignore rules and docs**

Remove `frontend/node_modules/`, `frontend/dist/`, `pkg/gui/dist/*`, and `!pkg/gui/dist/.gitkeep` from `.gitignore`.

Update `README.md`: replace the Wails/webview, Unix-socket daemon, and TUI descriptions with the shirei desktop app, the root-command boot, and `--png`. Remove the "frontend embedded in the Go binary" gotcha from `AGENTS.md` and replace it with a note that the GUI is in-process (`pkg/desktop`) over a transport-free core (`pkg/gui`).

- [ ] **Step 4: Verify the Node-free build**

Run: `rg -n 'npm|vite|frontend|tsc' mise.toml README.md AGENTS.md`
Expected: no build-instruction matches (historical mentions in specs/plans are fine).

Run: `mise run build`
Expected: produces `bin/localrpg` with no Node involvement.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: remove the React SPA and Node toolchain"
```

---

### Task 4: Turn on the CGO-free gate

**Files:**
- Modify: `mise.toml` (add `desktop:build`, `desktop:png`)
- Modify: `AGENTS.md` (Commands)
- Test: none new.

**Interfaces:**
- Consumes: Tasks 1-3 (all three cgo sources gone: Wails in Task 2, `oto` in the Portability plan, sherpa gated in the Portability plan).
- Produces: `mise run desktop:build` green.

- [ ] **Step 1: Confirm the default build is CGO-free**

Run: `CGO_ENABLED=0 go build ./...`
Expected: clean. If it fails, the failure names the remaining cgo dependency; stop and report — the Portability plan was supposed to have removed the other two.

- [ ] **Step 2: Add the cross-compile and PNG tasks**

Append to `mise.toml`:

```toml
[tasks."desktop:build"]
description = "Cross-compile the desktop GUI for Linux, Windows, and macOS (CGO-free)"
run = """
mkdir -p bin
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o bin/localrpg-linux-amd64     ./cmd/localrpg
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -o bin/localrpg-linux-arm64     ./cmd/localrpg
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/localrpg-windows-amd64.exe ./cmd/localrpg
CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -o bin/localrpg-darwin-amd64    ./cmd/localrpg
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o bin/localrpg-darwin-arm64    ./cmd/localrpg
"""
sources = ["cmd/**/*", "pkg/**/*", "go.mod"]

[tasks."desktop:png"]
description = "Render one frame of the GUI to a PNG and exit"
run = "go run ./cmd/localrpg --png /tmp/localrpg-frame.png"
```

- [ ] **Step 3: Verify and record**

Run: `mise run desktop:build`
Expected: five binaries under `bin/`.

Run: `mise run desktop:png`
Expected: `/tmp/localrpg-frame.png` written.

In `AGENTS.md`'s `## Commands` block, add:

```
mise run desktop:build  # cross-compile localrpg for linux/windows/darwin (CGO-free)
mise run desktop:png    # render one GUI frame to a PNG
```

Also add a Global-Constraints-style note near the Go conventions: the desktop binary must stay `CGO_ENABLED=0`-buildable, and any new dependency that needs cgo must be build-tagged opt-in.

- [ ] **Step 4: Commit**

```bash
git add mise.toml AGENTS.md
git commit -m "build(gui): enable the CGO-free cross-compile gate"
```

---

### Task 5: Final verification

**Files:** none (verification only).

**Interfaces:**
- Consumes: Tasks 1-4.
- Produces: documented evidence the teardown is complete.

- [ ] **Step 1: Confirm the old stack is gone**

Run: `rg -n 'wails|frontend|assets\.go|debugger|driver|scenarios' --glob '*.go' --glob 'mise.toml' --glob '*.md' | rg -v 'docs/superpowers|docs/proposals'`
Expected: no references outside the dated specs/plans and the research memo.

Run: `ls frontend 2>&1; ls pkg/tui 2>&1; ls pkg/driver 2>&1; ls pkg/debugger 2>&1`
Expected: all report "No such file or directory".

- [ ] **Step 2: Confirm the gates**

Run: `CGO_ENABLED=0 go build ./... && go vet ./...`
Expected: clean.

Run: `mise run desktop:build && mise run test`
Expected: PASS.

- [ ] **Step 3: Manual smoke test**

Run: `bin/localrpg` (or `mise run dev:gui` if the task is kept/replaced).
Expected: the shirei window opens on the launcher; a campaign opens; a turn plays with audio; the chronicle, drawers, settings, studios, and theatre all work; `bin/localrpg export video --dir <dir> <game-id> --out /tmp/out.mp4` renders a theatre video.

- [ ] **Step 4: No commit** (verification task).

---

## Self-Review

**Spec coverage (Phase 8):**

| Spec item | Task |
| --- | --- |
| shirei is the only UI; root boots it | Task 1 |
| delete `pkg/gui/server.go`/`socket.go`/`assets.go`/`middleware.go` | Task 2 |
| delete Wails | Task 2 |
| delete `frontend/`, Node from mise | Task 3 |
| CGO-free `go build ./...` + `desktop:build` green | Task 4 |
| `LOCALRPG_UI` and `gui` subcommand gone | Task 1 |
| docs/AGENTS updated | Tasks 3, 4 |

**Deliberate correction:** `pkg/gui/types.go` is **kept** (the spec's rough file map listed it for deletion); the desktop views depend on those DTOs as the in-process model. Recorded in Global Constraints.

**Placeholder scan:** No TBDs.

**Type consistency:** `runMain`, `bootDesktop`, `desktop.Config`, the `desktop:build`/`desktop:png` tasks, and the deleted file list are consistent with Plans 1-10. `--png` matches the flag introduced in Plan 1's `desktop.Run`.

**Known follow-ups (not gaps):** the optional `pkg/gui` → `pkg/session` rename, `shirei_bundle` packaging, and the shirei-audio mono/stereo question remain open per the spec's open-questions section.
