package media

import "testing"

func TestSpeciesFromTags(t *testing.T) {
	if got := speciesFor([]string{"orc"}).ID; got != "orc" {
		t.Fatalf("an orc tag selected %q", got)
	}
	if got := speciesFor(nil).ID; got != "human" {
		t.Fatalf("no tag selected %q, want human", got)
	}
	if got := speciesFor([]string{"ELVES"}).ID; got != "elf" {
		t.Fatalf("matching must be case-insensitive, got %q", got)
	}
}

func TestSpeciesChangesTheSilhouette(t *testing.T) {
	orc := speciesFor([]string{"orc"})
	elf := speciesFor([]string{"elf"})
	if orc.EarShape == elf.EarShape && orc.Jaw == elf.Jaw {
		t.Fatal("species should differ in silhouette")
	}
}

func TestEverySpeciesHasADistinctSilhouette(t *testing.T) {
	seen := make(map[string]string, len(speciesTable))
	for id, sp := range speciesTable {
		shape := sp.EarShape + "/" + sp.Brow + "/" + sp.Jaw
		if other, ok := seen[shape]; ok {
			t.Fatalf("%s and %s share the silhouette %s", other, id, shape)
		}
		seen[shape] = id
		if len(sp.Skin) == 0 {
			t.Fatalf("%s declares no skin hue", id)
		}
	}
}
