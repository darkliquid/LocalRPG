package gui

import (
	"bytes"
	"compress/gzip"
	"context"

	"encoding/base64"
	"github.com/chromedp/cdproto/page"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/log"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/darkliquid/localrpg/pkg/engine"
)

// exportedBundleFixture builds a played campaign and exports it as a web bundle,
// returning the bundle's index.html path. It skips when the player has not been
// built, because a bundle ships that build.
func exportedBundleFixture(t *testing.T) (string, []string) {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yaml"),
		[]byte("media:\n  tts:\n    type: builtin\n    auto_play: false\n  image:\n    type: disabled\n"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root)
	t.Cleanup(svc.Close)
	paths := svc.GetResolver()

	for name, body := range map[string]string{
		"systems/freeform/system.yaml": "id: freeform\nname: Freeform\nversion: \"1.0\"\n",
		"worlds/harbour/world.yaml":    "id: harbour\nname: Harbour Realm\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	session, err := engine.InitGame(paths, engine.InitOptions{
		GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour", PlayerName: "Sean",
	})
	if err != nil {
		t.Fatalf("InitGame: %v", err)
	}
	_ = session.Close()

	entities := filepath.Join(paths.GameDir("campaign-01"), "entities")
	if err := os.MkdirAll(entities, 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"garrick.md":  "---\nid: garrick\nname: Garrick\ntype: character\n---\nA grim guard.\n",
		"the-quay.md": "---\nid: the-quay\nname: The Quay\ntype: location\n---\nSalt air.\n",
		"sean.md":     "---\nid: sean\nname: Sean O'Malley\ntype: character\n---\nA traveller.\n",
	} {
		if err := os.WriteFile(filepath.Join(entities, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	history := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"The quay is quiet.","location":"the-quay","segments":[{"kind":"narration","text":"The quay is quiet by [[the-quay]]."},{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"Keep moving."},{"kind":"speech","speaker":"Sean","speaker_id":"sean","player":true,"text":"I will."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(paths.GameDir("campaign-01"), "history.jsonl"), []byte(history), 0644); err != nil {
		t.Fatal(err)
	}

	events := svc.SubscribeExportEvents()
	defer svc.UnsubscribeExportEvents(events)

	out := t.TempDir()
	// The app's own export request: art and clips are on by default in the UI, and a
	// bundle that carries neither is not what the theatre shows.
	if _, err := svc.StartExport(context.Background(), ExportRequestDTO{
		GameID: "campaign-01", Format: "web", OutDir: out, Art: true, Audio: true,
	}); err != nil {
		t.Fatalf("StartExport: %v", err)
	}

	// The export runs behind the service, so wait for its own report rather than
	// guessing at a sleep.
	var messages []string
	deadline := time.After(60 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Phase == "error" {
				t.Fatalf("export failed: %s", event.Error)
			}
			if event.Message != "" {
				messages = append(messages, event.Message)
			}
			if event.Phase == "done" {
				page := filepath.Join(out, "campaign-01-web.html")
				if _, err := os.Stat(page); err != nil {
					t.Fatalf("export reported done without a page: %v", err)
				}
				return page, messages
			}
		case <-deadline:
			t.Fatal("the export did not finish in time")
		}
	}
}

// TestExportedBundlePlaysTheTheatre is the feedback loop for "the export should
// look and sound like the theatre": it opens a real bundle in a headless browser and
// asserts the theatre's own stage, portraits, dialogue panel, and transport render,
// and that the story advances.
func TestExportedBundlePlaysTheTheatre(t *testing.T) {
	browser := chromePath()
	if browser == "" {
		t.Skip("no chrome/chromium available; skipping the bundle browser loop")
	}
	if _, err := AssetFS(); err != nil {
		t.Skipf("the player has not been built: %v", err)
	}

	bundlePath, messages := exportedBundleFixture(t)

	// The export's own report reaches the client, which is what the export modal shows.
	var coverage string
	for _, message := range messages {
		if strings.HasPrefix(message, "audio:") {
			coverage = message
		}
	}
	if coverage == "" {
		t.Errorf("the export reported no audio coverage: %v", messages)
	} else if !strings.Contains(coverage, "speech 2/2") {
		t.Errorf("coverage = %q, want both speech beats counted", coverage)
	}

	allocOptions := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocOptions...)
	defer cancelAlloc()
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()
	ctx, cancelTimeout := context.WithTimeout(taskCtx, 45*time.Second)
	defer cancelTimeout()

	var console []string
	chromedp.ListenTarget(taskCtx, func(ev interface{}) {
		switch event := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			parts := make([]string, 0, len(event.Args))
			for _, arg := range event.Args {
				parts = append(parts, string(arg.Value))
			}
			console = append(console, event.Type.String()+": "+strings.Join(parts, " "))
		case *runtime.EventExceptionThrown:
			console = append(console, "exception: "+event.ExceptionDetails.Error())
		case *log.EventEntryAdded:
			console = append(console, "log: "+event.Entry.Text)
		}
	})

	const instrument = `(function () {
	  window.__clips = [];
	  var Real = window.Audio;
	  window.Audio = function (src) {
	    var audio = new Real(src);
	    var entry = { src: String(src).slice(0, 22), played: false, error: null };
	    window.__clips.push(entry);
	    audio.addEventListener('error', function () { entry.error = 'load'; });
	    var play = audio.play.bind(audio);
	    audio.play = function () {
	      var result = play();
	      if (result && result.then) {
	        result.then(function () { entry.played = true; }).catch(function (err) { entry.error = String(err && err.name ? err.name : err); });
	      }
	      return result;
	    };
	    return audio;
	  };
	  window.Audio.prototype = Real.prototype;
	})();`
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(instrument).Do(c)
		return err
	})); err != nil {
		t.Fatalf("instrument the page: %v", err)
	}

	bundleURL := (&url.URL{Scheme: "file", Path: bundlePath}).String()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(bundleURL),
		chromedp.WaitVisible(`//*[@id="story-player"]//header`, chromedp.BySearch),
	); err != nil {
		t.Fatalf("open the bundle: %v\npage console:\n%s", err, strings.Join(console, "\n"))
	}

	// Everything the page needs is inside it: no sidecars, no network, and the clips
	// and faces the browser renders are carried in the file it already loaded.
	raw, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="localrpg-bundle"`, "DecompressionStream"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the bundle is not self-contained: %q is missing", want)
		}
	}
	for _, unwanted := range []string{`src="./assets/`, `href="./assets/`, `type="module"`} {
		if strings.Contains(string(raw), unwanted) {
			t.Errorf("the bundle still refers outside itself: %q", unwanted)
		}
	}
	for _, want := range []string{"data:audio/", "data:image/"} {
		if !strings.Contains(bundleStory(t, bundlePath), want) {
			t.Errorf("the story carries no %q asset", want)
		}
	}

	if failures := failedRequests(console); len(failures) > 0 {
		t.Fatalf("the bundle's page failed to load:\n%s", strings.Join(failures, "\n"))
	}

	// A story must not start itself: the beat it opens on is still the beat it is on
	// two seconds later, and the transport offers to start it. The first beat is waited
	// for, because the page inflates its bundle before it renders anything.
	waitFor(t, ctx, "THE QUAY")
	before := bundleDom(t, ctx)
	if !strings.Contains(before, "Play") {
		t.Errorf("expected a play control, got:\n%s", before)
	}
	time.Sleep(2 * time.Second)
	if after := bundleDom(t, ctx); after != before {
		t.Errorf("the story advanced without being started:\nbefore: %s\nafter:  %s", before, after)
	}

	// The theatre's own furniture, not a lookalike page: the header names the
	// campaign and the place, the stage keeps the protagonist on it, and the
	// transport is the theatre's.
	first := bundleDom(t, ctx)
	for _, want := range []struct{ label, needle string }{
		{"the header names the campaign", "CAMPAIGN-01"},
		{"the header names the scene", "The Quay"},
		{"the title card", "THE QUAY"},
		{"the transport", "Playback speed"},
	} {
		if !strings.Contains(strings.ToUpper(first), strings.ToUpper(want.needle)) {
			t.Errorf("%s: expected %q in the rendered bundle:\n%s", want.label, want.needle, first)
		}
	}
	if !strings.Contains(first, "data:image/") {
		t.Errorf("expected the protagonist's portrait on the stage, inlined, got:\n%s", first)
	}

	// Play is the gesture the browser needs for audio, and the story then runs itself.
	if err := chromedp.Run(ctx, chromedp.Click(`button[data-transport="toggle"]`, chromedp.ByQuery)); err != nil {
		t.Fatalf("start the bundle: %v", err)
	}

	// A beat is never shortened by audio: the card holds for its own pace, so it is
	// still on screen a second into playback. Before this, a clip the browser refused
	// to start advanced every beat at once and the story flashed past.
	time.Sleep(time.Second)
	if mid := bundleDom(t, ctx); strings.Contains(mid, "NARRATOR") {
		t.Fatalf("the opening beat was skipped instead of being held:\n%s", mid)
	}

	// The narration beat renders through the theatre's dialogue panel, and its link reads
	// as the place's name: a bundle has no codex to open.
	waitFor(t, ctx, "NARRATOR", "The quay is quiet by The Quay.")
	if link := bundleDom(t, ctx); strings.Contains(link, "[[") {
		t.Errorf("the bundle's prose still carries a link:\n%s", link)
	}

	// The speech beat puts the speaker's face on the stage beside their line, and the
	// protagonist is labelled under their own.
	waitFor(t, ctx, "GARRICK", "Keep moving.", "data:image/")
	waitFor(t, ctx, "Sean O'Malley")

	// Every clip the story reached must have played. A character line that the browser
	// refused shows up here as an error or as a clip that never started, which is the
	// difference between a bundle missing audio and a player failing to play it.
	var clips string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify(window.__clips || [])`, &clips)); err != nil {
		t.Fatal(err)
	}
	if clips == "[]" {
		t.Fatalf("the story played no clips at all:\n%s", bundleDom(t, ctx))
	}
	if strings.Contains(clips, `"error":"`) {
		t.Errorf("a clip failed to play: %s", clips)
	}
	if strings.Contains(clips, `"played":false`) {
		t.Errorf("a clip never started: %s", clips)
	}
	if strings.Contains(bundleDom(t, ctx), "could not play") {
		t.Errorf("the player reported a line it could not play:\n%s", bundleDom(t, ctx))
	}
}

