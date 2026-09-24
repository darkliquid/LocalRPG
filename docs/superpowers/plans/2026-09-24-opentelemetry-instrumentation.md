# OpenTelemetry Instrumentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tracing, metrics, and logs to LocalRPG via the OpenTelemetry Go SDK, exported over OTLP/gRPC, with automatic SQLite and HTTP instrumentation, defaulting to fully disabled.

**Architecture:** A new `pkg/telemetry` package owns provider construction, global registration, shutdown, the OTLP exporters, the `pkg/trace` bridge, and test in-memory providers. Other packages obtain tracers/meters from the OTel global provider through thin `telemetry.Tracer`/`telemetry.Meter` helpers and place spans at the seams that already thread `ctx`. Storage instruments the `database/sql` driver once; the GUI instruments its mux once.

**Tech Stack:** Go 1.27.1, `go.opentelemetry.io/otel` + `/sdk` + OTLP/gRPC exporters, `otelhttp`, `otelslog`, `github.com/XSAM/otelsql`, `modernc.org/sqlite`.

**Spec:** `docs/superpowers/specs/2026-09-24-opentelemetry-instrumentation-design.md`

## Global Constraints

- Telemetry is **off by default**; no OTLP connection is attempted unless enabled in config or by environment.
- Only the OTLP/gRPC exporter is compiled in. Default endpoint `localhost:4317`, `insecure` true for localhost.
- Tests must **never** require a network or a collector; use in-memory providers only.
- Module path `github.com/darkliquid/localrpg`. Use `interface{}`, not `any`. Wrap errors with `fmt.Errorf("...: %w", err)`. Tests use only the standard library (`testing`, `t.TempDir()`) plus the OTel in-memory test packages.
- Keep `pkg/trace` JSONL output and the GUI Debug panel working unchanged.
- `go vet ./...` must stay clean; `go test -count=1 ./...` is the gate.
- Package import alias: `pkg/telemetry` imports OTel trace/metric as `oteltrace`/`otelmetric` and `pkg/trace` as `trace`.

---

### Task 1: Telemetry foundation, configuration, and lifecycle

**Files:**
- Create: `pkg/telemetry/telemetry.go`
- Create: `pkg/telemetry/inmemory.go`
- Create: `pkg/telemetry/telemetry_test.go`
- Modify: `pkg/config/types.go` (add `TelemetryConfig`, `Config.Telemetry`, `DefaultConfig`)
- Modify: `pkg/config/types_test.go`
- Modify: `cmd/localrpg/gui.go`
- Modify: `cmd/localrpg/play.go`
- Modify: `go.mod` (add OTel modules in this task)

**Interfaces:**
- Produces: `config.TelemetryConfig`, `telemetry.BuildInfo`, `telemetry.New`, `telemetry.NewInMemory`, `(*telemetry.Provider).Shutdown`, `(*telemetry.Provider).Enabled`, `telemetry.Tracer`, `telemetry.Meter`, `telemetry.Recorder`, `telemetry.ResetGlobalForTest`.

- [ ] **Step 1: Add the config block and its test**

In `pkg/config/types.go` add near `PreferencesConfig`:

```go
// TelemetryConfig configures OpenTelemetry export. The zero value is disabled,
// so configuration written before telemetry existed behaves as it did.
type TelemetryConfig struct {
	Enabled     bool              `yaml:"enabled" json:"enabled"`
	Endpoint    string            `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Protocol    string            `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	Insecure    bool              `yaml:"insecure,omitempty" json:"insecure,omitempty"`
	SampleRatio float64           `yaml:"sample_ratio,omitempty" json:"sample_ratio,omitempty"`
	Traces      bool              `yaml:"traces" json:"traces"`
	Metrics     bool              `yaml:"metrics" json:"metrics"`
	Logs        bool              `yaml:"logs" json:"logs"`
	Headers     map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	ServiceName string            `yaml:"service_name,omitempty" json:"service_name,omitempty"`
}
```

