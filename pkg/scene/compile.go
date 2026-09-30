package scene

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
)

// Source supplies the campaign facts compilation needs, which keeps grouping and
// resolution testable without a filesystem or a database.
type Source interface {
	Turns() ([]engine.Turn, error)
	Location(id string) (*entity.Entity, error)
}

// Options controls what compilation resolves. Art, audio, and portraits are
// decoration: a failure to resolve any of them degrades a beat, never the export.
type Options struct {
	Art            bool
	Audio          bool
	WorldStyle     string
	ProviderParams string
	// PlayerID is the protagonist, whose portrait stays on stage for the whole
	// story rather than per beat.
	PlayerID   string
	OnProgress func(format string, args ...interface{})
	// Progress reports structured progress for a caller that wants a bar or an
	// event stream. OnProgress is kept for the CLI's line output.
	Progress ProgressFunc
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

// SpeechResolver returns a beat's clip paths and their total duration, or
// ErrAudioUnavailable. A beat is several clips when it is several sentences.
type SpeechResolver interface {
	SegmentAudio(ctx context.Context, segment entity.TurnSegment) ([]string, time.Duration, error)
}

// PortraitResolver returns a character's portrait file, or ErrAudioUnavailable
// when the character has none. A missing portrait is a normal state: the theatre
// and the exported player both fall back to a procedural bust.
type PortraitResolver interface {
	Portrait(ctx context.Context, characterID string) (string, error)
}

// Compiler turns a campaign's timeline into a playable script.
type Compiler struct {
	source    Source
	art       ArtResolver
	speech    SpeechResolver
	portraits PortraitResolver
}

// NewCompiler builds a compiler that reads a campaign through source.
func NewCompiler(source Source) *Compiler {
	return &Compiler{source: source}
}

// SetArtResolver enables scene art. Without one, scenes have no imagery.
func (c *Compiler) SetArtResolver(art ArtResolver) { c.art = art }

// SetSpeechResolver enables per-beat audio. Without one, every beat is silent.
func (c *Compiler) SetSpeechResolver(speech SpeechResolver) { c.speech = speech }

// SetPortraitResolver enables per-beat portraits. Without one, beats carry none.
func (c *Compiler) SetPortraitResolver(portraits PortraitResolver) { c.portraits = portraits }

// Compile walks the campaign's turns, grouping them into scenes by location and
// flattening each turn's segments into beats.
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

	// The protagonist's portrait is resolved once: the theatre keeps it on stage for
	// the whole story rather than per beat.
	if c.portraits != nil && opts.PlayerID != "" {
		if path, err := c.portraits.Portrait(ctx, opts.PlayerID); err == nil {
			script.PlayerPortrait = path
		}
	}

	for i, turn := range turns {
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
				Player:     segment.Player,
			}

			if opts.Audio && c.speech != nil {
				c.resolveAudio(ctx, &beat, segment, &silent)
			}
			c.resolvePortrait(ctx, &beat, segment)

			beat.Duration = BeatDuration(beat)
			current.Beats = append(current.Beats, beat)
			current.Duration += beat.Duration
		}

		emitProgress(opts.Progress, Progress{Phase: "compile", Done: i + 1, Total: len(turns)})
	}

	// Sum the scenes rather than tracking a running total beside them, so the card
	// durations counted in openScene cannot be forgotten here.
	for _, sc := range script.Scenes {
		script.TotalDuration += sc.Duration
	}

	if silent > 0 {
		message := fmt.Sprintf("%d beats have no audio clip and will play silently", silent)
		if opts.OnProgress != nil {
			opts.OnProgress("%s", message)
		}
		emitProgress(opts.Progress, Progress{Phase: "compile", Message: message})
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

// resolveAudio attaches a beat's clips when they can be resolved. A missing clip is
// counted and skipped so one silent line cannot abandon the export.
func (c *Compiler) resolveAudio(ctx context.Context, beat *Beat, segment entity.TurnSegment, silent *int) {
	paths, duration, err := c.speech.SegmentAudio(ctx, segment)
	if err != nil || len(paths) == 0 {
		*silent++
		return
	}

	beat.AudioPaths = paths
	beat.AudioDuration = duration
}

// resolvePortrait attaches the speaker's portrait to a speech beat. Only a line
// someone says has a face: narration is the narrator's, and a failure is silent.
func (c *Compiler) resolvePortrait(ctx context.Context, beat *Beat, segment entity.TurnSegment) {
	if c.portraits == nil || beat.Kind != BeatSpeech {
		return
	}

	ref := segment.SpeakerID
	if ref == "" {
		ref = entity.Slugify(segment.Speaker)
	}
	if ref == "" {
		return
	}
	if path, err := c.portraits.Portrait(ctx, ref); err == nil {
		beat.PortraitPath = path
	}
}

func beatKind(kind string) BeatKind {
	if kind == entity.SegmentSpeech {
		return BeatSpeech
	}
	return BeatNarration
}
