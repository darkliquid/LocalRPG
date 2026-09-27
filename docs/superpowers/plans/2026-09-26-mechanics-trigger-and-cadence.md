# Mechanics Trigger & Cadence Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a system's mechanics run on every relevant turn and make the GM actually call checks.

**Architecture:** The GUI rebuilds its `rules.JSEngine` every turn but loads `mechanics.js` only once, so hooks die after turn 1 — fix the lifecycle. Then add an engine-generated "when to roll" instruction to the GM prompt, and make the player's explicit Roll a structured `ProposedCheck` the submission must resolve or dismiss.

**Tech Stack:** Go 1.27 (stdlib `testing` only), `pkg/rules` (goja JS), `pkg/harness` prompt assembly, `pkg/engine` orchestrator, GUI service. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-26-mechanics-trigger-and-cadence-design.md`

## Global Constraints

- Go tests use only `testing` and `t.TempDir()`; no testify, no mock libraries.
- Use `interface{}`, never `any`; wrap errors with `fmt.Errorf("...: %w", err)`; `go vet ./...` must stay clean.
- Do not add dependencies.
- Conventional Commits with a scope; subject under 72 chars.
- `systems/` is gitignored, so edits there are local-only and never staged. Tracked template changes live in `frontend/src/templates/referenceTemplates.ts`.
- Spec refinement for this plan: a system "ships mechanics" when `systems/<id>/mechanics.js` exists **or** its `system.yaml` has a `mechanics` block. The instruction is generated in either case.
- Verification commands: `mise run test` (go + tsc), `mise run lint` (go vet), `mise run build`.

---

### Task 1: Load mechanics rules on every turn

**Files:**
- Modify: `pkg/gui/service.go` (struct field ~line 59-62, init ~line 120, prepareTurn ~line 1190-1205)
- Test: `pkg/gui/mechanics_turn_test.go` (create)

**Interfaces:**
- Consumes: `rules.NewRuleLoader(paths, jsEngine)`, `loader.LoadRules(systemID, worldID)` (existing).
- Produces: nothing new; the per-turn `JSEngine` now has hooks registered.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/mechanics_turn_test.go`:

```go
package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mechanicsFixture is turnFixture plus a system script that reports it ran for
// "do" actions, so a test can observe whether the hook was registered.
func mechanicsFixture(t *testing.T) (string, *Service) {
	t.Helper()
	gameID, svc := turnFixture(t)
	path := filepath.Join(svc.GetResolver().SystemDir("freeform"), "mechanics.js")
	script := `onAction("do", function(ctx) { return { success: true, outcome: "system-ran" }; });`
	if err := os.WriteFile(path, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	return gameID, svc
}

func TestMechanicsHookRunsOnEveryTurn(t *testing.T) {
	gameID, svc := mechanicsFixture(t)

	for turnNumber := 1; turnNumber <= 2; turnNumber++ {
		session, err := svc.BeginTurn(gameID)
		if err != nil {
			t.Fatalf("BeginTurn %d failed: %v", turnNumber, err)
		}
		var outcome string
		err = session.Run(context.Background(), TurnRequest{Mode: "Do", Input: "I try something risky"}, func(ev TurnEvent) error {
			if ev.Type == "turn" && ev.Turn != nil {
				outcome = ev.Turn.Outcome
			}
			return nil
		})
		session.Close()
		if err != nil {
			t.Fatalf("turn %d failed: %v", turnNumber, err)
		}
		if outcome != "system-ran" {
			t.Errorf("turn %d outcome = %q, want system-ran (mechanics hook did not run)", turnNumber, outcome)
		}
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestMechanicsHookRunsOnEveryTurn ./pkg/gui/ -v`
Expected: FAIL on turn 2 with `outcome = ""` (turn 1 passes; the VM is empty afterwards).

- [x] **Step 3: Remove the once-per-campaign guard**

In `pkg/gui/service.go`, delete the `rulesLoaded` field and the `rulesMu` mutex (they exist only for this guard):

```go
	// rulesLoaded records campaigns whose mechanics.js has been loaded, so a
	// campaign's script is parsed once. Remove this comment and the field.
	rulesMu     sync.Mutex
	rulesLoaded map[string]bool
```

