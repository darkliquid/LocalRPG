package harness

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func newTestEntityStore(t *testing.T) *storage.Store {
	t.Helper()

	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func saveExtractorEntity(t *testing.T, store *storage.Store, ent *entity.Entity) {
	t.Helper()

	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity(%q) failed: %v", ent.ID, err)
	}
}

func TestExtractAndSyncEntities(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Mock model provider that outputs structured JSON entity discovery
	mockModel := &mockProvider{
		id: "extractor-model",
		output: `[
			{
				"id": "garrick-the-fence",
				"name": "Garrick the Fence",
				"type": "character",
				"location": "[[alden-tavern]]",
				"body": "A shadowy broker dealing in stolen trinkets."
			}
		]`,
	}

	extractor := NewExtractor(mockModel)
	records, err := extractor.Extract(context.Background(), "You meet Garrick in the corner of the tavern.")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(records.Entities) != 1 || records.Entities[0].Name != "Garrick the Fence" {
		t.Fatalf("expected one extracted record, got %+v", records.Entities)
	}

	// Persisting records is the caller's job.
	if _, err := store.GetEntity("garrick-the-fence"); err == nil {
		t.Errorf("expected extraction to leave persistence to the caller")
	}
}

func TestEntityExtractor_AutoAssignsVoiceProfile(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	mockModel := &mockProvider{
		id: "extractor-model",
		output: `[
			{
				"id": "old-garrow",
				"name": "Old Garrow",
				"type": "character",
				"location": "[[alden-tavern]]",
				"body": "An elder veteran resting by the hearth with a pipe."
			}
		]`,
	}

	profiles := []config.VoiceProfile{
		{ID: "elder_sage", VoiceID: "bm_george", Pitch: 0.85, SpeechRate: 0.90, Tags: []string{"elder", "veteran"}},
	}

	extractor := NewExtractor(mockModel)
	extractor.SetVoiceProfiles(profiles)

	records, err := extractor.Extract(context.Background(), "You meet an elder veteran named [[Old Garrow]].")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if len(records.Entities) != 1 {
		t.Fatalf("expected 1 entity extracted, got %d", len(records.Entities))
	}

	garrow := &entity.Entity{
		ID:   records.Entities[0].ID,
		Name: records.Entities[0].Name,
		Type: records.Entities[0].Type,
		Body: records.Entities[0].Body,
	}
	AssignVoiceProfile(garrow, profiles)
	if garrow.Voice == nil || garrow.Voice.VoiceID != "bm_george" {
		t.Errorf("expected auto-assigned voice profile bm_george, got %+v", garrow.Voice)
	}
}

func TestMatchExistingEntityByIdentifiers(t *testing.T) {
	store := newTestEntityStore(t)
	saveExtractorEntity(t, store, &entity.Entity{
		ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Hash: "hash-tavern",
	})
	saveExtractorEntity(t, store, &entity.Entity{
		ID: "lady-evelyn", Name: "Lady Evelyn Vance", Type: "character", Hash: "hash-evelyn",
	})

	cases := []struct {
		name string
		raw  ExtractedEntity
		want string
	}{
		{
			name: "exact id",
			raw:  ExtractedEntity{ID: "alden-tavern", Name: "The Old Tavern", Type: "location"},
			want: "alden-tavern",
		},
		{
			name: "exact name",
			raw:  ExtractedEntity{ID: "evelyn-vance", Name: "Lady Evelyn Vance", Type: "character"},
			want: "lady-evelyn",
		},
		{
			name: "partial name",
			raw:  ExtractedEntity{ID: "evelyn", Name: "Evelyn", Type: "character"},
			want: "lady-evelyn",
		},
		{
			name: "unrelated",
			raw:  ExtractedEntity{ID: "garrick", Name: "Garrick", Type: "character"},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched := MatchExistingEntity(store, &tc.raw)
			if tc.want == "" {
				if matched != nil {
					t.Fatalf("expected no match, got %q", matched.ID)
				}
				return
			}
			if matched == nil {
				t.Fatalf("expected match %q, got none", tc.want)
			}
			if matched.ID != tc.want {
				t.Errorf("expected match %q, got %q", tc.want, matched.ID)
			}
		})
	}
}

func TestMatchExistingEntityByContext(t *testing.T) {
	store := newTestEntityStore(t)
	saveExtractorEntity(t, store, &entity.Entity{
		ID:       "watch-captain",
		Name:     "Captain Kaelen",
		Type:     "character",
		Tags:     []string{"captain", "guard"},
		Location: "[[harbour-gate]]",
		Hash:     "hash-captain",
	})

	raw := ExtractedEntity{
		ID:       "kaelen",
		Name:     "The Harbour Watchman",
		Type:     "character",
		Location: "[[harbour-gate]]",
		Body:     "A stern guard captain who remembers faces.",
	}

	matched := MatchExistingEntity(store, &raw)
	if matched == nil || matched.ID != "watch-captain" {
		t.Fatalf("expected contextual match to watch-captain, got %+v", matched)
	}
}

