// Package scene owns the shape of an exported campaign: the beats to play, how
// long each lasts, and which art and audio belong to it. Every renderer reads
// this model rather than deriving its own.
package scene

import (
	"strings"
	"time"
)

// BeatKind distinguishes what a beat is presenting.
type BeatKind string

const (
	// BeatSceneCard introduces a location: full-frame art and its name.
	BeatSceneCard BeatKind = "scene_card"
	// BeatNarration is prose in the narrator's voice.
	BeatNarration BeatKind = "narration"
	// BeatSpeech is an attributed line.
	BeatSpeech BeatKind = "speech"
)

// Beat is one unit of playback: a span of text, its imagery, and its audio.
type Beat struct {
	Kind          BeatKind
	TurnNumber    int
	Speaker       string
	SpeakerID     string
	Text          string
	ArtPath       string
	AudioPath     string
	AudioDuration time.Duration
	Duration      time.Duration
}

// Scene groups the beats that happened in one place.
type Scene struct {
	LocationID   string
	LocationName string
	ArtPath      string
	Beats        []Beat
	Duration     time.Duration
}

// Script is the whole export.
type Script struct {
	GameID        string
	GameName      string
	WorldStyle    string
	Scenes        []Scene
	TotalDuration time.Duration
}

// Beats flattens the scenes for renderers that walk a single sequence.
func (s Script) Beats() []Beat {
	total := 0
	for _, sc := range s.Scenes {
		total += len(sc.Beats)
	}

	beats := make([]Beat, 0, total)
	for _, sc := range s.Scenes {
		beats = append(beats, sc.Beats...)
	}
	return beats
}

// SceneCard is the beat that introduces a location, so both renderers agree on
// its text and duration.
func SceneCard(s Scene) Beat {
	text := strings.TrimSpace(s.LocationName)
	if text == "" {
		text = "Somewhere new"
	}

	beat := Beat{
		Kind:    BeatSceneCard,
		Text:    text,
		ArtPath: s.ArtPath,
	}
	beat.Duration = BeatDuration(beat)
	return beat
}
