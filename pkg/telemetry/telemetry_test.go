package telemetry_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

func TestInMemoryProviderCapturesSpans(t *testing.T) {
	recorder, provider, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	if !provider.Enabled() {
		t.Fatal("in-memory provider should report enabled for tests")
	}

	_, span := telemetry.Tracer("test").Start(context.Background(), "probe")
	span.End()

	spans := recorder.Spans()
	if len(spans) != 1 || spans[0].Name() != "probe" {
		t.Fatalf("expected one probe span, got %+v", spans)
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestDisabledConfigProducesNoProvider(t *testing.T) {
	provider, err := telemetry.New(context.Background(), config.TelemetryConfig{}, telemetry.BuildInfo{Version: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if provider.Enabled() {
		t.Fatal("zero config must produce a disabled provider")
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestResetGlobalForTestStopsCapture(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	telemetry.ResetGlobalForTest()

	_, span := telemetry.Tracer("test").Start(context.Background(), "after-reset")
	span.End()

	if got := len(recorder.Spans()); got != 0 {
		t.Fatalf("expected no spans after reset, got %d", got)
	}
}
