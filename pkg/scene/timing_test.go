package scene

import (
	"strings"
	"testing"
	"time"
)

func TestReadingDurationFollowsWordRate(t *testing.T) {
	// 180 words per minute means three words a second.
	text := strings.Repeat("word ", 180)

	got := ReadingDuration(text)
	if got < 59*time.Second || got > 61*time.Second {
		t.Errorf("ReadingDuration = %v, want about a minute for 180 words", got)
	}
}

func TestReadingDurationClampsToTheFloor(t *testing.T) {
	for _, text := range []string{"Wait.", "", "   ", "Three small words"} {
		if got := ReadingDuration(text); got != MinimumBeatDuration {
			t.Errorf("ReadingDuration(%q) = %v, want the %v floor", text, got, MinimumBeatDuration)
		}
	}
}

func TestReadingDurationUsesCharactersForUnspacedScripts(t *testing.T) {
	// Forty characters with no spaces would read as a single word, which would
	// collapse to the floor; the character rate gives it real time instead.
	text := strings.Repeat("語", 40)

	got := ReadingDuration(text)
	if got <= MinimumBeatDuration {
		t.Fatalf("ReadingDuration(%q) = %v, want more than the floor", text, got)
	}
	if got < 3*time.Second || got > 5*time.Second {
		t.Errorf("ReadingDuration = %v, want about four seconds for 40 characters", got)
	}
}

func TestBeatDurationPrefersAudio(t *testing.T) {
	short := Beat{Text: strings.Repeat("word ", 180), AudioDuration: 2 * time.Second}
	if got := BeatDuration(short); got != 2*time.Second+BeatGap {
		t.Errorf("BeatDuration = %v, want the clip length plus the gap", got)
	}

	silent := Beat{Text: strings.Repeat("word ", 180)}
	want := ReadingDuration(silent.Text) + BeatGap
	if got := BeatDuration(silent); got != want {
		t.Errorf("BeatDuration = %v, want the reading estimate plus the gap (%v)", got, want)
	}
}

func TestFramesForRoundsUpToAtLeastOne(t *testing.T) {
	cases := []struct {
		duration time.Duration
		fps      int
		want     int
	}{
		{duration: 2 * time.Second, fps: 15, want: 30},
		{duration: 1 * time.Second, fps: 15, want: 15},
		{duration: 100 * time.Millisecond, fps: 15, want: 2},
		{duration: 10 * time.Millisecond, fps: 15, want: 1},
		{duration: 2 * time.Second, fps: 0, want: 2 * DefaultFPS},
	}

	for _, tc := range cases {
		if got := FramesFor(tc.duration, tc.fps); got != tc.want {
			t.Errorf("FramesFor(%v, %d) = %d, want %d", tc.duration, tc.fps, got, tc.want)
		}
	}
}
