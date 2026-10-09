package core

// ResolutionProfile maps a roll to an outcome for one style of check. A system
// declares named profiles and the GM names one per check, so a single system can
// express a PbtA ladder, a d20 difficulty class, a success-count pool, or a
// position/effect style without the engine knowing any of them.
type ResolutionProfile struct {
	// Label is the human-facing name shown in instructions and the UI.
	Label string `yaml:"label,omitempty"`
	// Notation overrides the system's default dice notation for this profile.
	Notation string `yaml:"notation,omitempty"`
	// DC is the difficulty class: a total at or above it is a success.
	DC int `yaml:"dc,omitempty"`
	// Ladder is a set of thresholds, resolved highest-first.
	Ladder []LadderStep `yaml:"ladder,omitempty"`
	// SuccessOn is a pool comparison, for example ">=8", that marks a die as a
	// success. It is paired with Outcomes.
	SuccessOn string `yaml:"success_on,omitempty"`
	// Outcomes maps a success count to an outcome for a pool profile.
	Outcomes []SuccessOutcome `yaml:"outcomes,omitempty"`
	// Position is the allowed position vocabulary for a blades-style profile.
	Position []string `yaml:"position,omitempty"`
	// Effect is the allowed effect vocabulary for a blades-style profile.
	Effect []string `yaml:"effect,omitempty"`
	// Opposed names the stat the opponent rolls when a check uses this profile
	// and names no opponent stat of its own. Empty means the profile is not
	// opposed by default.
	Opposed string `yaml:"opposed,omitempty"`
	// Ties decides an opposed check whose totals are equal: "opponent" hands it
	// to the opponent, and empty or "actor" keeps it with the actor.
	Ties string `yaml:"ties,omitempty"`
}

// Tie rules for an opposed check.
const (
	// TieActor keeps a tied opposed check with the actor, which favours the
	// player in an ambiguous case. It is the default.
	TieActor = "actor"
	// TieOpponent hands a tied opposed check to the opponent.
	TieOpponent = "opponent"
)

// ValidTieRule reports whether rule is a tie rule this build understands.
func ValidTieRule(rule string) bool {
	switch rule {
	case "", TieActor, TieOpponent:
		return true
	default:
		return false
	}
}

// LadderStep is one threshold on a resolution ladder. The step with the highest
// Min that the total meets decides.
type LadderStep struct {
	Min     int    `yaml:"min"`
	Outcome string `yaml:"outcome"`
}

// SuccessOutcome maps a range of success counts to an outcome for a pool
// profile. A Max of -1 means unbounded above.
type SuccessOutcome struct {
	Min     int    `yaml:"min"`
	Max     int    `yaml:"max"`
	Outcome string `yaml:"outcome"`
}

// Validate reports problems with the declared profiles, so a malformed system is
// rejected when it loads rather than when a check is resolved. It returns nil
// when every profile is usable.
func (c CheckConventions) Validate() []string {
	var problems []string
	for name, p := range c.Profiles {
		switch {
		case p.DC != 0:
			// DC form: nothing else is required.
		case len(p.Outcomes) > 0:
			if p.SuccessOn == "" {
				problems = append(problems, "checks.profiles."+name+": outcomes without success_on")
			}
		case len(p.Ladder) > 0:
			// Ladder form.
		default:
			problems = append(problems, "checks.profiles."+name+": needs a dc, a ladder, or a pool")
		}
		for _, step := range p.Ladder {
			if step.Outcome == "" {
				problems = append(problems, "checks.profiles."+name+": a ladder step has no outcome")
			}
		}
		if !ValidTieRule(p.Ties) {
			problems = append(problems, "checks.profiles."+name+": ties must be actor or opponent")
		}
	}
	return problems
}
