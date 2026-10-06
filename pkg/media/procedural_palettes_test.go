package media

import (
	"math/rand"
	"testing"
)

func TestPaletteForIsDeterministicAndVaried(t *testing.T) {
	p1 := paletteFor("fantasy", "grim", "night", rand.New(rand.NewSource(1)))
	p2 := paletteFor("fantasy", "grim", "night", rand.New(rand.NewSource(1)))
	if p1 != p2 {
		t.Fatal("same inputs produced different palettes")
	}
	day := paletteFor("fantasy", "grim", "day", rand.New(rand.NewSource(1)))
	if p1 == day {
		t.Fatal("time of day did not change the palette")
	}
	if paletteFor("unknown", "", "", rand.New(rand.NewSource(1))) == (palette{}) {
		t.Fatal("an unknown genre should still yield a palette")
	}
}

func TestEveryBasePaletteIsValid(t *testing.T) {
	for genre, p := range basePalettes {
		if p == (palette{}) {
			t.Errorf("genre %q has an empty palette", genre)
		}
		for name, hex := range map[string]string{
			"sky top": p.SkyTop, "sky bottom": p.SkyBottom, "ground": p.Ground,
			"ridge": p.Ridge, "fog": p.Fog, "celestial": p.Celestial,
			"accent a": p.AccentA, "accent b": p.AccentB,
		} {
			if len(hex) != 7 || hex[0] != '#' {
				t.Errorf("genre %q %s = %q, want #rrggbb", genre, name, hex)
			}
		}
	}
}
