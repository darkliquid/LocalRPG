# Animated Story Export and Video Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the click-through replay bundle and the silent still-image video with an animated visual-novel player and a video renderer that produce the same experience from one scene model, grouped by location and paced by reading time.

**Architecture:** A new `pkg/scene` package owns the only answer to "what are the beats, how long is each, what art belongs to them": `Script`/`Scene`/`Beat`, a reading-time estimator, grouping by location, and a Go rasteriser for video frames. The two exporters become renderers of that model — the web one emitting a self-contained bundle with sidecar assets, the video one emitting a frame sequence muxed against per-clip audio.

**Tech Stack:** Go 1.27.1, `golang.org/x/image` (fonts, drawing, WebP decoding), `modernc.org/sqlite`, FFmpeg and ffprobe, `mise` tasks.

**Spec:** `docs/superpowers/specs/2026-09-21-animated-export-and-video-design.md`

## Global Constraints

- `pkg/scene` is the only place that decides beat shape, duration, or scene grouping. No renderer re-derives them.
- Pacing is reading-time estimation with a floor, never fixed spans. A clip's real length wins whenever a clip exists.
- The exported bundle is self-contained: no external URLs, no webfonts, no server. Relative asset paths only.
- Video frames are drawn in Go. No headless browser, no Node, no external rasteriser.
- A missing asset, unreadable clip, or failed probe degrades one beat and never fails an export.
- `golang.org/x/image` is pinned to `v0.46.0` (present in the local module cache, so `go get` needs no network).
- Tests use the standard library only (`testing`, `t.TempDir()`); no testify. Use `interface{}`, never `any`.
- Tests that need FFmpeg or ffprobe skip themselves when the binary is absent (`exec.LookPath`).
- `go vet ./...`, `go test -count=1 ./...`, and `cd frontend && npx tsc --noEmit` must pass. Every commit must build standalone: `git worktree add --detach /tmp/verify <sha> && (cd /tmp/verify && go build ./...)`.

## Scope & Splitting

Phases 1 and 2 are the model and compilation: nothing renders, and they are the prerequisite for everything else. Phase 3 (the web player) ships a complete, useful result on its own. Phases 4 and 5 (rasteriser, video) are independent of Phase 3 and can be taken in either order after Phase 2. Phase 6 is surface and cleanup. This plan assumes the attribution, location, and playback work has shipped: it consumes `Turn.Location`, `Turn.Segments`, `media.GenerateLocationImage`, `media.AppearanceHash`, and `TTSPipeline.SynthesizeSegment`.

---

## Phase 1: The Scene Model

### Task 1: The types every renderer reads

**Files:**
- Create: `pkg/scene/scene.go`, `pkg/scene/scene_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `scene.Script`, `scene.Scene`, `scene.Beat`, `scene.BeatKind`, `scene.BeatSceneCard`/`BeatNarration`/`BeatSpeech`, `scene.SceneCard(Scene) Beat`, `(Script).Beats() []Beat`

- [x] **Step 1: Write the failing test**

`pkg/scene/scene_test.go`:

```go
package scene

import (
	"testing"
	"time"
)

func TestScriptFlattensScenesInOrder(t *testing.T) {
	script := Script{
		GameID: "campaign-01",
		Scenes: []Scene{
			{LocationID: "alden-tavern", LocationName: "Alden Tavern", Beats: []Beat{
				{Kind: BeatNarration, Text: "Warm light."},
				{Kind: BeatSpeech, Text: "Welcome.", Speaker: "Garrick"},
			}},
			{LocationID: "aldon-harbour", LocationName: "Aldon Harbour", Beats: []Beat{
				{Kind: BeatNarration, Text: "Salt air."},
			}},
		},
	}

	beats := script.Beats()
	if len(beats) != 3 {
		t.Fatalf("expected 3 beats, got %d", len(beats))
	}
	if beats[0].Text != "Warm light." || beats[2].Text != "Salt air." {
		t.Errorf("beats are out of order: %+v", beats)
	}
}

func TestSceneCardCarriesTheLocationName(t *testing.T) {
	card := SceneCard(Scene{LocationID: "alden-tavern", LocationName: "Alden Tavern", ArtPath: "/tmp/tavern.svg"})

	if card.Kind != BeatSceneCard {
		t.Errorf("Kind = %q, want scene_card", card.Kind)
	}
	if card.Text != "Alden Tavern" || card.ArtPath != "/tmp/tavern.svg" {
		t.Errorf("unexpected card: %+v", card)
	}
	if card.Duration < MinimumBeatDuration {
		t.Errorf("Duration = %v, want at least the floor", card.Duration)
	}
}

func TestSceneCardFallsBackWhenANameIsMissing(t *testing.T) {
	card := SceneCard(Scene{LocationID: "opening-scene"})

	if card.Text == "" {
		t.Errorf("expected a placeholder name for an unnamed location")
	}
	if card.Duration < MinimumBeatDuration {
		t.Errorf("Duration = %v, want at least the floor", card.Duration)
	}
}

func TestBeatsForAnEmptyScript(t *testing.T) {
	script := Script{GameID: "campaign-01"}
	if got := script.Beats(); len(got) != 0 {
		t.Errorf("expected no beats, got %+v", got)
	}
	if script.TotalDuration != time.Duration(0) {
		t.Errorf("expected no duration, got %v", script.TotalDuration)
	}
}
```

`MinimumBeatDuration` arrives in Task 2; this test is the reason the two tasks ship together in one phase.

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 ./pkg/scene/`
Expected: FAIL — package does not exist.

- [x] **Step 3: Implement**

Create `pkg/scene/scene.go`:

```go
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
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/scene/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/scene/scene.go pkg/scene/scene_test.go
git commit -m "feat(scene): define the shape every export renderer reads"
```

### Task 2: Reading-time pacing and frame arithmetic

**Files:**
- Create: `pkg/scene/timing.go`, `pkg/scene/timing_test.go`

**Interfaces:**
- Consumes: `scene.Beat` (Task 1)
- Produces: `scene.ReadingWordsPerMinute`, `ReadingCharactersPerMinute`, `MinimumBeatDuration`, `BeatGap`, `TypewriterFraction`, `DefaultFPS`, `ReadingDuration(string) time.Duration`, `BeatDuration(Beat) time.Duration`, `FramesFor(time.Duration, int) int`

- [x] **Step 1: Write the failing test**

`pkg/scene/timing_test.go`:

```go
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
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestReadingDuration|TestBeatDuration|TestFramesFor" -count=1 ./pkg/scene/`
Expected: FAIL — `undefined: ReadingDuration`

- [x] **Step 3: Implement**

Create `pkg/scene/timing.go`:

```go
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
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/scene/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/scene/timing.go pkg/scene/timing_test.go
git commit -m "feat(scene): pace beats by reading time instead of fixed spans"
```

---

## Phase 2: Compilation

### Task 3: Group a campaign into location-keyed scenes

**Files:**
- Create: `pkg/scene/compile.go`, `pkg/scene/compile_test.go`

**Interfaces:**
- Consumes: `engine.Turn`, `entity.TurnSegment`, `entity.Mention`
- Produces: `scene.Source`, `scene.Compiler`, `scene.NewCompiler(Source) *Compiler`, `scene.Options`, `(*Compiler).Compile(ctx context.Context, gameID string, opts Options) (*Script, error)`

- [x] **Step 1: Write the failing test**

`pkg/scene/compile_test.go`:

```go
package scene

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/engine"
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
			"alden-tavern": {ID: "alden-tavern", Name: "Alden Tavern", Type: "location"},
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
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestCompile -count=1 ./pkg/scene/`
Expected: FAIL — `undefined: NewCompiler`

- [x] **Step 3: Implement**

Create `pkg/scene/compile.go`:

```go
package scene

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/engine"
)

// Source supplies the campaign facts compilation needs, which keeps grouping and
// resolution testable without a filesystem or a database.
type Source interface {
	Turns() ([]engine.Turn, error)
	Location(id string) (*entity.Entity, error)
}

// Options controls what compilation resolves. Art and audio are decoration: a
// failure to resolve either degrades a beat, never the export.
type Options struct {
	Art            bool
	Audio          bool
	WorldStyle     string
	ProviderParams string
	OnProgress     func(format string, args ...interface{})
}

// ErrAudioUnavailable means no TTS provider is configured, which is a normal
// state rather than a failure: the beat plays silently. It lives here rather than
// in pkg/gui because every consumer of a script needs it; pkg/gui keeps its
// exported alias.
var ErrAudioUnavailable = errors.New("audio unavailable")

// ArtResolver returns a scene's image path, generating it when needed.
type ArtResolver interface {
	SceneArt(ctx context.Context, location *entity.Entity, force bool) (string, error)
}

// SpeechResolver returns a clip's path and duration, or ErrAudioUnavailable.
type SpeechResolver interface {
	SegmentAudio(ctx context.Context, segment entity.TurnSegment) (string, time.Duration, error)
}

// Compiler turns a campaign's timeline into a playable script.
type Compiler struct {
	source Source
	art    ArtResolver
	speech SpeechResolver
}

func NewCompiler(source Source) *Compiler {
	return &Compiler{source: source}
}

// SetArtResolver enables scene art. Without one, scenes have no imagery.
func (c *Compiler) SetArtResolver(art ArtResolver) { c.art = art }

// SetSpeechResolver enables per-beat audio. Without one, every beat is silent.
func (c *Compiler) SetSpeechResolver(speech SpeechResolver) { c.speech = speech }

func (c *Compiler) Compile(ctx context.Context, gameID string, opts Options) (*Script, error) {
	turns, err := c.source.Turns()
	if err != nil {
		return nil, fmt.Errorf("load turns: %w", err)
	}
	if len(turns) == 0 {
		return nil, fmt.Errorf("campaign %q has no turns to export", gameID)
	}

	script := &Script{GameID: gameID, WorldStyle: opts.WorldStyle}
	silent := 0

	for _, turn := range turns {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// An unlocated turn always opens its own scene: "unknown" is not a place,
		// so two of them are not the same place either.
		newScene := len(script.Scenes) == 0 ||
			turn.Location == "" ||
			script.Scenes[len(script.Scenes)-1].LocationID != turn.Location
		if newScene {
			script.Scenes = append(script.Scenes, c.openScene(turn.Location, opts))
		}
		current := &script.Scenes[len(script.Scenes)-1]

		for _, segment := range turn.Segments {
			beat := Beat{
				Kind:       beatKind(segment.Kind),
				TurnNumber: turn.Number,
				Speaker:    segment.Speaker,
				SpeakerID:  segment.SpeakerID,
				Text:       segment.Text,
				ArtPath:    current.ArtPath,
			}

			if opts.Audio && c.speech != nil {
				if err := c.resolveAudio(ctx, &beat, segment, &silent); err != nil {
					return nil, err
				}
			}

			beat.Duration = BeatDuration(beat)
			current.Beats = append(current.Beats, beat)
			current.Duration += beat.Duration
		}
	}

	// Sum the scenes rather than tracking a running total beside them, so the card
	// durations counted in openScene cannot be forgotten here.
	for _, sc := range script.Scenes {
		script.TotalDuration += sc.Duration
	}

	if silent > 0 && opts.OnProgress != nil {
		opts.OnProgress("%d beats have no audio clip and will play silently", silent)
	}
	return script, nil
}

// openScene builds a scene, resolving its art once so a long conversation reuses
// one image.
func (c *Compiler) openScene(locationID string, opts Options) Scene {
	sc := Scene{LocationID: locationID}

	// An unlocated turn is a scene of its own with no card: "unknown" is not a place.
	if locationID == "" {
		return sc
	}

	if location, err := c.source.Location(locationID); err == nil && location != nil {
		sc.LocationName = location.Name

		if opts.Art && c.art != nil {
			if art, err := c.art.SceneArt(context.Background(), location, false); err == nil {
				sc.ArtPath = art
			} else if opts.OnProgress != nil {
				opts.OnProgress("scene %q has no art: %v", locationID, err)
			}
		}
	}

	card := SceneCard(sc)
	sc.Beats = append(sc.Beats, card)
	sc.Duration += card.Duration
	return sc
}

func (c *Compiler) resolveAudio(ctx context.Context, beat *Beat, segment entity.TurnSegment, silent *int) error {
	path, duration, err := c.speech.SegmentAudio(ctx, segment)
	if err != nil {
		*silent++
		return nil
	}

	beat.AudioPath = path
	beat.AudioDuration = duration
	return nil
}

func beatKind(kind string) BeatKind {
	if kind == entity.SegmentSpeech {
		return BeatSpeech
	}
	return BeatNarration
}
```

