package export

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/scene"
)

// VideoPipeline renders a script to a video file.
type VideoPipeline struct {
	rootDir  string
	width    int
	height   int
	fps      int
	still    bool
	progress scene.ProgressFunc
}

// NewVideoPipeline builds a renderer rooted at a campaign directory.
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

// SetProgress routes structured frame and encode progress to fn. When set, the
// pipeline stops writing human-readable progress to stderr.
func (v *VideoPipeline) SetProgress(fn scene.ProgressFunc) { v.progress = fn }

// FFmpegAvailable reports the ffmpeg binary path, or ok=false when it is absent.
func FFmpegAvailable() (string, bool) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", false
	}
	return path, true
}

// frameWriter renders a script's frames into a directory as PNGs.
type frameWriter struct {
	renderer *scene.Renderer
	dir      string
	fps      int
	still    bool
	progress scene.ProgressFunc
}

// write renders every beat's frames in order, numbering them so FFmpeg can read
// the directory as a sequence.
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
			w.progress(scene.Progress{Phase: "frames", Done: i + 1, Total: len(script.Scenes)})
		}
	}

	return number, nil
}

// writePNG encodes one frame. BestSpeed matters here: a 1080p frame is slow to
// compress, and the file size is irrelevant beside x264's encoding time.
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

	writerProgress := v.progress
	if writerProgress == nil {
		writerProgress = func(p scene.Progress) {
			fmt.Fprintf(os.Stderr, "export: %s %d/%d\n", p.Phase, p.Done, p.Total)
		}
	}
	writer := &frameWriter{
		renderer: renderer,
		dir:      framesDir,
		fps:      v.fps,
		still:    v.still,
		progress: writerProgress,
	}

	count, err := writer.write(script)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("render video: no frames were rendered")
	}

	if v.progress != nil {
		v.progress(scene.Progress{Phase: "encode"})
	}

	part := stagingPath(outputFile)
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
	if v.progress != nil {
		v.progress(scene.Progress{Phase: "done"})
	}
	return nil
}

// stagingPath names the file FFmpeg writes before it is published. It keeps the
// output's extension, because that is how FFmpeg picks a container: a plain
// ".part" suffix leaves it unable to choose a muxer at all.
func stagingPath(outputFile string) string {
	ext := filepath.Ext(outputFile)
	if ext == "" {
		return outputFile + ".part"
	}
	return strings.TrimSuffix(outputFile, ext) + ".part" + ext
}

// lastLines trims FFmpeg's output to the part worth showing a person.
func lastLines(output string, count int) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}
