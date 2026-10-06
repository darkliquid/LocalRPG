package rules

import (
	"sort"

	"github.com/darkliquid/localrpg/pkg/core"
)

// ResolveProfile maps a roll total and success count through a profile to an
// outcome. It returns ("", false) when the profile is empty or cannot decide, so
// the caller falls back to the conventions path. It is shared by the schema
// resolver and the engine's default resolver so the two cannot drift.
func ResolveProfile(p core.ResolutionProfile, total, successes int) (string, bool) {
	if len(p.Ladder) > 0 {
		steps := append([]core.LadderStep(nil), p.Ladder...)
		sort.SliceStable(steps, func(i, j int) bool { return steps[i].Min > steps[j].Min })
		for _, step := range steps {
			if total >= step.Min {
				return step.Outcome, true
			}
		}
		return steps[len(steps)-1].Outcome, true
	}
	if p.DC != 0 {
		if total >= p.DC {
			return "success", true
		}
		return "fail", true
	}
	if p.SuccessOn != "" && len(p.Outcomes) > 0 {
		for _, o := range p.Outcomes {
			if successes >= o.Min && (o.Max < 0 || successes <= o.Max) {
				return o.Outcome, true
			}
		}
	}
	return "", false
}

// ClampTo returns value when it is in the allowed list, otherwise the first
// allowed value, or empty when the list is empty. It keeps a GM-named position
// or effect within a profile's vocabulary.
func ClampTo(allowed []string, value string) string {
	if len(allowed) == 0 {
		return ""
	}
	for _, candidate := range allowed {
		if candidate == value {
			return value
		}
	}
	return allowed[0]
}