Note that `openScene` adds the card's duration to the scene but `Compile` adds segment durations, and the card's duration is also part of the script total; the `TestCompileTotalsDurations` test pins that arithmetic.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/scene/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/scene/compile.go pkg/scene/compile_test.go
git commit -m "feat(scene): group a campaign into location-keyed scenes"
```

### Task 4: Resolve scene art through the media pipeline

**Files:**
- Create: `pkg/media/location_art.go`, `pkg/media/location_art_test.go`
- Modify: `pkg/gui/service.go` (delegate `GetLocationArt` to the new helper)
- Test: `pkg/media/location_art_test.go`

**Interfaces:**
- Consumes: `NewImagePipeline`, `GenerateLocationImage`, `AppearanceHash`, `ContentCache`
- Produces: `media.ArtStore`, `media.NewArtStore(client ImageClient, cache *ContentCache, worldStyle, providerParams string) *ArtStore`, `(*ArtStore).SceneArt(ctx context.Context, location *entity.Entity, force bool) (string, error)`

- [x] **Step 1: Write the failing test**

`pkg/media/location_art_test.go`:

```go
package media

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestArtStoreResolvesOnceAndReuses(t *testing.T) {
	client := &stubImageClient{body: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)}
	store := NewArtStore(client, NewContentCache(t.TempDir()), "dark fantasy", "builtin:")

	location := &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm."}

	first, err := store.SceneArt(context.Background(), location, false)
	if err != nil {
		t.Fatalf("SceneArt failed: %v", err)
	}
	if filepath.Ext(first) != ".svg" {
		t.Errorf("path = %q, want an svg", first)
	}

	second, err := store.SceneArt(context.Background(), location, false)
	if err != nil {
		t.Fatalf("second SceneArt failed: %v", err)
	}
	if second != first {
		t.Errorf("expected a cache hit at %q, got %q", first, second)
	}
	if client.calls != 1 {
		t.Errorf("expected 1 provider call, got %d", client.calls)
	}

	if _, err := store.SceneArt(context.Background(), location, true); err != nil {
		t.Fatalf("forced SceneArt failed: %v", err)
	}
	if client.calls != 2 {
		t.Errorf("expected the force flag to regenerate, got %d calls", client.calls)
	}
}

