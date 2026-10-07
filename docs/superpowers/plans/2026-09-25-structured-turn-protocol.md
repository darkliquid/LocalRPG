# Structured Turn Protocol Implementation Plan

> **SUPERSEDED (2026-10-03).** The `submit_turn` approach it delivered is replaced
> by `2026-10-03-progressive-turn-stream.md`. Kept for history only.

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the GM author a typed turn through a terminal `submit_turn` tool call, with mid-stream `request_check` calls and an explicit action verdict, so narration, speech, personae, checks, and outcomes stop being reconstructed from prose.

**Architecture:** New transport-neutral protocol types in `pkg/harness` are exposed as two turn tools (`submit_turn`, `request_check`) alongside the existing query tools. The engine's tool loop resolves `request_check` through a `CheckResolver` interface and treats `submit_turn` as terminal. A validator audits the submission against the resolved checks. `ProcessActionStream` builds `Turn.Segments` from the submission instead of `dialogue.Parse`, creates persona stubs via `Timeline`, and keeps the extractor only as a fallback for tool-incapable providers or a malformed submission.

**Tech Stack:** Go 1.27.1 (`pkg/harness`, `pkg/engine`, `pkg/entity`, `pkg/tools`), React 19 + TypeScript (chronicle rendering).

**Spec:** `docs/superpowers/specs/2026-09-25-structured-turn-protocol-design.md`

## Global Constraints

- Go 1.27.1. Standard library only for tests (`testing`, `t.TempDir()`); no testify.
- Use `any`, not `interface{}`; wrap errors with `fmt.Errorf("...: %w", err)`; `go vet ./...` clean.
- The engine stays schema-agnostic; check resolution and state application are behind interfaces (`CheckResolver`, host API) provided by the mechanics spec.
- `Turn.Narration` remains populated (derived from narration segments); the change is additive and existing `history.jsonl` keeps working.
- The extractor is never invoked when a valid `submit_turn` was accepted.
- TypeScript: `strict`, `noUnusedLocals`, `noUnusedParameters`; `npx tsc --noEmit` is the gate.
- Conventional Commits with a scope; subject under 72 characters.

---

## File Map

**Create**
- `pkg/harness/turn.go` — protocol types (`ActionVerdict`, `SegmentSpec`, `PersonaDecl`, `MemoryDecl`, `StateChangeDecl`, `CheckRequest`, `CheckResult`, `TurnSubmission`).
- `pkg/harness/turn_tools.go` — `TurnToolSpecs()`, `TurnToolNames()`, `IsTurnTool(name)`, `ParseSubmission(args)`, `ParseCheckRequest(args)`.
- `pkg/harness/turn_test.go`, `pkg/harness/turn_tools_test.go`.
- `pkg/engine/submission.go` — `validateSubmission`, `buildSegments`, `stagePersonae`.
- `pkg/engine/submission_test.go`.
- `pkg/engine/check_resolver.go` — the `CheckResolver` interface and a deterministic default used until the mechanics spec lands.

**Modify**
- `pkg/engine/orchestrator.go` — loop handles `request_check`/`submit_turn`; `streamResult` carries `Submission` and `Checks`; `ProcessActionStream` consumes the submission; verdict/rejected persisted.
- `pkg/engine/history.go` — `Turn` gains `Verdict`, `Rejected`, `Checks`, `Personae`.
- `pkg/engine/segments.go` — `buildSegmentsFromSubmission` path; retire `dialogue.Parse` for structured turns.
- `pkg/engine/timeline.go` — persona stub staging.
- `pkg/gui/service.go`, `pkg/gui/types.go` — `TurnDTO` carries verdict/checks; turn tool capability wiring.
- `frontend/src/types.ts`, `frontend/src/components/Chronicle.tsx` (or equivalent) — render segments/verdict.

---

### Task 1: Protocol types

**Files:**
- Create: `pkg/harness/turn.go`
- Test: `pkg/harness/turn_test.go`

**Interfaces:**
- Consumes: `pkg/rules.RollResult`.
- Produces: the types listed in spec §3.2.

- [x] **Step 1: Write the failing test**

