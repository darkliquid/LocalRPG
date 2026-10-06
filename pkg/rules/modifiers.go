package rules

import "github.com/darkliquid/localrpg/pkg/harness"

// SumBonuses totals a check's stat, skill, and named modifiers. value resolves a
// declared stat or skill; a miss contributes nothing. The returned slice names
// every contribution so a result can be displayed.
func SumBonuses(value func(name string) (int, bool), req harness.CheckRequest) (int, []harness.AppliedModifier) {
	total := 0
	applied := make([]harness.AppliedModifier, 0, 2+len(req.Modifiers))
	for _, name := range []string{req.Stat, req.Skill} {
		if name == "" {
			continue
		}
		if v, ok := value(name); ok && v != 0 {
			total += v
			applied = append(applied, harness.AppliedModifier{Source: name, Value: v})
		}
	}
	for _, m := range req.Modifiers {
		if m.Value == 0 {
			continue
		}
		total += m.Value
		applied = append(applied, harness.AppliedModifier{Source: m.Source, Value: m.Value})
	}
	return total, applied
}
