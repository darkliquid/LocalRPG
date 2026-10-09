package scene

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// genreFixture is the shared table the frontend and Go both read, so the two
// palettes cannot drift.
type genreFixture struct {
	Genres []GenrePalette `json:"genres"`
	Copy   map[string]struct {
		Empty   string `json:"empty"`
		Loading string `json:"loading"`
	} `json:"copy"`
}

func loadGenreFixture(t *testing.T) genreFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "genre-palettes.json"))
	if err != nil {
		t.Fatalf("read genre fixture: %v", err)
	}
	var fixture genreFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("parse genre fixture: %v", err)
	}
	return fixture
}

// TestGenrePaletteMatchesTheFixture is the Go half of the parity test: the built-in
// table must equal the shared fixture the frontend also reads.
func TestGenrePaletteMatchesTheFixture(t *testing.T) {
	fixture := loadGenreFixture(t)
	if len(fixture.Genres) != len(GenrePalettes) {
		t.Fatalf("fixture has %d genres, the code has %d", len(fixture.Genres), len(GenrePalettes))
	}
	for i, want := range fixture.Genres {
		if got := GenrePalettes[i]; got != want {
			t.Fatalf("genre %d = %+v, want %+v", i, got, want)
		}
	}
}

func TestGenrePaletteResolvesAndDefaults(t *testing.T) {
	if got := GenrePaletteFor("cyberpunk"); got.ID != "cyberpunk" {
		t.Fatalf("cyberpunk resolved to %q", got.ID)
	}
	if got := GenrePaletteFor("SCI-FI"); got.ID != "scifi" {
		t.Fatalf("matching must be case-insensitive and alias-aware, got %q", got.ID)
	}
	if got := GenrePaletteFor("nonsense"); got.ID != "neutral" {
		t.Fatalf("an unknown genre should be neutral, got %q", got.ID)
	}
	if got := GenrePaletteFor(""); got.ID != "neutral" {
		t.Fatalf("an absent genre should be neutral, got %q", got.ID)
	}
}

// TestExportGradientUsesTheGenre guards that the no-art background is tinted by the
// campaign's genre rather than always the same warm gradient.
func TestExportGradientUsesTheGenre(t *testing.T) {
	renderer, err := NewRenderer(32, 18)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	cyberpunk := renderer.gradientFor("cyberpunk")
	fantasy := renderer.gradientFor("fantasy")
	if cyberpunk.RGBAAt(16, 9) == fantasy.RGBAAt(16, 9) {
		t.Fatal("two genres should not share the same background")
	}
	if neutral := renderer.gradientFor("nonsense"); neutral.RGBAAt(16, 9) != renderer.gradientFor("").RGBAAt(16, 9) {
		t.Fatal("an unknown genre should fall back to neutral")
	}
}

func TestGenreGradientIsCached(t *testing.T) {
	renderer, err := NewRenderer(16, 9)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	if renderer.gradientFor("fantasy") != renderer.gradientFor("fantasy") {
		t.Fatal("a genre's gradient should be built once")
	}
}

func TestHexColourFallsBack(t *testing.T) {
	fallback := color.RGBA{1, 2, 3, 255}
	if got := hexColour("not-a-colour", fallback); got != fallback {
		t.Fatalf("a malformed colour should fall back, got %+v", got)
	}
	if got := hexColour("#0e3b4a", fallback); got != (color.RGBA{14, 59, 74, 255}) {
		t.Fatalf("hex = %+v", got)
	}
}
