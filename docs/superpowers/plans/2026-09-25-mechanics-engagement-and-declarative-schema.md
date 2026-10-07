# Mechanics Engagement & Declarative Schema Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an optional declarative `mechanics` schema to `system.yaml`, expose it to `mechanics.js`, resolve checks through schema-or-js, apply the GM's `state_changes` authoritatively, add `onTurnBegin`, enrich `onTurnEnd`, and fix the GUI so it loads rules.

**Architecture:** `core.SystemManifest` gains an optional `MechanicsSpec`. The rules package reads it through the host API and resolves checks: a js `onCheck` resolver wins, else a declared convention, else a `2d6` pass/fail default. State changes and health effects flow through the host `SetStat`/`EntityWriter` so Markdown stays canonical. `pkg/gui` builds a `RuleLoader` and loads rules for a campaign.

**Tech Stack:** Go 1.27.1 (`pkg/core`, `pkg/rules`, `pkg/engine`, `pkg/gui`, `cmd/localrpg`), goja JS runtime.

**Spec:** `docs/superpowers/specs/2026-09-25-mechanics-engagement-and-declarative-schema-design.md`
**Depends on:** Structured Turn Protocol plan (`engine.CheckResolver`, `state_changes` in `TurnSubmission`).

## Global Constraints

- Go 1.27.1. Standard library only for tests; no testify.
- Use `any`, not `interface{}`; `go vet ./...` clean.
- The engine stays schema-agnostic; the schema lives in `pkg/core` and is consumed through interfaces.
- `mechanics` and every sub-field are optional; a nil `Mechanics` preserves today's behaviour exactly.
- `state.State` remains a free-form dotted-path map; only `mechanics.js` or a declared schema constrain it.
- The GUI rules-loading fix must not change campaigns that have no `mechanics.js`.
- Conventional Commits with a scope; subject under 72 characters.

---

## File Map

**Create**
- `pkg/core/mechanics.go` — `MechanicsSpec`, `StatSpec`, `SkillSpec`, `HealthSpec`, `CheckConventions`, `DifficultySpec`.
- `pkg/core/mechanics_test.go`.
- `pkg/rules/resolver.go` — `SchemaResolver` implementing the engine `CheckResolver` shape, and the default convention.
- `pkg/rules/state_changes.go` — `ApplyStateChanges`.
- `pkg/rules/resolver_test.go`, `pkg/rules/state_changes_test.go`.

**Modify**
- `pkg/core/types.go` — `SystemManifest.Mechanics`.
- `pkg/rules/host_api.go` — schema accessors on `GameHostAPI`/`DefaultHostBridge`.
- `pkg/rules/js_engine.go` — `onCheck`, `onTurnBegin`, enriched `ExecuteTurnEnd`.
- `pkg/engine/orchestrator.go` — apply state changes; `onTurnBegin`; pass checks to `onTurnEnd`.
- `pkg/engine/check_resolver.go` — default resolver delegates to the rules schema resolver when available.
- `pkg/gui/service.go` — load rules for a campaign.
- `pkg/gui/asset` no change; `cmd/localrpg/play.go` unchanged.

---

### Task 1: Declarative schema types

**Files:**
- Create: `pkg/core/mechanics.go`
- Modify: `pkg/core/types.go`
- Test: `pkg/core/mechanics_test.go`

**Interfaces:**
- Produces: `MechanicsSpec`, `StatSpec`, `SkillSpec`, `HealthSpec`, `CheckConventions`, `DifficultySpec`; `SystemManifest.Mechanics *MechanicsSpec`.

- [x] **Step 1: Write the failing test**

```go
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSystemManifestMechanics(t *testing.T) {
	dir := t.TempDir()
	yaml := `id: narrative
name: Narrative
mechanics:
  stats:
    - {id: might, type: number, default: 0}
  skills:
    - {id: athletics, label: Athletics, stat: might}
  health:
    stat: hp
    max_stat: hp_max
  checks:
    notation: "2d6"
    outcome: [pass, fail]
    difficulty:
      - {id: hard, target: 10}
