package config

import (
	"fmt"
	"strings"
)

// ApplyOfflinePreset configures the offline stack: narrative-oracle for GM,
// procedural-art for images, builtin-local for embeddings, and native-os (or
// sherpa-onnx) for TTS. It leaves paths, preferences, and named provider entries
// untouched, returning a list of human-readable changes made.
func ApplyOfflinePreset(cfg *Config, tts string) []string {
	var changes []string

	if strings.TrimSpace(tts) == "" {
		tts = "native-os"
	}

	// 1. GM role
	if cfg.Agents.Roles == nil {
		cfg.Agents.Roles = make(map[string]AgentRoleConfig)
	}
	gm := cfg.Agents.Roles["gm"]
	if gm.Type != "builtin" || gm.BuiltinName != "narrative-oracle" {
		cfg.Agents.Roles["gm"] = AgentRoleConfig{
			Type:        "builtin",
			BuiltinName: "narrative-oracle",
			Temperature: 0.7,
			MaxTokens:   1024,
		}
		changes = append(changes, "GM: set to narrative-oracle (builtin)")
	}

	// 2. Auxiliary roles
	narrator := cfg.Agents.Roles["narrator"]
	if narrator.Type != "disabled" {
		cfg.Agents.Roles["narrator"] = AgentRoleConfig{Type: "disabled"}
		changes = append(changes, "Narrator: disabled")
	}

	extractor := cfg.Agents.Roles[RoleExtractor]
	if extractor.Type != "inherit" && extractor.Type != "builtin" {
		cfg.Agents.Roles[RoleExtractor] = AgentRoleConfig{Type: "inherit", InheritFrom: RoleGM}
		changes = append(changes, "Extractor: inherit from GM")
	}

	completion := cfg.Agents.Roles[RoleCompletion]
	if completion.Type != "inherit" && completion.Type != "builtin" {
		cfg.Agents.Roles[RoleCompletion] = AgentRoleConfig{Type: "inherit", InheritFrom: RoleGM}
		changes = append(changes, "Completion: inherit from GM")
	}

	// 3. TTS
	if cfg.Media.TTS.Type != "builtin" || cfg.Media.TTS.BuiltinName != tts {
		cfg.Media.TTS.Type = "builtin"
		cfg.Media.TTS.BuiltinName = tts
		cfg.Media.TTS.Command = ""
		cfg.Media.TTS.Endpoint = ""
		cfg.Media.TTS.APIKey = ""
		changes = append(changes, fmt.Sprintf("TTS: set to %s (builtin)", tts))
	}

	// 4. Image
	if cfg.Media.Image.Type != "builtin" || cfg.Media.Image.BuiltinName != "procedural-art" {
		cfg.Media.Image.Type = "builtin"
		cfg.Media.Image.BuiltinName = "procedural-art"
		cfg.Media.Image.Command = ""
		cfg.Media.Image.Endpoint = ""
		changes = append(changes, "Image: set to procedural-art (builtin)")
	}

	// 5. STT
	if cfg.Media.STT.Type != "disabled" {
		cfg.Media.STT.Type = "disabled"
		cfg.Media.STT.Command = ""
		cfg.Media.STT.Endpoint = ""
		changes = append(changes, "STT: disabled")
	}

	// 6. Embeddings
	if !cfg.Embeddings.Enabled || cfg.Embeddings.Provider != "builtin-local" {
		cfg.Embeddings.Enabled = true
		cfg.Embeddings.Provider = "builtin-local"
		cfg.Embeddings.Model = "hash-projection"
		if cfg.Embeddings.Providers == nil {
			cfg.Embeddings.Providers = make(map[string]EmbeddingProviderConfig)
		}
		cfg.Embeddings.Providers["builtin-local"] = EmbeddingProviderConfig{
			Type:        "builtin",
			BuiltinName: "hash-projection",
		}
		changes = append(changes, "Embeddings: set to builtin-local (hash-projection)")
	}

	return changes
}
