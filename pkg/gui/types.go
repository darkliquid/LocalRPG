package gui

import (
	"fmt"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

type PlayerDTO struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	State      map[string]interface{} `json:"state"`
	Appearance string                 `json:"appearance,omitempty"`
	Voice      *config.VoiceProfile   `json:"voice,omitempty"`
}

type NarrativeArcDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Progress    int    `json:"progress"`
	MaxProgress int    `json:"max_progress"`
	Status      string `json:"status"`
}

type FactionClockDTO struct {
	Faction  string `json:"faction"`
	Name     string `json:"name"`
	Ticks    int    `json:"ticks"`
	MaxTicks int    `json:"max_ticks"`
}

type GameStateDTO struct {
	GameID        string            `json:"game_id"`
	GameName      string            `json:"game_name"`
	Player        PlayerDTO         `json:"player"`
	Arcs          []NarrativeArcDTO `json:"arcs"`
	Clocks        []FactionClockDTO `json:"clocks"`
	Locations     []string          `json:"locations"`
	OpeningPrompt string            `json:"opening_prompt,omitempty"`
	NarratorVoice string            `json:"narrator_voice,omitempty"`
	StartLocation string            `json:"start_location,omitempty"`
	BannerURL     string            `json:"banner_url,omitempty"`
}

type SegmentDTO struct {
	Kind      string  `json:"kind"`
	Speaker   string  `json:"speaker,omitempty"`
	SpeakerID string  `json:"speaker_id,omitempty"`
	Text      string  `json:"text"`
	AudioURL  string  `json:"audio_url,omitempty"`
	AudioKey  string  `json:"audio_key,omitempty"`
	Player    bool    `json:"player,omitempty"`
	Duration  float64 `json:"duration"`
}

type TurnDTO struct {
	TurnNumber      int           `json:"turn_number"`
	InputText       string        `json:"input_text"`
	Mode            string        `json:"mode"`
	Prose           string        `json:"prose"`
	Speaker         string        `json:"speaker,omitempty"`
	Dialogue        string        `json:"dialogue,omitempty"`
	ImageURL        string        `json:"image_url,omitempty"`
	EntitiesHit     []string      `json:"entities_hit,omitempty"`
	Segments        []SegmentDTO  `json:"segments,omitempty"`
	Outcome         string        `json:"outcome,omitempty"`
	Truncated       bool          `json:"truncated,omitempty"`
	Recovery        string        `json:"recovery,omitempty"`
	ToolCalls       []ToolCallDTO `json:"tool_calls,omitempty"`
	ContextNotes    []string      `json:"context_notes,omitempty"`
	ContinuityNotes []string      `json:"continuity_notes,omitempty"`
	LocationID      string        `json:"location_id,omitempty"`
	LocationName    string        `json:"location_name,omitempty"`
	LocationArtURL  string        `json:"location_art_url,omitempty"`
	// Structured turn fields: the action verdict, whether it was rejected, and
	// the checks the GM resolved.
	Verdict  *harness.ActionVerdict `json:"verdict,omitempty"`
	Rejected bool                   `json:"rejected,omitempty"`
	Checks   []harness.CheckResult  `json:"checks,omitempty"`
}

// ToolCallDTO is one tool a turn called, with only its name and result size: the
// arguments and results live in the trace.
type ToolCallDTO struct {
	Name        string `json:"name"`
	ResultChars int    `json:"result_chars"`
}

// MemoryDTO is one entity memory in an entity's timeline.
type MemoryDTO struct {
	Turn       int      `json:"turn"`
	Kind       string   `json:"kind"`
	Text       string   `json:"text"`
	Importance int      `json:"importance"`
	Tags       []string `json:"tags,omitempty"`
}

type EntityDTO struct {	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Markdown   string                 `json:"markdown"`
	State      map[string]interface{} `json:"state"`
	Backlinks  []string               `json:"backlinks"`
	History    []int                  `json:"history,omitempty"`
	ParseError bool                   `json:"parse_error,omitempty"`
}

// MergeEntityRequestDTO names the note that should survive a merge.
type MergeEntityRequestDTO struct {
	Into string `json:"into"`
	// Confirm must be true. Folding two notes into one is destructive, so a
	// stray POST without an explicit confirmation is refused.
	Confirm bool `json:"confirm"`
}

type GraphNodeDTO struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

type GraphLinkDTO struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type GraphDTO struct {
	Nodes []GraphNodeDTO `json:"nodes"`
	Links []GraphLinkDTO `json:"links"`
}

