package gui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/ingest"
	"github.com/darkliquid/localrpg/pkg/pricing"
	"github.com/darkliquid/localrpg/pkg/worldgen"
)

// The event types a world generation streams.
const (
	WorldEventStep     = "step"
	WorldEventEstimate = "estimate"
	WorldEventDraft    = "draft"
)

// ErrorCodeCallLimit and ErrorCodeSourceLimit mark a failure the user can fix by
// raising a limit. They are separate so the UI can offer the field that caused it
// rather than guessing from the message.
const (
	ErrorCodeCallLimit   = "generation_call_limit"
	ErrorCodeSourceLimit = "generation_source_limit"
)

// ErrSourceTooLarge reports a source that would need more calls than the chunk
// budget allows. It is refused before the first call, so a large folder costs
// nothing and the message names the setting that would let it through.
var ErrSourceTooLarge = errors.New("the source is larger than the chunk limit")

// generationLimits are the limits one generation runs under.
type generationLimits struct {
	MaxCalls  int
	MaxChunks int
}

// limitsFor resolves the limits a request runs under. A per-request override wins
// over the configured default, so a user who hits a limit can raise it for the
// run in front of them without leaving it raised for every later run.
func (s *Service) limitsFor(override *GenerationLimitsDTO) generationLimits {
	cfg := s.configMgr.Get()
	limits := generationLimits{MaxCalls: cfg.GenerationMaxCalls(), MaxChunks: cfg.GenerationMaxChunks()}
	if override != nil {
		if override.MaxCalls > 0 {
			limits.MaxCalls = override.MaxCalls
		}
		if override.MaxChunks > 0 {
			limits.MaxChunks = override.MaxChunks
		}
	}
	return limits
}

// generatorRole is the role a world generation runs as when one is configured.
const generatorRole = "generator"

// worldDraftsDir is where drafts live: a dot-directory under worlds/, which the
// syncer skips, so a draft is never mistaken for a world.
func (s *Service) worldDraftsDir() string {
	return filepath.Join(s.resolver.WorldsDir(), worldgen.DraftsDirName)
}

// routerGenerator adapts a role-routed provider to worldgen.Generator, counting
// the calls and tokens a generation spends so the ledger can report the actual
// cost.
type routerGenerator struct {
	router *harness.Router
	roles  []string

	mu    sync.Mutex
	calls int
	usage harness.Usage
}

// GenerateJSON asks each role in order for a structured reply, returning the
// first non-empty one. A reply is handed back raw: the pipeline repairs and
// parses it, so a fenced or padded response still works.
func (g *routerGenerator) GenerateJSON(ctx context.Context, prompt, schema string) ([]byte, error) {
	if g.router == nil {
		return nil, fmt.Errorf("worldgen: no provider router")
	}
	var lastErr error
	for _, role := range g.roles {
		resp, err := g.router.GenerateForRole(ctx, role, harness.GenerateRequest{
			System:    schemaHint(schema),
			Prompt:    prompt,
			MaxTokens: 8000,
		})
		if err != nil {
			lastErr = err
			continue
		}
		if resp == nil || strings.TrimSpace(resp.Text) == "" {
			lastErr = fmt.Errorf("the model returned no text")
			continue
		}
		g.record(resp.Usage)
		return []byte(resp.Text), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no provider available for %s", strings.Join(g.roles, ", "))
	}
	return nil, lastErr
}

// record accumulates one answered call's usage. A provider that reports none
// still counts as one request, so the ledger shows the call happened.
func (g *routerGenerator) record(u *harness.Usage) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if u == nil {
		g.usage.Requests++
		return
	}
	g.usage.InputTokens += u.InputTokens
	g.usage.OutputTokens += u.OutputTokens
	g.usage.Requests += max(u.Requests, 1)
	if u.Provider != "" {
		g.usage.Provider = u.Provider
	}
	if u.Model != "" {
		g.usage.Model = u.Model
	}
}

