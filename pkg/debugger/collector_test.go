package debugger_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/darkliquid/localrpg/pkg/debugger"
)

func TestCollectorRingBuffer(t *testing.T) {
	coll := debugger.NewCollector(5, 5) // Cap at 5 spans and 5 logs

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(coll))
	defer tp.Shutdown(context.Background())
	tracer := tp.Tracer("test")

	for i := 0; i < 7; i++ {
		_, span := tracer.Start(context.Background(), "span")
		span.SetAttributes(attribute.Int("index", i))
		span.End()
	}

	spans := coll.GetSpans()
	if len(spans) != 5 {
		t.Fatalf("Expected 5 spans due to ring buffer capacity, got %d", len(spans))
	}

	// Verify ring kept newest spans (indices 2 to 6)
	firstSpan := spans[0]
	if firstSpan.Attributes["index"] != "2" {
		t.Errorf("Expected oldest retained span to have index 2, got %v", firstSpan.Attributes["index"])
	}
}

func TestCollectorFindByActionID(t *testing.T) {
	coll := debugger.NewCollector(100, 100)
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(coll))
	defer tp.Shutdown(context.Background())
	tracer := tp.Tracer("test")

	_, span1 := tracer.Start(context.Background(), "turn-op")
	span1.SetAttributes(attribute.String("localrpg.action.id", "act-42"))
	span1.End()

	_, span2 := tracer.Start(context.Background(), "other-op")
	span2.End()

	matching := coll.FindSpansByActionID("act-42")
	if len(matching) != 1 {
		t.Fatalf("Expected 1 matching span, got %d", len(matching))
	}
	if matching[0].Name != "turn-op" {
		t.Errorf("Expected span 'turn-op', got %s", matching[0].Name)
	}
}
