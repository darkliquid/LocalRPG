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
		Parameters:  TurnSubmissionSchema(),
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
