# XDG Path Resolution Design

**Date:** 2026-09-26
**Status:** Proposed
**Scope:** Config discovery and the default/resolution rules for `paths.*`, across the GUI, CLI, and export entry points
**Related:** `pkg/config`, `pkg/core` (`PathResolver`), `pkg/gui/service.go`, `cmd/localrpg/*`, `pkg/export/script.go`

## 1. Overview & Goals

Storage and discovery paths default to folders relative to the process working
directory: `DefaultConfig` hardcodes `./systems`, `./worlds`, `./games`,
`./cache` (`pkg/config/types.go:272-277`), the GUI joins relative paths to
`rootDir` when `--dir` is set (`pkg/gui/service.go:95-110`), and the CLI/export
use `cfg.Paths.*` directly, so an unset value means "relative to cwd"
(`cmd/localrpg/play.go:48`, `cmd/localrpg/media.go:59`,
`pkg/export/script.go:91`). Config discovery is already XDG-ish
(`pkg/config/manager.go:20-31`).

This specification derives defaults from the XDG base directories (via
`github.com/adrg/xdg`, which already implements the cross-platform fallbacks
and is an existing indirect dependency), finds the config file through the XDG
config search path, and defines exactly what a relative path means.

**Goals:**

- Default `systems`/`worlds`/`games` under the XDG data directory and `cache`
  under the XDG cache directory, app-scoped as `localrpg`.
- Find `config.yaml` using the XDG config search path (`XDG_CONFIG_HOME` plus
  `XDG_CONFIG_DIRS`), falling back to the app's config dir for the first save.
- `paths.*` in the config are absolute; a relative value is resolved against
  the relevant XDG base (data for content, cache for cache).
- Under a local override (`./localrpg.yaml`) or `--dir`, a relative value stays
  relative to that project root, so local development is unchanged.
- One resolution point used by every entry point, instead of ad-hoc joins.

**Non-Goals:**

- Migrating existing installs (chosen: none). The meaning of a relative path
  under the global config changes; a user re-points their config.
- A settings UI for these paths (the existing Paths tab is unchanged except for
  what it displays).
- Any change to the on-disk layout inside a data directory.

**Success Criteria:**

- With no config file and no `--dir`, a first run resolves `systems`, `worlds`,
  `games` under `xdg.DataHome/localrpg` and `cache` under
  `xdg.CacheHome/localrpg`, and creates nothing in the working directory.
- With a config at `$XDG_CONFIG_HOME/localrpg/config.yaml`, discovery loads it.
- An absolute value in `paths.*` is used verbatim; a relative value in the
  global config is joined to the category base; a relative value under `--dir`
  or `./localrpg.yaml` is joined to that root.
- The GUI, `localrpg play`, `localrpg tts/image`, and `localrpg export` all
  resolve the same directories for the same config.
- macOS and Windows use `adrg/xdg`'s native fallbacks with no LocalRPG-specific
  branching.

## 2. Investigation Findings

- `PathsConfig` is four strings (`pkg/config/types.go:8-13`); defaults are
  `./…` (`:272-277`).
- `core.PathResolver` holds four unexported dirs plus `BaseDir`
  (`pkg/core/types.go:78-115`) and is built either by `NewPathResolver(baseDir)`
  (subdirs of one base) or `NewCustomPathResolver(...)`; the GUI uses the latter
  with per-key paths (`pkg/gui/service.go:114`).
- `gui.NewService(rootDir)` derives config and paths itself
  (`pkg/gui/service.go:77-121`); when `rootDir != "" && rootDir != "."` it reads
  `<rootDir>/config.yaml` and joins relative paths to `rootDir`. `SetPaths`
  updates them later (`pkg/gui/service.go:2711-2730`).
- `cmd/localrpg/play.go:48` builds a resolver from `cfg.Paths.*`;
  `cmd/localrpg/media.go:59` uses `cfg.Paths.Cache` directly;
  `pkg/export/script.go:91` uses `NewPathResolver(rootDir)`.
- `config.NewConfigManager` already prefers `$LOCALRPG_CONFIG_DIR`, then
  `$XDG_CONFIG_HOME/localrpg`, then `~/.config/localrpg`
  (`pkg/config/manager.go:20-31`), but ignores `XDG_CONFIG_DIRS` and does not use
  the library.