func TestArtStoreContextIsHonoured(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := NewArtStore(&stubImageClient{body: []byte("<svg")}, NewContentCache(t.TempDir()), "", "")
	if _, err := store.SceneArt(ctx, &entity.Entity{ID: "x", Type: "location"}, false); err == nil {
		t.Errorf("expected a cancelled context to fail the call")
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestArtStore -count=1 ./pkg/media/`
Expected: FAIL — `undefined: NewArtStore`

- [x] **Step 3: Implement**

Create `pkg/media/location_art.go`:

```go
package media

import (
	"context"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// ArtStore resolves scene art through the image pipeline, caching by appearance.
// It is the one implementation of "where does this location's image come from",
// shared by the GUI and the export so they cannot diverge.
type ArtStore struct {
	pipeline       *ImagePipeline
	worldStyle     string
	providerParams string
}

func NewArtStore(client ImageClient, cache *ContentCache, worldStyle, providerParams string) *ArtStore {
	return &ArtStore{
		pipeline:       NewImagePipeline(client, cache),
		worldStyle:     worldStyle,
		providerParams: providerParams,
	}
}

// SceneArt returns a location's image path, generating it when the appearance has
// changed or force is set.
func (s *ArtStore) SceneArt(ctx context.Context, location *entity.Entity, force bool) (string, error) {
	if location == nil {
		return "", fmt.Errorf("scene art: no location")
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("scene art for %q: %w", location.ID, err)
	}

	path, err := s.pipeline.GenerateLocationImage(ctx, location, s.worldStyle, s.providerParams, force)
	if err != nil {
		return "", fmt.Errorf("scene art for %q: %w", location.ID, err)
	}
	return path, nil
}
```

In `pkg/gui/service.go`, `GetLocationArt` keeps its signature and body shape but builds `media.NewArtStore(client, media.NewContentCache(s.resolver.CacheDir()), s.worldArtStyle(gameID), providerParams)` and calls `SceneArt(ctx, location, force)`, so the GUI route and the export share one resolver.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/media/ ./pkg/gui/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media/location_art.go pkg/media/location_art_test.go pkg/gui/service.go
git commit -m "feat(media): share one scene-art resolver between GUI and export"
```

### Task 5: Resolve per-beat audio, with durations and honest reporting

**Files:**
- Create: `pkg/media/probe.go`, `pkg/media/probe_test.go`
- Test: `pkg/scene/compile_test.go` (resolution behaviour with a fake resolver)

**Interfaces:**
- Consumes: `TTSPipeline.SynthesizeSegment`
- Produces: `media.ProbeAudioDuration(ctx context.Context, path string) (time.Duration, error)`

- [x] **Step 1: Write the failing tests**

`pkg/media/probe_test.go`:

```go
package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProbeAudioDurationRejectsAMissingFile(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}

	if _, err := ProbeAudioDuration(context.Background(), filepath.Join(t.TempDir(), "absent.wav")); err == nil {
		t.Errorf("expected an error for a missing file")
	}
}

func TestProbeAudioDurationReadsAGeneratedTone(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}

	path := filepath.Join(t.TempDir(), "tone.wav")
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot generate a test tone: %v: %s", err, out)
	}

	duration, err := ProbeAudioDuration(context.Background(), path)
	if err != nil {
		t.Fatalf("ProbeAudioDuration failed: %v", err)
	}
	if duration < 1900*time.Millisecond || duration > 2100*time.Millisecond {
		t.Errorf("duration = %v, want about 2s", duration)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("probing must not consume the file: %v", err)
	}
}
```

Append to `pkg/scene/compile_test.go`:

```go
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
```

`compile_test.go` already imports `fmt` and `strings` from Task 3.

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestProbeAudio|TestCompileResolvesAudio|TestCompileWithoutSpeechResolver" -count=1 ./pkg/media/ ./pkg/scene/`
Expected: FAIL — `undefined: ProbeAudioDuration`

- [x] **Step 3: Implement**

Create `pkg/media/probe.go`:

```go
package media

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ProbeAudioDuration reads a clip's real length with ffprobe, which is what lets
// pacing follow the audio rather than the text for beats that have a clip.
func ProbeAudioDuration(ctx context.Context, path string) (time.Duration, error) {
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		path,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("probe %q: %w", path, err)
	}

	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("probe %q: parse %q: %w", path, strings.TrimSpace(string(out)), err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
```

The scene side already handles a resolver that reports silence; this task only adds the probe and the resolver-driven tests.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/media/ ./pkg/scene/`
Expected: PASS (the ffprobe tests skip on a machine without it)

- [x] **Step 5: Commit**

```bash
git add pkg/media/probe.go pkg/media/probe_test.go pkg/scene/compile_test.go
git commit -m "feat(media): probe clip durations so pacing follows the audio"
```

<!-- PLAN-CONTINUES -->
---

## Phase 3: The Animated Web Player

### Task 6: Compile from the campaign and write a sidecar bundle

**Files:**
- Modify: `pkg/export/script.go` (adapter over `scene.Compile`)
- Modify: `pkg/export/web.go` (bundle writing), `pkg/export/types.go` (removed)
- Test: `pkg/export/web_test.go`

**Interfaces:**
- Consumes: `scene.NewCompiler`, `media.NewArtStore`, `media.NewSceneImageClient`, `media.NewTTSPipeline`, `media.ProbeAudioDuration`, `storage.OpenGameStore`
- Produces: `export.NewScriptCompiler(rootDir) *ScriptCompiler`, `(*ScriptCompiler).Compile(ctx, gameID) (*scene.Script, error)`, `(*WebExporter).Export(ctx, script *scene.Script, outDir string) (string, error)` returning the bundle directory

- [x] **Step 1: Write the failing test**

`pkg/export/web_test.go`:

```go
package export

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

func fixtureScript(t *testing.T, dir string) *scene.Script {
	t.Helper()

	art := filepath.Join(dir, "tavern.svg")
	if err := os.WriteFile(art, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatal(err)
	}
	clip := filepath.Join(dir, "welcome.wav")
	if err := os.WriteFile(clip, []byte("RIFF....WAVEfmt ....data"), 0644); err != nil {
		t.Fatal(err)
	}

	return &scene.Script{
		GameID:   "campaign-01",
		GameName: "Campaign One",
		Scenes: []scene.Scene{{
			LocationID:   "alden-tavern",
			LocationName: "Alden Tavern",
			ArtPath:      art,
			Duration:     5 * time.Second,
			Beats: []scene.Beat{
				{Kind: scene.BeatSceneCard, Text: "Alden Tavern", ArtPath: art, Duration: 2 * time.Second},
				{Kind: scene.BeatNarration, Text: "Warm light.", ArtPath: art, Duration: 2 * time.Second},
				{Kind: scene.BeatSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Welcome.", ArtPath: art,
					AudioPath: clip, AudioDuration: time.Second, Duration: 1400 * time.Millisecond},
			},
		}},
		TotalDuration: 5 * time.Second,
	}
}

func TestWebExportWritesSidecarAssets(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	bundle, err := NewWebExporter(".").Export(context.Background(), fixtureScript(t, dir), out)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if bundle != out {
		t.Errorf("bundle = %q, want the directory %q", bundle, out)
	}

	for _, want := range []string{
		"index.html",
		filepath.Join("assets", "scene-001.svg"),
		filepath.Join("audio", "beat-0001.wav"),
	} {
		if _, err := os.Stat(filepath.Join(out, want)); err != nil {
			t.Errorf("expected %s in the bundle: %v", want, err)
		}
	}

	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "assets/scene-001.svg") {
		t.Errorf("expected the player to reference the copied art")
	}
	if strings.Contains(string(html), "http://") || strings.Contains(string(html), "https://") {
		t.Errorf("a bundle must not reach out to the network")
	}
}

func TestWebExportEmbedsBeatDurations(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	if _, err := NewWebExporter(".").Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	const marker = "const SCRIPT = "
	idx := strings.Index(string(html), marker)
	if idx == -1 {
		t.Fatalf("expected the script to be embedded")
	}
	rest := string(html)[idx+len(marker):]
	rest = rest[:strings.Index(rest, ";\n")]

	var payload struct {
		GameName string `json:"game_name"`
		Scenes   []struct {
			Location string `json:"location"`
			Art      string `json:"art"`
			Beats    []struct {
				Kind     string  `json:"kind"`
				Speaker  string  `json:"speaker"`
				Text     string  `json:"text"`
				Audio    string  `json:"audio"`
				Duration float64 `json:"duration"`
			} `json:"beats"`
		} `json:"scenes"`
	}
	if err := json.Unmarshal([]byte(rest), &payload); err != nil {
		t.Fatalf("decode embedded script: %v\n%s", err, rest)
	}

	if payload.GameName != "Campaign One" || len(payload.Scenes) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	beats := payload.Scenes[0].Beats
	if len(beats) != 3 {
		t.Fatalf("expected 3 beats, got %d", len(beats))
	}
	if beats[2].Kind != "speech" || beats[2].Speaker != "Garrick" || beats[2].Audio != "audio/beat-0001.wav" {
		t.Errorf("unexpected speech beat: %+v", beats[2])
	}
	if beats[2].Duration < 1.3 || beats[2].Duration > 1.5 {
		t.Errorf("duration = %v, want seconds not nanoseconds", beats[2].Duration)
	}
}

func TestWebExportRejectsAnEmptyScript(t *testing.T) {
	if _, err := NewWebExporter(".").Export(context.Background(), &scene.Script{}, t.TempDir()); err == nil {
		t.Errorf("expected an error for a script with no scenes")
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestWebExport -count=1 ./pkg/export/`
Expected: FAIL — `cannot use fixtureScript(t, dir) (value of type *scene.Script) as *ReplayScript value`.

- [x] **Step 3: Implement**

In `pkg/export/script.go`, replace `ScriptCompiler` with the adapter that builds a configured `scene.Compiler`:

```go
// campaignSource supplies turns and locations from a campaign's files and index.
type campaignSource struct {
	resolver *core.PathResolver
	store    *storage.Store
	gameID   string
}

func (s *campaignSource) Turns() ([]engine.Turn, error) {
	path := filepath.Join(s.resolver.GameDir(s.gameID), "history.jsonl")
	return engine.NewHistoryLogger(path).LoadHistory()
}

func (s *campaignSource) Location(id string) (*entity.Entity, error) {
	return s.store.GetEntity(id)
}

// speechResolver synthesizes one segment at a time and probes the clip's length,
// falling back to the reading estimate when probing fails.
type speechResolver struct {
	pipeline *media.TTSPipeline
	store    *storage.Store
	narrator *entity.VoiceConfig
}

func (r *speechResolver) SegmentAudio(ctx context.Context, segment entity.TurnSegment) (string, time.Duration, error) {
	path, err := r.pipeline.SynthesizeSegment(ctx, segment, r.narrator, r.voiceFor)
	if err != nil {
		return "", 0, err
	}

	duration, err := media.ProbeAudioDuration(ctx, path)
	if err != nil {
		return path, 0, nil
	}
	return path, duration, nil
}

func (r *speechResolver) voiceFor(speakerID string) *entity.VoiceConfig {
	if speakerID == "" {
		return nil
	}
	ent, err := r.store.GetEntity(speakerID)
	if err != nil || ent == nil {
		return nil
	}
	return ent.Voice
}

// ScriptCompiler builds a scene script for a campaign.
type ScriptCompiler struct {
	rootDir  string
	resolver *core.PathResolver
	config   *config.Config
}

func NewScriptCompiler(rootDir string) *ScriptCompiler {
	cfg, _ := config.NewConfigManager().Load()
	return &ScriptCompiler{
		rootDir:  rootDir,
		resolver: core.NewPathResolver(rootDir),
		config:   cfg,
	}
}

func (c *ScriptCompiler) Compile(ctx context.Context, gameID string) (*scene.Script, error) {
	gameDir := c.resolver.GameDir(gameID)

	manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	store, err := storage.OpenGameStore(c.resolver, gameID)
	if err != nil {
		return nil, fmt.Errorf("open game store: %w", err)
	}

	worldStyle := ""
	if world, err := core.LoadWorldManifest(filepath.Join(c.resolver.WorldDir(manifest.WorldID), "world.yaml")); err == nil {
		worldStyle = strings.TrimSpace(strings.Join([]string{world.ArtStyle, world.Genre}, ", "))
	}

	compiler := scene.NewCompiler(&campaignSource{resolver: c.resolver, store: store, gameID: gameID})

	if c.config.Media.Image.BuiltinFallback || c.config.Media.Image.Type != "disabled" {
		if client, err := media.NewSceneImageClient(c.config.Media.Image); err == nil {
			cache := media.NewContentCache(c.resolver.CacheDir())
			params := c.config.Media.Image.Type + ":" + c.config.Media.Image.Model
			compiler.SetArtResolver(media.NewArtStore(client, cache, worldStyle, params))
		}
	}

	if c.config.Media.TTS.Type != "" && c.config.Media.TTS.Type != "disabled" {
		if client, err := media.NewTTSClient(c.config.Media.TTS); err == nil {
			cache := media.NewContentCache(c.resolver.CacheDir())
			compiler.SetSpeechResolver(&speechResolver{
				pipeline: media.NewTTSPipeline(client, cache),
				store:    store,
				narrator: &entity.VoiceConfig{
					VoiceID:    c.config.Media.TTS.DefaultVoice,
					Pitch:      c.config.Media.TTS.Pitch,
					SpeechRate: c.config.Media.TTS.SpeechRate,
				},
			})
		}
	}

	script, err := compiler.Compile(ctx, gameID, scene.Options{
		Art:            true,
		Audio:          true,
		WorldStyle:     worldStyle,
		ProviderParams: c.config.Media.Image.Type + ":" + c.config.Media.Image.Model,
		OnProgress: func(format string, args ...interface{}) {
			fmt.Fprintf(os.Stderr, "export: "+format+"\n", args...)
		},
	})
	if err != nil {
		return nil, err
	}

	script.GameName = manifest.Name
	return script, nil
}
```

In `pkg/export/web.go`, replace the inline HTML/JSON with the sidecar bundle. The embedded payload is seconds-based because the browser thinks in seconds:

```go
// webBeat is one beat as the browser sees it: durations in seconds and paths
// relative to the bundle root.
type webBeat struct {
	Kind     string  `json:"kind"`
	Speaker  string  `json:"speaker,omitempty"`
	Text     string  `json:"text"`
	Art      string  `json:"art,omitempty"`
	Audio    string  `json:"audio,omitempty"`
	Duration float64 `json:"duration"`
}

type webScene struct {
	Location string    `json:"location,omitempty"`
	Art      string    `json:"art,omitempty"`
	Beats    []webBeat `json:"beats"`
}

type webPayload struct {
	GameName string     `json:"game_name"`
	Scenes   []webScene `json:"scenes"`
	Total    float64    `json:"total_duration"`
}

// Export writes a self-contained animated player and returns its directory.
func (w *WebExporter) Export(ctx context.Context, script *scene.Script, outDir string) (string, error) {
	if script == nil || len(script.Scenes) == 0 {
		return "", fmt.Errorf("script has no scenes to export")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Join(outDir, "assets"), 0755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "audio"), 0755); err != nil {
		return "", fmt.Errorf("create audio dir: %w", err)
	}

	payload := &webPayload{GameName: script.GameName, Total: script.TotalDuration.Seconds()}
	beatNumber := 0

	for i := range script.Scenes {
		sc := script.Scenes[i]
		entry := webScene{Location: sc.LocationName}

		if sc.ArtPath != "" {
			name := fmt.Sprintf("scene-%03d%s", i+1, filepath.Ext(sc.ArtPath))
			if err := copyFile(sc.ArtPath, filepath.Join(outDir, "assets", name)); err == nil {
				entry.Art = "assets/" + name
			}
		}

		for j := range sc.Beats {
			beat := sc.Beats[j]
			beatNumber++

			jsBeat := webBeat{
				Kind:     string(beat.Kind),
				Speaker:  beat.Speaker,
				Text:     beat.Text,
				Art:      entry.Art,
				Duration: beat.Duration.Seconds(),
			}

			if beat.AudioPath != "" {
				name := fmt.Sprintf("beat-%04d%s", beatNumber, filepath.Ext(beat.AudioPath))
				if err := copyFile(beat.AudioPath, filepath.Join(outDir, "audio", name)); err == nil {
					jsBeat.Audio = "audio/" + name
				}
			}

			entry.Beats = append(entry.Beats, jsBeat)
		}

		payload.Scenes = append(payload.Scenes, entry)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	html := fmt.Sprintf(playerHTML, html.EscapeString(script.GameName), payloadJSON)
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(html), 0644); err != nil {
		return "", fmt.Errorf("write index.html: %w", err)
	}

	return outDir, nil
}

// copyFile copies a bundle asset, so the export never depends on the original
// cache or campaign directory still existing.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
```

Delete `pkg/export/types.go` (`ReplayScript`, `SceneBeat`) and update `cmd/localrpg/export.go` to pass a `*scene.Script` through unchanged.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/export/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/export cmd/localrpg/export.go
git commit -m "feat(export): compile campaigns into scenes and write a real bundle"
```

### Task 7: A player that runs itself

**Files:**
- Modify: `pkg/export/web.go` (the `playerHTML` template)
- Test: `pkg/export/web_test.go`

**Interfaces:**
- Consumes: the payload from Task 6
- Produces: `playerHTML` with `const SCRIPT = %s`, transport controls, typewriter reveal, autoplay with a gesture fallback, and reduced-motion support

- [x] **Step 1: Write the failing test**

Append to `pkg/export/web_test.go`:

```go
func TestPlayerCarriesTheRequiredBehaviours(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	if _, err := NewWebExporter(".").Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)

	required := map[string]string{
		"embedded script":     "const SCRIPT = ",
		"transport":           "data-action=\"play\"",
		"typewriter reveal":   "revealCount",
		"autoplay recovery":   ".catch(",
		"reduced motion":      "prefers-reduced-motion",
		"no webfont":          "Georgia, serif",
		"relative asset paths": "assets/",
	}

	for label, marker := range required {
		if !strings.Contains(page, marker) {
			t.Errorf("expected %s (%q) in the player", label, marker)
		}
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestPlayerCarriesTheRequiredBehaviours -count=1 ./pkg/export/`
Expected: FAIL — the current template still renders one beat with Previous/Next buttons.

- [x] **Step 3: Implement**

Replace `playerHTML` in `pkg/export/web.go` with a player that walks the beats:

```go
// playerHTML is the exported player. `%s` is the game name and `%s` the embedded
// script payload; it must reference nothing outside the bundle.
const playerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s - Story Theater</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: #0c0a09; color: #e7e5e4; font-family: Georgia, serif; overflow: hidden; }
  #stage { position: fixed; inset: 0; background-size: cover; background-position: center; transition: opacity 300ms ease; }
  #scrim { position: fixed; inset: 0; background: linear-gradient(180deg, rgba(12,10,9,0.35), rgba(12,10,9,0.92)); }
  #card { position: relative; height: 100vh; display: flex; flex-direction: column; justify-content: center;
          align-items: center; padding: 6vh 8vw; text-align: center; gap: 1.5rem; }
  #speaker { font-size: 0.9rem; letter-spacing: 0.2em; text-transform: uppercase; color: #f59e0b; min-height: 1.2rem; }
  #text { font-size: clamp(1.25rem, 2.4vw, 2rem); line-height: 1.6; max-width: 46rem; }
  #scene { position: fixed; top: 1.5rem; left: 1.5rem; font-size: 0.8rem; letter-spacing: 0.2em;
           text-transform: uppercase; color: #a8a29e; }
  #controls { position: fixed; bottom: 1.25rem; left: 50%%; transform: translateX(-50%%); display: flex;
              gap: 0.75rem; align-items: center; }
  button { background: rgba(255,255,255,0.08); border: 1px solid rgba(255,255,255,0.15); color: #e7e5e4;
           border-radius: 999px; padding: 0.35rem 0.9rem; font: inherit; font-size: 0.85rem; cursor: pointer; }
  button:hover { border-color: #f59e0b; color: #fbbf24; }
  #progress { position: fixed; bottom: 0; left: 0; height: 3px; background: #f59e0b; width: 0; }
</style>
</head>
<body>
<div id="stage"></div>
<div id="scrim"></div>
<div id="scene"></div>
<div id="card">
  <div id="speaker"></div>
  <div id="text"></div>
</div>
<div id="controls">
  <button data-action="play">Play</button>
  <button data-action="prev">Previous</button>
  <button data-action="next">Next</button>
</div>
<div id="progress"></div>
<script>
const SCRIPT = %s;

const beats = [];
SCRIPT.scenes.forEach((scene) => scene.beats.forEach((beat) => beats.push({ ...beat, scene: scene.location })));

const stage = document.getElementById('stage');
const sceneLabel = document.getElementById('scene');
const speaker = document.getElementById('speaker');
const textEl = document.getElementById('text');
const progress = document.getElementById('progress');
const playButton = document.querySelector('[data-action="play"]');

const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

let index = 0;
let timer = null;
let audio = null;
let paused = false;

function render(index) {
  const beat = beats[index];
  if (!beat) { finish(); return; }

  if (beat.art) stage.style.backgroundImage = 'url("' + beat.art + '")';
  sceneLabel.textContent = beat.scene || '';
  speaker.textContent = beat.kind === 'speech' ? (beat.speaker || 'UNKNOWN') : '';
  textEl.textContent = '';

  if (audio) { audio.pause(); audio = null; }
  if (beat.audio) {
    audio = new Audio(beat.audio);
    audio.play().catch(() => { pause(); });
  }

  startReveal(beat, performance.now());
}

function startReveal(beat, startedAt) {
  const revealMs = Math.max(1, beat.duration * 1000 * 0.6);
  const characters = Array.from(beat.text);

  const step = (now) => {
    if (paused) return;
    const elapsed = now - startedAt;
    const revealCount = reducedMotion ? characters.length : Math.ceil((elapsed / revealMs) * characters.length);
    textEl.textContent = characters.slice(0, Math.min(revealCount, characters.length)).join('');

    if (elapsed >= beat.duration * 1000) { next(); return; }
    timer = requestAnimationFrame(step);
  };

  timer = requestAnimationFrame(step);
}

function stop() {
  if (timer) cancelAnimationFrame(timer);
  timer = null;
  if (audio) { audio.pause(); audio = null; }
}

function schedule() {
  stop();
  const beat = beats[index];
  if (!beat) { finish(); return; }
  render(index);
  progress.style.width = ((index + 1) / beats.length * 100) + '%%';
}

function next() { index = Math.min(index + 1, beats.length); schedule(); }
function prev() { index = Math.max(index - 1, 0); schedule(); }
function finish() { playButton.textContent = 'Replay'; sceneLabel.textContent = ''; speaker.textContent = ''; }
function pause() { paused = true; stop(); playButton.textContent = 'Play'; }
function play() { paused = false; playButton.textContent = 'Pause'; schedule(); }

playButton.addEventListener('click', () => {
  if (paused) { play(); return; }
  if (index >= beats.length) { index = 0; play(); return; }
  pause();
});

document.querySelector('[data-action="next"]').addEventListener('click', next);
document.querySelector('[data-action="prev"]').addEventListener('click', prev);

// Browsers refuse to start audio without a gesture; render the first beat and
// wait for the click rather than running silently ahead.
render(0);
pause();
</script>
</body>
</html>
`
```

The player holds on the first beat until the viewer clicks Play, which is both the reliable autoplay path and the accessible one. Add `"html"` to the file's imports for `html.EscapeString`.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/export/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/export/web.go pkg/export/web_test.go
git commit -m "feat(export): make the exported player run itself"
```

<!-- PLAN-CONTINUES -->

---

## Phase 4: The Rasteriser

### Task 8: Frames, fonts, and text

**Files:**
- Modify: `go.mod`, `go.sum` (`golang.org/x/image` at `v0.46.0`)
- Create: `pkg/scene/render.go`, `pkg/scene/render_test.go`

**Interfaces:**
- Consumes: `scene.Beat`, `scene.Scene`, `TypewriterFraction`
- Produces: `scene.FrameRequest`, `scene.Renderer`, `scene.NewRenderer(width, height int) (*Renderer, error)`, `(*Renderer).Frame(FrameRequest) *image.RGBA`, `scene.revealText(string, float64) string`, `scene.wrapText(font.Face, string, int, int) []string`

- [x] **Step 1: Add the dependency and write the failing test**

Run: `go get golang.org/x/image@v0.46.0`

`pkg/scene/render_test.go`:

```go
package scene

import (
	"bytes"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"
)

func renderFrame(t *testing.T, r *Renderer, req FrameRequest) []byte {
	t.Helper()

	img := r.Frame(req)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	return buf.Bytes()
}

func narrationBeat() Beat {
	return Beat{Kind: BeatNarration, Text: "The hall is quiet and the candles gutter.", Duration: 3 * time.Second}
}

func TestFrameIsTheRequestedSize(t *testing.T) {
	r, err := NewRenderer(640, 360)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	img := r.Frame(FrameRequest{Beat: narrationBeat(), Progress: 1})
	if img.Bounds().Dx() != 640 || img.Bounds().Dy() != 360 {
		t.Errorf("bounds = %v, want 640x360", img.Bounds())
	}
}

func TestFrameIsDeterministic(t *testing.T) {
	r, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	req := FrameRequest{Beat: narrationBeat(), Progress: 1}
	if !bytes.Equal(renderFrame(t, r, req), renderFrame(t, r, req)) {
		t.Errorf("the same frame request must render identically")
	}
}

func TestFrameRevealsTextOverTheBeat(t *testing.T) {
	r, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	early := renderFrame(t, r, FrameRequest{Beat: narrationBeat(), Progress: 0.1})
	whole := renderFrame(t, r, FrameRequest{Beat: narrationBeat(), Progress: 1})
	if bytes.Equal(early, whole) {
		t.Errorf("expected the reveal to change the frame")
	}

	// Past the typewriter window the frame settles.
	settled := renderFrame(t, r, FrameRequest{Beat: narrationBeat(), Progress: 0.7})
	if !bytes.Equal(settled, whole) {
		t.Errorf("expected the text to be fully revealed after %.0f%% of the beat", TypewriterFraction*100)
	}
}

func TestFrameDrawsTextPixels(t *testing.T) {
	r, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	img := r.Frame(FrameRequest{Beat: narrationBeat(), Progress: 1})

	background := color.RGBA{12, 10, 9, 255}
	distinct := 0
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if img.RGBAAt(x, y) != background {
				distinct++
			}
		}
	}
	if distinct < 100 {
		t.Errorf("expected visible text, found only %d non-background pixels", distinct)
	}
}

func TestRevealTextFollowsTheTypewriterWindow(t *testing.T) {
	text := "abcdefghij"

	if got := revealText(text, 0); got != "" {
		t.Errorf("revealText at progress 0 = %q, want empty", got)
	}
	if got := revealText(text, TypewriterFraction); got != text {
		t.Errorf("revealText at the window = %q, want the whole text", got)
	}
	half := revealText(text, TypewriterFraction/2)
	if len(half) != 5 {
		t.Errorf("revealText halfway = %q (%d runes), want 5", half, len(half))
	}
	if got := revealText(text, 1); got != text {
		t.Errorf("revealText at progress 1 = %q, want the whole text", got)
	}
}

func TestWrapTextBoundsTheFrame(t *testing.T) {
	r, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	long := strings.Repeat("word ", 400)
	lines := wrapText(r.body, long, 200, 4)

	if len(lines) > 4 {
		t.Errorf("expected at most 4 lines, got %d", len(lines))
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[len(lines)-1]), "…") {
		t.Errorf("expected the last line to be elided, got %q", lines[len(lines)-1])
	}
}
```

`"time"` is in the import block above; Task 9 adds `"image"`, `"image/draw"`, `"os"`, and `"path/filepath"` as its tests need them.

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestFrame|TestRevealText|TestWrapText" -count=1 ./pkg/scene/`
Expected: FAIL — `undefined: NewRenderer`

- [x] **Step 3: Implement**

Create `pkg/scene/render.go`:

```go
package scene

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	// textMargin is the share of the width kept clear on each side.
	textMargin = 0.08
	// maxTextLines bounds a frame's text so long prose cannot overflow it.
	maxTextLines = 8
	// crossfadeShare is the share of a scene's first beat spent blending in.
	crossfadeShare = 0.12
	// driftStart and driftEnd are the background scale at a beat's ends.
	driftStart = 1.02
	driftEnd   = 1.06
	// baseColour matches the app and the player's background.
	baseColour = color.RGBA{12, 10, 9, 255}
)

// FrameRequest describes one frame to draw.
type FrameRequest struct {
	Scene       Scene
	Beat        Beat
	Progress    float64 // 0 at the beat's start, 1 at its end
	PreviousArt string  // the outgoing scene's art, for the crossfade
}

// Renderer draws video frames. Faces live as long as the renderer, which is what
// keeps parsing the bundled fonts to once per export.
type Renderer struct {
	width  int
	height int
	body   font.Face
	label  font.Face
}

func NewRenderer(width, height int) (*Renderer, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse regular font: %w", err)
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse bold font: %w", err)
	}

	bodySize := float64(height) / 28
	body, err := opentype.NewFace(regular, &opentype.FaceOptions{Size: bodySize, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, fmt.Errorf("new body face: %w", err)
	}
	label, err := opentype.NewFace(bold, &opentype.FaceOptions{Size: bodySize * 0.55, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, fmt.Errorf("new label face: %w", err)
	}

	return &Renderer{width: width, height: height, body: body, label: label}, nil
}

// Frame draws one frame: the scene's imagery behind, the beat's text over it.
func (r *Renderer) Frame(req FrameRequest) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.width, r.height))
	draw.Draw(img, img.Bounds(), image.NewUniform(baseColour), image.Point{}, draw.Src)

	r.drawBackground(img, req)
	r.drawText(img, req)
	return img
}

// revealText shows the share of text a beat has reached, so the video's
// typewriter matches the player's.
func revealText(text string, progress float64) string {
	if progress >= 1 {
		return text
	}
	if progress <= 0 {
		return ""
	}

	runes := []rune(text)
	revealed := progress / TypewriterFraction
	if revealed > 1 {
		revealed = 1
	}

	count := int(float64(len(runes)) * revealed)
	if count > len(runes) {
		count = len(runes)
	}
	return string(runes[:count])
}

// wrapText breaks text into lines that fit width, capping the count and eliding
// the remainder rather than letting prose run off the frame.
func wrapText(face font.Face, text string, width, maxLines int) []string {
	words := strings.Fields(text)
	lines := make([]string, 0, maxLines)
	current := ""

	flush := func() {
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
	}

	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if font.MeasureString(face, candidate).Ceil() <= width {
			current = candidate
			continue
		}
		flush()
		current = word
	}
	flush()

	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = strings.TrimSpace(lines[maxLines-1]) + "…"
	}
	return lines
}

func (r *Renderer) drawText(img *image.RGBA, req FrameRequest) {
	revealed := revealText(req.Beat.Text, req.Progress)
	wrapWidth := r.width - 2*int(float64(r.width)*textMargin)
	lines := wrapText(r.body, revealed, wrapWidth, maxTextLines)

	lineHeight := r.body.Metrics().Height.Ceil() + 8
	labelHeight := 0
	if req.Beat.Speaker != "" {
		labelHeight = r.label.Metrics().Height.Ceil() + 18
	}

	blockHeight := lineHeight * len(lines)
	baseline := (r.height-blockHeight-labelHeight)/2 + lineHeight

	if req.Beat.Speaker != "" {
		r.drawCentred(img, r.label, strings.ToUpper(req.Beat.Speaker), baseline-18, color.RGBA{245, 158, 11, 255})
	}
	for _, line := range lines {
		r.drawCentred(img, r.body, line, baseline, color.RGBA{231, 229, 228, 255})
		baseline += lineHeight
	}
}

func (r *Renderer) drawCentred(img *image.RGBA, face font.Face, text string, baseline int, col color.Color) {
	if strings.TrimSpace(text) == "" {
		return
	}

	width := font.MeasureString(face, text).Ceil()
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P((r.width-width)/2, baseline),
	}
	drawer.DrawString(text)
}
```

`drawBackground` arrives in Task 9; until then the flat base colour is the background, which is what makes this task testable on its own. Add a stub with that behaviour:

```go
// drawBackground paints the scene's imagery. Until imagery lands, the base colour
// already laid down is the background.
func (r *Renderer) drawBackground(img *image.RGBA, req FrameRequest) {}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/scene/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add go.mod go.sum pkg/scene/render.go pkg/scene/render_test.go
git commit -m "feat(scene): draw frames and text with bundled fonts"
```

### Task 9: Backgrounds, decoded art, and drift

**Files:**
- Modify: `pkg/scene/render.go`
- Test: `pkg/scene/render_test.go`

**Interfaces:**
- Consumes: `AppearanceHash` output (a hex string), `image/png`, `image/jpeg`, `golang.org/x/image/webp`
- Produces: `scene.loadArt(path string) (image.Image, error)`, `scene.proceduralBackground(seed string, width, height int) *image.RGBA`, `scene.drawCover(dst *image.RGBA, src image.Image, scale float64)`

- [x] **Step 1: Write the failing test**

Append to `pkg/scene/render_test.go`:

```go
func TestProceduralBackgroundIsDeterministicAndVaried(t *testing.T) {
	first := proceduralBackground("aldon-harbour", 160, 90)
	repeat := proceduralBackground("aldon-harbour", 160, 90)
	if !bytes.Equal(pngBytes(t, first), pngBytes(t, repeat)) {
		t.Errorf("the same seed must produce the same background")
	}

	other := proceduralBackground("alden-tavern", 160, 90)
	if bytes.Equal(pngBytes(t, first), pngBytes(t, other)) {
		t.Errorf("different locations must not share a background")
	}
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestLoadArtDecodesRasterAndRejectsSVG(t *testing.T) {
	dir := t.TempDir()

	pngPath := filepath.Join(dir, "art.png")
	if err := os.WriteFile(pngPath, pngBytes(t, image.NewRGBA(image.Rect(0, 0, 8, 8))), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadArt(pngPath); err != nil {
		t.Errorf("expected a PNG to decode: %v", err)
	}

	svgPath := filepath.Join(dir, "art.svg")
	if err := os.WriteFile(svgPath, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadArt(svgPath); err == nil {
		t.Errorf("expected SVG art to be rejected so the procedural background is used")
	}

	if _, err := loadArt(filepath.Join(dir, "absent.png")); err == nil {
		t.Errorf("expected an error for a missing file")
	}
}

func TestFrameUsesSceneArtWhenItDecodes(t *testing.T) {
	r, err := NewRenderer(160, 90)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	dir := t.TempDir()
	artPath := filepath.Join(dir, "art.png")
	flat := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.RGBA{200, 30, 30, 255}), image.Point{}, draw.Src)
	if err := os.WriteFile(artPath, pngBytes(t, flat), 0644); err != nil {
		t.Fatal(err)
	}

	withArt := renderFrame(t, r, FrameRequest{
		Scene: Scene{LocationID: "alden-tavern", ArtPath: artPath},
		Beat:  Beat{Kind: BeatNarration, Text: "Red room."}, Progress: 1,
	})
	without := renderFrame(t, r, FrameRequest{Beat: narrationBeat(), Progress: 1})

	if bytes.Equal(withArt, without) {
		t.Errorf("expected scene art to change the frame")
	}
}

func TestDrawCoverHandlesTinyArt(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	drawCover(img, image.NewRGBA(image.Rect(0, 0, 1, 1)), 1.05)
	drawCover(img, image.NewRGBA(image.Rect(0, 0, 400, 10)), 1.05)
}
```

Add `"image/draw"`, `"os"`, `"path/filepath"` to the test imports.

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestProceduralBackground|TestLoadArt|TestFrameUsesSceneArt|TestDrawCover" -count=1 ./pkg/scene/`
Expected: FAIL — `undefined: proceduralBackground`

- [x] **Step 3: Implement**

Replace the `drawBackground` stub in `pkg/scene/render.go`:

```go
// drawBackground paints the scene's imagery: decoded raster art when the file is
// one, otherwise a raster background drawn from the location's identity, since
// x/image cannot rasterise the SVG the built-in generator produces. The drift
// scale makes a held beat feel alive rather than frozen.
func (r *Renderer) drawBackground(img *image.RGBA, req FrameRequest) {
	scale := driftStart + (driftEnd-driftStart)*clamp01(req.Progress)

	if art, err := loadArt(req.Scene.ArtPath); err == nil {
		drawCover(img, art, scale)
	} else {
		seed := req.Scene.LocationID
		if seed == "" && req.Beat.ArtPath != "" {
			seed = req.Beat.ArtPath
		}
		drawCover(img, proceduralBackground(seed, r.width, r.height), scale)
	}
}

// loadArt decodes a raster scene image. SVG is deliberately rejected: the caller
// falls back to the procedural background rather than failing the frame.
func loadArt(path string) (image.Image, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("load art: no path")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("load art %q: %w", path, err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode art %q: %w", path, err)
	}
	return img, nil
}

// proceduralBackground draws a deterministic background from a seed, so a location
// looks the same every time it appears and different from every other location.
func proceduralBackground(seed string, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	hasher := fnv.New64a()
	hasher.Write([]byte(seed))
	h := hasher.Sum64()

	// A sky-to-ground gradient tinted by the seed.
	top := color.RGBA{
		R: uint8(18 + (h>>8)%40),
		G: uint8(16 + (h>>16)%32),
		B: uint8(28 + (h>>24)%48),
		A: 255,
	}
	bottom := color.RGBA{R: 8, G: 7, B: 6, A: 255}

	for y := 0; y < height; y++ {
		t := float64(y) / float64(max(1, height-1))
		row := color.RGBA{
			R: uint8(float64(top.R)*(1-t) + float64(bottom.R)*t),
			G: uint8(float64(top.G)*(1-t) + float64(bottom.G)*t),
			B: uint8(float64(top.B)*(1-t) + float64(bottom.B)*t),
			A: 255,
		}
		draw.Draw(img, image.Rect(0, y, width, y+1), image.NewUniform(row), image.Point{}, draw.Src)
	}

	// Ridgelines: three silhouettes whose heights come from the seed.
	rng := rand.New(rand.NewSource(int64(h)))
	ground := color.RGBA{R: 5, G: 5, B: 6, A: 255}
	for layer := 0; layer < 3; layer++ {
		baseline := height - (height/6)*(layer+1)
		amplitude := height / (6 + layer)
		phase := rng.Float64() * 6.28

		for x := 0; x < width; x++ {
			wave := math.Sin(float64(x)/float64(max(1, width))*6.28+phase) * float64(amplitude)
			top := int(float64(baseline) - wave)
			if top < 0 {
				top = 0
			}
			draw.Draw(img, image.Rect(x, top, x+1, height), image.NewUniform(ground), image.Point{}, draw.Src)
		}
	}

	return img
}

// drawCover scales src to cover dst, centred, so art of any aspect ratio fills the
// frame without distortion.
func drawCover(dst *image.RGBA, src image.Image, scale float64) {
	bounds := dst.Bounds()
	targetW := int(float64(bounds.Dx()) * scale)
	targetH := int(float64(bounds.Dy()) * scale)
	if targetW < 1 || targetH < 1 || src.Bounds().Dx() < 1 || src.Bounds().Dy() < 1 {
		return
	}

	scaled := scaleImage(src, targetW, targetH)
	offset := image.Pt((bounds.Dx()-targetW)/2, (bounds.Dy()-targetH)/2)
	draw.Draw(dst, image.Rect(offset.X, offset.Y, offset.X+targetW, offset.Y+targetH), scaled, image.Point{}, draw.Over)
}

// scaleImage resamples with nearest-neighbour, which is enough for a drifting
// background and keeps the export dependency-free.
func scaleImage(src image.Image, width, height int) *image.RGBA {
	srcBounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		sy := srcBounds.Min.Y + y*srcBounds.Dy()/height
		for x := 0; x < width; x++ {
			sx := srcBounds.Min.X + x*srcBounds.Dx()/width
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
```

Add `hash/fnv`, `math`, and `math/rand` to `render.go`'s imports, and import the image decoders for their `init` registration:

```go
import (
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/scene/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/scene/render.go pkg/scene/render_test.go
git commit -m "feat(scene): paint scene backgrounds, decoded or drawn"
```

### Task 10: Scene cards and crossfades

**Files:**
- Modify: `pkg/scene/render.go`
- Test: `pkg/scene/render_test.go`

**Interfaces:**
- Consumes: `FrameRequest.PreviousArt`, `crossfadeShare`
- Produces: a crossfade from the previous scene's art on a scene's opening beats, and card styling for `BeatSceneCard`

- [x] **Step 1: Write the failing test**

Append to `pkg/scene/render_test.go`:

```go
func TestFrameCrossfadesBetweenScenes(t *testing.T) {
	r, err := NewRenderer(160, 90)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	dir := t.TempDir()
	red := filepath.Join(dir, "red.png")
	blue := filepath.Join(dir, "blue.png")

	paint := func(path string, col color.RGBA) {
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		draw.Draw(img, img.Bounds(), image.NewUniform(col), image.Point{}, draw.Src)
		if err := os.WriteFile(path, pngBytes(t, img), 0644); err != nil {
			t.Fatal(err)
		}
	}
	paint(red, color.RGBA{220, 20, 20, 255})
	paint(blue, color.RGBA{20, 20, 220, 255})

	beat := Beat{Kind: BeatNarration, Text: "The docks.", Duration: 3 * time.Second}
	sc := Scene{LocationID: "aldon-harbour", ArtPath: blue}

	blending := renderFrame(t, r, FrameRequest{Scene: sc, Beat: beat, Progress: crossfadeShare / 2, PreviousArt: red})
	settled := renderFrame(t, r, FrameRequest{Scene: sc, Beat: beat, Progress: 1, PreviousArt: red})

	if bytes.Equal(blending, settled) {
		t.Errorf("expected the opening frames to differ while the scene blends in")
	}
}

func TestSceneCardStandsOut(t *testing.T) {
	r, err := NewRenderer(160, 90)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	card := renderFrame(t, r, FrameRequest{
		Scene: Scene{LocationID: "alden-tavern", LocationName: "Alden Tavern"},
		Beat:  SceneCard(Scene{LocationName: "Alden Tavern"}),
		Progress: 1,
	})
	narration := renderFrame(t, r, FrameRequest{Beat: narrationBeat(), Progress: 1})

	if bytes.Equal(card, narration) {
		t.Errorf("expected a scene card to look different from narration")
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run "TestFrameCrossfades|TestSceneCardStandsOut" -count=1 ./pkg/scene/`
Expected: FAIL — the crossfade is ignored.

- [x] **Step 3: Implement**

In `drawBackground`, blend from the previous scene's art during the beat's opening:

```go
func (r *Renderer) drawBackground(img *image.RGBA, req FrameRequest) {
	scale := driftStart + (driftEnd-driftStart)*clamp01(req.Progress)

	if blend := crossfadeAlpha(req.Progress); blend < 1 && req.PreviousArt != "" {
		if previous, err := loadArt(req.PreviousArt); err == nil {
			drawCover(img, previous, scale)
			drawCoverAlpha(img, r.sceneArt(req), scale, blend)
			return
		}
	}
	drawCover(img, r.sceneArt(req), scale)
}

// sceneArt is the scene's decoded art, or a procedural background when the file
// is missing or not a raster image.
func (r *Renderer) sceneArt(req FrameRequest) image.Image {
	if art, err := loadArt(req.Scene.ArtPath); err == nil {
		return art
	}

	seed := req.Scene.LocationID
	if seed == "" {
		seed = req.Beat.ArtPath
	}
	return proceduralBackground(seed, r.width, r.height)
}

// crossfadeAlpha is how opaque the incoming scene is: it rises across the first
// share of a beat so a scene change reads as a transition, not a glitch.
func crossfadeAlpha(progress float64) float64 {
	if progress >= crossfadeShare {
		return 1
	}
	return clamp01(progress / crossfadeShare)
}

// drawCoverAlpha scales src to cover dst, centred, blended at alpha.
func drawCoverAlpha(dst *image.RGBA, src image.Image, scale, alpha float64) {
	if alpha <= 0 {
		return
	}
	if alpha >= 1 {
		drawCover(dst, src, scale)
		return
	}

	bounds := dst.Bounds()
	targetW := int(float64(bounds.Dx()) * scale)
	targetH := int(float64(bounds.Dy()) * scale)
	if targetW < 1 || targetH < 1 {
		return
	}

	scaled := scaleImage(src, targetW, targetH)
	offset := image.Pt((bounds.Dx()-targetW)/2, (bounds.Dy()-targetH)/2)
	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			dstX, dstY := offset.X+x, offset.Y+y
			if !image.Pt(dstX, dstY).In(bounds) {
				continue
			}

			r16, g16, b16, _ := scaled.At(x, y).RGBA()
			existing := dst.RGBAAt(dstX, dstY)
			dst.SetRGBA(dstX, dstY, color.RGBA{
				R: uint8(float64(existing.R)*(1-alpha) + float64(r16>>8)*alpha),
				G: uint8(float64(existing.G)*(1-alpha) + float64(g16>>8)*alpha),
				B: uint8(float64(existing.B)*(1-alpha) + float64(b16>>8)*alpha),
				A: 255,
			})
		}
	}
}
```

Scene cards already stand out because their text is the location name with no speaker label; give them one more distinction in `drawText`:

```go
	if req.Beat.Kind == BeatSceneCard {
		// A card is a title, so it borrows the accented label face and colour and
		// skips the speaker and wrap machinery entirely.
		r.drawCentred(img, r.label, strings.ToUpper(req.Beat.Text), r.height/2, color.RGBA{245, 158, 11, 255})
		return
	}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/scene/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/scene/render.go pkg/scene/render_test.go
git commit -m "feat(scene): blend scene changes and style scene cards"
```

<!-- PLAN-CONTINUES -->

---

## Phase 5: Video Rendering

### Task 11: Render the frame sequence

**Files:**
- Modify: `pkg/export/video.go`
- Test: `pkg/export/video_test.go`

**Interfaces:**
- Consumes: `scene.NewRenderer`, `scene.FramesFor`, `scene.BeatDuration`
- Produces: `export.frameWriter`, `(*frameWriter).write(script *scene.Script) (int, error)`, `(*VideoPipeline).SetSize(w, h int)`, `SetFPS(fps int)`, `SetStill(still bool)`

- [x] **Step 1: Write the failing test**

Replace `pkg/export/video_test.go`'s contents with a frame-rendering test plus the command assertions from Task 12:

```go
package export

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

func smallScript() *scene.Script {
	return &scene.Script{
		GameID:   "campaign-01",
		GameName: "Campaign One",
		Scenes: []scene.Scene{{
			LocationID:   "alden-tavern",
			LocationName: "Alden Tavern",
			Duration:     4 * time.Second,
			Beats: []scene.Beat{
				{Kind: scene.BeatSceneCard, Text: "Alden Tavern", Duration: 2 * time.Second},
				{Kind: scene.BeatNarration, Text: "Warm light.", Duration: 2 * time.Second},
			},
		}},
		TotalDuration: 4 * time.Second,
	}
}

func TestFrameWriterRendersOneFramePerBeatWhenStill(t *testing.T) {
	renderer, err := scene.NewRenderer(160, 90)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	dir := t.TempDir()
	writer := &frameWriter{renderer: renderer, dir: dir, fps: 5, still: true}

	count, err := writer.write(smallScript())
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 frames for 2 beats, got %d", count)
	}

	file, err := os.Open(filepath.Join(dir, "frame-000000.png"))
	if err != nil {
		t.Fatalf("expected the first frame: %v", err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	if img.Bounds().Dx() != 160 || img.Bounds().Dy() != 90 {
		t.Errorf("frame bounds = %v, want 160x90", img.Bounds())
	}
}

func TestFrameWriterFollowsPacingWhenAnimating(t *testing.T) {
	renderer, err := scene.NewRenderer(64, 36)
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	dir := t.TempDir()
	writer := &frameWriter{renderer: renderer, dir: dir, fps: 10}

	count, err := writer.write(smallScript())
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Two beats of two seconds at 10fps.
	if count != 40 {
		t.Errorf("expected 40 frames, got %d", count)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 40 {
		t.Errorf("expected 40 files on disk, got %d", len(entries))
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestFrameWriter -count=1 ./pkg/export/`
Expected: FAIL — `undefined: frameWriter`

- [x] **Step 3: Implement**

In `pkg/export/video.go`:

```go
// VideoPipeline renders a script to a video file.
type VideoPipeline struct {
	rootDir string
	width   int
	height  int
	fps     int
	still   bool
}

func NewVideoPipeline(rootDir string) *VideoPipeline {
	return &VideoPipeline{rootDir: rootDir, width: 1920, height: 1080, fps: scene.DefaultFPS}
}

// SetSize changes the output resolution.
func (v *VideoPipeline) SetSize(width, height int) {
	if width > 0 && height > 0 {
		v.width, v.height = width, height
	}
}

// SetFPS changes the frame rate.
func (v *VideoPipeline) SetFPS(fps int) {
	if fps > 0 {
		v.fps = fps
	}
}

// SetStill renders one frame per beat instead of an animated sequence, for a fast
// export on a weak machine.
func (v *VideoPipeline) SetStill(still bool) { v.still = still }

// frameWriter renders a script's frames into a directory as PNGs.
type frameWriter struct {
	renderer *scene.Renderer
	dir      string
	fps      int
	still    bool
	progress func(format string, args ...interface{})
}

func (w *frameWriter) write(script *scene.Script) (int, error) {
	number := 0
	previousArt := ""

	for i := range script.Scenes {
		sc := script.Scenes[i]

		for _, beat := range sc.Beats {
			frames := scene.FramesFor(beat.Duration, w.fps)
			if w.still {
				frames = 1
			}

			for f := 0; f < frames; f++ {
				progress := 1.0
				if frames > 1 {
					progress = float64(f) / float64(frames-1)
				}

				img := w.renderer.Frame(scene.FrameRequest{
					Scene:       sc,
					Beat:        beat,
					Progress:    progress,
					PreviousArt: previousArt,
				})

				path := filepath.Join(w.dir, fmt.Sprintf("frame-%06d.png", number))
				if err := writePNG(path, img); err != nil {
					return 0, err
				}
				number++
			}
		}

		previousArt = sc.ArtPath
		if w.progress != nil {
			w.progress("rendered scene %d/%d", i+1, len(script.Scenes))
		}
	}

	return number, nil
}

// writePNG encodes one frame.
func writePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create frame %q: %w", path, err)
	}
	defer file.Close()

	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(file, img); err != nil {
		return fmt.Errorf("encode frame %q: %w", path, err)
	}
	return nil
}
```

`png.BestSpeed` matters: a 1080p frame is otherwise slow to encode, and PNG size is irrelevant next to `x264`'s time.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/export/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/export/video.go pkg/export/video_test.go
git commit -m "feat(export): render a campaign's frames"
```

### Task 12: Mux the frames against per-clip audio

**Files:**
- Modify: `pkg/export/video.go`
- Test: `pkg/export/video_test.go`

**Interfaces:**
- Consumes: `scene.Script.Beats()`, `Beat.AudioPath`, `Beat.Duration`
- Produces: `(*VideoPipeline).BuildCommand(ctx context.Context, script *scene.Script, framesDir, outputFile string) (*exec.Cmd, error)`

- [x] **Step 1: Write the failing test**

Append to `pkg/export/video_test.go`:

```go
func TestBuildCommandKeepsPictureAndSoundInStep(t *testing.T) {
	pipeline := NewVideoPipeline(".")
	pipeline.SetFPS(10)

	script := smallScript()
	script.Scenes[0].Beats[1].AudioPath = "/cache/welcome.wav"
	script.Scenes[0].Beats[1].AudioDuration = 1500 * time.Millisecond
	script.Scenes[0].Beats[1].Duration = 1900 * time.Millisecond

	cmd, err := pipeline.BuildCommand(context.Background(), script, "/frames", "/out.mp4")
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}

	args := strings.Join(cmd.Args, " ")
	for _, want := range []string{
		"-framerate 10",
		"/frames/frame-%06d.png",
		"-i /cache/welcome.wav",
		"anullsrc=r=44100:cl=stereo",   // the silent card needs its own input
		"concat=n=2:v=0:a=1[a]",
		"-map 0:v",
		"-map [a]",
		"-c:v libx264",
		"-pix_fmt yuv420p",
		"-shortest",
		"/out.mp4",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("expected %q in the ffmpeg invocation:\n%s", want, args)
		}
	}

	// The silent beat's length comes from its own duration, so the audio track
	// matches the frame count.
	if !strings.Contains(args, "-t 2.000") {
		t.Errorf("expected the card's duration as a silence length:\n%s", args)
	}
}

func TestBuildCommandWithOnlySilence(t *testing.T) {
	pipeline := NewVideoPipeline(".")

	cmd, err := pipeline.BuildCommand(context.Background(), smallScript(), "/frames", "/out.mp4")
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}

	args := strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "concat=n=2:v=0:a=1[a]") {
		t.Errorf("expected both beats to contribute silence:\n%s", args)
	}
	if strings.Contains(args, "-i /cache") {
		t.Errorf("expected no clip inputs:\n%s", args)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestBuildCommand -count=1 ./pkg/export/`
Expected: FAIL — the old command has no clip inputs, no `concat`, and no frame pattern.

- [x] **Step 3: Implement**

Replace `BuildCommand`:

```go
// BuildCommand assembles the FFmpeg invocation for a rendered frame sequence.
// Inputs follow beat order so the concat filter's stream indices line up, and a
// silent beat gets an anullsrc input whose length is that beat's own duration.
func (v *VideoPipeline) BuildCommand(ctx context.Context, script *scene.Script, framesDir, outputFile string) (*exec.Cmd, error) {
	if len(script.Scenes) == 0 {
		return nil, fmt.Errorf("build command: script has no scenes")
	}

	args := []string{
		"-y",
		"-framerate", strconv.Itoa(v.fps),
		"-i", filepath.Join(framesDir, "frame-%06d.png"),
	}

	streams := make([]string, 0, len(script.Beats()))
	index := 1

	for _, beat := range script.Beats() {
		if beat.AudioPath != "" {
			args = append(args, "-i", beat.AudioPath)
		} else {
			silence := beat.Duration.Seconds()
			if silence <= 0 {
				silence = scene.MinimumBeatDuration.Seconds()
			}
			args = append(args,
				"-f", "lavfi",
				"-t", strconv.FormatFloat(silence, 'f', 3, 64),
				"-i", "anullsrc=r=44100:cl=stereo",
			)
		}

		streams = append(streams, fmt.Sprintf("[%d:a]", index))
		index++
	}

	if len(streams) > 0 {
		args = append(args,
			"-filter_complex", fmt.Sprintf("%sconcat=n=%d:v=0:a=1[a]", strings.Join(streams, ""), len(streams)),
			"-map", "0:v",
			"-map", "[a]",
		)
	}

	args = append(args,
		"-c:v", "libx264",
		"-tune", "stillimage",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "192k",
		"-shortest",
		outputFile,
	)

	return exec.CommandContext(ctx, "ffmpeg", args...), nil
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/export/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/export/video.go pkg/export/video_test.go
git commit -m "feat(export): mux frames against each beat's own audio"
```

### Task 13: Render a video end to end

**Files:**
- Modify: `pkg/export/video.go`
- Test: `pkg/export/video_test.go`

**Interfaces:**
- Consumes: `frameWriter`, `BuildCommand`, `exec.LookPath`
- Produces: `(*VideoPipeline).RenderVideo(ctx context.Context, script *scene.Script, outputFile string) error` writing a complete file or nothing at all

- [x] **Step 1: Write the failing test**

Append to `pkg/export/video_test.go`:

```go
func TestRenderVideoProducesAFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}

	pipeline := NewVideoPipeline(".")
	pipeline.SetSize(320, 180)
	pipeline.SetFPS(5)
	pipeline.SetStill(true)

	out := filepath.Join(t.TempDir(), "replay.mp4")
	if err := pipeline.RenderVideo(context.Background(), smallScript(), out); err != nil {
		t.Fatalf("RenderVideo failed: %v", err)
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("expected an output file: %v", err)
	}
	if info.Size() < 1024 {
		t.Errorf("output is only %d bytes, which is not a video", info.Size())
	}
	if _, err := os.Stat(out + ".part"); !os.IsNotExist(err) {
		t.Errorf("expected the intermediate file to be renamed away, stat err = %v", err)
	}
}

