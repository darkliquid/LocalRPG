---
id: 29-trace-and-telemetry
title: Trace & Telemetry
category: Configuration & Providers
order: 29
description: Record what a turn did in a local JSONL trace, or export spans and metrics to an OpenTelemetry collector.
---

# Trace & Telemetry

The **trace** is local, written to a file, and made to be read: one JSON object per
line, with prompt sizes, decisions, and timings. **Telemetry** is the standard
OpenTelemetry path, which sends spans, metrics, and logs to a collector you run.
Both are off by default, because neither is free and ordinary play should not pay
for them.

## Trace levels

`preferences.trace_level` selects how much the trace records.

| Level | Records |
| --- | --- |
| `off` | Nothing. This is the default. |
| `summary` | Decisions, sizes, timings, and errors, without prompts or completions. |
| `full` | Everything `summary` records, plus prompts, generated text, and raw provider lines. |

A `summary` trace answers what happened and how long it took. A `full` trace answers
what was sent and what came back. Start with `summary`: it is small enough to read,
and it leaves prompts out.

Set the level in one of three places:

```yaml
preferences:
  trace_level: summary
```

- The configuration file, which applies to every run.
- The settings **Debug** tab, which changes the same key and shows the file's
  bounds beside it.
- `--trace` on a command-line run, which overrides the configuration for that run.
  A bare `--trace` means `full`; `--trace=summary` names a level.

```bash
localrpg play my-campaign --trace summary
localrpg gui --trace full
```

A run started with `--trace` writes events to standard error as well as to the
file. A terminal session shows them as they happen.

## The trace file

The trace is appended to `trace.jsonl` in the cache directory's `trace/` folder,
under the platform's cache location. Each line is one event:

```json
{"ts":"2026-10-19T09:14:02.118Z","event":"provider.error","level":"summary","game":"my-campaign","role":"summariser","error":"context deadline exceeded"}
```

| Field | Meaning |
| --- | --- |
| `ts` | When the event was recorded, in UTC. |
| `event` | The event's name, such as `provider.error`. |
| `level` | The level the run was tracing at. |
| `game` | The campaign, when the run is playing one, so one appended file can still be filtered per campaign. |
| The rest | Whatever the call site recorded, under the field names that call site uses. |

A file that grows without bound is a trap. The trace rotates by size and keeps a
fixed number of files, which bounds what one long session can write. The bounds are
configurable:

| Key | Default | Meaning |
| --- | --- | --- |
| `agents.trace_max_bytes` | 268435456 (256 MiB) | Rotate when the file grows to this size. |
| `agents.trace_max_files` | 3 | How many rotated files to keep. |
| `agents.trace_rotate_check` | 200 | How many events pass between rotation checks. |
| `agents.trace_payload_chars` | 20000 | The longest string recorded, per field. |
| `agents.trace_chunk_limit` | 500 | The most chunk or wire events recorded per provider call. |

Because the format is JSONL, the trace reads with tools you already have:

```bash
tail -f ~/.cache/localrpg/trace/trace.jsonl
jq 'select(.event == "provider.error")' ~/.cache/localrpg/trace/trace.jsonl
```

## Redaction

The trace records payloads by design. One redaction policy is applied in one place,
and every field passes through it before it is written, which keeps a careless call
site from leaking a credential:

- **Secret field names are replaced at every level.** `api_key`, `apikey`,
  `authorization`, `token`, `secret`, and `password` become `[redacted]`, whatever
  their value.
- **Registered secret values are replaced wherever they appear.** A provider
  registers its key when it is constructed, so a key that appears inside a prompt, a
  URL, or an error message is redacted too.
- **Payload fields are dropped below `full`.** Prompts, narration, completions, and
  raw request and response bodies are not written at `summary` level.
- **A recorded string is capped** at `agents.trace_payload_chars` and marked when it
  was truncated, so one enormous prompt cannot fill the disk.

The redaction logic, and the test that exercises it, are security-relevant code.
Treat a change to `pkg/trace` as a change to a security boundary.

## Reading a trace in the app

The settings **Debug** tab reads the current trace: the level and bounds above, the
**Recorded Events** list, and the **Raw Provider Lines** for a selected call. Use it
to answer whether the narrator returned nothing or the provider failed.

For a turn-by-turn view with a span tree, start the debug server:

```bash
localrpg debug server --port 8080 --debugger-port 8089
```

The dashboard at `http://localhost:8089` lists each turn with its result and
duration. Opening one shows the assembled prompt, the raw completion, the failure
code when the turn failed, the span waterfall, and the raw attributes. Its collector
runs in the process, in place of a Docker service or an external collector.
`docs/debugging.md` in the repository covers the dashboard and the scripted
scenarios in full, and `localrpg debug test-run` writes the same information into a
standalone HTML report.

## OpenTelemetry export

Telemetry sends the same activity to a collector you run, as spans, metrics, and
logs. It is off until `telemetry.enabled` is true.

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
  headers:
    authorization: "env:COLLECTOR_TOKEN"
```

| Key | Default | Meaning |
| --- | --- | --- |
| `telemetry.enabled` | `false` | Export anything at all. |
| `telemetry.endpoint` | `localhost:4317` | The collector's OTLP endpoint. |
| `telemetry.protocol` | `grpc` | The transport. |
| `telemetry.insecure` | `true` | Skip TLS, which is what a local collector needs. |
| `telemetry.sample_ratio` | `1.0` | The fraction of traces sampled. |
| `telemetry.traces`, `metrics`, `logs` | each `true` | Which signals to export. |
| `telemetry.service_name` | `localrpg` | The resource's service name. |
| `telemetry.headers` | none | Extra headers, for a collector that authenticates. |

`OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_SERVICE_NAME` override the two matching
keys, and the standard environment variables work as they do elsewhere. The whole
section round-trips through the configuration like every other section, and the
built-in defaults already point at a local collector, so enabling telemetry is one
field.

A disabled or unreachable collector never fails a run. Startup returns a provider
whose signals are no-ops, and a turn that cannot export still plays.

### Exported spans and metrics

One turn produces a span tree rooted at `turn`:

```text
turn
├── turn.prepare
├── context.assemble
├── provider.generate        (one span per model round)
│   └── tool.call            (one span per tool the model asks for)
├── extract.entities
├── continuity.check
├── timeline.record_turn
└── timeline.finalise
```

The metrics report the same activity in aggregate, using `localrpg.*` identifiers:
turn duration, time to first token, provider request duration and errors, tool call
duration and rounds, context tokens per section, media durations and cache hits,
generation errors, and the fallback providers engaged. Point a Grafana, Jaeger, or
Tempo instance at the app, and the turn pipeline is observable without reading a
log.

### Trace and telemetry together

The trace and the telemetry complement each other. The JSONL trace is per run,
written to disk, and readable with `jq`. Telemetry is continuous, in memory, and
made for a collector that aggregates. With telemetry enabled the local sink is still
written, because the two hooks are the same one: every event goes to the file and to
the collector, with the same redaction applied on the way.
