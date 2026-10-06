package harness

import "github.com/darkliquid/localrpg/pkg/provider"

// KeyFor maps a role configuration to the canonical provider key a facade
// should build. ok is false when the configuration has no registry provider (a
// disabled role, or the debug echo fallback), and the caller must not invent a
// key of its own.
func KeyFor(cfg ProviderConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "http":
		return provider.InstanceOrSelf(provider.KeyLLMOpenAIChat, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint))), true
	case "cli":
		return provider.InstanceOrSelf(provider.KeyLLMCLI, provider.InstanceDiscriminator(cfg.Instance, provider.CommandDiscriminator(cfg.Command))), true
	case "gemini":
		return provider.InstanceOrSelf(provider.KeyLLMGemini, provider.InstanceDiscriminator(cfg.Instance, "")), true
	case "inworld":
		return provider.InstanceOrSelf(provider.KeyLLMInworld, provider.InstanceDiscriminator(cfg.Instance, "")), true
	case "builtin", "":
		switch cfg.BuiltinName {
		case "gemini":
			return provider.InstanceOrSelf(provider.KeyLLMGemini, provider.InstanceDiscriminator(cfg.Instance, "")), true
		case "inworld":
			return provider.InstanceOrSelf(provider.KeyLLMInworld, provider.InstanceDiscriminator(cfg.Instance, "")), true
		case "narrative-oracle":
			return provider.InstanceOrSelf(provider.KeyLLMNarrativeOracle, provider.InstanceDiscriminator(cfg.Instance, "")), true
		}
		if cfg.Command != "" {
			return provider.InstanceOrSelf(provider.KeyLLMCLI, provider.InstanceDiscriminator(cfg.Instance, provider.CommandDiscriminator(cfg.Command))), true
		}
		return "", false
	default:
		return "", false
	}
}
