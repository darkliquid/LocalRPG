// Package e2e holds the end-to-end browser suite and the harness it runs on.
//
// The browser tests carry the `e2e` build tag, so the default `go test ./...`
// never compiles them and a machine without Chrome stays fast. Run them with
// `mise run test:e2e` (`go test -tags e2e ./pkg/e2e/...`).
package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/log"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/darkliquid/localrpg/pkg/driver"
)

const defaultTimeout = 45 * time.Second

// requireBrowser returns a usable browser path, or skips the test. Finding the
// binary is not enough, so it asks driver.Available, which launches one; the two
// share their allocator options, so a capability probe cannot disagree with a run.
func requireBrowser(t *testing.T) string {
	t.Helper()
	path := driver.ChromePath()
	if path == "" {
		t.Skip("no chrome/chromium available; skipping browser test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := driver.Available(ctx); err != nil {
		t.Skipf("no usable browser in this environment; skipping browser test: %v", err)
	}
	return path
}

// Browser is a headless Chrome session driving one page.
type Browser struct {
	t       *testing.T
	ctx     context.Context
	cancel  context.CancelFunc
	baseURL string
	dir     string

	mu      sync.Mutex
	console []string
}

// NewBrowser launches a browser bound to baseURL and skips the test when the
// host has no usable one. baseURL may be empty for a page the test builds itself
// (a file:// probe); only a relative Navigate path needs it.
func NewBrowser(t *testing.T, baseURL string) *Browser {
	t.Helper()
	browserPath := requireBrowser(t)

	allocOptions := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browserPath),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.WSURLReadTimeout(defaultTimeout),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocOptions...)
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	ctx, cancelTimeout := context.WithTimeout(taskCtx, defaultTimeout)

	b := &Browser{t: t, ctx: ctx, baseURL: baseURL, dir: artifactDir(t)}
	b.cancel = func() { cancelTimeout(); cancelTask(); cancelAlloc() }

	chromedp.ListenTarget(taskCtx, func(ev any) {
		switch event := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			parts := make([]string, 0, len(event.Args))
			for _, arg := range event.Args {
				parts = append(parts, string(arg.Value))
			}
			b.record(event.Type.String() + ": " + strings.Join(parts, " "))
		case *runtime.EventExceptionThrown:
			b.record("exception: " + event.ExceptionDetails.Error())
		case *log.EventEntryAdded:
			b.record("log: " + event.Entry.Text)
		}
	})

	t.Cleanup(b.cancel)
	return b
}

func (b *Browser) record(entry string) {
	b.mu.Lock()
	b.console = append(b.console, entry)
	b.mu.Unlock()
}

// Console returns the page's console and exception entries so far.
func (b *Browser) Console() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.console...)
}

// run executes actions under the session deadline. A failure captures artifacts
// and the console, because a CDP error alone rarely says what the page did.
func (b *Browser) run(actions ...chromedp.Action) {
	b.t.Helper()
	if err := chromedp.Run(b.ctx, actions...); err != nil {
		b.dumpArtifacts()
		b.t.Fatalf("%v\npage console:\n%s", err, strings.Join(b.Console(), "\n"))
	}
}

