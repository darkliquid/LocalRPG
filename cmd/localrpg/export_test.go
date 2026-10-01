// cmd/localrpg/export_test.go
package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

func TestCLIExportHelp(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "export", "--help")
	out, err := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "Usage: localrpg export") && !strings.Contains(output, "Usage of export") {
		t.Errorf("unexpected output: %s, err: %v", output, err)
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
