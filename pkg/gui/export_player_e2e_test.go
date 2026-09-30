package gui

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/darkliquid/localrpg/pkg/engine"
)

// exportedBundleFixture builds a played campaign and exports it as a web bundle,
// returning the bundle's index.html path. It skips when the player has not been
// built, because a bundle ships that build.
func exportedBundleFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yaml"),
		[]byte("media:\n  tts:\n    type: disabled\n  image:\n    type: disabled\n"), 0644); err != nil {
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
	} {
		if err := os.WriteFile(filepath.Join(entities, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	history := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"The quay is quiet.","location":"the-quay","segments":[{"kind":"narration","text":"The quay is quiet."},{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"Keep moving."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(paths.GameDir("campaign-01"), "history.jsonl"), []byte(history), 0644); err != nil {
		t.Fatal(err)
	}

	events := svc.SubscribeExportEvents()
	defer svc.UnsubscribeExportEvents(events)

	out := t.TempDir()
	if _, err := svc.StartExport(context.Background(), ExportRequestDTO{
		GameID: "campaign-01", Format: "web", OutDir: out,
	}); err != nil {
		t.Fatalf("StartExport: %v", err)
	}

	// The export runs behind the service, so wait for its own report rather than
	// guessing at a sleep.
	deadline := time.After(60 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Phase == "error" {
				t.Fatalf("export failed: %s", event.Error)
			}
			if event.Phase == "done" {
				page := filepath.Join(out, "campaign-01-web", "index.html")
				if _, err := os.Stat(page); err != nil {
					t.Fatalf("export reported done without a page: %v", err)
				}
				return page
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

	page := exportedBundleFixture(t)

	allocOptions := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		// A bundle has no server: it is opened from the file system.
		chromedp.Flag("allow-file-access-from-files", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocOptions...)
	defer cancelAlloc()
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()
	ctx, cancelTimeout := context.WithTimeout(taskCtx, 45*time.Second)
	defer cancelTimeout()

	bundleURL := (&url.URL{Scheme: "file", Path: page}).String()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(bundleURL),
		chromedp.WaitVisible(`//*[@id="story-player"]//header`, chromedp.BySearch),
	); err != nil {
		t.Fatalf("open the bundle: %v", err)
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
	if !strings.Contains(first, "portrait-player.svg") {
		t.Errorf("expected the protagonist's portrait on the stage, got:\n%s", first)
	}

	// The narration beat renders through the theatre's dialogue panel.
	advance(t, ctx)
	waitFor(t, ctx, "NARRATOR", "The quay is quiet.")

	// The speech beat puts the speaker's face on the stage beside their line.
	advance(t, ctx)
	waitFor(t, ctx, "GARRICK", "Keep moving.", "portrait-garrick.svg")
}

// advance steps the bundle to its next beat through the transport's own control.
func advance(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Click(`button[data-transport="next"]`, chromedp.ByQuery)); err != nil {
		t.Fatalf("advance the bundle: %v", err)
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

// bundleDom reports what the player actually rendered, so a failure says whether
// the stage, the portraits, or the dialogue is missing.
func bundleDom(t *testing.T, ctx context.Context) string {
	t.Helper()
	const script = `JSON.stringify({
	  header: (document.querySelector('header') || {}).innerText || '',
	  portraits: Array.from(document.querySelectorAll('img')).map(img => (img.getAttribute('src') || '').split('/').pop()),
	  namePlate: (document.querySelector('#story-player [class*="absolute -top-3.5"]') || {}).textContent || '',
	  text: (document.querySelector('#story-player [class*="text-stone-2"]') || {}).textContent || document.getElementById('story-player').innerText.slice(0, 400),
	  transport: Array.from(document.querySelectorAll('#story-player button')).map(b => b.getAttribute('title') || b.textContent || '').join('|'),
	  backgrounds: Array.from(document.querySelectorAll('#story-player [style*="background"]')).length,
	})`
	var out string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &out)); err != nil {
		return "probe failed: " + err.Error()
	}
	return out
}
