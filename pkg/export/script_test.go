package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestCompileReplayScript(t *testing.T) {
	isolateConfig(t)

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
	})

	_ = logger.AppendTurn(engine.Turn{
		Number:    2,
		Timestamp: time.Now(),
		Mode:      "Say",
		Input:     "Good evening, Evelyn.",
		Narration: "Evelyn: \"You made it back in one piece.\"",
	})

	compiler := NewScriptCompiler(tempDir)
	script, err := compiler.Compile(context.Background(), "shadow-campaign")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if script.GameID != "shadow-campaign" {
		t.Errorf("expected game ID shadow-campaign, got %s", script.GameID)
	}
	if script.GameName != "Shadow Realm" {
		t.Errorf("GameName = %q, want the manifest name", script.GameName)
	}

	// Neither turn recorded a location, so each opens its own uncarded scene.
	if len(script.Scenes) != 2 {
		t.Fatalf("expected 2 scenes, got %d: %+v", len(script.Scenes), script.Scenes)
	}

	beats := script.Beats()
	if len(beats) != 2 {
		t.Fatalf("expected 2 beats, got %d", len(beats))
	}
	if beats[0].TurnNumber != 1 || beats[0].Kind != scene.BeatNarration {
		t.Errorf("unexpected beat 0: %+v", beats[0])
	}

	// The second turn predates segments, so its prose is parsed instead: the
	// attributed line still arrives as a speech beat.
	speech := beats[1]
	if speech.Kind != scene.BeatSpeech || speech.Speaker != "Evelyn" {
		t.Errorf("expected one attributed speech beat for Evelyn, got %+v", speech)
	}
}

func TestCompileKeepsAttributedSegments(t *testing.T) {
	isolateConfig(t)

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

	beats := script.Beats()
	if len(beats) != 3 {
		t.Fatalf("expected 3 beats, got %d: %+v", len(beats), beats)
	}
	if beats[0].SpeakerID != "player" || beats[2].SpeakerID != "garrick" {
		t.Errorf("expected speaker IDs to survive compilation, got %#v", beats)
	}
	if beats[1].Kind != scene.BeatNarration {
		t.Errorf("expected the middle segment to be narration, got %+v", beats[1])
	}
}

func TestCompileResolvesTheLocationNameFromTheIndex(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "located")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte("id: located\nname: Located\n"), 0644); err != nil {
		t.Fatal(err)
	}

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"Warm.","location":"alden-tavern","segments":[{"kind":"narration","text":"Warm."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	// Compilation reads the location from the index, which is where a synced note
	// ends up.
	store, err := storage.OpenGameStore(core.NewPathResolver(root), "located")
	if err != nil {
		t.Fatalf("open game store: %v", err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Hash: "h1"}); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "located")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(script.Scenes) != 1 {
		t.Fatalf("expected 1 scene, got %d", len(script.Scenes))
	}
	if script.Scenes[0].LocationName != "Alden Tavern" {
		t.Errorf("LocationName = %q, want the entity name", script.Scenes[0].LocationName)
	}

	beats := script.Beats()
	if len(beats) != 2 || beats[0].Kind != scene.BeatSceneCard {
		t.Fatalf("expected a scene card then the narration, got %+v", beats)
	}
	if beats[1].Text != "Warm." {
		t.Errorf("unexpected narration beat: %+v", beats[1])
	}
}

// A bundle shows faces, so compilation has to resolve them: the note's own
// portrait file when the campaign has one, and the procedural bust the app serves
// when it does not.
func TestCompileResolvesPortraits(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "portraits")
	if err := os.MkdirAll(filepath.Join(gameDir, "assets", "portraits"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"),
		[]byte("id: portraits\nname: Portraits\nsystem: freeform\nworld: harbour\nplayer: sean\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "assets", "portraits", "sean.png"), []byte("png-bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Say","input":"hello","narration":"Sean speaks.","segments":[` +
		`{"kind":"speech","speaker":"Sean","speaker_id":"sean","player":true,"text":"Hello."},` +
		`{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"Welcome."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := storage.OpenGameStore(core.NewPathResolver(root), "portraits")
	if err != nil {
		t.Fatalf("open game store: %v", err)
	}
	for _, ent := range []*entity.Entity{
		{ID: "sean", Name: "Sean", Type: "character", Portrait: "assets/portraits/sean.png", Hash: "h1"},
		{ID: "garrick", Name: "Garrick", Type: "character", Hash: "h2"},
	} {
		if err := store.SaveEntity(ent); err != nil {
			t.Fatal(err)
		}
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "portraits")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if filepath.Base(script.PlayerPortrait) != "sean.png" {
		t.Errorf("PlayerPortrait = %q, want the protagonist's own file", script.PlayerPortrait)
	}

	var portraits = map[string]string{}
	for _, beat := range script.Beats() {
		if beat.Kind == scene.BeatSpeech && beat.PortraitPath != "" {
			portraits[beat.SpeakerID] = beat.PortraitPath
		}
	}
	if filepath.Base(portraits["sean"]) != "sean.png" {
		t.Errorf("sean's portrait = %q, want the note's file", portraits["sean"])
	}
	if _, err := os.Stat(portraits["garrick"]); err != nil {
		t.Errorf("garrick's portrait %q is not a file: %v", portraits["garrick"], err)
	}
	if !strings.HasSuffix(portraits["garrick"], ".svg") {
		t.Errorf("garrick's portrait = %q, want the procedural bust", portraits["garrick"])
	}
}
