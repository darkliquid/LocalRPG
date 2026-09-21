package export

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

// isolateConfig points config loading at a throwaway directory so a test never
// picks up the developer's own providers.
func isolateConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("media:\n  tts:\n    type: disabled\n  image:\n    type: disabled\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALRPG_CONFIG_DIR", dir)
}

func fixtureScript(t *testing.T, dir string) *scene.Script {
	t.Helper()

	art := filepath.Join(dir, "tavern.svg")
	if err := os.WriteFile(art, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatal(err)
	}
	clip := filepath.Join(dir, "welcome.wav")
	if err := os.WriteFile(clip, []byte("RIFF....WAVEfmt ....data"), 0644); err != nil {
		t.Fatal(err)
	}

	return &scene.Script{
		GameID:   "campaign-01",
		GameName: "Campaign One",
		Scenes: []scene.Scene{{
			LocationID:   "alden-tavern",
			LocationName: "Alden Tavern",
			ArtPath:      art,
			Duration:     5 * time.Second,
			Beats: []scene.Beat{
				{Kind: scene.BeatSceneCard, Text: "Alden Tavern", ArtPath: art, Duration: 2 * time.Second},
				{Kind: scene.BeatNarration, Text: "Warm light.", ArtPath: art, Duration: 2 * time.Second},
				{Kind: scene.BeatSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Welcome.", ArtPath: art,
					AudioPath: clip, AudioDuration: time.Second, Duration: 1400 * time.Millisecond},
			},
		}},
		TotalDuration: 5 * time.Second,
	}
}

func TestWebExportWritesSidecarAssets(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	bundle, err := NewWebExporter(".").Export(context.Background(), fixtureScript(t, dir), out)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if bundle != out {
		t.Errorf("bundle = %q, want the directory %q", bundle, out)
	}

	for _, want := range []string{
		"index.html",
		filepath.Join("assets", "scene-001.svg"),
		filepath.Join("audio", "beat-0001.wav"),
	} {
		if _, err := os.Stat(filepath.Join(out, want)); err != nil {
			t.Errorf("expected %s in the bundle: %v", want, err)
		}
	}

	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "assets/scene-001.svg") {
		t.Errorf("expected the player to reference the copied art")
	}
	if strings.Contains(string(page), "http://") || strings.Contains(string(page), "https://") {
		t.Errorf("a bundle must not reach out to the network")
	}
}

func TestWebExportEmbedsBeatDurations(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	if _, err := NewWebExporter(".").Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	const marker = "const SCRIPT = "
	idx := strings.Index(string(page), marker)
	if idx == -1 {
		t.Fatalf("expected the script to be embedded")
	}
	rest := string(page)[idx+len(marker):]
	rest = rest[:strings.Index(rest, ";\n")]

	var payload struct {
		GameName string `json:"game_name"`
		Scenes   []struct {
			Location string `json:"location"`
			Art      string `json:"art"`
			Beats    []struct {
				Kind     string  `json:"kind"`
				Speaker  string  `json:"speaker"`
				Text     string  `json:"text"`
				Audio    string  `json:"audio"`
				Duration float64 `json:"duration"`
			} `json:"beats"`
		} `json:"scenes"`
	}
	if err := json.Unmarshal([]byte(rest), &payload); err != nil {
		t.Fatalf("decode embedded script: %v\n%s", err, rest)
	}

	if payload.GameName != "Campaign One" || len(payload.Scenes) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	beats := payload.Scenes[0].Beats
	if len(beats) != 3 {
		t.Fatalf("expected 3 beats, got %d", len(beats))
	}
	if beats[2].Kind != "speech" || beats[2].Speaker != "Garrick" || beats[2].Audio != "audio/beat-0001.wav" {
		t.Errorf("unexpected speech beat: %+v", beats[2])
	}
	if beats[2].Duration < 1.3 || beats[2].Duration > 1.5 {
		t.Errorf("duration = %v, want seconds not nanoseconds", beats[2].Duration)
	}
}

func TestWebExportRejectsAnEmptyScript(t *testing.T) {
	if _, err := NewWebExporter(".").Export(context.Background(), &scene.Script{}, t.TempDir()); err == nil {
		t.Errorf("expected an error for a script with no scenes")
	}
}

func TestPlayerCarriesTheRequiredBehaviours(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	if _, err := NewWebExporter(".").Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(page)

	required := map[string]string{
		"embedded script":      "const SCRIPT = ",
		"transport":            `data-action="play"`,
		"typewriter reveal":    "revealCount",
		"autoplay recovery":    ".catch(",
		"reduced motion":       "prefers-reduced-motion",
		"no webfont":           "Georgia, serif",
		"relative asset paths": "assets/",
	}

	for label, marker := range required {
		if !strings.Contains(content, marker) {
			t.Errorf("expected %s (%q) in the player", label, marker)
		}
	}
}