func TestRenderVideoLeavesNothingBehindOnFailure(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}

	pipeline := NewVideoPipeline(".")
	pipeline.SetSize(160, 90)
	pipeline.SetFPS(5)
	pipeline.SetStill(true)

	// A directory where the file should go makes the final rename fail.
	out := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}

	if err := pipeline.RenderVideo(context.Background(), smallScript(), out); err == nil {
		t.Errorf("expected an error when the output path is a directory")
	}
	if _, err := os.Stat(out + ".part"); !os.IsNotExist(err) {
		t.Errorf("expected no leftover intermediate file, stat err = %v", err)
	}
}

func TestRenderVideoRequiresScenes(t *testing.T) {
	if err := NewVideoPipeline(".").RenderVideo(context.Background(), &scene.Script{}, "out.mp4"); err == nil {
		t.Errorf("expected an error for a script with no scenes")
	}
}
```

Add `"os/exec"` to the test imports.

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestRenderVideo -count=1 ./pkg/export/`
Expected: FAIL — `RenderVideo` still builds the old silent command.

- [x] **Step 3: Implement**

```go
// RenderVideo draws every frame, muxes them against the campaign's audio, and
// renames the result into place, so a failed render never leaves a file that
// looks playable.
func (v *VideoPipeline) RenderVideo(ctx context.Context, script *scene.Script, outputFile string) error {
	if script == nil || len(script.Scenes) == 0 {
		return fmt.Errorf("render video: script has no scenes")
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg is required for video export: %w", err)
	}

	renderer, err := scene.NewRenderer(v.width, v.height)
	if err != nil {
		return fmt.Errorf("build renderer: %w", err)
	}

	framesDir, err := os.MkdirTemp("", "localrpg-frames-")
	if err != nil {
		return fmt.Errorf("create frames dir: %w", err)
	}
	defer os.RemoveAll(framesDir)

	writer := &frameWriter{
		renderer: renderer,
		dir:      framesDir,
		fps:      v.fps,
		still:    v.still,
		progress: func(format string, args ...interface{}) {
			fmt.Fprintf(os.Stderr, "export: "+format+"\n", args...)
		},
	}

	count, err := writer.write(script)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("render video: no frames were rendered")
	}

	part := outputFile + ".part"
	cmd, err := v.BuildCommand(ctx, script, framesDir, part)
	if err != nil {
		return err
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		os.Remove(part)
		return fmt.Errorf("ffmpeg failed: %w: %s", err, lastLines(stderr.String(), 5))
	}

	if err := os.Rename(part, outputFile); err != nil {
		os.Remove(part)
		return fmt.Errorf("publish video: %w", err)
	}
	return nil
}

// lastLines trims FFmpeg's output to the part worth showing a person.
func lastLines(output string, count int) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}
```