```go
package harness

import "testing"

func TestTurnSubmissionRoundTrip(t *testing.T) {
	raw := `{
		"action_verdict": {"feasibility": "uncertain", "reason": "a rope bridge over a chasm"},
		"segments": [
			{"kind": "narration", "text": "The bridge sways."},
			{"kind": "speech", "speaker": "Kae", "text": "Hold the rope!"}
		],
		"personae": [{"name": "Kae", "type": "character", "new": true, "gender": "woman", "role_tags": ["scout"]}],
		"memories": [{"kind": "event", "entity_refs": ["kae", "player"], "text": "Crossed the rope bridge.", "importance": 3}],
		"state_changes": [{"entity": "player", "path": "hp", "op": "sub", "value": 1, "reason": "strain"}]
	}`
	sub, err := ParseSubmission(raw)
	if err != nil {
		t.Fatalf("ParseSubmission: %v", err)
	}
	if sub.Verdict.Feasibility != FeasibilityUncertain {
		t.Fatalf("feasibility = %q", sub.Verdict.Feasibility)
	}
	if len(sub.Segments) != 2 || sub.Segments[1].Speaker != "Kae" {
		t.Fatalf("segments = %+v", sub.Segments)
	}
	if len(sub.Personae) != 1 || !sub.Personae[0].New {
		t.Fatalf("personae = %+v", sub.Personae)
	}
	if len(sub.StateChanges) != 1 || sub.StateChanges[0].Op != "sub" {
		t.Fatalf("state changes = %+v", sub.StateChanges)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestTurnSubmissionRoundTrip ./pkg/harness/`
Expected: FAIL (`undefined: ParseSubmission`).

- [x] **Step 3: Implement the types**

Create `pkg/harness/turn.go` with the type definitions from spec §3.2 plus:

```go
const (
	FeasibilityAutomatic ActionFeasibility = "automatic"
	FeasibilityUncertain ActionFeasibility = "uncertain"
	FeasibilityImpossible ActionFeasibility = "impossible"
)

// ParseSubmission decodes a submit_turn argument object.
func ParseSubmission(args string) (*TurnSubmission, error) {
	var sub TurnSubmission
	if err := json.Unmarshal([]byte(args), &sub); err != nil {
		return nil, fmt.Errorf("parse submit_turn: %w", err)
	}
	return &sub, nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestTurnSubmissionRoundTrip ./pkg/harness/` and `go build ./...`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/harness/turn.go pkg/harness/turn_test.go
git commit -m "feat(harness): add structured turn protocol types"
```

---

### Task 2: Turn tool specs

**Files:**
- Create: `pkg/harness/turn_tools.go`
- Test: `pkg/harness/turn_tools_test.go`

**Interfaces:**
- Produces: `TurnToolSpecs() []ToolSpec`; `TurnToolNames() []string`; `IsTurnTool(name string) bool`; `ParseCheckRequest(args string) (*CheckRequest, error)`.

- [x] **Step 1: Write the failing test**

```go
package harness

import "testing"

func TestTurnToolSpecs(t *testing.T) {
	names := map[string]bool{}
	for _, spec := range TurnToolSpecs() {
		names[spec.Name] = true
		if spec.Description == "" || spec.Parameters == nil {
			t.Fatalf("tool %q missing description or parameters", spec.Name)
		}
	}
	for _, want := range []string{"submit_turn", "request_check"} {
		if !names[want] {
			t.Fatalf("missing turn tool %q", want)
		}
	}
	if !IsTurnTool("submit_turn") || IsTurnTool("search_entities") {
		t.Fatal("IsTurnTool misclassified a tool")
	}
}

