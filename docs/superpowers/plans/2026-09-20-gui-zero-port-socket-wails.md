# Zero-TCP GUI, Unix Domain Socket & Embedded SPA Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide zero-TCP port desktop and headless GUI execution for LocalRPG by serving the embedded React 19 SPA through native Wails v3 in-process WebKit IPC or a private Unix domain socket (`0600`), reserving TCP port binding strictly as an opt-in flag.

**Architecture:** 
- The React 19 frontend is compiled into `pkg/gui/dist` and embedded into the Go binary via `//go:embed all:dist`.
- `gui.Server` handles both `/api/game/...` REST routes and SPA static file routing (falling back to `index.html` for client-side routing).
- `pkg/gui/socket.go` implements safe Unix domain socket creation, stale socket detection, and `0600` permissions.
- `cmd/localrpg/gui.go` dispatches: native Wails v3 window (default with display), Unix domain socket (`--socket` or headless fallback), or TCP (`--port`).

**Tech Stack:** Go 1.27.1, Wails v3 (`github.com/wailsapp/wails/v3`), React 19, Vite, Tailwind CSS v4, Unix Domain Sockets (`net.Listen("unix", ...)`).

---

### Task 1: SPA Asset Bundling & Embedded FS

**Files:**
- Modify: `frontend/vite.config.ts`
- Modify: `mise.toml:12-25`
- Create: `pkg/gui/dist/.gitkeep`
- Modify: `pkg/gui/assets.go`
- Test: `pkg/gui/server_test.go`

- [ ] **Step 1: Create placeholder in `pkg/gui/dist` so `go:embed` compiles**

```bash
mkdir -p pkg/gui/dist
touch pkg/gui/dist/.gitkeep
```

- [ ] **Step 2: Update `frontend/vite.config.ts` and `mise.toml` to build into `pkg/gui/dist`**

Update `frontend/vite.config.ts`:
```typescript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8080'
    }
  },
  build: {
    outDir: path.resolve(__dirname, '../pkg/gui/dist'),
    emptyOutDir: true
  }
});
```

Update `mise.toml` task definitions for frontend and backend builds:
```toml
[tasks."build:frontend"]
description = "Build the React 19 frontend bundle into pkg/gui/dist"
dir = "frontend"
run = "npm run build && cp -r dist/* ../pkg/gui/dist/ 2>/dev/null || true"
sources = ["src/**/*", "index.html", "package.json"]
outputs = ["../pkg/gui/dist/index.html"]

[tasks."build:backend"]
description = "Build the localrpg CLI binary"
depends = ["build:frontend"]
run = "go build -o bin/localrpg ./cmd/localrpg"
sources = ["cmd/**/*", "pkg/**/*", "go.mod"]
outputs = ["bin/localrpg"]
```

- [ ] **Step 3: Write failing test for SPA asset routing in `pkg/gui/server_test.go`**

Add test to `pkg/gui/server_test.go`:
```go
func TestSPARouting(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	// Create test server with standard asset handler
	server := NewServer(svc, AssetHandler())

	// Test GET / returns index.html or fallback
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected text/html content type for /, got %s", contentType)
	}

	// Test GET /chronicle (client-side route) also returns HTML
	reqRoute := httptest.NewRequest("GET", "/chronicle", nil)
	wRoute := httptest.NewRecorder()
	server.ServeHTTP(wRoute, reqRoute)

	if wRoute.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /chronicle, got %d", wRoute.Code)
	}
	if !strings.Contains(wRoute.Header().Get("Content-Type"), "text/html") {
		t.Errorf("expected text/html for client route, got %s", wRoute.Header().Get("Content-Type"))
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test -v ./pkg/gui -run TestSPARouting`  
Expected: FAIL with `undefined: AssetHandler`

- [ ] **Step 5: Implement `AssetHandler` with embedded FS and SPA fallback in `pkg/gui/assets.go`**