Add `"bytes"`, `"image"`, `"image/png"`, `"os"`, `"strconv"`, and the `scene` import to `video.go`; drop the old fixed-duration code path.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/export/`
Expected: PASS (the render tests skip without FFmpeg)

- [x] **Step 5: Commit**

```bash
git add pkg/export/video.go pkg/export/video_test.go
git commit -m "feat(export): render a complete video or nothing"
```

---

## Phase 6: Surface and Cleanup

### Task 14: CLI flags and an honest README

**Files:**
- Modify: `pkg/export/script.go` (`SetMedia`)
- Modify: `cmd/localrpg/export.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `scene.Options`, `VideoPipeline.SetSize/SetFPS/SetStill`
- Produces: `(*ScriptCompiler).SetMedia(art, audio bool)`

- [x] **Step 1: Write the failing test**

Append to `cmd/localrpg/media_test.go` (the CLI tests already shell out, so this follows their style):

```go
func TestCLIExportUsageListsMediaFlags(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "export", "--help")
	out, _ := cmd.CombinedOutput()

	for _, want := range []string{"no-art", "no-audio", "still", "fps", "size"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("expected --%s in the export usage output:\n%s", want, out)
		}
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestCLIExportUsageListsMediaFlags -count=1 ./cmd/localrpg/`
Expected: FAIL — the flags do not exist yet.