func TestParseCheckRequest(t *testing.T) {
	req, err := ParseCheckRequest(`{"actor":"player","check_kind":"skill","stat":"stealth","difficulty":"hard","stakes":"avoid the guard","outcomes":{"pass":"sneak past","fail":"spotted"}}`)
	if err != nil {
		t.Fatalf("ParseCheckRequest: %v", err)
	}
	if req.Stat != "stealth" || req.Outcomes["fail"] != "spotted" {
		t.Fatalf("req = %+v", req)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestTurnToolSpecs|TestParseCheckRequest' ./pkg/harness/`
Expected: FAIL.

- [x] **Step 3: Implement**

Create `pkg/harness/turn_tools.go` with `ToolSpec` entries whose JSON Schema mirror the structs (properties for each field; `submit_turn` requires `action_verdict` and `segments`; `request_check` requires `actor`, `check_kind`, `stakes`, `outcomes`), plus `ParseCheckRequest` as in Task 1. `IsTurnTool` checks membership in `TurnToolNames()`.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestTurnToolSpecs|TestParseCheckRequest' ./pkg/harness/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/harness/turn_tools.go pkg/harness/turn_tools_test.go
git commit -m "feat(harness): add submit_turn and request_check tool specs"
```

---

### Task 3: Check resolver boundary

**Files:**
- Create: `pkg/engine/check_resolver.go`
- Test: `pkg/engine/check_resolver_test.go`

**Interfaces:**
- Consumes: `harness.CheckRequest`, `harness.CheckResult`, `rules.RollResult`.
- Produces: `type CheckResolver interface { Resolve(ctx context.Context, req harness.CheckRequest, actor *entity.Entity) (*harness.CheckResult, error) }`; `type defaultCheckResolver struct{}` that rolls the request's notation (or `2d6`) and returns `pass`/`fail` at total ≥ 8; `assignCheckIDs` helper.

The mechanics spec replaces the default with a schema/js resolver; the interface is the seam.

- [x] **Step 1: Write the failing test**

```go
func TestDefaultCheckResolver(t *testing.T) {
	r := defaultCheckResolver{}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Actor: "player", CheckKind: "skill", Notation: "1d6+10",
		Outcomes: map[string]string{"pass": "ok", "fail": "no"},
	}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != "pass" {
		t.Fatalf("outcome = %q, want pass", res.Outcome)
	}
	if res.CheckID == "" {
		t.Fatal("CheckID is empty")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestDefaultCheckResolver ./pkg/engine/`
Expected: FAIL.

- [x] **Step 3: Implement**

```go
type CheckResolver interface {
	Resolve(ctx context.Context, req harness.CheckRequest, actor *entity.Entity) (*harness.CheckResult, error)
}

type defaultCheckResolver struct{}

func (defaultCheckResolver) Resolve(_ context.Context, req harness.CheckRequest, _ *entity.Entity) (*harness.CheckResult, error) {
	notation := req.Notation
	if notation == "" {
		notation = "2d6"
	}
	roll, err := rules.EvaluateRoll(notation)
	if err != nil {
		return nil, fmt.Errorf("resolve check: %w", err)
	}
	outcome := "fail"
	if roll.Total >= 8 {
		outcome = "pass"
	}
	return &harness.CheckResult{CheckID: newCheckID(), Roll: roll, Outcome: outcome}, nil
}

func newCheckID() string { return "chk_" + strconv.FormatInt(time.Now().UnixNano(), 36) }
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestDefaultCheckResolver ./pkg/engine/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/check_resolver.go pkg/engine/check_resolver_test.go
git commit -m "feat(engine): add a check resolver boundary"
```

---

### Task 4: Tool loop handles turn tools

**Files:**
- Modify: `pkg/engine/orchestrator.go` (`streamResult`, `runGenerationLoop`)
- Test: `pkg/engine/structured_turn_test.go`

**Interfaces:**
- Consumes: `harness.ParseSubmission`, `harness.ParseCheckRequest`, `IsTurnTool`, `CheckResolver`.
- Produces: `streamResult.Submission *harness.TurnSubmission`, `streamResult.Checks []harness.CheckResult`; `(*TurnOrchestrator).SetCheckResolver(CheckResolver)`.

- [x] **Step 1: Write the failing test**

Use `toolLoopOrchestrator(t, provider)` and a scripted provider that returns a `request_check` call then a `submit_turn` call:

```go
func TestLoopResolvesCheckThenSubmits(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "request_check", Arguments: `{"actor":"player","check_kind":"skill","stakes":"jump","outcomes":{"pass":"clear","fail":"fall"}}`}}},
		{tools: []harness.ToolCall{{ID: "2", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"uncertain","reason":"a gap"},"segments":[{"kind":"narration","text":"You leap.","check_ref":"1"}]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	o.SetCheckResolver(defaultCheckResolver{})
	// runGenerationLoop is unexported; a small exported-for-test wrapper is added.
	result, err := o.runGenerationLoopForTest(context.Background())
	if err != nil {
		t.Fatalf("loop: %v", err)
	}
	if result.Submission == nil {
		t.Fatal("no submission")
	}
	if len(result.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(result.Checks))
	}
}
```

Provide `runGenerationLoopForTest` as a thin wrapper in a `_test.go` file that builds a minimal `AssembleResult` and calls `runGenerationLoop`.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestLoopResolvesCheckThenSubmits ./pkg/engine/`
Expected: FAIL.

- [x] **Step 3: Implement**

- Add `Submission *harness.TurnSubmission` and `Checks []harness.CheckResult` to `streamResult`.
- Add `checkResolver CheckResolver` to `TurnOrchestrator` (default `defaultCheckResolver{}`) and `SetCheckResolver`.
- In `runGenerationLoop`, when `offerTools` and a call `IsTurnTool(name)`:
  - `request_check`: `ParseCheckRequest`, `Resolve`, append to a running `checks` slice, append a tool message with the JSON `CheckResult`, continue.
  - `submit_turn`: `ParseSubmission`, set `result.Submission`, attach `checks`, return immediately.
- Keep query tools going to `o.toolExecutor` as today.
- Add `submit_turn`/`request_check` to the offered tools only when `offerTools` (turn tools are withheld with the rest after the cap).

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestLoopResolvesCheckThenSubmits ./pkg/engine/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/structured_turn_test.go
git commit -m "feat(engine): handle submit_turn and request_check in the loop"
```

---

### Task 5: Submission validation

**Files:**
- Create: `pkg/engine/submission.go`
- Test: `pkg/engine/submission_test.go`

**Interfaces:**
- Produces: `validateSubmission(sub *harness.TurnSubmission, checks []harness.CheckResult, declaredStats map[string]bool) error`.

- [x] **Step 1: Write the failing test**

```go
func TestValidateSubmission(t *testing.T) {
	base := func() *harness.TurnSubmission {
		return &harness.TurnSubmission{
			Verdict:  harness.ActionVerdict{Feasibility: harness.FeasibilityUncertain, Reason: "gap"},
			Segments: []harness.SegmentSpec{{Kind: "narration", Text: "You leap."}},
		}
	}
	if err := validateSubmission(base(), nil, nil); err == nil {
		t.Fatal("uncertain with no check should fail")
	}
	sub := base()
	sub.Segments[0].CheckRef = "1"
	if err := validateSubmission(sub, []harness.CheckResult{{CheckID: "1"}}, nil); err != nil {
		t.Fatalf("valid submission rejected: %v", err)
	}
	imp := base()
	imp.Verdict.Feasibility = harness.FeasibilityImpossible
	if err := validateSubmission(imp, []harness.CheckResult{{CheckID: "1"}}, nil); err == nil {
		t.Fatal("impossible with a check should fail")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestValidateSubmission ./pkg/engine/`
Expected: FAIL.

- [x] **Step 3: Implement**

`validateSubmission` returns a typed `submissionError{Code, Detail}` (implementing `error`) for: `no_check`, `impossible_with_check`, `unknown_check`, `unknown_speaker` (speaker unresolved and not declared), `undeclared_stat`, `unjustified_dismissal`. The engine retries once with `"protocol validation failed: <code>: <detail>"` appended as a system reminder.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestValidateSubmission ./pkg/engine/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/submission.go pkg/engine/submission_test.go
git commit -m "feat(engine): validate structured turn submissions"
```

---

### Task 6: Build segments and personae from the submission

**Files:**
- Modify: `pkg/engine/submission.go` (add `buildSegments`, `stagePersonae`), `pkg/engine/orchestrator.go`, `pkg/engine/timeline.go`, `pkg/engine/history.go`
- Test: `pkg/engine/submission_segments_test.go`

**Interfaces:**
- Produces: `buildSegments(sub *harness.TurnSubmission, resolve func(string) (string, bool)) (string, []entity.TurnSegment)`; `stagePersonae(sub *harness.TurnSubmission) []entity.Entity`; `Turn.Personae []string`.

- [x] **Step 1: Write the failing test**

```go
func TestBuildSegmentsFromSubmission(t *testing.T) {
	sub := &harness.TurnSubmission{
		Segments: []harness.SegmentSpec{
			{Kind: "narration", Text: "The hall is cold."},
			{Kind: "speech", Speaker: "Kae", Text: "We should leave."},
		},
	}
	narration, segments := buildSegments(sub, func(name string) (string, bool) {
		if name == "Kae" { return "kae", true }
		return "", false
	})
	if narration != "The hall is cold." {
		t.Fatalf("narration = %q", narration)
	}
	if len(segments) != 2 || segments[1].SpeakerID != "kae" || segments[1].Kind != entity.SegmentSpeech {
		t.Fatalf("segments = %+v", segments)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestBuildSegmentsFromSubmission ./pkg/engine/`
Expected: FAIL.

- [x] **Step 3: Implement**

- `buildSegments` maps `SegmentSpec` to `entity.TurnSegment` (narration joins into `narration`), resolving speakers via the resolver; an unresolved declared-new persona resolves to its slugified id.
- `stagePersonae` turns `new` personae into `entity.Entity` stubs (`ID: entity.Slugify(Name)`, Type, State seeded from gender/pronouns/role tags under `state`, Body = description).
- In `ProcessActionStream`, when `result.Submission != nil`: skip `harness.Extractor`, set `turn.Narration`/`turn.Segments` from `buildSegments`, stage personae through `Timeline.RecordTurnContext`, and set `turn.Verdict`/`turn.Rejected`/`turn.Checks`/`turn.Personae`.
- Add `Personae []string json:"personae,omitempty"` to `Turn` and thread the created ids.
- Keep the extractor path for `result.Submission == nil`.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestBuildSegmentsFromSubmission ./pkg/engine/` and `go test ./pkg/engine/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/submission.go pkg/engine/orchestrator.go pkg/engine/timeline.go pkg/engine/history.go pkg/engine/submission_segments_test.go
git commit -m "feat(engine): build segments and personae from a submission"
```

---

### Task 7: Verdict, rejection, and persistence

**Files:**
- Modify: `pkg/engine/history.go`, `pkg/engine/orchestrator.go`, `pkg/gui/types.go`, `pkg/gui/service.go`
- Test: `pkg/engine/submission_verdict_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestImpossibleVerdictRejectsAction(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"impossible","reason":"no wings"},"segments":[{"kind":"narration","text":"You cannot fly."}]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	o.SetCheckResolver(defaultCheckResolver{})
	turn, err := o.ProcessActionStream(context.Background(), "Do", "I fly over the wall", nil)
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if !turn.Rejected || turn.Verdict == nil || turn.Verdict.Feasibility != harness.FeasibilityImpossible {
		t.Fatalf("turn = %+v", turn)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestImpossibleVerdictRejectsAction ./pkg/engine/`
Expected: FAIL.

- [x] **Step 3: Implement**

- `Turn.Verdict *harness.ActionVerdict`, `Turn.Rejected bool`, `Turn.Checks []harness.CheckResult`; populate in `ProcessActionStream`; `Rejected` true iff feasibility is `impossible`.
- These fields persist via `history.jsonl` automatically (whole `Turn` marshalled). Extend `TurnDTO` (`pkg/gui/types.go`) with `Verdict`/`Rejected`/`Checks` and map them in the DTO builder (`pkg/gui/service.go`).

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestImpossibleVerdictRejectsAction ./pkg/engine/` and `go test ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/history.go pkg/engine/orchestrator.go pkg/gui/types.go pkg/gui/service.go pkg/engine/submission_verdict_test.go
git commit -m "feat(engine): persist action verdict and rejection"
```

---

### Task 8: Player-proposed checks and mode mapping

**Files:**
- Modify: `pkg/engine/orchestrator.go`, `pkg/engine/history.go`
- Test: `pkg/engine/proposed_check_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestRollModeBecomesProposedCheck(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"automatic","reason":"player proposed a check; I dismiss it"},"segments":[{"kind":"narration","text":"No need to roll."}],"dismissed_checks":[{"check_ref":"proposed_1","reason":"no uncertainty"}]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	o.SetCheckResolver(defaultCheckResolver{})
	if _, err := o.ProcessActionStream(context.Background(), "Roll", "stealth 2d6", nil); err != nil {
		t.Fatalf("turn: %v", err)
	}
	// Assert the assembled prompt/context carried a proposed_check for this turn.
	if !o.lastContextCarriedProposedCheck() {
		t.Fatal("proposed check was not passed to the model")
	}
}
```

(Add a small test-only accessor recording the last assembled context.)

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestRollModeBecomesProposedCheck ./pkg/engine/`
Expected: FAIL.

- [x] **Step 3: Implement**

- Parse `Roll`/skill input into a `proposed_check` line added to the GM's context (`assembly.Prompt` prefix or an extra context section), e.g. `[PROPOSED CHECK: stealth 2d6 by player]`.
- `Do`/`Say`/`Story` share one pipeline; `/gm` sets `verdict` enforcement off for the turn and logs `gm.override`; `System` stays non-narrative.
- Validate `dismissed_checks` reasons in Task 5's validator.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestRollModeBecomesProposedCheck ./pkg/engine/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/history.go pkg/engine/proposed_check_test.go
git commit -m "feat(engine): support player-proposed checks and mode mapping"
```

---

### Task 9: Fallback and protocol events

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/submission_fallback_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestMalformedSubmissionFallsBackToProse(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "submit_turn", Arguments: `{"segments":[]}`}}},
		{tools: []harness.ToolCall{{ID: "2", Name: "submit_turn", Arguments: `{"segments":[]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	o.SetCheckResolver(defaultCheckResolver{})
	turn, err := o.ProcessActionStream(context.Background(), "Do", "look around", nil)
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if turn.Verdict != nil {
		t.Fatal("fallback turn should have no verdict")
	}
}
```

(This relies on the scripted provider's prose fallback; if no prose is produced the existing no-narration error path applies. Adjust the script to include a prose reply between the two malformed calls.)

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestMalformedSubmissionFallsBackToProse ./pkg/engine/`
Expected: FAIL.

- [x] **Step 3: Implement**

- On a validation failure, retry once with a system reminder; on a second failure, accept provisional prose (if any) as narration, skip personae/memories/state, leave `Verdict` nil, emit `turn.protocol_fallback` with the reason.
- The extractor runs only when the provider is tool-incapable or a fallback occurred.
- Emit `turn.protocol_error` on each rejected attempt and `turn.protocol_fallback` on acceptance.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestMalformedSubmissionFallsBackToProse ./pkg/engine/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/submission_fallback_test.go
git commit -m "feat(engine): fall back to prose on a malformed submission"
```

---

### Task 10: Frontend rendering

**Files:**
- Modify: `frontend/src/types.ts`, `frontend/src/components/Chronicle.tsx` (and its turn renderer), `frontend/src/api/client.ts` if needed
- Test: `mise run test:frontend`

- [x] **Step 1: Extend types**

Add to the turn DTO type: `verdict?: { feasibility: 'automatic'|'uncertain'|'impossible'; reason?: string }`, `rejected?: boolean`, and ensure `segments` is present.

- [x] **Step 2: Render**

- If `segments` is present, render them in order: narration as prose, speech as a speaker-attributed block; else fall back to `narration`.
- Show a rejected-action banner when `rejected` is true.
- Show inline check results (notation, total, outcome) when `checks` is present.

- [x] **Step 3: Verify**

Run: `mise run test:frontend && mise run build:frontend`
Expected: PASS.

- [x] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/Chronicle.tsx
git commit -m "feat(frontend): render structured turn segments and verdicts"
```

---

### Task 11: Full verification

- [x] **Step 1: Run everything**

Run: `mise run test` and `mise run lint`
Expected: PASS.

- [x] **Step 2: Manual checks**

1. With a tool-capable provider, one turn produces typed segments, a verdict, and any checks, with no extractor pass in the trace.
2. A speech line from a new persona creates exactly one stub entity.
3. An impossible action is stored rejected and does not resolve.
4. A tool-incapable provider still produces a usable turn via the extractor.

- [x] **Step 3: Commit fixups**

```bash
git add -A
git commit -m "test: verify structured turn protocol"
```
