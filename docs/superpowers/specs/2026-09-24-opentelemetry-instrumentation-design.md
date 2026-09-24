# OpenTelemetry Instrumentation Specification

- **Date:** 2026-09-24
- **Status:** Approved (design); spec pending review
- **Scope:** First-class tracing, metrics, and logs for LocalRPG using the
  OpenTelemetry Go SDK, exported over OTLP/gRPC. Covers turn/provider/tool/media
  spans, a metrics catalogue, structured logging, automatic SQLite and HTTP
  instrumentation, configuration, lifecycle, and the bridge to the existing
  `pkg/trace` JSONL logger.
- **Related:** `pkg/trace`, `pkg/engine/orchestrator.go`,
  `pkg/harness`, `pkg/media`, `pkg/storage`, `pkg/gui/server.go`,
  `pkg/config`.

---

## 1. Overview & Goals

LocalRPG is local-first: a single Go binary serving a Wails GUI, a Bubbletea
TUI, and an HTTP/Unix-socket API. It already records a bespoke JSONL trace
(`pkg/trace`), but that trace is a flat event stream with no correlation, no
metrics, and no standard tooling. Diagnosing a bad turn means grepping a file
and reconstructing ordering by timestamp.

This specification adds OpenTelemetry so the whole application can be fully
instrumented and analysed: a turn becomes one trace with nested spans for
context assembly, each provider round, every tool call, extraction, storage
writes, and media synthesis; the SQLite index and every outbound HTTP call are
instrumented automatically; and metrics expose throughput, latency, token
usage, cache hit rates, and error rates.

### 1.1 Goals

1. **Traces**: one root span per user-visible operation (a turn, an HTTP API
   call, a media synthesis) with correlated child spans across packages.
2. **Metrics**: histograms and counters for latency, volume, tokens, cache
   hit/miss, and errors, using bounded-cardinality attributes.
3. **Logs**: structured logs through `log/slog`, exported over OTLP and to
   stderr, carrying the active trace/span ids for correlation.
4. **SQLite visibility**: every query issued through `database/sql` becomes a
   span with a sanitised statement, duration, and error.
5. **HTTP visibility**: inbound GUI/API requests and outbound provider/media
   calls are instrumented with standard semantic conventions.
6. **Local-first**: telemetry is **off by default**; enabling it configures an
   OTLP/gRPC exporter (default `localhost:4317`). Normal play makes no network
   calls to a collector and pays no measurable overhead.
7. **Compatibility**: the existing `pkg/trace` JSONL sink and the GUI Debug
   panel keep working; trace events are bridged into OTel so both can be used
   together.

### 1.2 Non-Goals

1. Bundling any vendor-specific exporter or dashboard. Only OTLP/gRPC is
   compiled in; a collector fans out to Jaeger/Tempo/Prometheus/etc.
2. Always-on telemetry. There is no default collector and no default network
   egress.
3. Capturing data beyond the existing redaction policy. `trace.Sanitize`
   remains the single place that decides what payload text may leave the
   process.
4. Replacing `pkg/trace`. The JSONL file remains the offline, payload-rich
   record; OTel is the correlatable, standard layer on top.
5. Frontend (webview) instrumentation. This spec covers the Go process only.

### 1.3 Success Criteria

- With telemetry enabled, one turn produces a single trace whose root span
  contains child spans for context assembly, one provider round per tool loop
  iteration, each tool execution, extraction, the timeline write, and every
  SQL statement, sharing one `trace_id`.
- With telemetry disabled, no OTLP connection is attempted, no goroutine leaks,
  and the full test suite passes with the in-memory providers only.
- `go test ./...` never needs a network or a collector.

---

## 2. Architecture

### 2.1 New package: `pkg/telemetry`

A single package owns provider construction and teardown so every entry point
(GUI, TUI, daemon) shares one implementation and every other package depends
only on `go.opentelemetry.io/otel` interfaces, never on exporters.

