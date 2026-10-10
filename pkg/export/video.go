package export

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/media/webm"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/scene"
)

// defaultQuality is the VP8 quality used when a caller asks for none.
const defaultQuality = 80

// DefaultEffort is the encoder effort a caller gets when it asks for none.
const DefaultEffort = webm.DefaultMethod

// Buffer defaults. The intro and outro hold the picture before the story starts
// and after it ends, so a player does not open or close on a hard cut, and the
// gap is held after every beat so one segment does not run into the next.
const (
	defaultIntro = 1500 * time.Millisecond
	defaultOutro = 1500 * time.Millisecond
	defaultGap   = 300 * time.Millisecond
)

// keyframeEvery is how often a keyframe is forced inside a beat. Inter frames
// predict from the one before, so error accumulates across a run of them; a
// keyframe resets it, keeps a seek close to where it was asked for, and is what
// stops a long beat's text slowly smearing.
const keyframeEvery = 2 * time.Second

// VideoPipeline renders a script to a WebM file, entirely in Go.
type VideoPipeline struct {
	rootDir     string
	width       int
	height      int
	fps         int
	quality     int
	effort      int
	still       bool
	intro       time.Duration
	outro       time.Duration
	gap         time.Duration
	displayMode scene.DisplayMode
	progress    scene.ProgressFunc
}

// NewVideoPipeline builds a renderer rooted at a campaign directory.
func NewVideoPipeline(rootDir string) *VideoPipeline {
	return &VideoPipeline{
		rootDir:     rootDir,
		width:       1920,
		height:      1080,
		fps:         scene.DefaultFPS,
		quality:     defaultQuality,
		effort:      webm.DefaultMethod,
		intro:       defaultIntro,
		outro:       defaultOutro,
		gap:         defaultGap,
		displayMode: scene.DisplayStageDirections,
	}
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

// SetQuality changes the VP8 quality, 0-100.
func (v *VideoPipeline) SetQuality(quality int) {
	if quality > 0 && quality <= 100 {
		v.quality = quality
	}
}

// SetEffort changes the encoder's quality/speed trade-off, 0-6. Higher methods
// search harder for the best mode, which is slower but cleaner on detailed art
// and coloured text.
func (v *VideoPipeline) SetEffort(effort int) {
	if effort >= 0 && effort <= 6 {
		v.effort = effort
	}
}

// SetIntro holds the opening picture for d before the story starts.
func (v *VideoPipeline) SetIntro(d time.Duration) {
	if d >= 0 {
		v.intro = d
	}
}

// SetOutro holds the closing picture for d after the story ends.
func (v *VideoPipeline) SetOutro(d time.Duration) {
	if d >= 0 {
		v.outro = d
	}
}

// SetGap holds every beat for d longer than the script paces it, so one segment
// does not run straight into the next.
func (v *VideoPipeline) SetGap(d time.Duration) {
	if d >= 0 {
		v.gap = d
	}
}

// SetStill disables animation: one fully revealed frame per beat.
func (v *VideoPipeline) SetStill(still bool) { v.still = still }

// SetDisplayMode chooses how performance tags render.
func (v *VideoPipeline) SetDisplayMode(mode scene.DisplayMode) {
	if mode != "" {
		v.displayMode = mode
	}
}

// SetProgress routes structured progress to fn.
func (v *VideoPipeline) SetProgress(fn scene.ProgressFunc) { v.progress = fn }

// RenderVideo draws every frame, muxes the campaign's audio, and renames the
// result into place, so a failed or cancelled render leaves no file behind.
func (v *VideoPipeline) RenderVideo(ctx context.Context, script *scene.Script, outputFile string) error {
	cleanOut, err := pathutil.ValidateUserPath(outputFile)
	if err != nil {
		return fmt.Errorf("invalid video output path: %w", err)
	}
	outputFile = cleanOut

	if script == nil || len(script.Scenes) == 0 {
		return fmt.Errorf("render video: script has no scenes")
	}

	renderer, err := scene.NewRenderer(v.width, v.height)
	if err != nil {
		return fmt.Errorf("build renderer: %w", err)
	}

	track, err := v.opusTrack(script)
	if err != nil {
		return err
	}

	part := stagingPath(outputFile)
	file, err := os.Create(part)
	if err != nil {
		return fmt.Errorf("create video: %w", err)
	}

	totals, err := v.writeFrames(ctx, renderer, script, file, track)
	if err != nil {
		closeQuietly(file)
		os.Remove(part)
		return err
	}
	// The muxer owns the writer and has already closed it.
	if err := os.Rename(part, outputFile); err != nil {
		os.Remove(part)
		return fmt.Errorf("publish video: %w", err)
	}

	// The subtitle track goes beside the video with the same base name. A muxed
	// WebM subtitle is possible, but a sidecar plays in every player.
	sidecar, err := writeSubtitleSidecar(outputFile, script)
	if err != nil {
		os.Remove(outputFile)
		return err
	}
	if v.progress != nil && sidecar != "" {
		v.progress(scene.Progress{Phase: "encode", Message: "wrote " + filepath.Base(sidecar)})
	}

	// The chapters go beside it too, in the ffmpeg metadata format.
	chapters, err := writeChapterSidecar(outputFile, script)
	if err != nil {
		os.Remove(outputFile)
		return err
	}
	if v.progress != nil && chapters != "" {
		v.progress(scene.Progress{Phase: "encode", Message: "wrote " + filepath.Base(chapters)})
	}

	if v.progress != nil {
		v.progress(scene.Progress{
			Phase:             "done",
			Done:              totals.frames,
			Total:             totals.plan.Frames.Total,
			Frames:            totals.frames,
			ImageFrames:       totals.imageFrames,
			RepeatFrames:      totals.repeatFrames,
			AudioPackets:      totals.audioPackets,
			TotalAudioPackets: totals.plan.AudioPackets,
			AudioBytes:        totals.audioBytes,
			TotalAudioBytes:   totals.plan.AudioBytes,
			Elapsed:           totals.elapsed,
			Length:            totals.plan.Length,
		})
	}
	return nil
}

// opusTrack lays every beat's clips on one continuous timeline, padding the gaps
// left by a beat with no clip.
// opusTrack lays every beat's clips on one continuous timeline, holding each beat
// for its own span and adding the intro and outro buffers.
func (v *VideoPipeline) opusTrack(script *scene.Script) (*webm.OpusTrack, error) {
	track := webm.NewOpusTrack(1)
	if v.intro > 0 {
		if err := track.AppendSilence(v.intro); err != nil {
			return nil, err
		}
	}

	for _, beat := range script.Beats() {
		before := track.Duration()
		for _, path := range beat.AudioPaths {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read clip %q: %w", path, err)
			}
			if err := track.AppendClip(data); err != nil {
				return nil, err
			}
		}

		// Hold the beat for its whole span, not just the length of its clips. A
		// beat is a clip plus a gap, and without the gap every spoken beat ends
		// early and the whole track creeps ahead of the picture.
		played := track.Duration() - before
		if hold := beat.Duration - played + v.gap; hold > 0 {
			if err := track.AppendSilence(hold); err != nil {
				return nil, err
			}
		}
	}

	// The outro is padded with real silence packets rather than left as a gap: a
	// player whose clock follows the audio would otherwise reach the end of the
	// audio's packets and stop, with the closing picture still to come.
	if v.outro > 0 {
		if err := track.AppendSilencePadded(v.outro); err != nil {
			return nil, err
		}
	}
	return track, nil
}

