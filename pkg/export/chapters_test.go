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

// twoSceneScript is a script whose chapters are set, as compilation would set
// them, so the export tests do not need a full campaign.
func twoSceneScript() *scene.Script {
	script := smallScript()
	script.Scenes = append(script.Scenes, scene.Scene{
		LocationID:   "aldon-harbour",
		LocationName: "Aldon Harbour",
		Duration:     4 * time.Second,
		Beats: []scene.Beat{
			{Kind: scene.BeatSceneCard, Text: "Aldon Harbour", Duration: 2 * time.Second},
			{Kind: scene.BeatNarration, Text: "Salt air.", Duration: 2 * time.Second},
		},
	})
	script.TotalDuration = 8 * time.Second
	script.Chapters = []scene.Chapter{
		{Title: "Alden Tavern", Start: 0},
		{Title: "Aldon Harbour", Start: 4 * time.Second},
	}
	return script
}

func TestWebBundleCarriesChapters(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	if _, err := testExporter().Export(context.Background(), twoSceneScript(), out); err != nil {
		t.Fatalf("Export: %v", err)
	}
	payload := readWebPayload(t, out)
	if len(payload.Chapters) != 2 {
		t.Fatalf("chapters = %+v, want 2", payload.Chapters)
	}
	if payload.Chapters[0].Title != "Alden Tavern" || payload.Chapters[1].Start != 4 {
		t.Fatalf("unexpected chapters: %+v", payload.Chapters)
	}
	if !strings.Contains(payload.ChaptersVTT, "WEBVTT") || !strings.Contains(payload.ChaptersVTT, "Aldon Harbour") {
		t.Fatalf("chapters vtt = %q", payload.ChaptersVTT)
	}
}

func TestVideoExportWritesChapterSidecar(t *testing.T) {
	pipeline := NewVideoPipeline(".")
	pipeline.SetSize(64, 48)
	pipeline.SetFPS(5)

	out := filepath.Join(t.TempDir(), "replay.webm")
	if err := pipeline.RenderVideo(context.Background(), twoSceneScript(), out); err != nil {
		t.Fatalf("RenderVideo: %v", err)
	}

	sidecar := ChapterSidecarPath(out)
	data, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("expected a chapter sidecar at %q: %v", sidecar, err)
	}
	body := string(data)
	if !strings.Contains(body, ";FFMETADATA1") {
		t.Fatalf("sidecar = %q, want the ffmpeg metadata header", body)
	}
	if !strings.Contains(body, "title=Alden Tavern") || !strings.Contains(body, "title=Aldon Harbour") {
		t.Fatalf("sidecar = %q, want both chapter titles", body)
	}
}