Add `Telemetry TelemetryConfig `yaml:"telemetry,omitempty" json:"telemetry,omitempty"`` to `type Config struct`. In `DefaultConfig`, set:

```go
Telemetry: TelemetryConfig{Enabled: false, Endpoint: "localhost:4317", Protocol: "grpc", Insecure: true, SampleRatio: 1.0, Traces: true, Metrics: true, Logs: true, ServiceName: "localrpg"},
```

In `pkg/config/types_test.go` add:

```go
func TestDefaultTelemetryIsDisabled(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Telemetry.Enabled {
		t.Fatal("telemetry must default to disabled")
	}
	if cfg.Telemetry.Endpoint != "localhost:4317" || !cfg.Telemetry.Traces {
		t.Fatalf("unexpected telemetry defaults: %+v", cfg.Telemetry)
	}
}
```

- [ ] **Step 2: Add the OTel dependencies**

Run:

```bash
go get go.opentelemetry.io/otel go.opentelemetry.io/otel/sdk go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc
go mod tidy
```

Expected: `go.mod` gains the modules; `go build ./...` still passes.

- [ ] **Step 3: Write the failing foundation test**

Create `pkg/telemetry/telemetry_test.go`:

```go
package telemetry_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

func TestDisabledProviderInstallsNoopAndShutsDown(t *testing.T) {
	recorder, provider, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	if !provider.Enabled() {
		t.Fatal("in-memory provider should report enabled for tests")
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if got := len(recorder.Spans()); got != 0 {
		t.Fatalf("expected no spans, got %d", got)
	}
}

func TestDisabledConfigProducesNoProvider(t *testing.T) {
	provider, err := telemetry.New(context.Background(), config.TelemetryConfig{}, telemetry.BuildInfo{Version: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if provider.Enabled() {
		t.Fatal("zero config must produce a disabled provider")
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test -run TestDisabled ./pkg/telemetry/ -v`
Expected: FAIL with "no Go files" / undefined `telemetry.NewInMemory`.

- [ ] **Step 5: Implement `pkg/telemetry/telemetry.go`**

```go
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
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

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
// provider whose signals are no-ops and no error.
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

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(build.Version),
	))
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}
	if build.ConfigFile != "" {
		// A non-semconv attribute is attached via the raw attribute API.
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
		mp := metric.NewMeterProvider(metric.WithReader(metric.NewPeriodicReader(exp)), metric.WithResource(res))
		p.meter = mp
		p.shut = append(p.shut, mp.Shutdown)
		otel.SetMeterProvider(mp)
	}

	if cfg.Logs {
		exp, err := otlploggrpc.New(ctx, grpcLogOptions(endpoint, cfg)...)
		if err != nil {
			return nil, fmt.Errorf("telemetry: log exporter: %w", err)
		}
		lp := log.NewLoggerProvider(log.WithProcessor(log.NewBatchProcessor(exp)), log.WithResource(res))
		p.logger = lp
		p.shut = append(p.shut, lp.Shutdown)
		global.SetLoggerProvider(lp)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage))
	return p, nil
}

// Enabled reports whether any signal is exported.
func (p *Provider) Enabled() bool {
	return p != nil && (p.tracer != nil || p.meter != nil || p.logger != nil)
}

// Shutdown flushes every provider with a bounded timeout. Safe to call when
// disabled.
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
```

Add `grpcTraceOptions`, `grpcMetricOptions`, `grpcLogOptions`, and `samplerFromEnv` in the same file (small helpers building `otlptracegrpc.WithEndpoint`, `WithInsecure`, `WithHeaders` and `sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))`); the exact helper bodies are mechanical and may be split into `options.go`.

- [ ] **Step 6: Implement `pkg/telemetry/inmemory.go`**

