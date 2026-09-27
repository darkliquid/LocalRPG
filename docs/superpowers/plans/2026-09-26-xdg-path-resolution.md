# XDG Path Resolution Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Derive storage and config paths from the XDG base directories, with a single resolution point used by the GUI, CLI, and export.

**Architecture:** A new `pkg/paths` package wraps `github.com/adrg/xdg` (already an indirect dependency) and turns a config's `paths.*` into absolute directories. Defaults become empty, resolution supplies them, and project mode (`--dir` or `./localrpg.yaml`) keeps today's project-relative behaviour.

**Tech Stack:** Go 1.27 (stdlib `testing`), `github.com/adrg/xdg`. No other new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-26-xdg-path-resolution-design.md`

## Global Constraints

- Go tests use only `testing` and `t.TempDir()`; no testify, no mocks.
- Use `interface{}`, never `any`; wrap errors with `fmt.Errorf("...: %w", err)`; `go vet ./...` clean.
- `github.com/adrg/xdg` is already at v0.5.3 in `go.sum`; only move it to a direct require, do not upgrade.
- Resolution must be pure: no directory creation, no reads, so a bad config writes nothing on load.
- Conventional Commits with a scope; subject under 72 chars.
- Verification: `mise run test`, `mise run lint`, `mise run build`.
- Known pre-existing `pkg/gui` TempDir flake — re-run before treating a failure as real.

---

### Task 1: The `pkg/paths` package

**Files:**
- Create: `pkg/paths/paths.go`
- Modify: `go.mod` (move `github.com/adrg/xdg v0.5.3` out of the indirect block)
- Test: `pkg/paths/paths_test.go`

**Interfaces:**
- Produces:

```go
type Bases struct{ Config, Data, Cache string }
func System() Bases
func App(base string) string
type Dirs struct{ Systems, Worlds, Games, Cache string }
func Resolve(bases Bases, paths config.PathsConfig, rootDir string) Dirs
func ConfigFile() (read, write string, err error)
```

- [x] **Step 1: Write the failing test**

Create `pkg/paths/paths_test.go`:

```go
package paths

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestResolveGlobalDefaults(t *testing.T) {
	bases := Bases{
		Config: filepath.Join(t.TempDir(), "config"),
		Data:   filepath.Join(t.TempDir(), "data"),
		Cache:  filepath.Join(t.TempDir(), "cache"),
	}
	dirs := Resolve(bases, config.PathsConfig{}, "")

	if dirs.Systems != filepath.Join(bases.Data, "localrpg", "systems") {
		t.Errorf("Systems = %q", dirs.Systems)
	}
	if dirs.Games != filepath.Join(bases.Data, "localrpg", "games") {
		t.Errorf("Games = %q", dirs.Games)
	}
	if dirs.Cache != filepath.Join(bases.Cache, "localrpg") {
		t.Errorf("Cache = %q", dirs.Cache)
	}
	if filepath.Dir(dirs.Cache) == filepath.Dir(dirs.Systems) {
		t.Error("cache and data must use different bases")
	}
}

func TestResolveRelativeAndAbsolute(t *testing.T) {
	bases := Bases{Data: filepath.Join(t.TempDir(), "data"), Cache: filepath.Join(t.TempDir(), "cache")}

	global := Resolve(bases, config.PathsConfig{Systems: "shared/systems", Cache: "clips"}, "")
	if global.Systems != filepath.Join(bases.Data, "localrpg", "shared/systems") {
		t.Errorf("relative global Systems = %q", global.Systems)
	}
	if global.Cache != filepath.Join(bases.Cache, "localrpg", "clips") {
		t.Errorf("relative global Cache = %q", global.Cache)
	}

	abs := "/opt/localrpg/systems"
	if got := Resolve(bases, config.PathsConfig{Systems: abs}, "").Systems; got != abs {
		t.Errorf("absolute Systems = %q, want %q", got, abs)
	}
}

