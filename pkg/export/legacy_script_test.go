package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/scene"
)

func TestCompileReadsLegacyTurns(t *testing.T) {
	isolateConfig(t)

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

	// A turn written before segments existed still produces a beat, because the
	// compiler parses its prose rather than dropping it.
	beats := script.Beats()
	if len(beats) != 1 {
		t.Fatalf("expected 1 beat, got %d: %+v", len(beats), beats)
	}
	if beats[0].Text != "The hall is quiet." {
		t.Errorf("expected the legacy output to compile as prose, got %q", beats[0].Text)
	}
	if beats[0].Kind != scene.BeatNarration {
		t.Errorf("expected a narration beat, got %q", beats[0].Kind)
	}
	if beats[0].TurnNumber != 1 {
		t.Errorf("expected the beat to remember its turn, got %d", beats[0].TurnNumber)
	}
}

func TestCompileShapesALegacyConversationAsSpeech(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "legacy-talk")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte("id: legacy-talk\nname: Legacy Talk\n"), 0644); err != nil {
		t.Fatal(err)
	}

	legacy := `{"number":1,"timestamp":"2026-09-20T10:00:00Z","mode":"Say","input":"Hello","output":"She does not look up.\nGarrick: \"Keep walking.\""}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "legacy-talk")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	beats := script.Beats()
	if len(beats) != 2 {
		t.Fatalf("expected the prose and the line as two beats, got %+v", beats)
	}
	if beats[0].Kind != scene.BeatNarration {
		t.Errorf("expected narration first, got %+v", beats[0])
	}
	if beats[1].Kind != scene.BeatSpeech || beats[1].Speaker != "Garrick" {
		t.Errorf("expected an attributed speech beat, got %+v", beats[1])
	}
}
