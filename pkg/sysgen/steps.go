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
		return systemShape{}, fmt.Errorf("sysgen shape: %w", err)
	}
	var shape systemShape
	if err := decodeJSON(raw, &shape); err != nil {
		return systemShape{}, fmt.Errorf("sysgen shape decode: %w", err)
	}
	return shape, nil
}

// runSchema prompts the generator for the full MechanicsSpec and validates it.
func runSchema(ctx context.Context, gen Generator, brief Brief, shape systemShape) (*core.MechanicsSpec, error) {
	raw, err := gen.GenerateJSON(ctx, schemaPrompt(brief, shape), schemaSchema)
	if err != nil {
		return nil, fmt.Errorf("sysgen schema: %w", err)
	}
	var spec core.MechanicsSpec
	if err := decodeJSON(raw, &spec); err != nil {
		return nil, fmt.Errorf("sysgen schema decode: %w", err)
	}

	// Validate check resolution profiles if any are declared
	if problems := spec.Checks.Validate(); len(problems) > 0 {
		return nil, fmt.Errorf("sysgen schema validation: %s", strings.Join(problems, "; "))
	}

	return &spec, nil
}
