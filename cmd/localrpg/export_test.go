// cmd/localrpg/export_test.go
package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

func TestRunExportCommandTable(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{
			name:     "empty args prints usage",
			args:     nil,
			wantCode: 0,
		},
		{
			name:       "unknown flag fails",
			args:       []string{"--unknown-flag"},
			wantCode:   1,
			wantStderr: "flag provided but not defined",
		},
		{
			name:       "inspect missing path",
			args:       []string{"inspect"},
			wantCode:   1,
			wantStderr: "Error: missing bundle path",
		},
		{
			name:       "inspect nonexistent file",
			args:       []string{"inspect", "nonexistent.zip"},
			wantCode:   1,
			wantStderr: "Inspect failed:",
		},
		{
			name:       "missing game id",
			args:       []string{"web"},
			wantCode:   1,
			wantStderr: "Error: missing game ID",
		},
		{
			name:       "compile nonexistent game fails",
			args:       []string{"web", "nonexistent-game-xyz"},
			wantCode:   1,
			wantStderr: "Failed to compile replay script",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runExportCommand(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestResolveOutPathNamesTheFileAfterTheGame(t *testing.T) {
	if got := resolveOutPath("", "campaign-01", ".webm"); got != "campaign-01.webm" {
		t.Errorf("empty out = %q, want campaign-01.webm", got)
	}

	dir := t.TempDir()
	if got, want := resolveOutPath(dir, "campaign-01", ".webm"), filepath.Join(dir, "campaign-01.webm"); got != want {
		t.Errorf("directory out = %q, want %q", got, want)
	}

	nested := filepath.Join(t.TempDir(), "nested")
	if got, want := resolveOutPath(nested+string(os.PathSeparator), "campaign-01", ".webm"), filepath.Join(nested, "campaign-01.webm"); got != want {
		t.Errorf("trailing separator out = %q, want %q", got, want)
	}

	chosen := filepath.Join(dir, "chosen.webm")
	if got := resolveOutPath(chosen, "campaign-01", ".webm"); got != chosen {
		t.Errorf("file out = %q, want %q", got, chosen)
	}
}

func TestSplitExportArgsFindsFlagsAnywhere(t *testing.T) {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.String("dir", ".", "")
	fs.Bool("still", false, "")

	flags, positional := splitExportArgs(fs, []string{"video", "--dir", "x", "--still", "game"})
	if got := strings.Join(positional, ","); got != "video,game" {
		t.Errorf("positional = %q, want video,game", got)
	}
	if got := strings.Join(flags, " "); got != "--dir x --still" {
		t.Errorf("flags = %q, want \"--dir x --still\"", got)
	}
}

func TestFormatProgressReportsFramesAndAudio(t *testing.T) {
	line := formatProgress(scene.Progress{
		Phase:             "frames",
		Done:              50,
		Total:             100,
		Frames:            50,
		ImageFrames:       40,
		RepeatFrames:      10,
		AudioPackets:      5,
		TotalAudioPackets: 20,
		AudioBytes:        1024,
		TotalAudioBytes:   4096,
		Elapsed:           2 * time.Second,
	}, 10)

	for _, want := range []string{" 50%", "50/100 frames", "40 new", "10 repeat", "5/20 audio", "1 KB/4 KB", "2s"} {
		if !strings.Contains(line, want) {
			t.Errorf("progress line %q is missing %q", line, want)
		}
	}
}
