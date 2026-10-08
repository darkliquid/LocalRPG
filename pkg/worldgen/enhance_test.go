package worldgen

import (
	"context"
	"strings"
	"testing"
)

func TestEnhanceReturnsCappedProposals(t *testing.T) {
	g := &jsonGen{responses: []string{`{"proposals":[
	  {"kind":"lore","title":"History","body":"Long ago..."},
	  {"kind":"hook","title":"The debt","body":"A creditor arrives."}]}`}}
	got, err := Enhance(context.Background(), g, WorldContext{ID: "w"}, "deepen it", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Kind != "lore" || got[1].Kind != "hook" {
		t.Fatalf("proposals = %+v", got)
	}
}

func TestEnhanceLinksAnEntityProposal(t *testing.T) {
	g := &jsonGen{responses: []string{`{"proposals":[{"kind":"entity","title":"Rival","body":"x",
	  "entity":{"name":"The Salt Circle","type":"faction","description":"Rivals of [[The Tidewatch]]."}}]}`}}
	world := WorldContext{ID: "w", Entities: []EntitySummary{{ID: "the-tidewatch", Name: "The Tidewatch"}}}
	got, err := Enhance(context.Background(), g, world, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Entity == nil {
		t.Fatalf("proposals = %+v", got)
	}
	if !strings.Contains(got[0].Entity.Body, "[[The Tidewatch]]") {
		t.Fatalf("body = %q", got[0].Entity.Body)
	}
	if got[0].Entity.ID != "the-salt-circle" {
		t.Fatalf("id = %q", got[0].Entity.ID)
	}
}

func TestEnhanceCapsTheProposalList(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"proposals":[`)
	for i := 0; i < MaxProposals+5; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"kind":"lore","title":"T","body":"b"}`)
	}
	b.WriteString(`]}`)
	g := &jsonGen{responses: []string{b.String()}}
	got, err := Enhance(context.Background(), g, WorldContext{ID: "w"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxProposals {
		t.Fatalf("proposals = %d, want %d", len(got), MaxProposals)
	}
}

func TestEnhanceDropsAnUnknownKind(t *testing.T) {
	g := &jsonGen{responses: []string{`{"proposals":[{"kind":"nonsense","title":"T","body":"b"}]}`}}
	got, err := Enhance(context.Background(), g, WorldContext{ID: "w"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("proposals = %+v", got)
	}
}

func TestEnhancePromptCarriesTheWorld(t *testing.T) {
	g := &jsonGen{responses: []string{`{}`}}
	world := WorldContext{
		ID: "w", Name: "Ashen Reach", Lore: "Old lore.",
		Entities: []EntitySummary{{ID: "saltmarch", Name: "Saltmarch", Type: "location"}},
	}
	if _, err := Enhance(context.Background(), g, world, "deepen it", []string{"lore"}); err != nil {
		t.Fatal(err)
	}
	if !containsAll(g.lastPrompt, "Ashen Reach", "Old lore.", "saltmarch", "deepen it", "lore") {
		t.Fatalf("prompt = %q", g.lastPrompt)
	}
}
