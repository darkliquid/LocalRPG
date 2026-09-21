// pkg/export/script_test.go
package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestCompileReplayScript(t *testing.T) {
	tempDir := t.TempDir()
	gameDir := filepath.Join(tempDir, "games", "shadow-campaign")
	_ = os.MkdirAll(gameDir, 0755)

	manifestContent := `id: shadow-campaign
name: Shadow Realm
system: core-d20
world: dark-fantasy
player: elena
`
	_ = os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(manifestContent), 0644)

	historyFile := filepath.Join(gameDir, "history.jsonl")
	logger := engine.NewHistoryLogger(historyFile)

	_ = logger.AppendTurn(engine.Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I step into the tavern.",
		Narration: "The tavern is warm and loud. Evelyn looks up from her book.",
		AudioRefs: []string{"audio/turn-1.wav"},
	})

	_ = logger.AppendTurn(engine.Turn{
		Number:    2,
		Timestamp: time.Now(),
		Mode:      "Say",
		Input:     "Good evening, Evelyn.",
		Narration: "Evelyn: \"You made it back in one piece.\"",
		AudioRefs: []string{"audio/turn-2.wav"},
	})

	compiler := NewScriptCompiler(tempDir)
	script, err := compiler.Compile(context.Background(), "shadow-campaign")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if script.GameID != "shadow-campaign" {
		t.Errorf("expected game ID shadow-campaign, got %s", script.GameID)
	}
	if len(script.Beats) != 2 {
		t.Fatalf("expected 2 beats, got %d", len(script.Beats))
	}

	if script.Beats[0].TurnNumber != 1 || script.Beats[0].AudioPath != "audio/turn-1.wav" {
		t.Errorf("unexpected beat 0: %+v", script.Beats[0])
	}
	speech := script.Beats[1].Segments
	if len(speech) != 1 || speech[0].Kind != entity.SegmentSpeech || speech[0].Speaker != "Evelyn" {
		t.Errorf("expected one attributed speech segment for Evelyn, got %+v", speech)
	}
}
