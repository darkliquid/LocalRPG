package scene

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
)

// fakeSource supplies turns and locations from memory, so grouping is testable
// without a filesystem or a database.
type fakeSource struct {
	turns     []engine.Turn
	locations map[string]*entity.Entity
}

func (s *fakeSource) Turns() ([]engine.Turn, error) { return s.turns, nil }

func (s *fakeSource) Location(id string) (*entity.Entity, error) {
	if ent, ok := s.locations[id]; ok {
		return ent, nil
	}
	return nil, fmt.Errorf("location %q not found", id)
}

func turn(number int, location string, segments ...entity.TurnSegment) engine.Turn {
	return engine.Turn{
		Number:    number,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I act",
		Narration: "Something happens.",
		Location:  location,
		Segments:  segments,
	}
}

func twoLocationSource() *fakeSource {
	return &fakeSource{
		turns: []engine.Turn{
			turn(1, "alden-tavern", entity.TurnSegment{Kind: entity.SegmentNarration, Text: "Warm light."}),
			turn(2, "alden-tavern", entity.TurnSegment{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Welcome."}),
			turn(3, "aldon-harbour", entity.TurnSegment{Kind: entity.SegmentNarration, Text: "Salt air."}),
		},
		locations: map[string]*entity.Entity{
			"alden-tavern":  {ID: "alden-tavern", Name: "Alden Tavern", Type: "location"},
			"aldon-harbour": {ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location"},
		},
	}
}

func TestCompileGroupsConsecutiveTurnsByLocation(t *testing.T) {
	compiler := NewCompiler(twoLocationSource())

	script, err := compiler.Compile(context.Background(), "campaign-01", Options{})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(script.Scenes) != 2 {
		t.Fatalf("expected 2 scenes, got %d: %+v", len(script.Scenes), script.Scenes)
	}
	if script.Scenes[0].LocationID != "alden-tavern" || script.Scenes[1].LocationID != "aldon-harbour" {
		t.Errorf("scenes out of order: %+v", script.Scenes)
	}
	if script.Scenes[0].LocationName != "Alden Tavern" {
		t.Errorf("LocationName = %q, want the entity name", script.Scenes[0].LocationName)
	}

	// Each scene opens with its card, then its turns' segments.
	if got := script.Scenes[0].Beats[0]; got.Kind != BeatSceneCard || got.Text != "Alden Tavern" {
		t.Errorf("expected a scene card first, got %+v", got)
	}
	if len(script.Scenes[0].Beats) != 3 {
		t.Errorf("expected 2 segments plus a card, got %+v", script.Scenes[0].Beats)
	}

	beats := script.Beats()
	if beats[1].Kind != BeatNarration || beats[1].Text != "Warm light." {
		t.Errorf("unexpected first segment: %+v", beats[1])
	}
	if beats[2].Kind != BeatSpeech || beats[2].SpeakerID != "garrick" {
		t.Errorf("expected the speech segment to carry its speaker, got %+v", beats[2])
	}
}

func TestCompileKeepsUnlocatedTurnsSeparateAndUncarded(t *testing.T) {
	source := &fakeSource{
		turns: []engine.Turn{
			turn(1, "", entity.TurnSegment{Kind: entity.SegmentNarration, Text: "Before locations existed."}),
			turn(2, "", entity.TurnSegment{Kind: entity.SegmentNarration, Text: "And again."}),
		},
		locations: map[string]*entity.Entity{},
	}

	script, err := NewCompiler(source).Compile(context.Background(), "legacy", Options{})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(script.Scenes) != 2 {
		t.Fatalf("unlocated turns must not merge, got %d scenes", len(script.Scenes))
	}
	for _, sc := range script.Scenes {
		if sc.ArtPath != "" {
			t.Errorf("expected no art without a location, got %q", sc.ArtPath)
		}
		if len(sc.Beats) != 1 || sc.Beats[0].Kind != BeatNarration {
			t.Errorf("expected one narration beat with no card, got %+v", sc.Beats)
		}
	}
}

func TestCompileRejectsAnEmptyCampaign(t *testing.T) {
	script, err := NewCompiler(&fakeSource{}).Compile(context.Background(), "campaign-01", Options{})
	if err == nil {
		t.Fatalf("expected an error for a campaign with no turns, got %+v", script)
	}
}

func TestCompileTotalsDurations(t *testing.T) {
	script, err := NewCompiler(twoLocationSource()).Compile(context.Background(), "campaign-01", Options{})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var sum time.Duration
	for _, sc := range script.Scenes {
		var sceneSum time.Duration
		for _, beat := range sc.Beats {
			sceneSum += beat.Duration
		}
		if sc.Duration != sceneSum {
			t.Errorf("scene %q duration = %v, want %v", sc.LocationID, sc.Duration, sceneSum)
		}
		sum += sc.Duration
	}
	if script.TotalDuration != sum {
		t.Errorf("TotalDuration = %v, want %v", script.TotalDuration, sum)
	}
}

type fakeSpeech struct {
	clips map[string]struct {
		path     string
		duration time.Duration
	}
	unavailable bool
}

func (f *fakeSpeech) SegmentAudio(ctx context.Context, segment entity.TurnSegment) (string, time.Duration, error) {
	if f.unavailable {
		return "", 0, ErrAudioUnavailable
	}
	if clip, ok := f.clips[segment.Text]; ok {
		return clip.path, clip.duration, nil
	}
	return "", 0, ErrAudioUnavailable
}

func TestCompileResolvesAudioAndCountsSilence(t *testing.T) {
	source := twoLocationSource()
	compiler := NewCompiler(source)

	var warnings []string
	compiler.SetSpeechResolver(&fakeSpeech{clips: map[string]struct {
		path     string
		duration time.Duration
	}{
		"Welcome.": {path: "/cache/welcome.wav", duration: 3 * time.Second},
	}})

	script, err := compiler.Compile(context.Background(), "campaign-01", Options{
		Audio:      true,
		OnProgress: func(format string, args ...interface{}) { warnings = append(warnings, fmt.Sprintf(format, args...)) },
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	beats := script.Beats()
	var speech *Beat
	for i := range beats {
		if beats[i].Kind == BeatSpeech {
			speech = &beats[i]
		}
	}
	if speech == nil || speech.AudioPath != "/cache/welcome.wav" {
		t.Fatalf("expected the clip on the speech beat, got %+v", speech)
	}
	if speech.Duration != 3*time.Second+BeatGap {
		t.Errorf("speech duration = %v, want the clip length plus the gap", speech.Duration)
	}

	// Beats without a clip stay silent and are reported once.
	if len(warnings) != 1 || !strings.Contains(warnings[0], "silently") {
		t.Errorf("expected one silence warning, got %v", warnings)
	}
}

func TestCompileWithoutSpeechResolverIsSilent(t *testing.T) {
	script, err := NewCompiler(twoLocationSource()).Compile(context.Background(), "campaign-01", Options{Audio: true})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	for _, beat := range script.Beats() {
		if beat.AudioPath != "" {
			t.Errorf("expected silence without a resolver, got %+v", beat)
		}
	}
}

func TestCompileWarnsWhenAResolverReportsNoAudio(t *testing.T) {
	compiler := NewCompiler(twoLocationSource())
	compiler.SetSpeechResolver(&fakeSpeech{unavailable: true})

	var warnings []string
	script, err := compiler.Compile(context.Background(), "campaign-01", Options{
		Audio:      true,
		OnProgress: func(format string, args ...interface{}) { warnings = append(warnings, fmt.Sprintf(format, args...)) },
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	for _, beat := range script.Beats() {
		if beat.AudioPath != "" {
			t.Errorf("expected silence when audio is unavailable, got %+v", beat)
		}
	}
	if len(warnings) != 1 {
		t.Errorf("expected exactly one silence warning, got %v", warnings)
	}
}
