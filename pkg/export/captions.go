package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/scene"
)

// SubtitlePath names the WebVTT sidecar beside a video: the same base name with a
// .vtt extension, which is what a player looks for when it is handed the video.
func SubtitlePath(videoPath string) string {
	return strings.TrimSuffix(videoPath, filepath.Ext(videoPath)) + ".vtt"
}

// writeSubtitleSidecar writes a script's captions beside its video, so a player
// that reads a sidecar can show the spoken lines. It returns the sidecar's path,
// or empty when there is no script to caption.
func writeSubtitleSidecar(videoPath string, script *scene.Script) (string, error) {
	if script == nil {
		return "", nil
	}
	path := SubtitlePath(videoPath)
	if err := os.WriteFile(path, []byte(scene.Captions(script.Beats())), 0644); err != nil {
		return "", fmt.Errorf("write subtitle sidecar: %w", err)
	}
	return path, nil
}
