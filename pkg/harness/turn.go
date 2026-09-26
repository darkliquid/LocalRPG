package harness

import (
	"encoding/json"
	"fmt"
)

// ActionFeasibility is the GM's verdict on the player's action.
type ActionFeasibility string

const (
	FeasibilityAutomatic  ActionFeasibility = "automatic"
	FeasibilityUncertain  ActionFeasibility = "uncertain"
	FeasibilityImpossible ActionFeasibility = "impossible"
)

// ActionVerdict states whether the player's action was automatic, uncertain, or
// impossible, with a reason.
type ActionVerdict struct {
	Feasibility ActionFeasibility `json:"feasibility"`
	Reason      string            `json:"reason,omitempty"`
}

// SegmentSpec is one authored narration or speech segment.
type SegmentSpec struct {
	Kind     string `json:"kind"`               // "narration" | "speech"
	Speaker  string `json:"speaker,omitempty"`  // name or id, speech only
	Text     string `json:"text"`
	CheckRef string `json:"check_ref,omitempty"`
}

// PersonaDecl is a character (or other entity) the GM introduces or references.
type PersonaDecl struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	New         bool     `json:"new,omitempty"`
	Gender      string   `json:"gender,omitempty"`
	Pronouns    string   `json:"pronouns,omitempty"`
	RoleTags    []string `json:"role_tags,omitempty"`
	Description string   `json:"description,omitempty"`
	VoiceHint   string   `json:"voice_hint,omitempty"`
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

// CheckRequest is a mid-turn request to resolve a check.
type CheckRequest struct {
	Actor      string            `json:"actor"`
	Target     string            `json:"target,omitempty"`
	CheckKind  string            `json:"check_kind"`
	Stat       string            `json:"stat,omitempty"`
	Difficulty string            `json:"difficulty,omitempty"`
	Stakes     string            `json:"stakes"`
	Outcomes   map[string]string `json:"outcomes"`
	Notation   string            `json:"notation,omitempty"`
}

// RollSummary is harness's view of a die roll, so the protocol types never
// import pkg/rules (rules imports harness and would cycle).
type RollSummary struct {
	Notation  string `json:"notation"`
	Total     int    `json:"total"`
	Successes int    `json:"successes"`
	RollCount int    `json:"roll_count"`
}

// CheckResult is the resolved outcome of a CheckRequest.
type CheckResult struct {
	CheckID   string                 `json:"check_id"`
	Actor     string                 `json:"actor,omitempty"`
	Target    string                 `json:"target,omitempty"`
	CheckKind string                 `json:"check_kind,omitempty"`
	Stakes    string                 `json:"stakes,omitempty"`
	Roll      *RollSummary           `json:"roll"`
	Outcome   string                 `json:"outcome"`
	Breakdown map[string]interface{} `json:"breakdown,omitempty"`
}

// DismissedCheck records a player-proposed check the GM chose not to resolve.
type DismissedCheck struct {
	CheckRef string `json:"check_ref"`
	Reason   string `json:"reason"`
}

// ProposedCheck is a player's explicit request to roll, carried as structured
// data so the engine can require the GM to resolve or dismiss it. Ref is the
// stable id the GM references in dismissed_checks.
type ProposedCheck struct {
	Ref         string `json:"ref"`
	Actor       string `json:"actor,omitempty"`
	Description string `json:"description,omitempty"`
}

// TurnSubmission is the terminal payload the GM authors for one turn.
type TurnSubmission struct {
	Verdict         ActionVerdict     `json:"action_verdict"`
	Segments        []SegmentSpec     `json:"segments"`
	Personae        []PersonaDecl     `json:"personae,omitempty"`
	Memories        []MemoryDecl      `json:"memories,omitempty"`
	StateChanges    []StateChangeDecl `json:"state_changes,omitempty"`
	PlayerLocation  string            `json:"player_location,omitempty"`
	DismissedChecks []DismissedCheck  `json:"dismissed_checks,omitempty"`
}

// ParseSubmission decodes a submit_turn argument object.
func ParseSubmission(args string) (*TurnSubmission, error) {
	var sub TurnSubmission
	if err := json.Unmarshal([]byte(args), &sub); err != nil {
		return nil, fmt.Errorf("parse submit_turn: %w", err)
	}
	return &sub, nil
}
