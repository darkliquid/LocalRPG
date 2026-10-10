package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

// captionedScript is the small video fixture plus one spoken beat, so a subtitle
// track has a cue.
func captionedScript() *scene.Script {
	script := smallScript()
	script.Scenes[0].Beats = append(script.Scenes[0].Beats, scene.Beat{
		Kind: scene.BeatSpeech, Speaker: "Garrick", Text: "Welcome.", Duration: 2 * time.Second,
	})
	return script
}

// TestWebBundleCarriesCaptions proves the exported page carries the WebVTT track,
// so a viewer who cannot hear the audio still reads the spoken lines.
func TestWebBundleCarriesCaptions(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	if _, err := testExporter().Export(context.Background(), captionedScript(), out); err != nil {
		t.Fatalf("Export: %v", err)
	}
	payload := readWebPayload(t, out)
	if !strings.Contains(payload.Captions, "WEBVTT") {
		t.Fatalf("captions = %q, want a WEBVTT document", payload.Captions)
	}
	if !strings.Contains(payload.Captions, "Welcome.") {
		t.Fatalf("captions = %q, want the spoken line", payload.Captions)
	}
}

// TestVideoExportWritesASidecar proves a video export leaves the subtitle track
// beside it with the same base name.
func TestVideoExportWritesASidecar(t *testing.T) {
	pipeline := NewVideoPipeline(".")
	pipeline.SetSize(64, 48)
	pipeline.SetFPS(5)

	out := filepath.Join(t.TempDir(), "replay.webm")
	if err := pipeline.RenderVideo(context.Background(), captionedScript(), out); err != nil {
		t.Fatalf("RenderVideo: %v", err)
	}

	sidecar := SubtitlePath(out)
	data, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("expected a subtitle sidecar at %q: %v", sidecar, err)
	}
	if !strings.Contains(string(data), "WEBVTT") || !strings.Contains(string(data), "Welcome.") {
		t.Fatalf("sidecar = %q", data)
	}
}
