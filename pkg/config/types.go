package config

import (
	"strings"
	"time"
)

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
	// ContextTokenBudget bounds the assembled prompt, in estimated tokens. Zero
	// means unbounded, which is the behaviour for configuration written before
	// this key existed.
	ContextTokenBudget int `yaml:"context_token_budget" json:"context_token_budget"`
	// RecentTurnWindow is how many prior turns the narrator is reminded of.
	RecentTurnWindow int `yaml:"recent_turn_window" json:"recent_turn_window"`
	// RecentTurnCharLimit caps the text recalled from any one prior turn.
	RecentTurnCharLimit int `yaml:"recent_turn_char_limit" json:"recent_turn_char_limit"`
	// Tracing bounds. Tracing is opt-in, so these guard against a debug session
	// left running rather than rationing normal play, and they can afford to be
	// generous.
	TracePayloadChars int   `yaml:"trace_payload_chars" json:"trace_payload_chars"`
	TraceMaxBytes     int64 `yaml:"trace_max_bytes" json:"trace_max_bytes"`
	TraceMaxFiles     int   `yaml:"trace_max_files" json:"trace_max_files"`
	TraceRotateCheck  int   `yaml:"trace_rotate_check" json:"trace_rotate_check"`
	TraceChunkLimit   int   `yaml:"trace_chunk_limit" json:"trace_chunk_limit"`
	// Recall bounds. Scene recall covers the current location; retrieval covers the
	// turns that share entities with what is in play.
	SceneRecallTurns       int `yaml:"scene_recall_turns" json:"scene_recall_turns"`
	SceneRecallChars       int `yaml:"scene_recall_chars" json:"scene_recall_chars"`
	RetrievalTurns         int `yaml:"retrieval_turns" json:"retrieval_turns"`
	RetrievalChars         int `yaml:"retrieval_chars" json:"retrieval_chars"`
	RetrievalHalfLifeTurns int `yaml:"retrieval_halflife_turns" json:"retrieval_halflife_turns"`
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
	// TraceLevel is "off", "summary", or "full". Off is the default so normal
	// play writes nothing.
	TraceLevel string `yaml:"trace_level" json:"trace_level"`
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
			RecentTurnWindow:    6,
			RecentTurnCharLimit: 1200,
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

// RecentTurns is how many prior turns the narrator is reminded of.
func (c *Config) RecentTurns() int {
	if c.Agents.RecentTurnWindow <= 0 {
		return 6
	}
	return c.Agents.RecentTurnWindow
}

// RecentTurnChars caps the text recalled from any one prior turn, so a single
// long scene cannot crowd out everything else.
func (c *Config) RecentTurnChars() int {
	if c.Agents.RecentTurnCharLimit <= 0 {
		return 1200
	}
	return c.Agents.RecentTurnCharLimit
}

// ContextBudget is the estimated token ceiling for an assembled prompt. Zero
// means unbounded.
func (c *Config) ContextBudget() int {
	return c.Agents.ContextTokenBudget
}

// ChunkTimeout is the silence tolerated between narration deltas.
func (c *Config) ChunkTimeout() time.Duration {
	seconds := c.Agents.ChunkTimeoutSeconds
	if seconds <= 0 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

// TraceLevel is the configured trace detail, defaulting to off.
func (c *Config) TraceLevel() string {
	if strings.TrimSpace(c.Preferences.TraceLevel) == "" {
		return "off"
	}
	return c.Preferences.TraceLevel
}

// TracePayloadChars caps any single recorded string.
func (c *Config) TracePayloadChars() int {
	if c.Agents.TracePayloadChars <= 0 {
		return 20000
	}
	return c.Agents.TracePayloadChars
}

// TraceMaxBytes is the size at which the trace rotates.
func (c *Config) TraceMaxBytes() int64 {
	if c.Agents.TraceMaxBytes <= 0 {
		return 268435456
	}
	return c.Agents.TraceMaxBytes
}

// TraceMaxFiles is how many rotated trace files are kept.
func (c *Config) TraceMaxFiles() int {
	if c.Agents.TraceMaxFiles <= 0 {
		return 3
	}
	return c.Agents.TraceMaxFiles
}

// TraceRotateCheck is how many events pass between rotation checks.
func (c *Config) TraceRotateCheck() int {
	if c.Agents.TraceRotateCheck <= 0 {
		return 200
	}
	return c.Agents.TraceRotateCheck
}

// TraceChunkLimit bounds how many wire or chunk events one provider call records.
func (c *Config) TraceChunkLimit() int {
	if c.Agents.TraceChunkLimit <= 0 {
		return 500
	}
	return c.Agents.TraceChunkLimit
}

// SceneRecallTurns is how many prior turns at the current location are recalled.
func (c *Config) SceneRecallTurns() int {
	if c.Agents.SceneRecallTurns <= 0 {
		return 4
	}
	return c.Agents.SceneRecallTurns
}

// SceneRecallChars caps the excerpt taken from one recalled turn.
func (c *Config) SceneRecallChars() int {
	if c.Agents.SceneRecallChars <= 0 {
		return 800
	}
	return c.Agents.SceneRecallChars
}

// RetrievalTurns is how many turns are retrieved by entity overlap.
func (c *Config) RetrievalTurns() int {
	if c.Agents.RetrievalTurns <= 0 {
		return 3
	}
	return c.Agents.RetrievalTurns
}

// RetrievalChars caps the excerpt taken from one retrieved turn.
func (c *Config) RetrievalChars() int {
	if c.Agents.RetrievalChars <= 0 {
		return 800
	}
	return c.Agents.RetrievalChars
}

// RetrievalHalfLifeTurns is the age at which a retrieved turn's recency weight
// halves. A linear weight reaching zero at the window edge would make retrieval
// useless for exactly the cases it exists for.
func (c *Config) RetrievalHalfLifeTurns() int {
	if c.Agents.RetrievalHalfLifeTurns <= 0 {
		return 12
	}
	return c.Agents.RetrievalHalfLifeTurns
}
