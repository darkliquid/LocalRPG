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

var KokoroVoiceProfiles = []VoiceProfile{
	{ID: "af_alloy", Name: "Alloy (American Female)", VoiceID: "af_alloy", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "alloy", "clear", "neutral"}, Description: "American female voice, neutral, balanced, and articulate."},
	{ID: "af_aoede", Name: "Aoede (American Female)", VoiceID: "af_aoede", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "aoede", "melodic", "expressive"}, Description: "American female voice, musical, dramatic, and expressive."},
	{ID: "af_bella", Name: "Bella (American Female)", VoiceID: "af_bella", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "bella", "warm", "friendly"}, Description: "American female voice, warm, approachable, and pleasant."},
	{ID: "af_heart", Name: "Heart (American Female)", VoiceID: "af_heart", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "heart", "calm", "gentle"}, Description: "American female voice, soft-spoken, comforting, and calm."},
	{ID: "af_jessica", Name: "Jessica (American Female)", VoiceID: "af_jessica", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "jessica", "bright", "conversational"}, Description: "American female voice, energetic, clear, and conversational."},
	{ID: "af_kore", Name: "Kore (American Female)", VoiceID: "af_kore", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "kore", "mystical", "soft"}, Description: "American female voice, ethereal, gentle, and quiet."},
	{ID: "af_nicole", Name: "Nicole (American Female)", VoiceID: "af_nicole", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "nicole", "youthful", "energetic"}, Description: "American female voice, brisk, youthful, and direct."},
	{ID: "af_nova", Name: "Nova (American Female)", VoiceID: "af_nova", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "nova", "dynamic", "sharp"}, Description: "American female voice, focused, sharp, and confident."},
	{ID: "af_river", Name: "River (American Female)", VoiceID: "af_river", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "river", "smooth", "casual"}, Description: "American female voice, smooth, relaxed, and natural."},
	{ID: "af_sarah", Name: "Sarah (American Female)", VoiceID: "af_sarah", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "sarah", "poised", "narrative"}, Description: "American female voice, polished, measured, and story-oriented."},
	{ID: "af_sky", Name: "Sky (American Female)", VoiceID: "af_sky", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "sky", "light", "airy"}, Description: "American female voice, light, gentle, and breathy."},
	{ID: "am_adam", Name: "Adam (American Male)", VoiceID: "am_adam", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "adam", "deep", "authoritative"}, Description: "American male voice, deep, steady, and commanding."},
	{ID: "am_echo", Name: "Echo (American Male)", VoiceID: "am_echo", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "echo", "resonant", "neutral"}, Description: "American male voice, resonant, clear, and balanced."},
	{ID: "am_eric", Name: "Eric (American Male)", VoiceID: "am_eric", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "eric", "grounded", "steady"}, Description: "American male voice, solid, plainspoken, and trustworthy."},
	{ID: "am_fenrir", Name: "Fenrir (American Male)", VoiceID: "am_fenrir", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "fenrir", "fierce", "husky"}, Description: "American male voice, rough, intense, and gravelly."},
	{ID: "am_liam", Name: "Liam (American Male)", VoiceID: "am_liam", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "liam", "warm", "relatable"}, Description: "American male voice, youthful, warm, and friendly."},
	{ID: "am_michael", Name: "Michael (American Male)", VoiceID: "am_michael", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "michael", "commanding", "formal"}, Description: "American male voice, disciplined, authoritative, and formal."},
	{ID: "am_onyx", Name: "Onyx (American Male)", VoiceID: "am_onyx", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "onyx", "dark", "gravelly"}, Description: "American male voice, deep, shadowy, and solemn."},
	{ID: "am_puck", Name: "Puck (American Male)", VoiceID: "am_puck", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "puck", "playful", "mischievous"}, Description: "American male voice, spirited, upbeat, and sly."},
	{ID: "bf_alice", Name: "Alice (British Female)", VoiceID: "bf_alice", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "alice", "articulate", "refined"}, Description: "British female voice, cultured, articulate, and poised."},
	{ID: "bf_emma", Name: "Emma (British Female)", VoiceID: "bf_emma", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "emma", "gentle", "poised"}, Description: "British female voice, elegant, gentle, and softly spoken."},
	{ID: "bf_isabella", Name: "Isabella (British Female)", VoiceID: "bf_isabella", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "isabella", "noble", "melodic"}, Description: "British female voice, aristocratic, melodic, and graceful."},
	{ID: "bf_lily", Name: "Lily (British Female)", VoiceID: "bf_lily", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "lily", "sweet", "youthful"}, Description: "British female voice, sweet, youthful, and crisp."},
	{ID: "bm_daniel", Name: "Daniel (British Male)", VoiceID: "bm_daniel", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "daniel", "scholarly", "calm"}, Description: "British male voice, scholarly, calm, and deliberate."},
	{ID: "bm_fable", Name: "Fable (British Male)", VoiceID: "bm_fable", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "fable", "dramatic", "storyteller"}, Description: "British male voice, theatrical, expressive, and storied."},
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
