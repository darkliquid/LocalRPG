# LocalRPG Debugging & Telemetry Guide

LocalRPG includes an embedded debugging suite designed to diagnose why turns fail, why an AI Game Master returns no narration, and how model prompts and provider attempts behave end-to-end.

The suite combines:
1. **Interactive Dual-Server Mode (`localrpg debug server`)**: Runs the LocalRPG web GUI side-by-side with an embedded OpenTelemetry debugging dashboard.
2. **Automated Application Driver (`localrpg debug test-run`)**: Drives the UI headlessly using the Chrome DevTools Protocol (CDP via `chromedp`) to run declarative YAML test scenarios.
3. **In-Process OpenTelemetry Collector**: Bounded in-memory ring buffers that buffer spans, traces, and structured logs without requiring external Docker services or Jaeger collectors.
4. **Action-Telemetry Correlator**: Marries user clicks and turn submissions to their exact backend spans, assembled prompt layers, raw model completions, and failure codes.
5. **Standalone HTML Reports**: Self-contained report artifacts with trace waterfalls, token counts, and failure screenshots.

---

## 1. Quick Start: Debugging a Silent or Failing GM

When you take an action in LocalRPG and the GM returns no narration, it typically means one of three things:
- The model returned an empty string or only whitespaces (`empty_response`).
- The model produced tool calls or internal thoughts without finalizing prose narration.
- A provider error, timeout, or context prompt overflow caused the generation to abort.

### Step 1: Start the Debug Server

Run:
```bash
./bin/localrpg debug server --port 8080 --debugger-port 8089
```

This launches:
- **LocalRPG Application**: `http://localhost:8080` (or `http://localhost:3000` when running the Vite frontend dev server).
- **Debugger Dashboard**: `http://localhost:8089`.

### Step 2: Open Both Interfaces

1. Open `http://localhost:8080` in one browser tab (or connect the desktop client).
2. Open `http://localhost:8089` in another tab to view the live debugger dashboard.

### Step 3: Trigger the Problematic Turn

In the LocalRPG app, play the campaign turn that is failing or producing blank narration.

### Step 4: Inspect in the Debugger Dashboard

In `http://localhost:8089`, the turn automatically appears in the left sidebar (e.g. `[OK] turn #1 (1240ms)` or `[FAIL] turn #1 (850ms)`).

Click on the turn to view:
- **Prompt & LLM Tab**:
  - **Assembled Prompt**: The exact layered prompt (system rules, world lore, entity wikilinks, active scene scope, conversation memory) sent to the LLM.
  - **Raw Completion**: Exactly what the model returned. If it is empty, you will immediately see `""`. If the model returned thoughts, unexpected JSON, or partial text, you can inspect it verbatim.
  - **Failure Code**: Shows classified error codes like `empty_response`, `provider_timeout`, `rate_limit`, or `auth_failure`.
- **Spans Waterfall Tab**:
  - Shows the complete span tree: `turn` &rarr; `context.assemble` &rarr; `provider.generate` &rarr; `tool.call` &rarr; `timeline.record_turn`.
  - Highlights latency bottlenecks and marks which specific span encountered an error.
- **Raw JSON Tab**:
  - Full OpenTelemetry attributes, duration in milliseconds, token counts, and parent/child span IDs.

---

## 2. Automated Test Runner (`localrpg debug test-run`)

You can run automated browser test scenarios to reproduce bugs or verify UI workflows without manual clicking.

```bash
# Run a declarative test scenario headlessly
./bin/localrpg debug test-run --scenario scenarios/smoke-test.yaml

# Run with a visible browser window
./bin/localrpg debug test-run --scenario scenarios/turn-failure-recovery.yaml --headless=false

# Specify custom ports and output directory for HTML reports
./bin/localrpg debug test-run \
  --scenario scenarios/turn-failure-recovery.yaml \
  --port 8080 \
  --debugger-port 8089 \
  --report-dir test-results
```

### CLI Flags

| Flag | Default | Description |
|---|---|---|
| `--scenario` | *(required)* | Path to the declarative YAML scenario file. |
| `--headless` | `true` | When `true`, runs Chrome without a visible window. Set to `false` to watch browser actions live. |
| `--port` | `8080` | LocalRPG application port. |
| `--debugger-port` | `8089` | Port for the live debugger web dashboard. |
| `--report-dir` | `test-results` | Output directory where `report-<timestamp>.html` is saved upon completion. |

---

## 3. Writing Declarative Scenarios

Test scenarios are defined in human-readable YAML files. Scenarios specify environment setup and an ordered sequence of UI actions.

### Example Scenario (`scenarios/turn-failure-recovery.yaml`)