```go
package telemetry

// Provider owns the OTel providers and their exporters. Construct one with New;
// a provider built while telemetry is disabled returns no-op tracers, meters,
// and loggers, so no caller branches on whether telemetry is on. The OTel
// packages are aliased because pkg/trace already owns the name "trace".
type Provider struct {
    tracer   *sdktrace.TracerProvider
    meter    *sdkmetric.MeterProvider
    logger   *sdklog.LoggerProvider
    cfg      config.TelemetryConfig
}

// New builds the providers from configuration. When telemetry is disabled it
// returns a provider whose Tracer/Meter return no-ops and no error, so startup
// never depends on a collector.
func New(ctx context.Context, cfg config.TelemetryConfig, build BuildInfo) (*Provider, error)

// Logger returns the OTel-aware bridge that also fans out to the local JSONL
// sink, so existing SetLogger seams gain telemetry with no call-site change.
func (p *Provider) Logger(local trace.Logger) trace.Logger

// Shutdown flushes all providers with a bounded timeout and is safe to call
// when disabled. It is expected to be deferred from main/run.
func (p *Provider) Shutdown(ctx context.Context) error

// Tracer/Meter/Logger expose the OTel interfaces for packages that hold ctx.
func (p *Provider) Tracer(name string) oteltrace.Tracer
func (p *Provider) Meter(name string) otelmetric.Meter
func (p *Provider) Enabled() bool
```

`BuildInfo` carries `service.version` (from the existing version command) and,
optionally, the active config file path.

### 2.2 Configuration

`pkg/config` gains a top-level `telemetry` block, defaulted off:

```go
type TelemetryConfig struct {
    Enabled     bool              `yaml:"enabled" json:"enabled"`
    Endpoint    string            `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`   // default localhost:4317
    Protocol    string            `yaml:"protocol,omitempty" json:"protocol,omitempty"`   // "grpc" (default) | "http"
    Insecure    bool              `yaml:"insecure,omitempty" json:"insecure,omitempty"`   // default true for localhost
    SampleRatio float64           `yaml:"sample_ratio,omitempty" json:"sample_ratio,omitempty"` // default 1.0
    Traces      bool              `yaml:"traces" json:"traces"`
    Metrics     bool              `yaml:"metrics" json:"metrics"`
    Logs        bool              `yaml:"logs" json:"logs"`
    Headers     map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
    ServiceName string            `yaml:"service_name,omitempty" json:"service_name,omitempty"` // default "localrpg"
}
```

Resolution order follows the existing config manager. Environment overrides are
supported for the standard variables (`OTEL_EXPORTER_OTLP_ENDPOINT`,
`OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`,
`OTEL_TRACES_SAMPLER`) and take precedence over the file, so an operator can
turn on telemetry for one session without editing config.

`DefaultConfig` returns telemetry disabled with all three signals on and
`SampleRatio 1.0`, so enabling is a single switch.

### 2.3 Lifecycle

- `cmd/localrpg/gui.go` and `cmd/localrpg/play.go` construct the `Provider`
  after config load and before wiring providers, then
  `defer provider.Shutdown(shutdownCtx)` with a 5s bound.
- The bridge logger is passed to the existing `SetLogger` seams:
  `TurnOrchestrator`, `RouterFromConfigWithLogger`, `ExtractorFromConfig`,
  `SummariserFromConfig`, `TTSPipeline`, the GUI `Service`, and every provider
  constructor that already accepts a logger.
- Shutdown is best-effort: a flush failure is logged and never changes the
  process exit code or blocks exit beyond the bound.

### 2.4 Resource and sampler

- Resource attributes: `service.name` (default `localrpg`), `service.version`,
  `service.instance.id` (a process-lifetime UUID so concurrent instances are
  distinguishable), `os.type`, and `localrpg.config_file`. SDK/telemetry
  attributes are added automatically by the SDK.
- Sampler: `ParentBased(TraceIDRatioBased(ratio))`, defaulting to
  `ParentBased(AlwaysSample)` at ratio 1.0. Sampling is decided once at the
  root (`turn` or `http.server`), so a sampled turn is complete.

---

## 3. Trace Model

Spans are created from `ctx`, which already flows through generation, tools,
storage, and media. No new plumbing of context objects is required; the work is
placing `tracer.Start`/`defer span.End()` at the existing seams and adding
attributes.

### 3.1 Span hierarchy

```
turn                                  engine, root per turn
├── context.assemble                  harness.ContextAssembler
├── provider.generate                 harness, per tool-loop round
│   └── http.client (auto)            otelhttp transport, outbound LLM call
├── tool.call                         tools.Executor, per call
│   └── storage.query (auto)          otelsql
├── extract.entities                  harness.Extractor
├── summarise                         harness.Summariser (every N turns)
├── continuity.check                  engine continuity pass
├── timeline.record_turn              engine.Timeline
│   └── storage.query (auto)          otelsql
└── media.tts.synthesize              media.TTSPipeline
```

