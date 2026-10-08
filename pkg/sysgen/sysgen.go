// Package sysgen generates tabletop RPG systems from natural language descriptions.
package sysgen

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/jsonrepair"
)

// Brief is the starting description for generating a tabletop RPG system.
type Brief struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description"`
}

// VerifyResult records the outcome of verifying a generated system.
type VerifyResult struct {
	OK       bool     `json:"ok"`
	Failures []string `json:"failures,omitempty"`
	Script   bool     `json:"script,omitempty"`
}

// System is a generated tabletop RPG system before it is saved.
type System struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Description string              `json:"description"`
	Mechanics   *core.MechanicsSpec `json:"mechanics,omitempty"`
	Script      string              `json:"script,omitempty"`
	RulesPrompt string              `json:"rules_prompt,omitempty"`
	Verify      VerifyResult        `json:"verify"`
}

// Generator defines the structured-output seam for system generation.
type Generator interface {
	GenerateJSON(ctx context.Context, prompt, schema string) ([]byte, error)
}

func decodeJSON(raw []byte, v any) error {
	payload := raw
	if res := jsonrepair.Repair(raw); res.OK {
		payload = res.Payload
	}
	return json.Unmarshal(payload, v)
}

// Generate creates a System from a Brief using the provided Generator.
func Generate(ctx context.Context, gen Generator, brief Brief) (System, error) {
	if gen == nil {
		return System{}, fmt.Errorf("sysgen: no generator configured")
	}
	if err := ctx.Err(); err != nil {
		return System{}, err
	}

	shape, err := runShape(ctx, gen, brief)
	if err != nil {
		return System{}, fmt.Errorf("sysgen: %w", err)
	}

	mech, err := runSchema(ctx, gen, brief, shape)
	if err != nil {
		return System{}, fmt.Errorf("sysgen: %w", err)
	}

	name := brief.Name
	if name == "" {
		name = "Custom System"
	}

	id := entity.Slugify(name)
	if id == "" {
		id = "custom-system"
	}

	return System{
		ID:          id,
		Name:        name,
		Version:     "1.0.0",
		Description: brief.Description,
		Mechanics:   mech,
		Verify:      VerifyResult{},
	}, nil
}
