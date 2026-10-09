package gui

import (
	"context"
	"errors"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/sysgen"
	"github.com/darkliquid/localrpg/pkg/worldgen"
)

// systemContext loads a system in the shape the generator reads.
func (s *Service) systemContext(ctx context.Context, id string) (sysgen.System, error) {
	detail, err := s.GetSystem(ctx, id)
	if err != nil {
		return sysgen.System{}, err
	}
	return sysgen.System{
		ID:          detail.ID,
		Name:        detail.Name,
		Version:     detail.Version,
		Description: detail.Description,
		Mechanics:   detail.Mechanics,
		Script:      detail.Script,
		RulesPrompt: detail.RulesPrompt,
	}, nil
}

// EnhanceSystem proposes additions to an existing system. Nothing is written;
// the caller applies the accepted set. Each proposal is validated with the smoke
// gate before it is offered.
func (s *Service) EnhanceSystem(ctx context.Context, id string, req SystemEnhanceRequestDTO) (*SystemEnhanceResponseDTO, error) {
	sys, err := s.systemContext(ctx, id)
	if err != nil {
		return nil, err
	}

	resolved := s.resolveSystemGenerator()
	budget := &worldgen.BudgetGenerator{Inner: resolved.Generator, Max: s.configMgr.Get().GenerationMaxCalls()}

	proposals, err := sysgen.Propose(ctx, budget, sys, req.Instruction, req.Kinds)
	if err != nil {
		if errors.Is(err, worldgen.ErrCallBudgetExceeded) {
			return nil, fmt.Errorf("generation stopped: %w", err)
		}
		return nil, err
	}

	out := &SystemEnhanceResponseDTO{
		Proposals: make([]SystemProposalDTO, 0, len(proposals)),
		Oracle:    resolved.Oracle,
	}
	for _, proposal := range proposals {
		dto := systemProposalDTO(proposal)
		gate := sysgen.ValidateEnhancement(sys, []sysgen.Proposal{proposal})
		dto.Valid = gate.OK
		if !gate.OK {
			dto.Problems = gateProblems(gate)
		}
		out.Proposals = append(out.Proposals, dto)
	}
	return out, nil
}

// ApplySystemEnhancements writes the accepted proposals by appending them to the
// system's mechanics. Applying an empty set changes nothing, and an enhancement
// that would break the system is refused.
func (s *Service) ApplySystemEnhancements(ctx context.Context, id string, req SystemEnhanceApplyRequestDTO) (*SystemEnhanceApplyResultDTO, error) {
	sys, err := s.systemContext(ctx, id)
	if err != nil {
		return nil, err
	}
	result := &SystemEnhanceApplyResultDTO{}
	if len(req.Proposals) == 0 {
		return result, nil
	}

	accepted := make([]sysgen.Proposal, 0, len(req.Proposals))
	for _, proposal := range req.Proposals {
		accepted = append(accepted, systemProposal(proposal))
	}

	if gate := sysgen.ValidateEnhancement(sys, accepted); !gate.OK {
		return nil, fmt.Errorf("the enhancement would break the system: %s", gate.FailureText())
	}

	enhanced, err := sysgen.ApplyAdditions(sys.Mechanics, accepted)
	if err != nil {
		return nil, err
	}

	detail, err := s.SaveSystem(ctx, CreateSystemRequestDTO{
		ID:          sys.ID,
		Name:        sys.Name,
		Version:     sys.Version,
		Description: sys.Description,
		Script:      sys.Script,
		RulesPrompt: sys.RulesPrompt,
		Mechanics:   enhanced,
	})
	if err != nil {
		return nil, err
	}
	result.Written = append(result.Written, "mechanics")
	result.Detail = detail
	return result, nil
}

// ExplainSystem returns a plain-language description of a system's mechanics.
func (s *Service) ExplainSystem(ctx context.Context, id string) (*SystemExplainResponseDTO, error) {
	sys, err := s.systemContext(ctx, id)
	if err != nil {
		return nil, err
	}

	resolved := s.resolveSystemGenerator()
	budget := &worldgen.BudgetGenerator{Inner: resolved.Generator, Max: s.configMgr.Get().GenerationMaxCalls()}

	text, err := sysgen.Explain(ctx, budget, sys)
	if err != nil {
		if errors.Is(err, worldgen.ErrCallBudgetExceeded) {
			return nil, fmt.Errorf("generation stopped: %w", err)
		}
		return nil, err
	}
	return &SystemExplainResponseDTO{Explanation: text, Oracle: resolved.Oracle}, nil
}

func systemProposalDTO(p sysgen.Proposal) SystemProposalDTO {
	return SystemProposalDTO{
		Kind:        p.Kind,
		Title:       p.Title,
		Reason:      p.Reason,
		Stat:        p.Stat,
		Skill:       p.Skill,
		Profile:     p.Profile,
		Advancement: p.Advancement,
	}
}

func systemProposal(dto SystemProposalDTO) sysgen.Proposal {
	return sysgen.Proposal{
		Kind:        dto.Kind,
		Title:       dto.Title,
		Reason:      dto.Reason,
		Stat:        dto.Stat,
		Skill:       dto.Skill,
		Profile:     dto.Profile,
		Advancement: dto.Advancement,
	}
}

func gateProblems(gate sysgen.GateResult) []string {
	problems := make([]string, 0, len(gate.Failures))
	for _, failure := range gate.Failures {
		if failure.Detail != "" {
			problems = append(problems, failure.Detail)
		}
	}
	return problems
}
