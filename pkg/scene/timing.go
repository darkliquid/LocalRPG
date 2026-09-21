package scene

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// ReadingWordsPerMinute sits below the ~200wpm adult average deliberately: a
	// viewer cannot scroll back to re-read a beat that has passed.
	ReadingWordsPerMinute = 180
	// ReadingCharactersPerMinute covers scripts that do not separate words, where
	// counting fields under-measures badly.
	ReadingCharactersPerMinute = 600
	// MinimumBeatDuration keeps a three-word line on screen long enough to register.
	MinimumBeatDuration = 2 * time.Second
	// BeatGap separates consecutive beats perceptually.
	BeatGap = 400 * time.Millisecond
	// TypewriterFraction is the share of a beat spent revealing its text; the rest
	// is the viewer's.
	TypewriterFraction = 0.6
	// DefaultFPS is the frame rate used when a caller asks for none.
	DefaultFPS = 15
)

// ReadingDuration estimates the time a viewer needs for text, never below the
// floor. Word counting drives spaced scripts; unspaced scripts fall back to a
// character rate, which is why two constants exist rather than one.
func ReadingDuration(text string) time.Duration {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return MinimumBeatDuration
	}

	var minutes float64
	if fields := strings.Fields(trimmed); len(fields) >= 3 {
		minutes = float64(len(fields)) / ReadingWordsPerMinute
	} else {
		minutes = float64(utf8.RuneCountInString(trimmed)) / ReadingCharactersPerMinute
	}

	if duration := time.Duration(minutes * float64(time.Minute)); duration > MinimumBeatDuration {
		return duration
	}
	return MinimumBeatDuration
}

// BeatDuration is how long a beat is shown: a clip's real length when one exists,
// otherwise the reading estimate, plus the gap. Pacing follows the audio whenever
// there is audio and the text whenever there is not.
func BeatDuration(beat Beat) time.Duration {
	if beat.AudioDuration > 0 {
		return beat.AudioDuration + BeatGap
	}
	return ReadingDuration(beat.Text) + BeatGap
}

// FramesFor is how many frames a duration occupies at a frame rate, never zero,
// so a very short beat still renders one frame.
func FramesFor(duration time.Duration, fps int) int {
	if fps <= 0 {
		fps = DefaultFPS
	}

	frames := int(math.Round(duration.Seconds() * float64(fps)))
	if frames < 1 {
		return 1
	}
	return frames
}