type GameSummaryDTO struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	SystemID        string `json:"system_id"`
	WorldID         string `json:"world_id"`
	PlayerName      string `json:"player_name"`
	TurnCount       int    `json:"turn_count"`
	LastPlayed      string `json:"last_played"`
	ThumbnailURL    string `json:"thumbnail_url"`
	BannerURL       string `json:"banner_url,omitempty"`
	IconURL         string `json:"icon_url,omitempty"`
	PlayTimeSeconds int64  `json:"play_time_seconds,omitempty"`
}

type SystemSummaryDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type WorldSummaryDTO struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Genre             string   `json:"genre"`
	ArtStyle          string   `json:"art_style,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	CompatibleSystems []string `json:"compatible_systems"`
	BannerURL         string   `json:"banner_url,omitempty"`
	IconURL           string   `json:"icon_url,omitempty"`
}

type CreateGameRequestDTO struct {
	ID            string             `json:"id,omitempty"`
	Name          string             `json:"name"`
	SystemID      string             `json:"system_id"`
	WorldID       string             `json:"world_id"`
	PlayerName    string             `json:"player_name"`
	Player        PlayerCharacterDTO `json:"player,omitempty"`
	OpeningPrompt string             `json:"opening_prompt,omitempty"`
	NarratorVoice string             `json:"narrator_voice,omitempty"`
	StartLocation string             `json:"start_location,omitempty"`
}

// PlayerCharacterDTO is the authored protagonist gathered at campaign creation.
type PlayerCharacterDTO struct {
	Appearance string               `json:"appearance,omitempty"`
	Age        string               `json:"age,omitempty"`
	Gender     string               `json:"gender,omitempty"`
	Pronouns   string               `json:"pronouns,omitempty"`
	Background string               `json:"background,omitempty"`
	Voice      *config.VoiceProfile `json:"voice,omitempty"`
	Extra      map[string]string    `json:"extra,omitempty"`
}

// EntitySummaryDTO is one note as the codex browser lists it.
type EntitySummaryDTO struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Location   string   `json:"location,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	ParseError bool     `json:"parse_error,omitempty"`
}

// ThreadDTO is one unresolved arc as the client sees it.
type ThreadDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	LastAdvanced int    `json:"last_advanced"`
	Idle         int    `json:"idle"`
}

// RecapDTO is a campaign's long memory as the client reads it.
type RecapDTO struct {
	Summary     string `json:"summary,omitempty"`
	ThroughTurn int    `json:"through_turn"`
	// Enabled is false when summarisation is off, so a client can offer the panel
	// without offering a refresh that would do nothing.
	Enabled bool        `json:"enabled"`
	Threads []ThreadDTO `json:"threads,omitempty"`
}

// GameSettingsPatchDTO is a partial update of a campaign's settings. An absent
// field is left alone, which is what makes it a patch rather than a replace.
type GameSettingsPatchDTO struct {
	OpeningPrompt *string `json:"opening_prompt,omitempty"`
	NarratorVoice *string `json:"narrator_voice,omitempty"`
	StartLocation *string `json:"start_location,omitempty"`
}

type SystemDetailDTO struct {
	ID                string                     `json:"id"`
	Name              string                     `json:"name"`
	Version           string                     `json:"version"`
	Description       string                     `json:"description"`
	Script            string                     `json:"script"`
	RulesPrompt       string                     `json:"rules_prompt"`
	CharacterCreation core.CharacterCreationSpec `json:"character_creation"`
}

type CreateSystemRequestDTO struct {
	ID                string                     `json:"id,omitempty"`
	Name              string                     `json:"name"`
	Version           string                     `json:"version,omitempty"`
	Description       string                     `json:"description,omitempty"`
	Script            string                     `json:"script,omitempty"`
	RulesPrompt       string                     `json:"rules_prompt,omitempty"`
	CharacterCreation core.CharacterCreationSpec `json:"character_creation,omitempty"`
}

type WorldEntitySummaryDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type WorldDetailDTO struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	Genre         string                  `json:"genre"`
	DefaultSystem string                  `json:"default_system"`
	ArtStyle      string                  `json:"art_style"`
	Tags          []string                `json:"tags"`
	LorePrompt    string                  `json:"lore_prompt"`
	Entities      []WorldEntitySummaryDTO `json:"entities"`
}

type CreateWorldRequestDTO struct {
	ID            string   `json:"id,omitempty"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Genre         string   `json:"genre,omitempty"`
	DefaultSystem string   `json:"default_system,omitempty"`
	ArtStyle      string   `json:"art_style,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	LorePrompt    string   `json:"lore_prompt,omitempty"`
}

type WorldEntityDetailDTO struct {
	ID       string `json:"id"`
	Markdown string `json:"markdown"`
}

type SettingsResponseDTO struct {
	Config          config.Config `json:"config"`
	ConfigFilePath  string        `json:"config_file_path"`
	IsLocalOverride bool          `json:"is_local_override"`
}

