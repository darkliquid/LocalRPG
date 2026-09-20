package config

type PathsConfig struct {
	Systems string `yaml:"systems" json:"systems"`
	Worlds  string `yaml:"worlds" json:"worlds"`
	Games   string `yaml:"games" json:"games"`
	Cache   string `yaml:"cache" json:"cache"`
}

type AgentRoleConfig struct {
	Type        string   `yaml:"type" json:"type"` // "builtin", "http", "cli", "disabled"
	BuiltinName string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command     string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args        []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint    string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model       string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey      string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Temperature float64  `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens   int      `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
}

type AgentsConfig struct {
	DefaultRole string                     `yaml:"default_role" json:"default_role"`
	Roles       map[string]AgentRoleConfig `yaml:"roles" json:"roles"`
	Fallbacks   map[string]string          `yaml:"fallbacks,omitempty" json:"fallbacks,omitempty"`
}

type TTSConfig struct {
	Type         string   `yaml:"type" json:"type"` // "builtin", "http", "cli", "disabled"
	BuiltinName  string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command      string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args         []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint     string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model        string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey       string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	DefaultVoice string   `yaml:"default_voice,omitempty" json:"default_voice,omitempty"`
	Pitch        float64  `yaml:"pitch,omitempty" json:"pitch,omitempty"`
	SpeechRate   float64  `yaml:"speech_rate,omitempty" json:"speech_rate,omitempty"`
	AutoPlay     bool     `yaml:"auto_play" json:"auto_play"`
	MasterVolume float64  `yaml:"master_volume" json:"master_volume"`
}

type STTConfig struct {
	Type        string   `yaml:"type" json:"type"`
	BuiltinName string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command     string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args        []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint    string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model       string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey      string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}

type ImageConfig struct {
	Type         string   `yaml:"type" json:"type"`
	BuiltinName  string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command      string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args         []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint     string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model        string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey       string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	AutoGenerate bool     `yaml:"auto_generate" json:"auto_generate"`
}

type MediaConfig struct {
	TTS   TTSConfig   `yaml:"tts" json:"tts"`
	STT   STTConfig   `yaml:"stt" json:"stt"`
	Image ImageConfig `yaml:"image" json:"image"`
}

type PreferencesConfig struct {
	Streaming        bool   `yaml:"streaming" json:"streaming"`
	TypingSpeedMS    int    `yaml:"typing_speed_ms" json:"typing_speed_ms"`
	CinematicEffects bool   `yaml:"cinematic_effects" json:"cinematic_effects"`
	FontScale        string `yaml:"font_scale" json:"font_scale"`
}

type Config struct {
	Version     string            `yaml:"version" json:"version"`
	Paths       PathsConfig       `yaml:"paths" json:"paths"`
	Agents      AgentsConfig      `yaml:"agents" json:"agents"`
	Media       MediaConfig       `yaml:"media" json:"media"`
	Preferences PreferencesConfig `yaml:"preferences" json:"preferences"`
}

func DefaultConfig() *Config {
	return &Config{
		Version: "1",
		Paths: PathsConfig{
			Systems: "./systems",
			Worlds:  "./worlds",
			Games:   "./games",
			Cache:   "./cache",
		},
		Agents: AgentsConfig{
			DefaultRole: "gm",
			Roles: map[string]AgentRoleConfig{
				"gm": {
					Type:        "cli",
					Command:     "echo",
					Args:        []string{},
					Temperature: 0.7,
					MaxTokens:   1024,
				},
				"narrator": {
					Type: "disabled",
				},
			},
			Fallbacks: make(map[string]string),
		},
		Media: MediaConfig{
			TTS: TTSConfig{
				Type:         "disabled",
				DefaultVoice: "default",
				Pitch:        1.0,
				SpeechRate:   1.0,
				AutoPlay:     false,
				MasterVolume: 1.0,
			},
			STT: STTConfig{
				Type: "disabled",
			},
			Image: ImageConfig{
				Type:         "disabled",
				AutoGenerate: false,
			},
		},
		Preferences: PreferencesConfig{
			Streaming:        true,
			TypingSpeedMS:    15,
			CinematicEffects: true,
			FontScale:        "medium",
		},
	}
}