// writeFrames walks the beats, renders and encodes each frame, and feeds the
// muxer, which interleaves the audio it was given up front.
func (v *VideoPipeline) writeFrames(ctx context.Context, renderer *scene.Renderer, script *scene.Script, file *os.File, track *webm.OpusTrack) (renderTotals, error) {
	muxer, err := webm.NewMuxer(file, v.width, v.height, track)
	if err != nil {
		return renderTotals{}, err
	}
	encoder := webm.NewEncoder(v.width, v.height, v.quality)
	encoder.SetMethod(v.effort)

	animate := !v.still
	plan := NewRenderPlan(script, track, v.fps, animate, v.intro, v.outro, v.gap)

	started := time.Now()
	elapsed := time.Duration(0)
	previousArt := ""
	// The first frame is always a keyframe, so the last one starts out "overdue".
	lastKeyframe := -keyframeEvery

	totals := renderTotals{plan: plan}
	var lastImage *image.RGBA

	// Progress is throttled so a long render reports steadily without flooding a
	// subscriber; a completed phase always reports.
	lastReport := time.Time{}
	report := func(force bool) {
		if v.progress == nil {
			return
		}
		if !force && time.Since(lastReport) < 250*time.Millisecond {
			return
		}
		lastReport = time.Now()
		packets, bytes := muxer.AudioWritten()
		v.progress(scene.Progress{
			Phase:             "frames",
			Done:              totals.frames,
			Total:             plan.Frames.Total,
			Frames:            totals.frames,
			ImageFrames:       totals.imageFrames,
			RepeatFrames:      totals.repeatFrames,
			AudioPackets:      packets,
			TotalAudioPackets: plan.AudioPackets,
			AudioBytes:        bytes,
			TotalAudioBytes:   plan.AudioBytes,
			Elapsed:           time.Since(started),
			Length:            plan.Length,
		})
	}

	// writeFrame encodes and writes one frame at an explicit timestamp.
	//
	// The muxer is told exactly what the encoder produced: marking an inter frame
	// as a keyframe starts a cluster a decoder cannot reconstruct, which corrupts
	// the picture until the next real keyframe.
	writeFrame := func(img *image.RGBA, at time.Duration, forceKeyframe bool) error {
		keyframe := forceKeyframe || at-lastKeyframe >= keyframeEvery
		data, err := encoder.Encode(img, keyframe)
		if err != nil {
			return err
		}
		if err := muxer.WriteVideo(data, keyframe, at); err != nil {
			return err
		}
		if keyframe {
			lastKeyframe = at
		}
		totals.frames++
		report(false)
		return nil
	}

	// emit writes a frame at the current time and advances the timeline by span.
	emit := func(img *image.RGBA, span time.Duration, forceKeyframe bool) error {
		if err := writeFrame(img, elapsed, forceKeyframe); err != nil {
			return err
		}
		elapsed += span
		return nil
	}

	report(true)

	// The intro holds the opening stage before anything is written on it, so the
	// video opens on a scene rather than on a line of dialogue.
	if v.intro > 0 && len(script.Scenes) > 0 && len(script.Scenes[0].Beats) > 0 {
		if err := ctx.Err(); err != nil {
			return totals, err
		}
		opening := renderer.Frame(scene.FrameRequest{
			Script:      script,
			SceneIndex:  0,
			Scene:       script.Scenes[0],
			Beat:        script.Scenes[0].Beats[0],
			Progress:    0,
			Animate:     true,
			DisplayMode: v.displayMode,
		})
		lastImage = opening
		totals.imageFrames++
		if err := emit(opening, v.intro, true); err != nil {
			return totals, err
		}
	}

	for index := range script.Scenes {
		sc := script.Scenes[index]
		for beatIndex, beat := range sc.Beats {
			if err := ctx.Err(); err != nil {
				return totals, err
			}
			// A beat opens on new content, so it opens on a keyframe: prediction
			// error from the beat before would otherwise smear the new text.
			beatStart := true
			sceneStart := beatIndex == 0
			for _, step := range scene.BeatFramePlan(beat, v.fps, animate) {
				// A repeat frame is the frame before it, so it is re-encoded rather
				// than drawn again: the heartbeat costs an inter frame, not a render.
				var img *image.RGBA
				if step.Repeat && lastImage != nil {
					img = lastImage
					totals.repeatFrames++
				} else {
					img = renderer.Frame(scene.FrameRequest{
						Script:      script,
						SceneIndex:  index,
						Scene:       sc,
						Beat:        beat,
						Progress:    step.Progress,
						PreviousArt: previousArt,
						Animate:     animate,
						DisplayMode: v.displayMode,
					})
					lastImage = img
					totals.imageFrames++
				}

				// A scene's opening crossfade blends two pictures, which is the
				// most an inter frame has to carry, so it gets keyframes too.
				crossfading := sceneStart && step.Progress < scene.CrossfadeShare
				if err := emit(img, step.Span, beatStart || crossfading); err != nil {
					return totals, err
				}
				beatStart = false
			}

			// Hold the beat's closing picture for the extra gap, so one segment
			// does not run straight into the next.
			if v.gap > 0 && lastImage != nil {
				totals.repeatFrames++
				if err := emit(lastImage, v.gap, false); err != nil {
					return totals, err
				}
			}
		}
		previousArt = sc.ArtPath
	}

	// The outro holds the closing picture after the last line, and then a closing
	// block goes at the very end of the timeline.
	//
	// That last block is what gives the picture its time: a file's duration is its
	// last block's timestamp, so without one the closing frame has no time to be
	// shown at all and the video stops the moment it is drawn.
	if v.outro > 0 {
		elapsed += v.outro
	}
	if lastImage != nil {
		totals.repeatFrames++
		if err := writeFrame(lastImage, elapsed, false); err != nil {
			return totals, err
		}
	}

	if v.progress != nil {
		packets, bytes := muxer.AudioWritten()
		v.progress(scene.Progress{
			Phase:             "encode",
			Done:              totals.frames,
			Total:             plan.Frames.Total,
			Frames:            totals.frames,
			ImageFrames:       totals.imageFrames,
			RepeatFrames:      totals.repeatFrames,
			AudioPackets:      packets,
			TotalAudioPackets: plan.AudioPackets,
			AudioBytes:        bytes,
			TotalAudioBytes:   plan.AudioBytes,
			Elapsed:           time.Since(started),
			Length:            plan.Length,
		})
	}
	if err := muxer.Close(); err != nil {
		return totals, err
	}
	totals.audioPackets, totals.audioBytes = muxer.AudioWritten()
	totals.elapsed = time.Since(started)
	return totals, nil
}

// renderTotals is what a finished render produced, so the caller can publish a
// final progress snapshot once the file is in place.
type renderTotals struct {
	plan         RenderPlan
	frames       int
	imageFrames  int
	repeatFrames int
	audioPackets int
	audioBytes   int64
	elapsed      time.Duration
}

// stagingPath names the file written before it is published.
func stagingPath(outputFile string) string {
	ext := filepath.Ext(outputFile)
	if ext == "" {
		return outputFile + ".part"
	}
	return strings.TrimSuffix(outputFile, ext) + ".part" + ext
}

// closeQuietly closes a file that is being discarded, tolerating a file the muxer
// already closed.
func closeQuietly(file *os.File) {
	if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		// The file is being thrown away; there is nothing useful to report.
		return
	}
}
