# Mechanics Engagement & Visibility Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make mechanics engagement tunable (`off`/`auto`/`ask`), make the prompt reflect the policy, force a check when the model goes quiet, and make resolved checks legible in the chronicle.

**Architecture:** One resolved policy drives the prompt text, the offered tools, and submission validation. A cadence floor uses provider tool-call forcing where available. `ask` adds a `propose_check` tool and a pending-check turn that the player rolls.

**Tech Stack:** Go 1.27 (stdlib `testing`), provider tool-call modes, React 19. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-26-mechanics-engagement-and-visibility-design.md`

## Global Constraints

- Go tests use only `testing` and `t.TempDir()`; no testify, no mocks.
- Use `interface{}`, never `any`; wrap errors with `fmt.Errorf("...: %w", err)`; `go vet ./...` clean.
- No new dependencies.
- Conventional Commits with a scope; subject under 72 chars.
- Default policy is `auto`; default cadence 3; cap one forced check per turn.
- Verification: `mise run test`, `mise run lint`, `mise run build`.

---

### Task 1: The engagement policy

**Files:**
- Modify: `pkg/core/mechanics.go` (`MechanicsSpec` ~line 6-14)
- Modify: `pkg/config/types.go` (`Config` ~line 256-266, new `MechanicsConfig`, accessors; `DefaultConfig` ~line 271)
- Create: `pkg/engine/engagement.go`
- Test: `pkg/config/types_test.go`, `pkg/engine/engagement_test.go`

**Interfaces:**
- Produces: `core.MechanicsSpec.Engagement string`; `config.MechanicsConfig{Engagement, CadenceTurns}`; `(*Config).MechanicsEngagement()`, `(*Config).MechanicsCadenceTurns()`; `engine.ResolveEngagement(manifest, system, cfg) string`.

- [x] **Step 1: Write the failing tests**

Append to `pkg/config/types_test.go`:

```go
func TestMechanicsEngagementDefaults(t *testing.T) {
	if got := (&Config{}).MechanicsEngagement(); got != "auto" {
		t.Errorf("MechanicsEngagement() = %q, want auto", got)
	}
	if got := (&Config{Mechanics: MechanicsConfig{Engagement: "ask"}}).MechanicsEngagement(); got != "ask" {
		t.Errorf("MechanicsEngagement() = %q, want ask", got)
	}
	if got := (&Config{Mechanics: MechanicsConfig{Engagement: "banana"}}).MechanicsEngagement(); got != "auto" {
		t.Errorf("unknown engagement = %q, want auto", got)
	}
	if got := (&Config{}).MechanicsCadenceTurns(); got != 3 {
		t.Errorf("MechanicsCadenceTurns() = %d, want 3", got)
	}
}
```

Create `pkg/engine/engagement_test.go`:

```go
package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
)

