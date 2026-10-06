package harness

import "strings"

// PersonaDecl is a character (or other entity) the GM introduces or references.
type PersonaDecl struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	New         bool     `json:"new,omitempty"`
	Reveals     string   `json:"reveals,omitempty"`
	Replaces    string   `json:"replaces,omitempty"`
	Identifies  string   `json:"identifies,omitempty"`
	Gender      string   `json:"gender,omitempty"`
	Pronouns    string   `json:"pronouns,omitempty"`
	RoleTags    []string `json:"role_tags,omitempty"`
	Description string   `json:"description,omitempty"`
	VoiceHint   string   `json:"voice_hint,omitempty"`
}

// PreviousIdentity returns the generic or earlier name this persona reveals or replaces, if any.
func (p PersonaDecl) PreviousIdentity() string {
	if p.Reveals != "" {
		return strings.TrimSpace(p.Reveals)
	}
	if p.Replaces != "" {
		return strings.TrimSpace(p.Replaces)
	}
	if p.Identifies != "" {
		return strings.TrimSpace(p.Identifies)
	}
	return ""
}

// MemoryDecl is a narrative memory the GM attaches to entities.
type MemoryDecl struct {
	Kind       string   `json:"kind"` // event|relationship|discovery|dialogue
	EntityRefs []string `json:"entity_refs"`
	Text       string   `json:"text"`
	Importance int      `json:"importance"` // 1-5
	Tags       []string `json:"tags,omitempty"`
}

// StateChangeDecl is a proposed change to an entity's state.
type StateChangeDecl struct {
	Entity string      `json:"entity"`
	Path   string      `json:"path"`
	Op     string      `json:"op"` // set|add|sub
	Value  interface{} `json:"value"`
	Reason string      `json:"reason,omitempty"`
}

// CheckModifier is one named adjustment to a check's total.
type CheckModifier struct {
	Source string `json:"source"`
	Value  int    `json:"value"`
	Reason string `json:"reason,omitempty"`
}

// AppliedModifier is one contribution to a resolved check total.
type AppliedModifier struct {
	Source string `json:"source"`
	Value  int    `json:"value"`
}

// CheckRequest is a mid-turn request to resolve a check.
type CheckRequest struct {
	Actor     string `json:"actor"`
	Target    string `json:"target,omitempty"`
	CheckKind string `json:"check_kind"`
	Stat      string `json:"stat,omitempty"`
	// Skill names a declared skill whose rating is added, alongside Stat.
	Skill string `json:"skill,omitempty"`
	// Modifiers are situational adjustments the GM names. Their sum is added.
	Modifiers  []CheckModifier   `json:"modifiers,omitempty"`
	Difficulty string            `json:"difficulty,omitempty"`
	Stakes     string            `json:"stakes"`
	Outcomes   map[string]string `json:"outcomes"`
	Notation   string            `json:"notation,omitempty"`
	// Profile names a resolution profile from the system's checks. Empty uses the
	// system's default conventions.
	Profile string `json:"profile,omitempty"`
	// Position and Effect are the Blades-style stakes a profile may define.
	Position string `json:"position,omitempty"`
	Effect   string `json:"effect,omitempty"`
}

// DieFace is one die as it landed. Symbol is the notation's own way of showing
// that face, so a Fate die reads as a blank or a plus rather than as 0 or 1.
type DieFace struct {
	Value  int    `json:"value"`
	Symbol string `json:"symbol,omitempty"`
}

// RollSummary is harness's view of a die roll, so the protocol types never
// import pkg/rules (rules imports harness and would cycle). Dice carries the
// individual faces, because a total alone cannot be shown honestly.
type RollSummary struct {
	Notation  string    `json:"notation"`
	Total     int       `json:"total"`
	Successes int       `json:"successes"`
	RollCount int       `json:"roll_count"`
	Dice      []DieFace `json:"dice,omitempty"`
}

// CheckResult is the resolved outcome of a CheckRequest.
type CheckResult struct {
	CheckID   string       `json:"check_id"`
	Actor     string       `json:"actor,omitempty"`
	Target    string       `json:"target,omitempty"`
	CheckKind string       `json:"check_kind,omitempty"`
	Stakes    string       `json:"stakes,omitempty"`
	Roll      *RollSummary `json:"roll"`
	Outcome   string       `json:"outcome"`
	// OutcomeText is the system's own description of the outcome, from the
	// request's outcomes map, so a label such as "weak" reads as fiction.
	OutcomeText string `json:"outcome_text,omitempty"`
	// OutcomeVocabulary is the system's declared outcome order, so a client can
	// tone a result without hardcoding pass and fail.
	OutcomeVocabulary []string `json:"outcome_vocabulary,omitempty"`
	// Applied lists every bonus that contributed, for display.
	Applied   []AppliedModifier      `json:"applied,omitempty"`
	Breakdown map[string]interface{} `json:"breakdown,omitempty"`
	// Profile names the resolution profile that decided the outcome, when one did.
	Profile string `json:"profile,omitempty"`
	// Position and Effect are the Blades-style stakes the profile carries.
	Position string `json:"position,omitempty"`
	Effect   string `json:"effect,omitempty"`
	// Successes is the count of dice meeting the pool threshold, when the profile
	// is a success-count pool.
	Successes int `json:"successes,omitempty"`
}

// ProposedCheck is a player's explicit request to roll, carried as structured
// data. Roll mode builds one and the orchestrator turns it into an advisory
// [PROPOSED CHECK] directive for the GM; enforcement is a future concern, so the
// Ref is an identifier, not a contract the GM must honour.
type ProposedCheck struct {
	Ref         string `json:"ref"`
	Actor       string `json:"actor,omitempty"`
	Description string `json:"description,omitempty"`
}

// PendingCheck is a check the GM proposed under the ask policy and the player
// has not yet rolled. It is stored on the turn so a later roll can resolve it.
type PendingCheck struct {
	Ref        string       `json:"ref"`
	Request    CheckRequest `json:"request"`
	ProposedBy string       `json:"proposed_by,omitempty"`
}