and in `NewService` remove `rulesLoaded: make(map[string]bool),`.

Then replace the guarded load in `prepareTurn`:

```go
	// The VM is rebuilt every turn, so its hooks must be re-registered every
	// turn. Loading once per campaign left every turn after the first running
	// with an empty mechanics VM and no hooks.
	loader := rules.NewRuleLoader(s.resolver, jsEngine)
	if err := loader.LoadRules(manifest.SystemID, manifest.WorldID); err != nil {
		logger.Event("rules.load_error", map[string]interface{}{"error": err.Error()})
	}
```

Leave `sync` imported (other mutexes in the file still use it). Confirm with `go build ./pkg/gui/`.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestMechanicsHookRunsOnEveryTurn ./pkg/gui/ -v`
Expected: PASS (both turns report `system-ran`).

- [x] **Step 5: Run the package and the full suite**

Run: `go test ./pkg/gui/` then `mise run test`
Expected: all pass.

- [x] **Step 6: Commit**

```bash
git add pkg/gui/service.go pkg/gui/mechanics_turn_test.go
git commit -m "fix(rules): reload mechanics hooks on every turn"
```

---

### Task 2: Engine-generated "when to roll" instruction

**Files:**
- Create: `pkg/harness/mechanics_instructions.go`
- Modify: `pkg/harness/context.go` (ContextRequest struct ~line 75-100; section list ~line 260-273)
- Test: `pkg/harness/mechanics_instructions_test.go` (create)

**Interfaces:**
- Produces: `harness.FormatMechanicsInstructions(spec *core.MechanicsSpec) string`; `harness.ContextRequest.MechanicsPrompt string`.
- Consumes: `core.MechanicsSpec` (`pkg/core/mechanics.go`).

- [x] **Step 1: Write the failing tests**

Create `pkg/harness/mechanics_instructions_test.go`:

```go
package harness

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestFormatMechanicsInstructions(t *testing.T) {
	generic := FormatMechanicsInstructions(nil)
	if !strings.Contains(generic, "## RESOLVING UNCERTAINTY") {
		t.Fatalf("missing header in %q", generic)
	}
	if !strings.Contains(generic, "request_check") {
		t.Fatalf("missing request_check guidance in %q", generic)
	}

	declared := FormatMechanicsInstructions(&core.MechanicsSpec{
		Checks: core.CheckConventions{
			Notation:   "2d6",
			Outcome:    []string{"strong", "weak", "miss"},
			Difficulty: []core.DifficultySpec{{ID: "standard", Label: "Standard", Target: 8}},
		},
	})
	for _, want := range []string{"2d6", "strong, weak, miss", "Standard 8"} {
		if !strings.Contains(declared, want) {
			t.Errorf("declared instruction missing %q: %q", want, declared)
		}
	}
}

func TestContextRendersMechanicsPrompt(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t, tempDir)
	assembler := NewContextAssembler(store)

	withPrompt, err := assembler.Assemble(ContextRequest{
		LocationID:      "loc1",
		PlayerID:        "p1",
		Action:          "I leap the gap",
		MechanicsPrompt: "## RESOLVING UNCERTAINTY\nRoll when it matters.\n",
	})
	if err != nil {
		t.Fatalf("assemble failed: %v", err)
	}
	if !strings.Contains(withPrompt.Prompt, "Roll when it matters.") {
		t.Fatalf("mechanics prompt not rendered:\n%s", withPrompt.Prompt)
	}

	withoutPrompt, err := assembler.Assemble(ContextRequest{LocationID: "loc1", PlayerID: "p1", Action: "I wait"})
	if err != nil {
		t.Fatalf("assemble failed: %v", err)
	}
	if strings.Contains(withoutPrompt.Prompt, "RESOLVING UNCERTAINTY") {
		t.Fatalf("mechanics prompt rendered when empty")
	}
}
```

If no `newTestStore` helper exists in the package, inline the store setup used by `TestContextAssembler` (`pkg/harness/context_test.go:15-21`): `storage.NewStore(filepath.Join(tempDir, "index.db"))` with `defer store.Close()`.

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestFormatMechanicsInstructions|TestContextRendersMechanicsPrompt' ./pkg/harness/ -v`
Expected: FAIL — `FormatMechanicsInstructions` undefined; `ContextRequest` has no field `MechanicsPrompt`.

