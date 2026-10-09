package media

import (
	"math/rand"
	"testing"
)

func TestArchetypeFromTags(t *testing.T) {
	if got := archetypeFor([]string{"warrior"}, rand.New(rand.NewSource(1))).ID; got != "warrior" {
		t.Fatalf("a warrior tag selected %q", got)
	}
	if got := archetypeFor([]string{"MERCHANT"}, rand.New(rand.NewSource(1))).ID; got != "noble" {
		t.Fatalf("matching must be case-insensitive, got %q", got)
	}
}

func TestArchetypeAccessoryDiffers(t *testing.T) {
	w := archetypeFor([]string{"warrior"}, rand.New(rand.NewSource(1)))
	m := archetypeFor([]string{"mage"}, rand.New(rand.NewSource(1)))
	if w.Accessory == m.Accessory {
		t.Fatalf("archetypes should differ in accessory, both %q", w.Accessory)
	}
}

func TestArchetypeSeededDefaultIsStable(t *testing.T) {
	a := archetypeFor(nil, rand.New(rand.NewSource(7)))
	b := archetypeFor(nil, rand.New(rand.NewSource(7)))
	if a.ID == "" || a.ID != b.ID {
		t.Fatalf("a seeded default should be stable, got %q and %q", a.ID, b.ID)
	}
}

func TestEveryArchetypeHasAnAccessoryAndGarment(t *testing.T) {
	seen := make(map[string]string, len(archetypeTable))
	for id, arch := range archetypeTable {
		if arch.Accessory == "" || len(arch.Garment) == 0 {
			t.Fatalf("%s declares no accessory or garment", id)
		}
		if other, ok := seen[arch.Accessory]; ok {
			t.Fatalf("%s and %s share the accessory %s", other, id, arch.Accessory)
		}
		seen[arch.Accessory] = id
	}
}