func TestResolveProjectMode(t *testing.T) {
	root := t.TempDir()
	bases := Bases{Data: filepath.Join(t.TempDir(), "data"), Cache: filepath.Join(t.TempDir(), "cache")}

	dirs := Resolve(bases, config.PathsConfig{Systems: "systems", Cache: "cache"}, root)
	if dirs.Systems != filepath.Join(root, "systems") {
		t.Errorf("project Systems = %q", dirs.Systems)
	}
	if dirs.Cache != filepath.Join(root, "cache") {
		t.Errorf("project Cache = %q", dirs.Cache)
	}

	empty := Resolve(bases, config.PathsConfig{}, root)
	if empty.Systems != filepath.Join(root, "systems") || empty.Cache != filepath.Join(root, "cache") {
		t.Errorf("project empty defaults = %+v", empty)
	}

	abs := filepath.Join(root, "elsewhere", "systems")
	if got := Resolve(bases, config.PathsConfig{Systems: abs}, root).Systems; got != abs {
		t.Errorf("absolute wins in project mode: %q", got)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/paths/ -v`
Expected: FAIL — package does not exist.

- [x] **Step 3: Implement the package**

Create `pkg/paths/paths.go`:

```go
// Package paths resolves LocalRPG's storage and config locations from the XDG
// base directories, so a fresh install does not scatter content around the
// working directory.
package paths

import (
	"os"
	"path/filepath"

	"github.com/adrg/xdg"

	"github.com/darkliquid/localrpg/pkg/config"
)

// Bases are the three roots a configured path can be relative to. Production
// fills them from xdg; a test fills them with temp directories.
type Bases struct {
	Config string
	Data   string
	Cache  string
}

// System reads the XDG bases, honouring the XDG_* environment and falling back
// to the OS-native locations on platforms that do not use XDG.
func System() Bases {
	return Bases{Config: xdg.ConfigHome, Data: xdg.DataHome, Cache: xdg.CacheHome}
}

// App is the application directory under a base.
func App(base string) string { return filepath.Join(base, "localrpg") }

// Dirs are the resolved, absolute directories a campaign uses.
type Dirs struct {
	Systems string
	Worlds  string
	Games   string
	Cache   string
}

// Resolve turns a config's paths into absolute directories. An empty value uses
// the default; an absolute value is used verbatim; a relative value joins
// rootDir when one is set (project mode) or the category base otherwise.
func Resolve(bases Bases, paths config.PathsConfig, rootDir string) Dirs {
	data := App(bases.Data)
	cache := App(bases.Cache)
	return Dirs{
		Systems: resolveValue(paths.Systems, data, filepath.Join(data, "systems"), "systems", rootDir),
		Worlds:  resolveValue(paths.Worlds, data, filepath.Join(data, "worlds"), "worlds", rootDir),
		Games:   resolveValue(paths.Games, data, filepath.Join(data, "games"), "games", rootDir),
		Cache:   resolveValue(paths.Cache, cache, cache, "cache", rootDir),
	}
}

func resolveValue(value, relBase, globalDefault, projectName, rootDir string) string {
	if value == "" {
		if rootDir != "" {
			return filepath.Join(rootDir, projectName)
		}
		return globalDefault
	}
	if filepath.IsAbs(value) {
		return value
	}
	if rootDir != "" {
		return filepath.Join(rootDir, value)
	}
	return filepath.Join(relBase, value)
}

// ConfigFile returns the config file to read and where to write one. An explicit
// LOCALRPG_CONFIG_DIR wins; otherwise the XDG config search path is used, and the
// write path is the application's config directory.
func ConfigFile() (read, write string, err error) {
	if dir := os.Getenv("LOCALRPG_CONFIG_DIR"); dir != "" {
		path := filepath.Join(dir, "config.yaml")
		return path, path, nil
	}
	rel := filepath.Join("localrpg", "config.yaml")
	if found, searchErr := xdg.SearchConfigFile(rel); searchErr == nil && found != "" {
		return found, found, nil
	}
	fallback := xdg.ConfigFile(rel)
	return fallback, fallback, nil
}
```

- [x] **Step 4: Promote the dependency**

Run: `go mod edit -require=github.com/adrg/xdg@v0.5.3 && go mod tidy`
Expected: `go.mod` lists `github.com/adrg/xdg v0.5.3` in the direct require block; `go.sum` unchanged.

- [x] **Step 5: Run test to verify it passes**

Run: `go test ./pkg/paths/ -v`
Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add pkg/paths go.mod go.sum
git commit -m "feat(paths): resolve storage locations from the XDG bases"
```

---

### Task 2: Empty path defaults

**Files:**
- Modify: `pkg/config/types.go` (`DefaultConfig` `Paths` ~line 272-277)
- Test: `pkg/config/types_test.go` (append)

**Interfaces:**
- Produces: `DefaultConfig().Paths` is all empty strings; resolution supplies defaults.

- [x] **Step 1: Write the failing test**

Append to `pkg/config/types_test.go`:

```go
func TestDefaultPathsAreEmptySoResolutionSuppliesThem(t *testing.T) {
	paths := DefaultConfig().Paths
	if paths.Systems != "" || paths.Worlds != "" || paths.Games != "" || paths.Cache != "" {
		t.Fatalf("default paths = %+v, want empty", paths)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestDefaultPathsAreEmpty ./pkg/config/ -v`
Expected: FAIL — defaults are `./systems` etc.

- [x] **Step 3: Change the defaults**

In `pkg/config/types.go`:

```go
		Paths: PathsConfig{},
```

Leave `PathsConfig` as-is. Check for other assertions on the old defaults:
`grep -rn "\./systems\|\./worlds\|\./games\|\./cache" pkg --include=*_test.go` and update any that assert the old defaults (they now assert empty or a resolved value).

- [x] **Step 4: Run the package**

Run: `go test ./pkg/config/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go
git commit -m "refactor(config): leave path defaults to resolution"
```

---

### Task 3: Config discovery via the XDG search path

**Files:**
- Modify: `pkg/config/manager.go` (`NewConfigManager` ~line 20-31)
- Test: `pkg/config/manager_test.go` (append)

**Interfaces:**
- Consumes: `paths.ConfigFile()`.
- Produces: config discovery honours `XDG_CONFIG_HOME` and `XDG_CONFIG_DIRS`; `LOCALRPG_CONFIG_DIR` still wins.

- [x] **Step 1: Write the failing test**

Append to `pkg/config/manager_test.go`:

```go
func TestConfigManagerFindsTheXDGConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CONFIG_DIRS", "")
	xdg.Reload()
	t.Cleanup(xdg.Reload)

	mgr := NewConfigManager()
	want := filepath.Join(dir, "localrpg", "config.yaml")
	if mgr.userConfigPath != want {
		t.Fatalf("userConfigPath = %q, want %q", mgr.userConfigPath, want)
	}
}
```

Add `github.com/adrg/xdg` and `path/filepath` to the test imports; it must be a direct requirement (done in Task 1).

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestConfigManagerFindsTheXDGConfig ./pkg/config/ -v`
Expected: FAIL — the manager builds the path by hand and ignores the library (and `XDG_CONFIG_DIRS`).

- [x] **Step 3: Use the package**

In `pkg/config/manager.go`:

```go
func NewConfigManager() *ConfigManager {
	read, write, err := paths.ConfigFile()
	if err != nil || read == "" {
		read, write = write, write
	}
	userPath := write
	if read != "" {
		userPath = read
	}
	localPath := "./localrpg.yaml"
	return NewConfigManagerWithPaths(userPath, localPath)
}
```

Import `github.com/darkliquid/localrpg/pkg/paths`. Confirm no import cycle: `pkg/paths` imports `pkg/config`, so `pkg/config` importing `pkg/paths` **is** a cycle. **Resolve by moving `ConfigFile` into `pkg/config`** (it is config discovery, and `pkg/config` may import `github.com/adrg/xdg` directly), and keep `pkg/paths` for path resolution only. Update Task 1 accordingly: `ConfigFile` becomes `config.DetectConfigFile() (read, write string)`, and Task 1's test for it moves to `pkg/config`.

Concretely:

```go
// pkg/config/manager.go
func DetectConfigFile() (read, write string) {
	if dir := os.Getenv("LOCALRPG_CONFIG_DIR"); dir != "" {
		path := filepath.Join(dir, "config.yaml")
		return path, path
	}
	rel := filepath.Join("localrpg", "config.yaml")
	if found, err := xdg.SearchConfigFile(rel); err == nil && found != "" {
		return found, found
	}
	fallback := xdg.ConfigFile(rel)
	return fallback, fallback
}

func NewConfigManager() *ConfigManager {
	read, write := DetectConfigFile()
	_ = write
	return NewConfigManagerWithPaths(read, "./localrpg.yaml")
}
```

(`write` is unused here because `Save` writes to `userConfigPath`; when no file exists, `read` is already the application config path, which is where a new file should go.)

- [x] **Step 4: Run test and the package**

Run: `go test -run TestConfigManagerFindsTheXDGConfig ./pkg/config/ -v && go test ./pkg/config/ ./pkg/paths/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/config/manager.go pkg/config/manager_test.go
git commit -m "feat(config): discover config.yaml on the XDG search path"
```

---

### Task 4: Wire the GUI

**Files:**
- Modify: `pkg/gui/service.go` (`NewService` ~line 77-121, `SaveSettings` path block ~line 2711-2730)
- Test: `pkg/gui/paths_test.go` (create)

**Interfaces:**
- Consumes: `paths.System`, `paths.Resolve`.
- Produces: `NewService` classifies `""`/`"."` as global mode and anything else as project mode.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/paths_test.go`:

```go
package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
)

func TestServiceGlobalModeUsesXDG(t *testing.T) {
	data := t.TempDir()
	cache := t.TempDir()
	cfgHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("LOCALRPG_CONFIG_DIR", "")
	xdg.Reload()
	t.Cleanup(xdg.Reload)

	svc := NewService(".")
	resolver := svc.GetResolver()
	if !strings.HasPrefix(resolver.SystemsDir(), data) {
		t.Errorf("SystemsDir = %q, want under %q", resolver.SystemsDir(), data)
	}
	if !strings.HasPrefix(resolver.CacheDir(), cache) {
		t.Errorf("CacheDir = %q, want under %q", resolver.CacheDir(), cache)
	}
	if _, err := os.Stat(filepath.Join(".", "systems")); err == nil {
		t.Error("global mode must not create ./systems")
	}
}

func TestServiceProjectModeStaysRelative(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	if got := svc.GetResolver().SystemsDir(); got != filepath.Join(root, "systems") {
		t.Fatalf("SystemsDir = %q, want under the project root", got)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestServiceGlobalMode|TestServiceProjectMode' ./pkg/gui/ -v`
Expected: FAIL — global mode still resolves `./systems`.

- [x] **Step 3: Replace the discovery and join logic**

In `pkg/gui/service.go`, replace the config-dir block and the `IsAbs` joins with:

```go
func NewService(rootDir string) *Service {
	projectMode := rootDir != "" && rootDir != "."
	read, _ := config.DetectConfigFile()
	userPath := read
	localPath := filepath.Join(rootDir, "localrpg.yaml")
	if projectMode {
		userPath = filepath.Join(rootDir, "config.yaml")
	}
	mgr := config.NewConfigManagerWithPaths(userPath, localPath)
	cfg, _ := mgr.Load()

	projectRoot := ""
	if projectMode {
		projectRoot = rootDir
	}
	dirs := paths.Resolve(paths.System(), cfg.Paths, projectRoot)

	return &Service{
		rootDir:        rootDir,
		resolver:       core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache),
		configMgr:      mgr,
		// …existing fields…
		modelsManager:  models.NewManager(dirs.Cache),
	}
}
```

Keep `s.rootDir` for the project-mode flag. Update `SaveSettings`'s path block to the same rule:

```go
	projectRoot := ""
	if s.rootDir != "" && s.rootDir != "." {
		projectRoot = s.rootDir
	}
	dirs := paths.Resolve(paths.System(), cfg.Paths, projectRoot)
	s.mu.Lock()
	s.resolver.SetPaths(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache)
	s.mu.Unlock()
	_ = os.MkdirAll(dirs.Systems, 0755)
	_ = os.MkdirAll(dirs.Worlds, 0755)
	_ = os.MkdirAll(dirs.Games, 0755)
	_ = os.MkdirAll(dirs.Cache, 0755)
```

- [x] **Step 4: Run tests and the package**

Run: `go test -run 'TestServiceGlobalMode|TestServiceProjectMode' ./pkg/gui/ -v && go test ./pkg/gui/`
Expected: PASS (re-run once on the known flake).

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/paths_test.go
git commit -m "feat(gui): resolve paths from XDG unless a project root is set"
```

---

### Task 5: Wire the CLI and export

**Files:**
- Modify: `cmd/localrpg/play.go` (~line 48)
- Modify: `cmd/localrpg/media.go` (~line 59)
- Modify: `pkg/export/script.go` (`NewScriptCompiler` ~line 88-96)
- Test: `pkg/export/paths_test.go` (create); CLI covered by build + existing command tests

**Interfaces:**
- Consumes: `paths.Resolve`, `config.DetectConfigFile`.

- [x] **Step 1: Write the failing test**

Create `pkg/export/paths_test.go`:

```go
package export

import (
	"path/filepath"
	"testing"
)

func TestScriptCompilerUsesTheResolvedCache(t *testing.T) {
	root := t.TempDir()
	compiler := NewScriptCompiler(root)
	if got := compiler.resolver.CacheDir(); got != filepath.Join(root, "cache") {
		t.Fatalf("CacheDir = %q, want %q", got, filepath.Join(root, "cache"))
	}
}
```

- [x] **Step 2: Run test to verify it fails or passes for the right reason**

Run: `go test -run TestScriptCompilerUsesTheResolvedCache ./pkg/export/ -v`
Expected: PASS against the current `NewPathResolver(rootDir)`; keep it green after Step 3 (the resolved project cache is `rootDir/cache`, unchanged).

- [x] **Step 3: Switch the entry points**

`pkg/export/script.go`:

```go
func NewScriptCompiler(rootDir string) *ScriptCompiler {
	cfg, _ := config.NewConfigManager().Load()
	projectRoot := ""
	if rootDir != "" && rootDir != "." {
		projectRoot = rootDir
	}
	dirs := paths.Resolve(paths.System(), cfg.Paths, projectRoot)
	return &ScriptCompiler{
		rootDir:  rootDir,
		resolver: core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache),
		config:   cfg,
		art:      true,
		audio:    true,
	}
}
```

`cmd/localrpg/play.go`:

```go
	cfg, _ := config.NewConfigManager().Load()
	dirs := paths.Resolve(paths.System(), cfg.Paths, "")
	pathsResolver := core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache)
