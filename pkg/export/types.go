package export

import "time"

type SceneBeat struct {
	TurnNumber  int       `json:"turn_number"`
	Timestamp   time.Time `json:"timestamp"`
	Mode        string    `json:"mode"`
	PlayerInput string    `json:"player_input"`
	Prose       string    `json:"prose"`
	Speaker     string    `json:"speaker,omitempty"`
	Dialogue    string    `json:"dialogue,omitempty"`
	AudioPath   string    `json:"audio_path,omitempty"`
	ImagePath   string    `json:"image_path,omitempty"`
	DurationSec float64   `json:"duration_sec"`
}

type ReplayScript struct {
	GameID        string      `json:"game_id"`
	GameName      string      `json:"game_name"`
	TotalDuration float64     `json:"total_duration"`
	Beats         []SceneBeat `json:"beats"`
}