func TestResolveEngagementOrder(t *testing.T) {
	sys := &core.SystemManifest{Mechanics: &core.MechanicsSpec{Engagement: "ask"}}
	cfg := &config.Config{Mechanics: config.MechanicsConfig{Engagement: "off"}}

	if got := ResolveEngagement(&core.GameManifest{}, sys, cfg); got != "ask" {
		t.Errorf("system default = %q, want ask", got)
	}
	campaign := &core.GameManifest{Settings: map[string]interface{}{"mechanics_engagement": "off"}}
	if got := ResolveEngagement(campaign, sys, cfg); got != "off" {
		t.Errorf("campaign override = %q, want off", got)
	}
	if got := ResolveEngagement(&core.GameManifest{}, nil, cfg); got != "off" {
		t.Errorf("config default = %q, want off", got)
	}
	if got := ResolveEngagement(&core.GameManifest{}, nil, &config.Config{}); got != "auto" {
		t.Errorf("built-in default = %q, want auto", got)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestMechanicsEngagement|TestResolveEngagement' ./pkg/config/ ./pkg/engine/ -v`
Expected: FAIL — undefined field/method.

- [x] **Step 3: Add the types and accessors**

In `pkg/core/mechanics.go`, add to `MechanicsSpec`:

```go
	// Engagement is the system's default mechanics policy: "off", "auto", or
	// "ask". Empty means the configured default.
	Engagement string `yaml:"engagement,omitempty"`
```

In `pkg/config/types.go`:

```go
// MechanicsConfig tunes how mechanics are engaged during play.
type MechanicsConfig struct {
	// Engagement is "off", "auto", or "ask". Empty means auto.
	Engagement string `yaml:"engagement,omitempty" json:"engagement,omitempty"`
	// CadenceTurns forces a check after this many turns without one. 0 disables.
	CadenceTurns int `yaml:"cadence_turns,omitempty" json:"cadence_turns,omitempty"`
}
```

add `Mechanics MechanicsConfig` to `Config`, and:

```go
// MechanicsEngagement is the configured policy, defaulting to "auto".
func (c *Config) MechanicsEngagement() string {
	switch strings.ToLower(strings.TrimSpace(c.Mechanics.Engagement)) {
	case "off", "auto", "ask":
		return strings.ToLower(strings.TrimSpace(c.Mechanics.Engagement))
	default:
		return "auto"
	}
}

// MechanicsCadenceTurns is how many turns without a check force one; 0 disables.
func (c *Config) MechanicsCadenceTurns() int {
	if c.Mechanics.CadenceTurns <= 0 {
		return 3
	}
	return c.Mechanics.CadenceTurns
}
```

- [x] **Step 4: Add the resolver**

Create `pkg/engine/engagement.go`:

```go
package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
)

// ResolveEngagement returns the mechanics policy in force: the campaign
// setting, else the system default, else the configured default.
func ResolveEngagement(manifest *core.GameManifest, system *core.SystemManifest, cfg *config.Config) string {
	if manifest != nil && manifest.Settings != nil {
		if value, ok := manifest.Settings["mechanics_engagement"].(string); ok {
			if normalized := normalizeEngagement(value); normalized != "" {
				return normalized
			}
		}
	}
	if system != nil && system.Mechanics != nil {
		if normalized := normalizeEngagement(system.Mechanics.Engagement); normalized != "" {
			return normalized
		}
	}
	if cfg != nil {
		return cfg.MechanicsEngagement()
	}
	return "auto"
}

func normalizeEngagement(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "auto", "ask":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}
```

- [x] **Step 5: Run tests and commit**

Run: `go test -run 'TestMechanicsEngagement|TestResolveEngagement' ./pkg/config/ ./pkg/engine/ -v && go test ./pkg/config/ ./pkg/engine/`

```bash
git add pkg/core/mechanics.go pkg/config/types.go pkg/config/types_test.go pkg/engine/engagement.go pkg/engine/engagement_test.go
git commit -m "feat(mechanics): add a tunable engagement policy"
```

---

### Task 2: Policy-aware prompt

**Files:**
- Modify: `pkg/harness/mechanics_instructions.go` (`FormatMechanicsInstructions` ~line 14-41)
- Modify: `pkg/engine/orchestrator.go` (`LoadPrompts` ~line 388-409; struct field `mechanicsPrompt`; Add a `SetMechanicsEngagement`)
- Modify: `pkg/gui/service.go` (prepareTurn, near `LoadPrompts` call ~line 1270)
- Test: `pkg/harness/mechanics_instructions_test.go` (update), `pkg/engine/mechanics_prompt_test.go` (update)

**Interfaces:**
- Produces: `harness.FormatMechanicsInstructions(spec *core.MechanicsSpec, engagement string) string`; `(*TurnOrchestrator).SetMechanicsEngagement(string)`.
- Consumes: `engine.ResolveEngagement`.

- [x] **Step 1: Write the failing tests**

In `pkg/harness/mechanics_instructions_test.go`, change the existing calls to pass a policy and add:

```go
func TestFormatMechanicsInstructionsPerPolicy(t *testing.T) {
	spec := &core.MechanicsSpec{Checks: core.CheckConventions{Notation: "2d6", Outcome: []string{"strong", "weak", "miss"}}}

	off := FormatMechanicsInstructions(spec, "off")
	if !strings.Contains(off, "disabled") || strings.Contains(off, "request_check") {
		t.Errorf("off text = %q", off)
	}
	auto := FormatMechanicsInstructions(spec, "auto")
	if !strings.Contains(auto, "request_check") || !strings.Contains(auto, "2d6") {
		t.Errorf("auto text = %q", auto)
	}
	ask := FormatMechanicsInstructions(spec, "ask")
	if !strings.Contains(ask, "propose_check") || strings.Contains(ask, "Call request_check") {
		t.Errorf("ask text = %q", ask)
	}
}
```

In `pkg/engine/mechanics_prompt_test.go`, update `o.LoadPrompts(...)` tests to set an engagement on the orchestrator and assert the prompt differs:

```go
	o.SetMechanicsEngagement("off")
	o.LoadPrompts(paths, "sys", "")
	if strings.Contains(o.mechanicsPrompt, "request_check") {
		t.Fatalf("off policy should not mention request_check: %q", o.mechanicsPrompt)
	}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run TestFormatMechanicsInstructions ./pkg/harness/ -v`
Expected: FAIL — signature mismatch.

- [x] **Step 3: Make the formatter policy-aware**

In `pkg/harness/mechanics_instructions.go`, change the signature and split the header by policy (keep the declared notation/outcome/difficulty tail shared):

```go
func FormatMechanicsInstructions(spec *core.MechanicsSpec, engagement string) string {
	var sb strings.Builder
	switch engagement {
	case "off":
		sb.WriteString("## RESOLVING UNCERTAINTY\n")
		sb.WriteString("Mechanics are disabled for this campaign. Do not roll, and do not call request_check or propose_check. Decide outcomes from the fiction and narrate consequences directly.\n")
	case "ask":
		sb.WriteString("## RESOLVING UNCERTAINTY\n")
		sb.WriteString("When an action has a chance of consequences, call propose_check with the stakes and the possible outcomes, then stop. Do not resolve it yourself; the player rolls and you adjudicate the result.\n")
	default:
		sb.WriteString("## RESOLVING UNCERTAINTY\n")
		sb.WriteString("Resolve with request_check before narrating whenever an outcome could cost or grant something the player would care about: harm, resources, standing, or a lasting change. ")
		sb.WriteString("State the stakes and the possible outcomes first. Do not roll for safe or trivial actions. NPCs do not roll; resolve opposition through the protagonist's check. Honour the outcome the engine returns.\n")
	}
	// ...existing notation/outcome/difficulty tail, unchanged...
	return sb.String()
}
```

In `pkg/engine/orchestrator.go`, add a field and setter:

```go
	mechanicsEngagement string
```

```go
// SetMechanicsEngagement selects the policy the mechanics instruction reflects.
func (o *TurnOrchestrator) SetMechanicsEngagement(engagement string) {
	o.mechanicsEngagement = engagement
}
```

In `LoadPrompts`, pass it: `o.mechanicsPrompt = harness.FormatMechanicsInstructions(spec, o.mechanicsEngagement)` (and the `nil`-spec branch likewise).

In `pkg/gui/service.go` prepareTurn, after `orchestrator.LoadPrompts(...)`:

```go
	orchestrator.SetMechanicsEngagement(engine.ResolveEngagement(manifest, sm, cfg))
```

(`sm` is the system manifest already loaded there; reuse it. If it is only loaded for stats, load it once into a variable used by both.)

- [x] **Step 4: Run tests and commit**

Run: `go test ./pkg/harness/ ./pkg/engine/ ./pkg/gui/`

```bash
git add pkg/harness/mechanics_instructions.go pkg/harness/mechanics_instructions_test.go pkg/engine/orchestrator.go pkg/engine/mechanics_prompt_test.go pkg/gui/service.go
git commit -m "feat(mechanics): make the instruction reflect the policy"
```

---

### Task 3: Tool surface and validation per policy

**Files:**
- Modify: `pkg/harness/turn_tools.go` (`TurnToolSpecs` ~line 21-78; add `propose_check` spec)
- Modify: `pkg/engine/orchestrator.go` (tool offering ~line 1402; tool dispatch ~line 1512-1536)
- Modify: `pkg/engine/submission.go` (`validateSubmission` ~line 101-161)
- Test: `pkg/engine/engagement_tools_test.go` (create)

**Interfaces:**
- Produces: `harness.TurnToolSpecsFor(engagement string) []ToolSpec`; `validateSubmission(sub, checks, declaredStats, proposed, engagement)`.

- [x] **Step 1: Write the failing test**

Create `pkg/engine/engagement_tools_test.go`:

```go
package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func baseSubmission() *harness.TurnSubmission {
	return &harness.TurnSubmission{
		Verdict:  harness.ActionVerdict{Feasibility: harness.FeasibilityAutomatic, Reason: "safe"},
		Segments: []harness.SegmentSpec{{Kind: "narration", Text: "You walk on."}},
	}
}

func TestValidationRejectsChecksWhenOff(t *testing.T) {
	withCheck := baseSubmission()
	withCheck.Segments[0].CheckRef = "c1"
	err := validateSubmission(withCheck, []harness.CheckResult{{CheckID: "c1"}}, nil, nil, "off")
	if err == nil {
		t.Fatal("off policy should reject a resolved check")
	}
}

func TestValidationRejectsSelfResolvedCheckWhenAsking(t *testing.T) {
	withCheck := baseSubmission()
	withCheck.Verdict.Feasibility = harness.FeasibilityUncertain
	withCheck.Segments[0].CheckRef = "c1"
	err := validateSubmission(withCheck, []harness.CheckResult{{CheckID: "c1"}}, nil, nil, "ask")
	if err == nil {
		t.Fatal("ask policy should reject a model-resolved check")
	}
}

func TestToolSpecsPerPolicy(t *testing.T) {
	off := harness.TurnToolSpecsFor("off")
	for _, spec := range off {
		if spec.Name == "request_check" || spec.Name == "propose_check" {
			t.Errorf("off should not offer %q", spec.Name)
		}
	}
	ask := harness.TurnToolSpecsFor("ask")
	var hasPropose, hasRequest bool
	for _, spec := range ask {
		hasPropose = hasPropose || spec.Name == "propose_check"
		hasRequest = hasRequest || spec.Name == "request_check"
	}
	if !hasPropose || hasRequest {
		t.Errorf("ask tools wrong: propose=%v request=%v", hasPropose, hasRequest)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestValidation* ./pkg/engine/ -v`
Expected: FAIL — signatures/undefined.

- [x] **Step 3: Policy-aware tools and validation**

In `pkg/harness/turn_tools.go`, add a `propose_check` spec modelled on `request_check` (same parameters, description: "Propose a check to the player: state the stakes and possible outcomes, then stop. The player rolls."), and:

```go
// TurnToolSpecsFor returns the turn tools a policy offers.
func TurnToolSpecsFor(engagement string) []ToolSpec {
	base := []ToolSpec{submitTurnSpec()}
	switch engagement {
	case "off":
		return base
	case "ask":
		return append(base, proposeCheckSpec())
	default:
		return append(base, requestCheckSpec())
	}
}
```

(Refactor the existing literal slice into `submitTurnSpec()`, `requestCheckSpec()`, and the new `proposeCheckSpec()`; keep `TurnToolSpecs()` as `TurnToolSpecsFor("auto")` for existing callers.)

In `pkg/engine/orchestrator.go`, replace `append(harness.ToolSpecs(), harness.TurnToolSpecs()...)` with `harness.TurnToolSpecsFor(o.mechanicsEngagement)`. In the tool dispatch switch, add a `propose_check` case that records a pending check (Task 6) — for now, return a tool error if the policy is not `ask`.

In `pkg/engine/submission.go`, add the parameter and rules:

```go
func validateSubmission(sub *harness.TurnSubmission, checks []harness.CheckResult, declaredStats map[string]core.StatSpec, proposed *harness.ProposedCheck, engagement string) error {
	// ...existing checks...
	if engagement == "off" && len(checks) > 0 {
		return &submissionError{Code: "checks_disabled", Detail: "mechanics are off"}
	}
	if engagement == "ask" && len(checks) > 0 {
		return &submissionError{Code: "check_not_player_rolled", Detail: "resolve via the player's roll"}
	}
	// ...existing rules...
}
```

Update the one production caller (`orchestrator.go:1543`) and every test caller to pass the policy (`nil`/`"auto"` in existing tests preserves behaviour).

- [x] **Step 4: Run tests and commit**

Run: `go test ./pkg/harness/ ./pkg/engine/`

```bash
git add pkg/harness/turn_tools.go pkg/engine/orchestrator.go pkg/engine/submission.go pkg/engine/submission_test.go pkg/engine/engagement_tools_test.go
git commit -m "feat(mechanics): offer and accept tools by policy"
```

---

### Task 4: Tool-call forcing plumbing

**Files:**
- Modify: `pkg/harness/types.go` (`GenerateRequest` ~line 73-90)
- Modify: `pkg/provider/openaichat/http.go` (request body ~line 220-272)
- Modify: `pkg/provider/geminillm/provider.go` (genai request ~line 465-578)
- Test: `pkg/provider/openaichat/usage_test.go` extends; `pkg/provider/geminillm/*_test.go` extends

**Interfaces:**
- Produces: `harness.GenerateRequest.ToolChoice string` (`""`, `"required"`, `"none"`).

- [x] **Step 1: Write the failing test**

In `openaichat`, assert the marshalled body carries `tool_choice` when `ToolChoice: "required"`:

```go
func TestToolChoiceRequiredMarshals(t *testing.T) {
	body := buildChatBody(harness.GenerateRequest{Prompt: "hi", ToolChoice: "required"}, nil, true)
	if !strings.Contains(string(body), `"tool_choice":"required"`) {
		t.Fatalf("body omitted tool_choice: %s", body)
	}
}
```

(If there is no `buildChatBody` helper, add the mapping inside the existing request construction and assert on the captured request body as the usage test does.)

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestToolChoiceRequired ./pkg/provider/openaichat/ -v`
Expected: FAIL.

- [x] **Step 3: Add the field and mappings**

`pkg/harness/types.go`: add `ToolChoice string` to `GenerateRequest` with a comment that providers may ignore it.

`pkg/provider/openaichat/http.go`: add `ToolChoice string \`json:"tool_choice,omitempty"\`` to the request struct and set it from `req.ToolChoice` (OpenAI accepts `"required"`/`"none"`).

`pkg/provider/geminillm/provider.go`: when `req.ToolChoice` is set, pass `ToolConfig: &genai.ToolConfig{FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAny}}` (`NONE` for `"none"`).

Other providers ignore the field.

- [x] **Step 4: Run tests and commit**

Run: `go test ./pkg/provider/... && mise run lint`

```bash
git add pkg/harness/types.go pkg/provider/openaichat/http.go pkg/provider/geminillm/provider.go pkg/provider/openaichat/*_test.go pkg/provider/geminillm/*_test.go
git commit -m "feat(providers): support forcing a tool call"
```

---

### Task 5: The cadence floor (`auto` only)

**Files:**
- Modify: `pkg/engine/orchestrator.go` (turn start, near mode dispatch)
- Test: `pkg/engine/cadence_test.go` (create)

**Interfaces:**
- Produces: `(*TurnOrchestrator).SetMechanicsCadence(int)`; behaviour: after N turns with no checks, the first assistant round carries `ToolChoice: "required"` (or a prompt nudge when unsupported), capped at once per turn.

- [x] **Step 1: Write the failing test**

Create `pkg/engine/cadence_test.go` using `toolLoopOrchestrator` and a scripted provider that records the generation request's `ToolChoice`:

```go
func TestCadenceForcesACheckAfterQuietTurns(t *testing.T) {
	o, recorder := cadenceOrchestrator(t) // existing scaffolding: builtin provider + history with 3 turns whose Checks are empty
	o.SetMechanicsCadence(3)
	if _, err := o.ProcessActionStream(context.Background(), "Do", "look around", nil); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if !recorder.SawToolChoice("required") {
		t.Fatal("expected the quiet-turn cadence to force a tool call")
	}
}
```

Build `cadenceOrchestrator` from the existing test scaffolding used by `toolLoopOrchestrator`; the history is written with `NewHistoryLogger(...).AppendTurn(engine.Turn{Number: n, Checks: nil})` three times, and the orchestrator is given a tool-capable provider.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestCadenceForcesACheck ./pkg/engine/ -v`
Expected: FAIL.

- [x] **Step 3: Implement the floor**

In `ProcessActionStream`, before the generation loop, when the resolved policy is `auto` and the cadence is reached:

```go
	if o.mechanicsEngagement == "auto" && o.mechanicsCadence > 0 {
		if o.quietTurns(pastTurns) >= o.mechanicsCadence {
			o.forceToolChoice = true
			o.logger.Event("mechanics.cadence", map[string]interface{}{"quiet_turns": o.quietTurns(pastTurns)})
		}
	}
```

```go
// quietTurns counts the trailing turns that resolved no checks.
func (o *TurnOrchestrator) quietTurns(turns []Turn) int {
	n := 0
	for i := len(turns) - 1; i >= 0; i-- {
		if len(turns[i].Checks) > 0 {
			break
		}
		n++
	}
	return n
}
```

In `runGenerationLoop`, for round 0 only, if `o.forceToolChoice`, set `ToolChoice: "required"` on the request when the provider's tool capability is `yes`, else append the nudge line to the prompt:

```
"The last {n} turns resolved without a check; if this action carries any consequence, call request_check before narrating."
```

Reset `forceToolChoice` at the end of the turn so it never applies twice.

- [x] **Step 4: Run tests and commit**

Run: `go test ./pkg/engine/`

```bash
git add pkg/engine/orchestrator.go pkg/engine/cadence_test.go
git commit -m "feat(mechanics): force a check when the model goes quiet"
```

---

### Task 6: `ask` — propose and wait

**Files:**
- Modify: `pkg/harness/turn.go` (add `PendingCheck`)
- Modify: `pkg/harness/turn_tools.go` (`propose_check` handling helper, e.g. `ParseCheckRequest` already parses it)
- Modify: `pkg/engine/orchestrator.go` (dispatch `propose_check`; end the turn pending; resolve a pending roll)
- Modify: `pkg/engine/history.go` (`Turn.PendingCheck`)
- Modify: `pkg/gui/types.go` (`TurnRequest.PendingCheckRef`)
- Test: `pkg/engine/pending_check_test.go` (create)

**Interfaces:**
- Produces: `harness.PendingCheck{Ref, Request, ProposedBy}`; `history.Turn.PendingCheck`; `TurnRequest.PendingCheckRef`; a `propose_check` tool that ends the turn pending; a roll that resolves it and continues.

- [x] **Step 1: Write the failing test**

Create `pkg/engine/pending_check_test.go`:

```go
func TestProposeCheckEndsTheTurnPending(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "p1", Name: "propose_check", Arguments: `{"actor":"player","check_kind":"skill","stakes":"the bridge","outcomes":{"pass":"cross","fail":"fall"}}`}}},
	}}
	o, recorder := toolLoopOrchestrator(t, provider)
	o.SetMechanicsEngagement("ask")
	o.SetTools(&fakeExecutor{}, "yes")
	turn, err := o.ProcessActionStream(context.Background(), "Do", "cross the rope bridge", nil)
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if turn.PendingCheck == nil || turn.PendingCheck.Ref == "" {
		t.Fatalf("expected a pending check, got %+v", turn.PendingCheck)
	}
	_ = recorder
}

