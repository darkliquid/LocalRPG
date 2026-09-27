package gui

import (
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/pricing"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// RecordUsage implements harness.UsageSink: it prices a provider call and writes
// it to the campaign's ledger. It never fails a turn.
func (s *Service) RecordUsage(gameID string, turn int, role string, u harness.Usage) {
	store, err := s.store(gameID)
	if err != nil {
		return
	}
	price := pricing.Resolve(u.Provider, u.Model, s.configMgr.Get())
	rec := storage.UsageRecord{
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
