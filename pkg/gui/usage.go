package gui

import (
	"context"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/pricing"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// UsageScopeGlobal names the shared spend that belongs to no campaign: studio
// generations, provider tests, and a new campaign's work before it exists.
const UsageScopeGlobal = "global"

// usageScopePending marks rows held for a campaign that has not been created
// yet; CommitDeferredUsage moves them onto the campaign once it exists.
const usageScopePending = "pending:"

// RecordUsage implements harness.UsageSink: it prices a provider call and writes
// it to the campaign's ledger. An empty game id means the call was not tied to a
// campaign, so it lands in the shared ledger. It never fails a turn.
func (s *Service) RecordUsage(gameID string, turn int, role string, u harness.Usage) {
	if gameID == "" {
		s.recordUsageScope(UsageScopeGlobal, turn, role, u)
		return
	}
	store, err := s.store(gameID)
	if err != nil {
		return
	}
	s.saveUsage(store, "", turn, role, u)
}

// RecordUsageGlobal records spend that belongs to no campaign: a studio asset, a
// provider test, or a preview.
func (s *Service) RecordUsageGlobal(role string, u harness.Usage) {
	s.recordUsageScope(UsageScopeGlobal, 0, role, u)
}

// RecordUsageDeferred records spend incurred for a campaign that does not exist
// yet, against a token BeginDeferredUsage handed out. CommitDeferredUsage moves
// it onto the campaign; DiscardDeferredUsage leaves it as shared spend.
func (s *Service) RecordUsageDeferred(token, role string, u harness.Usage) {
	if token == "" {
		s.RecordUsageGlobal(role, u)
		return
	}
	s.recordUsageScope(usageScopePending+token, 0, role, u)
}

// CommitDeferredUsage moves every row held for a token onto a campaign, so the
// campaign's own ledger shows the work that created it.
func (s *Service) CommitDeferredUsage(token, gameID string) error {
	if token == "" || gameID == "" {
		return nil
	}
	ledger, err := s.usageLedger()
	if err != nil {
		return err
	}
	scope := usageScopePending + token
	rows, err := ledger.UsageByGame(scope)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	store, err := s.store(gameID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		row.GameID = ""
		if err := store.SaveUsage(row); err != nil {
			return err
		}
	}
	return ledger.DeleteUsageByGame(scope)
}

// DiscardDeferredUsage folds a token's rows back into shared spend, which is
// what an abandoned creation flow should cost.
func (s *Service) DiscardDeferredUsage(token string) error {
	if token == "" {
		return nil
	}
	ledger, err := s.usageLedger()
	if err != nil {
		return err
	}
	_, err = ledger.RetagUsage(usageScopePending+token, UsageScopeGlobal)
	return err
}

func (s *Service) recordUsageScope(scope string, turn int, role string, u harness.Usage) {
	ledger, err := s.usageLedger()
	if err != nil {
		return
	}
	s.saveUsage(ledger, scope, turn, role, u)
}

// saveUsage prices a provider call and writes it to a ledger.
func (s *Service) saveUsage(store *storage.Store, gameID string, turn int, role string, u harness.Usage) {
	price := pricing.Resolve(u.Provider, u.Model, s.configMgr.Get())
	rec := storage.UsageRecord{
		GameID:       gameID,
		TurnNumber:   turn,
		Role:         role,
		Provider:     u.Provider,
		Model:        u.Model,
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		Characters:   u.Characters,
		Requests:     u.Requests,
		Estimated:    u.Estimated,
		CostMicros:   int64(pricing.CostMicros(u, price)),
	}
	if err := store.SaveUsage(rec); err != nil {
		trace.OrNil(s.logger).Event("usage.record_error", map[string]interface{}{"error": err.Error()})
	}
}

// usageLedger opens the shared usage database once, under the cache directory.
// It holds spend that belongs to no campaign, and the pending rows a creation
// flow produces before its campaign exists.
func (s *Service) usageLedger() (*storage.Store, error) {
	s.globalUsageMu.Lock()
	defer s.globalUsageMu.Unlock()
	if s.globalUsage != nil {
		return s.globalUsage, nil
	}
	path := filepath.Join(s.resolver.CacheDir(), "usage.db")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	store, err := storage.NewStore(path)
	if err != nil {
		return nil, err
	}
	s.globalUsage = store
	return store, nil
}

// mediaUsage converts a media provider's own usage shape into the harness one,
// which is where pricing and the ledger live. key is the provider's canonical
// identity, resolved from its configuration.
func mediaUsage(u media.Usage, key provider.Key, model string) harness.Usage {
	return harness.Usage{
		Provider:     string(key),
		Model:        model,
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		Characters:   u.Characters,
		Requests:     u.Requests,
		Estimated:    u.Estimated,
	}
}

// usageCurrency is the configured display currency for cost figures.
func (s *Service) usageCurrency() string {
	return s.configMgr.Get().Providers.Currency
}

func usageRowDTO(rec storage.UsageRecord) UsageRowDTO {
	return UsageRowDTO{
		TurnNumber:   rec.TurnNumber,
		Role:         rec.Role,
		Provider:     rec.Provider,
		Model:        rec.Model,
		InputTokens:  rec.InputTokens,
		OutputTokens: rec.OutputTokens,
		Characters:   rec.Characters,
		Requests:     rec.Requests,
		Estimated:    rec.Estimated,
		CostMicros:   rec.CostMicros,
	}
}

func mergeUsage(dto *UsageDTO, summary storage.UsageSummary) {
	dto.TotalCost += summary.TotalCostMicros
	for provider, cost := range summary.ByProvider {
		dto.ByProvider[provider] += cost
	}
	for role, cost := range summary.ByRole {
		dto.ByRole[role] += cost
	}
}

// GameUsage reports one campaign's spend, with the underlying rows.
func (s *Service) GameUsage(ctx context.Context, gameID string) (*UsageDTO, error) {
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	summary, err := store.UsageSummary()
	if err != nil {
		return nil, err
	}
	rows, err := store.UsageAll()
	if err != nil {
		return nil, err
	}
	dto := &UsageDTO{
		ByProvider: summary.ByProvider,
		ByRole:     summary.ByRole,
		TotalCost:  summary.TotalCostMicros,
		Currency:   s.usageCurrency(),
	}
	for _, row := range rows {
		dto.Rows = append(dto.Rows, usageRowDTO(row))
	}
	return dto, nil
}

// GlobalUsage totals every campaign plus the shared ledger, with a per-campaign
// drilldown so a spend view can break the total down.
func (s *Service) GlobalUsage(ctx context.Context) (*UsageDTO, error) {
	dto := &UsageDTO{
		ByProvider: map[string]int64{},
		ByRole:     map[string]int64{},
		Currency:   s.usageCurrency(),
	}
	games, err := s.ListGames(ctx)
	if err != nil {
		return nil, err
	}
	for _, game := range games {
		store, err := s.store(game.ID)
		if err != nil {
			continue
		}
		summary, err := store.UsageSummary()
		if err != nil {
			continue
		}
		mergeUsage(dto, summary)
		dto.Campaigns = append(dto.Campaigns, CampaignUsageDTO{
			GameID:    game.ID,
			Name:      game.Name,
			TotalCost: summary.TotalCostMicros,
		})
	}
	if ledger, err := s.usageLedger(); err == nil {
		if summary, err := ledger.UsageSummaryByGame(); err == nil {
			mergeUsage(dto, summary)
			dto.Campaigns = append(dto.Campaigns, CampaignUsageDTO{
				GameID:    UsageScopeGlobal,
				Name:      "Shared",
				TotalCost: summary.ByGame[UsageScopeGlobal],
			})
		}
	}
	return dto, nil
}
