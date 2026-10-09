package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/scene"
)

func writeArt(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestWebExportEmbedsTurnIllustration guards that a beat with its own scene
// illustration embeds that image rather than the scene's backdrop, so an exported
// story shows the moment the app showed.
func TestWebExportEmbedsTurnIllustration(t *testing.T) {
	isolateConfig(t)
	dir := t.TempDir()

	backdrop := writeArt(t, dir, "hall.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><rect width="8" height="8" fill="#112233"/></svg>`))
	illustration := writeArt(t, dir, "turn-2.png", []byte("\x89PNG\r\n\x1a\nillustration"))
	plain := writeArt(t, dir, "turn-1.png", []byte("\x89PNG\r\n\x1a\nplain"))

	script := &scene.Script{
		GameName: "Campaign One",
		Scenes: []scene.Scene{{
			LocationName: "The Hall",
			ArtPath:      backdrop,
			Beats: []scene.Beat{
				{Kind: scene.BeatNarration, TurnNumber: 1, Text: "One.", ArtPath: plain},
				{Kind: scene.BeatNarration, TurnNumber: 2, Text: "Two.", ArtPath: illustration},
				{Kind: scene.BeatNarration, TurnNumber: 3, Text: "Three.", ArtPath: backdrop},
			},
		}},
	}

	out := filepath.Join(dir, "story.html")
	if _, err := testExporter().Export(context.Background(), script, out); err != nil {
		t.Fatalf("Export: %v", err)
	}

	story := readWebPayload(t, out)
	beats := story.Scenes[0].Beats
	if len(beats) != 3 {
		t.Fatalf("beats = %d", len(beats))
	}
	if beats[1].Art == "" {
		t.Fatal("the illustrated beat embedded no art")
	}
	if beats[1].Art == beats[0].Art {
		t.Fatal("the illustrated beat should differ from the plain beat")
	}
	if beats[2].Art == beats[1].Art {
		t.Fatal("a beat with no illustration should keep the backdrop")
	}
	if story.Scenes[0].Art == "" {
		t.Fatal("the scene backdrop should still embed")
	}
}

// TestWebExportWithoutIllustrationsIsUnchanged guards the flat path: a beat whose
// art is the scene's own resolves to the scene art, as it always did.
func TestWebExportWithoutIllustrationsIsUnchanged(t *testing.T) {
	isolateConfig(t)
	dir := t.TempDir()
	backdrop := writeArt(t, dir, "hall.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><rect width="8" height="8" fill="#112233"/></svg>`))

	script := &scene.Script{
		GameName: "Campaign One",
		Scenes: []scene.Scene{{
			LocationName: "The Hall",
			ArtPath:      backdrop,
			Beats: []scene.Beat{
				{Kind: scene.BeatNarration, TurnNumber: 1, Text: "One.", ArtPath: backdrop},
				{Kind: scene.BeatNarration, TurnNumber: 2, Text: "Two.", ArtPath: backdrop},
			},
		}},
	}

	out := filepath.Join(dir, "story.html")
	if _, err := testExporter().Export(context.Background(), script, out); err != nil {
		t.Fatalf("Export: %v", err)
	}
	story := readWebPayload(t, out)
	for i, beat := range story.Scenes[0].Beats {
		if beat.Art != story.Scenes[0].Art {
			t.Fatalf("beat %d art = %q, want the scene backdrop", i, beat.Art)
		}
	}
}
