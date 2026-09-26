// Package paths resolves LocalRPG's storage locations from the XDG base
// directories, so a fresh install does not scatter content around the working
// directory.
package paths

import (
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
