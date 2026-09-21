package export

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

func TestBuildFFmpegCommand(t *testing.T) {
	tempDir := t.TempDir()
	pipeline := NewVideoPipeline(tempDir)

	script := &scene.Script{
		GameID:        "test-game",
		GameName:      "Test Campaign",
		TotalDuration: 10 * time.Second,
		Scenes: []scene.Scene{{
			LocationID: "tavern",
			Beats: []scene.Beat{
				{Kind: scene.BeatNarration, Text: "A lone hero approaches.", Duration: 5 * time.Second},
				{Kind: scene.BeatSpeech, Speaker: "Guard", Text: "Halt!", Duration: 5 * time.Second},
			},
		}},
	}

	outFile := filepath.Join(tempDir, "output.mp4")
	cmd, err := pipeline.BuildCommand(context.Background(), script, outFile)
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}

	if cmd == nil || len(cmd.Args) == 0 {
		t.Fatalf("expected non-empty command args")
	}

	argsStr := strings.Join(cmd.Args, " ")
	if !strings.Contains(argsStr, "ffmpeg") {
		t.Errorf("expected command to call ffmpeg, got: %s", argsStr)
	}
	if !strings.Contains(argsStr, "d=10.0") {
		t.Errorf("expected the script's duration to drive the clip, got: %s", argsStr)
	}
}

func TestBuildFFmpegCommandFallsBackToADefaultDuration(t *testing.T) {
	pipeline := NewVideoPipeline(t.TempDir())

	cmd, err := pipeline.BuildCommand(context.Background(), &scene.Script{}, "out.mp4")
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}
	if args := strings.Join(cmd.Args, " "); !strings.Contains(args, "d=5.0") {
		t.Errorf("expected a default duration for an empty script, got: %s", args)
	}
}
