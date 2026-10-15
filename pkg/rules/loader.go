package rules

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
)

// RuleLoader loads a campaign's scripts off disk: the system's mechanics.js,
// then the world's overrides for that system.
type RuleLoader struct {
	paths    *core.PathResolver
	jsEngine *JSEngine
}

// NewRuleLoader pairs the paths a system and a world live at with the engine the
// scripts run in.
func NewRuleLoader(paths *core.PathResolver, jsEngine *JSEngine) *RuleLoader {
	return &RuleLoader{
		paths:    paths,
		jsEngine: jsEngine,
	}
}

// LoadRules reads a system's manifest into the host bridge, then runs the
// system's mechanics.js and, when the campaign has a world, that world's
// system_overrides/<system>/hooks.js after it, so a world can override what the
// system registered. A script either file does not have is not an error: a system
// may declare its mechanics in YAML alone.
func (r *RuleLoader) LoadRules(systemID, worldID string) error {
	// 0. Hand the declarative mechanics schema to the host bridge, so a script
	// can read the stats, skills, and check conventions it declares.
	manifestPath := filepath.Join(r.paths.SystemDir(systemID), "system.yaml")
	if manifest, err := core.LoadSystemManifest(manifestPath); err == nil && manifest != nil {
		r.jsEngine.SetManifest(manifest)
	}

	// 1. Load base system JS if present
	sysScriptPath := filepath.Join(r.paths.SystemDir(systemID), "mechanics.js")
	if data, err := os.ReadFile(sysScriptPath); err == nil {
		if err := r.jsEngine.LoadScript(string(data)); err != nil {
			return fmt.Errorf("load base system script %q: %w", sysScriptPath, err)
		}
	}

	// 2. Load world overrides JS if present
	if worldID != "" {
		overridePath := filepath.Join(r.paths.WorldDir(worldID), "system_overrides", systemID, "hooks.js")
		if data, err := os.ReadFile(overridePath); err == nil {
			if err := r.jsEngine.LoadScript(string(data)); err != nil {
				return fmt.Errorf("load world override script %q: %w", overridePath, err)
			}
		}
	}

	return nil
}
