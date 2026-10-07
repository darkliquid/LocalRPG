package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/darkliquid/localrpg/pkg/provider"
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

// VerifyOffline inspects each configured role and media provider against the
// provider registry, reporting any that are not declared offline.
func VerifyOffline(cfg *Config) provider.OfflineReport {
	var issues []provider.OfflineIssue

	if cfg == nil {
		return provider.OfflineReport{Offline: true}
	}

	// 1. Roles
	roleNames := make([]string, 0, len(cfg.Agents.Roles))
	for role := range cfg.Agents.Roles {
		roleNames = append(roleNames, role)
	}
	slices.Sort(roleNames)

	for _, role := range roleNames {
		roleCfg := cfg.Agents.Roles[role]
		if roleCfg.Type == "disabled" {
			continue
		}
		if roleCfg.Type == "inherit" {
			inherit := roleCfg.InheritFrom
			if inherit == "" {
				inherit = RoleGM
			}
			roleCfg = cfg.Agents.Roles[inherit]
			if roleCfg.Type == "disabled" || roleCfg.Type == "inherit" {
				continue
			}
		}

		key := roleKey(roleCfg)
		if key == "" {
			continue
		}
		if issue, ok := provider.InspectOffline(role, key); !ok {
			issues = append(issues, issue)
		}
	}

	// 2. TTS
	if cfg.Media.TTS.Type != "disabled" {
		key := ttsKey(cfg.Media.TTS)
		if key != "" {
			if issue, ok := provider.InspectOffline("tts", key); !ok {
				issues = append(issues, issue)
			}
		}
	}

	// 3. Image
	if cfg.Media.Image.Type != "disabled" {
		key := imageKey(cfg.Media.Image)
		if key != "" {
			if issue, ok := provider.InspectOffline("image", key); !ok {
				issues = append(issues, issue)
			}
		}
	}

	// 4. STT
	if cfg.Media.STT.Type != "disabled" {
		key := sttKey(cfg.Media.STT)
		if key != "" {
			if issue, ok := provider.InspectOffline("stt", key); !ok {
				issues = append(issues, issue)
			}
		}
	}

	// 5. Embeddings
	if cfg.Embeddings.Enabled {
		embCfg := cfg.Embeddings.ProviderFor(cfg.Embeddings.Provider)
		key := embeddingKey(embCfg)
		if key != "" {
			if issue, ok := provider.InspectOffline("embeddings", key); !ok {
				issues = append(issues, issue)
			}
		}
	}

	return provider.OfflineReport{
		Offline: len(issues) == 0,
		Issues:  issues,
	}
}

func roleKey(cfg AgentRoleConfig) provider.Key {
	switch cfg.Type {
	case "http":
		return provider.InstanceOrSelf(provider.KeyLLMOpenAIChat, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint)))
	case "cli":
		return provider.InstanceOrSelf(provider.KeyLLMCLI, provider.InstanceDiscriminator(cfg.Instance, provider.CommandDiscriminator(cfg.Command)))
	case "gemini":
		return provider.InstanceOrSelf(provider.KeyLLMGemini, provider.InstanceDiscriminator(cfg.Instance, ""))
	case "inworld":
		return provider.InstanceOrSelf(provider.KeyLLMInworld, provider.InstanceDiscriminator(cfg.Instance, ""))
	case "builtin", "":
		switch cfg.BuiltinName {
		case "gemini":
			return provider.InstanceOrSelf(provider.KeyLLMGemini, provider.InstanceDiscriminator(cfg.Instance, ""))
		case "inworld":
			return provider.InstanceOrSelf(provider.KeyLLMInworld, provider.InstanceDiscriminator(cfg.Instance, ""))
		case "narrative-oracle":
			return provider.InstanceOrSelf(provider.KeyLLMNarrativeOracle, provider.InstanceDiscriminator(cfg.Instance, ""))
		}
		if cfg.Command != "" {
			return provider.InstanceOrSelf(provider.KeyLLMCLI, provider.InstanceDiscriminator(cfg.Instance, provider.CommandDiscriminator(cfg.Command)))
		}
	}
	return ""
}

