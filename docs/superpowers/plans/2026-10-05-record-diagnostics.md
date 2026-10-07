# Record Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show, in the turn and the Debug panel, when the GM's control records were repaired or dropped.

**Architecture:** `engine.Turn` gains a persisted `RecordReport` built from the parser's RB-1 report; `TurnDTO` mirrors it; the frontend renders a quiet chip with a details popover and a Debug panel block. Nil for clean turns, so nothing changes for them.

**Tech Stack:** Go standard library; React 19 + Tailwind v4 on the frontend (no new dependencies).

**Spec:** `docs/superpowers/specs/2026-10-05-record-diagnostics-design.md`
**Depends on:** RB-1 (`docs/superpowers/plans/2026-10-05-record-repair-layer.md`) must be merged first.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- The report is nil for a clean turn; never emit an empty report.
- `Issues` is capped at 5; error strings are trimmed to 120 runes.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The report types and their builder

**Files:**
- Create: `pkg/engine/record_report.go`
- Test: `pkg/engine/record_report_test.go`

**Interfaces:**
- Consumes: `turnstream.Parser.RepairReport`, `turnstream.Parser.Records`, `turnstream.Record.Repaired`, `turnstream.Record.Err` (RB-1).
- Produces: `type RecordIssue struct { Type, Repair, Error string }`, `type RecordReport struct { Total, Repaired, Failed int; Issues []RecordIssue }`, `func (o *TurnOrchestrator) recordReport() *RecordReport`.

- [ ] **Step 1: Write the failing test**

```go
package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/turnstream"
)

func TestRecordReportNilWhenClean(t *testing.T) {
	o := &TurnOrchestrator{parser: turnstream.NewParser(newTestRoster())}
	o.parser.Feed("@roll {\"actor\":\"x\",\"check_kind\":\"do\"}\n")
	if got := o.recordReport(); got != nil {
		t.Fatalf("clean turn report = %+v, want nil", got)
	}
}

func TestRecordReportCountsAndCaps(t *testing.T) {
	o := &TurnOrchestrator{parser: turnstream.NewParser(newTestRoster())}
	o.parser.Feed("@roll {\"actor\":\"x\",\"check_kind\":\"do\",}\n") // repaired
	o.parser.Feed("@roll not json at all\n")                          // dropped
	for i := 0; i < 8; i++ {
		o.parser.Feed("@move not json\n") // more drops to exceed the cap
	}
	rep := o.recordReport()
	if rep == nil || rep.Repaired != 1 || rep.Failed != 9 {
		t.Fatalf("report = %+v, want repaired 1 failed 9", rep)
	}
	if len(rep.Issues) > 5 {
		t.Fatalf("issues = %d, want <= 5", len(rep.Issues))
	}
}
```

Use the existing `newTestRoster` helper in `pkg/engine` tests (add the RB-1 one if absent).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestRecordReport -v`
Expected: FAIL, `undefined: recordReport`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/engine/record_report.go`:

```go
package engine

import "github.com/darkliquid/localrpg/pkg/turnstream"

// maxRecordIssues bounds the per-turn issue list so a pathological reply cannot
// bloat the history file.
const maxRecordIssues = 5

// maxRecordIssueChars bounds one issue's error text.
const maxRecordIssueChars = 120

// RecordIssue is one control record that needed repair or was dropped.
type RecordIssue struct {
	Type   string `json:"type"`
	Repair string `json:"repair,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RecordReport is a turn's control-record health summary.
type RecordReport struct {
	Total    int           `json:"total"`
	Repaired int           `json:"repaired"`
	Failed   int           `json:"failed"`
	Issues   []RecordIssue `json:"issues,omitempty"`
}

// recordReport summarises the parser's record health. It returns nil when every
// record arrived valid, so a clean turn adds nothing to the history.
func (o *TurnOrchestrator) recordReport() *RecordReport {
	if o.parser == nil {
		return nil
	}
	summary := o.parser.RepairReport()
	if summary.Repaired == 0 && summary.Failed == 0 {
		return nil
	}
	rep := &RecordReport{
		Total:    summary.Total,
		Repaired: summary.Repaired,
		Failed:   summary.Failed,
	}
	for _, rec := range o.parser.Records() {
		if len(rep.Issues) >= maxRecordIssues {
			break
		}
		switch {
		case rec.Err != nil:
			rep.Issues = append(rep.Issues, RecordIssue{
				Type:  rec.Type,
				Error: trimRunes(rec.Err.Error(), maxRecordIssueChars),
			})
		case rec.Repaired != "":
			rep.Issues = append(rep.Issues, RecordIssue{
				Type:   rec.Type,
				Repair: string(rec.Repaired),
			})
		}
	}
	return rep
}

