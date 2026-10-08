package worldgen

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestPlacesStepParsesLocationsAndFactions(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"name":"Ashen Reach","genre":"dark fantasy","premise":"a dying frontier"}`,
		`{"locations":[{"name":"Saltmarch","tags":["coast"]}],"factions":[{"name":"The Tidewatch"}]}`,
	}}
	draft, err := Generate(context.Background(), g, Brief{Premise: "x", Counts: Counts{Locations: 1, Factions: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if draft.World.Name != "Ashen Reach" || len(draft.Entities) < 2 {
		t.Fatalf("draft = %+v", draft)
	}
	if draft.World.ID != "ashen-reach" {
		t.Fatalf("world id = %q", draft.World.ID)
	}
}

func TestCharactersStepSeedsFromPlaces(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"name":"Ashen Reach","premise":"a dying frontier"}`,
		`{"locations":[{"name":"Saltmarch"}]}`,
		`{"characters":[{"name":"Maren Vale","role":"harbourmaster","home_location":"Saltmarch"}]}`,
		`{"entities":[{"id":"saltmarch","body":"A port watched by [[Maren Vale]]."}]}`,
	}}
	draft, err := Generate(context.Background(), g, Brief{Premise: "x", Counts: Counts{Locations: 1, Characters: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Entities) != 2 {
		t.Fatalf("entities = %+v", draft.Entities)
	}
	if !strings.Contains(g.lastPrompt, "Saltmarch") {
		t.Fatal("the character prompt should be seeded with the places")
	}
	if !containsAll(draft.Entities[0].Body, "[[Maren Vale]]") {
		t.Fatalf("body = %q", draft.Entities[0].Body)
	}
	if len(draft.Entities[0].Links) != 1 || draft.Entities[0].Links[0] != "Maren Vale" {
		t.Fatalf("links = %+v", draft.Entities[0].Links)
	}
}

func TestLinkDraftResolvesAndDropsUnknownLinks(t *testing.T) {
	d := Draft{Entities: []DraftEntity{
		{ID: "saltmarch", Body: "Near [[The Tidewatch]] and [[Nowhere]]."},
		{ID: "the-tidewatch", Body: "A faction."},
	}}
	got := linkDraft(d)
	if !strings.Contains(got.Entities[0].Body, "[[The Tidewatch]]") {
		t.Fatal("a known link should survive")
	}
	if strings.Contains(got.Entities[0].Body, "[[Nowhere]]") {
		t.Fatal("an unknown link should be dropped")
	}
	if !strings.Contains(got.Entities[0].Body, "Nowhere") {
		t.Fatal("the prose around a dropped link should stay")
	}
	if len(got.Entities[0].Dropped) != 1 || got.Entities[0].Dropped[0] != "Nowhere" {
		t.Fatalf("dropped = %+v", got.Entities[0].Dropped)
	}
}

func TestLinkDraftKeepsALabel(t *testing.T) {
	d := Draft{Entities: []DraftEntity{
		{ID: "saltmarch", Body: "Near [[the-tidewatch|the Tidewatch]]."},
		{ID: "the-tidewatch", Body: "A faction."},
	}}
	got := linkDraft(d)
	if !strings.Contains(got.Entities[0].Body, "[[the-tidewatch|the Tidewatch]]") {
		t.Fatalf("a resolvable labelled link should survive: %q", got.Entities[0].Body)
	}
}

func TestDraftEntitiesParseAsEntityNotes(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"name":"Ashen Reach","premise":"a dying frontier"}`,
		`{"locations":[{"name":"Saltmarch","description":"A port."}]}`,
	}}
	draft := mustDraft(t, g, Brief{Premise: "x", Counts: Counts{Locations: 1}})
	if len(draft.Entities) != 1 {
		t.Fatalf("entities = %+v", draft.Entities)
	}
	note := RenderEntityNote(draft.Entities[0])
	parsed, err := entity.ParseMarkdownEntity([]byte(note))
	if err != nil {
		t.Fatalf("ParseMarkdownEntity: %v", err)
	}
	if parsed.ID != "saltmarch" || parsed.Type != "location" {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestClampCountsBoundsARequest(t *testing.T) {
	got := normalizeBrief(Brief{Premise: "x", Counts: Counts{Locations: 99, Factions: -3, Characters: 2}})
	if got.Counts.Locations != MaxBatch || got.Counts.Factions != 0 || got.Counts.Characters != 2 {
		t.Fatalf("counts = %+v", got.Counts)
	}
}
