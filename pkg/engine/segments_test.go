package engine

import (
	"reflect"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestBuildTurnSegmentsResolvesSpeakers(t *testing.T) {
	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Hash: "h2"})

	narration := "The docks are quiet.\nGarrick the Fence: \"You didn't see me here.\"\nAs you declare: \"I draw my blade.\""
	segments := buildTurnSegments(store, narration, harness.Extraction{})

	want := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The docks are quiet."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick the Fence", SpeakerID: "garrick-the-fence", Text: "You didn't see me here."},
		{Kind: entity.SegmentNarration, Text: `As you declare: "I draw my blade."`},
	}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("buildTurnSegments() = %#v, want %#v", segments, want)
	}
}

func TestBuildTurnSegmentsAttributesEntitiesIntroducedThisTurn(t *testing.T) {
	// The extractor proposes the character; nothing is indexed yet. A wikilinked
	// speaker must still resolve so their first line is spoken, not narrated.
	store := newTestStore(t)
	narration := "A warden stands vigil.\n[[Guard Kael]]: \"The mist thickens.\""

	extraction := harness.Extraction{
		Entities: []harness.ExtractedEntity{{ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden."}},
	}
	segments := buildTurnSegments(store, narration, extraction)

	if len(segments) != 2 {
		t.Fatalf("buildTurnSegments() = %#v, want narration then speech", segments)
	}
	if segments[1].Kind != entity.SegmentSpeech || segments[1].SpeakerID != "guard-kael" {
		t.Errorf("second segment = %+v, want speech by guard-kael", segments[1])
	}
}

func TestBuildTurnSegmentsAppliesExtractorAttribution(t *testing.T) {
	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "lady-evelyn", Name: "Lady Evelyn Vance", Type: "character", Hash: "h1"})

	narration := `Vance looks up. She says "You made it back in one piece."`
	attributions := []harness.ExtractedDialogue{
		{Speaker: "Lady Evelyn Vance", Text: "You made it back in one piece."},
	}

	segments := buildTurnSegments(store, narration, harness.Extraction{Dialogue: attributions})

	want := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "Vance looks up. She says"},
		{Kind: entity.SegmentSpeech, Speaker: "Lady Evelyn Vance", SpeakerID: "lady-evelyn", Text: "You made it back in one piece."},
	}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("buildTurnSegments() = %#v, want %#v", segments, want)
	}
}

func TestSpeechMentionsDeduplicatesSpeakers(t *testing.T) {
	segments := []entity.TurnSegment{
		{Kind: entity.SegmentSpeech, SpeakerID: "garrick"},
		{Kind: entity.SegmentNarration, Text: "silence"},
		{Kind: entity.SegmentSpeech, SpeakerID: "garrick"},
		{Kind: entity.SegmentSpeech, SpeakerID: ""},
	}

	mentions := speechMentions(segments)
	if len(mentions) != 1 || mentions[0].ID != "garrick" || mentions[0].Kind != entity.MentionSpeech {
		t.Errorf("speechMentions() = %+v, want one garrick speech mention", mentions)
	}
}
