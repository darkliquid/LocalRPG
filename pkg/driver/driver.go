package driver

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"

	"github.com/darkliquid/localrpg/pkg/debugger"
)

// Config configures the Chrome DevTools Protocol driver.
type Config struct {
	BaseURL  string
	Headless bool
}

// Driver executes declarative scenario steps via Chrome DevTools Protocol.
type Driver struct {
	cfg Config
}

// New constructs a new CDP browser driver.
func New(cfg Config) *Driver {
	return &Driver{cfg: cfg}
}

// StepCallback is notified after each action step completes.
type StepCallback func(rec debugger.ActionRecord)

// Run executes all scenario steps sequentially, recording actions and telemetry tags.
func (d *Driver) Run(ctx context.Context, s *Scenario, cb StepCallback) ([]debugger.ActionRecord, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", d.cfg.Headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	// Chrome is not always on PATH (a distribution package installs to
	// /opt/google/chrome/chrome), so let the caller point at it, the same way
	// the browser tests do.
	if browser := os.Getenv("CHROME_EXEC"); browser != "" {
		opts = append(opts, chromedp.ExecPath(browser))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	var records []debugger.ActionRecord
	var currentActionID string
	var mu sync.Mutex

	// Intercept outbound network requests to inject X-LocalRPG-Action-ID
	chromedp.ListenTarget(taskCtx, func(ev interface{}) {
		switch ev.(type) {
		case *network.EventRequestWillBeSent:
			mu.Lock()
			_ = currentActionID
			mu.Unlock()
		}
	})

	for i, step := range s.Steps {
		actionID := uuid.NewString()
		mu.Lock()
		currentActionID = actionID
		mu.Unlock()

		start := time.Now()
		rec := debugger.ActionRecord{
			ID:         actionID,
			StepIndex:  i,
			ActionType: string(step.Action),
			Selector:   step.Selector,
			InputData:  step.Text,
			Timestamp:  start,
			Status:     "passed",
		}

		// Inject request headers for this step's network traffic
		headers := network.Headers{
			"X-LocalRPG-Action-ID": actionID,
		}
		if err := chromedp.Run(taskCtx, network.SetExtraHTTPHeaders(headers)); err != nil {
			rec.Status = "failed"
			rec.FailureReason = fmt.Sprintf("set headers: %v", err)
		} else {
			err := d.executeStep(taskCtx, step, &rec)
			if err != nil {
				rec.Status = "failed"
				rec.FailureReason = err.Error()

				// Capture failure screenshot
				var buf []byte
				if captureErr := chromedp.Run(taskCtx, chromedp.CaptureScreenshot(&buf)); captureErr == nil {
					rec.ScreenshotB64 = base64.StdEncoding.EncodeToString(buf)
				}
			}
		}

		rec.DurationMs = time.Since(start).Milliseconds()
		records = append(records, rec)
		if cb != nil {
			cb(rec)
		}

		if rec.Status == "failed" {
			return records, fmt.Errorf("step %d (%s) failed: %s", i, step.Action, rec.FailureReason)
		}
	}

	return records, nil
}

func (d *Driver) executeStep(ctx context.Context, step Step, rec *debugger.ActionRecord) error {
	timeout := 10 * time.Second
	if step.TimeoutMs > 0 {
		timeout = time.Duration(step.TimeoutMs) * time.Millisecond
	}

	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch step.Action {
	case ActionNavigate:
		targetURL := step.URL
		if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
			targetURL = strings.TrimRight(d.cfg.BaseURL, "/") + "/" + strings.TrimLeft(targetURL, "/")
		}
		return chromedp.Run(stepCtx, chromedp.Navigate(targetURL))

	case ActionClick:
		return chromedp.Run(stepCtx,
			chromedp.WaitVisible(step.Selector),
			chromedp.Click(step.Selector),
		)

	case ActionTypeInput:
		return chromedp.Run(stepCtx,
			chromedp.WaitVisible(step.Selector),
			chromedp.SendKeys(step.Selector, step.Text),
		)

	case ActionWaitVisible:
		return chromedp.Run(stepCtx, chromedp.WaitVisible(step.Selector))

	case ActionAssertVisible:
		var text string
		if err := chromedp.Run(stepCtx,
			chromedp.WaitVisible(step.Selector),
			chromedp.Text(step.Selector, &text),
		); err != nil {
			return err
		}
		if step.TextContains != "" && !strings.Contains(text, step.TextContains) {
			return fmt.Errorf("expected element '%s' to contain '%s', got '%s'", step.Selector, step.TextContains, text)
		}
		return nil

	case ActionScreenshot:
		if step.Path == "" {
			return fmt.Errorf("screenshot action requires a path")
		}

		// CaptureScreenshot writes a PNG of the viewport; the full-page variant
		// only emits JPEG. A desktop app is captured at a fixed window size, so
		// an optional width and height emulate the window it was designed for.
		actions := make([]chromedp.Action, 0, 2)
		if step.Width > 0 && step.Height > 0 {
			actions = append(actions, chromedp.EmulateViewport(int64(step.Width), int64(step.Height)))
		}
		var pngData []byte
		actions = append(actions, chromedp.CaptureScreenshot(&pngData))
		if err := chromedp.Run(stepCtx, actions...); err != nil {
			return err
		}

		data, err := encodeScreenshot(pngData, screenshotFormat(step), step.Quality)
		if err != nil {
			return err
		}
		if dir := filepath.Dir(step.Path); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create screenshot directory: %w", err)
			}
		}
		if err := os.WriteFile(step.Path, data, 0o644); err != nil {
			return fmt.Errorf("write screenshot: %w", err)
		}
		rec.ScreenshotB64 = base64.StdEncoding.EncodeToString(data)
		return nil

	case ActionSleep:
		time.Sleep(timeout)
		return nil

	default:
		return nil
	}
}

// defaultScreenshotQuality keeps small UI text legible after JPEG encoding.
const defaultScreenshotQuality = 88

// screenshotFormat resolves the image format for a capture. An explicit format
// wins; otherwise the file extension decides, so a ".jpg" path is never handed
// PNG bytes.
func screenshotFormat(step Step) string {
	if step.Format != "" {
		return strings.ToLower(step.Format)
	}
	switch strings.ToLower(filepath.Ext(step.Path)) {
	case ".jpg", ".jpeg":
		return "jpeg"
	default:
		return "png"
	}
}

// encodeScreenshot converts a captured PNG into the requested format. Chrome
// only ever hands back PNG, so a JPEG target is decoded and re-encoded here
// rather than pulling an external image tool into the capture flow.
func encodeScreenshot(pngData []byte, format string, quality int) ([]byte, error) {
	switch strings.ToLower(format) {
	case "", "png":
		return pngData, nil

	case "jpeg", "jpg":
		img, err := png.Decode(bytes.NewReader(pngData))
		if err != nil {
			return nil, fmt.Errorf("decode screenshot: %w", err)
		}
		if quality <= 0 || quality > 100 {
			quality = defaultScreenshotQuality
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("encode screenshot: %w", err)
		}
		return buf.Bytes(), nil

	default:
		return nil, fmt.Errorf("unsupported screenshot format %q", format)
	}
}
