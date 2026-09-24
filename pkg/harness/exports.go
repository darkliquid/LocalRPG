package harness

// ProviderIDFor maps a role configuration to the registry ID a facade should
// build. An empty result means the configuration has no registry provider (a
// disabled role, or the debug echo fallback) and the caller handles it.
func ProviderIDFor(cfg ProviderConfig) string {
	switch cfg.Type {
	case "http":
		return "openaichat"
	case "cli":
		return "cli"
	case "gemini":
		return "gemini"
	case "builtin", "mock", "":
		switch cfg.BuiltinName {
		case "gemini":
			return "gemini"
		case "narrative-oracle":
			return "narrative-oracle"
		}
		if cfg.Command != "" {
			return "cli"
		}
		return ""
	default:
		return ""
	}
}
