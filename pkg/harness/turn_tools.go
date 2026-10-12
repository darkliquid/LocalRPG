package harness

import (
	"encoding/json"
	"fmt"
)

// arrayProperty is one JSON Schema array parameter.

// TurnToolSpecsFor returns the roll tools a mechanics policy offers: request_check
// under auto, propose_check under ask, and neither under off. A turn's prose and
// its control records are the turn itself, not a tool call.
func TurnToolSpecsFor(engagement string) []ToolSpec {
	switch engagement {
	case "off":
		return nil
	case "ask":
		return []ToolSpec{proposeCheckSpec()}
	default:
		return []ToolSpec{requestCheckSpec()}
	}
}

// TurnToolSpecs is the auto policy's tool surface, for callers that do not carry
// a policy.
func TurnToolSpecs() []ToolSpec { return TurnToolSpecsFor("auto") }

func requestCheckSpec() ToolSpec {
	return ToolSpec{
		Name:        "request_check",
		Description: "Resolve a check before continuing: state the stakes and possible outcomes, and the engine rolls and returns one outcome. Name a skill as well as a stat when the check tests a trained ability, and list situational modifiers. Call it, then keep narrating. The engine renders the roll for the player, so never restate the dice, the total, or the modifiers in your prose.",
		Parameters: objectSchema(map[string]any{
			"actor":      stringProperty("The entity attempting the action."),
			"target":     stringProperty("Optional opposing entity; for an opposed check, the opponent."),
			"check_kind": stringProperty("The kind of check, mapped to a system convention, for example 'skill'."),
			"stat":       stringProperty("The stat used."),
			"skill":      stringProperty("A declared skill whose rating is added, alongside stat."),
			"difficulty": stringProperty("Optional difficulty id from the system."),
			"stakes":     stringProperty("What is at stake if the check fails."),
			"outcomes":   map[string]any{"type": "object", "description": "Map of outcome key to the result text, for example {'pass': '...', 'fail': '...'}.", "additionalProperties": map[string]any{"type": "string"}},
			"notation":   stringProperty("Optional dice notation override, for example '2d6'."),
			"profile":    stringProperty("Optional named resolution profile from the system, for example 'pbta' or 'd20'."),
			"position":   stringProperty("Optional position from a blades-style profile, for example 'risky'."),
			"effect":     stringProperty("Optional effect from a blades-style profile, for example 'limited'."),
			"opposed":    stringProperty("Optional stat the opponent rolls when the check is opposed, for example 'might'. Name the opponent in target."),
			"modifiers": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"source": map[string]any{"type": "string"},
						"value":  map[string]any{"type": "integer"},
						"reason": map[string]any{"type": "string"},
					},
					"required": []string{"source", "value"},
				},
				"description": "Situational bonuses and penalties, each named.",
			},
		}, "actor", "check_kind", "stakes", "outcomes"),
	}
}

func proposeCheckSpec() ToolSpec {
	return ToolSpec{
		Name:        "propose_check",
		Description: "Propose a check to the player: state the stakes and the possible outcomes, then stop. Name a skill as well as a stat when the check tests a trained ability, and list situational modifiers. The player rolls and you adjudicate the result in the next turn. Do not resolve it yourself.",
		Parameters: objectSchema(map[string]any{
			"actor":      stringProperty("The entity attempting the action."),
			"target":     stringProperty("Optional opposing entity; for an opposed check, the opponent."),
			"check_kind": stringProperty("The kind of check, mapped to a system convention, for example 'skill'."),
			"stat":       stringProperty("The stat used."),
			"skill":      stringProperty("A declared skill whose rating is added, alongside stat."),
			"difficulty": stringProperty("Optional difficulty id from the system."),
			"stakes":     stringProperty("What is at stake if the check fails."),
			"outcomes":   map[string]any{"type": "object", "description": "Map of outcome key to the result text, for example {'pass': '...', 'fail': '...'}.", "additionalProperties": map[string]any{"type": "string"}},
			"notation":   stringProperty("Optional dice notation override, for example '2d6'."),
			"profile":    stringProperty("Optional named resolution profile from the system, for example 'pbta' or 'd20'."),
			"position":   stringProperty("Optional position from a blades-style profile, for example 'risky'."),
			"effect":     stringProperty("Optional effect from a blades-style profile, for example 'limited'."),
			"opposed":    stringProperty("Optional stat the opponent rolls when the check is opposed, for example 'might'. Name the opponent in target."),
			"modifiers": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"source": map[string]any{"type": "string"},
						"value":  map[string]any{"type": "integer"},
						"reason": map[string]any{"type": "string"},
					},
					"required": []string{"source", "value"},
				},
				"description": "Situational bonuses and penalties, each named.",
			},
		}, "actor", "check_kind", "stakes", "outcomes"),
	}
}

// TurnToolNames lists every turn tool, regardless of policy, so tool dispatch and
// parsing recognise one a model emits anyway.
func TurnToolNames() []string {
	return []string{"request_check", "propose_check"}
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
