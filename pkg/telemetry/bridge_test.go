package telemetry_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

func TestBridgeForwardsToLocalAndOTel(t *testing.T) {
	recorder, provider, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	local := trace.NewMemory(trace.LevelFull)
	bridge := provider.Logger(local)
	bridge.Event("tool.call", map[string]interface{}{"name": "search_entities"})

	if _, ok := local.Find("tool.call"); !ok {
		t.Fatal("local sink must still receive the event")
	}
	if len(recorder.Logs()) == 0 {
		t.Fatal("expected an OTel log record")
	}
}

func TestEventCtxAttachesSpanEvent(t *testing.T) {
	recorder, provider, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	local := trace.NewMemory(trace.LevelFull)
	contextual, ok := provider.Logger(local).(trace.ContextLogger)
	if !ok {
		t.Fatal("bridge must implement trace.ContextLogger")
	}

	ctx, span := telemetry.Tracer("test").Start(context.Background(), "probe")
	contextual.EventCtx(ctx, "tool.call", map[string]interface{}{"name": "search_entities"})
	span.End()

	found := false
	for _, recorded := range recorder.Spans() {
		for _, event := range recorded.Events() {
			if event.Name == "tool.call" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected a span event from EventCtx")
	}
}

func TestDisabledProviderLoggerIsPassThrough(t *testing.T) {
	provider, err := telemetry.New(context.Background(), config.TelemetryConfig{}, telemetry.BuildInfo{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	local := trace.NewMemory(trace.LevelFull)
	if _, ok := provider.Logger(local).(trace.ContextLogger); ok {
		t.Fatal("disabled provider must not wrap the local logger")
	}
}