func TestRollingAPendingCheckResolvesAndContinues(t *testing.T) {
	// Seed history with a pending check, then run a turn carrying PendingCheckRef.
	// Assert the resulting turn has a resolved check and no PendingCheck.
}
```

Fill the second test by writing a turn with `PendingCheck` to history, then invoking `ProcessActionStream` with a request whose `PendingCheckRef` matches; assert `turn.Checks` is non-empty and `turn.PendingCheck == nil`.

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestProposeCheck|TestRollingAPendingCheck' ./pkg/engine/ -v`
Expected: FAIL.

- [x] **Step 3: Implement pending checks**

`pkg/harness/turn.go`:

```go
// PendingCheck is a check the GM proposed and the player has not yet rolled.
type PendingCheck struct {
	Ref        string       `json:"ref"`
	Request    CheckRequest `json:"request"`
	ProposedBy string       `json:"proposed_by"`
}
```

`pkg/engine/history.go`: add `PendingCheck *harness.PendingCheck \`json:"pending_check,omitempty"\`` to `Turn`.

`pkg/harness/turn_tools.go`: `propose_check` shares `ParseCheckRequest`.

`pkg/engine/orchestrator.go`:

- In the dispatch switch, `case "propose_check":` parse the request; when the policy is `ask`, build `pending = &harness.PendingCheck{Ref: call.ID, Request: *req, ProposedBy: "gm"}` and return a `streamResult` with `PendingCheck: pending` (add the field to `streamResult`), ending the turn.
- Assign `turn.PendingCheck = result.PendingCheck` when building the turn, and skip the "uncertain needs a check" validation for a pending turn.
- At turn start, if the request carries `PendingCheckRef`, load the referenced pending check from history, resolve it through `o.resolveCheck`, carry the resolved check into generation as `checks`, and clear the pending flag on the new turn.