func TestResolveEntityMentions(t *testing.T) {
	store := newTestEntityStore(t)
	saveExtractorEntity(t, store, &entity.Entity{ID: "hero", Name: "Sean", Type: "character", Hash: "h1"})
	saveExtractorEntity(t, store, &entity.Entity{ID: "aldon-tavern", Name: "Alden Tavern", Type: "location", Hash: "h2"})
	saveExtractorEntity(t, store, &entity.Entity{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Hash: "h3"})

	mentions := ResolveEntityMentions(store, "hero", "aldon-tavern", "Garrick nods at [[Garrick the Fence]] and [[Nobody At All]].")
	if len(mentions) != 3 {
		t.Fatalf("expected 3 mentions, got %+v", mentions)
	}
	if mentions[0].ID != "hero" || mentions[0].Kind != entity.MentionPlayer {
		t.Errorf("expected the player first, got %+v", mentions[0])
	}
	if mentions[1].ID != "aldon-tavern" || mentions[1].Kind != entity.MentionLocation {
		t.Errorf("expected the location second, got %+v", mentions[1])
	}
	if mentions[2].ID != "garrick-the-fence" || mentions[2].Kind != entity.MentionWikilink {
		t.Errorf("expected the wikilink third, got %+v", mentions[2])
	}
}

func TestResolveEntityMentionsSkipsMissingEntities(t *testing.T) {
	store := newTestEntityStore(t)

	mentions := ResolveEntityMentions(store, "ghost-player", "", "[[Nobody]]")
	if len(mentions) != 0 {
		t.Errorf("expected no mentions for unknown entities, got %+v", mentions)
	}
}

func TestResolveSpeakerID(t *testing.T) {
	store := newTestEntityStore(t)
	saveExtractorEntity(t, store, &entity.Entity{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Hash: "h1"})
	saveExtractorEntity(t, store, &entity.Entity{ID: "lady-evelyn", Name: "Lady Evelyn Vance", Type: "character", Hash: "h2"})

	cases := map[string]string{
		"Garrick the Fence":             "garrick-the-fence",
		"[[Garrick the Fence|Garrick]]": "garrick-the-fence",
		"Evelyn":                        "lady-evelyn",
		"As you declare":                "",
		"":                              "",
	}
	for name, want := range cases {
		if got := ResolveSpeakerID(store, name); got != want {
			t.Errorf("ResolveSpeakerID(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestExtractorReturnsRecordsWithoutPersisting(t *testing.T) {
	store := newTestEntityStore(t)

	model := &mockProvider{
		id: "extractor-model",
		output: `[
			{"id": "garrick-the-fence", "name": "Garrick the Fence", "type": "character", "location": "[[alden-tavern]]", "body": "A shadowy broker."}
		]`,
	}

	records, err := NewExtractor(model).Extract(context.Background(), "You meet Garrick.")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if len(records.Entities) != 1 || records.Entities[0].Name != "Garrick the Fence" {
		t.Fatalf("unexpected records: %+v", records.Entities)
	}
	if _, err := store.GetEntity("garrick-the-fence"); err == nil {
		t.Errorf("expected extraction to leave persistence to the caller")
	}
}

func TestExtractorParsesObjectAndArrayResponses(t *testing.T) {
	cases := []struct {
		name     string
		output   string
		wantEnts int
		wantDial int
	}{
		{
			name:     "object",
			output:   `{"entities":[{"id":"garrick","name":"Garrick","type":"character","body":"A broker."}],"dialogue":[{"speaker":"Garrick","text":"Keep walking."}]}`,
			wantEnts: 1,
			wantDial: 1,
		},
		{
			name:     "bare array from older prompts",
			output:   `[{"id":"garrick","name":"Garrick","type":"character","body":"A broker."}]`,
			wantEnts: 1,
			wantDial: 0,
		},
		{
			name:     "empty object",
			output:   `{"entities":[],"dialogue":[]}`,
			wantEnts: 0,
			wantDial: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewExtractor(&mockProvider{id: "extractor-model", output: tc.output}).Extract(context.Background(), "narrative")
			if err != nil {
				t.Fatalf("Extract failed: %v", err)
			}
			if len(result.Entities) != tc.wantEnts || len(result.Dialogue) != tc.wantDial {
				t.Errorf("got %d entities and %d dialogue lines, want %d and %d",
					len(result.Entities), len(result.Dialogue), tc.wantEnts, tc.wantDial)
			}
		})
	}
}

func TestExtractorReturnsAProposedPlayerLocation(t *testing.T) {
	model := &mockProvider{
		id: "extractor-model",
		output: `{
			"entities": [],
			"dialogue": [],
			"player_location": "[[Alden Harbour]]"
		}`,
	}

	result, err := NewExtractor(model).Extract(context.Background(), "You walk down to the harbour.")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if result.PlayerLocation != "[[Alden Harbour]]" {
		t.Errorf("PlayerLocation = %q, want the proposed reference", result.PlayerLocation)
	}

	// The bare-array shape stays valid and carries no proposal.
	legacy := &mockProvider{id: "extractor-model", output: `[]`}
	result, err = NewExtractor(legacy).Extract(context.Background(), "Nothing happens.")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if result.PlayerLocation != "" {
		t.Errorf("PlayerLocation = %q, want empty for the legacy shape", result.PlayerLocation)
	}
}

func TestMergeOnlyFillsAnEmptyAppearance(t *testing.T) {
	existing := &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Appearance: "warm and lamp-lit"}
	raw := &ExtractedEntity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Appearance: "gutted by fire"}

	merged := MergeExtractedEntity(existing, raw)
	if merged.Appearance != "warm and lamp-lit" {
		t.Errorf("Appearance = %q, want the authored value kept", merged.Appearance)
	}

	empty := &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location"}
	merged = MergeExtractedEntity(empty, raw)
	if merged.Appearance != "gutted by fire" {
		t.Errorf("Appearance = %q, want the extracted value applied to an empty field", merged.Appearance)
	}
}

