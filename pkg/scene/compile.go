package scene

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	Art        bool
	Audio      bool
	WorldStyle string
	// Genre is the world's genre alone, which tints the no-art background.
	Genre          string
	ProviderParams string
	// AssetsDir is a campaign's assets directory, where a turn's own scene
	// illustration lives. Empty means only the location backdrop is resolved.
	AssetsDir string
	// PlayerID is the protagonist, whose portrait stays on stage for the whole
	// story rather than per beat.
	PlayerID string
	// BannerPath is the campaign's own image, which a player shows when a scene has
	// no art: the theatre falls back to it rather than to nothing.
	BannerPath string
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

// turnArtExtensions are the file types a turn's illustration may be stored as,
// probed in this order so a campaign with more than one kind resolves steadily.
var turnArtExtensions = []string{".png", ".webp", ".jpg", ".jpeg", ".svg"}

// TurnArt resolves a turn's own scene illustration in a campaign's assets
// directory. A missing or empty file reports false, so the caller falls back to
// the location backdrop.
func TurnArt(assetsDir string, turn int) (string, bool) {
	if strings.TrimSpace(assetsDir) == "" || turn <= 0 {
		return "", false
	}
	for _, ext := range turnArtExtensions {
		path := filepath.Join(assetsDir, "scenes", fmt.Sprintf("turn-%d%s", turn, ext))
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() == 0 {
			continue
		}
		return path, true
	}
	return "", false
}

// ErrNoSpeakableText means a beat reduces to nothing once its Markdown and performance
// tags are removed: a stage direction, say. Such a beat is deliberately never spoken, so it
// is not a beat that needs a clip and its absence is not a silence to report.
var ErrNoSpeakableText = errors.New("beat has no speakable text")

// ArtResolver returns a scene's image path, generating it when needed.
type ArtResolver interface {
	SceneArt(ctx context.Context, location *entity.Entity, force bool) (string, error)
}

// SpeechResolver returns a beat's clip paths and their total duration, or
// ErrAudioUnavailable. A beat is several clips when it is several sentences.
type SpeechResolver interface {
	SegmentAudio(ctx context.Context, segment entity.TurnSegment) ([]string, time.Duration, error)
}

