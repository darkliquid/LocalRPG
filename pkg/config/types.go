package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/provider"
)

type PathsConfig struct {
	Systems string `yaml:"systems" json:"systems"`
	Worlds  string `yaml:"worlds" json:"worlds"`
	Games   string `yaml:"games" json:"games"`
	Cache   string `yaml:"cache" json:"cache"`
}

// Agent role names routed by the harness router.
const (
	RoleGM         = "gm"
	RoleNarrator   = "narrator"
	RoleExtractor  = "extractor"
	RoleCompletion = "completion"
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
	// SupportsTools is "auto", "yes", or "no". Empty means auto: an HTTP provider
	// gets tools and every other type does not.
	SupportsTools string `yaml:"supports_tools,omitempty" json:"supports_tools,omitempty"`

	ThinkingBudget *int     `yaml:"thinking_budget,omitempty" json:"thinking_budget,omitempty"`
	TopP           *float64 `yaml:"top_p,omitempty" json:"top_p,omitempty"`
	TopK           *int     `yaml:"top_k,omitempty" json:"top_k,omitempty"`

	// Instance is an optional user-chosen discriminator for this provider
	// configuration. It becomes the key's "@<instance>" segment, so two configs
	// of one adapter at one endpoint stay distinct. Empty means the discriminator
	// is derived from the endpoint or command.
	Instance string `yaml:"instance,omitempty" json:"instance,omitempty"`
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
	// SummaryEvery is how many turns pass between regenerations of the story so far.
	// Zero disables summarisation, so a campaign can opt out entirely.
	SummaryEvery     int `yaml:"summary_every" json:"summary_every"`
	SummaryCharLimit int `yaml:"summary_char_limit" json:"summary_char_limit"`
	// ThreadIdleTurns is when the UI nudges about a thread. It is presentation only:
	// the prompt always lists unresolved threads, however long they have been quiet.
	ThreadIdleTurns int `yaml:"thread_idle_turns" json:"thread_idle_turns"`
	// ThreadsMax caps the open-threads block, most stale first.
	ThreadsMax int `yaml:"threads_max" json:"threads_max"`
	// ContinuityChecks runs the deterministic drift pass. A pointer distinguishes
	// "not configured" from "switched off", because the default is on.
	ContinuityChecks *bool `yaml:"continuity_checks" json:"continuity_checks,omitempty"`
	// ActionEcho prepends the narrator's third-person restatement of the player's
	// action to each turn. A pointer distinguishes "not configured" (on) from
	// "switched off".
	ActionEcho *bool `yaml:"action_echo" json:"action_echo,omitempty"`
	// Completion governs how a narrator reply that stops mid-thought is repaired.
	Completion CompletionConfig `yaml:"completion" json:"completion"`
	// ToolRounds caps how many times a turn may call tools before tools are
	// withdrawn. Zero or negative means unbounded (with a 100-round runaway safety ceiling).
	ToolRounds int `yaml:"tool_rounds" json:"tool_rounds"`
	// ToolResultChars caps one tool result. Zero means the default of 4000.
	ToolResultChars int `yaml:"tool_result_chars" json:"tool_result_chars"`
}

// CompletionConfig governs how a narrator reply that stops mid-thought is
// repaired. The zero value means "auto" with the documented defaults, so a
// configuration written before these keys existed keeps working.
type CompletionConfig struct {
	// Mode is "auto", "continue", "trim", or "off". Empty means "auto".
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
	// MaxAttempts caps continuation calls per turn. Zero means one.
	MaxAttempts int `yaml:"max_attempts,omitempty" json:"max_attempts,omitempty"`
	// TailChars is how much of the partial reply the continuation call sees.
	TailChars int `yaml:"tail_chars,omitempty" json:"tail_chars,omitempty"`
	// MinIncompleteChars skips recovery for replies shorter than this.
	MinIncompleteChars int `yaml:"min_incomplete_chars,omitempty" json:"min_incomplete_chars"`
	// TimeoutSeconds bounds one continuation call.
	TimeoutSeconds int `yaml:"timeout_seconds,omitempty" json:"timeout_seconds"`
}

