package export

import (
	"context"
	"image/png"
	"os"
	"os/exec"
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
	dir := t.TempDir()
	writer := &frameWriter{dir: dir, fps: 5, width: 160, height: 90, still: true}

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
	dir := t.TempDir()
	writer := &frameWriter{dir: dir, fps: 10, width: 64, height: 36}

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
		"anullsrc=r=44100:cl=stereo", // the silent card needs its own input
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

func TestBuildCommandGivesAZeroLengthBeatTheFloor(t *testing.T) {
	pipeline := NewVideoPipeline(".")

	script := smallScript()
	script.Scenes[0].Beats[0].Duration = 0

	cmd, err := pipeline.BuildCommand(context.Background(), script, "/frames", "/out.mp4")
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}

	if args := strings.Join(cmd.Args, " "); !strings.Contains(args, "-t 2.000") {
		t.Errorf("expected the minimum beat duration as a silence length:\n%s", args)
	}
}

func TestBuildCommandRequiresScenes(t *testing.T) {
	if _, err := NewVideoPipeline(".").BuildCommand(context.Background(), &scene.Script{}, "/frames", "/out.mp4"); err == nil {
		t.Errorf("expected an error for a script with no scenes")
	}
}

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
	if _, err := os.Stat(stagingPath(out)); !os.IsNotExist(err) {
		t.Errorf("expected the staging file to be renamed away, stat err = %v", err)
	}
}

func TestStagingPathKeepsTheContainerExtension(t *testing.T) {
	// FFmpeg picks its muxer from the extension, so the staging file cannot be
	// named with a bare ".part" suffix.
	if got := stagingPath("/tmp/replay.mp4"); got != "/tmp/replay.part.mp4" {
		t.Errorf("stagingPath = %q, want a .part file that still ends in .mp4", got)
	}
	if got := stagingPath("/tmp/replay"); got != "/tmp/replay.part" {
		t.Errorf("stagingPath without an extension = %q", got)
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
	if _, err := os.Stat(stagingPath(out)); !os.IsNotExist(err) {
		t.Errorf("expected no leftover staging file, stat err = %v", err)
	}
}

func TestRenderVideoRequiresScenes(t *testing.T) {
	if err := NewVideoPipeline(".").RenderVideo(context.Background(), &scene.Script{}, "out.mp4"); err == nil {
		t.Errorf("expected an error for a script with no scenes")
	}
}

func TestVideoPipelineSettersIgnoreNonsense(t *testing.T) {
	pipeline := NewVideoPipeline(".")

	pipeline.SetSize(0, -1)
	pipeline.SetFPS(0)

	if pipeline.width != 1920 || pipeline.height != 1080 {
		t.Errorf("size = %dx%d, want the defaults to survive", pipeline.width, pipeline.height)
	}
	if pipeline.fps != scene.DefaultFPS {
		t.Errorf("fps = %d, want the default to survive", pipeline.fps)
	}
}
