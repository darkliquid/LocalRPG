package media

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ProbeAudioDuration reads a clip's real length with ffprobe, which is what lets
// pacing follow the audio rather than the text for beats that have a clip.
func ProbeAudioDuration(ctx context.Context, path string) (time.Duration, error) {
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		path,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("probe %q: %w", path, err)
	}

	raw := strings.TrimSpace(string(out))
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("probe %q: parse %q: %w", path, raw, err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
