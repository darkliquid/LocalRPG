package rules

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
)

type RuleLoader struct {
	paths    *core.PathResolver
	jsEngine *JSEngine
}

func NewRuleLoader(paths *core.PathResolver, jsEngine *JSEngine) *RuleLoader {
	return &RuleLoader{
		paths:    paths,
		jsEngine: jsEngine,
	}
}

func (r *RuleLoader) LoadRules(systemID, worldID string) error {
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
