package media

import (
	"math/rand"
	"strings"
	"testing"
)

func TestStructureForSelectsByTagThenGenre(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if s := structureFor([]string{"forest"}, "fantasy", rng); s == nil {
		t.Fatal("no structure for a forest tag")
	}
	frag := structureFor([]string{"city"}, "fantasy", rng)(paletteFor("fantasy", "", "day", rng), rng, 800, 600)
	if !strings.Contains(frag, "<") {
		t.Fatalf("a structure should emit SVG fragments, got %q", frag)
	}
}

func TestEveryStructureRenders(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	p := paletteFor("fantasy", "", "day", rng)
	for _, name := range structureOrder {
		frag := structures[name](p, rng, 800, 600)
		if !strings.Contains(frag, "<") {
			t.Errorf("structure %q emitted no SVG", name)
		}
	}
}
