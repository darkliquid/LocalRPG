# Design Specification: Zero-TCP Port GUI Architecture

**Date:** 2026-09-20  
**Status:** Approved  
**Topic:** Zero-TCP Port Desktop GUI, Native Wails v3 In-Process IPC, Unix Domain Socket Fallback, and Embedded React 19 SPA Asset Pipeline

---

## 1. Problem Statement & Motivation
In early scaffolding for Milestone 6, the `localrpg gui` command opened an unauthenticated TCP listener on `127.0.0.1:8080` serving a placeholder HTML document. 

This had two major shortcomings:
1. **Network Exposure:** Desktop games and local-first apps should not unnecessarily expose open TCP ports on the network stack. A desktop app should run portless using native platform IPC or restricted local filesystem sockets.
2. **Missing Real Application:** The bundled React 19 + Tailwind CSS glassmorphic frontend (featuring Twintail Launcher aesthetics, character sheets, lore graphs, codex editing, and the Story Theater) was not embedded or served by the Go binary; visitors only saw a single-line placeholder.

This specification redesigns `localrpg gui` to be **zero-TCP by default**, leveraging native Wails v3 in-process WebKit IPC and direct asset handling, while providing a Unix domain socket fallback for headless/server use and fully embedding the production React application.

---

## 2. Architecture & Operating Modes

`localrpg gui` will support three operating modes:

```
                          ┌────────────────────────┐
                          │    localrpg gui        │
                          └───────────┬────────────┘
                                      │
              ┌───────────────────────┼───────────────────────┐
              ▼                       ▼                       ▼
      [Default Mode]          [Socket Mode]              [Web Mode]
   No flags + GUI display    --socket [path]           --port <number>
              │                       │                       │
              ▼                       ▼                       ▼
     Native Wails v3          Unix Domain Socket          Local TCP Socket
   In-Process WebKit IPC    net.Listen("unix", ...)   net.Listen("tcp", ...)
     0 Ports / No TCP         0 Ports / File Perms        127.0.0.1:PORT
   application.AssetOptions        Mode: 0600             Explicit Opt-In
```

### 2.1 Mode 1: Native Desktop Window (Default)
- **Trigger:** Running `localrpg gui` when a desktop display environment is active (`$DISPLAY` or `$WAYLAND_DISPLAY` is non-empty on Linux, or standard window manager on macOS/Windows).
- **Runtime:** Native Wails v3 (`github.com/wailsapp/wails/v3/pkg/application`).
- **Networking:** **0 TCP ports, 0 network listeners.**
- **Asset & API Routing:** 
  - The in-process `gui.Server` (implementing `http.Handler`) is provided to Wails as `application.AssetOptions{ Handler: server }`.
  - WebKit custom URI scheme handling intercepts all requests (e.g. `/api/game/...`, static assets, and `/`) in-memory within the application process.
  - The frontend `APIClient` makes standard `fetch('/api/game/...')` calls that are resolved in-process without network overhead.
- **Window Appearance:**
  - Configured with `BackgroundType: application.BackgroundTypeTranslucent` to complement the glassmorphic panels and dark vignette theme.
  - Dimensions: Default 1280×800, Min 900×600.
  - Auto-fallback: If no display environment is available, automatically switches to Unix domain socket mode with clear terminal logging.

### 2.2 Mode 2: Unix Domain Socket Server (`--socket [path]`)
- **Trigger:** Running `localrpg gui --socket [path]` or running in headless mode.
- **Runtime:** Go standard library HTTP server listening on a Unix domain socket via `net.Listen("unix", socketPath)`.
- **Networking:** **0 TCP ports.**
- **Socket Path Resolution:**
  1. Explicit `--socket <path>` if passed by user.
  2. Otherwise `$XDG_RUNTIME_DIR/localrpg.sock` (standard Linux per-user runtime directory).
  3. Fallback if `$XDG_RUNTIME_DIR` is unset: `~/.local/state/localrpg/gui.sock` (created with `0700` parent dir) or `/tmp/localrpg.sock`.
- **Permissions & Security:**
  - File mode set to `0600` immediately upon listener creation (`os.Chmod(socketPath, 0600)`). Only the current user process can connect.
- **Stale Socket & Lifecycle Management:**
  - Prior to binding, tests if an existing socket file is alive via a fast 100ms dial. If active, reports that another instance is running; if connection is refused, unlinks stale socket file with `os.Remove(socketPath)` and binds cleanly.
  - Intercepts `os.Interrupt` (`SIGINT`) and `syscall.SIGTERM` to gracefully shut down the server and unlink the socket file.

### 2.3 Mode 3: Explicit TCP Web Server (`--port <number>` or `--web`)
- **Trigger:** Passing `--port <number>` (e.g. `localrpg gui --port 8080`) or `--web`.
- **Runtime:** `http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), server)`.
- **Purpose:** Strictly opt-in for remote browser testing or local network access.

---

## 3. Embedded React SPA Pipeline & Asset Router

### 3.1 Build Output & Embedding
- Update `frontend/vite.config.ts` so `npm run build` outputs directly to `pkg/gui/dist` (while also maintaining `frontend/dist`).
- In `pkg/gui/assets.go`:
  ```go
  //go:embed all:dist
  var embeddedDist embed.FS
  ```
- This ensures the compiled React 19 bundle, CSS, fonts, and Lucide icons are compiled directly into the single `localrpg` executable binary.

### 3.2 SPA Asset Handler Logic
The asset handler serves the frontend transparently:
1. **API Routes (`/api/`):** Handled directly by `gui.Server.handleGameRoutes`.
2. **Static Asset Resolution:** Checks if the requested path corresponds to an existing file in `embeddedDist` (subpath `dist`), with dev fallback to `frontend/dist` if available.
3. **SPA History Fallback:** For non-API routes that do not contain a file extension (e.g. `/`, `/chronicle`, `/character`, `/graph`), serves `index.html` to allow React router / component state to mount seamlessly.
4. **Fallback Handler:** If no assets were built into `embeddedDist` at compile time, returns informative instructions on running `mise run build:frontend`.

### 3.3 Mise Task Integration
`mise.toml` task definitions ensure the frontend is always compiled before binary compilation:
- `build:frontend`: Runs Vite build, populating `pkg/gui/dist`.
- `build:backend`: Depends on `build:frontend`, builds `bin/localrpg`.
- `dev:gui`: Runs GUI in dev mode with live reload.

---

## 4. Verification & Testing Plan

1. **Unix Domain Socket Integration Test (`pkg/gui/server_test.go`):**
   - Spins up a Unix domain socket listener on a temporary path in `t.TempDir()`.
   - Connects an `http.Client` using a custom `http.Transport` dialer configured for Unix sockets:
     ```go
     client := &http.Client{
         Transport: &http.Transport{
             DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
                 return net.Dial("unix", socketPath)
             },
         },
     }
     ```
   - Verifies GET `/api/game/test-game/state`, GET `/api/game/test-game/chronicle`, and GET `/` (SPA HTML response).
   - Confirms socket cleanup and permissions `0600`.
2. **CLI Flags & Dispatch Test (`cmd/localrpg/gui_test.go`):**
   - Tests flag parsing: `--socket`, `--port`, `--dir`.
   - Tests default socket resolution when no port is given.
3. **Full Regression Test Suite:**
   - Execute `mise run test` verifying all 12 Go packages and TypeScript type checks pass cleanly.
