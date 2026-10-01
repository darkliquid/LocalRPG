package gui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/darkliquid/localrpg/pkg/driver"
	"github.com/darkliquid/localrpg/pkg/engine"
)

// requireBrowser returns the path to a working browser, or skips the test if
// no browser is installed or can be started in this environment.
func requireBrowser(t *testing.T) string {
	t.Helper()
	browser := driver.ChromePath()
	if browser == "" {
		t.Skip("no chrome/chromium available; skipping browser test")
	}
	probeCtx, cancelProbe := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancelProbe()
	if err := driver.Available(probeCtx); err != nil {
		t.Skipf("no usable browser in this environment; skipping browser test: %v", err)
	}
	return browser
}

// launcherFixture builds a playable campaign with three turns in a temp root.
func launcherFixture(t *testing.T) *Service {
	t.Helper()

	root := t.TempDir()
	configYAML := "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n"
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root)
	t.Cleanup(svc.Close)
	paths := svc.GetResolver()

	sysDir := paths.SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	worldDir := paths.WorldDir("harbour-realm")
	if err := os.MkdirAll(filepath.Join(worldDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}

	session, err := engine.InitGame(paths, engine.InitOptions{
		GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Sean",
	})
	if err != nil {
		t.Fatalf("InitGame: %v", err)
	}
	_ = session.Close()

	history := filepath.Join(paths.GameDir("campaign-01"), "history.jsonl")
	lines := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"You look around."}` + "\n" +
		`{"number":2,"timestamp":"2026-09-21T10:05:00Z","mode":"Do","input":"wait","narration":"Time passes."}` + "\n" +
		`{"number":3,"timestamp":"2026-09-21T10:10:00Z","mode":"Do","input":"sleep","narration":"You rest."}` + "\n"
	if err := os.WriteFile(history, []byte(lines), 0644); err != nil {
		t.Fatal(err)
	}
	return svc
}

// TestDeletingTheLastCampaignResetsLauncherStats is the feedback loop for the
// report "when deleting a campaign, the stats in the top right of the launcher
// should reset". It drives the real SPA in a headless browser, deletes the only
// campaign, and asserts the stats badge is gone.
func TestDeletingTheLastCampaignResetsLauncherStats(t *testing.T) {
	browser := requireBrowser(t)

	svc := launcherFixture(t)
	server := httptest.NewServer(NewServer(svc, AssetHandler()))
	defer server.Close()

	allocOptions := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.WSURLReadTimeout(45*time.Second),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocOptions...)
	defer cancelAlloc()
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()
	ctx, cancelTimeout := context.WithTimeout(taskCtx, 45*time.Second)
	defer cancelTimeout()

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/"),
		chromedp.WaitVisible(`//button[@aria-label="Campaign Settings"]`, chromedp.BySearch),
	); err != nil {
		t.Fatalf("load launcher: %v", err)
	}

	if text := bodyText(t, ctx); !strings.Contains(text, "3 turns") {
		t.Fatalf("stats badge does not show the campaign's turn count before deletion:\n%s", text)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(`//button[@aria-label="Campaign Settings"]`, chromedp.BySearch),
		chromedp.WaitVisible(`//button[normalize-space()='Delete']`, chromedp.BySearch),
		chromedp.Click(`//button[normalize-space()='Delete']`, chromedp.BySearch),
		chromedp.WaitVisible(`//button[normalize-space()='Confirm Delete']`, chromedp.BySearch),
		chromedp.Click(`//button[normalize-space()='Confirm Delete']`, chromedp.BySearch),
	); err != nil {
		t.Fatalf("delete the campaign: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		text := bodyText(t, ctx)
		if !strings.Contains(text, "3 turns") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the launcher still shows the deleted campaign's stats after deletion:\n%s\n\nDOM probe:\n%s\n\n/api/games:\n%s", text, domProbe(t, ctx), apiProbe(t, server.URL))
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// domProbe reports the shape of the live page so a stale render can be
// attributed: how many hero stages are mounted, and whether both the stats badge
// and the zero-state are present at once.
func domProbe(t *testing.T, ctx context.Context) string {
	t.Helper()
	const script = `JSON.stringify({
	  mains: document.querySelectorAll('main').length,
	  settingsButtons: document.querySelectorAll('button[aria-label="Campaign Settings"]').length,
	  zeroState: document.body.innerText.includes('No Campaigns Yet'),
	  statsBadge: document.body.innerText.includes('PLAY TIME'),
	  settingsModal: document.body.innerText.includes('Delete Campaign'),
	  mainChildren: Array.from((document.querySelector('main') || {children: []}).children).map(c => c.className),
	  badges: Array.from(document.querySelectorAll('[class*="justify-end anim-slide-in-up"]')).map(el => ({
	    visible: el.offsetParent !== null,
	    text: (el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 60),
	    parent: el.parentElement ? el.parentElement.className.slice(0, 40) : '',
	  })),
	  url: location.href,
	})`
	var out string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &out)); err != nil {
		return "probe failed: " + err.Error()
	}
	return out
}

// apiProbe reads the games list the moment the assertion fails, to separate a
// server that still lists the campaign from a client that still renders it.
func apiProbe(t *testing.T, baseURL string) string {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/games")
	if err != nil {
		return "request failed: " + err.Error()
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "read failed: " + err.Error()
	}
	return resp.Status + "\n" + string(body)
}

func bodyText(t *testing.T, ctx context.Context) string {
	t.Helper()
	var text string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText`, &text)); err != nil {
		t.Fatalf("read page text: %v", err)
	}
	return text
}