HTTP API requests start an independent root:

```
http.server                           gui, via otelhttp.NewHandler
└── game.turn (for POST /turn)        engine turn, mirrors the turn span
```

### 3.2 Span attributes

| Span | Attributes |
|------|------------|
| `turn` | `game.id`, `turn.number`, `turn.mode`, `turn.location`, `turn.outcome`, `turn.truncated` |
| `context.assemble` | `context.budget`, `context.tokens`, `context.sections_included`, `context.sections_trimmed` |
| `provider.generate` | `gen_ai.system`, `gen_ai.request.model`, `localrpg.role`, `localrpg.round`, `localrpg.tools_offered`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`, `localrpg.finish_reason` |
| `tool.call` | `localrpg.tool.name`, `localrpg.tool.ok`, `localrpg.tool.bytes` |
| `extract.entities` | `localrpg.extractor.provider`, `localrpg.entities.created`, `localrpg.entities.updated` |
| `timeline.record_turn` | `localrpg.turn.number`, `localrpg.entities.noted` |
| `media.tts.synthesize` | `localrpg.tts.provider`, `localrpg.tts.voice`, `localrpg.tts.bytes`, `localrpg.cache.hit` |
| `media.image.generate` | `localrpg.image.provider`, `localrpg.image.model` |

`gen_ai.*` follows the OpenTelemetry GenAI semantic conventions so LLM calls are
comparable across providers. Payload text (prompts, narration) is **never** a
span attribute; it stays in the JSONL trace.

### 3.3 Span events

- `provider.generate`: `first_chunk` (time to first token), `tool_calls`
  (count), `thinking` (when the provider emits thought parts), `retry`
  (fallback provider engaged), `error`.
- `context.assemble`: `section` events mirroring the existing `sections`
  trace field, one per included/trimmed section.
- `tool.call`: `error` with the message when the executor reports failure.

### 3.4 Error recording

A span is marked `Error` and `RecordError(err)` is called on every failure
path that already returns an error, using the existing typed errors
(`engine.ErrGenerationStalled`, provider errors, storage errors). HTTP status
`>= 500` from an instrumented client records an error; `otelhttp` handles the
server side.

---

## 4. Metrics Catalogue

All metrics live under the `localrpg.` namespace and are created in the package
that owns the measurement. Attribute sets are bounded: **`game.id` is never a
metric attribute** (it is unbounded); it appears on spans only.

| Metric | Kind | Unit | Attributes |
|--------|------|------|------------|
| `localrpg.turn.duration` | histogram | ms | `turn.mode`, `turn.outcome` |
| `localrpg.turn.completed` | counter | 1 | `turn.mode`, `turn.truncated` |
| `localrpg.context.tokens` | histogram | tokens | `context.section` |
| `localrpg.provider.request.duration` | histogram | ms | `gen_ai.system`, `localrpg.role`, `localrpg.round` |
| `localrpg.provider.tokens` | counter | tokens | `gen_ai.system`, `gen_ai.token.type` (input/output) |
| `localrpg.provider.errors` | counter | 1 | `gen_ai.system`, `localrpg.role`, `error.kind` |
| `localrpg.provider.fallbacks` | counter | 1 | `localrpg.role` |
| `localrpg.tool.call.duration` | histogram | ms | `localrpg.tool.name`, `localrpg.tool.ok` |
| `localrpg.tool.rounds` | histogram | 1 | `localrpg.role` |
| `localrpg.storage.query.duration` | histogram | ms | `db.operation` |
| `localrpg.storage.query.errors` | counter | 1 | `db.operation` |
| `localrpg.media.tts.duration` | histogram | ms | `localrpg.tts.provider`, `localrpg.cache.hit` |
| `localrpg.media.tts.bytes` | histogram | bytes | `localrpg.tts.provider` |
| `localrpg.media.tts.cache` | counter | 1 | `localrpg.cache.result` (hit/miss) |
| `localrpg.media.image.duration` | histogram | ms | `localrpg.image.provider` |
| `localrpg.extraction.entities` | counter | 1 | `localrpg.entity.action` (created/updated) |
| `localrpg.continuity.findings` | counter | 1 | `localrpg.continuity.rule` |
| `localrpg.http.server.duration` | histogram | ms | `http.request.method`, `http.route`, `http.response.status_code` (via `otelhttp`) |

Histogram bucket boundaries: latency in `{1,2,5,10,25,50,100,250,500,1000,2500,5000,10000}` ms, tokens in
`{128,512,1024,2048,4096,8192,16384,32768,65536}`, bytes in powers of four from
`256` to `4194304`.

Metrics are recorded from the same code paths that place spans, so a call site
does one `span.End()` and one `metric.Record()` rather than a second traversal.

---

## 5. Logs

### 5.1 Structured logging

- Introduce `log/slog` as the application logger. A `slog.Handler` chain is
  configured in `pkg/telemetry`: an OTel handler (`otelslog`) when logs are
  enabled and a stderr handler always, at the configured level.
- Log records carry `trace_id`/`span_id` automatically when a span is active,
  so a log line joins its trace in any backend.

### 5.2 Bridge from `pkg/trace`

The existing `trace.Logger` seam stays intact. `telemetry.Logger(local)` returns
an adapter that:

1. Forwards to the local sink (`*trace.FileSink`), preserving the JSONL trace
   and GUI Debug panel exactly as they are.
2. Emits an OTel **log record** with the event name as the body and the fields
   as attributes (after `trace.Sanitize`).
3. If the caller used a context-aware variant, also records a span **event** on
   the active span.

To let span-attached events work where `ctx` is available, the adapter exposes:

```go
// EventCtx is Event with the active span taken from ctx.
type ctxLogger interface {
    EventCtx(ctx context.Context, name string, fields map[string]interface{})
}
```

`trace.Logger` is unchanged; `EventCtx` is an optional interface that the
orchestrator and providers (which hold `ctx`) type-assert for. `Event` remains
the fallback, so packages with no `ctx` (e.g. some media helpers) keep working
without change.

### 5.3 Redaction

All payload text passes through `trace.Sanitize` before it becomes a log record
attribute or span event, using the same `Level`/`PayloadChars` bounds. There is
exactly one redaction implementation, shared by JSONL and OTel.

---

## 6. SQLite Instrumentation

`pkg/storage/db.go:OpenDB` is the single place a game database is opened. It
will register one wrapped driver:

```go
// registered once, guarded by sync.Once
otelsql.Register("sqlite_otel", otelsql.WithAttributes(
    semconv.DBSystemSqlite,
), otelsql.WithSpanOptions(otelsql.SpanOptions{
    OmitRows: true, // row-by-row spans are noise
}))
db, err := sql.Open("sqlite_otel", "file:"+path+"?"+pragmas)
```

Effects:

- Every `QueryContext`/`ExecContext`/`PrepareContext` opens a `storage.query`
  span (`db.operation`, sanitised `db.statement`) and records
  `localrpg.storage.query.duration`.
- No `storage` call site changes; `*sql.DB` remains the interface.
- Statements containing user text are truncated to the configured payload
  length and never include bound parameter values.

A fallback path is required if `otelsql` cannot wrap `modernc.org/sqlite`: a
thin `driver.Connector` wrapper in `pkg/storage` implementing the same
`QueryContext`/`ExecContext` hooks. Task 3 of the plan proves the wrapper with
a test against a temp database before the rest of the storage work proceeds.

The FTS and migration statements (`EnsureFTS`, `migrate`) run through the same
wrapped handle, so schema work is visible too.

---

## 7. HTTP Instrumentation

### 7.1 Inbound

`pkg/gui/server.go:ServeHTTP` is a bare mux delegation. It becomes:

```go
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    s.handler.ServeHTTP(w, r)
}
```

where `s.handler = otelhttp.NewHandler(s.mux, "localrpg.http", ...)` with a
span-name formatter that uses the matched route (`/api/game/{id}/turn`) rather
than the raw path, keeping cardinality bounded. `http.server.duration` comes
from the same instrumentation.

### 7.2 Outbound

Every provider client that builds an `*http.Client` (OpenAI-compatible HTTP
providers, ElevenLabs, Gemini REST calls, image endpoints) uses a shared
transport:

```go
// pkg/telemetry
func (p *Provider) HTTPTransport(base http.RoundTripper) http.RoundTripper
```

which returns `otelhttp.NewTransport(base)` when enabled and `base` otherwise.
Constructors accept the transport via a functional option or a package-level
default set at startup, so tests continue to inject `httptest` servers unchanged
(the transport wraps whatever client they pass).

### 7.3 Wails / Unix socket

The Wails webview calls Go methods directly rather than over HTTP. Those
boundary methods (`gui.Service.*`) get a span from the `Service` tracer so the
native window is observable too. The Unix-socket server uses the same
`Server.ServeHTTP`, so it is instrumented identically to TCP.

---

## 8. Testing Strategy

1. **No network.** All tests use in-memory providers constructed by
   `telemetry.NewForTest()`: `sdk/trace/tracetest.InMemoryExporter`, an
   `sdk/metric` manual reader, and a recording `sdk/log` processor. A test that
   accidentally enables the OTLP exporter must be caught by asserting the
   endpoint is empty.
2. **Span assertions.** Extend the existing orchestrator tests to assert the
   span tree: a turn with one tool round yields `turn → provider.generate →
   tool.call → provider.generate` with a shared trace id.
3. **Metric assertions.** Use a manual reader and assert exact counts and
   attribute sets for one recorded turn.
4. **Log bridge.** Assert that a `trace.Memory` event and an OTel log record are
   both produced from one `Event` call, and that `EventCtx` attaches a span
   event.
5. **DB wrapper.** Prove `otelsql` wraps `modernc.org/sqlite` against a
   `t.TempDir()` database and that a failing statement records an error span.
6. **Disabled path.** Assert that a disabled `Provider` starts no goroutines
   (goroutine-count check) and that `Shutdown` returns immediately.
7. **TUI/CLI.** `localrpg play` constructs no telemetry provider by default;
   with `--telemetry` or config it shares the same `pkg/telemetry` path.

---

## 9. Dependencies

New modules, all compiled into the binary, none active unless enabled:

- `go.opentelemetry.io/otel`
- `go.opentelemetry.io/otel/sdk` (trace, metric, log, resource)
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc`
- `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc`
- `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`
- `go.opentelemetry.io/contrib/bridges/otelslog`
- `github.com/XSAM/otelsql`

