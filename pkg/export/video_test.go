// pkg/export/video_test.go
package export

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFFmpegCommand(t *testing.T) {
	tempDir := t.TempDir()
	pipeline := NewVideoPipeline(tempDir)

	script := &ReplayScript{
		GameID:        "test-game",
		GameName:      "Test Campaign",
		TotalDuration: 10.0,
		Beats: []SceneBeat{
			{
				TurnNumber:  1,
				Prose:       "A lone hero approaches.",
				DurationSec: 5.0,
			},
			{
				TurnNumber:  2,
				Speaker:     "Guard",
				Dialogue:    "Halt!",
				DurationSec: 5.0,
			},
		},
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
}
