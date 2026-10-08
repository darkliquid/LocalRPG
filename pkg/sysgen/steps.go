package sysgen

import (
	"context"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
)

// systemShape describes the broad mechanical architecture of a system
// before full schema expansion.
type systemShape struct {
	Resolution  string   `json:"resolution"`
	Stats       []string `json:"stats,omitempty"`
	Skills      []string `json:"skills,omitempty"`
	Health      string   `json:"health,omitempty"`
	Advancement bool     `json:"advancement"`
}

const shapeSchema = `{"type":"object","properties":{` +
	`"resolution":{"type":"string"},"stats":{"type":"array","items":{"type":"string"}},` +
	`"skills":{"type":"array","items":{"type":"string"}},"health":{"type":"string"},` +
	`"advancement":{"type":"boolean"}}}`

const schemaSchema = `{"type":"object","properties":{` +
	`"stats":{"type":"array"},"skills":{"type":"array"},"health":{"type":"object"},` +
	`"checks":{"type":"object"},"advancement":{"type":"object"}}}`

func shapePrompt(brief Brief) string {
	var b strings.Builder
	b.WriteString("Decide the mechanical shape for a tabletop RPG system.\n")
	if brief.Name != "" {
		fmt.Fprintf(&b, "System Name: %s\n", brief.Name)
	}
	fmt.Fprintf(&b, "Description: %s\n", brief.Description)
	b.WriteString("Specify the resolution style (e.g. d20, ladder, pool, blades), primary stats, skills, health mechanism, and whether advancement applies.\n")
	return b.String()
}

func schemaPrompt(brief Brief, shape systemShape) string {
	var b strings.Builder
	b.WriteString("Define the complete declarative mechanics specification (core.MechanicsSpec) for this tabletop RPG system.\n")
	if brief.Name != "" {
		fmt.Fprintf(&b, "System Name: %s\n", brief.Name)
	}
	fmt.Fprintf(&b, "Description: %s\n", brief.Description)
	fmt.Fprintf(&b, "Resolution style: %s\n", shape.Resolution)
	if len(shape.Stats) > 0 {
		fmt.Fprintf(&b, "Key stats: %s\n", strings.Join(shape.Stats, ", "))
	}
	if len(shape.Skills) > 0 {
		fmt.Fprintf(&b, "Key skills: %s\n", strings.Join(shape.Skills, ", "))
	}
	if shape.Health != "" {
		fmt.Fprintf(&b, "Health model: %s\n", shape.Health)
	}
	fmt.Fprintf(&b, "Advancement enabled: %t\n", shape.Advancement)
	b.WriteString("Include stats, skills, health (if any), checks (with notation, outcome vocabulary, and resolution profiles), and advancement if applicable.\n")
	return b.String()
}

// runShape prompts the generator for the system's high-level mechanical shape.
func runShape(ctx context.Context, gen Generator, brief Brief) (systemShape, error) {
	raw, err := gen.GenerateJSON(ctx, shapePrompt(brief), shapeSchema)
	if err != nil {
		return systemShape{}, fmt.Errorf("shape: %w", err)
	}
	var shape systemShape
	if err := decodeJSON(raw, &shape); err != nil {
		return systemShape{}, fmt.Errorf("shape decode: %w", err)
	}
	return shape, nil
}

// runSchema prompts the generator for the full MechanicsSpec and validates it.
func runSchema(ctx context.Context, gen Generator, brief Brief, shape systemShape) (*core.MechanicsSpec, error) {
	raw, err := gen.GenerateJSON(ctx, schemaPrompt(brief, shape), schemaSchema)
	if err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	var spec core.MechanicsSpec
	if err := decodeJSON(raw, &spec); err != nil {
		return nil, fmt.Errorf("schema decode: %w", err)
	}

	// Validate check resolution profiles if any are declared
	if problems := spec.Checks.Validate(); len(problems) > 0 {
		return nil, fmt.Errorf("schema validation: %s", strings.Join(problems, "; "))
	}

	return &spec, nil
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

func hooksPrompt(brief Brief, spec *core.MechanicsSpec) string {
	var b strings.Builder
	b.WriteString("Identify any custom mechanics.js script hooks needed for this tabletop RPG system.\n")
	b.WriteString("CRITICAL: The system is schema-first. If the declarative schema (stats, skills, health, checks, advancement) covers everything, return {\"hooks\": []}.\n")
	b.WriteString("Only emit hooks for behavior that the declarative schema CANNOT express (e.g. custom action handlers, turn end effects, resource spends, onHealthZero reactions).\n")
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

// runHooks prompts the generator for any custom JavaScript hooks and assembles them.
func runHooks(ctx context.Context, gen Generator, brief Brief, spec *core.MechanicsSpec) (string, error) {
	raw, err := gen.GenerateJSON(ctx, hooksPrompt(brief, spec), hooksSchema)
	if err != nil {
		return "", fmt.Errorf("hooks: %w", err)
	}
	var out hooksOutput
	if err := decodeJSON(raw, &out); err != nil {
		return "", fmt.Errorf("hooks decode: %w", err)
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
	raw, err := gen.GenerateJSON(ctx, rulesPrompt(brief, spec), rulesSchema)
	if err != nil {
		return "", fmt.Errorf("rules: %w", err)
	}
	var out rulesOutput
	if err := decodeJSON(raw, &out); err != nil {
		return "", fmt.Errorf("rules decode: %w", err)
	}
	return out.Rules, nil
}

