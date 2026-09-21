package gui

import (
	"github.com/darkliquid/localrpg/pkg/config"
)

type PlayerDTO struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Type  string                 `json:"type"`
	State map[string]interface{} `json:"state"`
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
	GameID    string            `json:"game_id"`
	GameName  string            `json:"game_name"`
	Player    PlayerDTO         `json:"player"`
	Arcs      []NarrativeArcDTO `json:"arcs"`
	Clocks    []FactionClockDTO `json:"clocks"`
	Locations []string          `json:"locations"`
}

type SegmentDTO struct {
	Kind      string `json:"kind"`
	Speaker   string `json:"speaker,omitempty"`
	SpeakerID string `json:"speaker_id,omitempty"`
	Text      string `json:"text"`
	AudioURL  string `json:"audio_url,omitempty"`
}

type TurnDTO struct {
	TurnNumber     int          `json:"turn_number"`
	InputText      string       `json:"input_text"`
	Mode           string       `json:"mode"`
	Prose          string       `json:"prose"`
	Speaker        string       `json:"speaker,omitempty"`
	Dialogue       string       `json:"dialogue,omitempty"`
	ImageURL       string       `json:"image_url,omitempty"`
	EntitiesHit    []string     `json:"entities_hit,omitempty"`
	Segments       []SegmentDTO `json:"segments,omitempty"`
	Outcome        string       `json:"outcome,omitempty"`
	LocationID     string       `json:"location_id,omitempty"`
	LocationName   string       `json:"location_name,omitempty"`
	LocationArtURL string       `json:"location_art_url,omitempty"`
}

type EntityDTO struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Markdown  string                 `json:"markdown"`
	State     map[string]interface{} `json:"state"`
	Backlinks []string               `json:"backlinks"`
	History   []int                  `json:"history,omitempty"`
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
	ID           string `json:"id"`
	Name         string `json:"name"`
	SystemID     string `json:"system_id"`
	WorldID      string `json:"world_id"`
	PlayerName   string `json:"player_name"`
	TurnCount    int    `json:"turn_count"`
	LastPlayed   string `json:"last_played"`
	ThumbnailURL string `json:"thumbnail_url"`
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
	CompatibleSystems []string `json:"compatible_systems"`
}

type CreateGameRequestDTO struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name"`
	SystemID   string `json:"system_id"`
	WorldID    string `json:"world_id"`
	PlayerName string `json:"player_name"`
}

type SystemDetailDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Script      string `json:"script"`
	RulesPrompt string `json:"rules_prompt"`
}

type CreateSystemRequestDTO struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Script      string `json:"script,omitempty"`
	RulesPrompt string `json:"rules_prompt,omitempty"`
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
}

type TestProviderResponseDTO struct {
	Success   bool   `json:"success"`
	LatencyMS int64  `json:"latency_ms"`
	Message   string `json:"message"`
	Preview   string `json:"preview,omitempty"`
}
