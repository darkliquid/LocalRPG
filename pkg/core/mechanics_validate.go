package core

import (
	"fmt"
	"regexp"
)

// mechanicsIDPattern is the id shape a system's declared elements must match:
// lower-case, starting alphanumeric, then alphanumerics, underscores, or dashes.
var mechanicsIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate reports non-fatal problems with a mechanics block, so a client that
// bypasses the editor cannot persist a broken system. It returns nil when the
// block is absent or sound.
func (m *MechanicsSpec) Validate() []string {
	if m == nil {
		return nil
	}
	var warnings []string

	statIDs := map[string]bool{}
	for _, stat := range m.Stats {
		if !mechanicsIDPattern.MatchString(stat.ID) {
			warnings = append(warnings, fmt.Sprintf("mechanics.stats: %q is not a valid id", stat.ID))
		}
		if statIDs[stat.ID] {
			warnings = append(warnings, fmt.Sprintf("mechanics.stats: duplicate id %q", stat.ID))
		}
		statIDs[stat.ID] = true
	}

	skillIDs := map[string]bool{}
	for _, skill := range m.Skills {
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

	if m.Health != nil {
		if m.Health.Stat != "" && !statIDs[m.Health.Stat] {
			warnings = append(warnings, fmt.Sprintf("mechanics.health: unknown stat %q", m.Health.Stat))
		}
		if m.Health.MaxStat != "" && !statIDs[m.Health.MaxStat] {
			warnings = append(warnings, fmt.Sprintf("mechanics.health: unknown max_stat %q", m.Health.MaxStat))
		}
	}

	warnings = append(warnings, m.Checks.Validate()...)

	difficultyIDs := map[string]bool{}
	for _, difficulty := range m.Checks.Difficulty {
		if !mechanicsIDPattern.MatchString(difficulty.ID) {
			warnings = append(warnings, fmt.Sprintf("mechanics.checks.difficulty: %q is not a valid id", difficulty.ID))
		}
		if difficultyIDs[difficulty.ID] {
			warnings = append(warnings, fmt.Sprintf("mechanics.checks.difficulty: duplicate id %q", difficulty.ID))
		}
		difficultyIDs[difficulty.ID] = true
	}
	for name := range m.Checks.Profiles {
		if !mechanicsIDPattern.MatchString(name) {
			warnings = append(warnings, fmt.Sprintf("mechanics.checks.profiles: %q is not a valid id", name))
		}
	}

	if m.Advancement != nil {
		unlockIDs := map[string]bool{}
		for _, unlock := range m.Advancement.Unlocks {
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
