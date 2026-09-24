package telemetry

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPTransport wraps base so outbound calls become client spans. A nil base
// means http.DefaultTransport, and a caller's injected client (for example an
// httptest server) is preserved.
func HTTPTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return otelhttp.NewTransport(base)
}