`pkg/gui/types.go` and `TurnRequest.validate`: carry `PendingCheckRef` through to the orchestrator (`ProcessActionStream` needs a variant that accepts it, or the service sets it on the orchestrator before the call; choose the latter to avoid changing the hot signature: `orchestrator.SetPendingCheckRef(ref)`).

- [x] **Step 4: Run tests and commit**

Run: `go test ./pkg/engine/ ./pkg/gui/`

```bash
git add pkg/harness/turn.go pkg/harness/turn_tools.go pkg/engine/orchestrator.go pkg/engine/history.go pkg/engine/pending_check_test.go pkg/gui/types.go pkg/gui/service.go
git commit -m "feat(mechanics): let the GM propose a check and the player roll it"
```

---

### Task 7: Visibility

**Files:**
- Modify: `pkg/gui/types.go` (`GameStateDTO` add `MechanicsEngagement`)
- Modify: `pkg/gui/service.go` (populate it)
- Modify: `frontend/src/types.ts`, `frontend/src/components/TurnSegments.tsx`, `frontend/src/components/DiceCheckCard.tsx`, `frontend/src/components/ActionConsole.tsx` (or a new strip component), `frontend/src/App.tsx`

**Interfaces:**
- Consumes: `TurnDTO.Checks`, `SegmentDTO.CheckRef`, `GameStateDTO.MechanicsEngagement`.