```yaml
name: "Turn Failure Recovery"
description: "Verify UI displays error banner and recovers when provider returns an empty response"
setup:
  world: "valeria"
  system: "dnd5e"
  mock_provider:
    role: "gm"
    behavior: "empty_response_once"
steps:
  - action: "navigate"
    url: "/"

  - action: "wait_visible"
    selector: "[data-testid='campaign-card-valeria']"
    timeout_ms: 5000

  - action: "click"
    selector: "[data-testid='campaign-card-valeria']"

  - action: "type"
    selector: "[data-testid='action-console-input']"
    text: "I search the chest for traps."

  - action: "click"
    selector: "[data-testid='action-console-submit']"

  - action: "assert_visible"
    selector: "[data-testid='turn-error-banner']"
    text_contains: "empty response"
```

### Supported Step Actions

| Action | Required Fields | Optional Fields | Description |
|---|---|---|---|
| `navigate` | `url` | - | Navigates the browser to the specified path or URL. |
| `click` | `selector` | `timeout_ms` | Waits for element visibility and performs a mouse click. |
| `type` | `selector`, `text` | `timeout_ms` | Waits for input visibility and sends keyboard strokes. |
| `wait_visible` | `selector` | `timeout_ms` | Waits until element matching CSS selector appears in the DOM. |
| `assert_visible`| `selector` | `text_contains`, `timeout_ms` | Asserts element is visible and optionally checks text content. |
| `assert_turn_outcome` | `expected` | - | Asserts that turn resulted in `success`, `error`, or `roll`. |
| `sleep` | - | `timeout_ms` | Pauses scenario execution for the given duration. |
| `fault_injection` | `fault` | - | Triggers mock provider behavior (e.g. `empty_response_once`). |

---

## 4. How Action-Telemetry Correlation Works

LocalRPG injects an `X-LocalRPG-Action-ID` header into every network request triggered by a driver step or manual session:

```
[Browser Action / User Click] 
           │ (Step ID: act-uuid-1234)
           ▼
[HTTP Request with X-LocalRPG-Action-ID]
           │
           ▼
[LocalRPG API Middleware] (pkg/gui/middleware.go)
           │
           ├── Sets attribute `localrpg.action.id = act-uuid-1234` on active span
           └── Propagates Action ID into span context
           │
           ▼
[Turn Orchestrator] (pkg/engine/orchestrator.go)
           │
           ├── `turn` span (`turn.number`, `turn.assembled_prompt`, `turn.raw_completion`)
           ├── `provider.generate` span (`provider.id`, round, prompt, raw text)
           └── `tool.call` span (tool name, arguments, result)
           │
           ▼
[In-Process Collector] (pkg/debugger/collector.go)
           │
           └── Stores spans in bounded ring buffer, indexed by Action ID & Trace ID
           │
           ▼
[Live Dashboard & Report Generator] (pkg/debugger/server.go)
           └── Marries Action ID <-> Trace ID, rendering waterfall & prompt inspection
```

---

## 5. Debugger Dashboard Features

The dashboard on `http://localhost:8089` exposes:

### 1. Action Timeline (Left Pane)
- Lists each turn or scenario action chronologically.
- Shows status badges: `[PASSED]` in green or `[FAILED]` in red.
- Displays execution duration in milliseconds and target selector.

### 2. Spans Waterfall (Right Pane - Tab 1)
- Interactive breakdown of all OpenTelemetry spans associated with the turn.
- Shows parent/child relationships, start times, durations, and HTTP status codes.

### 3. Prompt & LLM Inspector (Right Pane - Tab 2)
- Displays the complete text of the prompt assembled by `harness.ContextAssembler.Assemble(ContextRequest)`.
- Displays the exact raw text returned by the LLM provider.
- Displays error status and failure code if generation failed.

### 4. Raw JSON (Right Pane - Tab 3)
- Complete structured diagnostic payload, including provider attempts and token metrics.

---

## 6. Standalone HTML Reports

When running automated scenarios via `localrpg debug test-run`, LocalRPG automatically compiles a standalone report in the `--report-dir` (default: `test-results/`):

- **Self-contained HTML**: Contains embedded CSS, action logs, step durations, and pass/fail statuses.
- **Artifact Files**:
  - `test-results/report-<timestamp>.html`: Visual report readable in any web browser without a web server.
- **CI / Headless Integration**: Exit code `0` indicates all scenario steps passed; non-zero indicates failure.

---

## 7. Troubleshooting Tips

### Chrome / Chromium Not Found
If you encounter `Chrome/Chromium executable not found in PATH`:
- Ensure `google-chrome`, `chromium`, or `chromium-browser` is installed and reachable in your `$PATH`.
- On Debian/Ubuntu: `sudo apt-get install chromium-browser`
- On Arch Linux: `sudo pacman -S chromium`
- On Fedora: `sudo dnf install chromium`

### No Telemetry Appearing in Dashboard
- Ensure you started the app using `./bin/localrpg debug server`. If you started via standard `./bin/localrpg gui`, telemetry is routed to the configured sink rather than the in-process debugger collector.
- Check that your browser is connecting to `http://localhost:8080` (or `http://localhost:3000` with the Vite dev proxy).