- [x] **Step 3: Implement**

In `pkg/export/script.go`:

```go
// SetMedia disables art or audio resolution for an export. Both are on by
// default: an export is expected to look and sound like the campaign.
func (c *ScriptCompiler) SetMedia(art, audio bool) {
	c.art = art
	c.audio = audio
}
```

with `art` and `audio` fields defaulting to true in `NewScriptCompiler`, and `Compile` mapping them into `scene.Options{Art: c.art, Audio: c.audio, …}`.

In `cmd/localrpg/export.go`, add the flags and pass them through:

```go
	noArt := fs.Bool("no-art", false, "Skip scene imagery")
	noAudio := fs.Bool("no-audio", false, "Skip speech clips, using only cached audio")
	still := fs.Bool("still", false, "Render one frame per beat instead of animating")
	fps := fs.Int("fps", scene.DefaultFPS, "Video frame rate")
	size := fs.String("size", "1920x1080", "Video size as WxH")
```

then:

```go
	compiler := export.NewScriptCompiler(*dir)
	compiler.SetMedia(!*noArt, !*noAudio)

	script, err := compiler.Compile(context.Background(), gameID)
	if err != nil { … }

	switch format {
	case "video":
		pipeline := export.NewVideoPipeline(*dir)
		pipeline.SetStill(*still)
		pipeline.SetFPS(*fps)
		if width, height, err := parseSize(*size); err == nil {
			pipeline.SetSize(width, height)
		} else {
			fmt.Fprintf(os.Stderr, "export: ignoring --size %q: %v\n", *size, err)
		}
		…
	}
```