// waitFor waits for every needle to appear, which the typewriter reveal makes a
// matter of time rather than of a single frame.
func waitFor(t *testing.T, ctx context.Context, needles ...string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		shape := bundleDom(t, ctx)
		missing := ""
		for _, needle := range needles {
			if !strings.Contains(strings.ToUpper(shape), strings.ToUpper(needle)) {
				missing = needle
				break
			}
		}
		if missing == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the bundle never showed %q:\n%s", missing, shape)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// bundleStory decodes the compressed story a bundle carries, so a test can assert what
// the browser was given rather than what it rendered.
func bundleStory(t *testing.T, bundlePath string) string {
	t.Helper()
	raw, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}

	const marker = `<script id="localrpg-bundle" type="application/octet-stream">`
	body := string(raw)
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
	reader, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	return string(decoded)
}

// failedRequests reports the console entries that mean the page could not load what it
// needs, which is what a bundle opened from disk must never produce.
func failedRequests(console []string) []string {
	failures := make([]string, 0, len(console))
	for _, entry := range console {
		if strings.Contains(entry, "ERR_FAILED") || strings.Contains(entry, "CORS") || strings.Contains(entry, "Failed to load") {
			failures = append(failures, entry)
		}
	}
	return failures
}

// bundleDom reports what the player actually rendered, so a failure says whether
// the stage, the portraits, or the dialogue is missing.
func bundleDom(t *testing.T, ctx context.Context) string {
	t.Helper()
	const script = `JSON.stringify({
	  header: (document.querySelector('header') || {}).innerText || '',
	  portraits: Array.from(document.querySelectorAll('img')).map(img => (img.getAttribute('src') || '').slice(0, 22)),
	  namePlate: (document.querySelector('#story-player [class*="absolute -top-3.5"]') || {}).textContent || '',
	  text: (document.getElementById('story-player') || {}).innerText || '', 
	  transport: Array.from(document.querySelectorAll('#story-player button')).map(b => b.getAttribute('title') || b.textContent || '').join('|'),
	  backgrounds: Array.from(document.querySelectorAll('#story-player [style*="background"]')).length,
	})`
	var out string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &out)); err != nil {
		return "probe failed: " + err.Error()
	}
	return out
}
