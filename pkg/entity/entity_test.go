package entity

import (
	"reflect"
	"testing"
)

func TestParseMarkdownEntity(t *testing.T) {
	doc := `---
id: lady-evelyn
name: Lady Evelyn Vance
type: character
tags: [npc, rogue]
voice:
  provider: kokoro
  voice_id: bf_emma
location: "[[Alden-Tavern]]"
state:
  hp: 35
  armor: 14
---

# Lady Evelyn Vance

A former lieutenant in the Iron Guard, Evelyn hides at [[Alden-Tavern]].
She wields the [[Vorpal-Dagger|dread dagger]] and answers to [[The-Iron-Pact]].
`

	entity, err := ParseMarkdownEntity([]byte(doc))
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}

	if entity.ID != "lady-evelyn" {
		t.Errorf("expected ID 'lady-evelyn', got %q", entity.ID)
	}
	if entity.Name != "Lady Evelyn Vance" {
		t.Errorf("expected Name 'Lady Evelyn Vance', got %q", entity.Name)
	}
	if entity.Type != "character" {
		t.Errorf("expected Type 'character', got %q", entity.Type)
	}

	expectedLinks := []string{"Alden-Tavern", "Vorpal-Dagger", "The-Iron-Pact"}
	if !reflect.DeepEqual(entity.Wikilinks, expectedLinks) {
		t.Errorf("expected wikilinks %v, got %v", expectedLinks, entity.Wikilinks)
	}

	hp, ok := entity.State.Get("hp")
	if !ok || hp != 35 {
		t.Errorf("expected state hp=35, got %v", hp)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Lady Evelyn Vance":       "lady-evelyn-vance",
		"  Old Market  ":          "old-market",
		"Alden--Tavern":           "alden-tavern",
		"iron_pact":               "iron-pact",
		"Théâtre of Whispers":     "thtre-of-whispers",
		"!!!":                     "",
		"Turn 12: The Long Night": "turn-12-the-long-night",
	}

	for input, want := range cases {
		if got := Slugify(input); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWikilinkTarget(t *testing.T) {
	cases := map[string]string{
		"Alden-Tavern":                "Alden-Tavern",
		"[[Alden-Tavern]]":            "Alden-Tavern",
		"[[Vorpal-Dagger|dagger]]":    "Vorpal-Dagger",
		"  [[ The-Iron-Pact ]]  ":     "The-Iron-Pact",
		"":                            "",
		"[[Lord Vance|his lordship]]": "Lord Vance",
	}

	for input, want := range cases {
		if got := WikilinkTarget(input); got != want {
			t.Errorf("WikilinkTarget(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSerializeMarkdownEntity(t *testing.T) {
	e := &Entity{
		ID:   "alden-tavern",
		Name: "Alden Tavern",
		Type: "location",
		Tags: []string{"tavern", "safehouse"},
		Body: "A quiet tavern at the edge of the woods.\n",
	}
	e.InitState(map[string]interface{}{"capacity": 40})

	data, err := e.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}

	reparsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("reparse failed: %v", err)
	}
	if reparsed.ID != e.ID || reparsed.Name != e.Name || reparsed.Type != e.Type {
		t.Errorf("mismatch after serialization: got %+v", reparsed)
	}
}
