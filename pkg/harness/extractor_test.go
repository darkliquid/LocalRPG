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
