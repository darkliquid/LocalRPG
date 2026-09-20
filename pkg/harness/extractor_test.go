package harness

import (
	"context"
	"path/filepath"
	"testing"

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