```

(Use the existing local variable name for `cfg`; only the resolver construction changes.)

`cmd/localrpg/media.go`:

```go
	dirs := paths.Resolve(paths.System(), cfg.Paths, "")
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(dirs.Cache))
```

Check each command's `cfg` load order so the config is loaded before resolution (play and media already load it).

- [x] **Step 4: Build and run tests**

Run: `go build ./... && go test ./pkg/export/ ./cmd/...`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add cmd/localrpg/play.go cmd/localrpg/media.go pkg/export/script.go pkg/export/paths_test.go
git commit -m "feat(cli): resolve command paths from the XDG bases"
```

---

### Task 6: Legacy-relative warning and docs

**Files:**
- Modify: `pkg/paths/paths.go` (add `LegacyWarning`)
- Modify: `pkg/gui/service.go` (`NewService`, log once)
- Modify: `AGENTS.md` (migration note)
- Test: `pkg/paths/paths_test.go` (append)

**Interfaces:**
- Produces: `paths.LegacyWarning(bases Bases, paths config.PathsConfig) string` — a non-empty message when a legacy cwd directory exists and the resolved XDG directory is empty, else "".

- [x] **Step 1: Write the failing test**

Append to `pkg/paths/paths_test.go`:

```go
func TestLegacyWarningPointsAtTheWorkingDirectory(t *testing.T) {
	base := Bases{Data: filepath.Join(t.TempDir(), "data"), Cache: filepath.Join(t.TempDir(), "cache")}
	if got := LegacyWarning(base, config.PathsConfig{}); got != "" {
		t.Fatalf("no legacy directory exists, got %q", got)
	}
	// Create a legacy ./systems in a temp cwd and resolve against an empty XDG.
	dir := t.TempDir()
	legacy := filepath.Join(dir, "systems")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if got := LegacyWarning(base, config.PathsConfig{}); got == "" {
		t.Fatal("expected a warning for a legacy ./systems")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestLegacyWarning ./pkg/paths/ -v`
