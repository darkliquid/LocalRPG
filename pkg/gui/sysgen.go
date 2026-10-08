package gui

import (
	"context"
	"errors"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/sysgen"
	"github.com/darkliquid/localrpg/pkg/worldgen"
)

// systemGeneratorResolution is the generator a system generation runs against.
type systemGeneratorResolution struct {
	Generator sysgen.Generator
	Oracle    bool
	Reason    string
}

// resolveSystemGenerator chooses the generator a system generation runs against.
// A missing or unusable provider falls back to the deterministic oracle generator.
func (s *Service) resolveSystemGenerator() systemGeneratorResolution {
	cfg := s.configMgr.Get()
	role := resolveGeneratorRole(cfg)
	if role == "" {
		return systemGeneratorResolution{
			Generator: sysgen.NewOracleGenerator(),
			Oracle:    true,
			Reason:    "no agent role is set up to generate. Assign a model provider to gm or generator in Settings → AI Agents",
		}
	}

	router, err := textRouterFactory(cfg, s.logger)
	if err != nil {
		return systemGeneratorResolution{
			Generator: sysgen.NewOracleGenerator(),
			Oracle:    true,
			Reason:    fmt.Sprintf("the provider router could not be built: %v", err),
		}
	}
	if _, err := router.GetProviderForRole(role); err != nil {
		detail := err.Error()
		if buildErrs := router.BuildErrors(); len(buildErrs) > 0 {
			first := buildErrs[0]
			detail = fmt.Sprintf("the provider for the %q role could not be built: %v", first.Role, first.Err)
		}
		return systemGeneratorResolution{
			Generator: sysgen.NewOracleGenerator(),
			Oracle:    true,
			Reason:    detail,
		}
	}
	return systemGeneratorResolution{Generator: &routerGenerator{router: router, roles: []string{role}}}
}

// GenerateSystem runs the system generation pipeline, emitting step events and a final draft.
// A dry run emits the estimate and makes no model calls.
func (s *Service) GenerateSystem(ctx context.Context, req SystemGenerateRequestDTO, emit func(TurnEvent) error) (*SystemDraftDTO, error) {
	brief := sysgen.Brief{
		Name:        req.Name,
		Description: req.Description,
	}
	limits := s.limitsFor(req.Limits)
	role := resolveGeneratorRole(s.configMgr.Get())

	estimate := worldgen.Estimate{Calls: 4}
	if prices := s.worldPriceTable(role); prices != nil {
		if cost, ok := prices(estimate.Calls); ok {
			estimate.CostMicros = cost
			estimate.Priced = true
		}
	}

	if req.DryRun {
		if err := emit(TurnEvent{Type: WorldEventEstimate, Estimate: estimateDTO(estimate)}); err != nil {
			return nil, err
		}
		return nil, nil
	}

	genCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var emitErr error
	emitEvent := func(event TurnEvent) {
		if emitErr != nil {
			return
		}
		if err := emit(event); err != nil {
			emitErr = err
			cancel()
		}
	}

	resolved := s.resolveSystemGenerator()
	budget := &worldgen.BudgetGenerator{Inner: resolved.Generator, Max: limits.MaxCalls}

	draft, genErr := sysgen.Generate(genCtx, budget, brief, func(step sysgen.Step) {
		emitEvent(TurnEvent{Type: WorldEventStep, Step: &WorldGenStepDTO{
			Name:   step.Name,
			Status: step.Status,
			Detail: step.Detail,
		}})
	})

	if genErr != nil {
		if errors.Is(genErr, worldgen.ErrCallBudgetExceeded) {
			return nil, fmt.Errorf("generation stopped: %w", genErr)
		}
		return nil, genErr
	}
	if emitErr != nil {
		return nil, emitErr
	}

	calls := budget.Calls
	if counter, ok := resolved.Generator.(*routerGenerator); ok {
		recorded, usage := counter.totals()
		if recorded > 0 {
			calls = recorded
			s.RecordUsageGlobal(generatorRole, usage)
		}
	}

	if err := sysgen.SaveDraft(s.systemDraftsDir(), draft); err != nil {
		return nil, err
	}

	dto := systemDraftDTO(draft, resolved.Oracle, &estimate, calls)
	emitEvent(TurnEvent{Type: WorldEventDraft, SystemDraft: &dto})
	if emitErr != nil {
		return nil, emitErr
	}
	return &dto, nil
}

// GetSystemDraft loads a persisted system draft.
func (s *Service) GetSystemDraft(ctx context.Context, id string) (*SystemDraftDTO, error) {
	draft, err := sysgen.LoadDraft(s.systemDraftsDir(), id)
	if err != nil {
		return nil, err
	}
	dto := systemDraftDTO(draft, false, nil, 0)
	return &dto, nil
}

// ListSystemDraftIDs reports the system drafts awaiting review.
func (s *Service) ListSystemDraftIDs(ctx context.Context) ([]string, error) {
	return sysgen.ListDraftIDs(s.systemDraftsDir())
}

// DiscardSystemDraft deletes a system draft.
func (s *Service) DiscardSystemDraft(ctx context.Context, id string) error {
	return sysgen.DeleteDraft(s.systemDraftsDir(), id)
}

// CommitSystemDraft writes the system draft to systems/<id>/ and removes the draft on success.
func (s *Service) CommitSystemDraft(ctx context.Context, req SystemDraftCommitRequestDTO) (*SystemDetailDTO, error) {
	if req.DraftID == "" {
		return nil, fmt.Errorf("draft_id is required")
	}
	draft, err := sysgen.LoadDraft(s.systemDraftsDir(), req.DraftID)
	if err != nil {
		return nil, err
	}

	id := firstNonEmptyString(req.ID, draft.ID)
	name := firstNonEmptyString(req.Name, draft.Name)
	version := firstNonEmptyString(req.Version, draft.Version, "1.0.0")
	description := firstNonEmptyString(req.Description, draft.Description)

	mechanics := draft.Mechanics
	if req.Mechanics != nil {
		mechanics = req.Mechanics
	}

	script := draft.Script
	if req.Script != nil {
		script = *req.Script
	}

	rulesPrompt := draft.RulesPrompt
	if req.RulesPrompt != nil {
		rulesPrompt = *req.RulesPrompt
	}

	saveReq := CreateSystemRequestDTO{
		ID:          id,
		Name:        name,
		Version:     version,
		Description: description,
		Script:      script,
		RulesPrompt: rulesPrompt,
		Mechanics:   mechanics,
	}

	detail, err := s.SaveSystem(ctx, saveReq)
	if err != nil {
		return nil, err
	}

	if err := sysgen.DeleteDraft(s.systemDraftsDir(), req.DraftID); err != nil {
		return nil, err
	}

	return detail, nil
}

func systemDraftDTO(d sysgen.System, oracle bool, estimate *worldgen.Estimate, calls int) SystemDraftDTO {
	var estDTO *WorldEstimateDTO
	if estimate != nil {
		estDTO = estimateDTO(*estimate)
	}
	return SystemDraftDTO{
		ID:          d.ID,
		Name:        d.Name,
		Version:     d.Version,
		Description: d.Description,
		Mechanics:   d.Mechanics,
		Script:      d.Script,
		RulesPrompt: d.RulesPrompt,
		Verify:      d.Verify,
		Estimate:    estDTO,
		Calls:       calls,
		Oracle:      oracle,
	}
}
