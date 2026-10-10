package sysgen

import (
	"context"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/refsystems"
	"github.com/darkliquid/localrpg/pkg/systemtest"
)

const deriveSchema = `{"type":"object","properties":{` +
	`"name":{"type":"string"},"description":{"type":"string"},` +
	`"mechanics":{"type":"object"},"script":{"type":"string"},"rules":{"type":"string"}}}`

// Derive generates a variant of a base system from an instruction, keeping the
// base's structure and changing what the instruction asks. An instruction is
// required; without one there is nothing to derive.
func Derive(ctx context.Context, gen Generator, base refsystems.ReferenceSystem, instruction string) (System, error) {
	if strings.TrimSpace(instruction) == "" {
		return System{}, fmt.Errorf("sysgen: an instruction is required to derive a system")
	}
	if gen == nil {
		return System{}, fmt.Errorf("sysgen: no generator configured")
	}
	if err := ctx.Err(); err != nil {
		return System{}, err
	}

	var out struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		Mechanics   *core.MechanicsSpec `json:"mechanics"`
		Script      string              `json:"script"`
		Rules       string              `json:"rules"`
	}
	if err := generateJSON(ctx, gen, derivePrompt(base, instruction), deriveSchema, &out); err != nil {
		return System{}, fmt.Errorf("derive: %w", err)
	}

	name := strings.TrimSpace(out.Name)
	if name == "" {
		name = base.Name
	}
	description := strings.TrimSpace(out.Description)
	if description == "" {
		description = base.Description
	}
	mechanics := out.Mechanics
	if mechanics == nil {
		mechanics = base.Mechanics
	}
	script := strings.TrimSpace(out.Script)
	if script == "" {
		script = base.Script
	}
	rules := strings.TrimSpace(out.Rules)
	if rules == "" {
		rules = base.RulesPrompt
	}

	id := entity.Slugify(name)
	if id == "" {
		id = base.ID
	}

	sys := System{
		ID:          id,
		Name:        name,
		Version:     "1.0.0",
		Description: description,
		Mechanics:   mechanics,
		Script:      script,
		RulesPrompt: rules,
	}
	gate := Gate(sys)
	sys.Verify = VerifyResult{OK: gate.OK, Failures: gateFailureDetails(gate), Script: gate.Script}
	return sys, nil
}

// ValidateDerivation runs the smoke gate and the base's own scenarios against a
// derived system, so a variant that breaks the base's expected behaviour is
// flagged. The result is shown to the user, not enforced: a derivation may
// deliberately change what the base asserted.
func ValidateDerivation(sys System, baseScenarios []systemtest.Scenario) GateResult {
	gate := Gate(sys)
	failures := append([]systemtest.Failure{}, gate.Failures...)
	runner := systemtest.System{ID: sys.ID, Script: sys.Script, Mechanics: sys.Mechanics}
	failures = append(failures, systemtest.RunAll(runner, baseScenarios)...)
	return GateResult{OK: len(failures) == 0, Failures: failures, Script: gate.Script}
}

func gateFailureDetails(gate GateResult) []string {
	details := make([]string, 0, len(gate.Failures))
	for _, failure := range gate.Failures {
		if failure.Detail != "" {
			details = append(details, failure.Detail)
		}
	}
	return details
}

func derivePrompt(base refsystems.ReferenceSystem, instruction string) string {
	var b strings.Builder
	b.WriteString("Derive a variant of an existing tabletop RPG system. Keep the base's structure and change only what the instruction asks. Return one JSON object and nothing else.\n")
	fmt.Fprintf(&b, "\nBase System: %s (%s)\n", base.Name, base.ID)
	if base.Description != "" {
		fmt.Fprintf(&b, "Base Description: %s\n", base.Description)
	}
	if summary := mechanicsSummary(base.Mechanics); summary != "" {
		b.WriteString("\nBase mechanics (keep this shape):\n" + summary)
	}
	if strings.TrimSpace(base.Script) != "" {
		b.WriteString("\nBase script hooks:\n" + truncateText(base.Script, 2000) + "\n")
	}
	if strings.TrimSpace(base.RulesPrompt) != "" {
		b.WriteString("\nBase rules prose:\n" + truncateText(base.RulesPrompt, 2000) + "\n")
	}
	fmt.Fprintf(&b, "\nInstruction: %s\n", instruction)
	b.WriteString(`Return {"name":...,"description":...,"mechanics":{...},"script":...,"rules":...}, omitting any field that does not change.`)
	return b.String()
}