Expected: FAIL — undefined.

- [x] **Step 3: Implement the warning**

In `pkg/paths/paths.go`:

```go
// LegacyWarning reports a configured path that will move under the XDG bases
// while the old working-directory copy still exists, so a user can re-point it.
// It performs no migration and creates nothing.
func LegacyWarning(bases Bases, paths config.PathsConfig) string {
	keys := []struct {
		name   string
		value  string
		legacy string
	}{
		{"systems", paths.Systems, "systems"},
		{"worlds", paths.Worlds, "worlds"},
		{"games", paths.Games, "games"},
		{"cache", paths.Cache, "cache"},
	}
	dirs := Resolve(bases, paths, "")
	resolved := map[string]string{"systems": dirs.Systems, "worlds": dirs.Worlds, "games": dirs.Games, "cache": dirs.Cache}
	for _, key := range keys {
		if key.value != "" {
			continue
		}
		if _, err := os.Stat(key.legacy); err != nil {
			continue
		}
		if _, err := os.Stat(resolved[key.name]); err == nil {
			continue
		}
		return fmt.Sprintf("paths.legacy_relative detected ./%s but %s is empty; set paths.%s to an absolute path to keep using it", key.legacy, resolved[key.name], key.name)
	}
	return ""
}
```

In `NewService`, after resolving dirs, log it once:

