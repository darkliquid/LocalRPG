package export

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/scene"
)

// playerPage is the built player the bundle is made from, relative to the frontend
// build's root.
const playerPage = "player/player.html"

// assetRefPattern finds the assets a built page references.
var assetRefPattern = regexp.MustCompile(`(?:src|href)="([^"]+)"`)

// WebExporter writes a bundle as one self-contained page: the theatre's own player, the
// story, and every asset in a single file, so it opens by being opened.
type WebExporter struct {
	rootDir string
	assets  fs.FS
	// displayMode is the campaign's speech-cue display setting, baked into the bundle
	// so an export shows performance tags and stage directions the way the app does.
	displayMode string
}

// NewWebExporter builds an exporter rooted at a campaign directory.
func NewWebExporter(rootDir string) *WebExporter {
	return &WebExporter{rootDir: rootDir}
}

// SetAssets supplies the built player (the frontend's dist directory). Without it an
// export cannot produce a page anyone can watch, so Export refuses rather than writing
// a bundle that opens to nothing.
func (w *WebExporter) SetAssets(assets fs.FS) { w.assets = assets }

// SetDisplayMode records how the campaign renders performance tags.
func (w *WebExporter) SetDisplayMode(mode string) { w.displayMode = mode }

// webBeat is one beat as the player sees it: durations in seconds, and art, portraits,
// and clips as data URIs, because a bundle carries everything it needs.
type webBeat struct {
	Kind     string   `json:"kind"`
	Speaker  string   `json:"speaker,omitempty"`
	Text     string   `json:"text"`
	Art      string   `json:"art,omitempty"`
	Portrait string   `json:"portrait,omitempty"`
	Audio    []string `json:"audio,omitempty"`
	Duration float64  `json:"duration"`
	// Reading is what a viewer needs to read the line. A player holds a beat for the
	// longer of this and its compiled pace, so audio can never shorten a beat and a
	// clip that cannot play leaves the reading time.
	Reading float64 `json:"reading"`
	Player  bool    `json:"player,omitempty"`
	// Outcome is the turn's resolved outcome, which the player maps to a mood tint.
	Outcome string `json:"outcome,omitempty"`
}

type webScene struct {
	Location string    `json:"location,omitempty"`
	Art      string    `json:"art,omitempty"`
	Beats    []webBeat `json:"beats"`
	// Weather is the location's weather, which the player draws as an overlay.
	Weather string `json:"weather,omitempty"`
}

// webChapter is a navigable scene boundary, in seconds.
type webChapter struct {
	Title string  `json:"title"`
	Start float64 `json:"start"`
}

type webPayload struct {
	GameName       string `json:"game_name"`
	DisplayMode    string `json:"display_mode,omitempty"`
	PlayerPortrait string `json:"player_portrait,omitempty"`
	// Banner is the campaign's own image, which the player shows behind a scene that
	// has no art of its own, exactly as the theatre does.
	Banner string `json:"banner,omitempty"`
	// PlayerName labels the protagonist's portrait, as the theatre does.
	PlayerName string     `json:"player_name,omitempty"`
	Scenes     []webScene `json:"scenes"`
	// Captions is the story's WebVTT subtitle track, carried in the bundle so a
	// viewer who cannot hear the audio still reads the spoken lines.
	Captions string `json:"captions,omitempty"`
	// Chapters are the story's scene boundaries, and ChaptersVTT is the same list
	// as a WebVTT track for a player's native chapter controls.
	Chapters    []webChapter `json:"chapters,omitempty"`
	ChaptersVTT string       `json:"chapters_vtt,omitempty"`
	// Total is the script's own pacing, kept for a reader of the payload; the player
	// paces itself per beat.
	Total float64 `json:"total_duration"`
}

