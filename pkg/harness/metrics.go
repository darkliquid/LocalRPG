package harness

import (
	"sync"

	"go.opentelemetry.io/otel"
	otelmetric "go.opentelemetry.io/otel/metric"

	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// harnessInstruments caches the context instruments, rebuilt when the global
// meter provider changes so a test's in-memory provider is observed.
type harnessInstruments struct {
	contextTokens otelmetric.Int64Histogram
}

var (
	harnessInstrumentsMu       sync.Mutex
	harnessInstrumentsProvider otelmetric.MeterProvider
	harnessInstrumentSet       harnessInstruments
)

func contextMetrics() harnessInstruments {
	provider := otel.GetMeterProvider()
	harnessInstrumentsMu.Lock()
	defer harnessInstrumentsMu.Unlock()
	if provider == harnessInstrumentsProvider {
		return harnessInstrumentSet
	}
	meter := provider.Meter(telemetry.MeterName)
	harnessInstrumentSet = harnessInstruments{
		contextTokens: telemetry.Int64Histogram(meter, "localrpg.context.tokens", "tokens", "Tokens per assembled context section."),
	}
	harnessInstrumentsProvider = provider
	return harnessInstrumentSet
}
