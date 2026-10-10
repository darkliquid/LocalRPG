package export

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
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
<script type="module" crossorigin src="/assets/player-abc.js"></script>
<link rel="stylesheet" crossorigin href="/assets/style-abc.css">
</head>
<body>
<div id="story-player" class="fixed inset-0"></div>
</body>
</html>
`)},
		"player/assets/player-abc.js": &fstest.MapFile{Data: []byte("console.log('player')")},
		"player/assets/style-abc.css": &fstest.MapFile{Data: []byte("body{background:#000}")},
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

// A bundle is one file: a page opened from disk can fetch nothing beside it, and a
// module script over file:// is refused by CORS outright.
func TestWebExportWritesOneSelfContainedFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	bundle, err := testExporter().Export(context.Background(), fixtureScript(t, dir), out)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if bundle != out {
		t.Errorf("bundle = %q, want the file %q", bundle, out)
	}

	page, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("expected the bundle at %q: %v", out, err)
	}
	body := string(page)

	for _, want := range []string{
		`<script id="localrpg-bundle" type="application/octet-stream">`,
		"DecompressionStream",
		"<title>Campaign One</title>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in the bundle", want)
		}
	}

	// The player and the story travel compressed, so the page's own text is a
	// bootstrap rather than the build.
	if strings.Contains(body, "window.__LOCALRPG_STORY__ = {") {
		t.Error("the story is inlined uncompressed")
	}

	for _, unwanted := range []string{`src="./assets/`, `href="./assets/`, `src="/assets/`, `href="/assets/`, `type="module"`, "http://", "https://"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the bundle still refers outside itself: %q", unwanted)
		}
	}

	for _, sidecar := range []string{"assets", "audio"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(out), sidecar)); err == nil {
			t.Errorf("a bundle must not leave a %s directory beside it", sidecar)
		}
	}
}

// webPayloadFixture is the payload a player reads, so a test can assert what a
// bundle says about itself rather than how the page is built.
type webPayloadFixture struct {
	GameName       string `json:"game_name"`
	DisplayMode    string `json:"display_mode"`
	PlayerPortrait string `json:"player_portrait"`
	Banner         string `json:"banner"`
	Scenes         []struct {
		Location string `json:"location"`
		Art      string `json:"art"`
		Weather  string `json:"weather"`
		Beats    []struct {
			Kind     string   `json:"kind"`
			Speaker  string   `json:"speaker"`
			Text     string   `json:"text"`
			Art      string   `json:"art"`
			Portrait string   `json:"portrait"`
			Audio    []string `json:"audio"`
			Duration float64  `json:"duration"`
			Reading  float64  `json:"reading"`
			Player   bool     `json:"player"`
			Outcome  string   `json:"outcome"`
		} `json:"beats"`
	} `json:"scenes"`
	Captions    string  `json:"captions"`
	ChaptersVTT string  `json:"chapters_vtt"`
	Chapters    []struct {
		Title string  `json:"title"`
		Start float64 `json:"start"`
	} `json:"chapters"`
	Total float64 `json:"total_duration"`
}

// readBundle decodes the compressed bundle a page carries, which is how a test asserts
// what a bundle says about itself rather than how the page is built.
func readBundle(t *testing.T, outPath string) (string, string, webPayloadFixture) {
	t.Helper()
	page, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	const marker = `<script id="localrpg-bundle" type="application/octet-stream">`
	body := string(page)
	idx := strings.Index(body, marker)
	if idx == -1 {
		t.Fatal("expected a compressed bundle in the page")
	}
	rest := body[idx+len(marker):]
	encoded := rest[:strings.Index(rest, "</script>")]

	packed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if len(packed) < 2 || packed[0] != 0x1f || packed[1] != 0x8b {
		t.Fatal("the bundle is not gzipped")
	}

	reader, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}

	var bundle struct {
		JS    string          `json:"js"`
		CSS   string          `json:"css"`
		Story json.RawMessage `json:"story"`
	}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}

	var story webPayloadFixture
	if err := json.Unmarshal(bundle.Story, &story); err != nil {
		t.Fatalf("decode story: %v", err)
	}
	return bundle.JS, bundle.CSS, story
}

// readWebPayload is the story alone, for the tests that only care about it.
func readWebPayload(t *testing.T, outPath string) webPayloadFixture {
	t.Helper()
	_, _, story := readBundle(t, outPath)
	return story
}

// Every asset travels inside the page, encoded, because there is nothing beside it to
// load.
func TestWebExportInlinesEveryAsset(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	if _, err := testExporter().Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	payload := readWebPayload(t, out)
	if !strings.HasPrefix(payload.PlayerPortrait, "data:image/svg+xml;base64,") {
		t.Errorf("player portrait = %q, want a data URI", payload.PlayerPortrait)
	}
	if !strings.HasPrefix(payload.Scenes[0].Art, "data:image/svg+xml;base64,") {
		t.Errorf("scene art = %q, want a data URI", payload.Scenes[0].Art)
	}

	if payload.Banner != "" {
		t.Errorf("banner = %q, want none for a script without one", payload.Banner)
	}

	beats := payload.Scenes[0].Beats
	if !strings.HasPrefix(beats[2].Portrait, "data:image/svg+xml;base64,") {
		t.Errorf("speech portrait = %q, want a data URI", beats[2].Portrait)
	}
	if len(beats[2].Audio) != 1 || !strings.HasPrefix(beats[2].Audio[0], "data:audio/wav;base64,") {
		t.Errorf("audio = %#v, want a data URI per clip", beats[2].Audio)
	}
	// Narration has no face of its own: the narrator is not in the scene.
	if beats[1].Portrait != "" {
		t.Errorf("narration carries a portrait: %+v", beats[1])
	}
}

// The player's script and stylesheet are carried in the bundle, not left beside the page.
func TestWebExportCarriesThePlayerInTheBundle(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	if _, err := testExporter().Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	js, css, _ := readBundle(t, out)
	if !strings.Contains(js, "console.log('player')") {
		t.Errorf("bundle js = %q, want the built player", js)
	}
	if !strings.Contains(css, "body{background:#000}") {
		t.Errorf("bundle css = %q, want the built stylesheet", css)
	}
}

// Compression is the point of the bundle: the player's own code is a fixed third of a
// megabyte, and prose and art compress well.
func TestWebExportCompressesTheBundle(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	script := fixtureScript(t, dir)
	// A story with enough repeated text to compress, which is what prose looks like.
	for i := 0; i < 40; i++ {
		script.Scenes[0].Beats = append(script.Scenes[0].Beats, scene.Beat{
			Kind: scene.BeatNarration,
			Text: strings.Repeat("The quay is quiet, and the gulls have gone to ground. ", 8),
		})
	}

	if _, err := testExporter().Export(context.Background(), script, out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	page, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	// The page must be smaller than the parts it carries: the player's own code, its
	// stylesheet, and the story, all of which it holds compressed.
	js, css, _ := readBundle(t, out)
	storyJSON, err := json.Marshal(readWebPayload(t, out))
	if err != nil {
		t.Fatal(err)
	}
	held := len(js) + len(css) + len(storyJSON)
	if len(page) >= held {
		t.Errorf("bundle = %d bytes, want it smaller than the %d bytes it holds", len(page), held)
	}
}

func TestWebExportInlinesTheStory(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	if _, err := testExporter().Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	payload := readWebPayload(t, out)
	if payload.GameName != "Campaign One" || len(payload.Scenes) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	beats := payload.Scenes[0].Beats
	if len(beats) != 3 {
		t.Fatalf("expected 3 beats, got %d", len(beats))
	}
	if beats[2].Kind != "speech" || beats[2].Speaker != "Garrick" {
		t.Errorf("unexpected speech beat: %+v", beats[2])
	}
	if beats[2].Duration < 1.3 || beats[2].Duration > 1.5 {
		t.Errorf("duration = %v, want seconds not nanoseconds", beats[2].Duration)
	}
	// The reading estimate travels with the beat, so a player holds it for the longer
	// of the two and audio can never shorten a line.
	if beats[2].Reading < 1.5 {
		t.Errorf("reading = %v, want the time a viewer needs for the line", beats[2].Reading)
	}
}

// The theatre shows the campaign's own image behind a scene with no art, so a bundle
// carries it too.
func TestWebExportCarriesTheBanner(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	banner := filepath.Join(dir, "banner.png")
	if err := os.WriteFile(banner, []byte("png-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	script := fixtureScript(t, dir)
	script.Banner = banner

	if _, err := testExporter().Export(context.Background(), script, out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if got := readWebPayload(t, out).Banner; !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Errorf("banner = %q, want the campaign's image inlined", got)
	}
}

func TestWebExportCarriesTheDisplayMode(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
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

func TestWebExportInlinesEveryClipOfABeat(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
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
	for i, uri := range audio {
		if !strings.HasPrefix(uri, "data:audio/wav;base64,") {
			t.Errorf("clip %d = %.40q, want a data URI", i, uri)
		}
	}
}

func TestWebExportRejectsAnEmptyScript(t *testing.T) {
	if _, err := testExporter().Export(context.Background(), &scene.Script{}, filepath.Join(t.TempDir(), "bundle.html")); err == nil {
		t.Errorf("expected an error for a script with no scenes")
	}
}

// A bundle without the built player would open to nothing, so an export refuses rather
// than writing one, and says which task builds it.
func TestWebExportWithoutPlayerAssetsFails(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "bundle.html")

	_, err := NewWebExporter(".").Export(context.Background(), fixtureScript(t, dir), out)
	if err == nil {
		t.Fatal("expected an export without player assets to fail")
	}
	if !strings.Contains(err.Error(), "build:frontend") {
		t.Errorf("err = %v, want it to name the frontend build", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("a failed export must not leave a page behind")
	}
}

// A page that references a player asset it does not have could never boot, so the
// export refuses instead of writing it.
func TestWebExportRefusesAnIncompletePlayerBuild(t *testing.T) {
	assets := fstest.MapFS{
		"player/player.html": &fstest.MapFile{Data: []byte(
			`<html><head><script src="/assets/missing.js"></script></head><body></body></html>`)},
	}
	exporter := NewWebExporter(".")
	exporter.SetAssets(assets)

	out := filepath.Join(t.TempDir(), "bundle.html")
	if _, err := exporter.Export(context.Background(), fixtureScript(t, t.TempDir()), out); err == nil {
		t.Fatal("expected a bundle whose player is incomplete to be refused")
	}
}

// A clip cached under one name can hold another format, and a browser refuses a data URI
// whose type does not match its contents, so the type comes from the bytes.
func TestWebExportTypesAssetsFromTheirBytes(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	// An Opus clip with a .wav name, which is what a provider that changed its output
	// leaves behind.
	misnamed := filepath.Join(dir, "line.wav")
	if err := os.WriteFile(misnamed, append([]byte("OggS"), []byte("opaque opus bytes")...), 0644); err != nil {
		t.Fatal(err)
	}

	script := fixtureScript(t, dir)
	script.Scenes[0].Beats[2].AudioPaths = []string{misnamed}

	if _, err := testExporter().Export(context.Background(), script, out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	audio := readWebPayload(t, out).Scenes[0].Beats[2].Audio
	if len(audio) != 1 || !strings.HasPrefix(audio[0], "data:audio/ogg;base64,") {
		t.Errorf("audio = %.40q, want the type the bytes are", audio)
	}
}

// The inspector answers whether a bundle's clips will play, which is what a browser refusing
// a bundle needs and what the app that made it cannot tell you.
func TestInspectBundleReportsEveryClip(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	// A real clip, because the inspector checks the bytes rather than the file's name.
	cache := media.NewContentCache(t.TempDir())
	clip, err := media.NewTTSPipeline(&toneTTS{}, cache).
		SynthesizeUtterance(context.Background(), "garrick", &entity.VoiceConfig{VoiceID: "bm_george"}, "Keep moving.")
	if err != nil {
		t.Fatal(err)
	}

	script := fixtureScript(t, dir)
	script.Scenes[0].Beats[2].AudioPaths = []string{clip}
	if _, err := testExporter().Export(context.Background(), script, out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	beats, err := InspectBundle(out)
	if err != nil {
		t.Fatalf("InspectBundle: %v", err)
	}
	if len(beats) != 3 {
		t.Fatalf("beats = %d, want the bundle's three", len(beats))
	}

	var clips int
	for _, beat := range beats {
		for _, clip := range beat.Clips {
			clips++
			if !clip.Complete || !clip.Decodable {
				t.Errorf("clip in %s beat is unplayable: %s", beat.Kind, clip.Problem)
			}
		}
	}
	if clips != 1 {
		t.Errorf("clips = %d, want the one the beat carries", clips)
	}
}

// A clip a browser would refuse is reported with the reason, which is the whole point of
// inspecting a bundle.
func TestInspectBundleFlagsAnUnplayableClip(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.html")
	dir := t.TempDir()

	if _, err := testExporter().Export(context.Background(), fixtureScript(t, dir), out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	beats, err := InspectBundle(out)
	if err != nil {
		t.Fatalf("InspectBundle: %v", err)
	}

	var flagged int
	for _, beat := range beats {
		for _, clip := range beat.Clips {
			if clip.Problem != "" {
				flagged++
			}
		}
	}
	if flagged != 1 {
		t.Errorf("flagged = %d, want the fixture's clip reported as unplayable", flagged)
	}
}

func TestInspectBundleRejectsSomethingElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-bundle.html")
	if err := os.WriteFile(path, []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectBundle(path); err == nil {
		t.Error("expected an error for a file that is not a bundle")
	}
}
