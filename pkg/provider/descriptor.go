package provider

// Family is the kind of capability a provider offers.
type Family string

const (
	FamilyLLM       Family = "llm"
	FamilyTTS       Family = "tts"
	FamilySTT       Family = "stt"
	FamilyImage     Family = "image"
	FamilyEmbedding Family = "embedding"
)

// Feature is one capability a provider may advertise. Features are declared by
// the provider package and checked against the adapter's real interfaces by the
// drift guard, so a descriptor cannot claim what the code cannot do.
type Feature string

const (
	FeatureStreaming        Feature = "streaming"
	FeatureTools            Feature = "tools"
	FeatureThinking         Feature = "thinking"
	FeatureVision           Feature = "vision"
	FeatureVoiceCatalog     Feature = "voice_catalog"
	FeatureVoiceOptions     Feature = "voice_options"
	FeatureSpeechCues       Feature = "speech_cues"
	FeatureMarkdownEmphasis Feature = "markdown_emphasis"
	FeatureMetered          Feature = "metered"
	FeatureKeyRequired      Feature = "key_required"
	FeatureModelCatalogue   Feature = "model_catalogue"
	FeatureExtendedVoices   Feature = "extended_voices"
	FeatureOffline          Feature = "offline"
	FeatureAutoGenerate     Feature = "auto_generate"
	FeatureSessions         Feature = "sessions"
	FeatureContextCache     Feature = "context_cache"
)

// Tunable declares one user-adjustable provider parameter. It generalises
// media.VoiceOption and the agent generation parameters, so one renderer covers
// every family.
type Tunable struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"` // float | int | bool | string | enum
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	Step    float64  `json:"step,omitempty"`
	Options []string `json:"options,omitempty"`
	Default any      `json:"default,omitempty"`
	Help    string   `json:"help,omitempty"`
}

// Preset is a ready-made configuration skeleton. Config keys match the family's
// config struct so the UI can merge it into the editor.
type Preset struct {
	ID          string         `json:"id"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Config      map[string]any `json:"config"`
	Order       int            `json:"order"`
}

// Descriptor is everything a caller needs to know about a provider without
// constructing it.
type Descriptor struct {
	ID          string    `json:"id"`
	Family      Family    `json:"family"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	Source      string    `json:"source"` // builtin | cli | http | gemini
	Features    []Feature `json:"features"`
	Tunables    []Tunable `json:"tunables,omitempty"`
	Presets     []Preset  `json:"presets,omitempty"`
}
