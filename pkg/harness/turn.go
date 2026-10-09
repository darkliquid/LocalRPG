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
	// Opposed, when set, names the stat the opponent rolls with. Target then
	// names the opponent, and the check rolls both sides and compares the
	// totals. Empty keeps the fixed-difficulty path.
	Opposed string `json:"opposed,omitempty"`
	// ForcedTotal, when set, is the dice total a player entered rather than
	// rolled. The check's bonuses apply after it, so a player enters the dice and
	// the system still adds what it would have added.
	ForcedTotal *int `json:"-"`
	// ForcedDice, when set, are the individual dice a player entered. The server
	// sums them and records them, so the chronicle can show the faces.
	ForcedDice []int `json:"-"`
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
	// Source is "manual" when the total came from a forced entry rather than the
	// dice, so the chronicle can say so.
	Source string `json:"source,omitempty"`
	// OpposedRoll is the opponent's roll and total, when the check was opposed,
	// so the contest can be shown as one. OpposedActor names the opponent.
	OpposedRoll  *RollSummary `json:"opposed_roll,omitempty"`
	OpposedTotal int          `json:"opposed_total,omitempty"`
	OpposedActor string       `json:"opposed_actor,omitempty"`
}

// CounterProposal is a player's argument about a pending check: a reframed
// approach, a restatement of the stakes, or a different difficulty.
type CounterProposal struct {
	Approach   string `json:"approach,omitempty"`
	Stakes     string `json:"stakes,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
}

// The rulings a counter-proposal may receive.
const (
	// RulingAccept adopts the counter's terms.
	RulingAccept = "accept"
	// RulingAdjust adopts modified terms, with a reason.
	RulingAdjust = "adjust"
	// RulingHold keeps the original terms, with a reason.
	RulingHold = "hold"
)

// Adjudication is the GM's ruling on a counter-proposal. Stakes, Difficulty,
// Notation, and Profile carry the agreed terms; Reason explains the ruling, and
// is the whole of a hold.
type Adjudication struct {
	Ruling     string `json:"ruling"`
	Stakes     string `json:"stakes,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
	Notation   string `json:"notation,omitempty"`
	Profile    string `json:"profile,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// Agreed reports whether the ruling changed the check's terms.
func (a Adjudication) Agreed() bool {
	return a.Ruling == RulingAccept || a.Ruling == RulingAdjust
}

// NormalizeRuling maps whatever a model wrote to one of the three rulings,
// defaulting to hold so an unparseable answer cannot silently change a check.
func NormalizeRuling(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case RulingAccept, "accepted", "yes":
		return RulingAccept
	case RulingAdjust, "adjusted", "modify", "modified", "partial":
		return RulingAdjust
	default:
		return RulingHold
	}
}

// Negotiation is one counter-proposal and the GM's ruling on it, recorded on the
// turn so the chronicle can show the exchange.
type Negotiation struct {
	Counter CounterProposal `json:"counter"`
	Ruling  Adjudication    `json:"ruling"`
}

// PendingCheck is a check the GM proposed under the ask policy and the player
// has not yet rolled. It is stored on the turn so a later roll can resolve it.
type PendingCheck struct {
	Ref        string       `json:"ref"`
	Request    CheckRequest `json:"request"`
	ProposedBy string       `json:"proposed_by,omitempty"`
}
