package storage

import (
	"path/filepath"
	"testing"
)

func TestUsageRoundTripAndSummary(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rows := []UsageRecord{
		{TurnNumber: 1, Role: "gm", Provider: "gemini", Model: "gemini-2.5-pro", InputTokens: 100, OutputTokens: 50, CostMicros: 40},
		{TurnNumber: 1, Role: "tts", Provider: "elevenlabs", Characters: 200, CostMicros: 6},
		{TurnNumber: 2, Role: "gm", Provider: "gemini", Model: "gemini-2.5-pro", InputTokens: 80, OutputTokens: 40, Estimated: true, CostMicros: 30},
	}
	for _, row := range rows {
		if err := store.SaveUsage(row); err != nil {
			t.Fatalf("SaveUsage: %v", err)
		}
	}

	turnOne, err := store.UsageByTurn(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(turnOne) != 2 {
		t.Fatalf("UsageByTurn(1) = %d rows, want 2", len(turnOne))
	}
	if turnOne[1].Role != "tts" || turnOne[1].Characters != 200 {
		t.Fatalf("second row = %+v", turnOne[1])
	}
	if !turnOne[0].Estimated && turnOne[0].CostMicros != 40 {
		t.Fatalf("first row lost its fields: %+v", turnOne[0])
	}

	summary, err := store.UsageSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Rows != 3 || summary.TotalCostMicros != 76 {
		t.Fatalf("summary = %+v, want 3 rows and 76 micros", summary)
	}
	if summary.ByProvider["gemini"] != 70 || summary.ByRole["tts"] != 6 {
		t.Fatalf("breakdown wrong: %+v", summary)
	}

	total, err := store.UsageTotal()
	if err != nil {
		t.Fatal(err)
	}
	if total != 76 {
		t.Fatalf("UsageTotal = %d, want 76", total)
	}
}

func TestUsageScopeGroupsAndDeletes(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rows := []UsageRecord{
		{GameID: "global", Role: "image", Provider: "imagen", CostMicros: 5},
		{GameID: "global", Role: "stt", Provider: "whisper", CostMicros: 7},
		{GameID: "pending:abc", Role: "image", Provider: "imagen", CostMicros: 3},
	}
	for _, row := range rows {
		if err := store.SaveUsage(row); err != nil {
			t.Fatalf("SaveUsage: %v", err)
		}
	}

	summary, err := store.UsageSummaryByGame()
	if err != nil {
		t.Fatal(err)
	}
	if summary.ByGame["global"] != 12 || summary.ByGame["pending:abc"] != 3 || summary.TotalCostMicros != 15 {
		t.Fatalf("by-game summary = %+v", summary)
	}

	pending, err := store.UsageByGame("pending:abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Role != "image" {
		t.Fatalf("pending rows = %+v", pending)
	}

	if err := store.DeleteUsageByGame("pending:abc"); err != nil {
		t.Fatal(err)
	}
	if remaining, _ := store.UsageByGame("pending:abc"); len(remaining) != 0 {
		t.Fatalf("pending rows survived the delete: %+v", remaining)
	}
}
