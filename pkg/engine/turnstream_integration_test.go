package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
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

func TestProcessActionStreamEmitsPlayerSegmentInSayMode(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"The docks are quiet.\n\n> Kaelen: You didn't see me here.\n"}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)
	seedStreamEntity(t, timeline, store, "kaelen", "Kaelen", "character")

	var events []turnstream.Event
	orchestrator.SetSegmentObserver(func(event turnstream.Event) {
		events = append(events, event)
	})
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Say", "Hold the line.", func(string) error { return nil }); err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events (player, narration, speech), got %d: %#v", len(events), events)
	}
	if !events[0].Player || events[0].Kind != turnstream.KindSpeech || events[0].Text != "Hold the line." {
		t.Fatalf("first event = %#v, want player speech beat", events[0])
	}
	if events[1].Kind != turnstream.KindNarration {
		t.Fatalf("second event = %#v, want narration", events[1])
	}
	if events[2].Kind != turnstream.KindSpeech || events[2].SpeakerID != "kaelen" {
		t.Fatalf("third event = %#v, want kaelen speech", events[2])
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

func TestRecordsIntroducePersonaeAndAttributeTheirFirstLine(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{
		"@persona {\"name\":\"Kae\",\"type\":\"character\",\"new\":true}\n> Kae: Well met.\n",
	}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I arrive.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(turn.Personae) != 1 || turn.Personae[0] != "kae" {
		t.Fatalf("personae = %#v", turn.Personae)
	}
	if len(turn.Segments) != 1 || turn.Segments[0].Kind != entity.SegmentSpeech || turn.Segments[0].SpeakerID != "kae" {
		t.Fatalf("segments = %#v", turn.Segments)
	}
}

func TestRollRecordResolvesAndContinuesTheTurn(t *testing.T) {
	provider := &scriptedStreamProvider{chunksPerCall: [][]string{
		{"Kaelen steps onto the bridge.\n@roll {\"actor\":\"kaelen\",\"check_kind\":\"skill\",\"stakes\":\"the bridge\",\"outcomes\":{\"pass\":\"He makes it across.\",\"fail\":\"The plank gives way.\"}}\n"},
		{"Kaelen reaches the far side.\n"},
	}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I follow.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(turn.Checks) != 1 {
		t.Fatalf("checks = %#v, want one resolved roll", turn.Checks)
	}
	if !strings.Contains(turn.Narration, "far side") {
		t.Fatalf("the continuation must be recorded: %q", turn.Narration)
	}
	if strings.Contains(turn.Narration, "@roll") {
		t.Fatalf("the roll record must not survive into the prose: %q", turn.Narration)
	}
	if len(turn.Segments) < 2 || turn.Segments[1].CheckRef != turn.Checks[0].CheckID {
		t.Fatalf("the check must attach to the continuation's segment: segments=%#v", turn.Segments)
	}
}

func TestRollRecordUnderAskPolicyBecomesAPendingCheck(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{
		"Kaelen steps onto the bridge.\n@roll {\"actor\":\"kaelen\",\"check_kind\":\"skill\",\"stakes\":\"the bridge\"}\n",
	}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetMechanicsEngagement("ask")

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I follow.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.PendingCheck == nil {
		t.Fatalf("expected a pending check, got %+v", turn)
	}
}

func TestAPendingCheckTurnKeepsItsProse(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{
		"Kaelen steps onto the bridge, the planks swaying.\n" +
			"@roll {\"actor\":\"kaelen\",\"check_kind\":\"skill\",\"stakes\":\"the bridge\"}\n",
	}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetMechanicsEngagement("ask")

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I follow.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.PendingCheck == nil {
		t.Fatalf("expected a pending check, got %+v", turn)
	}
	if !strings.Contains(turn.Narration, "planks swaying") {
		t.Fatalf("narration = %q, want the prose before the check", turn.Narration)
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

func TestFinaliseAttributesUnseededSpeakerWithBlockquote(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"The docks are quiet.\n\n> Kaelen: You didn't see me here.\n"}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	// Do NOT seed kaelen in store! Test that dynamic resolution attributes Kaelen.

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(turn.Segments) != 2 || turn.Segments[1].Kind != entity.SegmentSpeech || turn.Segments[1].SpeakerID != "kaelen" {
		t.Fatalf("segments = %#v, want unseeded kaelen attributed as speech", turn.Segments)
	}
}

func TestFinaliseRescuesTrappedDialogueFromNarration(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"Kaelen pauses. \"We cannot stay here.\" He looks back.\n"}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)

	extractorModel := &mockTimelineModel{
		response: `{"entities":[],"dialogue":[{"speaker":"Kaelen","text":"We cannot stay here."}]}`,
	}
	orchestrator.SetExtractor(harness.NewExtractor(extractorModel))

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}

	foundSpeech := false
	for _, seg := range turn.Segments {
		if seg.Kind == entity.SegmentSpeech && seg.SpeakerID == "kaelen" && seg.Text == "We cannot stay here." {
			foundSpeech = true
			break
		}
	}
	if !foundSpeech {
		t.Fatalf("expected extracted dialogue to be rescued as speech segment, got segments: %#v", turn.Segments)
	}
}

func TestTurnStreamRevealsRemapsEarlierSegments(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{
		"> Unknown Voice: \"Who goes there?\"\n\n" +
			"A figure steps from the shadow.\n\n" +
			"@persona {\"name\":\"Doctor Cain\",\"reveals\":\"Unknown Voice\",\"type\":\"character\"}\n" +
			"> Doctor Cain: \"I am Cain.\"\n",
	}}
	orchestrator, _, store := streamingOrchestrator(t, provider)

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I listen.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}

	// Earlier segment should be remapped to doctor-cain
	if len(turn.Segments) < 3 {
		t.Fatalf("expected at least 3 segments, got %#v", turn.Segments)
	}
	if turn.Segments[0].Kind != entity.SegmentSpeech || turn.Segments[0].SpeakerID != "doctor-cain" {
		t.Errorf("first speech segment speakerID = %q, want doctor-cain", turn.Segments[0].SpeakerID)
	}
	if turn.Segments[2].Kind != entity.SegmentSpeech || turn.Segments[2].SpeakerID != "doctor-cain" {
		t.Errorf("second speech segment speakerID = %q, want doctor-cain", turn.Segments[2].SpeakerID)
	}

	// Check store has doctor-cain with Unknown Voice alias
	ent, err := store.GetEntity("doctor-cain")
	if err != nil || ent == nil {
		t.Fatalf("expected doctor-cain in store, err: %v", err)
	}
	if len(ent.Aliases) == 0 || ent.Aliases[0] != "Unknown Voice" {
		t.Errorf("doctor-cain aliases = %v, want Unknown Voice", ent.Aliases)
	}
}