```go
package telemetry

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log/logtest"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// Recorder exposes what an in-memory provider captured.
type Recorder struct {
	exporter *tracetest.InMemoryExporter
	reader   *sdkmetric.ManualReader
	logs     *logtest.Recorder
}

func (r *Recorder) Spans() []sdktrace.ReadOnlySpan { return r.exporter.GetSpans() }
func (r *Recorder) Logs() []sdklog.Record          { return r.logs.Records() }
func (r *Recorder) Reset()                         { r.exporter.Reset() }

// Metrics collects the current metric snapshot. A collection failure is
// returned, not ignored.
func (r *Recorder) Metrics(ctx context.Context) (metricdata.ResourceMetrics, error) {
	var out metricdata.ResourceMetrics
	if err := r.reader.Collect(ctx, &out); err != nil {
		return metricdata.ResourceMetrics{}, err
	}
	return out, nil
}

// NewInMemory installs in-memory providers as the OTel globals for tests and
// returns the recorder. ResetGlobalForTest must be deferred.
func NewInMemory() (*Recorder, *Provider, error) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	recorder := logtest.NewRecorder()
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(recorder))

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	global.SetLoggerProvider(lp)

	p := &Provider{
		tracer: tp,
		meter:  mp,
		logger: lp,
		shut:   []func(context.Context) error{tp.Shutdown, mp.Shutdown, lp.Shutdown},
	}
	return &Recorder{exporter: exporter, reader: reader, logs: recorder}, p, nil
}

// ResetGlobalForTest restores no-op globals so one test cannot leak into another.
func ResetGlobalForTest() {
	otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
	otel.SetMeterProvider(otelmetric.NewNoopMeterProvider())
	global.SetLoggerProvider(global.GetLoggerProvider()) // see note
}
```

(Implementer note: the OTel log package exposes `global.SetLoggerProvider`;
to restore a no-op logger provider use the SDK's default `sdklog.NewLoggerProvider()`
with no processor. `NewNoopMeterProvider` lives in `go.opentelemetry.io/otel/metric`.)

- [ ] **Step 7: Run the foundation tests**

Run: `go test -count=1 ./pkg/telemetry/ ./pkg/config/ -v`
Expected: PASS.

- [ ] **Step 8: Wire lifecycle into the entry points**

In `cmd/localrpg/gui.go` and `cmd/localrpg/play.go`, after config load:

```go
provider, err := telemetry.New(ctx, cfg.Telemetry, telemetry.BuildInfo{Version: version, ConfigFile: configPath})
if err != nil {
	return fmt.Errorf("telemetry: %w", err)
}
defer func() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = provider.Shutdown(shutdownCtx)
}()
_ = provider // wired into loggers in Task 5
```

- [ ] **Step 9: Verify and commit**

Run: `go vet ./... && go test -count=1 ./...`
Expected: all packages pass.

```bash
git add pkg/telemetry pkg/config cmd/localrpg go.mod go.sum
git commit -m "feat(telemetry): add disabled-by-default OpenTelemetry foundation"
```

---

