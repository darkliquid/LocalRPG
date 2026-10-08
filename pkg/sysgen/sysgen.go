// Package sysgen generates tabletop RPG systems from natural language descriptions.
package sysgen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/jsonrepair"
	"github.com/darkliquid/localrpg/pkg/systemtest"
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

	script, err := runHooks(ctx, gen, brief, mech)
	if err != nil {
		return System{}, fmt.Errorf("sysgen: %w", err)
	}

	rules, err := runRules(ctx, gen, brief, mech)
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

	s := System{
		ID:          id,
		Name:        name,
		Version:     "1.0.0",
		Description: brief.Description,
		Mechanics:   mech,
		Script:      script,
		RulesPrompt: rules,
		Verify:      VerifyResult{Script: strings.TrimSpace(script) != ""},
	}

	sys := systemtest.System{
		ID:        s.ID,
		Script:    s.Script,
		Mechanics: s.Mechanics,
	}

	scenario := systemtest.Scenario{Name: "smoke"}
	if mech != nil {
		if len(mech.Checks.Profiles) > 0 {
			for profName := range mech.Checks.Profiles {
				scenario.Steps = append(scenario.Steps, systemtest.Step{
					Action: "check",
					Input:  profName,
				})
				break
			}
		}
		if len(mech.Stats) > 0 {
			scenario.Steps = append(scenario.Steps, systemtest.Step{
				Action: "do",
				Input:  mech.Stats[0].ID,
			})
		}
	}

	failures := systemtest.Run(sys, scenario)
	if len(failures) == 0 {
		s.Verify.OK = true
	} else {
		s.Verify.OK = false
		for _, f := range failures {
			detail := f.Detail
			if f.Scenario != "" && f.Step > 0 {
				detail = fmt.Sprintf("[%s step %d] %s", f.Scenario, f.Step, f.Detail)
			} else if f.Scenario != "" {
				detail = fmt.Sprintf("[%s] %s", f.Scenario, f.Detail)
			}
			s.Verify.Failures = append(s.Verify.Failures, detail)
		}
	}

	return s, nil
}