type VoiceProfile struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	VoiceID     string   `yaml:"voice_id" json:"voice_id"`
	Provider    string   `yaml:"provider,omitempty" json:"provider,omitempty"`
	Pitch       float64  `yaml:"pitch" json:"pitch"`
	SpeechRate  float64  `yaml:"speech_rate" json:"speech_rate"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	// Options holds provider-declared tunables, keyed by VoiceOption.Key. It is
	// opaque to the engine the same way entity State is: only the provider
	// interprets it, and an empty map is omitted.
	Options map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
}

type TTSConfig struct {
	Type          string         `yaml:"type" json:"type"` // "builtin", "http", "cli", "disabled"
	BuiltinName   string         `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	ModelPath     string         `yaml:"model_path,omitempty" json:"model_path,omitempty"`
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
	// Markdown selects how narration Markdown is treated before synthesis:
	// "auto" (default) reduces it unless the provider is Markdown-aware, "strip"
	// always reduces it, and "keep" sends it unchanged.
	Markdown string `yaml:"markdown,omitempty" json:"markdown,omitempty"`
	// Metered marks a provider that charges per request. It overrides the
	// provider's own declaration, so an operator can flag a proxied endpoint.
	Metered *bool `yaml:"metered,omitempty" json:"metered,omitempty"`
	// Options holds provider-declared tunables for the default voice, keyed by
	// VoiceOption.Key. Absent means the provider's own defaults.
	Options map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
	// SpeechCues configures vocal performance steering tags and transcript display.
	SpeechCues SpeechCuesConfig `yaml:"speech_cues,omitempty" json:"speech_cues,omitempty"`
	// OpusBitrate is the target bitrate for stored Ogg/Opus clips, in bits per
	// second. Zero means the default.
	OpusBitrate int `yaml:"opus_bitrate,omitempty" json:"opus_bitrate,omitempty"`
	// StreamSentences synthesizes sentences while the model is still generating,
	// so the first beat is ready sooner. Nil means enabled, except for a provider
	// the operator has marked metered.
	StreamSentences *bool `yaml:"stream_sentences,omitempty" json:"stream_sentences,omitempty"`
	// Grouping selects whether adjacent same-speaker segments are sent in one
	// request: "auto" (default), "off", or "always".
	Grouping string `yaml:"grouping,omitempty" json:"grouping,omitempty"`
	// MultiSpeaker selects whether a run of two speakers is sent as one
	// multi-speaker request: "auto" (default), "off", or "always".
	MultiSpeaker string `yaml:"multi_speaker,omitempty" json:"multi_speaker,omitempty"`
	// Limits overrides the provider's advertised per-request limits, for a
	// proxied or self-hosted endpoint whose limits cannot be queried.
	Limits *TTSLimits `yaml:"limits,omitempty" json:"limits,omitempty"`

	// Instance is an optional user-chosen discriminator for this provider
	// configuration. It becomes the key's "@<instance>" segment, so two configs
	// of one adapter at one endpoint stay distinct. Empty means the discriminator
	// is derived from the endpoint or command.
	Instance string `yaml:"instance,omitempty" json:"instance,omitempty"`
}