- [x] **Step 1: Add the engagement to the DTO**

`pkg/gui/types.go`: `MechanicsEngagement string \`json:"mechanics_engagement,omitempty"\`` on `GameStateDTO`. In `GetGameState`, set it from `engine.ResolveEngagement(gameManifest, systemManifest, cfg)`.

- [x] **Step 2: Causal placement**

In `frontend/src/components/TurnSegments.tsx`, when a check has no referencing segment, insert it **before** the first narration segment rather than after the prose (change the append at the end of the stream builder to an unshift at the start).

- [x] **Step 3: Richer check card**

In `DiceCheckCard.tsx`, render the `stakes` line, the outcome label (`check.outcome`), and the notation (`check.roll?.notation`), keeping the tone colours.

- [x] **Step 4: Mechanics strip and header indicator**

Add a small component (e.g. `MechanicsStrip.tsx`) rendered above `ActionConsole` showing `engagement` and the turn's check count/outcome; add a header chip/dot bound to `gameState.mechanics_engagement`.

- [x] **Step 5: Verify and commit**

Run: `mise run test:frontend && mise run build`

```bash
git add pkg/gui/types.go pkg/gui/service.go frontend/src
git commit -m "feat(mechanics): surface engagement and check outcomes"
```

---

### Task 8: Full verification

- [x] **Step 1:** `mise run test`
- [x] **Step 2:** `mise run lint && mise run build`
- [x] **Step 3:** Manual: play a turn in each policy; confirm prompt/tools/validation change, a cadence check fires, and an `ask` turn ends pending and rolls through.

---

## Self-Review Notes

- Spec coverage: policy → Task 1; prompt → Task 2; tools/validation → Task 3; forcing plumbing → Task 4; cadence floor → Task 5; `ask` two-phase → Task 6; visibility → Task 7.
- Type consistency: `MechanicsConfig`, `ResolveEngagement`, `FormatMechanicsInstructions(spec, engagement)`, `TurnToolSpecsFor`, `GenerateRequest.ToolChoice`, `PendingCheck`, `TurnRequest.PendingCheckRef`, `MechanicsEngagement` are used consistently.
- `ask` is the largest task and is deliberately isolated; it can ship after 1-5.
