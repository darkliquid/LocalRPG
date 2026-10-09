package media

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/state"
)

func TestPortraitIsDeterministic(t *testing.T) {
	req := PortraitRequest{ID: "garrick", Name: "Garrick", Tags: []string{"human", "warrior"}}
	if !bytes.Equal(GenerateProceduralPortrait(req), GenerateProceduralPortrait(req)) {
		t.Fatal("the same request produced different portraits")
	}
}

func TestPortraitIsSVG(t *testing.T) {
	got := GenerateProceduralPortrait(PortraitRequest{ID: "x"})
	if !bytes.Contains(got, []byte("<svg")) || !bytes.Contains(got, []byte("</svg>")) {
		t.Fatalf("not svg: %q", got)
	}
}

func TestPortraitSpeciesAndArchetypeShow(t *testing.T) {
	orc := GenerateProceduralPortrait(PortraitRequest{ID: "a", Tags: []string{"orc", "warrior"}})
	elf := GenerateProceduralPortrait(PortraitRequest{ID: "a", Tags: []string{"elf", "mage"}})
	if bytes.Equal(orc, elf) {
		t.Fatal("species and archetype should change the portrait")
	}
}

func TestPortraitVariesBySpecies(t *testing.T) {
	seen := make(map[string]string)
	for species := range speciesTable {
		svg := GenerateProceduralPortrait(PortraitRequest{ID: "same", Tags: []string{species, "warrior"}})
		key := hashPortrait(svg)
		if other, ok := seen[key]; ok {
			t.Fatalf("%s and %s rendered identically", other, species)
		}
		seen[key] = species
	}
}

func TestPortraitVariesByArchetype(t *testing.T) {
	seen := make(map[string]string)
	for archetype := range archetypeTable {
		svg := GenerateProceduralPortrait(PortraitRequest{ID: "same", Tags: []string{"human", archetype}})
		key := hashPortrait(svg)
		if other, ok := seen[key]; ok {
			t.Fatalf("%s and %s rendered identically", other, archetype)
		}
		seen[key] = archetype
	}
}

func TestExpressionFromLowHealth(t *testing.T) {
	healthy := GenerateProceduralPortrait(PortraitRequest{ID: "a", State: map[string]any{"health": 10}})
	hurt := GenerateProceduralPortrait(PortraitRequest{ID: "a", State: map[string]any{"health": 1}})
	if bytes.Equal(healthy, hurt) {
		t.Fatal("a low health state should change the expression")
	}
}

func TestExpressionFromMaxHealth(t *testing.T) {
	full := GenerateProceduralPortrait(PortraitRequest{ID: "a", State: map[string]any{"health": 18, "health_max": 20}})
	low := GenerateProceduralPortrait(PortraitRequest{ID: "a", State: map[string]any{"health": 4, "health_max": 20}})
	if bytes.Equal(full, low) {
		t.Fatal("health at or below a third of its maximum should read as strained")
	}
}

func TestExpressionFromMood(t *testing.T) {
	calm := GenerateProceduralPortrait(PortraitRequest{ID: "a", State: map[string]any{"mood": "calm"}})
	angry := GenerateProceduralPortrait(PortraitRequest{ID: "a", State: map[string]any{"mood": "angry"}})
	if bytes.Equal(calm, angry) {
		t.Fatal("a mood should change the expression")
	}
}

func TestPortraitEmptyRequestIsStillABust(t *testing.T) {
	got := GenerateProceduralPortrait(PortraitRequest{})
	if !bytes.Contains(got, []byte("<svg")) || len(got) < 512 {
		t.Fatalf("an empty request should still render a bust, got %d bytes", len(got))
	}
}

func TestPortraitIgnoresTagOrder(t *testing.T) {
	a := GenerateProceduralPortrait(PortraitRequest{ID: "x", Tags: []string{"orc", "warrior"}})
	b := GenerateProceduralPortrait(PortraitRequest{ID: "x", Tags: []string{"warrior", "orc"}})
	if !bytes.Equal(a, b) {
		t.Fatal("tag order should not change the portrait")
	}
}

func TestPortraitRequestForReadsTheEntity(t *testing.T) {
	ent := &entity.Entity{
		ID: "garrick", Name: "Garrick", Gender: "male",
		Tags:  []string{"orc", "warrior"},
		State: state.NewState(map[string]any{"might": 4, "mood": "grim", "nested": map[string]any{"x": 1}}),
	}
	req := PortraitRequestFor(ent)
	if req.ID != "garrick" || req.Name != "Garrick" || req.Gender != "male" {
		t.Fatalf("request = %+v", req)
	}
	if len(req.Tags) != 2 {
		t.Fatalf("tags = %v", req.Tags)
	}
	if req.State["might"] != 4 || req.State["mood"] != "grim" {
		t.Fatalf("state = %+v", req.State)
	}
	if _, ok := req.State["nested"]; ok {
		t.Fatal("a non-scalar state value should not be carried")
	}
	if PortraitRequestFor(nil).ID != "" {
		t.Fatal("a nil entity should yield an empty request")
	}
}

func hashPortrait(svg []byte) string {
	sum := sha256.Sum256(svg)
	return hex.EncodeToString(sum[:])
}
