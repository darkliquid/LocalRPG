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

type rawSystemOutput struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Description string              `json:"description"`
	Mechanics   *core.MechanicsSpec `json:"mechanics,omitempty"`
	Script      string              `json:"script,omitempty"`
	RulesPrompt string              `json:"rules_prompt,omitempty"`
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

	prompt := fmt.Sprintf("Generate a tabletop RPG system based on this description: %s", brief.Description)
	raw, err := gen.GenerateJSON(ctx, prompt, "system")
	if err != nil {
		return System{}, fmt.Errorf("sysgen: generate: %w", err)
	}

	var rawSys rawSystemOutput
	if err := decodeJSON(raw, &rawSys); err != nil {
		return System{}, fmt.Errorf("sysgen: decode: %w", err)
	}

	if rawSys.Mechanics == nil {
		var mech core.MechanicsSpec
		if err := decodeJSON(raw, &mech); err == nil {
			if len(mech.Stats) > 0 || len(mech.Skills) > 0 || mech.Health != nil ||
				mech.Checks.Notation != "" || len(mech.Checks.Outcome) > 0 || len(mech.Checks.Profiles) > 0 {
				rawSys.Mechanics = &mech
			}
		}
	}

	name := rawSys.Name
	if name == "" {
		name = brief.Name
	}
	if name == "" {
		name = "Custom System"
	}

	id := rawSys.ID
	if id == "" {
		id = entity.Slugify(name)
	}
	if id == "" {
		id = "custom-system"
	}

	version := rawSys.Version
	if version == "" {
		version = "1.0.0"
	}

	description := rawSys.Description
	if description == "" {
		description = brief.Description
	}

	return System{
		ID:          id,
		Name:        name,
		Version:     version,
		Description: description,
		Mechanics:   rawSys.Mechanics,
		Script:      rawSys.Script,
		RulesPrompt: rawSys.RulesPrompt,
		Verify:      VerifyResult{},
	}, nil
}