- `github.com/adrg/xdg v0.5.3` is present as an indirect dependency
  (`go.mod:43`); it exposes `ConfigHome`, `ConfigDirs`, `DataHome`, `CacheHome`,
  `SearchConfigFile`, `ConfigFile`/`DataFile`/`CacheFile`, and `Reload`.

## 3. Design

### 3.1 A single resolution package

New `pkg/paths`, which depends only on `pkg/config` and `adrg/xdg`.

```go
// pkg/paths

// Bases are the three roots a path can be relative to. Production fills them
// from xdg; a test fills them with a temp directory, so resolution is hermetic.
type Bases struct {
	Config string // e.g. $XDG_CONFIG_HOME
	Data   string // e.g. $XDG_DATA_HOME
	Cache  string // e.g. $XDG_CACHE_HOME
}

// System reads the XDG bases, honouring the XDG_* environment and falling back
// to the OS-native locations on platforms without them.
func System() Bases

// App returns the app-scoped directory under a base: <base>/localrpg.
func App(base string) string

// Dirs are the resolved, absolute directories.
type Dirs struct {
	Systems string
	Worlds  string
	Games   string
	Cache   string
}

// Resolve turns a config's paths into absolute directories. An empty value uses
// the category default; an absolute value is used verbatim; a relative value is
// joined to rootDir when one is set (project mode), otherwise to the category
// base (global mode).
func Resolve(bases Bases, paths config.PathsConfig, rootDir string) Dirs

// ConfigFile returns the config file to read, searching the XDG config path,
// and the path to write when none exists.
func ConfigFile() (read string, write string, err error)
```

Resolution, per key:

| key | category base | empty default |
|---|---|---|
| `systems` | `App(bases.Data)` | `<base>/systems` |
| `worlds` | `App(bases.Data)` | `<base>/worlds` |
| `games` | `App(bases.Data)` | `<base>/games` |
| `cache` | `App(bases.Cache)` | `<base>` |

```go
func resolveValue(value, base, rootDir string) string {
	if value == "" {
		return base
	}
	if filepath.IsAbs(value) {
		return value
	}
	if rootDir != "" {
		return filepath.Join(rootDir, value)
	}
	return filepath.Join(base, value)
}
```

`rootDir` is `""` in global mode and the project directory under `--dir` or a
local override (see §3.3).

### 3.2 Config discovery

`config.NewConfigManager` uses `paths.ConfigFile()`:

- Read path: `xdg.SearchConfigFile("localrpg/config.yaml")` (searches
  `XDG_CONFIG_HOME` then `XDG_CONFIG_DIRS`). If none exists, the write path is
  `xdg.ConfigFile("localrpg/config.yaml")`.
- `LOCALRPG_CONFIG_DIR` remains an explicit override that replaces the search
  base, so tests and unusual installs stay possible.
- The local override stays `./localrpg.yaml` (cwd), unchanged.

`config.NewConfigManagerWithPaths` keeps its signature for tests.

### 3.3 Project mode vs global mode

- **Global mode** (no `--dir`, no `./localrpg.yaml`): config from the XDG search
  path; relative `paths.*` resolve against the XDG category bases; defaults are
  the table above.
- **Project mode** (`--dir <root>`, or a `./localrpg.yaml` is active): config is
  `<root>/config.yaml` (or the local override); `rootDir = <root>` so relative
  `paths.*` resolve against the project, matching today's behaviour. This is the
  deliberate exception that keeps the development workflow intact.

`gui.NewService(rootDir)` classifies `""` and `"."` as global mode and anything
else as project mode. `cmd/localrpg/*` and `pkg/export` resolve the same way,
using the cwd's `./localrpg.yaml` as the project flag when present.

### 3.4 Wiring

- `pkg/gui/service.go`: replace the hand-rolled config discovery and the
  `IsAbs`/`Join` block with `paths.ConfigFile()` and `paths.Resolve(...)`;
  `SetPaths` and the Paths settings tab keep working against the resolved
  absolute values (`pkg/gui/service.go:95-121,2711-2730`).
- `cmd/localrpg/play.go`: `core.NewCustomPathResolver(dirs.Systems, dirs.Worlds,
  dirs.Games, dirs.Cache)`.
- `cmd/localrpg/media.go`: use the resolved cache dir rather than
  `cfg.Paths.Cache`.
- `pkg/export/script.go`: resolve with `paths.Resolve` for the export root.
- `config.DefaultConfig`: set `Paths` to empty strings, so resolution is the
  single source of defaults and `DefaultConfig` stays free of I/O.