// trimRunes truncates s to at most n runes.
func trimRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run TestRecordReport -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/record_report.go pkg/engine/record_report_test.go
git commit -m "feat(engine): summarise a turn's control-record health"
```

---

### Task 2: Attach the report to the turn and persist it

**Files:**
- Modify: `pkg/engine/history.go` (the `Turn` struct)
- Modify: `pkg/engine/orchestrator.go` (where the `Turn` is assembled and `logRepairReport` is called)
- Test: `pkg/engine/timeline_test.go` (append)

**Interfaces:**
- Consumes: `RecordReport` (Task 1).
- Produces: `Turn.RecordReport *RecordReport` (JSON tag `record_report`).

- [ ] **Step 1: Write the failing test**

```go
func TestRecordReportPersistsAcrossReload(t *testing.T) {
	// Build a turn with a record report, write it through Timeline.RecordTurn,
	// reload history, and assert the report survives.
	// Use the existing timeline test fixtures in this package.
	turn := Turn{Number: 1, RecordReport: &RecordReport{Total: 2, Repaired: 1, Failed: 1,
		Issues: []RecordIssue{{Type: "roll", Repair: "trailing_comma"}, {Type: "move", Error: "not JSON"}}}}
	got := roundTripTurn(t, turn) // helper that records and reloads
	if got.RecordReport == nil || got.RecordReport.Repaired != 1 || got.RecordReport.Failed != 1 {
		t.Fatalf("reloaded report = %+v", got.RecordReport)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestRecordReportPersists -v`
Expected: FAIL, `Turn has no field RecordReport`.

- [ ] **Step 3: Write minimal implementation**

In `pkg/engine/history.go`, add to `Turn`:

```go
	// RecordReport summarises how the turn's control records fared. Nil when
	// every record arrived valid.
	RecordReport *RecordReport `json:"record_report,omitempty"`
```

In `pkg/engine/orchestrator.go`, at the point the `Turn` is assembled (where `Roll`, `Checks`,
and `PendingCheck` are set), add:

```go
	turn.RecordReport = o.recordReport()
```

next to the existing `o.logRepairReport()` call added in RB-1 Task 8.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/history.go pkg/engine/orchestrator.go pkg/engine/timeline_test.go
git commit -m "feat(engine): persist a turn's control-record report"
```

---

### Task 3: Map the report into the turn DTO

**Files:**
- Modify: `pkg/gui/types.go` (`TurnDTO`)
- Modify: `pkg/gui/service.go` (`turnDTO`)
- Test: `pkg/gui/service_test.go` (append)

**Interfaces:**
- Consumes: `engine.RecordReport`, `engine.RecordIssue`.
- Produces: `TurnDTO.RecordReport *RecordReportDTO`, `type RecordReportDTO`, `type RecordIssueDTO`.

- [ ] **Step 1: Write the failing test**

```go
func TestTurnDTOMapsRecordReport(t *testing.T) {
	turn := engine.Turn{Number: 1, RecordReport: &engine.RecordReport{
		Total: 1, Repaired: 1, Issues: []engine.RecordIssue{{Type: "roll", Repair: "close"}}}}
	dto := turnDTOForTest(t, turn)
	if dto.RecordReport == nil || dto.RecordReport.Repaired != 1 {
		t.Fatalf("dto report = %+v", dto.RecordReport)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestTurnDTOMapsRecordReport -v`
Expected: FAIL, `dto.RecordReport undefined`.

- [ ] **Step 3: Write minimal implementation**

In `pkg/gui/types.go`:

```go
// RecordIssueDTO is one repaired or dropped control record.
type RecordIssueDTO struct {
	Type   string `json:"type"`
	Repair string `json:"repair,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RecordReportDTO is a turn's control-record health summary.
type RecordReportDTO struct {
	Total    int              `json:"total"`
	Repaired int              `json:"repaired"`
	Failed   int              `json:"failed"`
	Issues   []RecordIssueDTO `json:"issues,omitempty"`
}
```

Add `RecordReport *RecordReportDTO` to `TurnDTO`. In `turnDTO` (`pkg/gui/service.go`), map it where
`Verdict` is mapped:

```go
	if turn.RecordReport != nil {
		rr := &RecordReportDTO{
			Total:    turn.RecordReport.Total,
			Repaired: turn.RecordReport.Repaired,
			Failed:   turn.RecordReport.Failed,
		}
		for _, issue := range turn.RecordReport.Issues {
			rr.Issues = append(rr.Issues, RecordIssueDTO{Type: issue.Type, Repair: issue.Repair, Error: issue.Error})
		}
		dto.RecordReport = rr
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestTurnDTOMaps -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/service_test.go
git commit -m "feat(gui): expose a turn's record report in the DTO"
```

---

### Task 4: Mirror the DTO in the frontend types

**Files:**
- Modify: `frontend/src/types.ts` (the `Turn` interface)

**Interfaces:**
- Consumes: `RecordReportDTO` (Task 3).
- Produces: `RecordIssue` and `RecordReport` TS types on `Turn`.

- [ ] **Step 1: Add the types**

```ts
export interface RecordIssue {
  type: string;
  repair?: string;
  error?: string;
}

export interface RecordReport {
  total: number;
  repaired: number;
  failed: number;
  issues?: RecordIssue[];
}
```

Add `record_report?: RecordReport;` to the `Turn` interface.

- [ ] **Step 2: Typecheck**

Run: `npx tsc --noEmit` (in `frontend/`)
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/types.ts
git commit -m "feat(frontend): type the turn record report"
```

---

### Task 5: The turn chip and popover

**Files:**
- Create: `frontend/src/components/RecordNotice.tsx`
- Modify: `frontend/src/components/TurnSegments.tsx` (render it)
- Test: `frontend/src/components/RecordNotice.test.tsx`

**Interfaces:**
- Consumes: `RecordReport` (Task 4).
- Produces: `<RecordNotice report={...} />`.

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { RecordNotice } from "./RecordNotice";

test("renders nothing without a report", () => {
  const { container } = render(<RecordNotice report={undefined} />);
  expect(container).toBeEmptyDOMElement();
});

test("names repairs and drops", () => {
  render(<RecordNotice report={{ total: 2, repaired: 1, failed: 1,
    issues: [{ type: "roll", repair: "close" }, { type: "move", error: "not JSON" }] }} />);
  expect(screen.getByText(/1 repaired/i)).toBeInTheDocument();
  expect(screen.getByText(/1 dropped/i)).toBeInTheDocument();
});
```

Use the existing frontend test runner and setup.

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- RecordNotice` (in `frontend/`)
Expected: FAIL, module not found.

- [ ] **Step 3: Write minimal implementation**

```tsx
import type { RecordReport } from "../types";

function summary(report: RecordReport): string {
  const parts: string[] = [];
  if (report.repaired > 0) parts.push(`${report.repaired} repaired`);
  if (report.failed > 0) parts.push(`${report.failed} dropped`);
  return `Record trouble: ${parts.join(", ")}`;
}

export function RecordNotice({ report }: { report?: RecordReport }) {
  if (!report || (report.repaired === 0 && report.failed === 0)) return null;
  const warning = report.failed > 0;
  return (
    <details className={`rounded-lg border px-3 py-2 text-xs ${
      warning ? "border-amber-500/40 bg-amber-500/10 text-amber-200"
              : "border-white/10 bg-white/5 text-zinc-300"}`}>
      <summary className="cursor-pointer select-none">{summary(report)}</summary>
      <ul className="mt-2 space-y-1">
        {(report.issues ?? []).map((issue, i) => (
          <li key={i}>
            <span className="font-mono">{issue.type}</span>
            {issue.repair ? <> — repaired ({issue.repair})</> : <> — dropped: {issue.error}</>}
          </li>
        ))}
      </ul>
    </details>
  );
}
```

Render `<RecordNotice report={turn.record_report} />` at the top of the turn's segment list in
`TurnSegments.tsx`.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- RecordNotice`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/RecordNotice.tsx frontend/src/components/RecordNotice.test.tsx frontend/src/components/TurnSegments.tsx
git commit -m "feat(frontend): surface a turn's record trouble"
```

---

### Task 6: The Debug panel block

**Files:**
- Modify: `frontend/src/components/DebugPanel.tsx`
- Test: `frontend/src/components/DebugPanel.test.tsx` (append)

**Interfaces:**
- Consumes: `RecordReport` (Task 4).

- [ ] **Step 1: Write the failing test**

```tsx
test("debug panel lists record issues", () => {
  render(<DebugPanel turn={{ /* minimal turn */ record_report: {
    total: 1, repaired: 0, failed: 1, issues: [{ type: "roll", error: "not JSON" }] } }} />);
  expect(screen.getByText(/roll/)).toBeInTheDocument();
  expect(screen.getByText(/not JSON/)).toBeInTheDocument();
});
```

Adapt to `DebugPanel`'s real props.

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- DebugPanel`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add a "Record report" section to `DebugPanel` that renders the counts and every issue as a row.
It reuses `RecordNotice` for consistency, or renders a table if the panel already uses tables.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- DebugPanel`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/DebugPanel.tsx frontend/src/components/DebugPanel.test.tsx
git commit -m "feat(frontend): show record diagnostics in the debug panel"
```

---

### Task 7: End-to-end and verification

**Files:**
- Test: `pkg/gui/` driver end-to-end (append to an existing `_e2e_test.go`)

**Interfaces:**
- Consumes: everything above.
- Produces: no production symbols.

- [ ] **Step 1: Write the end-to-end test**

Drive a turn whose GM reply contains a malformed `@roll` (for example a trailing comma), then
assert the `turn` DTO carries a `RecordReport` with `Repaired >= 1`.

Use the existing driver harness (`pkg/gui/export_player_e2e_test.go` shows the pattern).

- [ ] **Step 2: Run it**

Run: `go test ./pkg/gui/ -run RecordDiagnostics -v`
Expected: PASS (or SKIP when the driver is unavailable, matching repo convention).

- [ ] **Step 3: Full verification**

Run: `mise run test` and `mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- A clean turn shows nothing.
- A repaired-only turn shows a neutral chip.
- A dropped-record turn shows a warning chip.
- The Debug panel lists the issues.
- Reloading the campaign shows the same report.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: cover record diagnostics end to end"
```
