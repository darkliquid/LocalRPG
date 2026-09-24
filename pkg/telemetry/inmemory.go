package telemetry

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	logglobal "go.opentelemetry.io/otel/log/global"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// memoryLogExporter keeps log records in memory for tests.
type memoryLogExporter struct {
	mu      sync.Mutex
	records []log.Record
}

func (m *memoryLogExporter) Export(_ context.Context, records []log.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, records...)
	return nil
}

func (m *memoryLogExporter) Shutdown(context.Context) error   { return nil }
func (m *memoryLogExporter) ForceFlush(context.Context) error { return nil }

func (m *memoryLogExporter) Records() []log.Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]log.Record(nil), m.records...)
}

// Recorder exposes what an in-memory provider captured.
type Recorder struct {
	exporter *tracetest.InMemoryExporter
	reader   *metric.ManualReader
	logs     *memoryLogExporter
}

// Spans returns every span recorded so far, as snapshots so attributes are
// readable after the span has ended.
func (r *Recorder) Spans() []sdktrace.ReadOnlySpan {
	stubs := r.exporter.GetSpans()
	out := make([]sdktrace.ReadOnlySpan, 0, len(stubs))
	for _, stub := range stubs {
		out = append(out, stub.Snapshot())
	}
	return out
}

// Logs returns every log record recorded so far.
func (r *Recorder) Logs() []log.Record { return r.logs.Records() }

// Metrics collects the current metric snapshot. A collection failure is
// returned, not ignored.
func (r *Recorder) Metrics(ctx context.Context) (metricdata.ResourceMetrics, error) {
	var out metricdata.ResourceMetrics
	if err := r.reader.Collect(ctx, &out); err != nil {
		return metricdata.ResourceMetrics{}, err
	}
	return out, nil
}

// Reset drops recorded spans so a test can assert on a later phase.
func (r *Recorder) Reset() { r.exporter.Reset() }

// NewInMemory installs in-memory providers as the OTel globals for tests and
// returns the recorder. ResetGlobalForTest must be deferred.
func NewInMemory() (*Recorder, *Provider, error) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	reader := metric.NewManualReader()
	mp := metric.NewMeterProvider(metric.WithReader(reader))
	logs := &memoryLogExporter{}
	lp := log.NewLoggerProvider(log.WithProcessor(log.NewSimpleProcessor(logs)))

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	logglobal.SetLoggerProvider(lp)

	p := &Provider{
		tracer: tp,
		meter:  mp,
		logger: lp,
		shut:   []func(context.Context) error{tp.Shutdown, mp.Shutdown, lp.Shutdown},
	}
	return &Recorder{exporter: exporter, reader: reader, logs: logs}, p, nil
}

// ResetGlobalForTest restores no-op globals so one test cannot leak into another.
func ResetGlobalForTest() {
	otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
	otel.SetMeterProvider(noopmetric.NewMeterProvider())
	logglobal.SetLoggerProvider(log.NewLoggerProvider())
}
