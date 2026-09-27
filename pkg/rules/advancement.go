package rules

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// EarnFromTurn sums the awards a turn earned under a system's rules. Outcomes is
// the system's declared vocabulary, used to recognise the failure result. It
// takes the resolved checks rather than an engine.Turn, because pkg/rules cannot
// import pkg/engine.
func EarnFromTurn(spec *core.AdvancementSpec, outcomes []string, checks []harness.CheckResult) int {
	if spec == nil {
		return 0
	}
	total := 0
	for _, rule := range spec.Earn {
		switch rule.On {
		case "turn_end":
			total += rule.Amount
		case "miss":
			for _, check := range checks {
				if isFailureOutcome(outcomes, check.Outcome) {
					total += rule.Amount
					break
				}
			}
		case "check_outcome":
			for _, check := range checks {
				if strings.EqualFold(strings.TrimSpace(check.Outcome), strings.TrimSpace(rule.Outcome)) {
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
func isFailureOutcome(outcomes []string, outcome string) bool {
	value := strings.ToLower(strings.TrimSpace(outcome))
	if value == "" {
		return false
	}
	if len(outcomes) > 0 {
		last := strings.ToLower(strings.TrimSpace(outcomes[len(outcomes)-1]))
		if last != "" && value == last {
			return true
		}
	}
	return strings.Contains(value, "miss") || strings.Contains(value, "fail")
}

// ApplyEarn changes the currency by amount (negative to spend). It writes through
// the host bridge so the entity's Markdown stays canonical.
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

// AdvancementHookRunner is implemented by a rules engine that can run a
// system-declared advance hook when an unlock is bought.
type AdvancementHookRunner interface {
	RunAdvanceHook(hook string, unlockID string) error
}

// Affordable reports whether a currency value covers an unlock's cost.
func Affordable(value int, unlock core.UnlockSpec) bool {
	return value >= unlock.Cost
}

// RequirementsMet reports whether every entry an unlock requires is owned or
// carried as a tag.
func RequirementsMet(unlock core.UnlockSpec, owned []string, tags []string) bool {
	if len(unlock.Requires) == 0 {
		return true
	}
	have := make(map[string]bool, len(owned)+len(tags))
	for _, o := range owned {
		have[o] = true
	}
	for _, tag := range tags {
		have[tag] = true
	}
	for _, req := range unlock.Requires {
		if !have[req] {
			return false
		}
	}
	return true
}

// NextThreshold returns the lowest declared level whose threshold the value has
// crossed. The caller decides whether that level has already been applied.
func NextThreshold(spec *core.AdvancementSpec, value int) (core.LevelSpec, bool) {
	if spec == nil {
		return core.LevelSpec{}, false
	}
	for _, level := range spec.Levels {
		if value >= level.At {
			return level, true
		}
	}
	return core.LevelSpec{}, false
}

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
		if err := applyEffect(bridge, playerID, effect, unlock.ID); err != nil {
			return err
		}
	}
	return nil
}

// ApplyEffects applies a list of unlock or level effects to the player. sourceID
// names the unlock or level an effect came from, for hooks.
func ApplyEffects(bridge GameHostAPI, playerID string, effects []core.EffectSpec, sourceID string) error {
	if bridge == nil {
		return fmt.Errorf("apply effects: no host bridge")
	}
	for _, effect := range effects {
		if err := applyEffect(bridge, playerID, effect, sourceID); err != nil {
			return err
		}
	}
	return nil
}

func applyEffect(bridge GameHostAPI, playerID string, effect core.EffectSpec, unlockID string) error {
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
			return fmt.Errorf("set %s: %w", effect.Stat, err)
		}
	case "grant_tag":
		if err := grantTag(bridge, playerID, effect.Tag); err != nil {
			return fmt.Errorf("grant tag %s: %w", effect.Tag, err)
		}
	case "hook":
		if err := runAdvanceHook(bridge, effect.Hook, unlockID); err != nil {
			return fmt.Errorf("advance hook %s: %w", effect.Hook, err)
		}
	default:
		return fmt.Errorf("unknown effect type %q", effect.Type)
	}
	return nil
}

// grantTag adds a tag to the player entity if it is not already present, writing
// through the bridge so the note stays canonical.
func grantTag(bridge GameHostAPI, playerID string, tag string) error {
	if tag == "" {
		return nil
	}
	ent, err := bridge.GetEntity(playerID)
	if err != nil {
		return err
	}
	for _, existing := range ent.Tags {
		if existing == tag {
			return nil
		}
	}
	ent.Tags = append(ent.Tags, tag)
	return bridge.SaveEntity(ent)
}

// runAdvanceHook runs a declared hook when the bridge's engine supports one; a
// system with effects but no hook runner simply skips it.
func runAdvanceHook(bridge GameHostAPI, hook string, unlockID string) error {
	if hook == "" {
		return nil
	}
	if runner, ok := bridge.(AdvancementHookRunner); ok {
		return runner.RunAdvanceHook(hook, unlockID)
	}
	return nil
}
