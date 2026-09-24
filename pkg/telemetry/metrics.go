package telemetry

import (
	"context"

	otelmetric "go.opentelemetry.io/otel/metric"
)

// MeterName is the instrumentation scope every application instrument uses.
const MeterName = "github.com/darkliquid/localrpg"

// Int64Counter builds a counter on the supplied meter. A build error yields a
// no-op instrument, so recording never fails a turn.
func Int64Counter(meter otelmetric.Meter, name, unit, description string) otelmetric.Int64Counter {
	instrument, err := meter.Int64Counter(name, otelmetric.WithUnit(unit), otelmetric.WithDescription(description))
	if err != nil {
		return noopInt64Counter{}
	}
	return instrument
}

// Float64Histogram builds a histogram on the supplied meter.
func Float64Histogram(meter otelmetric.Meter, name, unit, description string) otelmetric.Float64Histogram {
	instrument, err := meter.Float64Histogram(name, otelmetric.WithUnit(unit), otelmetric.WithDescription(description))
	if err != nil {
		return noopFloat64Histogram{}
	}
	return instrument
}

// Int64Histogram builds an integer histogram on the supplied meter.
func Int64Histogram(meter otelmetric.Meter, name, unit, description string) otelmetric.Int64Histogram {
	instrument, err := meter.Int64Histogram(name, otelmetric.WithUnit(unit), otelmetric.WithDescription(description))
	if err != nil {
		return noopInt64Histogram{}
	}
	return instrument
}

type noopInt64Counter struct{ otelmetric.Int64Counter }

func (noopInt64Counter) Add(context.Context, int64, ...otelmetric.AddOption) {}

type noopFloat64Histogram struct{ otelmetric.Float64Histogram }

func (noopFloat64Histogram) Record(context.Context, float64, ...otelmetric.RecordOption) {}

type noopInt64Histogram struct{ otelmetric.Int64Histogram }

func (noopInt64Histogram) Record(context.Context, int64, ...otelmetric.RecordOption) {}