Replace `pkg/gui/assets.go`:
```go
package gui

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:dist
var embeddedDist embed.FS

// FallbackAssetHandler returns a fallback HTML handler when assets are not bundled
func FallbackAssetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<!DOCTYPE html><html><body><h1>LocalRPG GUI Backend Running</h1><p>Run 'mise run build:frontend' to compile the React frontend.</p></body></html>"))
	})
}

// AssetHandler returns an http.Handler that serves the embedded React application
// with SPA fallback to index.html for client-side navigation.
func AssetHandler() http.Handler {
	distFS, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		return FallbackAssetHandler()
	}

	// Verify if dist has index.html
	if _, err := fs.Stat(distFS, "index.html"); err != nil {
		// Check local disk fallback (useful during development)
		if _, err := os.Stat("frontend/dist/index.html"); err == nil {
			return spaHandler(os.DirFS("frontend/dist"))
		}
		if _, err := os.Stat("pkg/gui/dist/index.html"); err == nil {
			return spaHandler(os.DirFS("pkg/gui/dist"))
		}
		return FallbackAssetHandler()
	}

	return spaHandler(distFS)
}

func spaHandler(fileSystem fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fileSystem))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
		if path == "" || path == "." {
			path = "index.html"
		}

		// Try opening the requested file
		f, err := fileSystem.Open(path)
		if err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// If the path contains an extension, it's a missing asset -> 404
		if strings.Contains(filepath.Base(path), ".") {
			http.NotFound(w, r)
			return
		}

		// Otherwise, it's a client-side SPA route -> serve index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 6: Build frontend bundle to populate `pkg/gui/dist` and run test**

Run:
```bash
cd frontend && npm run build && cd ..
go test -v ./pkg/gui -run TestSPARouting
```
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add frontend/vite.config.ts mise.toml pkg/gui/dist/.gitkeep pkg/gui/assets.go pkg/gui/server_test.go
git commit -m "feat(gui): embed compiled React SPA bundle and support client-side routing"
```

---

### Task 2: Unix Domain Socket Listener & Lifecycle Management

**Files:**
- Create: `pkg/gui/socket.go`
- Test: `pkg/gui/socket_test.go`
- Modify: `pkg/gui/server_test.go`

- [ ] **Step 1: Write failing tests for Unix domain socket in `pkg/gui/socket_test.go`**

Create `pkg/gui/socket_test.go`:
```go
package gui

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultSocketPath(t *testing.T) {
	path := DefaultSocketPath()
	if path == "" {
		t.Fatal("expected non-empty default socket path")
	}
	if filepath.Ext(path) != ".sock" {
		t.Errorf("expected socket extension .sock, got %s", path)
	}
}

func TestListenUnix(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := ListenUnix(sockPath)
	if err != nil {
		t.Fatalf("ListenUnix failed: %v", err)
	}
	defer listener.Close()

	// Verify file was created
	info, err := os.Stat(sockPath)
	if err != nil {
		t.Fatalf("stat socket file failed: %v", err)
	}

	// Verify file mode is 0600
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected socket permissions 0600, got %o", perm)
	}

	// Test stale socket cleanup: close listener without removing file
	_ = listener.Close()
	// Re-listen on same path should detect dead socket and succeed
	listener2, err := ListenUnix(sockPath)
	if err != nil {
		t.Fatalf("re-listening on dead socket failed: %v", err)
	}
	defer listener2.Close()
}

func TestUnixSocketHTTPCommunication(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "http_test.sock")

	svc := NewService(tmpDir)
	server := NewServer(svc, AssetHandler())

	listener, err := ListenUnix(sockPath)
	if err != nil {
		t.Fatalf("ListenUnix failed: %v", err)
	}
	defer listener.Close()

	httpServer := &http.Server{Handler: server}
	go func() {
		_ = httpServer.Serve(listener)
	}()
	defer httpServer.Close()

	// Client connecting over unix domain socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get("http://unix/api/game/test-game/state")
	if err != nil {
		t.Fatalf("HTTP request over unix socket failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/gui -run "TestDefaultSocketPath|TestListenUnix|TestUnixSocketHTTPCommunication"`  
Expected: FAIL with `undefined: DefaultSocketPath`, `undefined: ListenUnix`

- [ ] **Step 3: Implement `DefaultSocketPath` and `ListenUnix` in `pkg/gui/socket.go`**

