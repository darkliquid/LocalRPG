package export

import (
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type SceneBeat struct {
	TurnNumber  int       `json:"turn_number"`
	Timestamp   time.Time `json:"timestamp"`
	Mode        string    `json:"mode"`
	PlayerInput string    `json:"player_input"`
	Prose       string    `json:"prose"`
	// Segments is the ordered playback script: narration and attributed speech.
	Segments    []entity.TurnSegment `json:"segments,omitempty"`
	ImagePath   string               `json:"image_path,omitempty"`
	DurationSec float64              `json:"duration_sec"`
}

type ReplayScript struct {
	GameID        string      `json:"game_id"`
	GameName      string      `json:"game_name"`
	TotalDuration float64     `json:"total_duration"`
	Beats         []SceneBeat `json:"beats"`
}
