package gui

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
)

func TestCachedHistoryReflectsNewTurns(t *testing.T) {
	gameID, svc := turnFixture(t)

	before, err := svc.cachedHistory(gameID)
	if err != nil {
		t.Fatalf("cachedHistory: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("expected an empty history, got %d", len(before))
	}

	path := filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl")
	if err := engine.NewHistoryLogger(path).AppendTurn(engine.Turn{Number: 1, Narration: "x"}); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	after, err := svc.cachedHistory(gameID)
	if err != nil {
		t.Fatalf("cachedHistory after append: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("cache did not pick up the appended turn: %d", len(after))
	}
}