- [x] **Step 3: Add the formatter**

Create `pkg/harness/mechanics_instructions.go`:

```go
package harness

import (
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
)

// FormatMechanicsInstructions is the engine's standing instruction to the GM
// about when to roll. It is generated whenever a system ships mechanics, and it
// cites the system's declared resolution when it has one. It is deliberately
// system-agnostic: the engine still knows no rules, only how to ask for a check.
func FormatMechanicsInstructions(spec *core.MechanicsSpec) string {
	var sb strings.Builder
	sb.WriteString("## RESOLVING UNCERTAINTY\n")
	sb.WriteString("Call request_check when an action is uncertain and failure would change the story. ")
	sb.WriteString("State the stakes and the possible outcomes first. Do not roll for safe or trivial actions. ")
	sb.WriteString("NPCs do not roll; resolve opposition through the protagonist's check. ")
	sb.WriteString("Honour the outcome the engine returns.\n")

	if spec == nil {
		return sb.String()
	}
	if notation := strings.TrimSpace(spec.Checks.Notation); notation != "" {
		sb.WriteString("Default notation: " + notation + ".\n")
	}
	if len(spec.Checks.Outcome) > 0 {
		sb.WriteString("Outcome vocabulary: " + strings.Join(spec.Checks.Outcome, ", ") + ".\n")
	}
	if len(spec.Checks.Difficulty) > 0 {
		parts := make([]string, 0, len(spec.Checks.Difficulty))
		for _, d := range spec.Checks.Difficulty {
			label := d.Label
			if label == "" {
				label = d.ID
			}
			parts = append(parts, label+" "+strconv.Itoa(d.Target))
		}
		sb.WriteString("Difficulties: " + strings.Join(parts, ", ") + ".\n")
	}
	return sb.String()
}
```

- [x] **Step 4: Add the request field and render it**

In `pkg/harness/context.go`, add to `ContextRequest` (next to `LorePrompt`):

```go
	// MechanicsPrompt is the engine's instruction on when to roll, generated
	// from the loaded system. Empty when the system ships no mechanics.
	MechanicsPrompt string
```

Add the section renderer next to `rulesSection`:

```go
func mechanicsSection(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	return strings.TrimSpace(prompt) + "\n\n"
}
```

Add the section to the returned slice, immediately after `rules`:

```go
		{name: "mechanics", source: "mechanics_prompt", text: mechanicsSection(req.MechanicsPrompt)},
```

- [x] **Step 5: Run tests to verify they pass**

Run: `go test -run 'TestFormatMechanicsInstructions|TestContextRendersMechanicsPrompt' ./pkg/harness/ -v`
Expected: PASS.

- [x] **Step 6: Run the harness package**

Run: `go test ./pkg/harness/`
Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add pkg/harness/mechanics_instructions.go pkg/harness/mechanics_instructions_test.go pkg/harness/context.go
git commit -m "feat(harness): tell the GM when to call a check"
```

---

### Task 3: Load and pass the mechanics instruction from the orchestrator

**Files:**
- Modify: `pkg/engine/orchestrator.go` (struct ~line 75-113; LoadPrompts ~line 387-401; Assemble call ~line 641-657)
- Test: `pkg/engine/mechanics_prompt_test.go` (create)

**Interfaces:**
- Consumes: `harness.FormatMechanicsInstructions`, `core.LoadSystemManifest`, `core.PathResolver`.
- Produces: `TurnOrchestrator.mechanicsPrompt` (unexported), forwarded as `ContextRequest.MechanicsPrompt`.

- [x] **Step 1: Write the failing tests**

Create `pkg/engine/mechanics_prompt_test.go`:

```go
package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestLoadPromptsBuildsMechanicsInstruction(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	sysDir := paths.SystemDir("sys")
	if err := os.MkdirAll(filepath.Join(sysDir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "mechanics.js"), []byte(`onAction("do", function(ctx) { return {}; });`), 0644); err != nil {
		t.Fatal(err)
	}
	systemYAML := "id: sys\nname: Sys\nmechanics:\n  checks:\n    notation: 2d6\n    outcome: [pass, fail]\n"
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte(systemYAML), 0644); err != nil {
		t.Fatal(err)
	}

	o := &TurnOrchestrator{}
	o.LoadPrompts(paths, "sys", "")

	if !strings.Contains(o.mechanicsPrompt, "## RESOLVING UNCERTAINTY") {
		t.Fatalf("mechanics prompt not built: %q", o.mechanicsPrompt)
	}
	if !strings.Contains(o.mechanicsPrompt, "2d6") {
		t.Errorf("mechanics prompt missing declared notation: %q", o.mechanicsPrompt)
	}
}