// GroupSpeechResolver is implemented by a speech resolver that renders a whole
// turn's groups, so beats that share a clip are synthesized once and the export
// resolves the same keys the app does.
type GroupSpeechResolver interface {
	TurnAudio(ctx context.Context, segments []entity.TurnSegment) ([]ClipGroup, error)
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

// turnGroups plans a turn's audio groups and resolves them, returning the group
// covering each segment index and which segments a preceding beat's clip already
// covers. It returns nil maps when the resolver does not group, so the compiler
// falls back to per-beat resolution.
func (c *Compiler) turnGroups(ctx context.Context, segments []entity.TurnSegment, opts Options) (map[int]ClipGroup, map[int]bool) {
	if !opts.Audio || c.speech == nil {
		return nil, nil
	}
	grouped, ok := c.speech.(GroupSpeechResolver)
	if !ok {
		return nil, nil
	}
	groups, err := grouped.TurnAudio(ctx, segments)
	if err != nil || len(groups) == 0 {
		return nil, nil
	}
	bySegment := make(map[int]ClipGroup, len(segments))
	covered := make(map[int]bool, len(segments))
	for _, group := range groups {
		for i, index := range group.SegmentIndexes {
			bySegment[index] = group
			if i > 0 {
				covered[index] = true
			}
		}
	}
	return bySegment, covered
}

// groupBeatShares splits each group's audio duration across its beats in
// proportion to their reading time, so the beats advance as the one clip plays
// rather than all holding for the whole clip and then repeating it.
func groupBeatShares(segments []entity.TurnSegment, groups map[int]ClipGroup) map[int]time.Duration {
	if len(groups) == 0 {
		return nil
	}
	members := map[string][]int{}
	for index, group := range groups {
		members[group.Key] = append(members[group.Key], index)
	}
	shares := make(map[int]time.Duration, len(groups))
	for _, indexes := range members {
		total := groups[indexes[0]].Duration
		if total <= 0 {
			continue
		}
		reads := make([]float64, len(indexes))
		var sum float64
		for i, index := range indexes {
			reads[i] = float64(ReadingDuration(segments[index].Text))
			sum += reads[i]
		}
		if sum <= 0 {
			share := total / time.Duration(len(indexes))
			for _, index := range indexes {
				shares[index] = share
			}
			continue
		}
		for i, index := range indexes {
			shares[index] = time.Duration(float64(total) * reads[i] / sum)
		}
	}
	return shares
}

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

	script := &Script{GameID: gameID, WorldStyle: opts.WorldStyle, Genre: opts.Genre, Banner: opts.BannerPath}
	var silent silence
	// Spoken beats are the ones that can speak at all: a beat that reduces to nothing is
	// never spoken, so it is not one of them. The split by kind is kept as the script is
	// built, so the report never has to guess what a beat turned out to be.
	spoken, withAudio := 0, 0
	byKind := map[BeatKind][2]int{}

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

		// A turn's audio is planned as groups first, so a run of beats that shares
		// one clip is synthesized once and the clip plays across the run: its first
		// beat carries the clip, and every beat in the run takes a share of its
		// duration so the run advances under the audio.
		groupClips, covered := c.turnGroups(ctx, turn.Segments, opts)
		shares := groupBeatShares(turn.Segments, groupClips)

		for segmentIndex, segment := range turn.Segments {
			// A turn's own illustration wins over the scene's backdrop, so an
			// export shows the moment the app showed rather than only the place.
			art := current.ArtPath
			if opts.Art {
				if turnArt, ok := TurnArt(opts.AssetsDir, turn.Number); ok {
					art = turnArt
				}
			}

			beat := Beat{
				Kind:       beatKind(segment.Kind),
				TurnNumber: turn.Number,
				Speaker:    segment.Speaker,
				SpeakerID:  segment.SpeakerID,
				Text:       segment.Text,
				ArtPath:    art,
				Player:     segment.Player,
				Outcome:    turn.Outcome,
			}

			if opts.Audio {
				outcome := outcomeUnspoken
				switch {
				case c.speech == nil:
					// Audio was asked for and no provider can give it: the most common
					// silent bundle, and the one most worth explaining.
					silent.note(errNoSpeechProvider)
					outcome = outcomeSilent
				case groupClips != nil:
					if group, ok := groupClips[segmentIndex]; ok {
						if !covered[segmentIndex] {
							beat.AudioPaths = group.AudioPaths
							beat.AudioDuration = group.Duration
						}
						if len(group.AudioPaths) > 0 {
							outcome = outcomeClips
						} else {
							outcome = outcomeSilent
						}
					} else {
						outcome = c.resolveAudio(ctx, &beat, segment, &silent)
					}
				default:
					outcome = c.resolveAudio(ctx, &beat, segment, &silent)
				}

				switch outcome {
				case outcomeClips:
					spoken++
					withAudio++
					entry := byKind[beat.Kind]
					entry[0]++
					byKind[beat.Kind] = entry
				case outcomeSilent:
					spoken++
					entry := byKind[beat.Kind]
					entry[1]++
					byKind[beat.Kind] = entry
				}
			}
			c.resolvePortrait(ctx, &beat, segment)

			beat.Duration = BeatDuration(beat)
			if share, ok := shares[segmentIndex]; ok && share > 0 {
				beat.Duration = share
			}
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

	// Chapters are the scenes as navigable boundaries: the title from the location
	// and the start from the running total, so the web and the video agree on where
	// a scene begins.
	start := time.Duration(0)
	for _, sc := range script.Scenes {
		script.Chapters = append(script.Chapters, Chapter{Title: chapterTitle(sc), Start: start})
		start += sc.Duration
	}

	// What the script can actually say, split by kind: a bundle whose characters are silent
	// looks the same as one whose narration is, unless the export says which.
	coverage := coverageReport(spoken, withAudio, byKind)
	if opts.OnProgress != nil {
		opts.OnProgress("%s", coverage)
	}
	emitProgress(opts.Progress, Progress{Phase: "compile", Message: coverage})

	if silent.beats > 0 {
		message := fmt.Sprintf("%d beats have no audio clip and will play silently", silent.beats)
		if silent.first != nil {
			// A bundle that cannot speak should say why it is quiet: the reason is the
			// difference between a campaign with no clips and a provider that failed.
			message = fmt.Sprintf("%d beats have no audio clip (first failure: %v) and will play silently", silent.beats, silent.first)
		}
		if opts.OnProgress != nil {
			opts.OnProgress("%s", message)
		}
		emitProgress(opts.Progress, Progress{Phase: "compile", Message: message})
	}
	return script, nil
}

// chapterTitle names a scene's chapter: its location, or a neutral fallback for
// an unlocated scene, which has no place name to borrow.
func chapterTitle(sc Scene) string {
	if title := strings.TrimSpace(sc.LocationName); title != "" {
		return title
	}
	return "Scene"
}

// stateString reads a string field from an entity's state, or empty when it is
// absent or not a string.
func stateString(ent *entity.Entity, key string) string {
	if ent == nil || ent.State == nil {
		return ""
	}
	raw, ok := ent.State.Get(key)
	if !ok {
		return ""
	}
	if value, ok := raw.(string); ok {
		return value
	}
	return ""
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
		sc.Weather = stateString(location, "weather")

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
// counted and skipped so one silent line cannot abandon the export, and the first
// failure is remembered so the export can say why it could not speak.
func (c *Compiler) resolveAudio(ctx context.Context, beat *Beat, segment entity.TurnSegment, silent *silence) audioOutcome {
	paths, duration, err := c.speech.SegmentAudio(ctx, segment)
	if errors.Is(err, ErrNoSpeakableText) {
		// The beat is deliberately not spoken, so it is not counted as a beat that should
		// have audio and nothing is reported about it.
		return outcomeUnspoken
	}
	if err != nil || len(paths) == 0 {
		silent.note(err)
		return outcomeSilent
	}

	beat.AudioPaths = paths
	beat.AudioDuration = duration
	return outcomeClips
}

// audioOutcome is what resolving one beat's audio produced.
type audioOutcome int

const (
	// outcomeClips is a beat with audio.
	outcomeClips audioOutcome = iota
	// outcomeUnspoken is a beat that is never spoken, so it needs no clip.
	outcomeUnspoken
	// outcomeSilent is a beat that should speak and has no clip.
	outcomeSilent
)

// errNoSpeechProvider reports an export that asked for audio and has no provider to
// synthesize it, which is the commonest reason a bundle is silent.
var errNoSpeechProvider = errors.New("no TTS provider is configured")

// coverageReport says how many beats that can speak have a clip, split by kind, so a caller
// can see whether a silence is narration or a character's own line. Only beats that can speak
// are counted: a scene card is a title rather than a line and is never spoken, so it is not a
// spoken beat and has no place in the total.
func coverageReport(spoken, withAudio int, byKind map[BeatKind][2]int) string {
	parts := make([]string, 0, 2)
	for _, kind := range []BeatKind{BeatNarration, BeatSpeech} {
		if entry, ok := byKind[kind]; ok {
			parts = append(parts, fmt.Sprintf("%s %d/%d", kind, entry[0], entry[0]+entry[1]))
		}
	}

	return fmt.Sprintf("audio: %d of %d spoken beats have clips (%s)", withAudio, spoken, strings.Join(parts, ", "))
}

// silence counts the beats that resolved no clip and remembers why, so an export reports
// what it could not speak rather than only how much.
type silence struct {
	beats int
	first error
}

func (s *silence) note(err error) {
	s.beats++
	if s.first == nil && err != nil {
		s.first = err
	}
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