// totals reports the calls made and the usage they incurred.
func (g *routerGenerator) totals() (int, harness.Usage) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls, g.usage
}

// schemaHint turns the declared JSON schema into a one-line system instruction.
// Providers with native structured outputs ignore the prompt's schema; the rest
// are told to answer in JSON, and jsonrepair recovers a fenced reply.
func schemaHint(schema string) string {
	return "You answer with one JSON object and nothing else, matching this schema exactly: " + schema
}

// worldGenerator resolves the generator a world generation runs against. It
// returns the deterministic oracle when no role can generate, so the feature
// works offline instead of failing.
func (s *Service) worldGenerator() (worldgen.Generator, bool) {
	role := resolveGeneratorRole(s.configMgr.Get())
	if role == "" {
		return worldgen.NewOracleGenerator(), true
	}
	router, err := textRouterFactory(s.configMgr.Get(), s.logger)
	if err != nil {
		return worldgen.NewOracleGenerator(), true
	}
	if _, err := router.GetProviderForRole(role); err != nil {
		return worldgen.NewOracleGenerator(), true
	}
	return &routerGenerator{router: router, roles: []string{role}}, false
}

// resolveGeneratorRole names the role a world generation runs as: an explicit
// "generator" role, else gm. It returns "" when neither can generate, so the
// pipeline answers from templates rather than calling a placeholder.
func resolveGeneratorRole(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if role := realGeneratorRole(cfg, generatorRole, 0); role != "" {
		return role
	}
	return realGeneratorRole(cfg, config.RoleGM, 0)
}

// realGeneratorRole resolves one role, following an inheritance, and reports ""
// when the role is unset, disabled, or a placeholder that cannot generate.
func realGeneratorRole(cfg *config.Config, role string, depth int) string {
	if depth > 4 {
		return ""
	}
	roleCfg, ok := cfg.Agents.Roles[role]
	if !ok {
		return ""
	}
	if roleCfg.Type == "inherit" {
		if roleCfg.InheritFrom == "" || roleCfg.InheritFrom == role {
			return ""
		}
		return realGeneratorRole(cfg, roleCfg.InheritFrom, depth+1)
	}
	if isPlaceholderRole(roleCfg) {
		return ""
	}
	return role
}

// isPlaceholderRole reports whether a role's configuration is a shipped default
// that cannot generate: the echo command, the deterministic oracle, a disabled
// role, or the builtin echo.
func isPlaceholderRole(role config.AgentRoleConfig) bool {
	switch role.Type {
	case "disabled":
		return true
	case "cli":
		return strings.TrimSpace(role.Command) == "echo"
	case "builtin":
		return role.BuiltinName == "" || role.BuiltinName == "echo" || role.BuiltinName == "narrative-oracle"
	default:
		return false
	}
}

// worldPriceTable prices a planned generation when the generator's provider has
// a rate, and reports nil (unpriced) when it does not. An unpriced estimate is
// honest: the cost of a provider with no rate is unknown, not zero.
func (s *Service) worldPriceTable(role string) worldgen.PriceTable {
	cfg := s.configMgr.Get()
	if cfg == nil || role == "" {
		return nil
	}
	roleCfg := cfg.Agents.Roles[role]
	key, hasKey := harness.KeyFor(harness.ProviderConfig{
		Type:        roleCfg.Type,
		BuiltinName: roleCfg.BuiltinName,
		Command:     roleCfg.Command,
		Endpoint:    roleCfg.Endpoint,
		Instance:    roleCfg.Instance,
	})
	if !hasKey {
		return nil
	}
	price := pricing.Resolve(string(key), roleCfg.Model, cfg)
	if price == (pricing.Price{}) {
		return nil
	}
	return func(calls int) (int64, bool) {
		cost := int64(pricing.CostMicros(harness.Usage{Requests: calls}, price))
		if cost <= 0 {
			return 0, false
		}
		return cost, true
	}
}