Constraints: pin the latest stable of each (the OTel Go logs SDK is the newest
surface and is isolated behind `pkg/telemetry`); keep `go vet` clean; do not
add any exporter other than OTLP/gRPC.

---

## 10. Rollout

One plan, five tasks, each independently testable:

1. **Foundation**: `pkg/telemetry`, config block, `cmd` lifecycle wiring,
   disabled-path test.
2. **Spans**: turn, context, provider, tool, extraction, timeline, media spans
   placed at existing seams.
3. **DB + HTTP**: `otelsql` wrapper and `otelhttp` on server and outbound
   clients.
4. **Metrics**: the catalogue from §4, recorded alongside spans.
5. **Logs + bridge**: `slog` handler, `trace.Logger` bridge, `EventCtx`,
   GUI Debug panel unaffected.

---

## 11. Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| OTel Go logs SDK is relatively new | Pin stable; isolate in `pkg/telemetry`; logs toggled independently of traces/metrics. |
| `otelsql` incompatible with `modernc.org/sqlite` | Prove with a test in task 3 before any storage work; hand-rolled connector fallback documented in §6. |
| Metric cardinality blow-up | Bounded attributes (§4); `game.id` spans-only; route-based span names (§7.1). |
| Overhead when enabled | Sampling at the root; payloads excluded from spans/metrics; `OmitRows`. |
| Telemetry accidentally on | Default off; disabled-path goroutine test; no default endpoint. |
| Duplicate telemetry with `pkg/trace` | Explicit bridge ownership: `telemetry.Logger(local)` is the only adapter. |

---

## 12. Testing & Verification Summary

1. Unit tests for `pkg/telemetry` construction, sampling, and shutdown.
2. Orchestrator span-tree tests using the in-memory exporter.
3. Metric catalogue tests using a manual reader.
4. Log bridge tests asserting both sinks receive one event.
5. `storage` tests proving query spans and error spans via the wrapped driver.
6. HTTP tests proving inbound route span names and outbound client spans.
7. Disabled-path test proving no goroutines and no network.
8. `go vet ./...` and `go test ./...` remain the gate.
