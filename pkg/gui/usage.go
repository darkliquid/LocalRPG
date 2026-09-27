package gui

import (
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/pricing"
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
// which is where pricing and the ledger live.
func mediaUsage(u media.Usage, provider, model string) harness.Usage {
	return harness.Usage{
		Provider:     provider,
		Model:        model,
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		Characters:   u.Characters,
		Requests:     u.Requests,
		Estimated:    u.Estimated,
	}
}