func ttsKey(cfg TTSConfig) provider.Key {
	switch cfg.Type {
	case "fish-audio":
		return provider.InstanceOrSelf(provider.KeyTTSFishAudio, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint)))
	case "gemini":
		return provider.InstanceOrSelf(provider.KeyTTSGemini, provider.InstanceDiscriminator(cfg.Instance, ""))
	case "inworld":
		return provider.InstanceOrSelf(provider.KeyTTSInworld, provider.InstanceDiscriminator(cfg.Instance, ""))
	case "cartesia":
		return provider.InstanceOrSelf(provider.KeyTTSCartesia, provider.InstanceDiscriminator(cfg.Instance, ""))
	case "builtin":
		switch cfg.BuiltinName {
		case "gemini":
			return provider.InstanceOrSelf(provider.KeyTTSGemini, provider.InstanceDiscriminator(cfg.Instance, ""))
		case "inworld":
			return provider.InstanceOrSelf(provider.KeyTTSInworld, provider.InstanceDiscriminator(cfg.Instance, ""))
		case "sherpa-onnx", "kokoro":
			return provider.InstanceOrSelf(provider.KeyTTSSherpaONNX, provider.InstanceDiscriminator(cfg.Instance, ""))
		case "native-os":
			return provider.InstanceOrSelf(provider.KeyTTSNativeOS, provider.InstanceDiscriminator(cfg.Instance, ""))
		case "elevenlabs":
			return provider.InstanceOrSelf(provider.KeyTTSElevenLabs, provider.InstanceDiscriminator(cfg.Instance, ""))
		case "cartesia":
			return provider.InstanceOrSelf(provider.KeyTTSCartesia, provider.InstanceDiscriminator(cfg.Instance, ""))
		}
	case "cli":
		return provider.InstanceOrSelf(provider.KeyTTSPiper, provider.InstanceDiscriminator(cfg.Instance, provider.CommandDiscriminator(cfg.Command)))
	case "http":
		lowerModel := strings.ToLower(cfg.Model)
		if strings.Contains(lowerModel, "fishaudio") || strings.Contains(lowerModel, "s2-pro") {
			return provider.InstanceOrSelf(provider.KeyTTSFishAudio, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint)))
		}
		return provider.InstanceOrSelf(provider.KeyTTSHTTP, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint)))
	}
	return ""
}

func imageKey(cfg ImageConfig) provider.Key {
	switch cfg.Type {
	case "builtin":
		switch cfg.BuiltinName {
		case "procedural-art":
			return provider.InstanceOrSelf(provider.KeyImageProceduralArt, provider.InstanceDiscriminator(cfg.Instance, ""))
		case "gemini":
			return provider.InstanceOrSelf(provider.KeyImageGemini, provider.InstanceDiscriminator(cfg.Instance, ""))
		}
	case "http", "comfyui", "automatic1111":
		return provider.InstanceOrSelf(provider.KeyImageHTTP, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint)))
	case "cli":
		return provider.InstanceOrSelf(provider.KeyImageCLI, provider.InstanceDiscriminator(cfg.Instance, provider.CommandDiscriminator(cfg.Command)))
	case "gemini":
		return provider.InstanceOrSelf(provider.KeyImageGemini, provider.InstanceDiscriminator(cfg.Instance, ""))
	}
	return ""
}

func sttKey(cfg STTConfig) provider.Key {
	switch cfg.Type {
	case "inworld":
		return provider.InstanceOrSelf(provider.KeySTTInworld, provider.InstanceDiscriminator(cfg.Instance, ""))
	case "cartesia":
		return provider.InstanceOrSelf(provider.KeySTTCartesia, provider.InstanceDiscriminator(cfg.Instance, ""))
	case "web-speech":
		return provider.InstanceOrSelf(provider.KeySTTWebSpeech, provider.InstanceDiscriminator(cfg.Instance, ""))
	case "builtin":
		switch cfg.BuiltinName {
		case "inworld":
			return provider.InstanceOrSelf(provider.KeySTTInworld, provider.InstanceDiscriminator(cfg.Instance, ""))
		case "cartesia":
			return provider.InstanceOrSelf(provider.KeySTTCartesia, provider.InstanceDiscriminator(cfg.Instance, ""))
		}
	case "http":
		return provider.InstanceOrSelf(provider.KeySTTWhisperHTTP, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint)))
	case "cli":
		return provider.InstanceOrSelf(provider.KeySTTWhisperCLI, provider.InstanceDiscriminator(cfg.Instance, provider.CommandDiscriminator(cfg.Command)))
	}
	return ""
}

func embeddingKey(cfg EmbeddingProviderConfig) provider.Key {
	switch cfg.Type {
	case "builtin":
		return provider.InstanceOrSelf(provider.KeyEmbeddingBuiltin, provider.InstanceDiscriminator(cfg.Instance, ""))
	case "onnx":
		return provider.InstanceOrSelf(provider.KeyEmbeddingONNX, provider.InstanceDiscriminator(cfg.Instance, ""))
	case "http":
		return provider.InstanceOrSelf(provider.KeyEmbeddingOpenAI, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint)))
	case "gemini":
		return provider.InstanceOrSelf(provider.KeyEmbeddingGemini, provider.InstanceDiscriminator(cfg.Instance, ""))
	}
	return ""
}

