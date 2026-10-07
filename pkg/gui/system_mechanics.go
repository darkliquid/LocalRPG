package gui

import (
	"fmt"
	"regexp"

	"github.com/darkliquid/localrpg/pkg/core"
)

// mechanicsIDPattern is the id shape a system's declared elements must match:
// lower-case, starting alphanumeric, then alphanumerics, underscores, or dashes.
var mechanicsIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// validateMechanics reports non-fatal problems with a system's mechanics block,
// so a client that bypasses the editor cannot persist a broken system. It returns
// nil when the block is absent or sound.
func validateMechanics(spec *core.MechanicsSpec) []string {
	if spec == nil {
		return nil
	}
	var warnings []string

	statIDs := map[string]bool{}
	for _, stat := range spec.Stats {
		if !mechanicsIDPattern.MatchString(stat.ID) {
			warnings = append(warnings, fmt.Sprintf("mechanics.stats: %q is not a valid id", stat.ID))
		}
		if statIDs[stat.ID] {
			warnings = append(warnings, fmt.Sprintf("mechanics.stats: duplicate id %q", stat.ID))
		}
		statIDs[stat.ID] = true
	}

	skillIDs := map[string]bool{}
	for _, skill := range spec.Skills {
		if !mechanicsIDPattern.MatchString(skill.ID) {
			warnings = append(warnings, fmt.Sprintf("mechanics.skills: %q is not a valid id", skill.ID))
		}
		if skillIDs[skill.ID] {
			warnings = append(warnings, fmt.Sprintf("mechanics.skills: duplicate id %q", skill.ID))
		}
		skillIDs[skill.ID] = true
		if skill.Stat != "" && !statIDs[skill.Stat] {
			warnings = append(warnings, fmt.Sprintf("mechanics.skills.%s: unknown stat %q", skill.ID, skill.Stat))
		}
	}

	if spec.Health != nil {
		if spec.Health.Stat != "" && !statIDs[spec.Health.Stat] {
			warnings = append(warnings, fmt.Sprintf("mechanics.health: unknown stat %q", spec.Health.Stat))
		}
		if spec.Health.MaxStat != "" && !statIDs[spec.Health.MaxStat] {
			warnings = append(warnings, fmt.Sprintf("mechanics.health: unknown max_stat %q", spec.Health.MaxStat))
		}
	}

	warnings = append(warnings, spec.Checks.Validate()...)

	difficultyIDs := map[string]bool{}
	for _, difficulty := range spec.Checks.Difficulty {
		if !mechanicsIDPattern.MatchString(difficulty.ID) {
			warnings = append(warnings, fmt.Sprintf("mechanics.checks.difficulty: %q is not a valid id", difficulty.ID))
		}
		if difficultyIDs[difficulty.ID] {
			warnings = append(warnings, fmt.Sprintf("mechanics.checks.difficulty: duplicate id %q", difficulty.ID))
		}
		difficultyIDs[difficulty.ID] = true
	}
	for name := range spec.Checks.Profiles {
		if !mechanicsIDPattern.MatchString(name) {
			warnings = append(warnings, fmt.Sprintf("mechanics.checks.profiles: %q is not a valid id", name))
		}
	}

	if spec.Advancement != nil {
		unlockIDs := map[string]bool{}
		for _, unlock := range spec.Advancement.Unlocks {
			if !mechanicsIDPattern.MatchString(unlock.ID) {
				warnings = append(warnings, fmt.Sprintf("mechanics.advancement.unlocks: %q is not a valid id", unlock.ID))
			}
			if unlockIDs[unlock.ID] {
				warnings = append(warnings, fmt.Sprintf("mechanics.advancement.unlocks: duplicate id %q", unlock.ID))
			}
			unlockIDs[unlock.ID] = true
			for _, effect := range unlock.Effects {
				if effect.Stat != "" && !statIDs[effect.Stat] {
					warnings = append(warnings, fmt.Sprintf("mechanics.advancement.unlocks.%s: unknown stat %q", unlock.ID, effect.Stat))
				}
			}
		}
	}

	return warnings
}
