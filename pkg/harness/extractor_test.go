package harness

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/storage"
)

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

	extractor := NewEntityExtractor(mockModel, store)
	count, err := extractor.ExtractFromTurn(context.Background(), "You meet Garrick in the corner of the tavern.")
	if err != nil {
		t.Fatalf("ExtractFromTurn failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 entity extracted, got %d", count)
	}

	// Verify Garrick is in storage
	garrick, err := store.GetEntity("garrick-the-fence")
	if err != nil || garrick.Name != "Garrick the Fence" {
		t.Errorf("expected Garrick in store, got %+v", garrick)
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

	extractor := NewEntityExtractor(mockModel, store)
	extractor.SetVoiceProfiles(profiles)

	count, err := extractor.ExtractFromTurn(context.Background(), "You meet an elder veteran named [[Old Garrow]].")
	if err != nil {
		t.Fatalf("ExtractFromTurn failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 entity extracted, got %d", count)
	}

	garrow, err := store.GetEntity("old-garrow")
	if err != nil {
		t.Fatalf("failed to get old-garrow: %v", err)
	}
	if garrow.Voice == nil || garrow.Voice.VoiceID != "bm_george" {
		t.Errorf("expected auto-assigned voice profile bm_george, got %+v", garrow.Voice)
	}
}

func TestExtractEntitiesWithProfiles_TagMatching(t *testing.T) {
	profiles := []config.VoiceProfile{
		{ID: "elder_sage", VoiceID: "bm_george", Pitch: 0.85, SpeechRate: 0.90, Tags: []string{"elder", "veteran"}},
	}

	prose := "You meet an elder veteran named [[Old Garrow]] resting by the hearth."
	entities := ExtractEntitiesWithProfiles(prose, profiles)

	if len(entities) == 0 {
		t.Fatalf("expected extracted entity")
	}
	if entities[0].Voice == nil || entities[0].Voice.VoiceID != "bm_george" {
		t.Errorf("expected auto-assigned voice profile bm_george, got %+v", entities[0].Voice)
	}
}
