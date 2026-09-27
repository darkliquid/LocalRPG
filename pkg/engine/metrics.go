package engine

import (
	"sync"

	"go.opentelemetry.io/otel"
	otelmetric "go.opentelemetry.io/otel/metric"

	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// engineInstruments caches the turn instruments. It is rebuilt whenever the
// global meter provider changes, so a test that installs an in-memory provider
// gets instruments bound to it rather than to a previous test's provider.
type engineInstruments struct {
	turnDuration     otelmetric.Float64Histogram
	turnCompleted    otelmetric.Int64Counter
	turnTTFT         otelmetric.Float64Histogram
	providerDuration otelmetric.Float64Histogram
	providerErrors   otelmetric.Int64Counter
	toolDuration     otelmetric.Float64Histogram
	toolRounds       otelmetric.Int64Histogram
}

var (
	instrumentsMu       sync.Mutex
	instrumentsProvider otelmetric.MeterProvider
	instruments         engineInstruments
)

func engineMetrics() engineInstruments {
	provider := otel.GetMeterProvider()
	instrumentsMu.Lock()
	defer instrumentsMu.Unlock()
	if provider == instrumentsProvider {
		return instruments
	}
	meter := provider.Meter(telemetry.MeterName)
	instruments = engineInstruments{
		turnDuration:     telemetry.Float64Histogram(meter, "localrpg.turn.duration", "ms", "Wall-clock duration of one turn."),
		turnCompleted:    telemetry.Int64Counter(meter, "localrpg.turn.completed", "1", "Turns that completed."),
		turnTTFT:         telemetry.Float64Histogram(meter, "localrpg.turn.ttft", "ms", "Time to the first streamed chunk of a turn."),
		providerDuration: telemetry.Float64Histogram(meter, "localrpg.provider.request.duration", "ms", "Duration of one provider round."),
		providerErrors:   telemetry.Int64Counter(meter, "localrpg.provider.errors", "1", "Provider rounds that failed."),
		toolDuration:     telemetry.Float64Histogram(meter, "localrpg.tool.call.duration", "ms", "Duration of one tool call."),
		toolRounds:       telemetry.Int64Histogram(meter, "localrpg.tool.rounds", "1", "Tool rounds used by a turn."),
	}
	instrumentsProvider = provider
	return instruments
}
