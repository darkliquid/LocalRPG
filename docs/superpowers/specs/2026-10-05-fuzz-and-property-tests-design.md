# Fuzz and Property Tests Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#71 RB-6](https://github.com/darkliquid/Projects/LocalRPG/issues/71)
**Epic:** [#23 Malformed output and playback integrity](https://github.com/darkliquid/Projects/LocalRPG/issues/23)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §5 (RB-6)
**Depends on:** [#66 RB-1](https://github.com/darkliquid/Projects/LocalRPG/issues/66), [#67 RB-2](https://github.com/darkliquid/Projects/LocalRPG/issues/67)
**Scope:** `pkg/turnstream`, `pkg/jsonrepair`, `pkg/media`, `pkg/engine`

---

## 1. Problem

This epic is about a class of bug: malformed input and duplicate output. Both are **edge-case** bugs
that example-based tests miss: the parser handles the cases someone thought of, and the group planner
handles the segment sequences someone wrote down. The real failures are the cases nobody imagined.

The repo already uses fuzzing where it matters (the `roll` library has fuzz targets) and the codebase
convention is standard-library tests. Fuzzing and property tests are the standard-library answer, and
they are cheap.

## 2. Goals

- **Fuzz** the `turnstream` parser: arbitrary bytes never panic, and every emitted event is
  well-formed.
- **Fuzz** `jsonrepair.Repair` (RB-1): a reported-OK payload is always valid JSON (RB-1's plan adds
  this; RB-6 wires it into the suite and extends it).
- **Property-test** the group planner: for arbitrary segment sequences, the streamed fold equals the
  finalised plan, and their segment-index sets agree (RB-2).
- A property test for the caption/timing agreement, where cheap.
- The targets run in CI with a short time budget, and the seeds are committed.

## 3. Non-goals

- Fuzzing everything. The epic's risky inputs are the parser and the repair; the planner is the risky
  logic.
- A performance benchmark; the size benchmark is TH-5.
- Replacing the example tests; fuzzing adds to them.

## 4. Design

### 4.1 Parser fuzz

```go
func FuzzParser(f *testing.F) {
	// Seeds: real replies, a reply with a malformed record, a multi-line record, prose.
	f.Fuzz(func(t *testing.T, data []byte) {
		p := turnstream.NewParser(roster)
		p.Feed(string(data))
		_ = p.Flush()
		for _, ev := range p.Events() {
			// An event is narration, speech, or a record; a speech event has a speaker;
			// a record event has a non-nil record; no event has an empty text of the
			// wrong kind.
		}
	})
}
```

Invariants:

- no panic;
- every event's `Kind` is one of the three;
- a `KindRecord` event's `Record` is non-nil;
- a `KindSpeech` event has a non-empty `Speaker` or `SpeakerID`;
- `Events()` is deterministic for the same input (fuzzing the same bytes twice yields the same
  events).

### 4.2 Repair fuzz

RB-1's plan adds `FuzzRepair`. RB-6 extends it:

- a `Result.OK` payload is `json.Valid`;
- `Repair` is idempotent: `Repair(Repair(x).Payload).Payload == Repair(x).Payload` when the first is
  OK;
- `BraceDepth` never panics on arbitrary bytes.

### 4.3 Group planner property test

The planner's property is that the **streaming fold** and the **batch plan** agree:

```go
func TestGroupFoldEqualsPlanProperty(t *testing.T) {
	// For many random segment sequences (speakers, lengths, kinds), fold with
	// GroupFolder(budget 0) and compare to GroupPlan; assert the group lines and
	// the segment-index sets are identical.
}
```

This generalises the existing `TestGroupFolderMatchesPlanGroups` (which uses a fixed sequence) to
random sequences, and adds the segment-index assertion RB-2 needs. It is a property test, not a fuzz
target, because the input space is structured (a sequence of speakers and texts) rather than bytes.

A second property: a streamed clip key equals the finalised group key for every group, for arbitrary
sequences. This is RB-2's guarantee, checked directly.

### 4.4 The caption/timing property

Where cheap: the WebVTT cue for a beat and the theatre's caption for the same beat agree (TH-4). A
property test over random beat sequences.

### 4.5 CI

The fuzz targets run with a short budget (`-fuzztime 10s` each) in a CI step, so a regression in the
seed corpus fails, and the seeds are committed under `testdata/fuzz`. A longer run is a local task
(`mise run test:fuzz`).

## 5. Behaviour

Not applicable; these are tests. The outcome: a malformed input or a divergent fold fails CI rather
than a player's turn.

## 6. Testing

This **is** the testing. The spec's test section is the targets themselves, plus:

- the seeds are committed;
- the CI step runs the targets with a budget;
- a `mise run test:fuzz` task runs them longer locally.

## 7. Rollout

Additive: test files, seeds, and a CI step. No production change.

## 8. Risks

- **Flaky fuzz findings.** A fuzz target that finds a real bug is a bug to fix, not to suppress. The
  seeds capture it, and the fix lands with the seed.
- **CI time.** A short budget bounds it; a longer run is local.
- **Over-constraining the invariants.** An invariant that is too strict fails on a legitimate input.
  Start with the invariants above, which are the parser's and the planner's actual contracts, and
  loosen one only with a reason.
