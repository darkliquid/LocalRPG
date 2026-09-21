package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCompileReadsLegacyTurns(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "legacy-campaign")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := "id: legacy-campaign\nname: Legacy Campaign\n"
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	legacy := `{"number":1,"timestamp":"2026-09-20T10:00:00Z","mode":"Do","input":"look","output":"The hall is quiet."}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "legacy-campaign")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if len(script.Beats) != 1 {
		t.Fatalf("expected 1 beat, got %d", len(script.Beats))
	}
	if script.Beats[0].Prose != "The hall is quiet." {
		t.Errorf("expected the legacy output to compile as prose, got %q", script.Beats[0].Prose)
	}
	if script.Beats[0].PlayerInput != "look" {
		t.Errorf("expected the player input to compile, got %q", script.Beats[0].PlayerInput)
	}
}

func TestCompileKeepsAttributedSegments(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "segmented")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte("id: segmented\nname: Segmented\n"), 0644); err != nil {
		t.Fatal(err)
	}

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Say","input":"Where is the ledger?","narration":"He does not look up. Garrick: \"Keep walking.\"","segments":[{"kind":"speech","speaker":"Sean","speaker_id":"player","text":"Where is the ledger?"},{"kind":"narration","text":"He does not look up."},{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"Keep walking."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "segmented")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if len(script.Beats) != 1 {
		t.Fatalf("expected 1 beat, got %d", len(script.Beats))
	}

	segments := script.Beats[0].Segments
	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %#v", segments)
	}
	if segments[0].SpeakerID != "player" || segments[2].SpeakerID != "garrick" {
		t.Errorf("expected speaker IDs to survive compilation, got %#v", segments)
	}
}
