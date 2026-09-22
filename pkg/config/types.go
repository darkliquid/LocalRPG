package config

import "time"

type PathsConfig struct {
	Systems string `yaml:"systems" json:"systems"`
	Worlds  string `yaml:"worlds" json:"worlds"`
	Games   string `yaml:"games" json:"games"`
	Cache   string `yaml:"cache" json:"cache"`
}

// Agent role names routed by the harness router.
const (
	RoleGM        = "gm"
	RoleNarrator  = "narrator"
	RoleExtractor = "extractor"
)

type AgentRoleConfig struct {
	Type        string   `yaml:"type" json:"type"`                                     // "builtin", "http", "cli", "inherit", "disabled"
	InheritFrom string   `yaml:"inherit_from,omitempty" json:"inherit_from,omitempty"` // role to inherit when type is "inherit"
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
	// TurnTimeoutSeconds bounds a whole turn; ChunkTimeoutSeconds bounds the
	// silence tolerated between narration deltas. Zero means "use the default",
	// so configuration written before these keys existed keeps working.
	TurnTimeoutSeconds  int `yaml:"turn_timeout_seconds" json:"turn_timeout_seconds"`
	ChunkTimeoutSeconds int `yaml:"chunk_timeout_seconds" json:"chunk_timeout_seconds"`
}

type VoiceProfile struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	VoiceID     string   `yaml:"voice_id" json:"voice_id"`
	Pitch       float64  `yaml:"pitch" json:"pitch"`
	SpeechRate  float64  `yaml:"speech_rate" json:"speech_rate"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
}

type TTSConfig struct {
	Type          string         `yaml:"type" json:"type"` // "builtin", "http", "cli", "disabled"
	BuiltinName   string         `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command       string         `yaml:"command,omitempty" json:"command,omitempty"`
	Args          []string       `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint      string         `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model         string         `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey        string         `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	DefaultVoice  string         `yaml:"default_voice,omitempty" json:"default_voice,omitempty"`
	Pitch         float64        `yaml:"pitch,omitempty" json:"pitch,omitempty"`
	SpeechRate    float64        `yaml:"speech_rate,omitempty" json:"speech_rate,omitempty"`
	AutoPlay      bool           `yaml:"auto_play" json:"auto_play"`
	MasterVolume  float64        `yaml:"master_volume" json:"master_volume"`
	VoiceProfiles []VoiceProfile `yaml:"voice_profiles,omitempty" json:"voice_profiles,omitempty"`
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
	// BuiltinFallback lets the built-in procedural generator stand in when no
	// provider is configured or a provider call fails, so imagery always exists
	// offline.
	BuiltinFallback bool `yaml:"builtin_fallback" json:"builtin_fallback"`
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
			DefaultRole:         "gm",
			TurnTimeoutSeconds:  300,
			ChunkTimeoutSeconds: 60,
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
				RoleExtractor: {
					Type:        "inherit",
					InheritFrom: RoleGM,
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
				VoiceProfiles: []VoiceProfile{
					{
						ID:          "elder_sage",
						Name:        "Elder Sage / Veteran",
						VoiceID:     "bm_george",
						Pitch:       0.85,
						SpeechRate:  0.90,
						Tags:        []string{"elder", "male", "wise", "gravelly", "veteran"},
						Description: "Ancient wizards, battle-weary commanders, village elders.",
					},
					{
						ID:          "young_scout",
						Name:        "Young Scout / Rogue",
						VoiceID:     "af_bella",
						Pitch:       1.05,
						SpeechRate:  1.10,
						Tags:        []string{"young", "female", "quick", "eager", "rogue"},
						Description: "Nimble rangers, streetwise thieves, eager apprentices.",
					},
					{
						ID:          "gruff_blacksmith",
						Name:        "Gruff Dwarf / Guard",
						VoiceID:     "am_adam",
						Pitch:       0.75,
						SpeechRate:  0.95,
						Tags:        []string{"stout", "male", "deep", "authoritative", "guard"},
						Description: "Dwarven smiths, tavern bouncers, fortress wardens.",
					},
					{
						ID:          "sinister_cultist",
						Name:        "Hushed Mystic / Villain",
						VoiceID:     "bf_emma",
						Pitch:       0.90,
						SpeechRate:  0.85,
						Tags:        []string{"eerie", "whisper", "sinister", "cultist"},
						Description: "Shadow mages, deceptive nobles, oracle priestesses.",
					},
				},
			},
			STT: STTConfig{
				Type: "disabled",
			},
			Image: ImageConfig{
				Type:            "disabled",
				AutoGenerate:    false,
				BuiltinFallback: true,
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

// TurnTimeout is the wall-clock budget for one turn.
func (c *Config) TurnTimeout() time.Duration {
	seconds := c.Agents.TurnTimeoutSeconds
	if seconds <= 0 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

// ChunkTimeout is the silence tolerated between narration deltas.
func (c *Config) ChunkTimeout() time.Duration {
	seconds := c.Agents.ChunkTimeoutSeconds
	if seconds <= 0 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}
