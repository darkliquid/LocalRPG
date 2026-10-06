package engine

import (
	"strings"

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

// healthEffectFor resolves the declared health-zero effect for one entity, or ""
// when the entity has no numeric health stat, is above zero, or no schema is
// declared. Requiring a real number means an entity that never had the stat does
// not fire the effect.
func (o *TurnOrchestrator) healthEffectFor(entityID string) string {
	if o.health == nil || o.health.Stat == "" || o.rulesEngine == nil || entityID == "" {
		return ""
	}
	raw, err := o.rulesEngine.HostAPI().GetStat(entityID, o.health.Stat)
	if err != nil {
		return ""
	}
	value, ok := numericValue(raw)
	if !ok || value > 0 {
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

// healthOutcome resolves the player's health-zero effect, for callers that only
// care about the protagonist.
func (o *TurnOrchestrator) healthOutcome() string {
	return o.healthEffectFor(o.playerID)
}

// healthOutcomes resolves a health-zero effect for the player and for every
// character named in the turn, so a downed ally is visible rather than only the
// protagonist. Entities are deduplicated and ordered player-first.
func (o *TurnOrchestrator) healthOutcomes(turn *Turn) []HealthEffect {
	if o.health == nil || o.health.Stat == "" || o.rulesEngine == nil {
		return nil
	}

	ids := make([]string, 0, len(turn.Entities)+1)
	if o.playerID != "" {
		ids = append(ids, o.playerID)
	}
	for _, mention := range turn.Entities {
		if mention.ID != "" && mention.ID != o.playerID {
			ids = append(ids, mention.ID)
		}
	}

	effects := make([]HealthEffect, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if effect := o.healthEffectFor(id); effect != "" {
			effects = append(effects, HealthEffect{Entity: id, Effect: effect})
		}
	}
	return effects
}

// mergeHealthEffects unions two passes, keeping the first effect per entity so a
// turn never records the same entity twice.
func mergeHealthEffects(passes ...[]HealthEffect) []HealthEffect {
	var out []HealthEffect
	seen := map[string]bool{}
	for _, effects := range passes {
		for _, effect := range effects {
			if seen[effect.Entity] {
				continue
			}
			seen[effect.Entity] = true
			out = append(out, effect)
		}
	}
	return out
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

// runTurnEndHooks runs the onTurnEnd hooks with the turn's facts. pending is the
// health-zero effects already resolved from the turn's own state changes, so a
// hook can react to them; a health change the hook itself makes is resolved by
// the caller afterwards.
func (o *TurnOrchestrator) runTurnEndHooks(turnNum int, turn *Turn, pending []HealthEffect) {
	if o.rulesEngine == nil {
		return
	}

	entityIDs := make([]string, 0, len(turn.Entities))
	for _, mention := range turn.Entities {
		entityIDs = append(entityIDs, mention.ID)
	}

	hookCtx := map[string]interface{}{
		"turn":           turnNum,
		"narration":      turn.Narration,
		"entities":       entityIDs,
		"checks":         len(turn.Checks),
		"health_effects": pending,
		"world_tick":     turn.WorldTick,
	}
	if err := o.rulesEngine.ExecuteTurnEnd(hookCtx); err != nil {
		o.logger.Event("turn.end_hook_error", map[string]interface{}{"error": err.Error()})
	}
}
