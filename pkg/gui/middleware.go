package gui

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// embeddedWebviewOrigins are the origins used by the native Wails webview when
// it requests the API through an in-process scheme handler rather than a
// network listener. They are trusted because those requests never leave the
// process.
var embeddedWebviewOrigins = []string{
	"wails://wails",
	"http://wails.localhost",
}

// ProtectCrossOrigin wraps h with the standard library's cross-origin request
// protection, so browsers on other sites cannot drive the local API while
// same-origin callers, local tooling, and the native webview keep working.
func ProtectCrossOrigin(h http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	for _, origin := range embeddedWebviewOrigins {
		_ = protection.AddTrustedOrigin(origin)
	}
	return protection.Handler(h)
}

type actionIDKey struct{}

// ActionIDHeader is the HTTP header used to correlate browser actions with backend spans.
const ActionIDHeader = "X-LocalRPG-Action-ID"

// ActionCorrelationMiddleware extracts X-LocalRPG-Action-ID from requests and annotates the trace context.
func ActionCorrelationMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actionID := r.Header.Get(ActionIDHeader)
		if actionID == "" {
			actionID = r.URL.Query().Get("action_id")
		}
		if actionID != "" {
			ctx := context.WithValue(r.Context(), actionIDKey{}, actionID)
			span := oteltrace.SpanFromContext(ctx)
			if span.IsRecording() {
				span.SetAttributes(attribute.String("localrpg.action.id", actionID))
			}
			r = r.WithContext(ctx)
		}
		h.ServeHTTP(w, r)
	})
}

// ActionIDFromContext retrieves the correlated action ID from context if present.
func ActionIDFromContext(ctx context.Context) string {
	if val, ok := ctx.Value(actionIDKey{}).(string); ok {
		return val
	}
	return ""
}