`
	path := filepath.Join(dir, "system.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadSystemManifest(path)
	if err != nil {
		t.Fatalf("LoadSystemManifest: %v", err)
	}
	if m.Mechanics == nil || len(m.Mechanics.Stats) != 1 || m.Mechanics.Stats[0].ID != "might" {
		t.Fatalf("mechanics = %+v", m.Mechanics)
	}
	if m.Mechanics.Checks.Notation != "2d6" || len(m.Mechanics.Checks.Difficulty) != 1 {
		t.Fatalf("checks = %+v", m.Mechanics.Checks)
	}
}

func TestLoadSystemManifestWithoutMechanics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "system.yaml")
	if err := os.WriteFile(path, []byte("id: plain\nname: Plain\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadSystemManifest(path)
	if err != nil {
		t.Fatalf("LoadSystemManifest: %v", err)
	}
	if m.Mechanics != nil {
		t.Fatalf("mechanics = %+v, want nil", m.Mechanics)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestLoadSystemManifestMechanics|TestLoadSystemManifestWithoutMechanics' ./pkg/core/`
Expected: FAIL.

- [x] **Step 3: Implement**

`pkg/core/mechanics.go` with the structs from spec §3; add `Mechanics *MechanicsSpec \`yaml:"mechanics,omitempty"\`` to `SystemManifest`. All fields optional.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestLoadSystemManifestMechanics|TestLoadSystemManifestWithoutMechanics' ./pkg/core/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/core/mechanics.go pkg/core/types.go pkg/core/mechanics_test.go
git commit -m "feat(core): add the optional mechanics schema"
```

---

### Task 2: Host API schema exposure

**Files:**
- Modify: `pkg/rules/host_api.go`, `pkg/rules/loader.go`
- Test: `pkg/rules/host_api_test.go`

**Interfaces:**
- Produces: `GameHostAPI.ListStats() []core.StatSpec`, `ListSkills() []core.SkillSpec`, `CheckConventions() core.CheckConventions`; `(*RuleLoader).SetManifest(m *core.SystemManifest)`; `DefaultHostBridge` implements the accessors from a stored manifest.

- [x] **Step 1: Write the failing test**

```go
func TestHostBridgeExposesSchema(t *testing.T) {
	bridge := NewHostBridge(nil, nil, "player")
	bridge.SetManifest(&core.SystemManifest{Mechanics: &core.MechanicsSpec{
		Stats: []core.StatSpec{{ID: "might"}},
		Skills: []core.SkillSpec{{ID: "athletics", Stat: "might"}},
	}})
	if got := bridge.ListStats(); len(got) != 1 || got[0].ID != "might" {
		t.Fatalf("ListStats = %+v", got)
	}
	if got := bridge.ListSkills(); len(got) != 1 || got[0].Stat != "might" {
		t.Fatalf("ListSkills = %+v", got)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestHostBridgeExposesSchema ./pkg/rules/`
Expected: FAIL.

- [x] **Step 3: Implement**

- Add the three accessors to `GameHostAPI` and `DefaultHostBridge` (nil manifest → empty results).
- `RuleLoader.LoadRules` gains the manifest (from `core.LoadSystemManifest`) and calls `SetManifest` before loading scripts, so `mechanics.js` can read the schema during load.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestHostBridgeExposesSchema ./pkg/rules/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/rules/host_api.go pkg/rules/loader.go pkg/rules/host_api_test.go
git commit -m "feat(rules): expose the mechanics schema to mechanics.js"
```

---

### Task 3: Schema and js check resolution

**Files:**
- Create: `pkg/rules/resolver.go`
- Modify: `pkg/rules/js_engine.go`
- Test: `pkg/rules/resolver_test.go`

**Interfaces:**
- Consumes: `core.CheckConventions`, `rules.RollResult`, `entity.Entity`.
- Produces: `type SchemaResolver struct { engine *JSEngine; conventions core.CheckConventions }` with `Resolve(ctx, req, actor) (*harness.CheckResult, error)`; `(*JSEngine).onCheck` binding; `(*JSEngine).ResolveCheck(ctx, req, actor)`.

- [x] **Step 1: Write the failing test**

```go
func TestSchemaResolverUsesDifficulty(t *testing.T) {
	conventions := core.CheckConventions{
		Notation: "1d6",
		Outcome:  []string{"pass", "fail"},
		Difficulty: []core.DifficultySpec{{ID: "trivial", Target: 1}, {ID: "hard", Target: 10}},
	}
	r := SchemaResolver{conventions: conventions}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{CheckKind: "skill", Difficulty: "trivial", Outcomes: map[string]string{"pass": "ok", "fail": "no"}}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != "pass" || res.Roll == nil {
		t.Fatalf("res = %+v", res)
	}
}

func TestJSCheckResolverOverridesSchema(t *testing.T) {
	engine := NewJSEngine(NewHostBridge(nil, nil, "player"))
	if err := engine.LoadScript(`onCheck("luck", function(req){ return {outcome:"pass"}; });`); err != nil {
		t.Fatalf("LoadScript: %v", err)
	}
	res, err := engine.Resolve(context.Background(), harness.CheckRequest{CheckKind: "luck", Outcomes: map[string]string{"pass": "ok"}}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != "pass" {
		t.Fatalf("outcome = %q", res.Outcome)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestSchemaResolverUsesDifficulty|TestJSCheckResolverOverridesSchema' ./pkg/rules/`
Expected: FAIL.

- [x] **Step 3: Implement**

- Bind `onCheck(kind, fn)` in `NewJSEngine`, storing resolvers in a `checkResolvers` map (mirroring `actionHandlers`).
- `JSEngine.Resolve(ctx, req, actor)`: if a resolver exists for `kind`, call it with the request as a JS object, normalise `{outcome, roll, breakdown}` into `harness.CheckResult`; else use `SchemaResolver.Resolve`. (`Resolve` is the method name that satisfies `engine.CheckResolver`.)
- `SchemaResolver.Resolve`: roll `conventions.Notation` (or the request's notation), add the actor's stat bonus when a skill/stat is named and declared, compare to the matched difficulty target, and map to a declared outcome (`pass` when total ≥ target, else the last declared outcome or `fail`). Assign a `CheckID`.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestSchemaResolverUsesDifficulty|TestJSCheckResolverOverridesSchema' ./pkg/rules/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/rules/resolver.go pkg/rules/js_engine.go pkg/rules/resolver_test.go
git commit -m "feat(rules): resolve checks by schema with a js override"
```

---

### Task 4: Wire the resolver into the engine

**Files:**
- Modify: `pkg/engine/check_resolver.go`, `pkg/engine/orchestrator.go`
- Test: `pkg/engine/check_wiring_test.go`

**Interfaces:**
- Consumes: `rules.JSEngine` (satisfying `engine.CheckResolver`).
- Produces: the orchestrator prefers a `rules`-backed resolver when the rules engine is set, else `defaultCheckResolver`.

- [x] **Step 1: Write the failing test**

```go
func TestOrchestratorPrefersRulesResolver(t *testing.T) {
	// A rules engine loaded with an onCheck resolver for "luck" must be used
	// by ProcessActionStream's request_check, not the default resolver.
}
```

Assert the recorded `CheckResult.Outcome` matches the js resolver's hardcoded value.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestOrchestratorPrefersRulesResolver ./pkg/engine/`
Expected: FAIL.

- [x] **Step 3: Implement**

- When `o.rulesEngine != nil`, set the check resolver to it via `SetCheckResolver(o.rulesEngine)` (the JSEngine's `Resolve` method satisfies `engine.CheckResolver`); otherwise keep `defaultCheckResolver{}`.
- Do this in the same place the orchestrator already wires `rulesEngine`.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestOrchestratorPrefersRulesResolver ./pkg/engine/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/check_resolver.go pkg/engine/orchestrator.go pkg/engine/check_wiring_test.go
git commit -m "feat(engine): use the rules check resolver when available"
```

---

### Task 5: Apply state changes

**Files:**
- Create: `pkg/rules/state_changes.go`
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/rules/state_changes_test.go`

**Interfaces:**
- Produces: `ApplyStateChanges(bridge GameHostAPI, changes []harness.StateChangeDecl, declared map[string]core.StatSpec) error`; engine calls it during turn recording.

- [x] **Step 1: Write the failing test**

```go
func TestApplyStateChanges(t *testing.T) {
	store := newRulesTestStore(t)
	bridge := NewHostBridge(store, nil, "player")
	// Seed player state hp=5.
	if err := bridge.SetStat("player", "hp", 5); err != nil {
		t.Fatal(err)
	}
	err := ApplyStateChanges(bridge, []harness.StateChangeDecl{
		{Entity: "player", Path: "hp", Op: "sub", Value: 2, Reason: "fall"},
	}, map[string]core.StatSpec{"hp": {ID: "hp", Type: "number"}})
	if err != nil {
		t.Fatalf("ApplyStateChanges: %v", err)
	}
	got, _ := bridge.GetStat("player", "hp")
	if got.(float64) != 3 {
		t.Fatalf("hp = %v, want 3", got)
	}
}

func TestApplyStateChangesRejectsUndeclared(t *testing.T) {
	bridge := NewHostBridge(newRulesTestStore(t), nil, "player")
	err := ApplyStateChanges(bridge, []harness.StateChangeDecl{
		{Entity: "player", Path: "gold", Op: "set", Value: 100},
	}, map[string]core.StatSpec{"hp": {ID: "hp", Type: "number"}})
	if err == nil {
		t.Fatal("undeclared stat should be rejected when a schema declares stats")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestApplyStateChanges ./pkg/rules/`
Expected: FAIL.

- [x] **Step 3: Implement**

- `ApplyStateChanges` walks the changes: `set` writes, `add`/`sub` coerce numerics and mutate, all through `bridge.SetStat`; rejects an undeclared path when `declared` is non-empty (unless the system sets `allow_freeform_state`).
- In `ProcessActionStream`, after a submission is accepted and before/with `RecordTurnContext`, call `ApplyStateChanges` with the manifest's declared stats.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestApplyStateChanges ./pkg/rules/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/rules/state_changes.go pkg/engine/orchestrator.go pkg/rules/state_changes_test.go
git commit -m "feat(rules): apply GM state changes through the host layer"
```

---

### Task 6: Health zero effect

**Files:**
- Modify: `pkg/rules/state_changes.go`, `pkg/rules/js_engine.go`
- Test: `pkg/rules/health_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestHealthZeroEffectRunsHook(t *testing.T) {
	engine := NewJSEngine(NewHostBridge(newRulesTestStore(t), nil, "player"))
	if err := engine.LoadScript(`onHealthZero(function(){ return "incapacitated"; });`); err != nil {
		t.Fatal(err)
	}
	effect, err := engine.EvaluateHealthZero("incapacitated")
	if err != nil {
		t.Fatalf("EvaluateHealthZero: %v", err)
	}
	if effect != "incapacitated" {
		t.Fatalf("effect = %q", effect)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestHealthZeroEffectRunsHook ./pkg/rules/`
Expected: FAIL.

- [x] **Step 3: Implement**

- Bind `onHealthZero(fn)`; `EvaluateHealthZero(effect string) (string, error)` calls the hook if registered, else returns the free-text effect.
- After `ApplyStateChanges`, the engine checks `health.stat` against 0 and calls `EvaluateHealthZero`, recording the result on the turn (`Turn.HealthEffect string`).

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestHealthZeroEffectRunsHook ./pkg/rules/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/rules/state_changes.go pkg/rules/js_engine.go pkg/rules/health_test.go
git commit -m "feat(rules): evaluate declared health-zero effects"
```

---

### Task 7: Turn lifecycle hooks

**Files:**
- Modify: `pkg/rules/js_engine.go`, `pkg/engine/orchestrator.go`
- Test: `pkg/rules/hooks_test.go`, `pkg/engine/hooks_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestTurnBeginAndEndHooks(t *testing.T) {
	engine := NewJSEngine(NewHostBridge(newRulesTestStore(t), nil, "player"))
	script := `
	  onTurnBegin(function(ctx){ log("begin " + ctx.turn); });
	  onTurnEnd(function(ctx){ log("end " + ctx.turn + " " + ctx.verdict); });
	`
	if err := engine.LoadScript(script); err != nil {
		t.Fatal(err)
	}
	if err := engine.ExecuteTurnBegin(map[string]interface{}{"turn": 4, "location": "hall"}); err != nil {
		t.Fatalf("ExecuteTurnBegin: %v", err)
	}
	if err := engine.ExecuteTurnEnd(map[string]interface{}{"turn": 4, "verdict": "uncertain"}); err != nil {
		t.Fatalf("ExecuteTurnEnd: %v", err)
	}
	if len(engine.GetLogs()) != 2 {
		t.Fatalf("logs = %v", engine.GetLogs())
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestTurnBeginAndEndHooks ./pkg/rules/`
Expected: FAIL.

- [x] **Step 3: Implement**

- Bind `onTurnBegin(fn)`; add `ExecuteTurnBegin(ctx map[string]interface{}) error`.
- In `ProcessActionStream`, call `ExecuteTurnBegin({turn, location})` after context assembly; call `ExecuteTurnEnd` with `{turn, verdict, checks, entities, narration}` and log (`turn.end_hook_error`) rather than discard its error.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestTurnBeginAndEndHooks ./pkg/rules/` and `go test ./pkg/engine/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/rules/js_engine.go pkg/engine/orchestrator.go pkg/rules/hooks_test.go
git commit -m "feat(rules): add onTurnBegin and enrich onTurnEnd"
```

---

### Task 8: GUI rules loading

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/rules_loading_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestGUILoadsRulesForCampaign(t *testing.T) {
	// Create a system with a mechanics.js that registers a side effect,
	// create a game, play a Do turn, and assert the hook ran.
}
```

(Use a temp rig with a hand-written `mechanics.js`.)

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestGUILoadsRulesForCampaign ./pkg/gui/`
Expected: FAIL.

- [x] **Step 3: Implement**

- Where `pkg/gui/service.go` builds the `JSEngine` (around line 1110), also build `rules.NewRuleLoader(paths, engine)` and call `LoadRules(systemID, worldID)` when the campaign is prepared (first turn / `ensureIndexed`), mirroring `cmd/localrpg/play.go:94`.
- Guard against reloading for every turn (once per campaign per process).

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestGUILoadsRulesForCampaign ./pkg/gui/` and `go test ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/rules_loading_test.go
git commit -m "fix(gui): load system rules for web campaigns"
```

---

### Task 9: Full verification

- [x] **Step 1: Run everything**

Run: `mise run test` and `mise run lint`
Expected: PASS.

- [x] **Step 2: Manual checks**

1. A system with a `mechanics` block resolves a check to a declared outcome and validates `state_changes`.
2. A js `onCheck` resolver overrides the schema convention.
3. A stat changed via `setStat` in js and via a GM `state_change` both persist to Markdown.
4. `onTurnBegin` fires once; `onTurnEnd` receives the verdict and checks.
5. A web campaign with a `mechanics.js` runs an `onAction` hook.
6. A system with no `mechanics` block behaves exactly as before.

- [x] **Step 3: Commit fixups**

```bash
git add -A
git commit -m "test: verify mechanics engagement and declarative schema"
```
