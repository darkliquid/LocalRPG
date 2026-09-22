package config

var AgentPresets = map[string]AgentRoleConfig{
	"ollama": {
		Type:        "http",
		Endpoint:    "http://localhost:11434/v1",
		Model:       "llama3.2",
		Temperature: 0.7,
		MaxTokens:   1024,
	},
	"lm-studio": {
		Type:        "http",
		Endpoint:    "http://localhost:1234/v1",
		Model:       "default",
		Temperature: 0.7,
		MaxTokens:   1024,
	},
	"localai": {
		Type:        "http",
		Endpoint:    "http://localhost:8080/v1",
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   1024,
	},
	"vllm": {
		Type:        "http",
		Endpoint:    "http://localhost:8000/v1",
		Model:       "default",
		Temperature: 0.7,
		MaxTokens:   1024,
	},
	"llama-cli": {
		Type:        "cli",
		Command:     "llama-cli",
		Args:        []string{"-m", "models/model.gguf", "-p"},
		Temperature: 0.7,
		MaxTokens:   1024,
	},
	"claude-cli": {
		Type:    "cli",
		Command: "claude",
		Args:    []string{"-p"},
	},
	"narrative-oracle": {
		Type:        "builtin",
		BuiltinName: "narrative-oracle",
	},
}

var TTSPresets = map[string]TTSConfig{
	"kokoro-fastapi": {
		Type:         "http",
		Endpoint:     "http://localhost:8880/v1/audio/speech",
		Model:        "kokoro",
		DefaultVoice: "af_bella",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"alltalk": {
		Type:         "http",
		Endpoint:     "http://localhost:7851/api/tts-generate",
		DefaultVoice: "default",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"piper": {
		Type:         "cli",
		Command:      "piper",
		Args:         []string{"--model", "en_US-lessac-medium.onnx", "--output_file", "-"},
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"native-os": {
		Type:         "builtin",
		BuiltinName:  "native-os",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"sherpa-onnx": {
		Type:         "builtin",
		BuiltinName:  "sherpa-onnx",
		DefaultVoice: "af_bella",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"openai-speech": {
		Type:         "http",
		Endpoint:     "https://api.openai.com/v1/audio/speech",
		Model:        "tts-1",
		DefaultVoice: "alloy",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
}

var STTPresets = map[string]STTConfig{
	"faster-whisper": {
		Type:     "http",
		Endpoint: "http://localhost:8000/v1/audio/transcriptions",
		Model:    "whisper-1",
	},
	"whisper-cli": {
		Type:    "cli",
		Command: "whisper-cli",
		Args:    []string{"-m", "models/ggml-base.bin", "-f", "%INPUT%", "-nt"},
	},
	"openai-whisper": {
		Type:     "http",
		Endpoint: "https://api.openai.com/v1/audio/transcriptions",
		Model:    "whisper-1",
	},
}

var ImagePresets = map[string]ImageConfig{
	"comfyui": {
		Type:     "http",
		Endpoint: "http://127.0.0.1:8188",
	},
	"automatic1111": {
		Type:     "http",
		Endpoint: "http://127.0.0.1:7860/sdapi/v1/txt2img",
	},
	"localai-image": {
		Type:     "http",
		Endpoint: "http://127.0.0.1:8080/v1/images/generations",
		Model:    "stablediffusion",
	},
	"sd-cli": {
		Type:    "cli",
		Command: "sd",
		Args:    []string{"-m", "models/sd-v1-5.gguf", "-p"},
	},
	"procedural-art": {
		Type:        "builtin",
		BuiltinName: "procedural-art",
	},
	"dall-e-3": {
		Type:     "http",
		Endpoint: "https://api.openai.com/v1/images/generations",
		Model:    "dall-e-3",
	},
}

func GetAgentPreset(id string) (AgentRoleConfig, bool) {
	p, ok := AgentPresets[id]
	return p, ok
}

func GetTTSPreset(id string) (TTSConfig, bool) {
	p, ok := TTSPresets[id]
	return p, ok
}

func GetSTTPreset(id string) (STTConfig, bool) {
	p, ok := STTPresets[id]
	return p, ok
}

func GetImagePreset(id string) (ImageConfig, bool) {
	p, ok := ImagePresets[id]
	return p, ok
}
