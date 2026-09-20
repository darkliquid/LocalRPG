package export

import (
	"context"
	"fmt"
	"os/exec"
)

type VideoPipeline struct {
	rootDir string
}

func NewVideoPipeline(rootDir string) *VideoPipeline {
	return &VideoPipeline{rootDir: rootDir}
}

func (v *VideoPipeline) BuildCommand(ctx context.Context, script *ReplayScript, outputFile string) (*exec.Cmd, error) {
	duration := fmt.Sprintf("%.1f", script.TotalDuration)
	if script.TotalDuration <= 0 {
		duration = "5.0"
	}

	args := []string{
		"-y",
		"-f", "lavfi",
		"-i", fmt.Sprintf("color=c=#0c0a09:s=1920x1080:d=%s", duration),
		"-f", "lavfi",
		"-i", fmt.Sprintf("anullsrc=r=44100:cl=stereo:d=%s", duration),
		"-c:v", "libx264",
		"-tune", "stillimage",
		"-c:a", "aac",
		"-b:a", "192k",
		"-pix_fmt", "yuv420p",
		"-shortest",
		outputFile,
	}

	return exec.CommandContext(ctx, "ffmpeg", args...), nil
}

func (v *VideoPipeline) RenderVideo(ctx context.Context, script *ReplayScript, outputFile string) error {
	cmd, err := v.BuildCommand(ctx, script, outputFile)
	if err != nil {
		return fmt.Errorf("build command: %w", err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg execution failed (%v): %s", err, string(out))
	}

	return nil
}
