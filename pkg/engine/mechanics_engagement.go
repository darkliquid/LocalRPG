package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// HealthEffect records that an entity's declared health stat reached zero and
// the effect the system resolved for it. It is persisted on the turn so the
// chronicle can show the consequence alongside the prose that caused it.
type HealthEffect struct {
	Entity string `json:"entity"`
	Effect string `json:"effect"`
}

// engagementOrAuto is the engagement policy the resolution instruction reflects.
func (o *TurnOrchestrator) engagementOrAuto() string {
	if strings.TrimSpace(o.mechanicsEngagement) == "" {
		return "auto"
	}
	return o.mechanicsEngagement
}

// mechanicsInstruction builds the resolution instruction for this turn. When a
// schema was supplied it is rebuilt from the player's current stats, so a
// check names a value the player actually has rather than a stale snapshot.
func (o *TurnOrchestrator) mechanicsInstruction() string {
	if !o.rebuildMechanicsPrompt {
		return o.mechanicsPrompt
	}
	return harness.FormatMechanicsInstructions(o.mechanics, o.engagementOrAuto(), o.statValues())
}

// statValues reads the player's declared stats and their current values through
// the same bridge the mechanics scripts write through. Only numeric stats are
// offered, because only those resolve a check.
func (o *TurnOrchestrator) statValues() []harness.StatValue {
	if o.mechanics == nil || o.rulesEngine == nil || o.playerID == "" {
		return nil
	}
	bridge := o.rulesEngine.HostAPI()
	values := make([]harness.StatValue, 0, len(o.mechanics.Stats))
	for _, stat := range o.mechanics.Stats {
		raw, err := bridge.GetStat(o.playerID, stat.ID)
		if err != nil {
			continue
		}
		value, ok := numericValue(raw)
		if !ok {
			continue
		}
		values = append(values, harness.StatValue{ID: stat.ID, Label: stat.Label, Value: value})
	}
	return values
}

// numericValue reports whether a raw stat is a number the engine can offer a
// check, and its value.
func numericValue(raw interface{}) (int, bool) {
	switch raw.(type) {
	case nil, bool, string:
		return 0, false
	}
	return intValue(raw), true
}

// healthOutcome resolves the declared health-zero effect for the player, or ""
// when health is positive or no health schema is declared.
func (o *TurnOrchestrator) healthOutcome() string {
	if o.health == nil || o.health.Stat == "" || o.rulesEngine == nil || o.playerID == "" {
		return ""
	}
	value, err := o.rulesEngine.HostAPI().GetStat(o.playerID, o.health.Stat)
	if err != nil {
		return ""
	}
	if intValue(value) > 0 {
		return ""
	}
	if strings.TrimSpace(o.health.ZeroEffect) == "" {
		return ""
	}
	effect, err := o.rulesEngine.EvaluateHealthZero(o.health.ZeroEffect)
	if err != nil {
		o.logger.Event("health.zero_error", map[string]interface{}{"error": err.Error()})
		return o.health.ZeroEffect
	}
	return effect
}

// worldTickDue reports whether this turn should run onWorldTick. The first turn
// is a tick, then every cadence turns after it.
func (o *TurnOrchestrator) worldTickDue(turnNum int) bool {
	if o.worldTickTurns <= 0 || turnNum <= 0 {
		return false
	}
	return (turnNum-1)%o.worldTickTurns == 0
}

// runWorldTick runs the onWorldTick hooks for this turn. A hook error is logged
// and never fails the turn, and the hooks' injected directives are drained later
// alongside the other hook directives.
func (o *TurnOrchestrator) runWorldTick(turnNum int, locationID string) {
	if o.rulesEngine == nil || !o.worldTickDue(turnNum) {
		return
	}
	o.logger.Event("mechanics.world_tick", map[string]interface{}{"turn": turnNum})
	if err := o.rulesEngine.ExecuteWorldTick(map[string]interface{}{
		"turn":     turnNum,
		"location": locationID,
		"player":   o.playerID,
	}); err != nil {
		o.logger.Event("mechanics.world_tick_error", map[string]interface{}{"error": err.Error()})
	}
}

// drainDirectives returns every directive hooks injected this turn. The host
// bridge accumulates them; without draining, injectGMDirection had no effect.
func (o *TurnOrchestrator) drainDirectives() []string {
	if o.rulesEngine == nil {
		return nil
	}
	directives := o.rulesEngine.HostAPI().GetDirectives()
	out := make([]string, 0, len(directives))
	for _, directive := range directives {
		if trimmed := strings.TrimSpace(directive); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// applyDirectives prepends each directive to the turn's GM directive, so an
// injected directive reaches the prompt before generation.
func applyDirectives(gmDirective string, directives []string) string {
	for i := len(directives) - 1; i >= 0; i-- {
		if gmDirective == "" {
			gmDirective = directives[i]
			continue
		}
		gmDirective = directives[i] + "\n" + gmDirective
	}
	return gmDirective
}

// healthSpec returns the declared health schema, or nil.
func (o *TurnOrchestrator) healthSpec() *core.HealthSpec { return o.health }
