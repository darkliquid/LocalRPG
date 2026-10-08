package ingest

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/worldgen"
)

func TestBuildProducesADraftWithProvenance(t *testing.T) {
	chunks := []Chunk{{Source: "a.md", Title: "Saltmarch", Text: "A port."}}
	g := &jsonGen{responses: []string{`{"entities":[{"name":"Saltmarch","type":"location"}]}`}}
	d, err := Build(context.Background(), g, chunks, worldgen.Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Entities) != 1 || d.Entities[0].Source != "a.md" {
		t.Fatalf("draft = %+v", d)
	}
	if d.Entities[0].Type != "location" {
		t.Fatalf("type = %q", d.Entities[0].Type)
	}
}

func TestBuildTakesTheOutlineFromTheFirstBatch(t *testing.T) {
	chunks := []Chunk{{Source: "a.md", Title: "Saltmarch", Text: "A port."}}
	g := &jsonGen{responses: []string{
		`{"name":"Saltmarch Reach","genre":"nautical","description":"A wet frontier.","lore":"# Lore\n\nWet.\n",
		  "entities":[{"name":"Saltmarch","type":"location"}]}`,
	}}
	d, err := Build(context.Background(), g, chunks, worldgen.Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if d.World.Name != "Saltmarch Reach" || d.ID != "saltmarch-reach" {
		t.Fatalf("world = %+v id %q", d.World, d.ID)
	}
	if d.World.Genre != "nautical" || !strings.Contains(d.Lore, "Wet.") {
		t.Fatalf("draft = %+v", d)
	}
	if len(d.Sections) == 0 {
		t.Fatalf("sections should be split: %+v", d)
	}
}

func TestBuildLinksEntitiesAcrossBatches(t *testing.T) {
	chunks := []Chunk{
		{Source: "a.md", Title: "A", Text: "A port."},
		{Source: "b.md", Title: "B", Text: "A crew."},
		{Source: "c.md", Title: "C", Text: "A storm."},
		{Source: "d.md", Title: "D", Text: "A reef."},
		{Source: "e.md", Title: "E", Text: "A wreck."},
	}
	g := &jsonGen{responses: []string{
		`{"name":"Reach","entities":[{"name":"Saltmarch","type":"location","description":"Watched by [[The Tidewatch]]."}]}`,
		`{"entities":[{"name":"The Tidewatch","type":"faction","description":"Based at [[Saltmarch]]."}]}`,
	}}
	d, err := Build(context.Background(), g, chunks, worldgen.Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Entities) != 2 {
		t.Fatalf("entities = %+v", d.Entities)
	}
	for _, e := range d.Entities {
		if len(e.Links) != 1 {
			t.Fatalf("entity %s links = %+v", e.ID, e.Links)
		}
	}
	if !strings.Contains(d.Entities[0].Source, "a.md") {
		t.Fatalf("source = %q", d.Entities[0].Source)
	}
	if !strings.Contains(d.Entities[1].Source, "e.md") {
		t.Fatalf("second batch source = %q", d.Entities[1].Source)
	}
}

func TestBuildRefusesAnEmptySource(t *testing.T) {
	if _, err := Build(context.Background(), &jsonGen{}, nil, worldgen.Brief{}); err == nil {
		t.Fatal("an empty chunk list must error")
	}
	if _, err := Build(context.Background(), nil, []Chunk{{Text: "x"}}, worldgen.Brief{}); err == nil {
		t.Fatal("a nil generator must error")
	}
}

func TestBuildIntoAnExistingWorld(t *testing.T) {
	chunks := []Chunk{{Source: "a.md", Title: "Saltmarch", Text: "A port."}}
	g := &jsonGen{responses: []string{
		`{"entities":[{"name":"Saltmarch","type":"location","description":"A port watched by [[The Tidewatch]]."}]}`,
	}}
	d, err := BuildInto(context.Background(), g, chunks, worldgen.Brief{}, BuildContext{
		Name:        "Ember Peak",
		Genre:       "fantasy",
		Description: "A frontier town.",
		Lore:        "# Lore\n\nThe frontier.\n",
		Entities:    []worldgen.EntitySummary{{ID: "the-tidewatch", Name: "The Tidewatch", Type: "faction"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The world's identity is kept, and the link to its own note survives.
	if d.World.Name != "Ember Peak" || d.World.Genre != "fantasy" {
		t.Fatalf("world = %+v", d.World)
	}
	if len(d.Entities) != 1 || !strings.Contains(d.Entities[0].Body, "[[The Tidewatch]]") {
		t.Fatalf("entities = %+v", d.Entities)
	}
	if len(d.Entities[0].Links) != 1 {
		t.Fatalf("links = %+v", d.Entities[0].Links)
	}

	// The prompt tells the model which world it is joining.
	if !containsAll(g.lastPrompt, "Ember Peak", "fantasy", "The Tidewatch", "already has") {
		t.Fatalf("prompt = %q", g.lastPrompt)
	}
}

func TestBuildPromptCarriesTheChunks(t *testing.T) {
	g := &jsonGen{responses: []string{`{}`}}
	chunks := []Chunk{{Source: "a.md", Title: "Saltmarch", Text: "A port with a long history."}}
	if _, err := Build(context.Background(), g, chunks, worldgen.Brief{Premise: "keep it nautical"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g.lastPrompt, "A port with a long history.") {
		t.Fatalf("prompt = %q", g.lastPrompt)
	}
	if !strings.Contains(g.lastPrompt, "keep it nautical") {
		t.Fatalf("prompt = %q", g.lastPrompt)
	}
}
