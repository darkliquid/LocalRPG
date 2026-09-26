package export

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/theater"
)

// VideoPipeline renders a script to a video file.
type VideoPipeline struct {
	rootDir string
	width   int
	height  int
	fps     int
	still   bool
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

// frameWriter renders a script's frames into a directory as PNGs.
type frameWriter struct {
	dir      string
	fps      int
	width    int
	height   int
	still    bool
	progress func(format string, args ...any)
}

// write renders every beat's frames in order, numbering them so FFmpeg can read
// the directory as a sequence.
func (w *frameWriter) write(script *scene.Script) (int, error) {
	if w.still {
		number := 0
		for si := range script.Scenes {
			for bi := range script.Scenes[si].Beats {
				frame := theater.Frame{Script: script, SceneIdx: si, BeatIdx: bi, Progress: 1}
				if err := w.writeFrame(number, frame); err != nil {
					return 0, err
				}
				number++
			}
		}
		return number, nil
	}

	total := theater.FrameCount(script, w.fps)
	for number := 0; number < total; number++ {
		if err := w.writeFrame(number, theater.BeatProgress(script, number, w.fps)); err != nil {
			return 0, err
		}
	}
	return total, nil
}

func (w *frameWriter) writeFrame(number int, frame theater.Frame) error {
	path := filepath.Join(w.dir, fmt.Sprintf("frame-%06d.png", number))
	if err := theater.WriteFramePNG(path, frame, w.width, w.height); err != nil {
		return fmt.Errorf("write frame %q: %w", path, err)
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

	framesDir, err := os.MkdirTemp("", "localrpg-frames-")
	if err != nil {
		return fmt.Errorf("create frames dir: %w", err)
	}
	defer os.RemoveAll(framesDir)

	writer := &frameWriter{
		dir:      framesDir,
		fps:      v.fps,
		width:    v.width,
		height:   v.height,
		still:    v.still,
		progress: func(format string, args ...any) {
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
