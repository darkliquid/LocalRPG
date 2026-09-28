package harness

import "github.com/darkliquid/localrpg/pkg/provider"

// KeyFor maps a role configuration to the canonical provider key a facade
// should build. ok is false when the configuration has no registry provider (a
// disabled role, or the debug echo fallback), and the caller must not invent a
// key of its own.
func KeyFor(cfg ProviderConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "http":
		return provider.InstanceOrSelf(provider.KeyLLMOpenAIChat, provider.HostDiscriminator(cfg.Endpoint)), true
	case "cli":
		return provider.InstanceOrSelf(provider.KeyLLMCLI, provider.CommandDiscriminator(cfg.Command)), true
	case "gemini":
		return provider.KeyLLMGemini, true
	case "builtin", "":
		switch cfg.BuiltinName {
		case "gemini":
			return provider.KeyLLMGemini, true
		case "narrative-oracle":
			return provider.KeyLLMNarrativeOracle, true
		}
		if cfg.Command != "" {
			return provider.InstanceOrSelf(provider.KeyLLMCLI, provider.CommandDiscriminator(cfg.Command)), true
		}
		return "", false
	default:
		return "", false
	}
}