type TestProviderRequestDTO struct {
	Category   string      `json:"category"` // "llm", "tts", "stt", "image"
	Provider   interface{} `json:"provider"`
	TestPrompt string      `json:"test_prompt,omitempty"`
	// VoiceID lets a probe audition a catalog voice that has not been saved yet.
	VoiceID string `json:"voice_id,omitempty"`
}

// TTSInspectRequestDTO asks what a TTS configuration can do. The config may be
// unsaved, which is what lets the editor describe a provider before it is applied.
type TTSInspectRequestDTO struct {
	Config  config.TTSConfig `json:"config"`
	Refresh bool             `json:"refresh,omitempty"`
}

// VoiceCatalogDTO is a provider's voices plus whether the provider can enumerate
// at all, so the UI can explain instead of offering a dead button.
type VoiceCatalogDTO struct {
	Available bool                  `json:"available"`
	FetchedAt time.Time             `json:"fetched_at,omitempty"`
	Stale     bool                  `json:"stale"`
	Voices    []media.ProviderVoice `json:"voices"`
}

// TTSInspectResponseDTO is everything the speech editor needs about one
// configuration. It never carries the configuration's API key.
type TTSInspectResponseDTO struct {
	ProviderKey string              `json:"provider_key"`
	Metered     bool                `json:"metered"`
	Options     []media.VoiceOption `json:"options,omitempty"`
	Catalog     VoiceCatalogDTO     `json:"catalog"`
	// KeyPresent reports whether a credential is configured. The key itself is
	// never included.
	KeyPresent bool `json:"key_present"`
	// KeyRequired reports whether this provider needs a credential at all, so the
	// editor can explain a missing key instead of showing one to every provider.
	KeyRequired bool `json:"key_required"`
	// Error is a non-fatal catalog failure, so the editor still renders options.
	Error string `json:"error,omitempty"`
	// SpeechCues describes vocal acting and performance steering capabilities.
	SpeechCues media.SpeechCueCapabilities `json:"speech_cues"`
}

type TestProviderResponseDTO struct {
	Success   bool   `json:"success"`
	LatencyMS int64  `json:"latency_ms"`
	Message   string `json:"message"`
	Preview   string `json:"preview,omitempty"`
	// AudioDataURI carries synthesized speech as an inline data URI so a client
	// can play the exact clip a probe produced instead of only reporting it.
	AudioDataURI string `json:"audio_data_uri,omitempty"`
	ModelMissing bool   `json:"model_missing,omitempty"`
	ModelID      string `json:"model_id,omitempty"`
}

// ModelCatalogueRequestDTO asks the provider what models a key can reach. The
// key is optional so a shared key configured on the Providers tab is used.
type ModelCatalogueRequestDTO struct {
	APIKey string `json:"api_key,omitempty"`
}

// ModelCatalogueResponseDTO is a provider's live model list. A failure is
// reported in Error so the editor can keep showing a static fallback.
type ModelCatalogueResponseDTO struct {
	Models []media.GeminiModel `json:"models"`
	Error  string              `json:"error,omitempty"`
}

// VoiceSearchRequestDTO searches a provider's extended voice library. Config is
// optional; when present its key and provider identity take part in resolution.
type VoiceSearchRequestDTO struct {
	Config       config.TTSConfig `json:"config"`
	Query        string           `json:"query,omitempty"`
	Type         string           `json:"type,omitempty"`
	LanguageCode string           `json:"language_code,omitempty"`
	Gender       string           `json:"gender,omitempty"`
	Accent       string           `json:"accent,omitempty"`
	Persona      string           `json:"persona,omitempty"`
}

// VoiceSearchResponseDTO is one page of extended voices. A failure is reported
// in Error rather than as an HTTP error, so search can degrade gracefully.
type VoiceSearchResponseDTO struct {
	Voices []media.ProviderVoice `json:"voices"`
	Error  string                `json:"error,omitempty"`
}

// ProviderCatalogDTO is every provider the build knows, as descriptors.
type ProviderCatalogDTO struct {
	Providers []provider.Descriptor `json:"providers"`
}

// TurnRequest is a player action as submitted from a client.
type TurnRequest struct {
	Mode  string `json:"mode"`
	Input string `json:"input"`
}

// TurnEvent is one NDJSON line sent while a turn runs.
type TurnEvent struct {
	Type    string   `json:"type"`                 // "chunk", "turn", "tool", "error", or "model_missing"
	Text    string   `json:"text,omitempty"`       // narration delta
	Turn    *TurnDTO `json:"turn,omitempty"`       // the persisted turn
	Message string   `json:"message,omitempty"`    // failure detail
	ModelID string   `json:"model_id,omitempty"`   // missing model ID
	Name    string   `json:"name,omitempty"`       // friendly model name
	Size    int64    `json:"size_bytes,omitempty"` // model size in bytes
	// Tool activity, present when Type is "tool".
	ToolName    string `json:"tool_name,omitempty"`
	ToolStatus  string `json:"tool_status,omitempty"`
	ToolSummary string `json:"tool_summary,omitempty"`
	// Structured generation failure detail, present when Type is "error".
	Code    string                     `json:"code,omitempty"`
	Detail  string                     `json:"detail,omitempty"`
	Failure *harness.GenerationFailure `json:"failure,omitempty"`
}

