package systemtest

import (
	"sort"

	"github.com/darkliquid/localrpg/pkg/core"
)

// SmokeScenario returns the minimal scenario every system must pass: the system
// loads, and a check resolves to an outcome its own declared vocabulary names.
// It is generated from the system's mechanics, so it tests what the system
// declares. A schema-agnostic system yields an empty scenario, which passes
// trivially.
func SmokeScenario(mechanics *core.MechanicsSpec) Scenario {
	scenario := Scenario{Name: "smoke"}
	if mechanics == nil {
		return scenario
	}

	profile, vocabulary := smokeCheck(mechanics)
	check := Step{Action: "check", Input: profile}
	if len(vocabulary) > 0 {
		check.Expect.OutcomeOneOf = vocabulary
	}
	scenario.Steps = append(scenario.Steps, check)

	if len(mechanics.Stats) > 0 {
		stat := mechanics.Stats[0]
		scenario.Setup.Player.Stats = map[string]any{stat.ID: smokeStatValue(stat)}
		scenario.Steps = append(scenario.Steps, Step{Action: "do", Input: stat.ID})
	}
	return scenario
}

// smokeCheck picks the profile the scenario names and the outcome vocabulary it
// must produce. It prefers a declared profile, and falls back to the system's
// default conventions for the profile's vocabulary and for the profile name.
func smokeCheck(mechanics *core.MechanicsSpec) (string, []string) {
	if len(mechanics.Checks.Profiles) == 0 {
		return "", mechanics.Checks.Outcome
	}
	names := make([]string, 0, len(mechanics.Checks.Profiles))
	for name := range mechanics.Checks.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	vocabulary := profileVocabulary(mechanics.Checks.Profiles[names[0]])
	if len(vocabulary) == 0 {
		vocabulary = mechanics.Checks.Outcome
	}
	return names[0], vocabulary
}

// profileVocabulary lists a profile's outcomes, strongest first, so the smoke
// check can assert an outcome the profile declares.
func profileVocabulary(profile core.ResolutionProfile) []string {
	switch {
	case len(profile.Ladder) > 0:
		steps := append([]core.LadderStep(nil), profile.Ladder...)
		sort.SliceStable(steps, func(i, j int) bool { return steps[i].Min > steps[j].Min })
		out := make([]string, 0, len(steps))
		for _, step := range steps {
			if step.Outcome != "" {
				out = append(out, step.Outcome)
			}
		}
		return out
	case len(profile.Outcomes) > 0:
		outcomes := append([]core.SuccessOutcome(nil), profile.Outcomes...)
		sort.SliceStable(outcomes, func(i, j int) bool { return outcomes[i].Min > outcomes[j].Min })
		out := make([]string, 0, len(outcomes))
		for _, outcome := range outcomes {
			if outcome.Outcome != "" {
				out = append(out, outcome.Outcome)
			}
		}
		return out
	}
	return nil
}

// smokeStatValue gives a declared stat a usable starting value: its declared
// default when it has one, and 1 otherwise.
func smokeStatValue(stat core.StatSpec) any {
	if stat.Default != nil {
		return stat.Default
	}
	return 1
}
