package scene

import (
	"math/rand"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestCaptionsEmitSpokenCuesOnly(t *testing.T) {
	beats := []Beat{
		{Kind: BeatSpeech, Speaker: "Garrick", Text: "Keep walking.", Duration: 2 * time.Second},
		{Kind: BeatNarration, Text: "The hall is quiet.", Duration: 2 * time.Second},
		{Kind: BeatSceneCard, Text: "Alden Tavern", Duration: 2 * time.Second},
	}
	got := Captions(beats)
	if !strings.HasPrefix(got, "WEBVTT") {
		t.Fatalf("vtt = %q, want a WEBVTT header", got)
	}
	if !strings.Contains(got, "<v Garrick>Keep walking.") {
		t.Fatalf("vtt = %q, want the spoken line with a voice tag", got)
	}
	if strings.Contains(got, "hall is quiet") || strings.Contains(got, "Alden Tavern") {
		t.Fatalf("vtt = %q, narration and cards should not be cues", got)
	}
}

func TestCaptionsAreWellFormed(t *testing.T) {
	beats := []Beat{
		{Kind: BeatSpeech, Speaker: "Garrick", Text: "One.", Duration: 90 * time.Minute},
		{Kind: BeatSpeech, Speaker: "Evelyn", Text: "Two.", Duration: time.Second},
	}
	got := Captions(beats)
	timestamps := regexp.MustCompile(`\d{2}:\d{2}:\d{2}\.\d{3} --> \d{2}:\d{2}:\d{2}\.\d{3}`).FindAllString(got, -1)
	if len(timestamps) != 2 {
		t.Fatalf("vtt = %q, want two timed cues", got)
	}
	if !strings.Contains(got, "01:30:00.000") {
		t.Fatalf("vtt = %q, want hours in the timestamp", got)
	}
}

// TestCaptionCueMatchesTheBeatProperty generalises the cue rules to arbitrary
// beat sequences: the cues are one per spoken beat, in order, each carrying the
// beat's text at the beat's own start time. It is the parity guard between the
// WebVTT track and the theatre's caption for the same beat.
func TestCaptionCueMatchesTheBeatProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 300; i++ {
		beats := randomCaptionBeats(rng)
		vtt := Captions(beats)
		cues := parseCues(vtt)

		start := time.Duration(0)
		spoken := 0
		for _, beat := range beats {
			if beat.Kind == BeatSpeech && strings.TrimSpace(beat.Text) != "" {
				if spoken >= len(cues) {
					t.Fatalf("case %d: missing cue for %+v", i, beat)
				}
				cue := cues[spoken]
				if !strings.Contains(cue.text, strings.TrimSpace(beat.Text)) {
					t.Fatalf("case %d: cue %d = %q, want the beat text %q", i, spoken, cue.text, beat.Text)
				}
				if cue.start != start {
					t.Fatalf("case %d: cue %d starts at %v, want %v", i, spoken, cue.start, start)
				}
				if cue.end != start+beat.Duration {
					t.Fatalf("case %d: cue %d ends at %v, want %v", i, spoken, cue.end, start+beat.Duration)
				}
				spoken++
			}
			start += beat.Duration
		}
		if spoken != len(cues) {
			t.Fatalf("case %d: %d cues for %d spoken beats", i, len(cues), spoken)
		}
	}
}

type vttCue struct {
	start time.Duration
	end   time.Duration
	text  string
}

var cueTimingPattern = regexp.MustCompile(`^(\d{2}):(\d{2}):(\d{2})\.(\d{3}) --> (\d{2}):(\d{2}):(\d{2})\.(\d{3})$`)

func parseCues(vtt string) []vttCue {
	blocks := strings.Split(vtt, "\n\n")
	cues := make([]vttCue, 0, len(blocks))
	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 3 {
			continue
		}
		match := cueTimingPattern.FindStringSubmatch(lines[1])
		if match == nil {
			continue
		}
		cues = append(cues, vttCue{
			start: parseVTTDuration(match[1:5]),
			end:   parseVTTDuration(match[5:9]),
			text:  strings.Join(lines[2:], " "),
		})
	}
	return cues
}

func parseVTTDuration(parts []string) time.Duration {
	toInt := func(s string) int {
		n := 0
		for _, r := range s {
			n = n*10 + int(r-'0')
		}
		return n
	}
	return time.Duration(toInt(parts[0]))*time.Hour +
		time.Duration(toInt(parts[1]))*time.Minute +
		time.Duration(toInt(parts[2]))*time.Second +
		time.Duration(toInt(parts[3]))*time.Millisecond
}

// randomCaptionBeats builds an arbitrary sequence of narration, speech, and card
// beats, including silent speech and beats with no text.
func randomCaptionBeats(rng *rand.Rand) []Beat {
	kinds := []BeatKind{BeatNarration, BeatSpeech, BeatSceneCard}
	words := []string{"the", "hall", "is", "quiet", "keep", "walking"}
	beats := make([]Beat, 0, 20)
	for i := 0; i < rng.Intn(21); i++ {
		kind := kinds[rng.Intn(len(kinds))]
		parts := make([]string, 0, 6)
		for j := 0; j < rng.Intn(6); j++ {
			parts = append(parts, words[rng.Intn(len(words))])
		}
		beat := Beat{Kind: kind, Text: strings.Join(parts, " ")}
		if kind == BeatSpeech {
			beat.Speaker = "Garrick"
		}
		beat.Duration = time.Duration(500+rng.Intn(4000)) * time.Millisecond
		beats = append(beats, beat)
	}
	return beats
}
