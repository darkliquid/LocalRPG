package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

// ChapterSidecarPath names the chapter sidecar beside a video: the same base name
// with a .chapters.txt extension.
func ChapterSidecarPath(videoPath string) string {
	return strings.TrimSuffix(videoPath, filepath.Ext(videoPath)) + ".chapters.txt"
}

// writeChapterSidecar writes a script's chapters beside its video in the ffmpeg
// metadata format. A WebM chapters element is possible, but the muxer does not
// write one, so a sidecar is the documented fallback: it plays in every tool that
// reads ffmpeg metadata. It returns the sidecar's path, or empty when there is no
// script or no chapter to write.
func writeChapterSidecar(videoPath string, script *scene.Script) (string, error) {
	if script == nil || len(script.Chapters) == 0 {
		return "", nil
	}
	var builder strings.Builder
	builder.WriteString(";FFMETADATA1\n")
	for i, chapter := range script.Chapters {
		end := script.TotalDuration
		if i+1 < len(script.Chapters) {
			end = script.Chapters[i+1].Start
		}
		if end <= chapter.Start {
			end = chapter.Start + time.Second
		}
		fmt.Fprintf(&builder, "[CHAPTER]\nTIMEBASE=1/1000\nSTART=%d\nEND=%d\ntitle=%s\n",
			chapter.Start.Milliseconds(), end.Milliseconds(), chapter.Title)
	}
	path := ChapterSidecarPath(videoPath)
	// The video path was validated by the caller with pathutil.ValidateUserPath.
	// lgtm[go/path-injection]
	if err := os.WriteFile(path, []byte(builder.String()), 0644); err != nil {
		return "", fmt.Errorf("write chapter sidecar: %w", err)
	}
	return path, nil
}