Create `pkg/gui/socket.go`:
```go
package gui

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// DefaultSocketPath returns the standard Unix domain socket path for LocalRPG.
// Priority:
// 1. $XDG_RUNTIME_DIR/localrpg.sock
// 2. ~/.local/state/localrpg/gui.sock
// 3. /tmp/localrpg-<uid>.sock
func DefaultSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "localrpg.sock")
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dir := filepath.Join(home, ".local", "state", "localrpg")
		_ = os.MkdirAll(dir, 0700)
		return filepath.Join(dir, "gui.sock")
	}

	return filepath.Join(os.TempDir(), fmt.Sprintf("localrpg-%d.sock", os.Getuid()))
}

// ListenUnix creates a net.Listener on a Unix domain socket.
// It handles stale socket cleanup and enforces 0600 permissions.
func ListenUnix(socketPath string) (net.Listener, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating socket directory %s: %w", dir, err)
	}

	// Check if socket file exists
	if _, err := os.Stat(socketPath); err == nil {
		// Attempt a quick connection to check if an active instance is alive
		conn, dialErr := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("another LocalRPG instance is already running on socket %s", socketPath)
		}
		// Connection refused -> stale socket file, remove it
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("removing stale socket %s: %w", socketPath, err)
		}
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listening on socket %s: %w", socketPath, err)
	}

	// Restrict permissions to owner only
	if err := os.Chmod(socketPath, 0600); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("setting socket permissions %s: %w", socketPath, err)
	}

	return listener, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/gui -run "TestDefaultSocketPath|TestListenUnix|TestUnixSocketHTTPCommunication"`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/socket.go pkg/gui/socket_test.go
git commit -m "feat(gui): implement unix domain socket listener and lifecycle management"
```

---

### Task 3: Wails v3 Native Desktop Integration & CLI Dispatch

**Files:**
- Modify: `go.mod`
- Modify: `cmd/localrpg/gui.go`
- Test: `cmd/localrpg/gui_test.go`

- [ ] **Step 1: Add `github.com/wailsapp/wails/v3` dependency**

Run:
```bash
go get github.com/wailsapp/wails/v3@v3.0.0-beta.24
go mod tidy
```

- [ ] **Step 2: Write failing test in `cmd/localrpg/gui_test.go`**

Update `cmd/localrpg/gui_test.go` to test flag parsing and configuration:
```go
package main

import (
	"testing"
)

func TestParseGUIConfig(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantPort   int
		wantSocket string
		wantWeb    bool
	}{
		{
			name:       "default flags",
			args:       []string{},
			wantPort:   0,
			wantSocket: "",
			wantWeb:    false,
		},
		{
			name:       "explicit socket",
			args:       []string{"--socket", "/tmp/custom.sock"},
			wantPort:   0,
			wantSocket: "/tmp/custom.sock",
			wantWeb:    false,
		},
		{
			name:       "explicit port",
			args:       []string{"--port", "9090"},
			wantPort:   9090,
			wantSocket: "",
			wantWeb:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseGUIConfig(tt.args)
			if err != nil {
				t.Fatalf("parseGUIConfig failed: %v", err)
			}
			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantPort)
			}
			if cfg.SocketPath != tt.wantSocket {
				t.Errorf("SocketPath = %q, want %q", cfg.SocketPath, tt.wantSocket)
			}
			if cfg.WebMode != tt.wantWeb {
				t.Errorf("WebMode = %v, want %v", cfg.WebMode, tt.wantWeb)
			}
		})
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -v ./cmd/localrpg -run TestParseGUIConfig`  
Expected: FAIL with `undefined: parseGUIConfig`

- [ ] **Step 4: Implement Wails v3 Desktop and Socket Dispatch in `cmd/localrpg/gui.go`**

Update `cmd/localrpg/gui.go`:
```go
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type guiConfig struct {
	Dir        string
	Port       int
	SocketPath string
	WebMode    bool
	Headless   bool
}

func parseGUIConfig(args []string) (*guiConfig, error) {
	fs := flag.NewFlagSet("gui", flag.ContinueOnError)
	port := fs.Int("port", 0, "Explicit TCP port to bind (opt-in web server)")
	socket := fs.String("socket", "", "Unix domain socket path (defaults to user runtime socket)")
	dir := fs.String("dir", ".", "Project root directory")
	headless := fs.Bool("headless", false, "Run in headless mode (listen on Unix domain socket)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &guiConfig{
		Dir:        *dir,
		Port:       *port,
		SocketPath: *socket,
		WebMode:    *port > 0,
		Headless:   *headless,
	}
	return cfg, nil
}