### Task 2: Turn, context, provider, and tool spans

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/harness/context.go`
- Modify: `pkg/engine/timeline.go`
- Create: `pkg/engine/orchestrator_telemetry_test.go`

**Interfaces:**
- Consumes: `telemetry.NewInMemory`, `telemetry.ResetGlobalForTest`, `telemetry.Tracer`.
- Produces: span names `turn`, `context.assemble`, `provider.generate`, `tool.call`, `extract.entities`, `continuity.check`, `timeline.record_turn`.

- [ ] **Step 1: Write the failing span-tree test**

Create `pkg/engine/orchestrator_telemetry_test.go` using the existing scripted-provider pattern from `pkg/engine/tools_loop_test.go`:

```go
func TestTurnProducesSpanTree(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	// Build an orchestrator whose gm provider scripts one tool round followed
	// by a final narration, exactly as tools_loop_test.go does.
	o := newToolLoopOrchestrator(t)
	if _, err := o.ProcessAction(context.Background(), "look around", nil); err != nil {
		t.Fatalf("ProcessAction: %v", err)
	}

	names := map[string]int{}
	var traceID oteltrace.TraceID
	for _, span := range recorder.Spans() {
		names[span.Name()]++
		if span.Name() == "turn" {
			traceID = span.SpanContext().TraceID()
		}
	}
	for _, want := range []string{"turn", "context.assemble", "provider.generate", "tool.call"} {
		if names[want] == 0 {
			t.Errorf("expected a %q span, got %v", want, names)
		}
	}
	if names["provider.generate"] < 2 {
		t.Errorf("expected one provider span per round, got %d", names["provider.generate"])
	}
	for _, span := range recorder.Spans() {
		if span.SpanContext().TraceID() != traceID {
			t.Errorf("span %q did not share the turn trace id", span.Name())
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestTurnProducesSpanTree ./pkg/engine/ -v`
Expected: FAIL — no `turn` span.

- [ ] **Step 3: Add spans to the orchestrator**

In `ProcessAction`, after `turn.begin` logging:

```go
ctx, span := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "turn",
	oteltrace.WithAttributes(
		attribute.String("game.id", o.gameID),
		attribute.Int("turn.number", turnNum),
		attribute.String("turn.mode", mode),
		attribute.String("turn.location", locationID),
	))
defer span.End()
```

Wrap the generation loop (in `runGenerationLoop`, inside the `for round` loop around `o.generateRequest`):

```go
roundCtx, roundSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "provider.generate",
	oteltrace.WithAttributes(
		attribute.String("localrpg.role", "gm"),
		attribute.Int("localrpg.round", round),
		attribute.Bool("localrpg.tools_offered", offerTools),
	))
result, err := o.generateRequest(roundCtx, request, onChunk)
if err != nil {
	roundSpan.RecordError(err)
	roundSpan.SetStatus(codes.Error, err.Error())
}
roundSpan.End()
```

Wrap each tool execution with `tool.call`, extraction with `extract.entities`, and the continuity pass with `continuity.check`, using the same pattern. Record `turn.outcome` and `turn.truncated` on the root span just before it ends.

- [ ] **Step 4: Add the context assembly span**

In `pkg/harness/context.go`, at the top of `(*ContextAssembler).Assemble`, start `context.assemble` from the request context (add a `Context context.Context` field to `harness.ContextRequest`, defaulting to `context.Background()` when nil), and for each returned section record a `section` span event. Set attributes `context.budget` and `context.tokens` from the returned assembly metadata.

- [ ] **Step 5: Add the timeline write span**

In `pkg/engine/timeline.go:RecordTurn`, wrap the body:

```go
ctx, span := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "timeline.record_turn",
	oteltrace.WithAttributes(attribute.Int("localrpg.turn.number", turn.Number)))
defer span.End()
```

(Thread `ctx` into `RecordTurn` if it does not already take one.)

- [ ] **Step 6: Run the tests**

Run: `go test -count=1 ./pkg/engine/ ./pkg/harness/ -v`
Expected: PASS, including all pre-existing engine/harness tests.

- [ ] **Step 7: Commit**

```bash
git add pkg/engine pkg/harness
git commit -m "feat(telemetry): trace the turn, context, provider, and tool spans"
```

---

### Task 3: Automatic SQLite and HTTP instrumentation

**Files:**
- Create: `pkg/storage/otel_driver.go`
- Create: `pkg/storage/otel_driver_test.go`
- Modify: `pkg/storage/db.go`
- Modify: `pkg/gui/server.go`
- Modify: `pkg/gui/server_test.go`
- Modify: `pkg/harness/http_provider.go`, `pkg/harness/gemini_provider.go`
- Modify: `pkg/media/elevenlabs_tts.go`, `pkg/media/gemini_image.go`
- Create: `pkg/telemetry/http.go`
- Create: `pkg/telemetry/http_test.go`

**Interfaces:**
- Consumes: `telemetry.NewInMemory`.
- Produces: `telemetry.HTTPTransport(base http.RoundTripper) http.RoundTripper`, `storage.openDBDriver`, wrapped route names on `http.server`.

- [ ] **Step 1: Prove `otelsql` wraps `modernc.org/sqlite`**

```bash
go get github.com/XSAM/otelsql
```

Create `pkg/storage/otel_driver_test.go`:

```go
package storage

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/telemetry"
)

