package export

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
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

// playerAssets stands in for the built player, which the frontend build writes into
// pkg/gui/dist/player. Tests must not depend on a build having run.
func playerAssets() fstest.MapFS {
	return fstest.MapFS{
		"player/player.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Story Theater</title>
<script type="module" crossorigin src="./assets/player-abc.js"></script>
<link rel="stylesheet" crossorigin href="./assets/player-abc.css">
</head>
<body>
<div id="story-player" class="fixed inset-0"></div>
</body>
</html>
`)},
		"player/assets/player-abc.js":  &fstest.MapFile{Data: []byte("console.log('player')")},
		"player/assets/player-abc.css": &fstest.MapFile{Data: []byte("body{background:#000}")},
	}
}

// testExporter builds an exporter with the player assets a bundle ships.
func testExporter() *WebExporter {
	exporter := NewWebExporter(".")
	exporter.SetAssets(playerAssets())
	return exporter
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
	portrait := filepath.Join(dir, "portrait-garrick.svg")
	if err := os.WriteFile(portrait, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatal(err)
	}
	playerPortrait := filepath.Join(dir, "portrait-player.svg")
	if err := os.WriteFile(playerPortrait, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatal(err)
	}

	return &scene.Script{
		GameID:         "campaign-01",
		GameName:       "Campaign One",
		PlayerPortrait: playerPortrait,
		Scenes: []scene.Scene{{
			LocationID:   "alden-tavern",
			LocationName: "Alden Tavern",
			ArtPath:      art,
			Duration:     5 * time.Second,
			Beats: []scene.Beat{
				{Kind: scene.BeatSceneCard, Text: "Alden Tavern", ArtPath: art, Duration: 2 * time.Second},
				{Kind: scene.BeatNarration, Text: "Warm light.", ArtPath: art, Duration: 2 * time.Second},
				{Kind: scene.BeatSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Welcome.", ArtPath: art,
					PortraitPath: portrait,
					AudioPaths:   []string{clip}, AudioDuration: time.Second, Duration: 1400 * time.Millisecond},
			},
		}},
		TotalDuration: 5 * time.Second,
	}
}

func TestWebExportWritesSidecarAssets(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	bundle, err := testExporter().Export(context.Background(), fixtureScript(t, dir), out)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if bundle != out {
		t.Errorf("bundle = %q, want the directory %q", bundle, out)
	}

	for _, want := range []string{
		"index.html",
		filepath.Join("assets", "scene-001.svg"),
		filepath.Join("assets", "portrait-garrick.svg"),
		filepath.Join("assets", "portrait-player.svg"),
		filepath.Join("assets", "player-abc.js"),
		filepath.Join("assets", "player-abc.css"),
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
	for _, want := range []string{
		"assets/scene-001.svg",
		"assets/portrait-garrick.svg",
		"assets/portrait-player.svg",
		`src="./assets/player-abc.js"`,
		`href="./assets/player-abc.css"`,
		"window.__LOCALRPG_STORY__",
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("expected %q in the bundle's page:\n%s", want, page)
		}
	}
	if strings.Contains(string(page), "http://") || strings.Contains(string(page), "https://") {
		t.Errorf("a bundle must not reach out to the network")
	}
}

// webPayloadFixture is the payload a player reads, so a test can assert what a
// bundle says about itself rather than how the page is built.
type webPayloadFixture struct {
	GameName       string `json:"game_name"`
	DisplayMode    string `json:"display_mode"`
	PlayerPortrait string `json:"player_portrait"`
	Scenes         []struct {
		Location string `json:"location"`
		Art      string `json:"art"`
		Beats    []struct {
			Kind     string   `json:"kind"`
			Speaker  string   `json:"speaker"`
			Text     string   `json:"text"`
			Art      string   `json:"art"`
			Portrait string   `json:"portrait"`
			Audio    []string `json:"audio"`
			Duration float64  `json:"duration"`
			Player   bool     `json:"player"`
		} `json:"beats"`
	} `json:"scenes"`
	Total float64 `json:"total_duration"`
}

func readWebPayload(t *testing.T, outDir string) webPayloadFixture {
	t.Helper()
	page, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	const marker = "window.__LOCALRPG_STORY__ = "
	body := string(page)
	idx := strings.Index(body, marker)
	if idx == -1 {
		t.Fatal("expected the story to be embedded")
	}
	rest := body[idx+len(marker):]
	rest = rest[:strings.Index(rest, ";</script>")]

	var payload webPayloadFixture
	if err := json.Unmarshal([]byte(rest), &payload); err != nil {
		t.Fatalf("decode embedded story: %v", err)
	}
	return payload
}

func TestWebExportInlinesTheStory(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	if _, err := testExporter().Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	payload := readWebPayload(t, out)
	if payload.GameName != "Campaign One" || len(payload.Scenes) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.PlayerPortrait != "assets/portrait-player.svg" {
		t.Errorf("player portrait = %q, want the copied portrait", payload.PlayerPortrait)
	}

	beats := payload.Scenes[0].Beats
	if len(beats) != 3 {
		t.Fatalf("expected 3 beats, got %d", len(beats))
	}
	if beats[2].Kind != "speech" || beats[2].Speaker != "Garrick" {
		t.Errorf("unexpected speech beat: %+v", beats[2])
	}
	if beats[2].Portrait != "assets/portrait-garrick.svg" {
		t.Errorf("speech portrait = %q, want the speaker's own", beats[2].Portrait)
	}
	if beats[2].Duration < 1.3 || beats[2].Duration > 1.5 {
		t.Errorf("duration = %v, want seconds not nanoseconds", beats[2].Duration)
	}
	if got := beats[2].Audio; len(got) != 1 || got[0] != "audio/beat-0001.wav" {
		t.Errorf("audio = %#v, want the beat's copied clip", got)
	}

	// Narration has no face of its own: the narrator is not in the scene.
	if beats[1].Portrait != "" {
		t.Errorf("narration carries a portrait: %+v", beats[1])
	}
}

func TestWebExportCarriesTheDisplayMode(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	exporter := testExporter()
	exporter.SetDisplayMode("hidden")
	if _, err := exporter.Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if got := readWebPayload(t, out).DisplayMode; got != "hidden" {
		t.Errorf("display mode = %q, want the campaign's setting", got)
	}
}

func TestWebExportCopiesEveryClipOfABeat(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	script := fixtureScript(t, dir)
	second := filepath.Join(dir, "walk.wav")
	if err := os.WriteFile(second, []byte("RIFF....WAVEfmt ....data"), 0644); err != nil {
		t.Fatal(err)
	}
	script.Scenes[0].Beats[2].AudioPaths = append(script.Scenes[0].Beats[2].AudioPaths, second)

	if _, err := testExporter().Export(context.Background(), script, out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	audio := readWebPayload(t, out).Scenes[0].Beats[2].Audio
	if len(audio) != 2 {
		t.Fatalf("audio = %#v, want both clips", audio)
	}
	for _, relative := range audio {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(relative))); err != nil {
			t.Errorf("expected %s in the bundle: %v", relative, err)
		}
	}
}

func TestWebExportRejectsAnEmptyScript(t *testing.T) {
	if _, err := testExporter().Export(context.Background(), &scene.Script{}, t.TempDir()); err == nil {
		t.Errorf("expected an error for a script with no scenes")
	}
}

// A bundle without the built player would open to nothing, so an export refuses
// rather than writing one, and says which task builds it.
func TestWebExportWithoutPlayerAssetsFails(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "bundle")

	_, err := NewWebExporter(".").Export(context.Background(), fixtureScript(t, dir), out)
	if err == nil {
		t.Fatal("expected an export without player assets to fail")
	}
	if !strings.Contains(err.Error(), "build:frontend") {
		t.Errorf("err = %v, want it to name the frontend build", err)
	}
	if _, statErr := os.Stat(filepath.Join(out, "index.html")); statErr == nil {
		t.Error("a failed export must not leave a page behind")
	}
}