// TTSLimits overrides a speech provider's per-request limits. A zero field keeps
// the provider's advertised value.
type TTSLimits struct {
	MaxChars    int `yaml:"max_chars,omitempty" json:"max_chars,omitempty"`
	MaxTokens   int `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	MaxSpeakers int `yaml:"max_speakers,omitempty" json:"max_speakers,omitempty"`
}

// SpeechCuesConfig controls how vocal acting and steering hints are used and rendered.
type SpeechCuesConfig struct {
	Enabled          bool   `yaml:"enabled" json:"enabled"`
	AudioTags        *bool  `yaml:"audio_tags,omitempty" json:"audio_tags,omitempty"`
	MarkdownEmphasis *bool  `yaml:"markdown_emphasis,omitempty" json:"markdown_emphasis,omitempty"`
	DisplayMode      string `yaml:"display_mode,omitempty" json:"display_mode,omitempty"`
}

type STTConfig struct {
	Type        string   `yaml:"type" json:"type"`
	BuiltinName string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command     string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args        []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint    string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model       string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey      string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`

	// Instance is an optional user-chosen discriminator for this provider
	// configuration. It becomes the key's "@<instance>" segment, so two configs
	// of one adapter at one endpoint stay distinct. Empty means the discriminator
	// is derived from the endpoint or command.
	Instance string `yaml:"instance,omitempty" json:"instance,omitempty"`
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
	// Trigger selects when a turn image is generated: off, scene_break,
	// significant, every_turn, or manual. Empty means significant.
	Trigger string `yaml:"trigger,omitempty" json:"trigger,omitempty"`

	AspectRatio      string `yaml:"aspect_ratio,omitempty" json:"aspect_ratio,omitempty"`
	PersonGeneration string `yaml:"person_generation,omitempty" json:"person_generation,omitempty"`

	// Instance is an optional user-chosen discriminator for this provider
	// configuration. It becomes the key's "@<instance>" segment, so two configs
	// of one adapter at one endpoint stay distinct. Empty means the discriminator
	// is derived from the endpoint or command.
	Instance string `yaml:"instance,omitempty" json:"instance,omitempty"`
}

