package media

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
)

// ProviderVoice is one voice a provider offers. It is the shared shape every
// catalog maps into, so the picker and the matcher never learn a provider's own
// vocabulary.
type ProviderVoice struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Language    string                 `json:"language,omitempty"`
	Gender      string                 `json:"gender,omitempty"`
	Accent      string                 `json:"accent,omitempty"`
	Categories  []string               `json:"categories,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Description string                 `json:"description,omitempty"`
	PreviewURL  string                 `json:"preview_url,omitempty"`
	Defaults    map[string]interface{} `json:"defaults,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// VoiceCatalog is implemented by providers that can enumerate their voices. A
// provider that cannot simply does not implement it, and the UI falls back to
// authored profiles with no error.
type VoiceCatalog interface {
	ListVoices(ctx context.Context) ([]ProviderVoice, error)
}

// VoiceOption declares one tunable a provider accepts, so the UI renders controls
// from the declaration rather than from provider-specific code.
type VoiceOption struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"` // "float" | "int" | "bool" | "string" | "enum"
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	Step    float64  `json:"step,omitempty"`
	Options []string `json:"options,omitempty"`
	Default any      `json:"default,omitempty"`
	Help    string   `json:"help,omitempty"`
}

// VoiceOptions is implemented by providers that declare the tunables they accept.
// Pitch and speech rate are the portable baseline and never declared here.
type VoiceOptions interface {
	VoiceOptions() []VoiceOption
}

// MeteredProvider is implemented by providers that charge per request.
type MeteredProvider interface {
	Metered() bool
}

// ProviderKey derives a stable identifier from a TTS configuration, used for
// catalog filenames, API parameters, and diagnostics:
// "builtin:elevenlabs", "builtin:sherpa-onnx", "http:localhost:8880", "cli:piper".
func ProviderKey(cfg config.TTSConfig) string {
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "", "disabled":
		return "disabled"
	case "builtin":
		name := strings.ToLower(strings.TrimSpace(cfg.BuiltinName))
		if name == "" {
			name = "echo"
		}
		return "builtin:" + sanitiseKey(name)
	case "cli":
		command := strings.ToLower(strings.TrimSpace(cfg.Command))
		if command == "" {
			return "cli"
		}
		return "cli:" + sanitiseKey(filepath.Base(command))
	case "http":
		return "http:" + sanitiseKey(endpointHost(cfg.Endpoint))
	default:
		return sanitiseKey(strings.ToLower(strings.TrimSpace(cfg.Type)))
	}
}

// endpointHost is the host and port of an endpoint, or the raw value when it does
// not parse, so an odd URL still names a distinct provider.
func endpointHost(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return "endpoint"
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return strings.TrimPrefix(strings.TrimPrefix(trimmed, "http://"), "https://")
	}
	return parsed.Host
}

// sanitiseKey lowercases and reduces a fragment to characters that are safe in an
// identifier. The colon that separates a namespace is preserved.
func sanitiseKey(value string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-', r == ':':
			sb.WriteRune(r)
		default:
			sb.WriteRune('-')
		}
	}
	return strings.Trim(sb.String(), "-")
}
