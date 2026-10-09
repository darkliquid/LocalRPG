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
// AudioPaths is ordered, one clip per sentence of reduced text. PortraitPath is a
// character's portrait, which the theatre shows beside the dialogue and Player
// marks the protagonist's own line.
type Beat struct {
	Kind          BeatKind
	TurnNumber    int
	Speaker       string
	SpeakerID     string
	Text          string
	ArtPath       string
	PortraitPath  string
	Player        bool
	AudioPaths    []string
	AudioDuration time.Duration
	Duration      time.Duration
}

// Scene groups the beats that happened in one place.
type Scene struct {
	LocationID   string
	LocationName string
	ArtPath      string
	// Layers, when present, are the scene's art split by depth, ordered back to
	// front. A renderer parallaxes them; a scene with no layers is a flat image
	// and draws as it always did.
	Layers   []SceneLayer
	Beats    []Beat
	Duration time.Duration
}

// SceneLayer is one depth of a scene's art. Depth runs from 0 at the back to 1 at
// the front, so the background moves least and the foreground most.
type SceneLayer struct {
	Depth float64
	Art   string
}

// ClipGroup is one clip a run of adjacent same-speaker beats shares, so the
// exporter resolves and plays it once rather than once per covered beat. The
// group's clip is carried by the first of its beats.
type ClipGroup struct {
	Key            string
	AudioPaths     []string
	Duration       time.Duration
	SegmentIndexes []int
}

// Script is the whole export.
type Script struct {
	GameID   string
	GameName string

	// PlayerPortrait is the protagonist's portrait, which the theatre keeps on
	// stage for the whole story rather than per beat, and PlayerName is the label
	// that goes under it.
	PlayerPortrait string
	PlayerName     string

	// Banner is the campaign's own image, which the theatre shows behind everything
	// when a scene has no art of its own.
	Banner string

	// WorldStyle is the world's art style and genre as one string, and Genre is the
	// world's genre alone, which the no-art background is tinted by.
	WorldStyle    string
	Genre         string
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

// DisplayMode is how the prose grammar treats a performance direction such as
// "[whispering]": the theatre's setting, mirrored from the campaign's speech cues.
type DisplayMode string

const (
	// DisplayStageDirections shows a direction as a styled pill.
	DisplayStageDirections DisplayMode = "stage_directions"
	// DisplayHidden strips a direction from the prose.
	DisplayHidden DisplayMode = "hidden"
	// DisplayRaw shows a direction verbatim.
	DisplayRaw DisplayMode = "raw"
)
