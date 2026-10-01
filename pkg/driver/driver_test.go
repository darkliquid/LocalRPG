package driver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/driver"
)

func TestDriverExecution(t *testing.T) {
	// Having Chrome installed is not the same as being able to start it: a
	// container without a usable sandbox or a big enough /dev/shm has the binary
	// and still refuses. That is a property of the host, not a fault in the
	// driver, so skip instead of reporting a failure nothing here can fix.
	probeCtx, cancelProbe := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelProbe()
	if err := driver.Available(probeCtx); err != nil {
		t.Skipf("no usable browser in this environment; skipping live browser driver test: %v", err)
	}

	var requestedActionID atomic.Value

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if act := r.Header.Get("X-LocalRPG-Action-ID"); act != "" {
			requestedActionID.Store(act)
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html><html><body><div id="target">Hello World</div></body></html>`))
	}))
	defer server.Close()

	d := driver.New(driver.Config{
		BaseURL:  server.URL,
		Headless: true,
	})

	scenario := &driver.Scenario{
		Name: "Test Run",
		Steps: []driver.Step{
			{Action: driver.ActionNavigate, URL: "/"},
			{Action: driver.ActionWaitVisible, Selector: "#target", TimeoutMs: 2000},
			{Action: driver.ActionAssertVisible, Selector: "#target", TextContains: "Hello World"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	records, err := d.Run(ctx, scenario, nil)
	if err != nil {
		t.Fatalf("Driver run failed: %v", err)
	}

	if len(records) != 3 {
		t.Errorf("Expected 3 action records, got %d", len(records))
	}
	for i, r := range records {
		if r.Status != "passed" {
			t.Errorf("Step %d failed: %s", i, r.FailureReason)
		}
	}
}