// Export writes one self-contained page to outPath and returns it.
func (w *WebExporter) Export(ctx context.Context, script *scene.Script, outPath string) (string, error) {
	cleanOut, err := pathutil.ValidateUserPath(outPath)
	if err != nil {
		return "", fmt.Errorf("invalid export output path: %w", err)
	}
	outPath = cleanOut

	if script == nil || len(script.Scenes) == 0 {
		return "", fmt.Errorf("script has no scenes to export")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if w.assets == nil {
		return "", fmt.Errorf("web export needs the built player: run `mise run build:frontend`")
	}

	payload := &webPayload{
		GameName:    script.GameName,
		DisplayMode: w.displayMode,
		Total:       script.TotalDuration.Seconds(),
		Captions:    scene.Captions(script.Beats()),
	}

	// A missing asset costs a face or a clip, never the bundle: the beat keeps the
	// pacing it was compiled with.
	if uri, err := imageDataURI(script.PlayerPortrait); err == nil {
		payload.PlayerPortrait = uri
	}
	if uri, err := imageDataURI(script.Banner); err == nil {
		payload.Banner = uri
	}
	payload.PlayerName = script.PlayerName
	for _, chapter := range script.Chapters {
		payload.Chapters = append(payload.Chapters, webChapter{Title: chapter.Title, Start: chapter.Start.Seconds()})
	}
	payload.ChaptersVTT = scene.ChaptersVTT(script.Chapters, script.TotalDuration)

	for i := range script.Scenes {
		sc := script.Scenes[i]
		entry := webScene{Location: sc.LocationName, Weather: sc.Weather}

		if uri, err := imageDataURI(sc.ArtPath); err == nil {
			entry.Art = uri
		}

		for j := range sc.Beats {
			beat := sc.Beats[j]

			jsBeat := webBeat{
				Kind:     string(beat.Kind),
				Speaker:  beat.Speaker,
				Text:     beat.Text,
				Art:      entry.Art,
				Duration: beat.Duration.Seconds(),
				Reading:  scene.ReadingDuration(beat.Text).Seconds(),
				Player:   beat.Player,
				Outcome:  beat.Outcome,
			}

			// A beat's own illustration wins over the scene's backdrop. A missing
			// asset keeps the backdrop rather than failing the bundle.
			if beat.ArtPath != "" && beat.ArtPath != sc.ArtPath {
				if uri, err := imageDataURI(beat.ArtPath); err == nil {
					jsBeat.Art = uri
				}
			}

			if uri, err := imageDataURI(beat.PortraitPath); err == nil {
				jsBeat.Portrait = uri
			}

			for _, clip := range beat.AudioPaths {
				if uri, err := dataURI(clip); err == nil {
					jsBeat.Audio = append(jsBeat.Audio, uri)
				}
			}

			entry.Beats = append(entry.Beats, jsBeat)
		}

		payload.Scenes = append(payload.Scenes, entry)
	}

	page, err := w.bundlePage(payload)
	if err != nil {
		return "", err
	}
	if dir := filepath.Dir(outPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("create export dir: %w", err)
		}
	}
	if err := os.WriteFile(outPath, page, 0644); err != nil {
		return "", fmt.Errorf("write bundle: %w", err)
	}

	return outPath, nil
}

// bundlePage is the built player with its stylesheet, its script, and the story in one
// compressed bundle, and a bootstrap that inflates it. A page opened from disk can fetch
// nothing beside it — a module script and a stylesheet over file:// are both refused by
// CORS — and the page's own code is a fixed third of a megabyte, so the whole bundle is
// compressed rather than the story alone.
func (w *WebExporter) bundlePage(payload *webPayload) ([]byte, error) {
	raw, err := fs.ReadFile(w.assets, playerPage)
	if err != nil {
		return nil, fmt.Errorf("read player page: %w", err)
	}

	story, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode story: %w", err)
	}

	bundle := pageBundle{Story: story}
	for _, ref := range pageAssetRefs(string(raw)) {
		content, err := fs.ReadFile(w.assets, "player/"+ref)
		if err != nil {
			return nil, fmt.Errorf("read player asset %q: %w", ref, err)
		}
		switch strings.ToLower(filepath.Ext(ref)) {
		case ".css":
			bundle.CSS = string(content)
		case ".js":
			bundle.JS = string(content)
		default:
			return nil, fmt.Errorf("player asset %q is neither a stylesheet nor a script", ref)
		}
	}
	if bundle.JS == "" {
		return nil, fmt.Errorf("the player page references no script: run `mise run build:frontend`")
	}

	encoded, err := compressBundle(bundle)
	if err != nil {
		return nil, err
	}

	// The page keeps the build's own skeleton, and replaces the references it cannot
	// load from disk with the bundle and the bootstrap that reads it.
	page := string(raw)
	page = regexp.MustCompile(`<link[^>]*rel="stylesheet"[^>]*>`).ReplaceAllString(page, "")
	page = regexp.MustCompile(`<script[^>]*src="[^"]*"[^>]*></script>`).ReplaceAllString(page, "")
	page = strings.Replace(page, "</body>", bundleHolder(encoded)+bootstrapScript+"\n</body>", 1)

	title := strings.TrimSpace(payload.GameName)
	if title == "" {
		title = "Story Theater"
	}
	page = strings.Replace(page, "<title>Story Theater</title>", "<title>"+html.EscapeString(title)+"</title>", 1)

	return []byte(page), nil
}

