package driver

import (
	"context"
	"encoding/base64"
	"fmt"
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
			err := d.executeStep(taskCtx, step)
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

func (d *Driver) executeStep(ctx context.Context, step Step) error {
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

	case ActionSleep:
		time.Sleep(timeout)
		return nil

	default:
		return nil
	}
}