// GenerateWorld runs the generation pipeline, reporting one step event per
// stage and a final draft event. A dry run reports the estimate and makes no
// call. Nothing is written into worlds/<id>/: the draft lands in worlds/.drafts
// for review.
func (s *Service) GenerateWorld(ctx context.Context, req WorldGenerateRequestDTO, emit func(TurnEvent) error) (*WorldDraftDTO, error) {
	brief := req.brief()
	limits := s.limitsFor(req.Limits)
	role := resolveGeneratorRole(s.configMgr.Get())
	chunks := 0
	if req.Source != nil && strings.EqualFold(req.Source.Kind, "url") {
		chunks = len(req.Source.URLs)
	}
	estimate := worldgen.EstimatePlan(generationKind(req), brief, chunks, s.worldPriceTable(role))

	if req.DryRun {
		if err := emit(TurnEvent{Type: WorldEventEstimate, Estimate: estimateDTO(estimate)}); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// A client that disconnects mid-generation cancels the work, and nothing is
	// persisted for a cancelled generation.
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

	gen, oracle := s.worldGenerator()

	var budget *worldgen.BudgetGenerator
	var draft worldgen.Draft
	var genErr error

	if req.Source != nil {
		sourceChunks, err := s.extractSource(ctx, *req.Source, limits.MaxChunks)
		if err != nil {
			return nil, err
		}
		// An ingestion is as big as the source is, so its budget follows the
		// chunk cap rather than the bounded pipeline's call cap.
		budget = &worldgen.BudgetGenerator{Inner: gen, Max: worldgen.ChunkCalls(len(sourceChunks))}
		genErr = s.buildFromChunks(genCtx, budget, brief, sourceChunks, emitEvent, &draft)
	} else {
		budget = &worldgen.BudgetGenerator{Inner: gen, Max: limits.MaxCalls}
		draft, genErr = worldgen.Generate(genCtx, budget, brief, func(step worldgen.Step) {
			emitEvent(TurnEvent{Type: WorldEventStep, Step: stepDTO(step)})
		})
	}

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
	if counter, ok := gen.(*routerGenerator); ok {
		recorded, usage := counter.totals()
		if recorded > 0 {
			calls = recorded
			s.RecordUsageGlobal(generatorRole, usage)
		}
	}

	draft.Estimate = &estimate
	draft.Calls = calls
	if err := worldgen.SaveDraft(s.worldDraftsDir(), draft); err != nil {
		return nil, err
	}

	dto := worldDraftDTO(draft, oracle)
	emitEvent(TurnEvent{Type: WorldEventDraft, Draft: &dto})
	if emitErr != nil {
		return nil, emitErr
	}
	return &dto, nil
}

// buildFromChunks assembles a draft from already-read chunks, reporting the same
// step events a from-scratch generation does.
func (s *Service) buildFromChunks(ctx context.Context, gen worldgen.Generator, brief worldgen.Brief, chunks []ingest.Chunk, emit func(TurnEvent), draft *worldgen.Draft) error {
	emit(TurnEvent{Type: WorldEventStep, Step: stepDTO(worldgen.Step{
		Name: "extract", Status: worldgen.StatusDone,
		Detail: fmt.Sprintf("%d chunk(s)", len(chunks)),
	})})

	built, err := ingest.Build(ctx, gen, chunks, brief)
	if err != nil {
		return err
	}
	*draft = built
	emit(TurnEvent{Type: WorldEventStep, Step: stepDTO(worldgen.Step{Name: worldgen.StepLink, Status: worldgen.StatusDone})})
	return nil
}

// extractSource reads a source and refuses one over the chunk budget. Refusing
// up front means a folder too large to read costs nothing and the message says
// how much it holds, so the user can raise the limit and carry on.
func (s *Service) extractSource(ctx context.Context, src WorldSourceDTO, maxChunks int) ([]ingest.Chunk, error) {
	chunks, err := ingest.Extract(ctx, ingest.Source{
		Kind: src.Kind,
		Path: src.Path,
		URLs: src.URLs,
	})
	if err != nil {
		return nil, err
	}
	if len(chunks) > maxChunks {
		return nil, fmt.Errorf("%w: it holds %d chunks and the limit is %d",
			ErrSourceTooLarge, len(chunks), maxChunks)
	}
	return chunks, nil
}

// generationKind names the pipeline a request uses, so the estimate matches.
func generationKind(req WorldGenerateRequestDTO) string {
	if req.Source != nil {
		return "ingest"
	}
	return "world"
}

// brief converts the request into the pipeline's own type.
func (r WorldGenerateRequestDTO) brief() worldgen.Brief {
	return worldgen.Brief{
		Name:    r.Name,
		Genre:   r.Genre,
		Premise: r.Premise,
		Themes:  r.Themes,
		Counts: worldgen.Counts{
			Locations:  r.Counts.Locations,
			Factions:   r.Counts.Factions,
			Characters: r.Counts.Characters,
		},
	}
}

func stepDTO(step worldgen.Step) *WorldGenStepDTO {
	return &WorldGenStepDTO{Name: step.Name, Status: step.Status, Detail: step.Detail}
}

func estimateDTO(e worldgen.Estimate) *WorldEstimateDTO {
	return &WorldEstimateDTO{Calls: e.Calls, Chunks: e.Chunks, CostMicros: e.CostMicros, Priced: e.Priced}
}

// worldDraftDTO renders a draft for the client.
func worldDraftDTO(d worldgen.Draft, oracle bool) WorldDraftDTO {
	sections := make([]WorldDraftSectionDTO, 0, len(d.Sections))
	for _, section := range d.Sections {
		sections = append(sections, WorldDraftSectionDTO{Title: section.Title, Body: section.Body})
	}
	entities := make([]WorldDraftEntityDTO, 0, len(d.Entities))
	for _, e := range d.Entities {
		entities = append(entities, draftEntityDTO(e))
	}
	var estimate *WorldEstimateDTO
	if d.Estimate != nil {
		estimate = estimateDTO(*d.Estimate)
	}
	return WorldDraftDTO{
		ID:          d.ID,
		Name:        d.World.Name,
		Description: d.World.Description,
		Genre:       d.World.Genre,
		ArtStyle:    d.World.ArtStyle,
		Tags:        d.World.Tags,
		Lore:        d.Lore,
		Sections:    sections,
		Entities:    entities,
		Estimate:    estimate,
		Calls:       d.Calls,
		Oracle:      oracle,
	}
}

func draftEntityDTO(e worldgen.DraftEntity) WorldDraftEntityDTO {
	return WorldDraftEntityDTO{
		ID:      e.ID,
		Name:    e.Name,
		Type:    e.Type,
		Tags:    e.Tags,
		Folder:  e.Folder,
		Body:    e.Body,
		Source:  e.Source,
		Links:   e.Links,
		Dropped: e.Dropped,
	}
}

// draftEntity converts a client-supplied entity back into the pipeline's type.
func draftEntity(e WorldDraftEntityDTO) worldgen.DraftEntity {
	return worldgen.DraftEntity{
		ID:     e.ID,
		Name:   e.Name,
		Type:   e.Type,
		Tags:   e.Tags,
		Folder: e.Folder,
		Body:   e.Body,
		Source: e.Source,
	}
}

// draftSections converts client-supplied sections back into the pipeline's type.
func draftSections(sections []WorldDraftSectionDTO) []worldgen.DraftSection {
	out := make([]worldgen.DraftSection, 0, len(sections))
	for _, section := range sections {
		out = append(out, worldgen.DraftSection{Title: section.Title, Body: section.Body})
	}
	return out
}
