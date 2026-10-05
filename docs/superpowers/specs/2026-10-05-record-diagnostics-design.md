# Record Diagnostics Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#70 RB-5](https://github.com/darkliquid/LocalRPG/issues/70)
**Epic:** [#23 Malformed output and playback integrity](https://github.com/darkliquid/LocalRPG/issues/23)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §5 (RB-5)
**Depends on:** [#66 RB-1](https://github.com/darkliquid/LocalRPG/issues/66)
**Scope:** `pkg/engine`, `pkg/gui`, `frontend`

---

## 1. Problem

A turn's control records are consumed by `TurnOrchestrator.applyRecords`
(`pkg/engine/streamsegments.go:41-74`) and `pendingRoll`
(`pkg/engine/streamsegments.go:78-93`). A record that failed to parse is logged as
`turn.record_error` and skipped (`pkg/engine/streamsegments.go:50-53`), and a repaired record is
now usable (RB-1). Both facts are visible **only in the JSONL trace**, which a player never opens.

So when the GM "ignores" a roll, or dialogue is attributed to the narrator, the player has no way
to know the model emitted a broken record, and an author filing a bug has to be told to read a
trace file.

RB-1 produces the data (`Parser.RepairReport`, `Record.Repaired`, `Record.Err`); RB-5 surfaces it.

## 2. Goals

- A turn that had records repaired or dropped says so **in the turn**, where the player is looking.
- The detail (which record, what went wrong) is one click away, and present in the Debug panel.
- The signal survives a reload: reopening a campaign still shows that turn's record trouble.
- The common case (no repairs, no failures) adds nothing to the UI.

## 3. Non-goals

- Repairing records. That is RB-1.
- Retrying the model. That is RB-4.
- Changing what a degraded turn does; this is reporting only.

## 4. Design

### 4.1 A compact, persisted report on the turn

`engine.Turn` (`pkg/engine/history.go`) gains one field:

```go
// RecordReport summarises how the turn's control records fared: how many were
// repaired or dropped, and a short list of the ones that were. Nil when every
// record arrived valid.
RecordReport *RecordReport `json:"record_report,omitempty"`
```

```go
// RecordIssue is one control record that needed repair or was dropped.
type RecordIssue struct {
	Type     string `json:"type"`
	Repair   string `json:"repair,omitempty"` // jsonrepair.Kind, empty when dropped
	Error    string `json:"error,omitempty"`  // empty when repaired
}

// RecordReport is the per-turn record-health summary.
type RecordReport struct {
	Total    int           `json:"total"`
	Repaired int           `json:"repaired"`
	Failed   int           `json:"failed"`
	Issues   []RecordIssue `json:"issues,omitempty"` // capped at 5
}
```

The orchestrator builds it once, next to the existing `logRepairReport` call (RB-1, Task 8), from
`o.parser.RepairReport()` and `o.parser.Records()`:

```go
func (o *TurnOrchestrator) recordReport() *RecordReport
```

It returns nil when `Repaired == 0 && Failed == 0`, so the field is absent for healthy turns and
the history file is unchanged for them. The `Issues` list is capped so a pathological reply cannot
bloat `history.jsonl`.

### 4.2 It reaches the client

`TurnDTO` (`pkg/gui/types.go`) gains `RecordReport *RecordReportDTO`, mapped from `engine.Turn` in
`turnDTO` (`pkg/gui/service.go`), the same place `Verdict`, `Checks`, and `PendingCheck` are
mapped. The `turn` NDJSON event already carries the DTO, so no new event type is needed.

On reload, `TurnDTO` is rebuilt from history, and because the report is persisted it is present
again.

### 4.3 It reaches the eye

Frontend, two surfaces:

1. **The turn.** A small, quiet chip in `TurnSegments.tsx` (or `ChronicleView.tsx`), rendered only
   when `turn.record_report` is present:
   - text: "Record trouble: 1 repaired, 1 dropped" (phrased from the counts);
   - a details popover listing each issue as `type — repaired (kind)` or `type — dropped: error`;
   - a tone: neutral when nothing failed, warning when `failed > 0`.
   It must not read as an error when a record was merely repaired, because a repaired record was
   used successfully.
2. **The Debug panel.** `DebugPanel.tsx` already renders turn traces; the report is shown as a
   structured block so the detail is available without a popover.

### 4.4 Copy

User-facing strings stay plain and non-alarming:

- all repaired: "A control record needed a small fix and was used."
- any dropped: "A control record could not be read and was skipped. This can make the GM miss a
  roll or misattribute a line."

The exact strings live in one place (a small helper) so they are consistent and translatable later.

## 5. Behaviour

| Report | Turn UI | Debug panel |
| --- | --- | --- |
| nil (clean turn) | nothing | nothing |
| repaired only | quiet chip, neutral tone | issues listed |
| failed only | chip, warning tone | issues listed |
| both | chip, warning tone, counts | issues listed |
| turn reloaded from history | identical to live | identical |

## 6. Testing

- `pkg/engine`: `recordReport` returns nil for a clean stream; counts a repaired record; counts a
  dropped record; caps `Issues` at 5; the report round-trips through `history.jsonl` and a reload.
- `pkg/gui`: `turnDTO` maps the report; the DTO is nil when the engine report is nil.
- `frontend`: the chip renders only with a report; the popover lists issues; the tone follows
  `failed`. (Component test in the existing frontend test setup.)
- A driver end-to-end turn with a deliberately malformed `@roll` shows the chip.

## 7. Rollout

Additive. Old `history.jsonl` records have no `record_report` and render nothing. No migration.

## 8. Risks

- **Noise.** A model that occasionally mis-formats a record would show a chip often, training the
  player to ignore it. Mitigation: the quiet tone for repaired-only, and the copy makes clear a
  repair is not a failure. If it proves noisy, the chip can be gated behind a preference without
  changing the data.
- **History size.** Bounded by the 5-issue cap.
- **Leaking model output.** An error string can contain a fragment of the model's text. That is
  acceptable (it is the same text the player already sees as prose), but the error string is
  trimmed to a short length before display.
