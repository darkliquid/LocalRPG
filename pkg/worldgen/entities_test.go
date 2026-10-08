package worldgen

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateEntitiesClampsAndGenerates(t *testing.T) {
	g := &jsonGen{responses: []string{`{"entities":[{"name":"A","type":"faction"},{"name":"B","type":"faction"}]}`}}
	got, err := GenerateEntities(context.Background(), g, WorldContext{ID: "w"}, EntityRequest{Instruction: "x", Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("entities = %d", len(got))
	}
	if got[0].ID != "a" || got[0].Type != "faction" {
		t.Fatalf("first = %+v", got[0])
	}
	if _, err := GenerateEntities(context.Background(), g, WorldContext{ID: "w"}, EntityRequest{Count: 99}); err != nil {
		t.Fatal(err)
	}
}

func TestEntityRequestIsClamped(t *testing.T) {
	got := normalizeEntityRequest(EntityRequest{Count: 99})
	if got.Count != MaxBatch {
		t.Fatalf("count = %d, want %d", got.Count, MaxBatch)
	}
	if got := normalizeEntityRequest(EntityRequest{}); got.Count != 3 {
		t.Fatalf("default count = %d, want 3", got.Count)
	}
}

func TestEntityPromptIsBounded(t *testing.T) {
	w := WorldContext{ID: "w", Lore: strings.Repeat("lore ", 5000)}
	for i := 0; i < 500; i++ {
		w.Entities = append(w.Entities, EntitySummary{ID: "e" + string(rune('a'+i%26)), Name: "X", Type: "character"})
	}
	p := buildEntityPrompt(w, EntityRequest{Instruction: "add a faction"})
	if len(p) > 8000 {
		t.Fatalf("prompt too long: %d", len(p))
	}
	if !strings.Contains(p, "add a faction") {
		t.Fatal("the instruction must be present")
	}
}

func TestLinkBatchResolvesExisting(t *testing.T) {
	batch := []DraftEntity{{ID: "new-crew", Body: "Rivals of [[The Tidewatch]]."}}
	existing := []EntitySummary{{ID: "the-tidewatch", Name: "The Tidewatch"}}
	got := linkBatch(batch, existing)
	if !strings.Contains(got[0].Body, "[[The Tidewatch]]") {
		t.Fatal("a link to an existing entity should survive")
	}
}

func TestLinkBatchDropsAnUnknownLink(t *testing.T) {
	batch := []DraftEntity{{ID: "new-crew", Body: "Rivals of [[Nobody]]."}}
	got := linkBatch(batch, nil)
	if strings.Contains(got[0].Body, "[[Nobody]]") {
		t.Fatal("an unresolved link should be dropped")
	}
	if len(got[0].Dropped) != 1 || got[0].Dropped[0] != "Nobody" {
		t.Fatalf("dropped = %+v", got[0].Dropped)
	}
}

func TestGenerateEntitiesSeedsFromTheWorld(t *testing.T) {
	g := &jsonGen{responses: []string{`{"entities":[{"name":"C","type":"faction"}]}`}}
	world := WorldContext{
		ID: "w", Name: "Ashen Reach", Lore: "A dying frontier.",
		Entities: []EntitySummary{{ID: "saltmarch", Name: "Saltmarch", Type: "location"}},
	}
	if _, err := GenerateEntities(context.Background(), g, world, EntityRequest{Instruction: "x"}); err != nil {
		t.Fatal(err)
	}
	if !containsAll(g.lastPrompt, "Ashen Reach", "A dying frontier.", "saltmarch", "x") {
		t.Fatalf("prompt = %q", g.lastPrompt)
	}
}

func TestGenerateEntitiesWithoutAGeneratorErrors(t *testing.T) {
	if _, err := GenerateEntities(context.Background(), nil, WorldContext{}, EntityRequest{}); err == nil {
		t.Fatal("a nil generator must error")
	}
}

func TestGenerateEntitiesKeepsWhatATruncatedReplyWrote(t *testing.T) {
	// A reply cut off inside a string, which closing structures cannot repair.
	g := &jsonGen{responses: []string{
		"```json\n{\"entities\":[{\"name\":\"A\",\"type\":\"faction\",\"description\":\"First.\"},{\"name\":\"B",
	}}
	got, err := GenerateEntities(context.Background(), g, WorldContext{ID: "w"}, EntityRequest{Count: 5})
	if err != nil {
		t.Fatalf("a cut-off reply should not fail the batch: %v", err)
	}
	if len(got) != 1 || got[0].Name != "A" {
		t.Fatalf("entities = %+v", got)
	}
}

func TestSalvageObjectsStripsAFence(t *testing.T) {
	raw := []byte("```json\n{\"entities\":[{\"name\":\"A\"},{\"name\":\"B")
	got := SalvageObjects(raw)
	if len(got) != 1 || string(got[0]) != `{"name":"A"}` {
		t.Fatalf("objects = %q", got)
	}
}
