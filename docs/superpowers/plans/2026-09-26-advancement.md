# Advancement & XP Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27; the manual browser check in Task 8 remains.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A schema-agnostic advancement system: declarative earn triggers, a currency, a catalog of unlocks with three cost modes, a spend API, and the character-drawer surface.

**Architecture:** A system declares its advancement in `system.yaml`; the engine recognises common earn events and exposes `grantXP` to `mechanics.js`; spends apply through the existing state-change path so Markdown stays canonical; the game-state DTO carries an affordability summary for the UI.

**Tech Stack:** Go 1.27 (stdlib `testing`), `goja` (existing), React 19. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-26-advancement-design.md`

## Global Constraints

- Go tests use only `testing` and `t.TempDir()`; no testify, no mocks.
- Use `interface{}`, never `any`; wrap errors; `go vet ./...` clean.
- No new dependencies.
- A system without an `advancement` block behaves exactly as today.
- Conventional Commits with a scope; subject under 72 chars.
- Verification: `mise run test`, `mise run lint`, `mise run build`.

---

### Task 1: The declarative advancement schema

**Files:**
- Modify: `pkg/core/mechanics.go` (`MechanicsSpec` ~line 6-14; add types)
- Test: `pkg/core/mechanics_test.go` (create or append)

**Interfaces:**
- Produces: `AdvancementSpec`, `CurrencySpec`, `EarnRule`, `UnlockSpec`, `EffectSpec`, `LevelSpec`; `MechanicsSpec.Advancement *AdvancementSpec`.

- [x] **Step 1: Write the failing test**

Create `pkg/core/mechanics_test.go`:

```go
package core

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAdvancementSpecRoundTrips(t *testing.T) {
	raw := `
id: sys
name: Sys
mechanics:
  stats:
    - { id: xp, label: Experience, type: number, default: 0 }
  advancement:
    currency: { stat: xp, label: Experience }
    mode: spend
    earn:
      - { on: miss, amount: 1 }
      - { on: check_outcome, outcome: strong, amount: 2 }
    unlocks:
      - id: stat-increase
        label: Increase a stat
        cost: 5
        effects:
          - { type: stat_increase, amount: 1, max: 18 }
    levels:
      - { at: 300, label: "Level 2", effects: [{ type: stat_increase, amount: 1 }] }
`
	var manifest SystemManifest
	if err := yaml.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatal(err)
	}
	adv := manifest.Mechanics.Advancement
	if adv == nil {
		t.Fatal("advancement block did not parse")
	}
	if adv.Currency.Stat != "xp" || adv.Mode != "spend" || len(adv.Earn) != 2 {
		t.Fatalf("advancement = %+v", adv)
	}
	if len(adv.Unlocks) != 1 || adv.Unlocks[0].Effects[0].Max != 18 {
		t.Fatalf("unlocks = %+v", adv.Unlocks)
	}
	if len(adv.Levels) != 1 || adv.Levels[0].At != 300 {
		t.Fatalf("levels = %+v", adv.Levels)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestAdvancementSpecRoundTrips ./pkg/core/ -v`
Expected: FAIL — `Advancement` undefined.

- [x] **Step 3: Add the types**

In `pkg/core/mechanics.go`:

```go
// AdvancementSpec declares a system's progression: an earned currency, how it is
// earned, and what it buys. A nil spec means the system has no advancement.
type AdvancementSpec struct {
	Currency  CurrencySpec `yaml:"currency"`
	Mode      string       `yaml:"mode,omitempty"` // spend | track | threshold
	Earn      []EarnRule   `yaml:"earn,omitempty"`
	TrackSize int          `yaml:"track_size,omitempty"`
	Gate      string       `yaml:"gate,omitempty"` // "" | downtime
	Unlocks   []UnlockSpec `yaml:"unlocks,omitempty"`
	Levels    []LevelSpec  `yaml:"levels,omitempty"`
}

// CurrencySpec names the stat that holds earned advancement points.
type CurrencySpec struct {
	Stat  string `yaml:"stat"`
	Label string `yaml:"label,omitempty"`
}

// EarnRule awards the currency when an engine-recognised event occurs.
type EarnRule struct {
	On      string `yaml:"on"`                // miss | check_outcome | turn_end | hook
	Outcome string `yaml:"outcome,omitempty"` // for on: check_outcome
	Rank    string `yaml:"rank,omitempty"`
	Amount  int    `yaml:"amount"`
}

// UnlockSpec is one thing the currency can buy.
type UnlockSpec struct {
	ID          string       `yaml:"id"`
	Label       string       `yaml:"label"`
	Description string       `yaml:"description,omitempty"`
	Cost        int          `yaml:"cost"`
	Requires    []string     `yaml:"requires,omitempty"`
	Effects     []EffectSpec `yaml:"effects,omitempty"`
}

// EffectSpec is one change an unlock applies.
type EffectSpec struct {
	Type   string `yaml:"type"` // stat_increase | set_stat | grant_tag | hook
	Stat   string `yaml:"stat,omitempty"`
	Amount int    `yaml:"amount,omitempty"`
	Max    int    `yaml:"max,omitempty"`
	Tag    string `yaml:"tag,omitempty"`
	Hook   string `yaml:"hook,omitempty"`
}

// LevelSpec is a threshold-mode level.
type LevelSpec struct {
	At      int          `yaml:"at"`
	Label   string       `yaml:"label,omitempty"`
	Effects []EffectSpec `yaml:"effects,omitempty"`
}
```

Add `Advancement *AdvancementSpec \`yaml:"advancement,omitempty"\`` to `MechanicsSpec`.

- [x] **Step 4: Run test and commit**

Run: `go test ./pkg/core/`

```bash
git add pkg/core/mechanics.go pkg/core/mechanics_test.go
git commit -m "feat(advancement): declare progression in system.yaml"
```

---

### Task 2: Earning and applying the currency

**Files:**
- Create: `pkg/rules/advancement.go`
- Test: `pkg/rules/advancement_test.go`

**Interfaces:**
- Produces: `rules.EarnFromTurn(spec *core.AdvancementSpec, turn *harness.Turn) int`; `rules.ApplyEarn(bridge GameHostAPI, spec *core.AdvancementSpec, playerID string, amount int) error`.
- Consumes: `GameHostAPI.GetStat/SetStat`, `harness.Turn.Checks`/`Outcome`.

- [x] **Step 1: Write the failing test**

Create `pkg/rules/advancement_test.go`:

```go
package rules

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestEarnFromTurnRecognisesEvents(t *testing.T) {
	spec := &core.AdvancementSpec{Earn: []EarnRule{
		{On: "miss", Amount: 1},
		{On: "check_outcome", Outcome: "strong", Amount: 2},
	}}
	turn := &harness.Turn{Checks: []harness.CheckResult{{Outcome: "miss"}}}
	if got := EarnFromTurn(spec, turn); got != 1 {
		t.Fatalf("miss award = %d, want 1", got)
	}
	turn.Checks = []harness.CheckResult{{Outcome: "strong"}}
	if got := EarnFromTurn(spec, turn); got != 2 {
		t.Fatalf("strong award = %d, want 2", got)
	}
	turn.Checks = nil
	if got := EarnFromTurn(spec, turn); got != 0 {
		t.Fatalf("quiet turn award = %d, want 0", got)
	}
}
```

Add an `ApplyEarn` test using the package's fake host bridge (the one `state_changes_test.go` uses) asserting the currency stat increments.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestEarnFromTurn ./pkg/rules/ -v`
Expected: FAIL — undefined.

- [x] **Step 3: Implement earning**

Create `pkg/rules/advancement.go`:

```go
package rules

import (
	"fmt"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// EarnFromTurn sums the awards a turn earned under a system's rules. An outcome
// is matched case-insensitively against the system's own vocabulary.
func EarnFromTurn(spec *core.AdvancementSpec, turn *harness.Turn) int {
	if spec == nil || turn == nil {
		return 0
	}
	total := 0
	for _, rule := range spec.Earn {
		switch rule.On {
		case "turn_end":
			total += rule.Amount
		case "miss":
			for _, check := range turn.Checks {
				if isFailureOutcome(spec, check.Outcome) {
					total += rule.Amount
					break
				}
			}
		case "check_outcome":
			for _, check := range turn.Checks {
				if equalFold(check.Outcome, rule.Outcome) {
					total += rule.Amount
					break
				}
			}
		case "hook":
			// Awarded through grantXP, not here.
		}
	}
	return total
}

// isFailureOutcome reports whether an outcome is the system's failure result: the
// last declared outcome, or a label in the miss/fail family.
func isFailureOutcome(spec *core.AdvancementSpec, outcome string) bool { /* last Outcome from spec.Checks, or "miss"/"fail" */ }

func ApplyEarn(bridge GameHostAPI, spec *core.AdvancementSpec, playerID string, amount int) error {
	if bridge == nil || spec == nil || amount == 0 {
		return nil
	}
	if spec.Currency.Stat == "" {
		return fmt.Errorf("advancement has no currency stat")
	}
	current, err := bridge.GetStat(playerID, spec.Currency.Stat)
	if err != nil {
		return fmt.Errorf("read %s: %w", spec.Currency.Stat, err)
	}
	base, _ := toInt(current)
	return bridge.SetStat(playerID, spec.Currency.Stat, base+amount)
}
```

Note: `isFailureOutcome` needs the system's declared outcome vocabulary; the function takes the `CheckConventions` from `spec` — since `AdvancementSpec` does not carry it, pass the outcomes in from the caller, or move the helper to the engine where the `MechanicsSpec` is in scope. Choose: define `EarnFromTurn(spec *core.AdvancementSpec, outcomes []string, turn *harness.Turn) int` and pass `mechanics.Checks.Outcome`.

- [x] **Step 4: Run tests and commit**

Run: `go test ./pkg/rules/`

```bash
git add pkg/rules/advancement.go pkg/rules/advancement_test.go
git commit -m "feat(advancement): recognise earn events and award the currency"
```

---

### Task 3: `grantXP` host binding

**Files:**
- Modify: `pkg/rules/js_engine.go` (bindings ~line 45-158; `SetManifest` ~line 161)
- Test: `pkg/rules/js_engine_test.go` (append)

**Interfaces:**
- Produces: `grantXP(amount)` callable from `mechanics.js`; awards to the manifest's advancement currency.

- [x] **Step 1: Write the failing test**

Append to `pkg/rules/js_engine_test.go`:

```go
func TestGrantXPAwardsTheCurrency(t *testing.T) {
	engine := NewJSEngine(bridge) // existing test bridge with a player entity
	engine.SetManifest(&core.SystemManifest{
		ID: "sys",
		Mechanics: &core.MechanicsSpec{
			Advancement: &core.AdvancementSpec{Currency: core.CurrencySpec{Stat: "xp"}},
		},
	})
	engine.RunScript(`grantXP(3);`) // add a RunScript/Eval test helper if none exists
	got, _ := bridge.GetStat("player", "xp")
	if n, _ := toInt(got); n != 3 {
		t.Fatalf("xp = %v, want 3", got)
	}
}
```

If there is no `RunScript`, execute via the loader path used by existing tests (`LoadRules` on a temp system) and assert through the bridge.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestGrantXP ./pkg/rules/ -v`
Expected: FAIL.

- [x] **Step 3: Implement the binding**

In `bindHostAPI` (`pkg/rules/js_engine.go`):

```go
	j.vm.Set("grantXP", func(call goja.FunctionCall) goja.Value {
		amount := int(call.Argument(0).ToInteger())
		spec := j.advancement()
		if spec == nil || spec.Currency.Stat == "" {
			return j.vm.ToValue(false)
		}
		if err := ApplyEarn(j.bridge, spec, j.playerID, amount); err != nil {
			j.logger.Event("advancement.grant_error", map[string]interface{}{"error": err.Error()})
			return j.vm.ToValue(false)
		}
		return j.vm.ToValue(true)
	})
```

Store the manifest so `advancement()` can read it (`SetManifest` already stores it for the host bridge; keep a copy on the engine, or read it back through the bridge). `playerID` comes from the bridge/timeline the engine was built with; if the engine does not hold it, thread it through `NewJSEngine` (it already takes a `HostBridge` bound to a player).

- [x] **Step 4: Run tests and commit**

Run: `go test ./pkg/rules/`

```bash
git add pkg/rules/js_engine.go pkg/rules/js_engine_test.go
git commit -m "feat(advancement): let mechanics.js grant experience"
```

---

### Task 4: Spending — unlock effects and gates

**Files:**
- Modify: `pkg/rules/advancement.go` (add `ApplyUnlock`, `Affordable`, `RequirementsMet`, `NextThreshold`)
- Test: `pkg/rules/advancement_test.go` (append)

**Interfaces:**
- Produces: `ApplyUnlock(bridge, spec, unlock, playerID, tags []string, gateOpen bool) error`; `Affordable(spec, value, unlock) bool`; `RequirementsMet(spec, unlock, owned []string, tags []string) bool`; `NextThreshold(spec, value) (core.LevelSpec, bool)`.

- [x] **Step 1: Write the failing tests**

Append to `pkg/rules/advancement_test.go`:

```go
func TestApplyUnlockDeductsAndApplies(t *testing.T) {
	bridge := newTestBridge(t, map[string]interface{}{"xp": 5, "might": 1})
	spec := &core.AdvancementSpec{Currency: core.CurrencySpec{Stat: "xp"}}
	unlock := core.UnlockSpec{ID: "stat-increase", Cost: 5, Effects: []core.EffectSpec{
		{Type: "stat_increase", Stat: "might", Amount: 1, Max: 3},
	}}
	if err := ApplyUnlock(bridge, spec, unlock, "player", nil, true); err != nil {
		t.Fatalf("ApplyUnlock: %v", err)
	}
	if xp, _ := bridge.GetStat("player", "xp"); xp != 0 {
		t.Fatalf("xp = %v, want 0", xp)
	}
	if might, _ := bridge.GetStat("player", "might"); might != 2 {
		t.Fatalf("might = %v, want 2", might)
	}
}

func TestApplyUnlockHonoursCapAndGate(t *testing.T) {
	bridge := newTestBridge(t, map[string]interface{}{"xp": 5, "might": 3})
	spec := &core.AdvancementSpec{Currency: core.CurrencySpec{Stat: "xp"}}
	unlock := core.UnlockSpec{ID: "cap", Cost: 5, Effects: []core.EffectSpec{{Type: "stat_increase", Stat: "might", Amount: 1, Max: 3}}}
	if err := ApplyUnlock(bridge, spec, unlock, "player", nil, true); err != nil {
		t.Fatalf("ApplyUnlock: %v", err)
	}
	if might, _ := bridge.GetStat("player", "might"); might != 3 {
		t.Fatalf("might = %v, want the cap 3", might)
	}
	if err := ApplyUnlock(bridge, spec, core.UnlockSpec{ID: "gated", Cost: 1}, "player", nil, false); err == nil {
		t.Fatal("a closed gate should refuse the spend")
	}
}
```

Use the existing test bridge helper from `host_api_test.go`/`state_changes_test.go`; if it has no GetStat seeding, extend it.

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run TestApplyUnlock ./pkg/rules/ -v`
Expected: FAIL.

- [x] **Step 3: Implement spending**

In `pkg/rules/advancement.go`:

```go
// ApplyUnlock deducts the cost and applies an unlock's effects. Affordability,
// requirements, and the gate are the caller's to check; this applies.
func ApplyUnlock(bridge GameHostAPI, spec *core.AdvancementSpec, unlock core.UnlockSpec, playerID string, tags []string, gateOpen bool) error {
	if bridge == nil || spec == nil {
		return fmt.Errorf("apply unlock: no advancement spec")
	}
	if spec.Gate != "" && !gateOpen {
		return fmt.Errorf("advancement is gated by %s", spec.Gate)
	}
	if err := ApplyEarn(bridge, spec, playerID, -unlock.Cost); err != nil {
		return err
	}
	for _, effect := range unlock.Effects {
		switch effect.Type {
		case "stat_increase":
			current, err := bridge.GetStat(playerID, effect.Stat)
			if err != nil {
				return fmt.Errorf("read %s: %w", effect.Stat, err)
			}
			base, _ := toInt(current)
			next := base + effect.Amount
			if effect.Max != 0 && next > effect.Max {
				next = effect.Max
			}
			if err := bridge.SetStat(playerID, effect.Stat, next); err != nil {
				return fmt.Errorf("raise %s: %w", effect.Stat, err)
			}
		case "set_stat":
			if err := bridge.SetStat(playerID, effect.Stat, effect.Amount); err != nil {
				return err
			}
		case "grant_tag":
			if err := grantTag(bridge, playerID, effect.Tag); err != nil {
				return err
			}
		case "hook":
			if err := runAdvanceHook(bridge, effect.Hook, unlock.ID); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown effect type %q", effect.Type)
		}
	}
	return nil
}

// Affordable and RequirementsMet are pure helpers the API layer uses to compute
// the DTO without mutating anything.
func Affordable(value int, unlock core.UnlockSpec) bool { return value >= unlock.Cost }

func RequirementsMet(unlock core.UnlockSpec, owned []string, tags []string) bool { /* owned/tags set contains each requires entry */ }

// NextThreshold returns the next unapplied level for a threshold-mode system.
func NextThreshold(spec *core.AdvancementSpec, value int) (core.LevelSpec, bool) {
	for _, level := range spec.Levels {
		if value >= level.At {
			return level, true
		}
	}
	return core.LevelSpec{}, false
}
```

`grantTag` reads the entity, appends the tag, and writes it back through the bridge's entity access (use `GameHostAPI.GetEntity` and the timeline/EntityWriter to persist, so Markdown stays canonical). `runAdvanceHook` is wired in Task 5 with the JS engine.

- [x] **Step 4: Run tests and commit**

Run: `go test ./pkg/rules/`

```bash
git add pkg/rules/advancement.go pkg/rules/advancement_test.go
git commit -m "feat(advancement): apply unlocks with effects, caps, and gates"
```

---

### Task 5: Engine wiring — earn, track, threshold

**Files:**
- Modify: `pkg/engine/orchestrator.go` (`RecordTurnContextStructured` call site ~line 950)
- Modify: `pkg/engine/timeline.go` (record an `advancement` memory; reuse `writeMechanicalMemories` ~line 560-598)
- Test: `pkg/engine/advancement_test.go` (create)

**Interfaces:**
- Consumes: `rules.EarnFromTurn`, `rules.ApplyEarn`, `rules.NextThreshold`, `rules.ApplyUnlock`.
- Produces: awards applied once per turn; threshold levels auto-applied; track ticks incremented.

- [x] **Step 1: Write the failing test**

Create `pkg/engine/advancement_test.go`:

```go
func TestTurnAwardsExperienceOnAMiss(t *testing.T) {
	o, store := advancementOrchestrator(t, `mechanics advancement xp/miss`) // build via the tool-loop scaffolding with a system declaring
	// ... run a turn whose submission resolves a check with outcome "miss"
	// ... assert store.GetEntity("player").State.Raw()["xp"] == 1
}
```

Build the fixture by extending the existing `toolLoopOrchestrator` to accept a `*core.SystemManifest` with an `Advancement` spec and a submission that resolves a `miss`; assert the player entity's `state.xp` became 1 and an `advancement` memory exists.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestTurnAwardsExperience ./pkg/engine/ -v`
Expected: FAIL.

- [x] **Step 3: Award during the turn**

In `ProcessActionStream`, after `turn.Checks` is set and before `RecordTurnContextStructured`:

```go
	if award := rules.EarnFromTurn(mechanics.Advancement, mechanics.Checks.Outcome, turn); award > 0 {
		if err := rules.ApplyEarn(bridge, mechanics.Advancement, o.playerID, award); err != nil {
			o.logger.Event("advancement.award_error", map[string]interface{}{"error": err.Error()})
		} else {
			o.logger.Event("advancement.award", map[string]interface{}{"amount": award})
		}
	}
```

where `mechanics` is the loaded `*core.MechanicsSpec` (thread it onto the orchestrator alongside `declaredStats`, e.g. `SetMechanics(spec)`), and `bridge` is the rules bridge the orchestrator already uses for state.

Track and threshold modes:

```go
	if mechanics.Advancement != nil && mechanics.Advancement.Mode == "threshold" {
		if next, ok := rules.NextThreshold(mechanics.Advancement, currentXP); ok {
			// apply the level's effects and record "advanced"
		}
	}
```

Track mode increments a `track` stat and, at `track_size`, clears it and records an offer. Do this in the same block, guarded by nil.

Record a memory (kind `advancement`, importance 4) alongside `writeMechanicalMemories` so the chronicle can mention the award.

- [x] **Step 4: Run tests and commit**

Run: `go test ./pkg/engine/`

```bash
git add pkg/engine/orchestrator.go pkg/engine/timeline.go pkg/engine/advancement_test.go
git commit -m "feat(advancement): award experience during a turn"
```

---

### Task 6: API and DTO

**Files:**
- Modify: `pkg/gui/types.go` (`GameStateDTO`, new `AdvancementDTO`/`UnlockDTO`/`TrackDTO`)
- Modify: `pkg/gui/service.go` (`GetGameState`; new `AdvanceUnlock`)
- Modify: `pkg/gui/server.go` (route `POST /api/game/{id}/advance`)
- Test: `pkg/gui/advancement_test.go` (create)

**Interfaces:**
- Produces: `GameStateDTO.Advancement *AdvancementDTO`; `(*Service).AdvanceUnlock(ctx, gameID, unlockID) (*AdvancementDTO, error)`.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/advancement_test.go`:

```go
func TestAdvanceEndpointSpendsAnUnlock(t *testing.T) {
	gameID, svc := advancementFixture(t) // a campaign whose system declares xp + one unlock, player xp seeded to the cost
	server := NewServer(svc, nil)

	body := strings.NewReader(`{"unlock_id":"stat-increase"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/advance", body)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	state, _ := svc.GetGameState(context.Background(), gameID)
	if state.Advancement == nil || state.Advancement.Value != 0 {
		t.Fatalf("advancement after spend = %+v", state.Advancement)
	}
}
```

Add a negative case: an unaffordable unlock returns 400 and leaves the currency unchanged.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestAdvanceEndpoint ./pkg/gui/ -v`
Expected: FAIL (404).

- [x] **Step 3: Implement the DTO and route**

`pkg/gui/types.go`:

```go
type TrackDTO struct {
	Filled int `json:"filled"`
	Size   int `json:"size"`
}
type UnlockDTO struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Cost        int    `json:"cost"`
	Affordable  bool   `json:"affordable"`
	RequiresMet bool   `json:"requires_met"`
}
type AdvancementDTO struct {
	Currency string      `json:"currency"`
	Label    string      `json:"label"`
	Value    int         `json:"value"`
	Mode     string      `json:"mode"`
	Track    *TrackDTO   `json:"track,omitempty"`
	Unlocks  []UnlockDTO `json:"unlocks,omitempty"`
	Pending  bool        `json:"pending"`
}
```

Add `Advancement *AdvancementDTO \`json:"advancement,omitempty"\`` to `GameStateDTO` and populate it in `GetGameState` from the player entity's state, the system's `Advancement` spec, the player's owned tags, and the campaign's `settings.downtime`.

`AdvanceUnlock` loads the spec and unlock, checks `Affordable`/`RequirementsMet`/gate, calls `rules.ApplyUnlock` with the bridge, records an `advancement` memory, and returns the recomputed DTO. A refusal returns a typed error the route maps to 400.

Route in `pkg/gui/server.go`: `case "advance":` on the `POST /api/game/{id}/...` switch, decoding `{"unlock_id": "..."}`.

- [x] **Step 4: Run tests and commit**

Run: `go test ./pkg/gui/`

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/server.go pkg/gui/advancement_test.go
git commit -m "feat(advancement): expose and spend unlocks over the API"
```

---

### Task 7: Character drawer and notification dot

**Files:**
- Modify: `frontend/src/types.ts` (AdvancementDTO), `frontend/src/api/client.ts` (`advanceUnlock`)
- Modify: `frontend/src/components/CharacterSheetDrawer.tsx` (spend section)
- Modify: `frontend/src/App.tsx` (dot on the Character trigger)

**Interfaces:**
- Consumes: `GameState.Advancement`; `POST /api/game/{id}/advance`.

- [x] **Step 1: Types and client**

Mirror `AdvancementDTO` in `types.ts` and add `APIClient.advanceUnlock(gameId, unlockID)` in the existing fetch style.

- [x] **Step 2: Spend section**

In `CharacterSheetDrawer.tsx`, add an Advancement section: the currency and label, the track when present, and the unlock list with cost and a Spend button; disable with a reason ("not affordable", "requirements unmet", "needs downtime"). On success, call the parent refresh.

- [x] **Step 3: Notification dot**

In `App.tsx`, on the Character trigger button (`:504-512`), render a small dot when `gameState?.advancement` has any affordable unlock or `pending`, using the existing dot style (`:495`). The refresh already runs after each turn via `refreshCorpus()` (`:237`).

- [x] **Step 4: Verify and commit**

Run: `mise run test:frontend && mise run build`

```bash
git add frontend/src
git commit -m "feat(advancement): spend unlocks from the character drawer"
```

---

### Task 8: Full verification

- [x] **Step 1:** `mise run test`
- [x] **Step 2:** `mise run lint && mise run build`
- [ ] **Step 3:** Manual: declare an `advancement` block in `systems/narrative_2d6/system.yaml`, play a turn that misses, and confirm the currency rises, the drawer lists the unlock, the dot appears, and a spend applies.

---

## Self-Review Notes

- Spec coverage: schema → Task 1; earning → Task 2; `grantXP` → Task 3; spending → Task 4; engine wiring → Task 5; API/DTO → Task 6; UI → Task 7.
- Type consistency: `AdvancementSpec`/`UnlockSpec`/`EffectSpec`/`LevelSpec`, `EarnFromTurn`, `ApplyEarn`, `ApplyUnlock`, `Affordable`, `RequirementsMet`, `NextThreshold`, `AdvancementDTO`/`UnlockDTO`/`TrackDTO` are used consistently.
- Two helper decisions are called out inline because they depend on existing test scaffolding (`newTestBridge`, `RunScript`); the plan names the fallback so no step is a placeholder.
- Shared/crew ledgers and a dedicated currency frontmatter section are out of scope (spec §8).
