package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestReadChronicleOfACampaignThatHasNone(t *testing.T) {
	store := newTestStore(t)

	chronicle, err := ReadChronicle(store)
	if err != nil {
		t.Fatalf("ReadChronicle failed: %v", err)
	}
	if chronicle.ThroughTurn != 0 || chronicle.Summary != "" {
		t.Errorf("expected an empty chronicle, got %+v", chronicle)
	}
}

func TestWriteThenReadChronicle(t *testing.T) {
	store := newTestStore(t)
	entitiesDir := t.TempDir()

	if err := WriteChronicle(store, entitiesDir, Chronicle{
		Summary:     "The party reached the harbour and learnt the oil was low.",
		ThroughTurn: 12,
	}); err != nil {
		t.Fatalf("WriteChronicle failed: %v", err)
	}

	// The note must be a normal entity, so the codex and the graph can see it.
	path := filepath.Join(entitiesDir, ChronicleEntityID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected a chronicle note on disk: %v", err)
	}
	parsed, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("parse chronicle note: %v", err)
	}
	if parsed.Type != "chronicle" {
		t.Errorf("type = %q, want chronicle", parsed.Type)
	}

	chronicle, err := ReadChronicle(store)
	if err != nil {
		t.Fatal(err)
	}
	if chronicle.ThroughTurn != 12 {
		t.Errorf("ThroughTurn = %d, want 12", chronicle.ThroughTurn)
	}
	if !strings.Contains(chronicle.Summary, "oil was low") {
		t.Errorf("summary did not round-trip: %q", chronicle.Summary)
	}
}
