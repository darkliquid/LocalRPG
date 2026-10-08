package worldgen

import (
	"context"
	"strings"
	"testing"
)

func TestOracleGeneratorProducesATemplate(t *testing.T) {
	g := NewOracleGenerator()
	d, err := Generate(context.Background(), g, Brief{Premise: "a drowned kingdom", Counts: Counts{Characters: 2}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.World.Name == "" || len(d.Entities) == 0 {
		t.Fatalf("draft = %+v", d)
	}
	if d.World.Name != "Drowned Kingdom" {
		t.Fatalf("name = %q", d.World.Name)
	}
	if !strings.Contains(d.Lore, "Drowned Kingdom") {
		t.Fatalf("lore = %q", d.Lore)
	}
}

func TestOracleGeneratorHonoursCounts(t *testing.T) {
	d, err := Generate(context.Background(), NewOracleGenerator(),
		Brief{Premise: "a drowned kingdom", Counts: Counts{Locations: 2, Factions: 1, Characters: 3}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, e := range d.Entities {
		counts[e.Type]++
	}
	if counts["location"] != 2 || counts["faction"] != 1 || counts["character"] != 3 {
		t.Fatalf("counts = %+v (entities %+v)", counts, d.Entities)
	}
}

func TestOracleGeneratorLinksEntities(t *testing.T) {
	d, err := Generate(context.Background(), NewOracleGenerator(),
		Brief{Premise: "a drowned kingdom", Counts: Counts{Locations: 2, Factions: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	linked := 0
	for _, e := range d.Entities {
		if len(e.Links) > 0 {
			linked++
		}
	}
	if linked == 0 {
		t.Fatalf("the oracle should cross-link its entities: %+v", d.Entities)
	}
}

func TestOracleIsRecognised(t *testing.T) {
	if !Oracle(NewOracleGenerator()) {
		t.Fatal("NewOracleGenerator should report as the oracle")
	}
	if Oracle(&stubGen{}) {
		t.Fatal("a model generator is not the oracle")
	}
}
