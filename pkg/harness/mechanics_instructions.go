package harness

import (
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
)

// FormatMechanicsInstructions is the engine's standing instruction to the GM
// about when to roll. It is generated whenever a system ships mechanics, and it
// cites the system's declared resolution when it has one. It is deliberately
// system-agnostic: the engine still knows no rules, only how to ask for a check.
func FormatMechanicsInstructions(spec *core.MechanicsSpec) string {
	var sb strings.Builder
	sb.WriteString("## RESOLVING UNCERTAINTY\n")
	sb.WriteString("Call request_check when an action is uncertain and failure would change the story. ")
	sb.WriteString("State the stakes and the possible outcomes first. Do not roll for safe or trivial actions. ")
	sb.WriteString("NPCs do not roll; resolve opposition through the protagonist's check. ")
	sb.WriteString("Honour the outcome the engine returns.\n")

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