func TestExtractorReadsAnEntityAppearance(t *testing.T) {
	model := &mockProvider{
		id:     "extractor-model",
		output: `{"entities":[{"id":"alden-tavern","name":"Alden Tavern","type":"location","appearance":"roof collapsed","body":"A ruin."}]}`,
	}

	result, err := NewExtractor(model).Extract(context.Background(), "The tavern is a burnt shell.")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if len(result.Entities) != 1 || result.Entities[0].Appearance != "roof collapsed" {
		t.Errorf("expected an extracted appearance, got %+v", result.Entities)
	}
}

func TestResolveProseMentionsFindsNamesInProse(t *testing.T) {
	store := newTestEntityStore(t)
	if err := store.SaveEntity(&entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden."}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "sera-vane", Name: "Sera Vane", Type: "character", Body: "A smuggler."}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt."}); err != nil {
		t.Fatal(err)
	}

	mentions := ResolveProseMentions(store, "Kael waits by the water.", "I ask after Sera Vane.")

	byID := make(map[string]string, len(mentions))
	for _, mention := range mentions {
		byID[mention.ID] = mention.Kind
	}
	if byID["guard-kael"] != entity.MentionProse {
		t.Errorf("expected Kael's bare surname to resolve, got %+v", mentions)
	}
	if byID["sera-vane"] != entity.MentionProse {
		t.Errorf("expected Sera Vane's full name to resolve, got %+v", mentions)
	}
	if _, present := byID["aldon-harbour"]; present {
		t.Errorf("a location named in passing should not be a prose mention")
	}
}

func TestResolveProseMentionsIgnoresDescriptionsAndPartialWords(t *testing.T) {
	store := newTestEntityStore(t)
	if err := store.SaveEntity(&entity.Entity{ID: "the-woman", Name: "The Woman", Type: "character", Body: "Unnamed."}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "kaeldrin", Name: "Kaeldrin", Type: "character", Body: "Another."}); err != nil {
		t.Fatal(err)
	}

	// A description is not a name, and a name is not a fragment of a longer one.
	// Neither text names either entity: "The woman" is a description rather than the
	// written name, and "Kael" is a fragment of a longer name, not the name itself.
	if mentions := ResolveProseMentions(store, "The woman said nothing at all.", "Kael watched the water."); len(mentions) != 0 {
		t.Errorf("expected no mentions, got %+v", mentions)
	}
}

func TestResolveSpeakerIDAndMatchingKnowAliases(t *testing.T) {
	store := newTestEntityStore(t)
	if err := store.SaveEntity(&entity.Entity{
		ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden.",
		Aliases: []string{"The Ember Warden"},
	}); err != nil {
		t.Fatal(err)
	}

	if got := ResolveSpeakerID(store, "The Ember Warden"); got != "guard-kael" {
		t.Errorf("ResolveSpeakerID(alias) = %q, want guard-kael", got)
	}

	matched := MatchExistingEntity(store, &ExtractedEntity{Name: "The Ember Warden", Type: "character"})
	if matched == nil || matched.ID != "guard-kael" {
		t.Errorf("expected the alias to match the existing entity, got %+v", matched)
	}

	// An alias does not invent an entity: an unknown name still matches nothing.
	if MatchExistingEntity(store, &ExtractedEntity{Name: "Someone Else", Type: "character"}) != nil {
		t.Errorf("expected an unrelated name not to match")
	}
}
