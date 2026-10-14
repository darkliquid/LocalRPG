package harness

import (
	"sort"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
)

// StatValue is one of the player's declared stats and its current value, so the
// resolution instruction can name what a check may test.
type StatValue struct {
	ID    string
	Label string
	Value int
}

// FormatMechanicsInstructions is the engine's standing instruction to the GM
// about when to roll. It is generated whenever a system ships mechanics, it
// reflects the engagement policy, and it cites the system's declared resolution
// when it has one. It is deliberately system-agnostic: the engine still knows no
// rules, only how to ask for a check. stats, when supplied, are the player's
// declared stats and their current values.
// FormatMechanicsInstructions is the engine's standing instruction to the GM
// about when to roll. It is generated whenever a system ships mechanics, it
// reflects the engagement policy, and it cites the system's declared resolution
// when it has one. It is deliberately system-agnostic: the engine still knows no
// rules, only how to ask for a check. stats, when supplied, are the player's
// declared stats and their current values.
func FormatMechanicsInstructions(spec *core.MechanicsSpec, engagement string, stats []StatValue) string {
	var sb strings.Builder
	writeEngagementInstruction(&sb, engagement)
	writeStatLine(&sb, stats)

	if spec == nil {
		return sb.String()
	}

	writeSkillsLine(&sb, spec.Skills)
	if notation := strings.TrimSpace(spec.Checks.Notation); notation != "" {
		sb.WriteString("Default notation: " + notation + ".\n")
	}
	if len(spec.Checks.Outcome) > 0 {
		sb.WriteString("Outcome vocabulary: " + strings.Join(spec.Checks.Outcome, ", ") + ".\n")
	}
	writeDifficultyLine(&sb, spec.Checks.Difficulty)
	writeProfileLines(&sb, spec.Checks.Profiles)

	return sb.String()
}

// writeEngagementInstruction is the paragraph that tells the GM when to roll, which the
// campaign's engagement policy decides: nothing at all, a proposal the player answers, or
// a check the engine resolves.
func writeEngagementInstruction(sb *strings.Builder, engagement string) {
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
		sb.WriteString("Name the skill as well as the stat when a check tests a trained ability, and list situational modifiers. ")
		sb.WriteString("NPCs do not roll; resolve opposition through the protagonist's check. ")
		sb.WriteString("Call request_check, or emit a @roll record, for any uncertain action; never narrate a resolution the engine has not given you. ")
		sb.WriteString("Honour the outcome the engine returns. ")
		sb.WriteString("Do not restate the dice: never write notation, totals, modifiers, or the arithmetic of a roll in your prose, because the engine renders the roll beside your words. Narrate only what the outcome means for the fiction.\n")
	}
}

// writeStatLine names the player's declared stats and their current values, so the
// instruction above can cite something the player actually has.
func writeStatLine(sb *strings.Builder, stats []StatValue) {
	if len(stats) == 0 {
		return
	}
	parts := make([]string, 0, len(stats))
	for _, stat := range stats {
		label := stat.Label
		if label == "" {
			label = stat.ID
		}
		parts = append(parts, label+" "+strconv.Itoa(stat.Value))
	}
	sb.WriteString("Player stats: " + strings.Join(parts, ", ") + ".\n")
}

// writeSkillsLine names the system's skills, which are the trained abilities a check may
// ask the player to use.
func writeSkillsLine(sb *strings.Builder, skills []core.SkillSpec) {
	if len(skills) == 0 {
		return
	}
	parts := make([]string, 0, len(skills))
	for _, skill := range skills {
		label := skill.Label
		if label == "" {
			label = skill.ID
		}
		parts = append(parts, label)
	}
	sb.WriteString("Skills: " + strings.Join(parts, ", ") + ".\n")
}

// writeDifficultyLine names the system's difficulty rungs with the number each one asks
// for, which is what makes a stated difficulty mean the same thing at the table.
func writeDifficultyLine(sb *strings.Builder, difficulties []core.DifficultySpec) {
	if len(difficulties) == 0 {
		return
	}
	parts := make([]string, 0, len(difficulties))
	for _, d := range difficulties {
		label := d.Label
		if label == "" {
			label = d.ID
		}
		parts = append(parts, label+" "+strconv.Itoa(d.Target))
	}
	sb.WriteString("Difficulties: " + strings.Join(parts, ", ") + ".\n")
}

// writeProfileLines names the system's resolution profiles in a stable order, and says
// what an opposed one means for the check the GM asks for.
func writeProfileLines(sb *strings.Builder, profiles map[string]core.ResolutionProfile) {
	if len(profiles) == 0 {
		return
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	opposed := false
	for _, name := range names {
		profile := profiles[name]
		label := profile.Label
		if label == "" {
			label = name
		}
		if profile.Opposed != "" {
			label += " (opposed: " + profile.Opposed + ")"
			opposed = true
		}
		parts = append(parts, label)
	}
	sb.WriteString("Resolution profiles: " + strings.Join(parts, ", ") + ".\n")
	if opposed {
		sb.WriteString("An opposed profile rolls for the opponent too: name the opponent in target and the stat it rolls in opposed.\n")
	}
}
