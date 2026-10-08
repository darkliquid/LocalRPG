package ingest

import (
	"context"
	"fmt"
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
	}, nil)
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

func TestPromptNamesTheKindsAndDemandsSubstance(t *testing.T) {
	probe := &jsonGen{responses: []string{`{}`}}
	chunks := []Chunk{{Source: "a.md", Title: "Customs", Text: "Metamorphosis Feasts are held."}}
	if _, err := Build(context.Background(), probe, chunks, worldgen.Brief{}); err != nil {
		t.Fatal(err)
	}

	// The kinds, so the model does not answer with characters and stop.
	for _, kind := range []string{"location", "character", "faction", "species", "item", "event", "concept"} {
		if !strings.Contains(probe.lastPrompt, `"`+kind+`"`) {
			t.Fatalf("the prompt does not name the %s kind:\n%s", kind, probe.lastPrompt)
		}
	}
	// Substance, so an entity is not a bare name.
	if !strings.Contains(probe.lastPrompt, "two to four sentences") {
		t.Fatalf("the prompt does not ask for a description:\n%s", probe.lastPrompt)
	}
	// Exhaustiveness, with the granularity spelled out.
	if !containsAll(probe.lastPrompt, "yields ten entities", "glossary entry") {
		t.Fatalf("the prompt does not ask for everything:\n%s", probe.lastPrompt)
	}
	// Lore, because it is collected from every part of the source.
	if !strings.Contains(probe.lastPrompt, "Include lore in every reply") {
		t.Fatalf("the prompt only asks for lore once:\n%s", probe.lastPrompt)
	}
}

func TestPromptCarriesWhatEarlierBatchesExtracted(t *testing.T) {
	probe := &jsonGen{responses: []string{
		`{"entities":[{"name":"Saltmarch","type":"location","description":"A port."}]}`,
		`{}`,
	}}
	// Nine chunks at four per call is three batches; the third must know what the
	// first two produced.
	chunks := make([]Chunk, 9)
	for i := range chunks {
		chunks[i] = Chunk{Source: "a.md", Title: "A", Text: "Some lore."}
	}
	if _, err := Build(context.Background(), probe, chunks, worldgen.Brief{}); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 3 {
		t.Fatalf("calls = %d, want 3", probe.calls)
	}
	if !containsAll(probe.lastPrompt, "Already extracted", "Saltmarch (location)") {
		t.Fatalf("a later batch does not know what came before:\n%s", probe.lastPrompt)
	}
}

func TestRepeatedEntityIsMergedNotDropped(t *testing.T) {
	probe := &jsonGen{responses: []string{
		`{"entities":[{"name":"Quezta","type":"species"}]}`,
		`{"entities":[{"name":"Quezta","type":"species","description":"Hiveborn soldiers, once Tck-Tck.","tags":["hive"]}]}`,
	}}
	chunks := make([]Chunk, 5)
	for i := range chunks {
		chunks[i] = Chunk{Source: "a.md", Title: "A", Text: "Some lore."}
	}
	d, err := Build(context.Background(), probe, chunks, worldgen.Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Entities) != 1 {
		t.Fatalf("entities = %+v", d.Entities)
	}
	// The later mention carries the description the first one lacked.
	if d.Entities[0].Body != "Hiveborn soldiers, once Tck-Tck." {
		t.Fatalf("body = %q", d.Entities[0].Body)
	}
	if len(d.Entities[0].Tags) != 1 || d.Entities[0].Tags[0] != "hive" {
		t.Fatalf("tags = %+v", d.Entities[0].Tags)
	}
}

