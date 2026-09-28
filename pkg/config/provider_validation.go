package config

import (
	"fmt"
	"strings"
)

// providerShape describes the accepted type and builtin values for one provider
// family. An empty type and "disabled" are always accepted.
type providerShape struct {
	types    map[string]bool
	builtins map[string]bool // nil means builtin names are not validated
}

var (
	llmShape = providerShape{
		types: map[string]bool{
			"": true, "disabled": true, "inherit": true,
			"http": true, "cli": true, "gemini": true, "builtin": true,
		},
		builtins: map[string]bool{"echo": true, "gemini": true, "narrative-oracle": true},
	}
	ttsShape = providerShape{
		types: map[string]bool{
			"": true, "disabled": true, "http": true, "cli": true,
			"gemini": true, "builtin": true,
		},
	}
	sttShape = providerShape{
		types: map[string]bool{
			"": true, "disabled": true, "http": true, "cli": true,
			"builtin": true, "web-speech": true,
		},
	}
	imageShape = providerShape{
		types: map[string]bool{
			"": true, "disabled": true, "http": true, "cli": true,
			"gemini": true, "builtin": true, "comfyui": true,
		},
	}
)

// providerProblems reports a malformed provider shape, one problem per line.
func providerProblems(path, typ, builtin, command, endpoint string, shape providerShape) []string {
	if !shape.types[typ] {
		return []string{fmt.Sprintf("%s: unknown type %q", path, typ)}
	}

	var problems []string
	switch typ {
	case "http", "comfyui":
		if strings.TrimSpace(endpoint) == "" {
			problems = append(problems, path+": endpoint is required for type "+typ)
		}
	case "cli":
		if strings.TrimSpace(command) == "" {
			problems = append(problems, path+": command is required for type cli")
		}
	case "builtin":
		if shape.builtins != nil && builtin != "" && !shape.builtins[builtin] {
			problems = append(problems, fmt.Sprintf("%s: unknown builtin %q", path, builtin))
		}
	}
	return problems
}
