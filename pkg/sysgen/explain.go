package sysgen

import (
	"context"
	"fmt"
	"strings"
)

const explainSchema = `{"type":"object","properties":{"explanation":{"type":"string"}},"required":["explanation"]}`

// Explain returns a plain-language description of a system's mechanics: how a
// check resolves, what the stats mean, how advancement works, and what the hooks
// do. It is prose for a person, not a machine.
func Explain(ctx context.Context, gen Generator, sys System) (string, error) {
	if gen == nil {
		return "", fmt.Errorf("sysgen: no generator configured")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var out struct {
		Explanation string `json:"explanation"`
	}
	if err := generateJSON(ctx, gen, explainPrompt(sys), explainSchema, &out); err != nil {
		return "", fmt.Errorf("explain: %w", err)
	}
	return strings.TrimSpace(out.Explanation), nil
}

func explainPrompt(sys System) string {
	var b strings.Builder
	b.WriteString("Explain this tabletop RPG system's mechanics in plain language for a person reading the rules. Cover how a check resolves, what each stat means, how advancement works, and what any script hooks do. Do not invent mechanics that are not present.\n")
	if sys.Name != "" {
		fmt.Fprintf(&b, "\nSystem Name: %s\n", sys.Name)
	}
	if sys.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", sys.Description)
	}
	if summary := mechanicsSummary(sys.Mechanics); summary != "" {
		b.WriteString("\nMechanics:\n" + summary)
	} else {
		b.WriteString("\nMechanics: none declared; the system is schema-agnostic.\n")
	}
	if strings.TrimSpace(sys.Script) != "" {
		b.WriteString("\nScript hooks:\n" + truncateText(sys.Script, 2000) + "\n")
	}
	if strings.TrimSpace(sys.RulesPrompt) != "" {
		b.WriteString("\nRules prose:\n" + truncateText(sys.RulesPrompt, 2000) + "\n")
	}
	b.WriteString("\nReturn a short explanation in Markdown.\n")
	return b.String()
}
