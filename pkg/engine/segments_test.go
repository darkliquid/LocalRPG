package engine

import (
	"reflect"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestBuildTurnSegmentsAttributesSayModePlayerAndResolvedSpeakers(t *testing.T) {
	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Hash: "h1"})
	saveTestEntity(t, store, &entity.Entity{ID: "garrick-the-fence", Name: "Garrick the Fence", Type: "character", Hash: "h2"})

	narration := "The docks are quiet.\nGarrick the Fence: \"You didn't see me here.\"\nAs you declare: \"I draw my blade.\""
	segments := buildTurnSegments(store, "Say", "player", "Where is the ledger?", narration, nil)

	want := []entity.TurnSegment{
		{Kind: entity.SegmentSpeech, Speaker: "Sean", SpeakerID: "player", Text: "Where is the ledger?"},
		{Kind: entity.SegmentNarration, Text: "The docks are quiet."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick the Fence", SpeakerID: "garrick-the-fence", Text: "You didn't see me here."},
		{Kind: entity.SegmentNarration, Text: `As you declare: "I draw my blade."`},
	}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("buildTurnSegments() = %#v, want %#v", segments, want)
	}
}

func TestBuildTurnSegmentsAppliesExtractorAttribution(t *testing.T) {
	store := newTestStore(t)
	saveTestEntity(t, store, &entity.Entity{ID: "lady-evelyn", Name: "Lady Evelyn Vance", Type: "character", Hash: "h1"})

	narration := `Vance looks up. She says "You made it back in one piece."`
	attributions := []harness.ExtractedDialogue{
		{Speaker: "Lady Evelyn Vance", Text: "You made it back in one piece."},
	}

	segments := buildTurnSegments(store, "Do", "player", "I enter", narration, attributions)

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
