package scene

import (
	"context"
	"errors"
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
		paths    []string
		duration time.Duration
	}
	// unspoken lists the texts that reduce to nothing and are never spoken.
	unspoken    []string
	unavailable bool
}

func (f *fakeSpeech) SegmentAudio(ctx context.Context, segment entity.TurnSegment) ([]string, time.Duration, error) {
	if f.unavailable {
		return nil, 0, ErrAudioUnavailable
	}
	for _, text := range f.unspoken {
		if segment.Text == text {
			return nil, 0, ErrNoSpeakableText
		}
	}
	if clip, ok := f.clips[segment.Text]; ok {
		return clip.paths, clip.duration, nil
	}
	return nil, 0, ErrAudioUnavailable
}

// silenceWarning picks the report about beats with no clip, so a test is not disturbed by
// the other lines an export reports.
func silenceWarning(warnings []string) string {
	for _, warning := range warnings {
		if strings.Contains(warning, "no audio clip") {
			return warning
		}
	}
	return ""
}

func TestCompileResolvesAudioAndCountsSilence(t *testing.T) {
	source := twoLocationSource()
	compiler := NewCompiler(source)

	var warnings []string
	compiler.SetSpeechResolver(&fakeSpeech{clips: map[string]struct {
		paths    []string
		duration time.Duration
	}{
		"Welcome.": {paths: []string{"/cache/welcome.wav"}, duration: 3 * time.Second},
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
	if speech == nil || len(speech.AudioPaths) != 1 || speech.AudioPaths[0] != "/cache/welcome.wav" {
		t.Fatalf("expected the clip on the speech beat, got %+v", speech)
	}
	if speech.Duration != 3*time.Second+BeatGap {
		t.Errorf("speech duration = %v, want the clip length plus the gap", speech.Duration)
	}

	// Beats without a clip stay silent and are reported once.
	if got := silenceWarning(warnings); got == "" || !strings.Contains(got, "silently") {
		t.Errorf("expected a silence report, got %v", warnings)
	}
}

func TestCompileWithoutSpeechResolverIsSilent(t *testing.T) {
	script, err := NewCompiler(twoLocationSource()).Compile(context.Background(), "campaign-01", Options{Audio: true})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	for _, beat := range script.Beats() {
		if len(beat.AudioPaths) != 0 {
			t.Errorf("expected silence without a resolver, got %+v", beat)
		}
	}
}

func TestCompileKeepsEveryClipOfABeatInOrder(t *testing.T) {
	source := twoLocationSource()
	compiler := NewCompiler(source)
	compiler.SetSpeechResolver(&fakeSpeech{clips: map[string]struct {
		paths    []string
		duration time.Duration
	}{
		"Welcome.": {paths: []string{"/cache/welcome-1.wav", "/cache/welcome-2.wav"}, duration: 3 * time.Second},
	}})

	script, err := compiler.Compile(context.Background(), "campaign-01", Options{Audio: true})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var found bool
	for _, beat := range script.Beats() {
		if beat.Text != "Welcome." {
			continue
		}
		found = true
		if len(beat.AudioPaths) != 2 || beat.AudioPaths[0] != "/cache/welcome-1.wav" || beat.AudioPaths[1] != "/cache/welcome-2.wav" {
			t.Errorf("AudioPaths = %#v, want both clips in order", beat.AudioPaths)
		}
		if beat.AudioDuration != 3*time.Second {
			t.Errorf("AudioDuration = %v, want the summed clip duration", beat.AudioDuration)
		}
		if want := 3*time.Second + BeatGap; beat.Duration != want {
			t.Errorf("Duration = %v, want %v", beat.Duration, want)
		}
	}
	if !found {
		t.Fatal("the fixture no longer contains the audio beat")
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
		if len(beat.AudioPaths) != 0 {
			t.Errorf("expected silence when audio is unavailable, got %+v", beat)
		}
	}
	if got := silenceWarning(warnings); got == "" {
		t.Errorf("expected a silence report, got %v", warnings)
	}
}

// fakePortraits resolves a character to a portrait path from memory.
type fakePortraits struct{ paths map[string]string }

func (f *fakePortraits) Portrait(_ context.Context, characterID string) (string, error) {
	if path, ok := f.paths[characterID]; ok {
		return path, nil
	}
	return "", ErrAudioUnavailable
}

// The theatre keeps the protagonist on stage and shows the speaker's face beside
// the line, so a compiled script has to carry both.
func TestCompileCarriesPortraitsAndThePlayerFlag(t *testing.T) {
	source := &fakeSource{
		turns: []engine.Turn{
			turn(1, "alden-tavern",
				entity.TurnSegment{Kind: entity.SegmentSpeech, Speaker: "Sean", SpeakerID: "sean", Text: "Hello.", Player: true},
				entity.TurnSegment{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Welcome."},
				entity.TurnSegment{Kind: entity.SegmentNarration, Text: "Warm light."},
			),
		},
		locations: map[string]*entity.Entity{
			"alden-tavern": {ID: "alden-tavern", Name: "Alden Tavern", Type: "location"},
		},
	}

	compiler := NewCompiler(source)
	compiler.SetPortraitResolver(&fakePortraits{paths: map[string]string{
		"sean":    "/cache/portrait-sean.svg",
		"garrick": "/cache/portrait-garrick.svg",
	}})

	script, err := compiler.Compile(context.Background(), "campaign-01", Options{PlayerID: "sean"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if script.PlayerPortrait != "/cache/portrait-sean.svg" {
		t.Errorf("PlayerPortrait = %q, want the protagonist's", script.PlayerPortrait)
	}

	var speeches int
	for _, beat := range script.Beats() {
		switch {
		case beat.Kind == BeatSpeech && beat.SpeakerID == "sean":
			speeches++
			if !beat.Player {
				t.Errorf("the protagonist's line lost its player flag: %+v", beat)
			}
			if beat.PortraitPath != "/cache/portrait-sean.svg" {
				t.Errorf("sean's portrait = %q", beat.PortraitPath)
			}
		case beat.Kind == BeatSpeech && beat.SpeakerID == "garrick":
			speeches++
			if beat.Player {
				t.Errorf("an NPC line carries the player flag: %+v", beat)
			}
			if beat.PortraitPath != "/cache/portrait-garrick.svg" {
				t.Errorf("garrick's portrait = %q", beat.PortraitPath)
			}
		case beat.Kind == BeatNarration && beat.PortraitPath != "":
			t.Errorf("narration gained a portrait: %+v", beat)
		}
	}
	if speeches != 2 {
		t.Fatalf("speech beats = %d, want both lines", speeches)
	}
}

func TestCompileWithoutAPortraitResolverCarriesNone(t *testing.T) {
	script, err := NewCompiler(twoLocationSource()).Compile(context.Background(), "campaign-01", Options{PlayerID: "sean"})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if script.PlayerPortrait != "" {
		t.Errorf("PlayerPortrait = %q, want none without a resolver", script.PlayerPortrait)
	}
	for _, beat := range script.Beats() {
		if beat.PortraitPath != "" {
			t.Errorf("beat gained a portrait without a resolver: %+v", beat)
		}
	}
}

// failingSpeech reports a synthesis failure, so the export can say why it is silent.
type failingSpeech struct{ err error }

func (f *failingSpeech) SegmentAudio(context.Context, entity.TurnSegment) ([]string, time.Duration, error) {
	return nil, 0, f.err
}

// A bundle that cannot speak should say why it is quiet: the reason is the difference
// between a campaign with no clips and a provider that failed.
func TestCompileReportsWhyBeatsAreSilent(t *testing.T) {
	compiler := NewCompiler(twoLocationSource())
	compiler.SetSpeechResolver(&failingSpeech{err: errors.New("provider returned 429")})

	var warnings []string
	if _, err := compiler.Compile(context.Background(), "campaign-01", Options{
		Audio:      true,
		OnProgress: func(format string, args ...interface{}) { warnings = append(warnings, fmt.Sprintf(format, args...)) },
	}); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	got := silenceWarning(warnings)
	if got == "" {
		t.Fatalf("warnings = %v, want a silence report", warnings)
	}
	for _, want := range []string{"no audio clip", "provider returned 429"} {
		if !strings.Contains(got, want) {
			t.Errorf("report %q does not mention %q", got, want)
		}
	}
}

// Asking for audio with no provider is the commonest silent bundle, so it is named too.
func TestCompileReportsAMissingSpeechProvider(t *testing.T) {
	var warnings []string
	if _, err := NewCompiler(twoLocationSource()).Compile(context.Background(), "campaign-01", Options{
		Audio:      true,
		OnProgress: func(format string, args ...interface{}) { warnings = append(warnings, fmt.Sprintf(format, args...)) },
	}); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if got := silenceWarning(warnings); !strings.Contains(got, "no TTS provider is configured") {
		t.Fatalf("warnings = %v, want the missing provider named", warnings)
	}
}

// A bundle whose characters are silent looks the same as one whose narration is, unless the
// export says which, so coverage is reported by kind.
func TestCompileReportsAudioCoverageByKind(t *testing.T) {
	compiler := NewCompiler(twoLocationSource())
	compiler.SetSpeechResolver(&fakeSpeech{clips: map[string]struct {
		paths    []string
		duration time.Duration
	}{
		"Welcome.": {paths: []string{"/cache/welcome.wav"}, duration: time.Second},
	}})

	var warnings []string
	if _, err := compiler.Compile(context.Background(), "campaign-01", Options{
		Audio:      true,
		OnProgress: func(format string, args ...interface{}) { warnings = append(warnings, fmt.Sprintf(format, args...)) },
	}); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var coverage string
	for _, warning := range warnings {
		if strings.HasPrefix(warning, "audio:") {
			coverage = warning
		}
	}
	if coverage == "" {
		t.Fatalf("no coverage reported: %v", warnings)
	}
	for _, want := range []string{"speech 1/1", "narration 0/2"} {
		if !strings.Contains(coverage, want) {
			t.Errorf("coverage %q does not say %q", coverage, want)
		}
	}
}

// A scene card is a title, not a line: it is never spoken, needs no clip, and must not be
// reported as a beat that is missing one.
func TestCompileCountsSceneCardsApartFromSpokenBeats(t *testing.T) {
	compiler := NewCompiler(twoLocationSource())
	compiler.SetSpeechResolver(&fakeSpeech{clips: map[string]struct {
		paths    []string
		duration time.Duration
	}{
		"Welcome.": {paths: []string{"/cache/welcome.wav"}, duration: time.Second},
	}})

	var coverage string
	if _, err := compiler.Compile(context.Background(), "campaign-01", Options{
		Audio: true,
		OnProgress: func(format string, args ...interface{}) {
			if line := fmt.Sprintf(format, args...); strings.HasPrefix(line, "audio:") {
				coverage = line
			}
		},
	}); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// The fixture has two scenes, so two cards, and three beats that can speak. The cards
	// are not spoken beats, so the total is three and they are not mentioned at all.
	if !strings.Contains(coverage, "1 of 3 spoken beats") {
		t.Errorf("coverage = %q, want the beats that can speak counted without the cards", coverage)
	}
	if strings.Contains(coverage, "scene_card") || strings.Contains(coverage, "card") {
		t.Errorf("coverage = %q, want no card counted as a beat or mentioned", coverage)
	}

	// A card is never asked for a clip, so nothing is generated for one.
	for _, beat := range mustScript(t, compiler).Beats() {
		if beat.Kind == BeatSceneCard && len(beat.AudioPaths) != 0 {
			t.Errorf("a scene card carries clips: %+v", beat)
		}
	}
}

// mustScript compiles the fixture and returns its script.
func mustScript(t *testing.T, compiler *Compiler) *Script {
	t.Helper()
	compiler.SetSpeechResolver(&fakeSpeech{clips: map[string]struct {
		paths    []string
		duration time.Duration
	}{
		"Welcome.": {paths: []string{"/cache/welcome.wav"}, duration: time.Second},
	}})
	script, err := compiler.Compile(context.Background(), "campaign-01", Options{Audio: true})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	return script
}

// A beat whose text reduces to nothing - a stage direction, say - is deliberately never
// spoken: it is not a beat that needs a clip, so it is not counted among spoken beats and its
// silence is not reported.
func TestCompileLeavesUnspokenBeatsOutOfTheCounts(t *testing.T) {
	compiler := NewCompiler(twoLocationSource())
	compiler.SetSpeechResolver(&fakeSpeech{
		clips: map[string]struct {
			paths    []string
			duration time.Duration
		}{
			"Welcome.":  {paths: []string{"/cache/welcome.wav"}, duration: time.Second},
			"Salt air.": {paths: []string{"/cache/salt.wav"}, duration: time.Second},
		},
		// The one beat that is never spoken.
		unspoken: []string{"Warm light."},
	})

	var coverage string
	var warnings []string
	if _, err := compiler.Compile(context.Background(), "campaign-01", Options{
		Audio: true,
		OnProgress: func(format string, args ...interface{}) {
			line := fmt.Sprintf(format, args...)
			if strings.HasPrefix(line, "audio:") {
				coverage = line
			}
			warnings = append(warnings, line)
		},
	}); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// The fixture has two narration beats, one of which is never spoken, and one speech
	// beat: two spoken beats in total, and both have clips.
	if !strings.Contains(coverage, "2 of 2 spoken beats") {
		t.Errorf("coverage = %q, want only the beats that can speak counted", coverage)
	}
	if !strings.Contains(coverage, "narration 1/1, speech 1/1") {
		t.Errorf("coverage = %q, want the kind split without the unspoken beat", coverage)
	}
	if got := silenceWarning(warnings); got != "" {
		t.Errorf("an unspoken beat was reported as silent: %q", got)
	}
}