with:

```go
// parseSize reads a WxH geometry.
func parseSize(value string) (int, int, error) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(value)), "x", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected WxH")
	}

	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if width < 16 || height < 16 {
		return 0, 0, fmt.Errorf("dimensions must be at least 16x16")
	}
	return width, height, nil
}
```

In `README.md`, replace the export line's claim with what it now does: an animated visual-novel web bundle (auto-running, location scenes, per-speaker audio, sidecar assets) and a video rendered from the same scene script, with the flags listed and the FFmpeg/ffprobe requirement named.

- [x] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./cmd/localrpg/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/export/script.go cmd/localrpg/export.go README.md
git commit -m "feat(cli): expose export media options and document the result"
```

### Task 15: Name and serve audio clips honestly

**Files:**
- Modify: `pkg/media/tts.go` (`AudioExtension`, `AudioContentType`, cached-clip lookup, clip naming)
- Modify: `pkg/gui/service.go` (`ErrAudioUnavailable` becomes an alias; the route serves the sniffed type)
- Test: `pkg/media/tts_test.go`, `pkg/gui/server_test.go`

**Interfaces:**
- Consumes: `ContentCache`
- Produces: `media.AudioExtension(data []byte) string`, `media.AudioContentType(data []byte) string`; `gui.ErrAudioUnavailable = scene.ErrAudioUnavailable`

- [ ] **Step 1: Write the failing test**

Append to `pkg/media/tts_test.go`:

```go
func TestAudioExtensionSniffsTheBytes(t *testing.T) {
	cases := map[string]string{
		"RIFF....WAVEfmt ":            ".wav",
		"ID3\x04\x00":                 ".mp3",
		"\xff\xfb\x90\x00":            ".mp3",
		"OggS\x00\x02":                ".ogg",
		"fLaC\x00\x00":                ".flac",
		"surprise bytes from a host": ".wav",
	}

	for body, want := range cases {
		if got := AudioExtension([]byte(body)); got != want {
			t.Errorf("AudioExtension(%q) = %q, want %q", body, got, want)
		}
	}

	if got := AudioContentType([]byte("ID3\x04\x00")); got != "audio/mpeg" {
		t.Errorf("AudioContentType = %q, want audio/mpeg", got)
	}
}

