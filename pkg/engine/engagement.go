package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
)

// ResolveEngagement returns the mechanics policy in force: the campaign
// setting, else the system default, else the configured default.
func ResolveEngagement(manifest *core.GameManifest, system *core.SystemManifest, cfg *config.Config) string {
	if manifest != nil && manifest.Settings != nil {
		if value, ok := manifest.Settings["mechanics_engagement"].(string); ok {
			if normalized := normalizeEngagement(value); normalized != "" {
				return normalized
			}
		}
	}
	if system != nil && system.Mechanics != nil {
		if normalized := normalizeEngagement(system.Mechanics.Engagement); normalized != "" {
			return normalized
		}
	}
	if cfg != nil {
		return cfg.MechanicsEngagement()
	}
	return "auto"
}

func normalizeEngagement(value string) string {
	switch mode := strings.ToLower(strings.TrimSpace(value)); mode {
	case "off", "auto", "ask":
		return mode
	default:
		return ""
	}
}
