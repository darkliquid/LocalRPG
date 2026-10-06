package config

import "sort"

// ReservedProviderName names the default entry, which is the singleton field.
const ReservedProviderName = "default"

// TTSFor returns the named TTS configuration, or the default when name is empty,
// "default", or unknown.
func (m MediaConfig) TTSFor(name string) TTSConfig {
	if name == "" || name == ReservedProviderName {
		return m.TTS
	}
	if cfg, ok := m.TTSProviders[name]; ok {
		return cfg
	}
	return m.TTS
}

// STTFor mirrors TTSFor for speech recognition.
func (m MediaConfig) STTFor(name string) STTConfig {
	if name == "" || name == ReservedProviderName {
		return m.STT
	}
	if cfg, ok := m.STTProviders[name]; ok {
		return cfg
	}
	return m.STT
}

// ImageFor mirrors TTSFor for image generation.
func (m MediaConfig) ImageFor(name string) ImageConfig {
	if name == "" || name == ReservedProviderName {
		return m.Image
	}
	if cfg, ok := m.ImageProviders[name]; ok {
		return cfg
	}
	return m.Image
}

// TTSNames returns every TTS name, default first.
func (m MediaConfig) TTSNames() []string { return providerNames(m.TTSProviders) }

// STTNames and ImageNames mirror TTSNames.
func (m MediaConfig) STTNames() []string   { return providerNames(m.STTProviders) }
func (m MediaConfig) ImageNames() []string { return providerNames(m.ImageProviders) }

// providerNames lists the reserved default first, then the named entries sorted.
func providerNames[V interface{}](providers map[string]V) []string {
	out := []string{ReservedProviderName}
	rest := make([]string, 0, len(providers))
	for name := range providers {
		rest = append(rest, name)
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// ProviderForPurpose returns the provider name a purpose resolves to: the
// configured name, or the reserved default when unset.
func (m MediaConfig) ProviderForPurpose(p Purpose) string {
	if name := m.Purposes[string(p)]; name != "" {
		return name
	}
	return ReservedProviderName
}

// TTSForPurpose resolves a TTS purpose to a concrete configuration.
func (m MediaConfig) TTSForPurpose(p Purpose) TTSConfig {
	return m.TTSFor(m.ProviderForPurpose(p))
}

// ImageForPurpose resolves an image purpose to a concrete configuration.
func (m MediaConfig) ImageForPurpose(p Purpose) ImageConfig {
	return m.ImageFor(m.ProviderForPurpose(p))
}
