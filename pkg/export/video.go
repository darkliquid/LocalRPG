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
	"github.com/darkliquid/localrpg/pkg/scene"
)

// defaultQuality is the VP8 quality used when a caller asks for none.
const defaultQuality = 80

// VideoPipeline renders a script to a WebM file, entirely in Go.
type VideoPipeline struct {
	rootDir     string
	width       int
	height      int
	fps         int
	quality     int
	still       bool
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
func (v *VideoPipeline) opusTrack(script *scene.Script) (*webm.OpusTrack, error) {
	track := webm.NewOpusTrack(1)
	for _, beat := range script.Beats() {
		if len(beat.AudioPaths) == 0 {
			if err := track.AppendSilence(beat.Duration); err != nil {
				return nil, err
			}
			continue
		}
		for _, path := range beat.AudioPaths {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read clip %q: %w", path, err)
			}
			if err := track.AppendClip(data); err != nil {
				return nil, err
			}
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

	animate := !v.still
	plan := NewRenderPlan(script, track, v.fps, animate)

	started := time.Now()
	elapsed := time.Duration(0)
	previousArt := ""
	first := true

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
	report(true)

	for index := range script.Scenes {
		sc := script.Scenes[index]
		for _, beat := range sc.Beats {
			if err := ctx.Err(); err != nil {
				return totals, err
			}
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

				data, err := encoder.Encode(img, first)
				if err != nil {
					return totals, err
				}
				if err := muxer.WriteVideo(data, first, elapsed); err != nil {
					return totals, err
				}
				first = false
				totals.frames++
				elapsed += step.Span
				report(false)
			}
		}
		previousArt = sc.ArtPath
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