// pageBundle is everything a page needs, compressed together: the player's script and
// stylesheet, and the story they render.
type pageBundle struct {
	JS    string          `json:"js"`
	CSS   string          `json:"css"`
	Story json.RawMessage `json:"story"`
}

// compressBundle encodes the bundle for the page to carry: gzipped, then base64, since a
// page is text. Prose, art, and the player's own code compress well; the clips are Opus
// and already compressed, so they are along for the ride.
func compressBundle(bundle pageBundle) (string, error) {
	raw, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("encode bundle: %w", err)
	}

	var packed bytes.Buffer
	writer := gzip.NewWriter(&packed)
	if _, err := writer.Write(raw); err != nil {
		return "", fmt.Errorf("compress bundle: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("compress bundle: %w", err)
	}

	return base64.StdEncoding.EncodeToString(packed.Bytes()), nil
}

// bundleHolder is the compressed bundle, kept in a script element the bootstrap reads
// rather than in the page's own script, so nothing tries to run it.
func bundleHolder(encoded string) string {
	return `<script id="localrpg-bundle" type="application/octet-stream">` + encoded + "</script>\n"
}

// bootstrapScript inflates the bundle and starts the player. It is deliberately plain and
// small: it is the one part of a page that cannot itself be compressed.
const bootstrapScript = `<script>
(async function () {
  function fail(why) {
    document.body.innerHTML = '<p style="font:16px system-ui;color:#d6d3d1;padding:2rem">This story could not be opened (' + why + '). Open it in a current browser.</p>';
  }
  var holder = document.getElementById('localrpg-bundle');
  if (!holder) { fail('no story in the file'); return; }
  if (typeof DecompressionStream !== 'function') { fail('this browser cannot decompress it'); return; }
  try {
    var bytes = Uint8Array.from(atob(holder.textContent.trim()), function (c) { return c.charCodeAt(0); });
    var text = await new Response(new Blob([bytes]).stream().pipeThrough(new DecompressionStream('gzip'))).text();
    var bundle = JSON.parse(text);
    var style = document.createElement('style');
    style.textContent = bundle.css;
    document.head.appendChild(style);
    window.__LOCALRPG_STORY__ = bundle.story;
    var script = document.createElement('script');
    script.textContent = bundle.js;
    document.body.appendChild(script);
  } catch (err) {
    fail(err && err.message ? err.message : 'the story is unreadable');
  }
})();
</script>`

// pageAssetRefs returns the player's own asset paths a page references, relative to the
// player build's root, so the build's hashed names are read rather than assumed.
func pageAssetRefs(page string) []string {
	refs := make([]string, 0, 2)
	for _, match := range assetRefPattern.FindAllStringSubmatch(page, -1) {
		ref := strings.TrimPrefix(strings.TrimPrefix(match[1], "./"), "/")
		if strings.HasPrefix(ref, "assets/") {
			refs = append(refs, ref)
		}
	}
	return refs
}

// replaceTag swaps the first tag matching pattern for replacement, and refuses a page
// whose referenced asset it cannot find.
func replaceTag(page, pattern, replacement string) string {
	return regexp.MustCompile(pattern).ReplaceAllString(page, replacement)
}

// dataURI encodes an asset for the page to carry. An empty path, or one that cannot be
// read, has no encoding: the caller degrades the beat instead.
func dataURI(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("no asset")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return "data:" + mimeTypeFor(path, data) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// mimeTypeFor reports what an asset is, from its bytes first: a provider returns whatever
// its engine produces, so a clip cached under one name can hold another format, and a
// browser refuses a data URI whose type does not match its contents. The extension is the
// fallback for a format this does not recognise.
func mimeTypeFor(path string, data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("OggS")):
		return "audio/ogg"
	case bytes.HasPrefix(data, []byte("fLaC")):
		return "audio/flac"
	case bytes.HasPrefix(data, []byte("ID3")):
		return "audio/mpeg"
	case len(data) > 1 && data[0] == 0xFF && data[1]&0xE0 == 0xE0:
		return "audio/mpeg"
	case len(data) > 11 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	case bytes.HasPrefix(data, []byte("RIFF")):
		return "audio/wav"
	case bytes.HasPrefix(data, []byte("\x89PNG")):
		return "image/png"
	case bytes.HasPrefix(data, []byte("\xFF\xD8\xFF")):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return "image/gif"
	case bytes.Contains(data[:min(len(data), 512)], []byte("<svg")):
		return "image/svg+xml"
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".opus", ".ogg":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".flac":
		return "audio/flac"
	default:
		return "application/octet-stream"
	}
}