func TestLoreIsCollectedFromEveryBatch(t *testing.T) {
	probe := &jsonGen{responses: []string{
		`{"lore":"# History\n\nOld.","entities":[]}`,
		`{"lore":"# Customs\n\nFeasts.","entities":[]}`,
	}}
	chunks := make([]Chunk, 5)
	for i := range chunks {
		chunks[i] = Chunk{Source: "a.md", Title: "A", Text: "Some lore."}
	}
	d, err := Build(context.Background(), probe, chunks, worldgen.Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(d.Lore, "# History", "Old.", "# Customs", "Feasts.") {
		t.Fatalf("lore = %q", d.Lore)
	}
}

func TestEntitiesAreFiledByKind(t *testing.T) {
	probe := &jsonGen{responses: []string{`{"entities":[
	  {"name":"Saltmarch","type":"location","description":"A port."},
	  {"name":"Maren","type":"character","description":"A harbormaster."},
	  {"name":"The Tidewatch","type":"faction","description":"A crew."}]}`}}
	d, err := Build(context.Background(), probe, []Chunk{{Source: "a.md", Text: "x"}}, worldgen.Brief{})
	if err != nil {
		t.Fatal(err)
	}
	folders := map[string]string{}
	for _, e := range d.Entities {
		folders[e.ID] = e.Folder
	}
	if folders["saltmarch"] != "locations" || folders["maren"] != "characters" || folders["the-tidewatch"] != "factions" {
		t.Fatalf("folders = %+v", folders)
	}
}

func TestProgressReportsEveryBatch(t *testing.T) {
	probe := &jsonGen{responses: []string{
		`{"entities":[{"name":"Saltmarch","type":"location","description":"A port."}]}`,
		`{"entities":[{"name":"Maren","type":"character","description":"A harbormaster."}]}`,
		`{}`,
	}}
	chunks := make([]Chunk, 9)
	for i := range chunks {
		chunks[i] = Chunk{Source: fmt.Sprintf("note-%d.md", i), Title: "Note", Text: "Some lore."}
	}

	var got []Progress
	if _, err := BuildInto(context.Background(), probe, chunks, worldgen.Brief{}, BuildContext{}, func(p Progress) {
		got = append(got, p)
	}); err != nil {
		t.Fatal(err)
	}

	// One event up front with the total, then one per batch.
	if len(got) != 4 {
		t.Fatalf("events = %d, want 4: %+v", len(got), got)
	}
	if got[0].Batch != 0 || got[0].Batches != 3 {
		t.Fatalf("first event = %+v, want the batch count up front", got[0])
	}
	if got[1].Batch != 1 || got[1].Batches != 3 || got[1].Total != 1 || got[1].Found != 1 {
		t.Fatalf("first batch = %+v", got[1])
	}
	if len(got[1].Names) != 1 || got[1].Names[0] != "Saltmarch" {
		t.Fatalf("names = %+v", got[1].Names)
	}
	if len(got[1].Sources) != 4 {
		t.Fatalf("sources = %+v, want the four files this batch read", got[1].Sources)
	}
	if got[3].Total != 2 || got[3].Found != 0 {
		t.Fatalf("last batch = %+v", got[3])
	}
}

func TestProgressReportsAMergedEntity(t *testing.T) {
	probe := &jsonGen{responses: []string{
		`{"entities":[{"name":"Quezta","type":"species"}]}`,
		`{"entities":[{"name":"Quezta","type":"species","description":"Hiveborn soldiers."}]}`,
	}}
	chunks := make([]Chunk, 5)
	for i := range chunks {
		chunks[i] = Chunk{Source: "a.md", Title: "A", Text: "Some lore."}
	}

	var last Progress
	if _, err := BuildInto(context.Background(), probe, chunks, worldgen.Brief{}, BuildContext{}, func(p Progress) {
		if p.Batch > 0 {
			last = p
		}
	}); err != nil {
		t.Fatal(err)
	}
	// A repeated name improves a note, which is work worth reporting.
	if last.Found != 1 || last.Total != 1 || last.Names[0] != "Quezta" {
		t.Fatalf("last = %+v", last)
	}
}

func TestATruncatedReplyKeepsWhatItWrote(t *testing.T) {
	// A reply cut off mid-entity: the two before the cut are whole, and losing
	// them, or failing the whole import, would be the wrong answer.
	probe := &jsonGen{responses: []string{
		`{"lore":"The city.","entities":[{"name":"Saltmarch","type":"location","description":"A port."},{"name":"Maren","type":"character","description":"A harbormaster."},{"name":"Quezta","ty`,
	}}
	var last Progress
	d, err := BuildInto(context.Background(), probe, []Chunk{{Source: "a.md", Text: "x"}}, worldgen.Brief{}, BuildContext{}, func(p Progress) {
		if p.Batch > 0 {
			last = p
		}
	})
	if err != nil {
		t.Fatalf("a cut-off reply should not fail the import: %v", err)
	}
	if len(d.Entities) != 2 {
		t.Fatalf("entities = %+v", d.Entities)
	}
	if last.CutOff != 1 {
		t.Fatalf("the progress should report the cut-off batch: %+v", last)
	}
}

func TestAnUnparseableBatchIsSplitRatherThanLost(t *testing.T) {
	// The first call answers with nothing usable; the halves do. Four pages is
	// two calls of two, so no entity is lost to one oversized reply.
	probe := &jsonGen{responses: []string{
		`not json at all`,
		`{"entities":[{"name":"Saltmarch","type":"location","description":"A port."}]}`,
		`{"entities":[{"name":"Maren","type":"character","description":"A harbormaster."}]}`,
	}}
	chunks := []Chunk{
		{Source: "a.md", Text: "one"}, {Source: "b.md", Text: "two"},
		{Source: "c.md", Text: "three"}, {Source: "d.md", Text: "four"},
	}
	d, err := BuildInto(context.Background(), probe, chunks, worldgen.Brief{}, BuildContext{}, nil)
	if err != nil {
		t.Fatalf("a split should recover the batch: %v", err)
	}
	if len(d.Entities) != 2 {
		t.Fatalf("entities = %+v", d.Entities)
	}
	if probe.calls != 3 {
		t.Fatalf("calls = %d, want 3 (the batch, then its two halves)", probe.calls)
	}
}

func TestABatchWithOnePageThatFailsIsSkippedNotFatal(t *testing.T) {
	probe := &jsonGen{responses: []string{`not json`, `{}`}}
	chunks := []Chunk{{Source: "a.md", Text: "one"}}
	var last Progress
	d, err := BuildInto(context.Background(), probe, chunks, worldgen.Brief{}, BuildContext{}, func(p Progress) {
		if p.Batch > 0 {
			last = p
		}
	})
	if err != nil {
		t.Fatalf("one unreadable page should not fail the import: %v", err)
	}
	if len(d.Entities) != 0 {
		t.Fatalf("entities = %+v", d.Entities)
	}
	if last.CutOff != 1 {
		t.Fatalf("the progress should report it: %+v", last)
	}
}
