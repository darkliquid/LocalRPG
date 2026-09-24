// Package telemetry owns OpenTelemetry setup for LocalRPG. It is disabled by
// default: a disabled provider returns no-op tracers, meters, and loggers, so
// no caller branches on whether telemetry is on.
package telemetry

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	logglobal "go.opentelemetry.io/otel/log/global"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/darkliquid/localrpg/pkg/config"
)

// BuildInfo names the process in exported telemetry.
type BuildInfo struct {
	Version    string
	ConfigFile string
}

// Provider owns the OTel providers. Construct with New.
type Provider struct {
	tracer *sdktrace.TracerProvider
	meter  *metric.MeterProvider
	logger *log.LoggerProvider
	shut   []func(context.Context) error
}

// New builds providers from configuration. A disabled configuration returns a
// provider whose signals are no-ops and no error, so startup never depends on a
// collector.
func New(ctx context.Context, cfg config.TelemetryConfig, build BuildInfo) (*Provider, error) {
	if !cfg.Enabled {
		return &Provider{}, nil
	}

	endpoint := firstNonEmpty(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), cfg.Endpoint, "localhost:4317")
	serviceName := firstNonEmpty(os.Getenv("OTEL_SERVICE_NAME"), cfg.ServiceName, "localrpg")
	ratio := cfg.SampleRatio
	if ratio <= 0 || ratio > 1 {
		ratio = 1.0
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(build.Version),
		semconv.ServiceInstanceID(newInstanceID()),
	}
	if build.ConfigFile != "" {
		attrs = append(attrs, attribute.String("localrpg.config_file", build.ConfigFile))
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL, attrs...))
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	p := &Provider{}

	if cfg.Traces {
		exp, err := otlptracegrpc.New(ctx, grpcTraceOptions(endpoint, cfg)...)
		if err != nil {
			return nil, fmt.Errorf("telemetry: trace exporter: %w", err)
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exp),
			sdktrace.WithResource(res),
			sdktrace.WithSampler(samplerFromEnv(cfg, ratio)),
		)
		p.tracer = tp
		p.shut = append(p.shut, tp.Shutdown)
		otel.SetTracerProvider(tp)
	}

	if cfg.Metrics {
		exp, err := otlpmetricgrpc.New(ctx, grpcMetricOptions(endpoint, cfg)...)
		if err != nil {
			return nil, fmt.Errorf("telemetry: metric exporter: %w", err)
		}
		mp := metric.NewMeterProvider(
			metric.WithReader(metric.NewPeriodicReader(exp)),
			metric.WithResource(res),
		)
		p.meter = mp
		p.shut = append(p.shut, mp.Shutdown)
		otel.SetMeterProvider(mp)
	}

	if cfg.Logs {
		exp, err := otlploggrpc.New(ctx, grpcLogOptions(endpoint, cfg)...)
		if err != nil {
			return nil, fmt.Errorf("telemetry: log exporter: %w", err)
		}
		lp := log.NewLoggerProvider(
			log.WithProcessor(log.NewBatchProcessor(exp)),
			log.WithResource(res),
		)
		p.logger = lp
		p.shut = append(p.shut, lp.Shutdown)
		logglobal.SetLoggerProvider(lp)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	return p, nil
}

// Enabled reports whether any signal is exported.
func (p *Provider) Enabled() bool {
	return p != nil && (p.tracer != nil || p.meter != nil || p.logger != nil)
}

// Shutdown flushes every provider with a bounded timeout. It is safe to call on
// a disabled provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var firstErr error
	for _, shut := range p.shut {
		if err := shut(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Tracer returns a tracer from the currently installed global provider. Call at
// span creation time, never at package init, so a provider set later is seen.
func Tracer(name string) oteltrace.Tracer { return otel.Tracer(name) }

// Meter returns a meter from the currently installed global provider.
func Meter(name string) otelmetric.Meter { return otel.Meter(name) }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func newInstanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "localrpg"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func samplerFromEnv(cfg config.TelemetryConfig, ratio float64) sdktrace.Sampler {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER"))) {
	case "always_off":
		return sdktrace.NeverSample()
	case "always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	default:
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	}
}

func grpcTraceOptions(endpoint string, cfg config.TelemetryConfig) []otlptracegrpc.Option {
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(cfg.Headers))
	}
	return opts
}

func grpcMetricOptions(endpoint string, cfg config.TelemetryConfig) []otlpmetricgrpc.Option {
	opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlpmetricgrpc.WithHeaders(cfg.Headers))
	}
	return opts
}

func grpcLogOptions(endpoint string, cfg config.TelemetryConfig) []otlploggrpc.Option {
	opts := []otlploggrpc.Option{otlploggrpc.WithEndpoint(endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlploggrpc.WithHeaders(cfg.Headers))
	}
	return opts
}