func TestWrappedDriverRecordsQuerySpan(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	db, err := OpenDB(t.TempDir() + "/index.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(context.Background(), "CREATE TABLE probe (id INTEGER)"); err != nil {
		t.Fatalf("exec: %v", err)
	}

	found := false
	for _, span := range recorder.Spans() {
		if span.Name() == "storage.query" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a storage.query span, got %d spans", len(recorder.Spans()))
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestWrappedDriver ./pkg/storage/ -v`
Expected: FAIL — no `storage.query` span.

- [ ] **Step 3: Register the wrapped driver**

Create `pkg/storage/otel_driver.go`:

```go
package storage

import (
	"database/sql"
	"sync"

	"github.com/XSAM/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// otelDriverName is the database/sql driver name used by OpenDB. The driver is
// registered once; otelsql wraps modernc.org/sqlite's "sqlite" driver so every
// QueryContext/ExecContext becomes a span without changing any storage call.
const otelDriverName = "sqlite_otel"

var registerDriverOnce sync.Once

func registerOTelDriver() error {
	var err error
	registerDriverOnce.Do(func() {
		_, err = otelsql.Register("sqlite",
			otelsql.WithAttributes(semconv.DBSystemSqlite),
			otelsql.WithSpanOptions(otelsql.SpanOptions{OmitRows: true}),
		)
	})
	return err
}

func openWrapped(path, pragmas string) (*sql.DB, error) {
	if err := registerOTelDriver(); err != nil {
		return nil, err
	}
	return sql.Open(otelDriverName, "file:"+path+"?"+pragmas)
}
```

`otelsql.Register(driverName, opts...)` registers a new name wrapping the named
driver; if the installed `otelsql` signature differs, use
`otelsql.Register("sqlite", ...)` and capture the returned registered name.
Adjust `otelDriverName` to the value `Register` returns.

- [ ] **Step 4: Use the wrapped driver in `OpenDB`**

In `pkg/storage/db.go`, replace `sql.Open("sqlite", "file:"+path+"?"+pragmas)` with `openWrapped(path, pragmas)`.

- [ ] **Step 5: Run storage tests**

Run: `go test -count=1 ./pkg/storage/ -v`
Expected: PASS, including the new query-span test.

- [ ] **Step 6: Add the shared HTTP transport helper**

Create `pkg/telemetry/http.go`:

```go
package telemetry

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPTransport wraps base so outbound calls become client spans. A nil base
// means http.DefaultTransport, and the caller's injected client (e.g. an
// httptest server) is preserved.
func HTTPTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return otelhttp.NewTransport(base)
}
```

- [ ] **Step 7: Instrument the GUI server**

In `pkg/gui/server.go`, keep the mux and wrap it in the constructor:

```go
s.handler = otelhttp.NewHandler(s.mux, "localrpg.http",
	otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		return routePattern(r) // returns "/api/game/{id}/turn" style names
	}),
)
```

and change `ServeHTTP` to `s.handler.ServeHTTP(w, r)`. `routePattern` maps known
API paths to stable templates; unknown paths return `"http.request"` so
cardinality stays bounded. Add a test in `pkg/gui/server_test.go` asserting an
in-memory recorder sees an `http.server` span whose name is the route pattern,
not the raw id.

- [ ] **Step 8: Instrument outbound clients**

In each constructor that builds an `*http.Client`, route it through
`telemetry.HTTPTransport`. For example in `pkg/harness/http_provider.go`:

```go
client: &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: ...},
```

For `pkg/harness/gemini_provider.go`, pass
`HTTPClient: &http.Client{Transport: telemetry.HTTPTransport(nil)}` to
`genai.ClientConfig`. Tests that inject `server.Client()` continue to work
because they pass their own client.

- [ ] **Step 9: Verify and commit**

Run: `go vet ./... && go test -count=1 ./pkg/storage/ ./pkg/gui/ ./pkg/harness/ ./pkg/telemetry/`
Expected: PASS.

```bash
git add pkg/storage pkg/gui pkg/harness pkg/media pkg/telemetry go.mod go.sum
git commit -m "feat(telemetry): instrument sqlite queries and http traffic"
```

---

### Task 4: Metrics catalogue

**Files:**
- Create: `pkg/telemetry/metrics.go`
- Create: `pkg/telemetry/metrics_test.go`
- Modify: `pkg/engine/orchestrator.go`, `pkg/engine/timeline.go`
- Modify: `pkg/harness/context.go`
- Modify: `pkg/media/tts.go`, `pkg/media/image.go`

**Interfaces:**
- Consumes: `telemetry.Meter`, `telemetry.Recorder.Metrics(ctx)`.
- Produces: metric instruments named exactly as in the spec §4.

- [ ] **Step 1: Write the failing metric test**

Create `pkg/telemetry/metrics_test.go` asserting one recorded turn increments
`localrpg.turn.completed` and records `localrpg.turn.duration`, then run it and
watch it fail. Use `Recorder.Metrics(ctx)` and search `metricdata.ResourceMetrics`
for the instrument names.

- [ ] **Step 2: Add instrument helpers**

Create `pkg/telemetry/metrics.go` with constructors that read the global meter:

```go
// Int64Counter and friends are thin wrappers so callers do not import the OTel
// metric package directly and so a no-op global yields no-op instruments.
func Int64Counter(name, unit, description string) (otelmetric.Int64Counter, error) {
	return otel.Meter("github.com/darkliquid/localrpg").Int64Counter(name,
		otelmetric.WithUnit(unit), otelmetric.WithDescription(description))
}
// Float64Histogram, Int64Histogram, Float64Counter similarly.
```

- [ ] **Step 3: Record metrics beside spans**

At each span site from Task 2, record the corresponding metric from §4:
`turn.duration`/`turn.completed` in `ProcessAction`, `context.tokens` per section in
`harness/context.go`, `provider.request.duration` plus `provider.tokens` and
`provider.errors` around each round, `tool.call.duration` and `tool.rounds` in the
loop, and `media.tts.duration`/`cache` in the TTS pipeline. Attributes are the
bounded sets from §4; `game.id` is never used.

- [ ] **Step 4: Run the tests**

Run: `go test -count=1 ./pkg/telemetry/ ./pkg/engine/ ./pkg/harness/ ./pkg/media/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/telemetry pkg/engine pkg/harness pkg/media
git commit -m "feat(telemetry): add the metrics catalogue"
```

---

### Task 5: Structured logs and the pkg/trace bridge

**Files:**
- Create: `pkg/telemetry/bridge.go`
- Create: `pkg/telemetry/bridge_test.go`
- Modify: `pkg/trace/trace.go` (add optional `ContextLogger` interface)
- Modify: `pkg/engine/orchestrator.go`, `pkg/harness/*_provider.go` (use `EventCtx` where ctx is held)
- Modify: `cmd/localrpg/gui.go`, `cmd/localrpg/play.go` (pass `provider.Logger(localSink)` into the seams)

**Interfaces:**
- Consumes: `trace.Logger`, `trace.ContextLogger`.
- Produces: `(*telemetry.Provider).Logger(local trace.Logger) trace.Logger`, `trace.ContextLogger`.

- [ ] **Step 1: Define the optional context-aware interface**

In `pkg/trace/trace.go`:

```go
// ContextLogger is an optional extension of Logger for callers that hold a
// context: an event can then be attached to the active span. Event remains the
// fallback, so packages with no context keep working unchanged.
type ContextLogger interface {
	Logger
	EventCtx(ctx context.Context, name string, fields map[string]interface{})
}
```

- [ ] **Step 2: Write the failing bridge test**

```go
func TestBridgeForwardsToLocalAndOTel(t *testing.T) {
	recorder, provider, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	local := trace.NewMemory(trace.LevelFull)
	bridge := provider.Logger(local)
	bridge.Event("tool.call", map[string]interface{}{"name": "search_entities"})

	if _, ok := local.Find("tool.call"); !ok {
		t.Fatal("local sink must still receive the event")
	}
	if len(recorder.Logs()) == 0 {
		t.Fatal("expected an OTel log record")
	}
}
```

Run: `go test -run TestBridge ./pkg/telemetry/ -v` and watch it fail.

- [ ] **Step 3: Implement the bridge**

Create `pkg/telemetry/bridge.go`: a struct holding the local `trace.Logger`, a
`log.Logger`, and a `trace.Tracer`. `Event` forwards to local, then emits a log
record whose body is the event name and whose attributes are the sanitized
fields. `EventCtx` does the same plus `oteltrace.SpanFromContext(ctx).AddEvent(name, ...)`.
`SetGame` forwards to local and stamps the logger; `Enabled` forwards to local.

- [ ] **Step 4: Use `EventCtx` where a context exists**

In `pkg/engine/orchestrator.go` and the providers, replace `o.logger.Event(...)`
with a helper that prefers `EventCtx`:

```go
func logEvent(ctx context.Context, logger trace.Logger, name string, fields map[string]interface{}) {
	if contextual, ok := logger.(trace.ContextLogger); ok {
		contextual.EventCtx(ctx, name, fields)
		return
	}
	logger.Event(name, fields)
}
```

Place it in `pkg/trace` so every package shares one implementation.

- [ ] **Step 5: Pass the bridge at startup**

In `cmd/localrpg/gui.go` and `play.go`, after building the local `FileSink`,
replace `logger` with `provider.Logger(sink)` wherever `SetLogger` is called
(orchestrator, router, extractor, summariser, TTS pipeline, GUI service). The GUI
Debug panel keeps reading the same JSONL file, so it is unaffected.

- [ ] **Step 6: Run the full suite**

Run: `go vet ./... && go test -count=1 ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/telemetry pkg/trace pkg/engine pkg/harness cmd/localrpg
git commit -m "feat(telemetry): bridge the trace logger into OTel logs and spans"
```

---

## File Map

| File | Responsibility |
|------|----------------|
| `pkg/telemetry/telemetry.go` | Provider lifecycle, exporters, resource, sampler, globals |
| `pkg/telemetry/inmemory.go` | In-memory test providers and `Recorder` |
| `pkg/telemetry/http.go` | `HTTPTransport` outbound wrapper |
| `pkg/telemetry/metrics.go` | Metric instrument constructors |
| `pkg/telemetry/bridge.go` | `trace.Logger` → OTel logs/spans adapter |
| `pkg/config/types.go` | `TelemetryConfig` and defaults |
| `pkg/storage/otel_driver.go` | Wrapped `database/sql` driver registration |
| `pkg/gui/server.go` | Inbound `otelhttp` handler and route span names |
| `pkg/engine/orchestrator.go` | Turn/round/tool/continuity spans and metrics |
| `pkg/harness/context.go` | Context-assembly span and per-section metrics |
| `pkg/engine/timeline.go` | Timeline-write span |
| `pkg/trace/trace.go` | `ContextLogger` optional interface |
| `cmd/localrpg/gui.go`, `cmd/localrpg/play.go` | Provider construction, shutdown, bridge wiring |

## Self-Review

- **Spec coverage:** §2 package/config/lifecycle → Task 1; §3 spans → Task 2; §6/§7 DB+HTTP → Task 3; §4 metrics → Task 4; §5 logs/bridge → Task 5; §1.1 goals and §3.2 attributes are exercised by the Task 2/4 tests; §8 testing appears in every task. No section is unmapped.
- **Placeholder scan:** the only deliberately deferred bodies are the gRPC option helpers in Task 1 Step 5 and `NewInMemory` in Step 6; both name their exact OTel calls so an implementer is not guessing. No TBD/TODO/"handle edge cases" remain.
- **Type consistency:** `telemetry.Tracer`/`Meter`/`HTTPTransport`/`Logger` and `Recorder.Spans`/`Metrics`/`Logs` are defined in Task 1/3 and used unchanged in Tasks 2, 4, and 5. `trace.ContextLogger` is defined in Task 5 before use.
