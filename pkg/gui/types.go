package gui

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

type TurnDTO struct {
	TurnNumber  int      `json:"turn_number"`
	InputText   string   `json:"input_text"`
	Mode        string   `json:"mode"`
	Prose       string   `json:"prose"`
	Speaker     string   `json:"speaker,omitempty"`
	Dialogue    string   `json:"dialogue,omitempty"`
	AudioURL    string   `json:"audio_url,omitempty"`
	ImageURL    string   `json:"image_url,omitempty"`
	EntitiesHit []string `json:"entities_hit,omitempty"`
}

type EntityDTO struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Markdown  string                 `json:"markdown"`
	State     map[string]interface{} `json:"state"`
	Backlinks []string               `json:"backlinks"`
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
