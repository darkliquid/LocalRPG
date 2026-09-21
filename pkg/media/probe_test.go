package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestProbeAudioDurationRejectsAMissingFile(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}

	if _, err := ProbeAudioDuration(context.Background(), filepath.Join(t.TempDir(), "absent.wav")); err == nil {
		t.Errorf("expected an error for a missing file")
	}
}

func TestProbeAudioDurationReadsAGeneratedTone(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}

	path := filepath.Join(t.TempDir(), "tone.wav")
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot generate a test tone: %v: %s", err, out)
	}

	duration, err := ProbeAudioDuration(context.Background(), path)
	if err != nil {
		t.Fatalf("ProbeAudioDuration failed: %v", err)
	}
	if duration < 1900*time.Millisecond || duration > 2100*time.Millisecond {
		t.Errorf("duration = %v, want about 2s", duration)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("probing must not consume the file: %v", err)
	}
}
