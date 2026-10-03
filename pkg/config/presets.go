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
	"inworld-frontier": {
		Type:        "builtin",
		BuiltinName: "inworld",
		Model:       "inworld/compare-frontier-models",
		Temperature: 0.7,
		MaxTokens:   2048,
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
	"gemini-2.5-flash": {
		Type:           "builtin",
		BuiltinName:    "gemini",
		Model:          "gemini-2.5-flash",
		Temperature:    0.7,
		MaxTokens:      2048,
		ThinkingBudget: intPtr(0),
		TopP:           floatPtr(0.95),
		TopK:           intPtr(40),
	},
	"gemini-2.5-pro": {
		Type:           "builtin",
		BuiltinName:    "gemini",
		Model:          "gemini-2.5-pro",
		Temperature:    0.7,
		MaxTokens:      4096,
		ThinkingBudget: intPtr(-1),
		TopP:           floatPtr(0.95),
		TopK:           intPtr(40),
	},
	"gemini-2.0-flash": {
		Type:        "builtin",
		BuiltinName: "gemini",
		Model:       "gemini-2.0-flash",
		Temperature: 0.7,
		MaxTokens:   2048,
		TopP:        floatPtr(0.95),
		TopK:        intPtr(40),
	},
	"gemini-2.0-flash-lite": {
		Type:        "builtin",
		BuiltinName: "gemini",
		Model:       "gemini-2.0-flash-lite",
		Temperature: 0.7,
		MaxTokens:   2048,
		TopP:        floatPtr(0.95),
		TopK:        intPtr(40),
	},
	"gemini": {
		Type:           "gemini",
		Model:          "gemini-3.8-flash",
		Temperature:    0.7,
		MaxTokens:      4096,
		ThinkingBudget: intPtr(0),
		TopP:           floatPtr(0.95),
		TopK:           intPtr(40),
	},
}

func intPtr(i int) *int { return &i }
func floatPtr(f float64) *float64 { return &f }

var TTSPresets = map[string]TTSConfig{
	"kokoro-fastapi": {
		Type:          "http",
		Endpoint:      "http://localhost:8880",
		Model:         "kokoro",
		DefaultVoice:  "af_bella",
		Pitch:         1.0,
		SpeechRate:    1.0,
		MasterVolume:  1.0,
		VoiceProfiles: KokoroVoiceProfiles,
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
		Type:          "builtin",
		BuiltinName:   "sherpa-onnx",
		DefaultVoice:  "af_bella",
		Pitch:         1.0,
		SpeechRate:    1.0,
		MasterVolume:  1.0,
		VoiceProfiles: KokoroVoiceProfiles,
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

	"elevenlabs": {
		Type:         "builtin",
		BuiltinName:  "elevenlabs",
		Model:        "eleven_multilingual_v2",
		DefaultVoice: "EXAVITQu4vr4xnSDxMaL", // "Sarah", a premade stock voice
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},

	"gemini-3.8-flash-tts": {
		Type:         "gemini",
		Model:        "gemini-3.8-flash-tts",
		DefaultVoice: "Aoede",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"gemini-3.8-flash-lite-tts": {
		Type:         "gemini",
		Model:        "gemini-3.8-flash-lite-tts",
		DefaultVoice: "Aoede",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},

	"gemini-3.1-flash-tts": {
		Type:         "gemini",
		Model:        "gemini-3.1-flash-tts-preview",
		DefaultVoice: "Aoede",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"gemini-2.5-flash-tts": {
		Type:         "gemini",
		Model:        "gemini-2.5-flash-preview-tts",
		DefaultVoice: "Aoede",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"gemini-2.5-pro-tts": {
		Type:         "gemini",
		Model:        "gemini-2.5-pro-preview-tts",
		DefaultVoice: "Aoede",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
}

var STTPresets = map[string]STTConfig{
	"web-speech": {
		Type: "web-speech",
	},
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
	"imagen-3": {
		Type:             "gemini",
		Model:            "imagen-3.0-generate-002",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
	"imagen-3-fast": {
		Type:             "gemini",
		Model:            "imagen-3.0-fast-generate-001",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
	"nano-banana-2": {
		Type:             "gemini",
		Model:            "gemini-3.1-flash-image",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
	"nano-banana-2-lite": {
		Type:             "gemini",
		Model:            "gemini-3.1-flash-lite-image",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
	"nano-banana-pro": {
		Type:             "gemini",
		Model:            "gemini-3-pro-image",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
	"nano-banana": {
		Type:             "gemini",
		Model:            "gemini-2.5-flash-image",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
}


var KokoroVoiceProfiles = []VoiceProfile{
	{ID: "af", Name: "Default (American Female)", VoiceID: "af", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "default", "neutral"}, Description: "The model's stock American female voice."},
	{ID: "af_bella", Name: "Bella (American Female)", VoiceID: "af_bella", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "bella", "warm", "friendly"}, Description: "American female voice, warm, approachable, and pleasant."},
	{ID: "af_nicole", Name: "Nicole (American Female)", VoiceID: "af_nicole", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "nicole", "youthful", "energetic"}, Description: "American female voice, brisk, youthful, and direct."},
	{ID: "af_sarah", Name: "Sarah (American Female)", VoiceID: "af_sarah", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "sarah", "poised", "narrative"}, Description: "American female voice, polished, measured, and story-oriented."},
	{ID: "af_sky", Name: "Sky (American Female)", VoiceID: "af_sky", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "sky", "light", "airy"}, Description: "American female voice, light, gentle, and breathy."},
	{ID: "am_adam", Name: "Adam (American Male)", VoiceID: "am_adam", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "adam", "deep", "authoritative"}, Description: "American male voice, deep, steady, and commanding."},
	{ID: "am_michael", Name: "Michael (American Male)", VoiceID: "am_michael", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "michael", "commanding", "formal"}, Description: "American male voice, disciplined, authoritative, and formal."},
	{ID: "bf_emma", Name: "Emma (British Female)", VoiceID: "bf_emma", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "emma", "gentle", "poised"}, Description: "British female voice, elegant, gentle, and softly spoken."},
	{ID: "bf_isabella", Name: "Isabella (British Female)", VoiceID: "bf_isabella", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "isabella", "noble", "melodic"}, Description: "British female voice, aristocratic, melodic, and graceful."},
	{ID: "bm_george", Name: "George (British Male)", VoiceID: "bm_george", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "george", "mature", "distinguished"}, Description: "British male voice, mature, distinguished, and resonant."},
	{ID: "bm_lewis", Name: "Lewis (British Male)", VoiceID: "bm_lewis", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "lewis", "thoughtful", "refined"}, Description: "British male voice, measured, polite, and reflective."},
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
