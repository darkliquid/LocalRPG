package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

// seedStreamEntity adds an entity to the store the orchestrator's roster reads.
func seedStreamEntity(t *testing.T, timeline *Timeline, store *storage.Store, id, name, typ string) {
	t.Helper()
	writeTestEntityNote(t, timeline.EntitiesDir(), &entity.Entity{ID: id, Name: name, Type: typ, Body: "Present."})
	if _, err := storage.NewSyncer(store).Sync(timeline.EntitiesDir()); err != nil {
		t.Fatal(err)
	}
}

func TestProcessActionStreamEmitsSegmentsAsTheyArrive(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"The docks are quiet.\n\n> Kaelen: You didn't see me here.\n"}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)
	seedStreamEntity(t, timeline, store, "kaelen", "Kaelen", "character")

	var kinds []string
	orchestrator.SetSegmentObserver(func(event turnstream.Event) {
		kinds = append(kinds, event.Kind)
	})
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait.", func(string) error { return nil }); err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(kinds) < 2 || kinds[0] != turnstream.KindNarration || kinds[1] != turnstream.KindSpeech {
		t.Fatalf("segment observer saw %#v", kinds)
	}
}

func TestFinalisePrefersParsedSegments(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"The docks are quiet.\n\n> Kaelen: You didn't see me here.\n"}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)
	seedStreamEntity(t, timeline, store, "kaelen", "Kaelen", "character")

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(turn.Segments) != 2 || turn.Segments[1].Kind != entity.SegmentSpeech || turn.Segments[1].SpeakerID != "kaelen" {
		t.Fatalf("segments = %#v", turn.Segments)
	}
}

func TestFinaliseExtractsLegacyQuotedSpeech(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{`Kaelen: "Keep walking."`}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)
	seedStreamEntity(t, timeline, store, "kaelen", "Kaelen", "character")

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(turn.Segments) != 1 || turn.Segments[0].Kind != entity.SegmentSpeech || turn.Segments[0].SpeakerID != "kaelen" {
		t.Fatalf("segments = %#v", turn.Segments)
	}
}

func TestRecordLinesAreStrippedFromNarration(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"@persona {\"name\":\"Kae\",\"type\":\"character\"}\nThe gate stands open.\n"}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open." {
		t.Fatalf("narration = %q", turn.Narration)
	}
}
