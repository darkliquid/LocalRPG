package engine

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/rules"
)

func TestTurnHistoryAppendAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	historyPath := filepath.Join(tempDir, "history.jsonl")

	hl := NewHistoryLogger(historyPath)

	t1 := Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I enter the tavern and look for Evelyn",
		Roll:      &rules.RollResult{Notation: "1d20", Total: 15},
		Output:    "The tavern is warm and bustling. Lady Evelyn sits in the corner.",
	}

	if err := hl.AppendTurn(t1); err != nil {
		t.Fatalf("AppendTurn failed: %v", err)
	}

	t2 := Turn{
		Number:    2,
		Timestamp: time.Now(),
		Mode:      "Say",
		Input:     "Greetings, my lady.",
		Output:    "Evelyn looks up with guarded eyes.",
	}

	if err := hl.AppendTurn(t2); err != nil {
		t.Fatalf("AppendTurn failed: %v", err)
	}

	turns, err := hl.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if turns[0].Input != t1.Input || turns[1].Input != t2.Input {
		t.Errorf("history mismatch: %+v", turns)
	}

	// Test Rewind to Turn 1
	if err := hl.RewindToTurn(1); err != nil {
		t.Fatalf("RewindToTurn failed: %v", err)
	}

	rewound, err := hl.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory after rewind failed: %v", err)
	}
	if len(rewound) != 1 || rewound[0].Number != 1 {
		t.Errorf("expected 1 turn after rewind, got %d", len(rewound))
	}
}
