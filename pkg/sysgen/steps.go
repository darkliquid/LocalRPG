package sysgen

import (
	"context"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
)

// templateChoice is the model's classification of a description onto a template.
// The dimensions are a closed set, so this is a classification rather than a
// generation: the model picks, it does not invent structure.
type templateChoice struct {
	Resolution  string `json:"resolution"`
	Health      string `json:"health,omitempty"`
	Advancement string `json:"advancement,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Gaps        string `json:"gaps,omitempty"`
}

const chooseSchema = `{"type":"object","properties":{` +
	`"resolution":{"type":"string"},"health":{"type":"string"},` +
	`"advancement":{"type":"string"},"reason":{"type":"string"},"gaps":{"type":"string"}}}`

func choosePrompt(brief Brief) string {
	var b strings.Builder
	b.WriteString("Choose the closest schema template for a tabletop RPG system. Pick values from the closed sets below; never invent a new one.\n")
	b.WriteString("resolution: ladder (2d6 partial success), dc (roll against a difficulty class), pool (count dice meeting a target).\n")
	b.WriteString("health: single (one health stat), wounds (several wound levels), none (no health).\n")
	b.WriteString("advancement: spend (earn and spend points), track (fill a track), threshold (levels), none.\n")
	if brief.Name != "" {
		fmt.Fprintf(&b, "System Name: %s\n", brief.Name)
	}
	fmt.Fprintf(&b, "Description: %s\n", brief.Description)
	b.WriteString("Return the chosen resolution, health, and advancement, a one-line reason, and in 'gaps' any requested behaviour the templates and escape hatches cannot express (empty when none).\n")
	return b.String()
}

// chooseTemplate classifies a brief onto a catalogue template. A choice that
// does not match a template exactly falls back to the closest one, so the
// pipeline always has a valid shape to build.
func chooseTemplate(ctx context.Context, gen Generator, brief Brief) (Template, templateChoice, error) {
	var choice templateChoice
	if err := generateJSON(ctx, gen, choosePrompt(brief), chooseSchema, &choice); err != nil {
		return Template{}, templateChoice{}, fmt.Errorf("choose: %w", err)
	}
	tpl, ok := MatchTemplate(choice.Resolution, choice.Health, choice.Advancement)
	if !ok {
		return Template{}, templateChoice{}, fmt.Errorf("choose: unknown resolution %q", choice.Resolution)
	}
	return tpl, choice, nil
}

const fillSchema = `{"type":"object","properties":{` +
	`"stats":{"type":"array"},"skills":{"type":"array"},"notation":{"type":"string"},` +
	`"ladder":{"type":"array"},"dc":{"type":"integer"},"success_on":{"type":"string"},` +
	`"outcomes":{"type":"array"},"health_stat":{"type":"string"},` +
	`"advancement_stat":{"type":"string"},"unlocks":{"type":"array"}}}`

func fillPrompt(brief Brief, tpl Template) string {
	var b strings.Builder
	b.WriteString("Fill the parameters for a tabletop RPG system's fixed schema template. Provide values only, never structure.\n")
	fmt.Fprintf(&b, "Template: %s (%s)\n", tpl.ID, tpl.Label)
	fmt.Fprintf(&b, "Resolution: %s\n", tpl.Resolution)
	switch tpl.Resolution {
	case "ladder":
		b.WriteString("Provide a ladder: steps with min and outcome, strongest first (e.g. 10+ strong, 7-9 weak, 6- miss).\n")
	case "dc":
		b.WriteString("Provide a dc (difficulty class) and the default dice notation.\n")
	case "pool":
		b.WriteString("Provide a success_on target (e.g. >=8) and outcomes mapping a success-count range to an outcome.\n")
	}
	if tpl.Health != "" && tpl.Health != "none" {
		b.WriteString("Provide health_stat: the id of the stat that holds health.\n")
	}
	if tpl.Advancement != "" && tpl.Advancement != "none" {
		b.WriteString("Provide advancement_stat: the id of the stat that holds the advancement currency.\n")
	}
	if brief.Name != "" {
		fmt.Fprintf(&b, "System Name: %s\n", brief.Name)
	}
	fmt.Fprintf(&b, "Description: %s\n", brief.Description)
	b.WriteString("Provide stats (id, label), skills (id, label, stat), and the notation.\n")
	return b.String()
}

// fillParams asks the model for the values of the chosen template's parameters.
// The template decides the shape; these values are checked by Template.Build.
func fillParams(ctx context.Context, gen Generator, tpl Template, brief Brief) (Params, error) {
	var params Params
	if err := generateJSON(ctx, gen, fillPrompt(brief, tpl), fillSchema, &params); err != nil {
		return Params{}, fmt.Errorf("fill: %w", err)
	}
	return params, nil
}

// escapeHatches is the closed list of behaviours that may be expressed as
// generated JavaScript. Anything outside it is reported as a note, not script.
var escapeHatches = []string{"resource_spend", "custom_on_check", "custom_on_health_zero", "custom_on_turn_end"}

// hatchKeywords maps each hatch to the phrases in a description that request it.
var hatchKeywords = map[string][]string{
	"resource_spend":        {"spend", "economy", "resource", "mana", "grit point", "point pool", "currency"},
	"custom_on_check":       {"opposed roll", "opposed check", "reroll", "custom check", "on a check"},
	"custom_on_health_zero": {"death", "dying", "when downed", "at zero", "health zero", "knocked out"},
	"custom_on_turn_end":    {"each turn", "per turn", "turn end", "end of turn", "world tick"},
}

// requestedHatches returns the escape hatches a brief asks for, in the closed
// list's order. A brief that requests none yields an empty slice.
func requestedHatches(brief Brief) []string {
	description := strings.ToLower(brief.Description)
	var out []string
	for _, hatch := range escapeHatches {
		for _, keyword := range hatchKeywords[hatch] {
			if strings.Contains(description, keyword) {
				out = append(out, hatch)
				break
			}
		}
	}
	return out
}

type hookEntry struct {
	Event string `json:"event,omitempty"`
	Name  string `json:"name,omitempty"`
	Code  string `json:"code,omitempty"`
	Raw   string `json:"raw,omitempty"`
}

type hooksOutput struct {
	Hooks []hookEntry `json:"hooks"`
}

const hooksSchema = `{"type":"object","properties":{"hooks":{"type":"array","items":{"type":"object","properties":{"event":{"type":"string"},"name":{"type":"string"},"code":{"type":"string"},"raw":{"type":"string"}}}}}}`

func hooksPrompt(brief Brief, spec *core.MechanicsSpec, hatches []string) string {
	var b strings.Builder
	b.WriteString("Write the mechanics.js script hooks for this tabletop RPG system.\n")
	b.WriteString("CRITICAL: JavaScript is generated only for the escape hatches listed below. If the list is empty, return {\"hooks\": []}.\n")
	if len(hatches) == 0 {
		b.WriteString("Escape hatches requested: none.\n")
	} else {
		fmt.Fprintf(&b, "Escape hatches requested: %s\n", strings.Join(hatches, ", "))
	}
	if brief.Name != "" {
		fmt.Fprintf(&b, "System Name: %s\n", brief.Name)
	}
	fmt.Fprintf(&b, "Description: %s\n", brief.Description)
	if spec != nil {
		fmt.Fprintf(&b, "Declared stats: %d, skills: %d\n", len(spec.Stats), len(spec.Skills))
	}
	b.WriteString("For each hook, specify event ('action', 'turn_begin', 'turn_end', 'world_tick', 'check', 'health_zero'), optional name (e.g. action type or check kind), and the JS code or body, or use 'raw' for raw JS definitions.\n")
	return b.String()
}

func assembleScript(hooks []hookEntry) string {
	if len(hooks) == 0 {
		return ""
	}
	var b strings.Builder
	for _, h := range hooks {
		if strings.TrimSpace(h.Raw) != "" {
			b.WriteString(strings.TrimSpace(h.Raw))
			b.WriteString("\n\n")
			continue
		}
		event := strings.ToLower(strings.TrimSpace(h.Event))
		code := strings.TrimSpace(h.Code)
		name := strings.TrimSpace(h.Name)

		switch event {
		case "action":
			if name == "" {
				name = "do"
			}
			fmt.Fprintf(&b, "onAction(%q, function(ctx) {\n  %s\n});\n\n", name, code)
		case "turn_begin", "turnbegin":
			fmt.Fprintf(&b, "onTurnBegin(function(ctx) {\n  %s\n});\n\n", code)
		case "turn_end", "turnend":
			fmt.Fprintf(&b, "onTurnEnd(function(ctx) {\n  %s\n});\n\n", code)
		case "world_tick", "worldtick":
			fmt.Fprintf(&b, "onWorldTick(function(ctx) {\n  %s\n});\n\n", code)
		case "check":
			if name == "" {
				name = "default"
			}
			fmt.Fprintf(&b, "onCheck(%q, function(req) {\n  %s\n});\n\n", name, code)
		case "health_zero", "healthzero":
			fmt.Fprintf(&b, "onHealthZero(function(effect) {\n  %s\n});\n\n", code)
		default:
			// Fallback: if raw or code is provided without a recognized event
			if code != "" {
				b.WriteString(code)
				b.WriteString("\n\n")
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// runHooks prompts the generator for the JavaScript hooks that implement the
// requested escape hatches and assembles them into a mechanics.js body.
func runHooks(ctx context.Context, gen Generator, brief Brief, spec *core.MechanicsSpec, hatches []string) (string, error) {
	var out hooksOutput
	if err := generateJSON(ctx, gen, hooksPrompt(brief, spec, hatches), hooksSchema, &out); err != nil {
		return "", fmt.Errorf("hooks: %w", err)
	}
	if len(out.Hooks) == 0 {
		return "", nil
	}
	return assembleScript(out.Hooks), nil
}

type rulesOutput struct {
	Rules string `json:"rules"`
}

const rulesSchema = `{"type":"object","properties":{"rules":{"type":"string"}},"required":["rules"]}`

func rulesPrompt(brief Brief, spec *core.MechanicsSpec) string {
	var b strings.Builder
	b.WriteString("Write the player-facing rules guide (prompts/rules.md) for this tabletop RPG system.\n")
	if brief.Name != "" {
		fmt.Fprintf(&b, "System Name: %s\n", brief.Name)
	}
	fmt.Fprintf(&b, "Description: %s\n", brief.Description)
	if spec != nil {
		if spec.Checks.Notation != "" {
			fmt.Fprintf(&b, "Dice Notation: %s\n", spec.Checks.Notation)
		}
		if len(spec.Checks.Outcome) > 0 {
			fmt.Fprintf(&b, "Possible Outcomes: %s\n", strings.Join(spec.Checks.Outcome, ", "))
		}
	}
	b.WriteString("Describe how to play, how checks resolve, character stats and health mechanisms, and rules flow in markdown format.\n")
	return b.String()
}

// runRules prompts the generator for the player-facing rules guide markdown.
func runRules(ctx context.Context, gen Generator, brief Brief, spec *core.MechanicsSpec) (string, error) {
	var out rulesOutput
	if err := generateJSON(ctx, gen, rulesPrompt(brief, spec), rulesSchema, &out); err != nil {
		return "", fmt.Errorf("rules: %w", err)
	}
	return out.Rules, nil
}

