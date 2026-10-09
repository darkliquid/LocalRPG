package gui

import (
	"context"
	"errors"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/refsystems"
	"github.com/darkliquid/localrpg/pkg/sysgen"
	"github.com/darkliquid/localrpg/pkg/systemtest"
	"github.com/darkliquid/localrpg/pkg/worldgen"
)

// ErrReferenceSystemNotFound reports an unknown base system id.
var ErrReferenceSystemNotFound = errors.New("reference system not found")

// DeriveSystem generates a variant of a reference base system from an
// instruction and saves it as a draft. Nothing is written to systems/<id>/
// until the draft is accepted. The base's own scenarios run too, so a variant
// that breaks the base's behaviour is flagged before it is offered.
func (s *Service) DeriveSystem(ctx context.Context, req SystemDeriveRequestDTO) (*SystemDraftDTO, error) {
	base, ok := refsystems.Get(req.BaseID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrReferenceSystemNotFound, req.BaseID)
	}

	resolved := s.resolveSystemGenerator()
	budget := &worldgen.BudgetGenerator{Inner: resolved.Generator, Max: s.configMgr.Get().GenerationMaxCalls()}

	sys, err := sysgen.Derive(ctx, budget, base, req.Instruction)
	if err != nil {
		if errors.Is(err, worldgen.ErrCallBudgetExceeded) {
			return nil, fmt.Errorf("generation stopped: %w", err)
		}
		return nil, err
	}

	gate := sysgen.ValidateDerivation(sys, referenceScenarios(base.ID))
	sys.Verify = sysgen.VerifyResult{OK: gate.OK, Script: gate.Script}
	for _, failure := range gate.Failures {
		detail := failure.Detail
		switch {
		case failure.Scenario != "" && failure.Step > 0:
			detail = fmt.Sprintf("[%s step %d] %s", failure.Scenario, failure.Step, failure.Detail)
		case failure.Scenario != "":
			detail = fmt.Sprintf("[%s] %s", failure.Scenario, failure.Detail)
		}
		sys.Verify.Failures = append(sys.Verify.Failures, detail)
	}

	if err := sysgen.SaveDraft(s.systemDraftsDir(), sys); err != nil {
		return nil, err
	}

	estimate := worldgen.Estimate{Calls: 1}
	calls := budget.Calls
	if counter, ok := resolved.Generator.(*routerGenerator); ok {
		recorded, usage := counter.totals()
		if recorded > 0 {
			calls = recorded
			s.RecordUsageGlobal(generatorRole, usage)
		}
	}
	dto := systemDraftDTO(sys, resolved.Oracle, &estimate, calls)
	return &dto, nil
}

// referenceScenarios loads a reference system's embedded scenarios, so a
// derivation can be checked against the behaviour the base already asserts.
func referenceScenarios(id string) []systemtest.Scenario {
	files := refsystems.ScenarioFiles(id)
	scenarios := make([]systemtest.Scenario, 0, len(files))
	for _, file := range files {
		scenario, err := systemtest.LoadScenario(file.Data)
		if err != nil {
			continue
		}
		scenarios = append(scenarios, scenario)
	}
	return scenarios
}
