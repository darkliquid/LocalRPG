package entity

import (
	"errors"
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
	e.InitState(map[string]any{"capacity": 40})

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
			Options:    map[string]any{"stability": 0.35, "similarity_boost": 0.8},
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

func TestDeclaredID(t *testing.T) {
	cases := map[string]string{
		"---\nid: silver-hand\nname: X\n---\n\nbody\n": "silver-hand",
		"---\nname: X\n---\n\nbody\n":                  "",
		"no frontmatter at all":                        "",
		"---\nid: broken\n":                            "",
	}
	for input, want := range cases {
		if got := DeclaredID([]byte(input)); got != want {
			t.Errorf("DeclaredID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFrontmatterErrorNamesItsLine(t *testing.T) {
	// The third document line is invalid: a value with an unclosed quote.
	doc := "---\nid: silver-hand\nname: \"unclosed\ntype: faction\n---\n\nBody.\n"

	_, err := ParseMarkdownEntity([]byte(doc))
	if err == nil {
		t.Fatal("expected a parse failure")
	}

	var fe *FrontmatterError
	if !errors.As(err, &fe) {
		t.Fatalf("error is %T, want *FrontmatterError", err)
	}
	if fe.Offset <= 0 {
		t.Fatalf("Offset = %d, want a positive offset into the document", fe.Offset)
	}
	if line := fe.Line(doc); line < 3 || line > 4 {
		t.Errorf("Line = %d, want the broken line (3 or 4)", line)
	}
	if column := fe.Column(doc); column < 1 {
		t.Errorf("Column = %d, want a 1-based column", column)
	}
}

func TestFrontmatterErrorOffsetPointsIntoTheDocument(t *testing.T) {
	doc := "---\nid: silver-hand\nname: X\n\tbad: [unclosed\n---\n\nBody.\n"

	_, err := ParseMarkdownEntity([]byte(doc))
	if err == nil {
		t.Fatal("expected a parse failure")
	}

	var fe *FrontmatterError
	if !errors.As(err, &fe) {
		t.Fatalf("error is %T, want *FrontmatterError", err)
	}
	if fe.Offset >= len(doc) {
		t.Fatalf("Offset = %d, want it inside the %d-byte document", fe.Offset, len(doc))
	}
	// The offset must land inside the frontmatter block, never on the header.
	if fe.Offset < len("---\n") {
		t.Fatalf("Offset = %d, want it past the frontmatter header", fe.Offset)
	}
}

func TestFrontmatterErrorLineAndColumnAreOneBased(t *testing.T) {
	doc := "---\nid: x\n---\n\nBody.\n"

	fe := &FrontmatterError{Offset: len("---\n"), Msg: "at the start of the frontmatter"}
	if line := fe.Line(doc); line != 2 {
		t.Errorf("Line = %d, want 2 (the line after the header)", line)
	}
	if column := fe.Column(doc); column != 1 {
		t.Errorf("Column = %d, want 1", column)
	}

	atStart := &FrontmatterError{Offset: 0, Msg: "at the very start"}
	if line := atStart.Line(doc); line != 1 {
		t.Errorf("Line = %d, want 1", line)
	}
}

func TestCurrentAppearanceRoundTripsWithoutBecomingExtraMeta(t *testing.T) {
	note := []byte("---\nid: kaelen\nname: Kaelen\ntype: character\n" +
		"appearance: Silver hair.\ncurrent_appearance: Soaked through.\n---\nA traveller.\n")

	ent, err := ParseMarkdownEntity(note)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ent.CurrentAppearance != "Soaked through." {
		t.Errorf("CurrentAppearance = %q, want the parsed value", ent.CurrentAppearance)
	}
	if _, ok := ent.ExtraMeta["current_appearance"]; ok {
		t.Error("current_appearance must not also land in ExtraMeta")
	}

	out, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	back, err := ParseMarkdownEntity(out)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if back.CurrentAppearance != "Soaked through." {
		t.Errorf("round trip lost the current appearance: %q", back.CurrentAppearance)
	}
	if back.Appearance != "Silver hair." {
		t.Errorf("round trip lost the baseline: %q", back.Appearance)
	}
}

func TestSerializeOmitsAnUnsetCurrentAppearance(t *testing.T) {
	ent := &Entity{ID: "kaelen", Name: "Kaelen", Type: "character", Appearance: "Silver hair.", Body: "A traveller.\n"}
	out, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if strings.Contains(string(out), "current_appearance") {
		t.Errorf("an unset current appearance must not be emitted:\n%s", out)
	}
}

func TestAllFieldsRoundTripMarkdown(t *testing.T) {
	ent := &Entity{
		ID:   "val-the-brave",
		Name: "Val the Brave",
		Type: "character",
		Tags: []string{"paladin", "champion"},
		Voice: &VoiceConfig{
			Provider:   "kokoro",
			VoiceID:    "af_sarah",
			Pitch:      1.15,
			SpeechRate: 0.95,
			Options:    map[string]any{"volume": "high"},
		},
		Portrait:          "portraits/val.png",
		PortraitVersion:   4,
		PortraitHistory:   []string{"portraits/val_v1.png", "portraits/val_v2.png"},
		Location:          "high-sanctuary",
		Faction:           "order-of-light",
		Appearance:        "A tall warrior with burnished silver plate armor.",
		CurrentAppearance: "Silver armor dented on the left pauldron.",
		Gender:            "non-binary",
		Age:               "32",
		Aliases:           []string{"Val", "The Silver Shield"},
		History:           []int{1, 3, 7},
		ExtraMeta:         map[string]any{"custom_title": "Knight Protector"},
		Body:              "\nVal swore an oath to protect [[high-sanctuary]] for the [[order-of-light]].\n",
		Wikilinks:         []string{"high-sanctuary", "order-of-light"},
		Hash:              "abc123hash",
		Folder:            "characters",
	}
	ent.InitState(map[string]any{
		"heroic": true,
		"title":  "Champion",
	})

	// Guard against unpopulated test fields
	val := reflect.ValueOf(ent).Elem()
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)
		if isFieldZero(fieldVal) {
			t.Fatalf("TestAllFieldsRoundTripMarkdown leaves Entity.%s unpopulated", field.Name)
		}
	}

	data, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown: %v", err)
	}

	reparsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("ParseMarkdownEntity: %v", err)
	}

	if reparsed.ID != ent.ID {
		t.Errorf("ID = %q, want %q", reparsed.ID, ent.ID)
	}
	if reparsed.Name != ent.Name {
		t.Errorf("Name = %q, want %q", reparsed.Name, ent.Name)
	}
	if reparsed.Type != ent.Type {
		t.Errorf("Type = %q, want %q", reparsed.Type, ent.Type)
	}
	if !reflect.DeepEqual(reparsed.Tags, ent.Tags) {
		t.Errorf("Tags = %v, want %v", reparsed.Tags, ent.Tags)
	}
	if !reflect.DeepEqual(reparsed.Voice, ent.Voice) {
		t.Errorf("Voice = %+v, want %+v", reparsed.Voice, ent.Voice)
	}
	if reparsed.Portrait != ent.Portrait {
		t.Errorf("Portrait = %q, want %q", reparsed.Portrait, ent.Portrait)
	}
	if reparsed.PortraitVersion != ent.PortraitVersion {
		t.Errorf("PortraitVersion = %d, want %d", reparsed.PortraitVersion, ent.PortraitVersion)
	}
	if !reflect.DeepEqual(reparsed.PortraitHistory, ent.PortraitHistory) {
		t.Errorf("PortraitHistory = %v, want %v", reparsed.PortraitHistory, ent.PortraitHistory)
	}
	if reparsed.Location != ent.Location {
		t.Errorf("Location = %q, want %q", reparsed.Location, ent.Location)
	}
	if reparsed.Faction != ent.Faction {
		t.Errorf("Faction = %q, want %q", reparsed.Faction, ent.Faction)
	}
	if reparsed.Appearance != ent.Appearance {
		t.Errorf("Appearance = %q, want %q", reparsed.Appearance, ent.Appearance)
	}
	if reparsed.CurrentAppearance != ent.CurrentAppearance {
		t.Errorf("CurrentAppearance = %q, want %q", reparsed.CurrentAppearance, ent.CurrentAppearance)
	}
	if reparsed.Gender != ent.Gender {
		t.Errorf("Gender = %q, want %q", reparsed.Gender, ent.Gender)
	}
	if reparsed.Age != ent.Age {
		t.Errorf("Age = %q, want %q", reparsed.Age, ent.Age)
	}
	if !reflect.DeepEqual(reparsed.Aliases, ent.Aliases) {
		t.Errorf("Aliases = %v, want %v", reparsed.Aliases, ent.Aliases)
	}
	if !reflect.DeepEqual(reparsed.History, ent.History) {
		t.Errorf("History = %v, want %v", reparsed.History, ent.History)
	}
	if !reflect.DeepEqual(reparsed.State.Raw(), ent.State.Raw()) {
		t.Errorf("State = %v, want %v", reparsed.State.Raw(), ent.State.Raw())
	}
	if !reflect.DeepEqual(reparsed.ExtraMeta, ent.ExtraMeta) {
		t.Errorf("ExtraMeta = %v, want %v", reparsed.ExtraMeta, ent.ExtraMeta)
	}
	if strings.TrimSpace(reparsed.Body) != strings.TrimSpace(ent.Body) {
		t.Errorf("Body = %q, want %q", reparsed.Body, ent.Body)
	}
	if !reflect.DeepEqual(reparsed.Wikilinks, ent.Wikilinks) {
		t.Errorf("Wikilinks = %v, want %v", reparsed.Wikilinks, ent.Wikilinks)
	}

	// Verify standard fields are not in ExtraMeta
	standardKeys := []string{
		"id", "name", "type", "tags", "voice", "portrait", "portrait_version",
		"portrait_history", "location", "faction", "appearance", "current_appearance",
		"gender", "age", "aliases", "history", "state",
	}
	for _, key := range standardKeys {
		if _, exists := reparsed.ExtraMeta[key]; exists {
			t.Errorf("standard field %q leaked into ExtraMeta", key)
		}
	}
}

func isFieldZero(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}
