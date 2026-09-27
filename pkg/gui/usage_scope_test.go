package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestStudioUsageIsRecordedGlobally(t *testing.T) {
	_, svc := turnFixture(t)
	svc.RecordUsageGlobal("image", harness.Usage{Provider: "imagen", Requests: 1, Estimated: true})

	ledger, err := svc.usageLedger()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ledger.UsageByGame(UsageScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Role != "image" {
		t.Fatalf("global rows = %+v", rows)
	}
}

func TestDeferredUsageMovesOntoTheCreatedCampaign(t *testing.T) {
	gameID, svc := turnFixture(t)
	token := "new-campaign"
	svc.RecordUsageDeferred(token, "image", harness.Usage{Provider: "imagen", Requests: 1, Estimated: true})

	ledger, err := svc.usageLedger()
	if err != nil {
		t.Fatal(err)
	}
	if rows, _ := ledger.UsageByGame(usageScopePending + token); len(rows) != 1 {
		t.Fatalf("pending rows = %+v", rows)
	}

	if err := svc.CommitDeferredUsage(token, gameID); err != nil {
		t.Fatal(err)
	}
	if rows, _ := ledger.UsageByGame(usageScopePending + token); len(rows) != 0 {
		t.Fatalf("pending rows survived the commit: %+v", rows)
	}

	store, err := svc.store(gameID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.UsageByGame("")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Role != "image" {
		t.Fatalf("campaign rows = %+v", rows)
	}
}

func TestDiscardedDeferredUsageBecomesSharedSpend(t *testing.T) {
	_, svc := turnFixture(t)
	token := "abandoned"
	svc.RecordUsageDeferred(token, "stt", harness.Usage{Provider: "whisper", Requests: 1, Estimated: true})

	if err := svc.DiscardDeferredUsage(token); err != nil {
		t.Fatal(err)
	}
	ledger, err := svc.usageLedger()
	if err != nil {
		t.Fatal(err)
	}
	if rows, _ := ledger.UsageByGame(usageScopePending + token); len(rows) != 0 {
		t.Fatalf("pending rows survived the discard: %+v", rows)
	}
	rows, err := ledger.UsageByGame(UsageScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Role != "stt" {
		t.Fatalf("global rows = %+v", rows)
	}
}