func TestSynthesizeUtteranceReusesALegacyWavNamedClip(t *testing.T) {
	client := &recordingTTSClient{}
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(client, cache)

	first, err := pipeline.SynthesizeUtterance(context.Background(), "narrator", nil, "hello")
	if err != nil {
		t.Fatalf("SynthesizeUtterance failed: %v", err)
	}

	// An older cache wrote every clip as .wav; a second run must reuse it rather
	// than synthesising again.
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, data, 0644); err != nil {
		t.Fatal(err)
	}

	second, err := pipeline.SynthesizeUtterance(context.Background(), "narrator", nil, "hello")
	if err != nil {
		t.Fatalf("second SynthesizeUtterance failed: %v", err)
	}
	if second != first {
		t.Errorf("expected the cached clip %q, got %q", first, second)
	}
	if client.calls != 1 {
		t.Errorf("expected 1 synthesis call, got %d", client.calls)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestAudioExtension|TestSynthesizeUtteranceReuses" -count=1 ./pkg/media/`
Expected: FAIL — `undefined: AudioExtension`

- [ ] **Step 3: Implement**

In `pkg/media/tts.go`:

```go
// audioExtensions are the names a cached clip may carry, sniffed first and the
// legacy .wav name last so existing caches stay warm.
var audioExtensions = []string{".wav", ".mp3", ".ogg", ".flac"}

// AudioExtension names a clip from its bytes, because a provider returns whatever
// its engine produces rather than what the configuration implies.
func AudioExtension(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("RIFF")):
		return ".wav"
	case bytes.HasPrefix(data, []byte("ID3")):
		return ".mp3"
	case len(data) > 1 && data[0] == 0xFF && data[1]&0xE0 == 0xE0:
		return ".mp3"
	case bytes.HasPrefix(data, []byte("OggS")):
		return ".ogg"
	case bytes.HasPrefix(data, []byte("fLaC")):
		return ".flac"
	default:
		return ".wav"
	}
}

// AudioContentType is the MIME type for a clip's bytes.
func AudioContentType(data []byte) string {
	switch AudioExtension(data) {
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	default:
		return "audio/wav"
	}
}
```

`SynthesizeUtterance` looks up before synthesising and names what it writes:

```go
	if path, ok := p.cachedClip(base); ok {
		return path, nil
	}

	audioBytes, err := p.client.Synthesize(ctx, text, voice)
	if err != nil {
		return "", fmt.Errorf("synthesize utterance: %w", err)
	}
	return p.cache.Put("audio", base+AudioExtension(audioBytes), audioBytes)
```

with:

```go
// cachedClip finds a clip under any known extension.
func (p *TTSPipeline) cachedClip(base string) (string, bool) {
	for _, ext := range audioExtensions {
		if p.cache.Exists("audio", base+ext) {
			return filepath.Join(p.cache.Subdir("audio"), base+ext), true
		}
	}
	return "", false
}
```

In `pkg/gui/service.go`, `ErrAudioUnavailable` becomes an alias so the scene package's sentinel and the GUI's are one value, and the audio route's content type comes from the bytes:

```go
// ErrAudioUnavailable is the scene package's sentinel, kept as an alias so the
// route and its tests read unchanged.
var ErrAudioUnavailable = scene.ErrAudioUnavailable
```

and in the route (which needs `os` and `media` imports in `pkg/gui/server.go`):

```go
		data, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", media.AudioContentType(data))
		_, _ = w.Write(data)
```

Append to `pkg/gui/server_test.go`:

```go
func TestSegmentAudioRouteSniffsTheContentType(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/turn/1/segment/1/audio", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "audio/") {
		t.Errorf("Content-Type = %q, want an audio type", ct)
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/media/ ./pkg/gui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/tts.go pkg/media/tts_test.go pkg/gui/service.go pkg/gui/server_test.go
git commit -m "fix(media): name and serve speech clips by what they contain"
```

### Task 16: Pace the in-app player from the same estimate

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go` (`SegmentDTO.Duration`)
- Modify: `frontend/src/types.ts`, `frontend/src/components/StoryTheater.tsx`
- Test: `pkg/gui/service_test.go`, `cd frontend && npx tsc --noEmit`

**Interfaces:**
- Consumes: `scene.ReadingDuration`
- Produces: `gui.SegmentDTO.Duration float64` (seconds), `TurnSegment.duration?: number`

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/service_test.go`:

```go
func TestChronicleReportsSegmentDurations(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	turns, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || len(turns[0].Segments) != 2 {
		t.Fatalf("unexpected chronicle: %+v", turns)
	}

	for _, segment := range turns[0].Segments {
		if segment.Duration < scene.MinimumBeatDuration.Seconds() {
			t.Errorf("segment %q duration = %v, want at least the floor", segment.Text, segment.Duration)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestChronicleReportsSegmentDurations -count=1 ./pkg/gui/`
Expected: FAIL — `segment.Duration undefined`

- [ ] **Step 3: Implement**

In `pkg/gui/types.go`, `SegmentDTO` gains `Duration float64 \`json:"duration"\``. In `pkg/gui/service.go`, `segmentDTOs` fills it:

```go
			Duration: scene.ReadingDuration(segment.Text).Seconds(),
```

In `frontend/src/types.ts`, `TurnSegment` gains `duration?: number;`. In `StoryTheater.tsx`, the current turn's advance uses the segment's reported duration when it has one, and its existing fallback otherwise:

```tsx
  const segmentDurationMs = (currentTurn?.segments?.[segmentIndex]?.duration ?? 0) * 1000;
```

where `segmentIndex` is the component's existing playback position; the auto-advance timer uses `segmentDurationMs > 0 ? segmentDurationMs : <existing fallback>`. The browser replaces the estimate with a clip's real length as soon as the audio element reports it, which is the same rule the video renderer applies.

- [ ] **Step 4: Verify the type check and the tests**

Run: `go test -count=1 ./pkg/gui/ && cd frontend && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat(gui): share one pacing estimate with the in-app player"
```

---

## Spec Coverage

| Spec section | Tasks |
| --- | --- |
| §3 scene model and types | 1 |
| §3.1 reading-time pacing and frame arithmetic | 2 |
| §4 compilation, grouping, resolution, reporting | 3, 4, 5 |
| §4 art per scene, through one shared resolver | 4 |
| §5 animated player, sidecar bundle, offline guarantee | 6, 7 |
| §6.1 rasteriser, fonts, text, backgrounds, drift | 8, 9 |
| §6.1 scene cards and crossfade | 10 |
| §6.2 audio assembly and concat command | 12 |
| §6.3 frame sequence, temp-and-rename, `--still`, FFmpeg detection | 11, 13 |
| §7 pacing parity with the in-app player | 16 |
| §8 audio cache extension and content types | 15 |
| §9 CLI surface | 14 |
| §10 file map | all |
| §12 verification plan | every task's Step 1 |

### Deviations and follow-ups

- **`gui.ErrAudioUnavailable` becomes an alias** of `scene.ErrAudioUnavailable` (Task 15) rather than a second sentinel, because the attribution and playback work defines it in `pkg/gui` first and the scene package needs the same value.
- **The clip-sniffing task also covers a pre-existing mislabelling**: every clip used to be named `.wav` whatever the provider returned, and the segment route served `audio/wav` for all of them. Task 15 fixes both, and keeps old caches warm by accepting a legacy `.wav` name.
- **`export.NewScriptCompiler` survives** as the adapter that assembles a configured `scene.Compiler`, so `cmd/localrpg/export.go` gains flags rather than being rewritten.
- **Story Theater's timer is adjusted, not redesigned** (Task 16): it consumes the reported duration where it already has a timer, with its existing fallback intact. Any larger redesign of that component is out of scope.
- **Deferred to the GUI turn submission spec**: submitting a turn from the desktop app, streaming the narration, and whatever the Chronicle needs to drive playback from the app rather than from a rendered bundle.
- **Task 6 also has to port `pkg/export/video.go`, `script_test.go`, `legacy_script_test.go` and `video_test.go`.** Deleting `ReplayScript` breaks `VideoPipeline`, which takes it, and the three older tests assert the retired beat shape. The video pipeline's signatures change to `*scene.Script` here (still rendering a still-backed clip); Task 11 replaces its body, not its interface.
- **The compiler still folds legacy prose into segments.** `campaignSource.Turns()` parses a turn's prose with `media.LegacySegments` when the record has no segments, which the retired compiler did and Task 3's `Compile` alone does not. Without it, every campaign recorded before segments existed would export as scene cards with no text. This lives in the source adapter rather than in `pkg/scene`, so the scene model does not depend on `pkg/media`.
- **Bundle clips are numbered in playback order, not by beat number.** Numbering every beat leaves holes in `audio/` (the first clip lands on `beat-0003.wav` when a scene card and a narration beat precede it), so the counter advances only when a clip is written. This matches the `audio/beat-0001.wav` the plan's own assertion expects.
- **Task 7's player template is written with Task 6.** Task 6's tests pin the payload and the asset layout independently of the player's behaviour, so writing a throwaway template first and replacing it immediately would be wasted work; Task 7's test asserts the behaviours the template already carries.
- **`baseColour` is a variable, not a constant** (Task 8). A `color.RGBA` composite literal is not a constant expression in Go, so the plan's `const` block cannot hold it.
- **Task 9 invalidates one of Task 8's assertions.** Task 8's `TestFrameRevealsTextOverTheBeat` checks that the frame "settles" once the typewriter window has passed, which was true only while the background was flat. Drift is continuous for the whole beat, so frames never settle pixel-for-pixel; the test now asserts the revealed text is complete instead, and the crossfade test compares two points that are both past the blend.
- **`drawBackground` splits out `sceneArt`** (Task 10). The crossfade needs the incoming scene drawn twice, once at full opacity and once blended, so resolving art moved into its own method rather than being inlined.
- **The staging file keeps the output's container extension** (Task 13). FFmpeg chooses its muxer from the filename, so `<out>.mp4.part` fails outright with "Unable to choose an output format". `stagingPath` inserts `.part` before the extension instead (`replay.part.mp4`), and the tests assert against `stagingPath(out)` rather than a literal `".part"` suffix, which would have been vacuous.
- **Tasks 11, 12 and 13 are one commit.** They rewrite `BuildCommand` and `RenderVideo`, so shipping the frame writer against the retired still-image command would have meant writing an interim `BuildCommand` only to replace it in the next task.

### Execution notes

- Task 8 is the only task that needs network access (`go get golang.org/x/image@v0.46.0`); the version is already in the local module cache, so even that should resolve offline.
- Tasks 5 and 13 have tests that skip when `ffprobe` or `ffmpeg` are absent, so the plan is testable on a machine without them, minus the end-to-end render.
