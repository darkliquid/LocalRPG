package gui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestActionIDMiddleware(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actionID := gui.ActionIDFromContext(r.Context())
		if actionID != "act-999" {
			t.Errorf("Expected action ID act-999 in context, got %s", actionID)
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := otelhttp.NewHandler(gui.ActionCorrelationMiddleware(innerHandler), "test.http")

	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	req.Header.Set("X-LocalRPG-Action-ID", "act-999")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatalf("Expected recorded spans")
	}

	var found bool
	for _, kv := range spans[0].Attributes {
		if string(kv.Key) == "localrpg.action.id" && kv.Value.AsString() == "act-999" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected span to have attribute localrpg.action.id = act-999")
	}
}
