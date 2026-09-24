package media

import (
	"sync"

	"go.opentelemetry.io/otel"
	otelmetric "go.opentelemetry.io/otel/metric"

	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// mediaInstruments caches the media instruments, rebuilt when the global meter
// provider changes so a test's in-memory provider is observed.
type mediaInstruments struct {
	ttsDuration otelmetric.Float64Histogram
	ttsBytes    otelmetric.Int64Histogram
	ttsCache    otelmetric.Int64Counter
}

var (
	mediaInstrumentsMu       sync.Mutex
	mediaInstrumentsProvider otelmetric.MeterProvider
	mediaInstrumentSet       mediaInstruments
)

func mediaMetrics() mediaInstruments {
	provider := otel.GetMeterProvider()
	mediaInstrumentsMu.Lock()
	defer mediaInstrumentsMu.Unlock()
	if provider == mediaInstrumentsProvider {
		return mediaInstrumentSet
	}
	meter := provider.Meter(telemetry.MeterName)
	mediaInstrumentSet = mediaInstruments{
		ttsDuration: telemetry.Float64Histogram(meter, "localrpg.media.tts.duration", "ms", "Duration of one speech synthesis."),
		ttsBytes:    telemetry.Int64Histogram(meter, "localrpg.media.tts.bytes", "bytes", "Size of one synthesized clip."),
		ttsCache:    telemetry.Int64Counter(meter, "localrpg.media.tts.cache", "1", "Speech cache hits and misses."),
	}
	mediaInstrumentsProvider = provider
	return mediaInstrumentSet
}
