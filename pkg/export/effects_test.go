package export

import (
	"context"
	"path/filepath"
	"testing"
)

// TestWebBundleCarriesEffects proves the payload a player reads carries the
// outcome and the weather the export's renderer used, so the web player's stage
// draws the same tint and overlay as the app and the video.
func TestWebBundleCarriesEffects(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()
	script := fixtureScript(t, dir)
	script.Scenes[0].Weather = "rain"
	script.Scenes[0].Beats[2].Outcome = "miss"

	if _, err := testExporter().Export(context.Background(), script, out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	payload := readWebPayload(t, out)
	if len(payload.Scenes) == 0 {
		t.Fatal("the payload has no scenes")
	}
	if payload.Scenes[0].Weather != "rain" {
		t.Errorf("weather = %q, want rain", payload.Scenes[0].Weather)
	}
	if payload.Scenes[0].Beats[2].Outcome != "miss" {
		t.Errorf("outcome = %q, want miss", payload.Scenes[0].Beats[2].Outcome)
	}
}