```go
	if warning := paths.LegacyWarning(paths.System(), cfg.Paths); warning != "" {
		trace.OrNil(s.logger).Event("paths.legacy_relative", map[string]interface{}{"detail": warning})
	}
```

(Use the logger the service already owns; `NewService` runs before a logger is set in some paths, so guard with `trace.OrNil`.)

- [x] **Step 4: Document the change**

Append to `AGENTS.md` under a path/config note: global config paths are resolved against the XDG bases; a relative path means "relative to the XDG category base" unless `--dir` or `./localrpg.yaml` puts the process in project mode; no content is migrated.

- [x] **Step 5: Run tests and the suite**

Run: `go test ./pkg/paths/ ./pkg/gui/ && mise run test`
Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add pkg/paths/paths.go pkg/paths/paths_test.go pkg/gui/service.go AGENTS.md
git commit -m "feat(paths): warn about legacy paths that moved under XDG"
```

---

### Task 7: Full verification

- [x] **Step 1: Run the whole suite**

Run: `mise run test`
Expected: PASS (re-run `./pkg/gui/` on the known flake).

- [x] **Step 2: Vet and build**

Run: `mise run lint && mise run build`
Expected: clean.

- [x] **Step 3: Manual smoke**

Run `mise run dev:gui` from a directory with no config: confirm it does not create `./systems` and that content lands under the XDG data dir. Then run it with `--dir .` and confirm the old project-relative behaviour.

---

## Self-Review Notes

- Spec coverage: resolution package and defaults → Tasks 1-2; config discovery → Task 3; GUI wiring → Task 4; CLI/export wiring → Task 5; legacy warning and docs → Task 6.
- Import cycle avoided explicitly: `pkg/paths` imports `pkg/config` for `PathsConfig`, so config discovery lives in `pkg/config` (`DetectConfigFile`) and `pkg/paths` stays resolution-only.
- Type consistency: `paths.Bases`, `paths.System`, `paths.App`, `paths.Resolve`, `paths.Dirs`, `config.DetectConfigFile`, `paths.LegacyWarning` are used consistently across tasks.
