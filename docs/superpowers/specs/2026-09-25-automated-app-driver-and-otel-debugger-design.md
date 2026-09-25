# Automated App Driver & In-Debugger OTel Collector Design

- **Date:** 2026-09-25
- **Status:** Approved
- **Scope:** Testing harness, automated browser driver, embedded OpenTelemetry collector, failure diagnostic dashboard (`pkg/driver`, `pkg/debugger`, `cmd/localrpg`)
- **Related:** `docs/superpowers/specs/2026-09-24-opentelemetry-instrumentation-design.md`, `docs/superpowers/specs/2026-09-25-generation-failure-diagnostics-design.md`, `pkg/telemetry`, `pkg/gui/server.go`

---

## 1. Overview & Goals

Diagnosing why turns or other AI generations fail (e.g. empty model completions, role fallback exhaustion, context prompt overflows, tool loop stalls, or UI deserialization issues) currently requires manual clicking and grepping through unstructured terminal logs. While LocalRPG has OpenTelemetry span models and failure diagnostics, developers lack an automated way to reproduce scenarios and correlate user actions with the underlying telemetry.

This specification introduces:
1. An **Automated Application Driver** using Chrome DevTools Protocol (CDP via `chromedp`) to run declarative UI and turn scenarios headlessly or with a live browser.
2. An **Embedded In-Process OpenTelemetry Collector** that receives traces, spans, metrics, and structured logs without requiring external collector infrastructure like Docker/Jaeger.
3. An **Action-Telemetry Correlator & Live Debugger Dashboard** that marries each user/driver action to its exact backend spans, prompts, provider attempt chains, logs, and screenshots.

### 1.1 Goals

1. **Automated Application Driver**:
   - Drive the application UI using CDP via `github.com/chromedp/chromedp` (zero Node.js/Puppeteer dependencies; single Go binary).
   - Execute declarative YAML test scenarios (`navigate`, `click`, `type`, `wait_visible`, `assert_visible`, `assert_turn_outcome`, `fault_injection`).
   - Propagate correlation headers (`X-LocalRPG-Action-ID`) and baggage into all browser interactions.
2. **In-Debugger OpenTelemetry Collector**:
   - Embed an in-process OTLP/trace receiver using `pkg/telemetry.Recorder`.
   - Record root turn spans, provider attempts, tool calls, SQL statements, and logs in a bounded in-memory circular ring buffer.
3. **Action-Telemetry Correlation**:
   - Map every UI action to its corresponding backend trace and child spans.
   - Automatically extract generation failure diagnostics (assembled prompts, role fallback chains, raw model text, failure codes).
4. **Interactive Live Debugger Dashboard**:
   - Serve a lightweight web dashboard on `--debugger-port` showing real-time action progression, trace waterfall graphs, prompt inspectors, and DOM snapshots.
5. **Self-Contained Report Export**:
   - Export standalone `report.html` and OTLP JSON files for offline analysis and CI artifacts.

### 1.2 Non-Goals

1. Replacing standard Go unit tests. This harness is an end-to-end integration and failure diagnostic tool.
2. Requiring external Docker services or remote telemetry endpoints. The debugger collector runs entirely in-memory inside the LocalRPG process.

---

## 2. CLI Surface & Architecture

### 2.1 Command Line Interface (`cmd/localrpg/debug.go`)

```bash
# Run a specific declarative test scenario headlessly
localrpg debug test-run --scenario scenarios/turn-failure-recovery.yaml

# Run with visible Chrome window and live web debugger dashboard
localrpg debug test-run --scenario smoke-test --headless=false --debugger-port 8089

# Run as an interactive debugging daemon awaiting manual play
localrpg debug server --port 8080 --debugger-port 8089
```

### 2.2 Component Hierarchy

```
localrpg debug test-run
├── LocalRPG Backend Server (pkg/gui/server.go)
│   ├── OpenTelemetry Telemetry Provider (pkg/telemetry)
│   └── Inbound Action-ID Middleware (extracts X-LocalRPG-Action-ID)
├── Embedded OTel Collector (pkg/debugger/collector.go)
│   ├── In-Memory Spans & Logs Ring Buffer
│   └── Metrics Collector
├── Browser Driver (pkg/driver)
│   ├── Chromedp Runner (CDP client)
│   ├── Declarative YAML Scenario Parser
│   └── Header & Baggage Injector
├── Action-Telemetry Correlator (pkg/debugger/correlator.go)
│   ├── Correlates Action ID <-> Trace ID / Spans
│   └── Generation Failure Diagnostics Extractor
└── Debugger Web Dashboard (pkg/debugger/server.go on :8089)
    ├── Live WebSocket Stream
    ├── Action Timeline View
    ├── Trace Waterfall & Span Inspector
    ├── Prompt & Raw LLM Completion Inspector
    └── HTML Report Generator
```