- `go.mod`: move `github.com/adrg/xdg` from the `// indirect` block to a direct
  require (it is already at v0.5.3).

### 3.5 Legacy relative paths (no migration)

Per the decision, no files are moved. Under the global config a legacy
`./systems` now resolves to `<XDG data>/localrpg/systems`, which may be empty.
To make this non-surprising, the first resolution logs a single warning when a
legacy directory exists in the working directory and the resolved XDG directory
is empty:

```
paths.legacy_relative detected ./systems but the resolved data directory is empty; set paths.systems to an absolute path to keep using it
```

This is a log line, not a behaviour change; it does not move or copy anything.
Documented in `AGENTS.md` as a migration note.

### 3.6 Interfaces

```go
// pkg/paths
func System() Bases
func App(base string) string
func Resolve(bases Bases, paths config.PathsConfig, rootDir string) Dirs
func ConfigFile() (read, write string, err error)

// pkg/config (unchanged signatures)
func NewConfigManager() *ConfigManager
func NewConfigManagerWithPaths(userPath, localPath string) *ConfigManager
```

## 4. Data Flow

```
environment (XDG_* / LOCALRPG_CONFIG_DIR)
  → xdg bases (System)
  → ConfigFile: read path (XDG search) and write path (config home)
  → ConfigManager.Load: defaults + user config + local override
  → paths.Resolve(bases, cfg.Paths, rootDir)
  → core.PathResolver (absolute dirs) used by gui, play, media, export
```

## 5. Error Handling

- `xdg` never fails; unset variables fall back to native locations. No error
  path from `System`.
- `ConfigFile` returns the write path when no file exists; `Load` already
  tolerates a missing file and falls back to `DefaultConfig`.
- A path that cannot be created surfaces at first use (the existing
  `os.MkdirAll` error), unchanged.
- Resolution is pure; it creates no directories, so a bad config never writes
  anything on load.

## 6. Testing & Verification

Go (stdlib `testing`, `t.TempDir()`):

- `pkg/paths`: `Resolve` with `Bases{t.TempDir()}`: empty values map to the
  category defaults; absolute values pass through; relative values join the base
  in global mode and the root in project mode; `systems` and `cache` land under
  different bases.
- `pkg/paths`: `ConfigFile` honours `XDG_CONFIG_HOME` and `XDG_CONFIG_DIRS`
  (set via `t.Setenv` plus `xdg.Reload()`), and returns a write path when no file
  exists.
- `pkg/config`: `DefaultConfig().Paths` is empty; a config round-trip preserves
  absolute paths and does not invent any.
- `pkg/gui`: `NewService(".")` resolves to the XDG bases; `NewService(tmpDir)`
  resolves relative paths under `tmpDir`; a `./localrpg.yaml` in a temp cwd keeps
  project mode.
- Entry-point smoke: `localrpg export` and `play` build a resolver with the same
  directories for the same config (unit-level on the resolution helper).

Frontend: unchanged (`tsc` only).

## 7. Compatibility & Rollout

- Project-mode behaviour is byte-for-byte what it is today.
- The global-config meaning of a relative path changes; no files move, and a
  warning points at the legacy directory.
- `DefaultConfig` losing the `./…` values is a visible change in the Settings →
  Paths tab: it shows the resolved absolute directories instead of `./systems`.
- `LOCALRPG_CONFIG_DIR` keeps working; `XDG_CONFIG_DIRS` starts working.

## 8. Open Questions

- Should the warning be a UI banner rather than a log line for GUI users?
- Should `LOCALRPG_DATA_DIR`/`LOCALRPG_CACHE_DIR` exist alongside
  `LOCALRPG_CONFIG_DIR`, or is `XDG_*` enough?
- Should the Settings → Paths tab offer a "reset to defaults" that clears the
  values so resolution supplies them again?

## 9. References

- `pkg/config/manager.go:20-31`; `pkg/config/types.go:8-13,272-277`;
  `pkg/core/types.go:78-115`; `pkg/gui/service.go:77-121,2711-2730`;
  `cmd/localrpg/play.go:48`; `cmd/localrpg/media.go:59`;
  `pkg/export/script.go:91`; `go.mod:43` (`github.com/adrg/xdg v0.5.3`).
- adrg/xdg — https://github.com/adrg/xdg
- XDG Base Directory Specification — https://specifications.freedesktop.org/basedir-spec/latest/
