package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
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
		Narration: "The tavern is warm and bustling. Lady Evelyn sits in the corner.",
	}

	if err := hl.AppendTurn(t1); err != nil {
		t.Fatalf("AppendTurn failed: %v", err)
	}

	t2 := Turn{
		Number:    2,
		Timestamp: time.Now(),
		Mode:      "Say",
		Input:     "Greetings, my lady.",
		Narration: "Evelyn looks up with guarded eyes.",
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

func TestLoadHistoryNormalisesLegacyOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	legacy := `{"number":1,"timestamp":"2026-09-20T10:00:00Z","mode":"Do","input":"look","output":"You look around.","entities":[{"id":"hero","mention":"player"}]}` + "\n"
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	logger := NewHistoryLogger(path)
	turns, err := logger.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if got := turns[0].Prose(); got != "You look around." {
		t.Errorf("Prose() = %q, want the legacy output text", got)
	}
	if turns[0].LegacyOutput != "" {
		t.Errorf("expected the legacy field to be cleared after normalisation")
	}
	if len(turns[0].Entities) != 1 || turns[0].Entities[0].Kind != entity.MentionPlayer {
		t.Errorf("expected entity involvement to survive loading, got %+v", turns[0].Entities)
	}

	if err := logger.RewindToTurn(1); err != nil {
		t.Fatalf("RewindToTurn failed: %v", err)
	}
	rewritten, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rewritten), `"output"`) {
		t.Errorf("expected rewrites to drop the legacy field, got %s", rewritten)
	}
	if !strings.Contains(string(rewritten), `"narration":"You look around."`) {
		t.Errorf("expected rewrites to emit narration, got %s", rewritten)
	}
}

func TestTurnToolCallsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	logger := NewHistoryLogger(filepath.Join(dir, "history.jsonl"))
	turn := Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "look",
		Narration: "You look.",
		ToolCalls: []ToolCallRecord{{Name: "search_entities", ResultChars: 42}},
	}
	if err := logger.AppendTurn(turn); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	turns, err := logger.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(turns) != 1 || len(turns[0].ToolCalls) != 1 {
		t.Fatalf("turns = %+v", turns)
	}
	if turns[0].ToolCalls[0].Name != "search_entities" || turns[0].ToolCalls[0].ResultChars != 42 {
		t.Errorf("provenance = %+v", turns[0].ToolCalls)
	}
}
