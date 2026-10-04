package entity

import (
	"reflect"
	"strings"
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

func TestAliasesRoundTripThroughMarkdown(t *testing.T) {
	original := &Entity{
		ID:      "guard-kael",
		Name:    "Guard Kael",
		Type:    "character",
		Body:    "A warden of the ember.",
		Aliases: []string{"The Ember Warden", "Kael"},
	}

	data, err := original.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}

	parsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}
	if len(parsed.Aliases) != 2 {
		t.Fatalf("Aliases = %v, want two", parsed.Aliases)
	}
	if parsed.Aliases[0] != "The Ember Warden" || parsed.Aliases[1] != "Kael" {
		t.Errorf("aliases did not round-trip: %v", parsed.Aliases)
	}
}

func TestAnEntityWithoutAliasesHasNone(t *testing.T) {
	parsed, err := ParseMarkdownEntity([]byte("---\nid: sera\nname: Sera\ntype: character\n---\nBody.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Aliases) != 0 {
		t.Errorf("expected no aliases, got %v", parsed.Aliases)
	}
}

func TestIsCharacterType(t *testing.T) {
	for _, want := range []string{"character", "Character", "npc", "NPC", "person", " creature "} {
		if !IsCharacterType(want) {
			t.Errorf("IsCharacterType(%q) = false, want true", want)
		}
	}
	for _, got := range []string{"location", "item", "faction", "arc", ""} {
		if IsCharacterType(got) {
			t.Errorf("IsCharacterType(%q) = true, want false", got)
		}
	}
}

func TestVoiceOptionsRoundTripThroughFrontmatter(t *testing.T) {
	original := &Entity{
		ID:   "aldric",
		Name: "Aldric",
		Type: "character",
		Body: "A guarded mercenary.",
		Voice: &VoiceConfig{
			Provider:   "builtin:elevenlabs",
			VoiceID:    "EXAVITQu4vr4xnSDxMaL",
			SpeechRate: 1,
			Options:    map[string]interface{}{"stability": 0.35, "similarity_boost": 0.8},
		},
	}

	data, err := original.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown: %v", err)
	}
	parsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("ParseMarkdownEntity: %v", err)
	}
	if parsed.Voice == nil {
		t.Fatalf("voice did not round-trip")
	}
	if parsed.Voice.Options["stability"] != 0.35 || parsed.Voice.Options["similarity_boost"] != 0.8 {
		t.Errorf("options = %v", parsed.Voice.Options)
	}
}

func TestEntityFrontmatterGenderAgeAndPortrait(t *testing.T) {
	raw := `---
id: elena-vance
name: Elena Vance
type: character
gender: female
age: "32"
appearance: A tall pilot with silver hair.
portrait: assets/portraits/elena-vance.png
---
Experienced navigator of the Maw.`

	ent, err := ParseMarkdownEntity([]byte(raw))
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}

	if ent.Gender != "female" {
		t.Errorf("Gender = %q, want female", ent.Gender)
	}
	if ent.Age != "32" {
		t.Errorf("Age = %q, want 32", ent.Age)
	}
	if ent.Portrait != "assets/portraits/elena-vance.png" {
		t.Errorf("Portrait = %q, want assets/portraits/elena-vance.png", ent.Portrait)
	}
	if ent.Appearance != "A tall pilot with silver hair." {
		t.Errorf("Appearance = %q, want expected appearance", ent.Appearance)
	}

	// Verify serialization round-trips
	data, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}
	reparsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("ParseMarkdownEntity roundtrip failed: %v", err)
	}
	if reparsed.Gender != "female" || reparsed.Age != "32" || reparsed.Portrait != "assets/portraits/elena-vance.png" {
		t.Errorf("Roundtrip mismatch: %+v", reparsed)
	}
}

func TestEntityPortraitVersioningFields(t *testing.T) {
	ent := &Entity{
		ID:              "vera",
		Name:            "Vera",
		Type:            "character",
		Portrait:        "assets/portraits/vera-v2.png",
		PortraitVersion: 2,
		PortraitHistory: []string{"assets/portraits/vera-v1.png"},
	}

	data, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}

	reparsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}

	if reparsed.PortraitVersion != 2 {
		t.Errorf("PortraitVersion = %d, want 2", reparsed.PortraitVersion)
	}
	if len(reparsed.PortraitHistory) != 1 || reparsed.PortraitHistory[0] != "assets/portraits/vera-v1.png" {
		t.Errorf("PortraitHistory = %v, want [assets/portraits/vera-v1.png]", reparsed.PortraitHistory)
	}
}

func TestSerializeMarkdownOmitsFolder(t *testing.T) {
	ent := &Entity{
		ID:      "silver-hand",
		Name:    "Silver Hand",
		Type:    "faction",
		Folder:  "factions/orders",
		Body:    "A guild of smiths.\n",
		Aliases: []string{"The Hand"},
	}

	data, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown: %v", err)
	}
	if strings.Contains(string(data), "folder") {
		t.Fatalf("folder is a location, not frontmatter; got:\n%s", data)
	}
	if !strings.Contains(string(data), "id: silver-hand") {
		t.Fatalf("frontmatter lost the id:\n%s", data)
	}
}

func TestWikilinkBasename(t *testing.T) {
	cases := map[string]string{
		"silver-hand":            "silver-hand",
		"guilds/silver-hand":     "silver-hand",
		"guilds/orders/the-hand": "the-hand",
		"  guilds/silver-hand  ": "silver-hand",
		"":                       "",
	}
	for input, want := range cases {
		if got := WikilinkBasename(input); got != want {
			t.Errorf("WikilinkBasename(%q) = %q, want %q", input, got, want)
		}
	}
}