// turnModes maps the mode names a client may send to the engine's casing.
var turnModes = map[string]string{
	"do": "Do", "say": "Say", "story": "Story", "roll": "Roll", "gm": "GM", "system": "System",
	"opening": engine.OpeningMode,
}

// validate normalises a submitted turn and rejects one the engine cannot run.
func (r *TurnRequest) validate() error {
	trimmedInput := strings.TrimSpace(r.Input)
	if strings.HasPrefix(trimmedInput, "/say ") {
		r.Mode = "say"
		r.Input = strings.TrimPrefix(trimmedInput, "/say ")
	} else if strings.HasPrefix(trimmedInput, "/do ") {
		r.Mode = "do"
		r.Input = strings.TrimPrefix(trimmedInput, "/do ")
	} else if strings.HasPrefix(trimmedInput, "/story ") {
		r.Mode = "story"
		r.Input = strings.TrimPrefix(trimmedInput, "/story ")
	} else if strings.HasPrefix(trimmedInput, "/roll ") {
		r.Mode = "roll"
		r.Input = strings.TrimPrefix(trimmedInput, "/roll ")
	}

	mode, ok := turnModes[strings.ToLower(strings.TrimSpace(r.Mode))]
	if !ok {
		return fmt.Errorf("unknown mode %q", r.Mode)
	}
	r.Mode = mode

	// The opening turn carries no player action: its instruction is the campaign's
	// opening prompt, so an empty input is correct rather than a mistake.
	if strings.TrimSpace(r.Input) == "" && mode != engine.OpeningMode {
		return fmt.Errorf("input is required")
	}
	return nil
}

// AudioStatusDTO reports whether the application can play audio itself and
// whether narration is currently running. A client uses it to choose between
// application playback and a browser audio element.
type AudioStatusDTO struct {
	Available bool `json:"available"`
	Playing   bool `json:"playing"`
}

// TraceEventDTO is one traced event. The event's own fields are nested rather
// than flattened so the envelope stays stable as the catalogue grows.
type TraceEventDTO struct {
	Time   string                 `json:"ts"`
	Event  string                 `json:"event"`
	Level  string                 `json:"level"`
	Fields map[string]interface{} `json:"fields,omitempty"`
}

// STTResponse is the transcription result returned from POST /api/stt.
type STTResponse struct {
	Text string `json:"text"`
}

type RefDTO struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Relation string `json:"relation,omitempty"`
}

type SectionReportDTO struct {
	Name     string   `json:"name"`
	Tokens   int      `json:"tokens"`
	Included bool     `json:"included"`
	Source   string   `json:"source,omitempty"`
	Refs     []RefDTO `json:"refs,omitempty"`
}

type ProviderSessionDTO struct {
	Provider    string `json:"provider"`
	ID          string `json:"id"`
	ThroughTurn int    `json:"through_turn"`
	Model       string `json:"model,omitempty"`
	PrefixHash  string `json:"prefix_hash,omitempty"`
}

type TurnContextDTO struct {
	TurnNumber      int                 `json:"turn_number"`
	Mode            string              `json:"mode"`
	Budget          int                 `json:"budget"`
	EstimatedTokens int                 `json:"estimated_tokens"`
	Sections        []SectionReportDTO  `json:"sections"`
	Refs            []RefDTO            `json:"refs"`
	WorkingSet      []RefDTO            `json:"working_set"`
	Threads         []string            `json:"threads,omitempty"`
	SummaryVersion  int                 `json:"summary_version"`
	WorldHash       string              `json:"world_hash,omitempty"`
	SystemHash      string              `json:"system_hash,omitempty"`
	PromptHash      string              `json:"prompt_hash"`
	Strategy        string              `json:"strategy"`
	PrefixHash      string              `json:"prefix_hash,omitempty"`
	Session         *ProviderSessionDTO `json:"session,omitempty"`
	CachedTokens    int                 `json:"cached_tokens,omitempty"`
	Prompt          string              `json:"prompt,omitempty"`
}

type WorkingEntryDTO struct {
	Kind     string  `json:"kind"`
	ID       string  `json:"id"`
	Name     string  `json:"name,omitempty"`
	Weight   float64 `json:"weight"`
	LastTurn int     `json:"last_turn"`
	Role     string  `json:"role,omitempty"`
}

