# LocalRPG Debugging & Telemetry Guide

LocalRPG exports OpenTelemetry over OTLP/gRPC and can write a local trace file.
There is no built-in dashboard any more: point the app at an external collector
and inspect the spans there.

---

## 1. Export OTLP to an external viewer

The exporter targets `OTEL_EXPORTER_OTLP_ENDPOINT` when it is set, otherwise
the endpoint in the telemetry config, otherwise `localhost:4317`.

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
```

The same settings live in `config.yaml`:

```yaml
telemetry:
  enabled: true
  endpoint: localhost:4317
  protocol: grpc
  insecure: true
  sample_ratio: 1.0
  traces: true
  metrics: true
  logs: true
  service_name: localrpg
```

Run a viewer, for example:

```bash
# Desktop app for OTLP traces
otel-desktop-viewer

# Terminal viewer
otel-tui

# Or Jaeger all-in-one
docker run --rm -p 16686:16686 -p 4317:4317 jaegertracing/all-in-one:latest
```

Then start LocalRPG and open a campaign. Spans for turns, model calls, media,
and storage appear in the viewer. Follow a turn's trace id to see one action
end to end.

Note the gRPC port is `4317`; `4318` is the HTTP/protobuf port and is not used
by the default exporter.

---

## 2. File-based tracing

If you would rather not run a collector, the app can write a sanitized JSONL
trace to `<cache>/trace/trace.jsonl` and mirror it to stderr:

```bash
localrpg --trace          # full
localrpg --trace=summary  # summary
localrpg --trace=off      # disabled
```

Levels are `off`, `summary`, and `full`. The configured default is
`preferences.trace_level` in `config.yaml`. Secrets (API keys, tokens) are
redacted before they are written.

---

## 3. What was removed

The previous debugging surfaces are gone:

- the chromedp scenario runner (`localrpg debug test-run`) and the YAML
  scenarios under `scenarios/`;
- the in-app debug dashboard and its server (`localrpg debug server`).

Automated UI verification is now done with shirei's headless snapshots
(`mise run desktop:snapshots`) and Go tests.

---

## 4. Troubleshooting

- **No spans appear.** Confirm the collector listens on the gRPC port and that
  `OTEL_EXPORTER_OTLP_ENDPOINT` matches it. Set `insecure: true` for a local
  collector without TLS.
- **Connection refused on startup.** A missing collector is not fatal; the
  exporter retries. Check the app's stderr for the exporter warning.
- **Traces work but logs or metrics do not.** The `traces`, `metrics`, and
  `logs` toggles are independent; enable the ones you need.