---

## 3. Declarative Scenarios & Driver

### 3.1 YAML Scenario Schema (`scenarios/*.yaml`)

```yaml
name: "Turn Failure Recovery"
description: "Verify UI displays error banner and recovers when provider returns an empty response"
setup:
  world: "valeria"
  system: "dnd5e"
  mock_provider:
    role: "gm"
    behavior: "empty_response_once" # Injects fault on first turn, succeeds on retry
steps:
  - action: "navigate"
    url: "/"
  - action: "click"
    selector: "[data-testid='campaign-card-valeria']"
  - action: "wait_visible"
    selector: "[data-testid='action-console-input']"
    timeout_ms: 5000
  - action: "type"
    selector: "[data-testid='action-console-input']"
    text: "I search the chest for traps."
  - action: "click"
    selector: "[data-testid='action-console-submit']"
  - action: "assert_visible"
    selector: "[data-testid='turn-error-banner']"
    text_contains: "Model returned empty response"
  - action: "click"
    selector: "[data-testid='turn-retry-button']"
  - action: "assert_turn_outcome"
    expected: "success"
```

### 3.2 Action Correlation Header Injection

For each step executed by `pkg/driver`:
1. Generate unique `action_id := uuid.NewString()`.
2. Attach `X-LocalRPG-Action-ID: <action_id>` to outbound network requests via CDP network request interception.
3. Server middleware in `pkg/gui/middleware.go` reads `X-LocalRPG-Action-ID` and injects attribute `localrpg.action.id = <action_id>` into the active span context.

---

## 4. In-Process OTel Collector & Correlator

### 4.1 In-Process Collector (`pkg/debugger/collector.go`)

- Connects directly to `pkg/telemetry.Recorder`.
- Bounded ring buffers:
  - Max 1,000 completed spans.
  - Max 5,000 log records.
- Zero network socket overhead: telemetry passes in-memory.

### 4.2 Correlator & Failure Extractor (`pkg/debugger/correlator.go`)

Matches driver action steps to traces using `localrpg.action.id`:
```go
type ActionRecord struct {
    ID            string            `json:"id"`
    StepIndex     int               `json:"step_index"`
    ActionType    string            `json:"action_type"`
    Selector      string            `json:"selector"`
    InputData     string            `json:"input_data"`
    Timestamp     time.Time         `json:"timestamp"`
    DurationMs    int64             `json:"duration_ms"`
    ScreenshotB64 string            `json:"screenshot_b64,omitempty"`
    RootTraceID   string            `json:"root_trace_id,omitempty"`
    Status        string            `json:"status"` // "passed" | "failed"
    FailureReason string            `json:"failure_reason,omitempty"`
    Spans         []SpanSummary     `json:"spans"`
    Diagnostics   *TurnDiagnostics  `json:"diagnostics,omitempty"`
}

type TurnDiagnostics struct {
    AssembledPrompt string             `json:"assembled_prompt"`
    RawCompletion   string             `json:"raw_completion"`
    FailureCode     string             `json:"failure_code"`
    Attempts        []ProviderAttempt  `json:"attempts"`
}
```

When a step fails:
1. Correlator extracts the root `turn` span.
2. Reads `failure_code` (e.g. `empty_response`, `provider_timeout`, `parse_error`).
3. Formats prompt layers and attempt history for instant viewing.

---

## 5. Debugger Dashboard & Reports

### 5.1 Dashboard Interface (`:8089`)

- **Sequential Action Log (Left Pane)**:
  - Displays each scenario action with status icons (green check, red cross), selectors, duration, and trace badge.
- **Diagnostic Inspector (Right Pane)**:
  - **Trace Waterfall**: Interactive SVG/Canvas waterfall showing parent/child span hierarchy and latency bottlenecks.
  - **Prompt & LLM Inspector**: Complete assembled prompt, role attempts, raw model replies, and token usage.
  - **Correlated Logs**: Filtered `slog` stream matching the selected action's trace ID.
  - **DOM & Screenshot View**: Visual snapshot of browser state at failure point.

### 5.2 Standalone Report Bundling

When execution concludes:
- Compiles a self-contained `test-results/<timestamp>-<scenario>/report.html` embedding the action log, trace waterfall, and screenshots.
- Writes raw `traces.json` (OTLP standard format) and `actions.json`.

---

## 6. Verification & Testing

1. Unit tests for declarative YAML scenario parsing and action correlation matching.
2. Smoke test scenario verifying CDP browser launch, campaign navigation, turn execution, and clean shutdown.
3. Fault injection test verifying that an empty model reply triggers the failure diagnostic inspector with correct failure code and assembled prompt.