type MediaConfig struct {
	TTS   TTSConfig   `yaml:"tts" json:"tts"`
	STT   STTConfig   `yaml:"stt" json:"stt"`
	Image ImageConfig `yaml:"image" json:"image"`
	// TTSProviders, STTProviders, and ImageProviders hold named configurations of
	// each family. The singleton fields above remain the default entry; a named
	// entry coexists with it and is selected by name.
	TTSProviders   map[string]TTSConfig   `yaml:"tts_providers,omitempty" json:"tts_providers,omitempty"`
	STTProviders   map[string]STTConfig   `yaml:"stt_providers,omitempty" json:"stt_providers,omitempty"`
	ImageProviders map[string]ImageConfig `yaml:"image_providers,omitempty" json:"image_providers,omitempty"`
	// Purposes maps a use name (narrator, npc, scene, portrait, placeholder) to a
	// provider name from the matching family map, or the family default when
	// unset.
	Purposes map[string]string `yaml:"purposes,omitempty" json:"purposes,omitempty"`
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

// TelemetryConfig configures OpenTelemetry export. The zero value is disabled,
// so configuration written before telemetry existed behaves as it did.
type TelemetryConfig struct {
	Enabled     bool              `yaml:"enabled" json:"enabled"`
	Endpoint    string            `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Protocol    string            `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	Insecure    bool              `yaml:"insecure,omitempty" json:"insecure,omitempty"`
	SampleRatio float64           `yaml:"sample_ratio,omitempty" json:"sample_ratio,omitempty"`
	Traces      bool              `yaml:"traces" json:"traces"`
	Metrics     bool              `yaml:"metrics" json:"metrics"`
	Logs        bool              `yaml:"logs" json:"logs"`
	Headers     map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	ServiceName string            `yaml:"service_name,omitempty" json:"service_name,omitempty"`
}

// ProvidersConfig groups shared credentials and defaults for external ecosystem providers.
type ProvidersConfig struct {
	Gemini   GeminiProviderConfig   `yaml:"gemini,omitempty" json:"gemini,omitempty"`
	Inworld  InworldProviderConfig  `yaml:"inworld,omitempty" json:"inworld,omitempty"`
	Cartesia CartesiaProviderConfig `yaml:"cartesia,omitempty" json:"cartesia,omitempty"`
	// Currency is the display currency for cost figures. Prices are expressed in
	// this currency; no conversion is performed.
	Currency string `yaml:"currency,omitempty" json:"currency,omitempty"`
	// Prices override the built-in price table, matched by provider then model.
	Prices []PriceConfig `yaml:"prices,omitempty" json:"prices,omitempty"`
}

// InworldProviderConfig holds the credential shared by every Inworld capability
// (LLM router, TTS, STT). A per-service api_key overrides it.
type InworldProviderConfig struct {
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}

// PriceConfig is one provider's price. A zero model matches every model of the
// provider. Values are in micros (1e-6 currency units).
type PriceConfig struct {
	Provider         string `yaml:"provider" json:"provider"`
	Model            string `yaml:"model,omitempty" json:"model,omitempty"`
	PerMillionInput  int64  `yaml:"per_million_input,omitempty" json:"per_million_input,omitempty"`
	PerMillionOutput int64  `yaml:"per_million_output,omitempty" json:"per_million_output,omitempty"`
	PerCharacter     int64  `yaml:"per_character,omitempty" json:"per_character,omitempty"`
	PerRequest       int64  `yaml:"per_request,omitempty" json:"per_request,omitempty"`
}

type GeminiProviderConfig struct {
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}

type CartesiaProviderConfig struct {
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}

type EmbeddingsConfig struct {
	Enabled    bool                               `yaml:"enabled" json:"enabled"`
	Provider   string                             `yaml:"provider" json:"provider"` // "builtin-local", "openai", "gemini", "disabled"
	Model      string                             `yaml:"model,omitempty" json:"model,omitempty"`
	Dimensions int                                `yaml:"dimensions,omitempty" json:"dimensions,omitempty"`
	BatchSize  int                                `yaml:"batch_size,omitempty" json:"batch_size,omitempty"`
	Providers  map[string]EmbeddingProviderConfig `yaml:"providers,omitempty" json:"providers,omitempty"`
}

type EmbeddingProviderConfig struct {
	Type        string `yaml:"type" json:"type"` // "builtin", "onnx", "http", "gemini", "disabled"
	BuiltinName string `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Endpoint    string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	URL         string `yaml:"url,omitempty" json:"url,omitempty"`
	APIKey      string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Model       string `yaml:"model,omitempty" json:"model,omitempty"`
	// ModelPath is the directory holding a local model's files. It is used by
	// the onnx provider; an empty value falls back to the app's model cache.
	ModelPath string `yaml:"model_path,omitempty" json:"model_path,omitempty"`

	// Instance is an optional user-chosen discriminator for this provider
	// configuration. It becomes the key's "@<instance>" segment, so two configs
	// of one adapter at one endpoint stay distinct. Empty means the discriminator
	// is derived from the endpoint or command.
	Instance string `yaml:"instance,omitempty" json:"instance,omitempty"`
}

type Config struct {
	Version     string            `yaml:"version" json:"version"`
	Paths       PathsConfig       `yaml:"paths" json:"paths"`
	Providers   ProvidersConfig   `yaml:"providers,omitempty" json:"providers,omitempty"`
	Agents      AgentsConfig      `yaml:"agents" json:"agents"`
	Media       MediaConfig       `yaml:"media" json:"media"`
	Embeddings  EmbeddingsConfig  `yaml:"embeddings,omitempty" json:"embeddings,omitempty"`
	Preferences PreferencesConfig `yaml:"preferences" json:"preferences"`
	Telemetry   TelemetryConfig   `yaml:"telemetry,omitempty" json:"telemetry,omitempty"`
	Mechanics   MechanicsConfig   `yaml:"mechanics,omitempty" json:"mechanics,omitempty"`
	Interactive InteractiveConfig `yaml:"interactive,omitempty" json:"interactive,omitempty"`
	Generation  GenerationConfig  `yaml:"generation,omitempty" json:"generation,omitempty"`
	Publishers  map[string]string `yaml:"publishers,omitempty" json:"publishers,omitempty"`
	Registries  RegistriesConfig  `yaml:"registries,omitempty" json:"registries,omitempty"`
}

// GenerationConfig bounds an AI world generation. It exists so a runaway
// generation cannot spend without limit, which is a control the usage ledger,
// recording spend after the fact, cannot provide.
type GenerationConfig struct {
	// MaxCalls caps the model calls one generation may make. Zero means the
	// default of 20, which is generous enough not to affect a normal generation.
	MaxCalls int `yaml:"max_calls,omitempty" json:"max_calls,omitempty"`
}

// DefaultGenerationMaxCalls is the per-generation call cap when none is set.
const DefaultGenerationMaxCalls = 20

// GenerationMaxCalls returns the resolved per-generation call cap.
func (c *Config) GenerationMaxCalls() int {
	if c != nil && c.Generation.MaxCalls > 0 {
		return c.Generation.MaxCalls
	}
	return DefaultGenerationMaxCalls
}

// RegistriesConfig lists URLs of static or git package indexes.
type RegistriesConfig struct {
	URLs []string `yaml:"urls,omitempty" json:"urls,omitempty"`
}

// InteractiveConfig governs how interactive rolls are recorded.
type InteractiveConfig struct {
	// Rolls selects "continuation" (a new turn carries the adjudication) or
	// "single-turn" (the proposing turn is completed in place).
	Rolls string `yaml:"rolls,omitempty" json:"rolls,omitempty"`
}

// InteractiveRolls returns the resolved interactive-rolls mode, normalising an
// empty or unknown value to "continuation".
func (c *Config) InteractiveRolls() string {
	if c != nil && c.Interactive.Rolls == "single-turn" {
		return "single-turn"
	}
	return "continuation"
}

// ImageTrigger returns the resolved image-trigger policy, normalising an empty or
// unknown value to "significant".
func (c *Config) ImageTrigger() string {
	if c == nil {
		return "significant"
	}
	switch c.Media.Image.Trigger {
	case "off", "scene_break", "significant", "every_turn", "manual":
		return c.Media.Image.Trigger
	}
	return "significant"
}

// CurrentVersion is the config schema version.
const CurrentVersion = "2"

// Validate reports human-readable problems with a configuration. It never fails
// a load: a bad entry is reported so it can be surfaced while the rest of the
// configuration keeps working. An entry with a malformed provider key matches
// nothing, so silencing it would hide the reason a price is not applied.
func (c *Config) Validate() []string {
	var problems []string
	for i, price := range c.Providers.Prices {
		if price.Provider == "" {
			problems = append(problems, fmt.Sprintf("providers.prices[%d]: provider is required", i))
			continue
		}
		if _, err := provider.ParseKey(price.Provider); err != nil {
			problems = append(problems, fmt.Sprintf(
				"providers.prices[%d].provider %q is not a canonical key (for example %s, %s, %s): %v",
				i, price.Provider, provider.KeyLLMOpenAIChat, provider.KeyLLMGemini, provider.KeyTTSHTTP, err))
		}
	}

	for role, roleCfg := range c.Agents.Roles {
		problems = append(problems, providerProblems(
			fmt.Sprintf("agents.roles[%q]", role),
			roleCfg.Type, roleCfg.BuiltinName, roleCfg.Command, roleCfg.Endpoint, llmShape)...)
	}
	problems = append(problems, providerProblems(
		"media.tts", c.Media.TTS.Type, c.Media.TTS.BuiltinName, c.Media.TTS.Command, c.Media.TTS.Endpoint, ttsShape)...)
	problems = append(problems, providerProblems(
		"media.stt", c.Media.STT.Type, c.Media.STT.BuiltinName, c.Media.STT.Command, c.Media.STT.Endpoint, sttShape)...)
	problems = append(problems, providerProblems(
		"media.image", c.Media.Image.Type, c.Media.Image.BuiltinName, c.Media.Image.Command, c.Media.Image.Endpoint, imageShape)...)

	problems = append(problems, c.instanceProblems()...)
	problems = append(problems, c.mediaProviderProblems()...)

	return problems
}

// mediaProviderProblems validates the names of the named media provider maps.
// The reserved default name would shadow the singleton, and a malformed name
// could not be selected.
func (c *Config) mediaProviderProblems() []string {
	var problems []string
	validateProviderNames("media.tts_providers", mapKeys(c.Media.TTSProviders), &problems)
	validateProviderNames("media.stt_providers", mapKeys(c.Media.STTProviders), &problems)
	validateProviderNames("media.image_providers", mapKeys(c.Media.ImageProviders), &problems)
	problems = append(problems, c.purposeProblems()...)
	return problems
}

// purposeProblems validates the media.purposes map: each key must be a known
// purpose, and each name must exist in the purpose's family.
func (c *Config) purposeProblems() []string {
	var problems []string
	for _, use := range slices.Sorted(maps.Keys(c.Media.Purposes)) {
		name := c.Media.Purposes[use]
		family := PurposeFamily(Purpose(use))
		if family == "" {
			problems = append(problems, "media.purposes."+use+": unknown purpose")
			continue
		}
		if name == "" || name == ReservedProviderName {
			continue
		}
		switch family {
		case "tts":
			if _, ok := c.Media.TTSProviders[name]; !ok {
				problems = append(problems, "media.purposes."+use+": tts provider "+name+" does not exist")
			}
		case "image":
			if _, ok := c.Media.ImageProviders[name]; !ok {
				problems = append(problems, "media.purposes."+use+": image provider "+name+" does not exist")
			}
		}
	}
	return problems
}

func validateProviderNames(path string, names []string, problems *[]string) {
	for _, name := range names {
		switch {
		case name == ReservedProviderName:
			*problems = append(*problems, path+": name "+ReservedProviderName+" is reserved")
		case !providerNamePattern.MatchString(name):
			*problems = append(*problems, fmt.Sprintf("%s: name %q must match %s", path, name, providerNamePattern))
		}
	}
}

func mapKeys[V interface{}](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	return out
}

var providerNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// instanceProblems validates the optional provider instance ids. Each must match
// the key discriminator grammar, and no two configurations of one family may
// share one, because they would collapse to the same canonical key and share a
// usage row, a price, and a cache namespace.
func (c *Config) instanceProblems() []string {
	var problems []string
	check := func(path, instance string, seen map[string]string) {
		id := strings.TrimSpace(instance)
		if id == "" {
			return
		}
		if err := provider.ValidateDiscriminator(id); err != nil {
			problems = append(problems, fmt.Sprintf("%s.instance %q is invalid: %v", path, id, err))
			return
		}
		if prev, ok := seen[id]; ok {
			problems = append(problems, fmt.Sprintf("%s.instance %q duplicates %s", path, id, prev))
			return
		}
		seen[id] = path
	}

	llmSeen := map[string]string{}
	for _, role := range slices.Sorted(maps.Keys(c.Agents.Roles)) {
		check(fmt.Sprintf("agents.roles[%q]", role), c.Agents.Roles[role].Instance, llmSeen)
	}
	embedSeen := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(c.Embeddings.Providers)) {
		check(fmt.Sprintf("embeddings.providers[%q]", name), c.Embeddings.Providers[name].Instance, embedSeen)
	}
	check("media.tts", c.Media.TTS.Instance, map[string]string{})
	check("media.stt", c.Media.STT.Instance, map[string]string{})
	check("media.image", c.Media.Image.Instance, map[string]string{})

	return problems
}

// MechanicsConfig tunes how mechanics are engaged during play.
type MechanicsConfig struct {
	// Engagement is "off", "auto", or "ask". Empty means auto.
	Engagement string `yaml:"engagement,omitempty" json:"engagement,omitempty"`
	// CadenceTurns forces a check after this many turns without one. Unset (0)
	// uses the default; a negative value disables the floor.
	CadenceTurns int `yaml:"cadence_turns,omitempty" json:"cadence_turns,omitempty"`
	// WorldTickTurns how often onWorldTick runs, in turns. Unset (0) or negative
	// disables it, so a campaign that ships no world tick is unaffected.
	WorldTickTurns int `yaml:"world_tick_turns,omitempty" json:"world_tick_turns,omitempty"`
}

// MechanicsEngagement is the configured policy, defaulting to "auto".
func (c *Config) MechanicsEngagement() string {
	switch mode := strings.ToLower(strings.TrimSpace(c.Mechanics.Engagement)); mode {
	case "off", "auto", "ask":
		return mode
	default:
		return "auto"
	}
}

// MechanicsCadenceTurns is how many turns without a check force one; a negative
// value disables the floor.
func (c *Config) MechanicsCadenceTurns() int {
	if c.Mechanics.CadenceTurns < 0 {
		return 0
	}
	if c.Mechanics.CadenceTurns == 0 {
		return 3
	}
	return c.Mechanics.CadenceTurns
}

// MechanicsWorldTickTurns is how often onWorldTick runs, in turns. Zero or a
// negative value disables it.
func (c *Config) MechanicsWorldTickTurns() int {
	if c.Mechanics.WorldTickTurns < 0 {
		return 0
	}
	return c.Mechanics.WorldTickTurns
}

func DefaultConfig() *Config {
	return &Config{
		Version: CurrentVersion,
		Paths:   PathsConfig{},
		Agents: AgentsConfig{
			DefaultRole:         "gm",
			TurnTimeoutSeconds:  300,
			ChunkTimeoutSeconds: 60,
			RecentTurnWindow:    6,
			RecentTurnCharLimit: 1200,
			SummaryEvery:        10,
			SummaryCharLimit:    2000,
			Completion: CompletionConfig{
				Mode:               "trim",
				MaxAttempts:        1,
				TailChars:          1500,
				MinIncompleteChars: 24,
				TimeoutSeconds:     45,
			},
			ToolRounds:      0,
			ToolResultChars: 4000,
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
				RoleCompletion: {
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
		Embeddings: EmbeddingsConfig{
			Enabled:    true,
			Provider:   "builtin-local",
			Model:      "hash-projection",
			Dimensions: 384,
			BatchSize:  16,
			Providers: map[string]EmbeddingProviderConfig{
				"builtin-local": {
					Type:        "builtin",
					BuiltinName: "hash-projection",
				},
			},
		},
		Preferences: PreferencesConfig{
			Streaming:        true,
			TypingSpeedMS:    15,
			CinematicEffects: true,
			FontScale:        "medium",
		},
		Telemetry: TelemetryConfig{
			Enabled:     false,
			Endpoint:    "localhost:4317",
			Protocol:    "grpc",
			Insecure:    true,
			SampleRatio: 1.0,
			Traces:      true,
			Metrics:     true,
			Logs:        true,
			ServiceName: "localrpg",
		},
		Publishers: make(map[string]string),
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

// SummaryEvery is how many turns pass between regenerations of the story so far.
// Zero disables summarisation.
func (c *Config) SummaryEvery() int {
	return c.Agents.SummaryEvery
}

// SummaryCharLimit caps the injected summary.
func (c *Config) SummaryCharLimit() int {
	if c.Agents.SummaryCharLimit <= 0 {
		return 2000
	}
	return c.Agents.SummaryCharLimit
}

// ThreadIdleTurns is when the UI nudges about an unresolved thread.
func (c *Config) ThreadIdleTurns() int {
	if c.Agents.ThreadIdleTurns <= 0 {
		return 10
	}
	return c.Agents.ThreadIdleTurns
}

// ThreadsMax caps how many open threads the prompt carries.
func (c *Config) ThreadsMax() int {
	if c.Agents.ThreadsMax <= 0 {
		return 8
	}
	return c.Agents.ThreadsMax
}

// ContinuityChecks runs the deterministic drift pass when explicitly configured on.
func (c *Config) ContinuityChecks() bool {
	return c.Agents.ContinuityChecks != nil && *c.Agents.ContinuityChecks
}

// ActionEcho reports whether the narrator should restate the player's action.
// The default is on, so a configuration that never mentions it keeps echoing.
func (c *Config) ActionEcho() bool {
	return c.Agents.ActionEcho == nil || *c.Agents.ActionEcho
}

// OpusBitrate is the bitrate stored speech is encoded at, defaulted and clamped
// to Opus's accepted range.
func (c *Config) OpusBitrate() int {
	bitrate := c.Media.TTS.OpusBitrate
	switch {
	case bitrate <= 0:
		return 32000
	case bitrate < 6000:
		return 6000
	case bitrate > 510000:
		return 510000
	default:
		return bitrate
	}
}

// TTSStreamSentences reports whether sentence-level pre-synthesis is enabled. It
// defaults on, and off for a provider the operator marked metered, so a metered
// provider is never charged for a clip a finished segment may not reuse.
func (c *Config) TTSStreamSentences() bool {
	if c.Media.TTS.StreamSentences != nil {
		return *c.Media.TTS.StreamSentences
	}
	return !(c.Media.TTS.Metered != nil && *c.Media.TTS.Metered)
}

// TTSGrouping is the grouping policy: "auto", "off", or "always". Unset is
// "auto", so grouping is used where a provider supports it.
func (c *Config) TTSGrouping() string {
	return normalizeTTSMode(c.Media.TTS.Grouping)
}

// TTSMultiSpeaker is the multi-speaker policy: "auto", "off", or "always".
// Unset is "auto", so a two-speaker run is grouped where a provider supports it.
func (c *Config) TTSMultiSpeaker() string {
	return normalizeTTSMode(c.Media.TTS.MultiSpeaker)
}

// normalizeTTSMode maps a grouping or multi-speaker value to one of "auto",
// "off", or "always". An unrecognised value is treated as "auto".
func normalizeTTSMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off":
		return "off"
	case "always":
		return "always"
	default:
		return "auto"
	}
}

// CompletionMode is the recovery policy: "auto", "continue", "trim", or "off".
// Unset defaults to "trim" so a cut reply ends instead of paying for a second
// full model call.
func (c *Config) CompletionMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Agents.Completion.Mode))
	switch mode {
	case "auto", "continue", "trim", "off":
		return mode
	default:
		return "trim"
	}
}

// CompletionAttempts caps continuation calls per turn.
func (c *Config) CompletionAttempts() int {
	if c.Agents.Completion.MaxAttempts <= 0 {
		return 1
	}
	return c.Agents.Completion.MaxAttempts
}

// CompletionTailChars is how much of the partial reply the continuation call sees.
func (c *Config) CompletionTailChars() int {
	if c.Agents.Completion.TailChars <= 0 {
		return 1500
	}
	return c.Agents.Completion.TailChars
}

// CompletionMinChars is the shortest incomplete reply worth recovering.
func (c *Config) CompletionMinChars() int {
	if c.Agents.Completion.MinIncompleteChars <= 0 {
		return 24
	}
	return c.Agents.Completion.MinIncompleteChars
}

// CompletionTimeout bounds one continuation call.
func (c *Config) CompletionTimeout() time.Duration {
	seconds := c.Agents.Completion.TimeoutSeconds
	if seconds <= 0 {
		seconds = 45
	}
	return time.Duration(seconds) * time.Second
}

// RoleSupportsTools is the tool capability for a role: "auto", "yes", or "no".
func (c *Config) RoleSupportsTools(role string) string {
	value := strings.ToLower(strings.TrimSpace(c.Agents.Roles[role].SupportsTools))
	switch value {
	case "yes", "no", "auto":
		return value
	default:
		return "auto"
	}
}

// defaultToolRounds bounds a turn's tool calls when none is configured. A
// negative value means unbounded.
const defaultToolRounds = 3

// ToolRounds caps how many times a turn may call tools. Zero or unset uses the
// default; a negative value means unbounded.
func (c *Config) ToolRounds() int {
	if c.Agents.ToolRounds < 0 {
		return 0
	}
	if c.Agents.ToolRounds == 0 {
		return defaultToolRounds
	}
	return c.Agents.ToolRounds
}

// ToolResultChars caps one tool result, so a broad query cannot flood the prompt.
func (c *Config) ToolResultChars() int {
	if c.Agents.ToolResultChars <= 0 {
		return 4000
	}
	return c.Agents.ToolResultChars
}