func handleGUICommand(args []string) {
	cfg, err := parseGUIConfig(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		os.Exit(1)
	}

	svc := gui.NewService(cfg.Dir)
	server := gui.NewServer(svc, gui.AssetHandler())

	// 1. Explicit TCP Web Mode
	if cfg.WebMode {
		addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
		fmt.Printf("Starting LocalRPG Web GUI on http://%s\n", addr)
		if err := http.ListenAndServe(addr, server); err != nil {
			fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 2. Explicit or Headless Unix Domain Socket Mode
	hasDisplay := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	if cfg.Headless || cfg.SocketPath != "" || !hasDisplay {
		sockPath := cfg.SocketPath
		if sockPath == "" {
			sockPath = gui.DefaultSocketPath()
		}

		listener, err := gui.ListenUnix(sockPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Socket error: %v\n", err)
			os.Exit(1)
		}
		defer listener.Close()
		defer os.Remove(sockPath)

		// Handle graceful shutdown signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Println("\nShutting down LocalRPG GUI socket daemon...")
			_ = listener.Close()
			_ = os.Remove(sockPath)
			os.Exit(0)
		}()

		fmt.Printf("LocalRPG GUI daemon listening on Unix domain socket: %s (0 TCP ports)\n", sockPath)
		if !hasDisplay && !cfg.Headless {
			fmt.Println("Note: No desktop display detected ($DISPLAY / $WAYLAND_DISPLAY unset).")
		}
		if err := http.Serve(listener, server); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Daemon failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 3. Default: Native Wails v3 Desktop Window (Zero-TCP)
	app := application.New(application.Options{
		Name:        "LocalRPG",
		Description: "Local-First LLM Tabletop RPG Client",
		Assets: application.AssetOptions{
			Handler: server,
		},
	})

	app.NewWebviewWindowWithOptions(application.WebviewWindowOptions{
		Title:          "LocalRPG",
		Width:          1280,
		Height:         800,
		MinWidth:       900,
		MinHeight:      600,
		URL:            "/",
		BackgroundType: application.BackgroundTypeTranslucent,
	})

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Wails application failed: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -v ./cmd/localrpg -run TestParseGUIConfig`  
Expected: PASS

- [ ] **Step 6: Verify full test suite across all packages**

Run: `mise run test`  
Expected: PASS across all 12 packages and frontend TypeScript checks.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum cmd/localrpg/gui.go cmd/localrpg/gui_test.go
git commit -m "feat(gui): wire wails v3 native desktop window and unix socket daemon"
```

---

### Task 4: Build Verification & Documentation Update

**Files:**
- Modify: `README.md`
- Verify: Full binary build with `mise run build`

- [ ] **Step 1: Update README.md with zero-TCP GUI execution documentation**

Update `README.md` GUI section:
```markdown
### Running the GUI

LocalRPG runs **zero-TCP by default**:

- **Native Desktop App (Default):**
  ```bash
  localrpg gui
  ```
  Launches a native Wails v3 window with glassmorphic Twintail Launcher styling. 0 TCP ports or network listeners are opened; assets and IPC are handled directly in-process.

- **Headless / Unix Domain Socket:**
  ```bash
  localrpg gui --headless
  # Or custom socket:
  localrpg gui --socket /path/to/localrpg.sock
  ```
  Runs a private local daemon over a Unix domain socket with `0600` permissions (readable/writable only by your user).

- **Web Browser Mode (Opt-In):**
  ```bash
  localrpg gui --port 8080
  ```
  Explicitly exposes an HTTP server on `127.0.0.1:8080` for standard web browsers.
```

- [ ] **Step 2: Execute full build and verification**

Run:
```bash
mise run build
./bin/localrpg gui --help
./bin/localrpg gui --socket /tmp/test-localrpg.sock &
PID=$!
sleep 1
curl --unix-socket /tmp/test-localrpg.sock http://localhost/api/game/test-game/state
kill $PID
rm -f /tmp/test-localrpg.sock
```
Expected: All build steps pass, curl over unix socket returns valid JSON with 0 TCP ports opened.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document zero-tcp gui execution modes and unix domain sockets"
```