func TestLoadPromptsOmitsMechanicsWhenNoneShipped(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	sysDir := paths.SystemDir("plain")
	if err := os.MkdirAll(filepath.Join(sysDir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: plain\nname: Plain\n"), 0644); err != nil {
		t.Fatal(err)
	}

	o := &TurnOrchestrator{}
	o.LoadPrompts(paths, "plain", "")

	if o.mechanicsPrompt != "" {
		t.Errorf("mechanics prompt = %q, want empty", o.mechanicsPrompt)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestLoadPromptsBuildsMechanicsInstruction|TestLoadPromptsOmitsMechanicsWhenNoneShipped' ./pkg/engine/ -v`
Expected: FAIL — `o.mechanicsPrompt` undefined.

- [x] **Step 3: Add the field and detection**

In `pkg/engine/orchestrator.go`, add to the `TurnOrchestrator` struct (next to `rulesPrompt`):

```go
	mechanicsPrompt  string
```

Extend `LoadPrompts`:

```go
func (o *TurnOrchestrator) LoadPrompts(paths *core.PathResolver, systemID, worldID string) {
	if paths != nil {
		if systemID != "" {
			sysDir := paths.SystemDir(systemID)
			if data, err := os.ReadFile(filepath.Join(sysDir, "prompts", "rules.md")); err == nil {
				o.rulesPrompt = string(data)
			}
			// A system ships mechanics when it has a script or a declarative
			// block. Either way the GM needs to know when to call a check.
			var spec *core.MechanicsSpec
			if manifest, err := core.LoadSystemManifest(filepath.Join(sysDir, "system.yaml")); err == nil {
				spec = manifest.Mechanics
			}
			_, scriptErr := os.Stat(filepath.Join(sysDir, "mechanics.js"))
			if scriptErr == nil || spec != nil {
				o.mechanicsPrompt = harness.FormatMechanicsInstructions(spec)
			}
		}
		if worldID != "" {
			if data, err := os.ReadFile(filepath.Join(paths.WorldDir(worldID), "prompts", "lore.md")); err == nil {
				o.lorePrompt = string(data)
			}
		}
	}
}
```

- [x] **Step 4: Pass it into the assembled request**

In the `ContextRequest{...}` literal passed to `o.assembler.Assemble...` (around line 641-657), add:

```go
		MechanicsPrompt:  o.mechanicsPrompt,
```

- [x] **Step 5: Run tests to verify they pass**

Run: `go test -run 'TestLoadPrompts' ./pkg/engine/ -v`
Expected: PASS.

- [x] **Step 6: Run the package and vet**

Run: `go test ./pkg/engine/ && mise run lint`
Expected: PASS, vet clean.

- [x] **Step 7: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/mechanics_prompt_test.go
git commit -m "feat(engine): load mechanics guidance into the turn prompt"
```

---

### Task 4: Enforce the player's proposed check

**Files:**
- Modify: `pkg/harness/turn.go` (types)
- Modify: `pkg/engine/orchestrator.go` (Roll branch ~line 552-560; `gmDirective` decl ~line 433; `runGenerationLoop` signature ~line 1221 and call ~line 672; validate call ~line 1500)
- Modify: `pkg/engine/submission.go` (`validateSubmission` signature and rule)
- Test: `pkg/harness/turn_test.go` (append), `pkg/engine/submission_test.go` (update calls, add cases)

**Interfaces:**
- Produces: `harness.ProposedCheck{ Ref, Actor, Description string }`.
- Changes: `validateSubmission(sub *harness.TurnSubmission, checks []harness.CheckResult, declaredStats map[string]core.StatSpec, proposed *harness.ProposedCheck) error`; `(*TurnOrchestrator).runGenerationLoop(..., proposed *harness.ProposedCheck, onChunk ...)`.

- [x] **Step 1: Write the failing tests**

Append to `pkg/engine/submission_test.go` (and update the five existing `validateSubmission(...)` calls to pass a final `nil`):

```go
func TestValidateSubmissionProposedCheck(t *testing.T) {
	proposed := &harness.ProposedCheck{Ref: "player-roll", Actor: "player", Description: "pick the lock"}

	base := func() *harness.TurnSubmission {
		return &harness.TurnSubmission{
			Verdict:  harness.ActionVerdict{Feasibility: harness.FeasibilityUncertain, Reason: "gap"},
			Segments: []harness.SegmentSpec{{Kind: "narration", Text: "You try."}},
		}
	}

	// Unresolved and undisclosed: rejected.
	if err := validateSubmission(base(), nil, nil, proposed); err == nil {
		t.Fatal("proposed check with no resolution or dismissal should fail")
	}

	// Resolved: accepted.
	resolved := base()
	resolved.Segments[0].CheckRef = "c1"
	if err := validateSubmission(resolved, []harness.CheckResult{{CheckID: "c1"}}, nil, proposed); err != nil {
		t.Fatalf("resolved proposed check rejected: %v", err)
	}

	// Dismissed with the matching ref and a reason: accepted.
	dismissed := base()
	dismissed.Verdict = harness.ActionVerdict{Feasibility: harness.FeasibilityAutomatic, Reason: "safe"}
	dismissed.DismissedChecks = []harness.DismissedCheck{{CheckRef: "player-roll", Reason: "no risk"}}
	if err := validateSubmission(dismissed, nil, nil, proposed); err != nil {
		t.Fatalf("dismissed proposed check rejected: %v", err)
	}

	// Dismissed with the wrong ref: rejected.
	wrongRef := base()
	wrongRef.Verdict = harness.ActionVerdict{Feasibility: harness.FeasibilityAutomatic, Reason: "safe"}
	wrongRef.DismissedChecks = []harness.DismissedCheck{{CheckRef: "other", Reason: "no risk"}}
	if err := validateSubmission(wrongRef, nil, nil, proposed); err == nil {
		t.Fatal("dismissal with the wrong ref should fail")
	}
}

func TestParseProposedCheckJSON(t *testing.T) {
	check := harness.ProposedCheck{Ref: "player-roll", Actor: "sean", Description: "pick the lock"}
	if check.Ref == "" || check.Actor != "sean" {
		t.Fatalf("unexpected zero value: %+v", check)
	}
}
```

Append to `pkg/harness/turn_test.go`:

```go
func TestProposedCheckCarriesItsRef(t *testing.T) {
	check := ProposedCheck{Ref: "player-roll", Actor: "sean", Description: "pick the lock"}
	if check.Ref != "player-roll" || check.Description != "pick the lock" {
		t.Fatalf("unexpected proposed check: %+v", check)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestValidateSubmissionProposedCheck|TestProposedCheckCarriesItsRef' ./pkg/engine/ ./pkg/harness/ -v`
Expected: FAIL — compile errors (`harness.ProposedCheck` undefined; `validateSubmission` takes 3 args).

- [x] **Step 3: Add the type**

In `pkg/harness/turn.go`, after `DismissedCheck`:

```go
// ProposedCheck is a player's explicit request to roll, carried as structured
// data so the engine can require the GM to resolve or dismiss it. Ref is the
// stable id the GM references in dismissed_checks.
type ProposedCheck struct {
	Ref         string `json:"ref"`
	Actor       string `json:"actor,omitempty"`
	Description string `json:"description,omitempty"`
}
```

- [x] **Step 4: Thread it through the orchestrator**

In `pkg/engine/orchestrator.go`, next to `var gmDirective string` (line ~433):

```go
	var proposedCheck *harness.ProposedCheck
```

Replace the Roll branch body (lines ~552-560) so it still renders the same prompt text but records the structure:

```go
	} else if !isOpening && strings.EqualFold(mode, "Roll") {
		// A player-initiated roll is a proposal, not an executed result: the GM
		// either adopts it with request_check or dismisses it. It is carried as
		// structured data so the submission can be held to that.
		proposal := strings.TrimSpace(actionInput)
		if proposal == "" {
			proposal = "a check"
		}
		proposedCheck = &harness.ProposedCheck{Ref: "player-roll", Actor: o.playerID, Description: proposal}
		gmDirective = fmt.Sprintf("[PROPOSED CHECK: %s by %s (ref: %s)]", proposal, o.playerID, proposedCheck.Ref)
	} else if !isOpening && o.rulesEngine != nil {
```

Change the `runGenerationLoop` signature (line ~1221) to accept it:

```go
func (o *TurnOrchestrator) runGenerationLoop(ctx context.Context, assembly *harness.AssembleResult, gmDirective string, proposed *harness.ProposedCheck, onChunk func(string) error) (streamResult, error) {
```

Update the call site (line ~672) to pass `proposedCheck`:

```go
	result, err := o.runGenerationLoop(ctx, &assembly, gmDirective, proposedCheck, onChunk)
```

Update the validation call inside the loop (line ~1500):

```go
					if vErr := validateSubmission(sub, checks, o.declaredStats, proposed); vErr != nil {
```

- [x] **Step 5: Add the enforcement rule**

In `pkg/engine/submission.go`, change the signature and add the rule before the `return nil`:

```go
func validateSubmission(sub *harness.TurnSubmission, checks []harness.CheckResult, declaredStats map[string]core.StatSpec, proposed *harness.ProposedCheck) error {
```

After the existing `DismissedChecks` loop:

```go
	if proposed != nil && sub.Verdict.Feasibility != harness.FeasibilityImpossible {
		resolved := len(checks) > 0
		dismissed := false
		for _, entry := range sub.DismissedChecks {
			if entry.CheckRef == proposed.Ref && strings.TrimSpace(entry.Reason) != "" {
				dismissed = true
			}
		}
		if !resolved && !dismissed {
			return &submissionError{Code: "unresolved_proposed_check", Detail: proposed.Ref}
		}
	}
```

- [x] **Step 6: Run tests to verify they pass**

Run: `go test -run 'TestValidateSubmission|TestProposedCheck' ./pkg/engine/ ./pkg/harness/ -v`
Expected: PASS (existing updated calls plus the new cases).

- [x] **Step 7: Run the packages and vet**

Run: `go test ./pkg/engine/ ./pkg/harness/ && mise run lint`
Expected: PASS, vet clean.

- [x] **Step 8: Commit**

```bash
git add pkg/harness/turn.go pkg/harness/turn_test.go pkg/engine/orchestrator.go pkg/engine/submission.go pkg/engine/submission_test.go
git commit -m "feat(engine): require a roll or dismissal for a proposed check"
```

---

### Task 5: Update the tracked system template

**Files:**
- Modify: `frontend/src/templates/referenceTemplates.ts` (lines 34-108, the `rules_prompt` and `script` strings)
- Local-only (gitignored, do not stage): `systems/narrative_2d6/mechanics.js`, `systems/narrative_2d6/prompts/rules.md`

**Interfaces:**
- Produces: a template whose script hooks the modes clients send (`do`, `say`, `story`) and whose prompt no longer advertises an unreachable `attack` mode.

- [x] **Step 1: Replace the template's `rules_prompt`**

In `frontend/src/templates/referenceTemplates.ts`, replace the `rules_prompt` value (lines 34-54) with:

```ts
  rules_prompt: `You are the Game Master adjudicating a campaign governed by the **Narrative 2d6 Engine**.

### 1. Core Resolution Ladder
Call \`request_check\` when an action is uncertain and failure would change the story; the engine rolls 2d6 and returns the result. Do not roll for safe or trivial actions.
- **10+ (Strong Hit / Full Success)**: The protagonist accomplishes their goal cleanly.
- **7–9 (Weak Hit / Partial Success)**: They succeed at a tangible cost: damage, stress, a complication, a trade-off, or diminished effect.
- **6- (Miss / Hard Move)**: Escalate the threat, introduce a twist, deplete a resource, or put the protagonist in peril.

### 2. Action Modes
- \`do\`: general active intent (physical feats, athletics, stealth, lockpicking).
- \`say\`: social dialogue, persuasion, interrogation, intimidation.
- \`story\`: narrative or reflective action that still carries risk.
- \`roll\`: the player's explicit request for a check; resolve it or state why no roll is needed.

NPCs do not roll; resolve opposition through the protagonist's check.

### 3. Handling Mechanics Results
When the prompt contains a \`[MECHANICS RESULT: ...]\` tag, honour it and weave it into the prose. Never contradict the roll total, damage, or state changes the engine reports.
`,
```

- [x] **Step 2: Replace the template's `script`**

Replace the `script` value (lines 55-108) with a version that hooks the modes clients send (the same body for `do`/`say`/`story`) and drops `attack`:

```ts
  script: `// ==========================================
// Narrative 2d6 Engine - Mechanics Script
// ==========================================

function resolve2d6(ctx) {
  var r = roll("2d6");
  var message = "";
  if (r.total >= 10) {
    message = "Strong Hit (Total: " + r.total + ") - complete triumph, no complications.";
  } else if (r.total >= 7) {
    message = "Weak Hit (Total: " + r.total + ") - success at a cost or complication.";
  } else {
    message = "Miss (Total: " + r.total + ") - the attempt falters; danger escalates.";
    injectGMDirection("The action failed. Introduce an immediate complication or escalate danger.");
  }
  return { success: r.total >= 7, message: message, roll: r };
}

onAction("do", resolve2d6);
onAction("say", resolve2d6);
onAction("story", resolve2d6);

onTurnEnd(function(ctx) {
  log("Turn " + ctx.turn + " completed in Narrative 2d6 Engine.");
});
`,
```

Verify the JS engine supports passing a named function to `onAction` (it stores the value and calls it); if it requires an inline function literal, wrap each call as `onAction("do", function(ctx) { return resolve2d6(ctx); });`.

- [x] **Step 3: Mirror the change into the local system (not committed)**

Update `systems/narrative_2d6/mechanics.js` and `systems/narrative_2d6/prompts/rules.md` with the same content. Confirm they are ignored: `git status --short` must not list either.

- [x] **Step 4: Typecheck and build the frontend**

Run: `mise run test:frontend && mise run build:frontend`
Expected: PASS.

- [x] **Step 5: Commit (tracked file only)**

```bash
git add frontend/src/templates/referenceTemplates.ts
git commit -m "docs(systems): hook do/say/story and drop the unreachable attack mode"
```

---

### Task 6: Full verification

**Files:** none.

- [x] **Step 1: Run the whole suite**

Run: `mise run test`
Expected: PASS (Go tests + `tsc --noEmit`).

- [x] **Step 2: Vet and build**

Run: `mise run lint && mise run build`
Expected: clean, binary built.

- [x] **Step 3: Manual smoke (optional but recommended)**

Run: `mise run dev:gui`, play three `Do` turns against `narrative_2d6`, and confirm a mechanics result appears on each, plus a `Roll` turn yields a check or a stated dismissal.

- [x] **Step 4: Commit any remaining changes**

```bash
git status --short
# If anything tracked remains, add and commit it.
```

---

## Deferred (spec D6)

The optional cadence nudge (force a check after N turns without one) is **not** in this plan. It is a pacing enhancement behind a config key, default off, and nothing else depends on it. A follow-up spec/plan can add it without touching the work above.

## Self-Review Notes

- Spec coverage: D1 → Task 1; D2 → Task 5 (mode contract is a hook/template change, no engine change — verified: `Do`/`Say`/`Story` already reach `ExecuteAction`); D3 → Tasks 2-3; D4 → Task 4; D5 → Task 1 (hook path for non-tool providers); D7 → Task 5; D6 → Deferred.
- Type consistency: `harness.MechanicsPrompt`, `harness.ProposedCheck`, `FormatMechanicsInstructions`, `validateSubmission(..., proposed *harness.ProposedCheck)`, and `runGenerationLoop(..., proposed *harness.ProposedCheck, ...)` are used consistently across tasks.
- No placeholders: every code and test step contains the actual content.
