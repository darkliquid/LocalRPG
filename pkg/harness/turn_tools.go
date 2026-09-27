package harness

import (
	"encoding/json"
	"fmt"
)

// arrayProperty is one JSON Schema array parameter.
func arrayProperty(description string, item map[string]interface{}) map[string]interface{} {
	if item == nil {
		item = map[string]interface{}{"type": "object"}
	}
	return map[string]interface{}{"type": "array", "description": description, "items": item}
}

// TurnToolSpecsFor returns the narrative-turn tools a mechanics policy offers:
// a terminal submit_turn always, and either request_check (auto) or
// propose_check (ask). The off policy offers neither.
func TurnToolSpecsFor(engagement string) []ToolSpec {
	specs := []ToolSpec{submitTurnSpec()}
	switch engagement {
	case "off":
		return specs
	case "ask":
		return append(specs, proposeCheckSpec())
	default:
		return append(specs, requestCheckSpec())
	}
}

// TurnToolSpecs is the auto policy's tool surface, for callers that do not carry
// a policy.
func TurnToolSpecs() []ToolSpec { return TurnToolSpecsFor("auto") }

func submitTurnSpec() ToolSpec {
	return ToolSpec{
		Name:        "submit_turn",
		Description: "Submit the finished turn: your verdict on the player's action, ordered narration/speech segments, any new personae, memories, and state changes. Begin with a short third-person restatement of the player's action before resolving it. This ends the turn; call it last.",
		Parameters: objectSchema(map[string]interface{}{
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
		}, "action_verdict", "segments"),
	}
}

func requestCheckSpec() ToolSpec {
	return ToolSpec{
		Name:        "request_check",
		Description: "Resolve a check before continuing: state the stakes and possible outcomes, and the engine rolls and returns one outcome. Call it, then keep narrating.",
		Parameters: objectSchema(map[string]interface{}{
			"actor":      stringProperty("The entity attempting the action."),
			"target":     stringProperty("Optional opposing entity."),
			"check_kind": stringProperty("The kind of check, mapped to a system convention, for example 'skill'."),
			"stat":       stringProperty("The stat or skill used."),
			"difficulty": stringProperty("Optional difficulty id from the system."),
			"stakes":     stringProperty("What is at stake if the check fails."),
			"outcomes":   map[string]interface{}{"type": "object", "description": "Map of outcome key to the result text, for example {'pass': '...', 'fail': '...'}.", "additionalProperties": map[string]interface{}{"type": "string"}},
			"notation":   stringProperty("Optional dice notation override, for example '2d6'."),
		}, "actor", "check_kind", "stakes", "outcomes"),
	}
}

func proposeCheckSpec() ToolSpec {
	return ToolSpec{
		Name:        "propose_check",
		Description: "Propose a check to the player: state the stakes and the possible outcomes, then stop. The player rolls and you adjudicate the result in the next turn. Do not resolve it yourself.",
		Parameters: objectSchema(map[string]interface{}{
			"actor":      stringProperty("The entity attempting the action."),
			"target":     stringProperty("Optional opposing entity."),
			"check_kind": stringProperty("The kind of check, mapped to a system convention, for example 'skill'."),
			"stat":       stringProperty("The stat or skill used."),
			"difficulty": stringProperty("Optional difficulty id from the system."),
			"stakes":     stringProperty("What is at stake if the check fails."),
			"outcomes":   map[string]interface{}{"type": "object", "description": "Map of outcome key to the result text, for example {'pass': '...', 'fail': '...'}.", "additionalProperties": map[string]interface{}{"type": "string"}},
			"notation":   stringProperty("Optional dice notation override, for example '2d6'."),
		}, "actor", "check_kind", "stakes", "outcomes"),
	}
}

// TurnToolNames lists every turn tool, regardless of policy, so tool dispatch and
// parsing recognise one a model emits anyway.
func TurnToolNames() []string {
	return []string{"submit_turn", "request_check", "propose_check"}
}

// IsTurnTool reports whether a tool belongs to the turn surface.
func IsTurnTool(name string) bool {
	for _, candidate := range TurnToolNames() {
		if candidate == name {
			return true
		}
	}
	return false
}

// ParseCheckRequest decodes a request_check or propose_check argument object.
func ParseCheckRequest(args string) (*CheckRequest, error) {
	var req CheckRequest
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return nil, fmt.Errorf("parse check request: %w", err)
	}
	return &req, nil
}
