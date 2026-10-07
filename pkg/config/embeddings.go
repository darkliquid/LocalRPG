package config

import "sort"

// ProviderNames lists every named embedding provider, sorted. Embeddings have no
// default singleton: the top-level Provider selector names which entry is
// active, so the default is not a name in this list.
func (e EmbeddingsConfig) ProviderNames() []string {
	names := make([]string, 0, len(e.Providers))
	for name := range e.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ProviderFor returns the named embedding provider. An empty name, or the
// reserved "default", resolves to the entry the top-level Provider selector
// names. An unknown name, or no configured providers, falls back to the built-in
// projection so a caller always gets a usable entry.
func (e EmbeddingsConfig) ProviderFor(name string) EmbeddingProviderConfig {
	if name == "" || name == ReservedProviderName {
		name = e.Provider
	}
	if cfg, ok := e.Providers[name]; ok {
		return cfg
	}
	return EmbeddingProviderConfig{Type: "builtin", Model: e.Model}
}

// SelectedProvider names the active embedding provider, or the reserved
// "default" when none is chosen.
func (e EmbeddingsConfig) SelectedProvider() string {
	if e.Provider == "" {
		return ReservedProviderName
	}
	return e.Provider
}
