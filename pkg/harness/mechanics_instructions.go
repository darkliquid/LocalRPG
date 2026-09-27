package harness

import (
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
)

// FormatMechanicsInstructions is the engine's standing instruction to the GM
// about when to roll. It is generated whenever a system ships mechanics, it
// reflects the engagement policy, and it cites the system's declared resolution
// when it has one. It is deliberately system-agnostic: the engine still knows no
// rules, only how to ask for a check.
func FormatMechanicsInstructions(spec *core.MechanicsSpec, engagement string) string {
	var sb strings.Builder
	switch engagement {
	case "off":
		sb.WriteString("## RESOLVING UNCERTAINTY\n")
		sb.WriteString("Mechanics are disabled for this campaign. Do not roll, and do not call request_check or propose_check. ")
		sb.WriteString("Decide outcomes from the fiction and narrate consequences directly.\n")
	case "ask":
		sb.WriteString("## RESOLVING UNCERTAINTY\n")
		sb.WriteString("When an action has a chance of consequences, call propose_check with the stakes and the possible outcomes, then stop. ")
		sb.WriteString("Do not resolve it yourself; the player rolls and you adjudicate the result.\n")
	default:
		sb.WriteString("## RESOLVING UNCERTAINTY\n")
		sb.WriteString("Resolve with request_check before narrating whenever an outcome could cost or grant something the player would care about: ")
		sb.WriteString("harm, resources, standing, or a lasting change. ")
		sb.WriteString("State the stakes and the possible outcomes first. Do not roll for safe or trivial actions. ")
		sb.WriteString("NPCs do not roll; resolve opposition through the protagonist's check. ")
		sb.WriteString("Honour the outcome the engine returns.\n")
	}

	if spec == nil {
		return sb.String()
	}
	if notation := strings.TrimSpace(spec.Checks.Notation); notation != "" {
		sb.WriteString("Default notation: " + notation + ".\n")
	}
	if len(spec.Checks.Outcome) > 0 {
		sb.WriteString("Outcome vocabulary: " + strings.Join(spec.Checks.Outcome, ", ") + ".\n")
	}
	if len(spec.Checks.Difficulty) > 0 {
		parts := make([]string, 0, len(spec.Checks.Difficulty))
		for _, d := range spec.Checks.Difficulty {
			label := d.Label
			if label == "" {
				label = d.ID
			}
			parts = append(parts, label+" "+strconv.Itoa(d.Target))
		}
		sb.WriteString("Difficulties: " + strings.Join(parts, ", ") + ".\n")
	}
	return sb.String()
}