func (b *Browser) resolve(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "file:") {
		return path
	}
	return strings.TrimRight(b.baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

// Navigate opens a path relative to the fixture's server, or an absolute URL.
func (b *Browser) Navigate(path string) {
	b.t.Helper()
	b.run(chromedp.Navigate(b.resolve(path)))
}

// WaitVisible waits for the selector to be visible.
func (b *Browser) WaitVisible(selector string) {
	b.t.Helper()
	b.run(chromedp.WaitVisible(selector, chromedp.BySearch))
}

// Click waits for the selector, then clicks it.
func (b *Browser) Click(selector string) {
	b.t.Helper()
	b.run(chromedp.WaitVisible(selector, chromedp.BySearch), chromedp.Click(selector, chromedp.BySearch))
}

// Type waits for the selector, then sends the text.
func (b *Browser) Type(selector, text string) {
	b.t.Helper()
	b.run(chromedp.WaitVisible(selector, chromedp.BySearch), chromedp.SendKeys(selector, text, chromedp.BySearch))
}

// SetInputValue sets a controlled input's value the way React observes it: it
// writes through the native value setter and dispatches an input event. Setting
// the element's value directly is invisible to a controlled component, which
// restores it on the next render, and so is a keyboard clear.
func (b *Browser) SetInputValue(selector, value string) {
	b.t.Helper()
	script := fmt.Sprintf(`(() => {
	  const el = document.evaluate(%q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
	  if (!el) return 'missing';
	  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
	  Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, %q);
	  el.dispatchEvent(new Event('input', { bubbles: true }));
	  return 'ok';
	})()`, selector, value)
	if got := b.Eval(script); got != "ok" {
		b.t.Fatalf("set input %q: %s", selector, got)
	}
}

// Text reads a selector's text content.
func (b *Browser) Text(selector string) string {
	b.t.Helper()
	var out string
	b.run(chromedp.WaitVisible(selector, chromedp.BySearch), chromedp.Text(selector, &out, chromedp.BySearch))
	return out
}

// InputValue reads an input's value.
func (b *Browser) InputValue(selector string) string {
	b.t.Helper()
	var out string
	b.run(chromedp.WaitVisible(selector, chromedp.BySearch), chromedp.Value(selector, &out, chromedp.BySearch))
	return out
}

// BodyText reads the whole page's inner text.
func (b *Browser) BodyText() string {
	b.t.Helper()
	return b.Eval(`document.body.innerText`)
}

// Eval runs a script and returns its string result.
func (b *Browser) Eval(script string) string {
	b.t.Helper()
	var out string
	b.run(chromedp.Evaluate(script, &out))
	return out
}

// WaitFor polls the page until every needle is present, or the deadline passes.
func (b *Browser) WaitFor(needles ...string) {
	b.t.Helper()
	b.waitFor("show", func(text string) bool {
		for _, needle := range needles {
			if !strings.Contains(strings.ToUpper(text), strings.ToUpper(needle)) {
				return false
			}
		}
		return true
	}, strings.Join(needles, ", "))
}

// WaitForGone polls the page until the needle is absent.
func (b *Browser) WaitForGone(needle string) {
	b.t.Helper()
	b.waitFor("lose", func(text string) bool {
		return !strings.Contains(text, needle)
	}, needle)
}

func (b *Browser) waitFor(verb string, ok func(string) bool, what string) {
	b.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last string
	for {
		last = b.BodyText()
		if ok(last) {
			return
		}
		if time.Now().After(deadline) {
			b.dumpArtifacts()
			b.t.Fatalf("the page never did %s %q:\n%s", verb, what, last)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Poll waits for an expression to become true, or the deadline passes. The
// expression is evaluated in the page and must yield a boolean.
func (b *Browser) Poll(expression, describe string) {
	b.t.Helper()
	pollCtx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()
	if err := chromedp.Run(pollCtx, chromedp.Poll(expression, nil)); err != nil {
		b.dumpArtifacts()
		b.t.Fatalf("the page never became ready (%s): %v", describe, err)
	}
}

// InstrumentAudio installs a probe that records every Audio element the page
// creates and whether its play() promise settled, so a test can tell "the page
// played the clip" from "the page created a clip it never played".
func (b *Browser) InstrumentAudio() {
	b.t.Helper()
	b.run(chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(instrumentAudioScript).Do(ctx)
		return err
	}))
}

// Clips returns the recorded audio probe as JSON.
func (b *Browser) Clips() string {
	b.t.Helper()
	return b.Eval(`JSON.stringify(window.__clips || [])`)
}

// WaitForClips polls until the recorded clips have settled, or the deadline passes.
func (b *Browser) WaitForClips() string {
	b.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		clips := b.Clips()
		settled := clips != "[]" &&
			(!strings.Contains(clips, `"played":false`) || strings.Contains(clips, `"error":"`))
		if settled || time.Now().After(deadline) {
			return clips
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Screenshot writes a PNG of the viewport to path.
func (b *Browser) Screenshot(path string) {
	b.t.Helper()
	var buf []byte
	b.run(chromedp.CaptureScreenshot(&buf))
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		b.t.Fatalf("write screenshot: %v", err)
	}
}

// FullScreenshot captures the whole page as a JPEG at the given quality.
func (b *Browser) FullScreenshot(quality int) []byte {
	b.t.Helper()
	var buf []byte
	b.run(chromedp.FullScreenshot(&buf, quality))
	return buf
}

const instrumentAudioScript = `(function () {
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

// artifactDir resolves where a test's failure artifacts go. CI sets
// E2E_ARTIFACT_DIR and uploads it; locally it defaults to test-results/.
func artifactDir(t *testing.T) string {
	base := os.Getenv("E2E_ARTIFACT_DIR")
	if base == "" {
		base = "test-results"
	}
	return filepath.Join(base, strings.NewReplacer("/", "_", " ", "_", "\\", "_").Replace(t.Name()))
}

// dumpArtifacts writes the page's text, a DOM probe, and a screenshot. It runs
// while a test is already failing, so every step ignores its own error.
func (b *Browser) dumpArtifacts() {
	if b.dir == "" {
		return
	}
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return
	}
	var text string
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(`document.body.innerText`, &text)); err == nil {
		_ = os.WriteFile(filepath.Join(b.dir, "body.txt"), []byte(text), 0o644)
	}
	var probe string
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(domProbeScript, &probe)); err == nil {
		_ = os.WriteFile(filepath.Join(b.dir, "dom.json"), []byte(probe), 0o644)
	}
	var buf []byte
	if err := chromedp.Run(b.ctx, chromedp.CaptureScreenshot(&buf)); err == nil {
		_ = os.WriteFile(filepath.Join(b.dir, "failure.png"), buf, 0o644)
	}
}

const domProbeScript = `JSON.stringify({
  url: location.href,
  title: document.title,
  mains: document.querySelectorAll('main').length,
  buttons: document.querySelectorAll('button').length,
  dialogs: document.querySelectorAll('[role="dialog"]').length,
  bodyStart: document.body.innerText.slice(0, 400),
})`
