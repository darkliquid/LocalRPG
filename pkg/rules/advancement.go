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
