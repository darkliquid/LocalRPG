package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestTurnUsageIsRecordedWithCost(t *testing.T) {
	gameID, svc := turnFixture(t)
	svc.configMgr.Get().Providers.Prices = []config.PriceConfig{
		{Provider: "gemini", PerMillionInput: 1_000_000, PerMillionOutput: 2_000_000},
	}
	svc.RecordUsage(gameID, 1, "gm", harness.Usage{Provider: "gemini", Model: "m", InputTokens: 1000, OutputTokens: 500})

	store, err := svc.store(gameID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.UsageByTurn(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Role != "gm" || rows[0].CostMicros != 2000 {
		t.Fatalf("usage rows = %+v, want 1 gm row costing 2000 micros", rows)
	}
}
