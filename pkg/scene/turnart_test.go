package scene

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestTurnArtFindsAnIllustration(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenes", "turn-3.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := TurnArt(dir, 3)
	if !ok || !strings.HasSuffix(got, "turn-3.png") {
		t.Fatalf("art = %q ok %v", got, ok)
	}
}

func TestTurnArtPrefersTheFirstKnownExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"turn-2.webp", "turn-2.png"} {
		if err := os.WriteFile(filepath.Join(dir, "scenes", name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := TurnArt(dir, 2)
	if !strings.HasSuffix(got, "turn-2.png") {
		t.Fatalf("art = %q, want the png", got)
	}
}

func TestTurnArtMissing(t *testing.T) {
	if _, ok := TurnArt(t.TempDir(), 3); ok {
		t.Fatal("a missing illustration should not resolve")
	}
	if _, ok := TurnArt("", 3); ok {
		t.Fatal("no assets directory should not resolve")
	}
	if _, ok := TurnArt(t.TempDir(), 0); ok {
		t.Fatal("an invalid turn should not resolve")
	}
}

func TestTurnArtIgnoresAnEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenes", "turn-4.png"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := TurnArt(dir, 4); ok {
		t.Fatal("an empty file is not an illustration")
	}
}

// TestCompilePrefersTurnArt guards the per-beat preference: a turn with its own
// illustration compiles to a beat that carries it, and a turn without one keeps
// the scene's location art.
func TestCompilePrefersTurnArt(t *testing.T) {
	assets := t.TempDir()
	if err := os.MkdirAll(filepath.Join(assets, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "scenes", "turn-2.png"), []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}

	source := &fakeSource{
		turns: []engine.Turn{
			turn(1, "hall", entity.TurnSegment{Kind: "narration", Text: "One."}),
			turn(2, "hall", entity.TurnSegment{Kind: "narration", Text: "Two."}),
		},
		locations: map[string]*entity.Entity{"hall": {ID: "hall", Name: "The Hall"}},
	}

	script, err := NewCompiler(source).Compile(t.Context(), "illustrated", Options{
		AssetsDir: assets,
		Art:       true,
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(script.Scenes) != 1 {
		t.Fatalf("scenes = %d, want one", len(script.Scenes))
	}

	var illustrated, plain string
	for _, beat := range script.Scenes[0].Beats {
		if beat.TurnNumber == 2 {
			illustrated = beat.ArtPath
			continue
		}
		if plain == "" {
			plain = beat.ArtPath
		}
	}
	if !strings.HasSuffix(illustrated, "turn-2.png") {
		t.Fatalf("turn 2 art = %q, want its illustration", illustrated)
	}
	if strings.HasSuffix(plain, "turn-2.png") {
		t.Fatalf("a turn with no illustration should keep the location art, got %q", plain)
	}
}
