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

// PendingCheck is a check the GM proposed under the ask policy and the player
// has not yet rolled. It is stored on the turn so a later roll can resolve it.
type PendingCheck struct {
	Ref        string       `json:"ref"`
	Request    CheckRequest `json:"request"`
	ProposedBy string       `json:"proposed_by,omitempty"`
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

// TurnSubmissionSchema returns the JSON Schema for a TurnSubmission payload.
func TurnSubmissionSchema() map[string]interface{} {
	return objectSchema(map[string]interface{}{
		"action_verdict": objectSchema(map[string]interface{}{
			"feasibility": stringProperty("'automatic', 'uncertain', or 'impossible'."),
			"reason":      stringProperty("Why the action is automatic, uncertain, or impossible."),
		}, "feasibility"),
		"segments": arrayProperty("Ordered narration and speech segments.", objectSchema(map[string]interface{}{
			"kind":      stringProperty("'narration' or 'speech'."),
			"speaker":   stringProperty("Speech only: the speaker's name or id."),
			"text":      stringProperty("The segment's text."),
			"check_ref": stringProperty("Optional: the check id this segment narrates the outcome of."),
		}, "kind", "text")),
		"personae": arrayProperty("Characters and entities this turn introduces or uses.", objectSchema(map[string]interface{}{
			"name":        stringProperty("Display name."),
			"type":        stringProperty("Entity type, for example 'character' or 'location'."),
			"new":         map[string]interface{}{"type": "boolean", "description": "True when this entity is new."},
			"gender":      stringProperty("Optional gender."),
			"pronouns":    stringProperty("Optional pronouns."),
			"role_tags":   arrayProperty("Optional role tags.", stringProperty("A tag.")),
			"description": stringProperty("One-line description."),
			"voice_hint":  stringProperty("Optional voice hint."),
		}, "name", "type")),
		"memories": arrayProperty("Narrative memories to attach to entities.", objectSchema(map[string]interface{}{
			"kind":        stringProperty("event|relationship|discovery|dialogue."),
			"entity_refs": arrayProperty("Entity ids or names this memory concerns.", stringProperty("An entity reference.")),
			"text":        stringProperty("The memory text."),
			"importance":  intProperty("Importance 1-5."),
			"tags":        arrayProperty("Optional tags.", stringProperty("A tag.")),
		}, "kind", "entity_refs", "text", "importance")),
		"state_changes": arrayProperty("Proposed changes to entity state.", objectSchema(map[string]interface{}{
			"entity": stringProperty("Entity id or name."),
			"path":   stringProperty("Dotted state path, for example 'hp'."),
			"op":     stringProperty("set|add|sub."),
			"value":  map[string]interface{}{"description": "The value to set or change by."},
			"reason": stringProperty("Why the state changed."),
		}, "entity", "path", "op", "value")),
		"player_location": stringProperty("Optional wikilink to move the player to."),
		"dismissed_checks": arrayProperty("Player-proposed checks you chose not to resolve.", objectSchema(map[string]interface{}{
			"check_ref": stringProperty("The proposed check reference."),
			"reason":    stringProperty("Why no check was needed."),
		}, "check_ref", "reason")),
	}, "action_verdict", "segments")
}
