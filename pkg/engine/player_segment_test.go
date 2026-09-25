package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestPlayerSegmentForSay(t *testing.T) {
	got := playerSegment("say", "  I draw my blade. ", "sean", "Sean")
	if got == nil {
		t.Fatal("expected a player beat for a say action")
	}
	if !got.Player || got.SpeakerID != "sean" || got.Kind != entity.SegmentSpeech {
		t.Fatalf("unexpected segment: %+v", got)
	}
	if got.Text != "I draw my blade." {
		t.Errorf("Text = %q, want the trimmed input", got.Text)
	}
}

func TestPlayerSegmentIgnoredForNonSpeechModes(t *testing.T) {
	for _, mode := range []string{"do", "story", "roll", "opening", ""} {
		if got := playerSegment(mode, "text", "sean", "Sean"); got != nil {
			t.Errorf("mode %q produced a player beat: %+v", mode, got)
		}
	}
}

func TestPlayerSegmentIgnoredForEmptyInput(t *testing.T) {
	if got := playerSegment("say", "   ", "sean", "Sean"); got != nil {
		t.Errorf("expected no beat for empty input, got %+v", got)
	}
}

func TestAttachPlayerSegment_DeduplicatesExistingEcho(t *testing.T) {
	existing := []entity.TurnSegment{
		{
			Kind:      entity.SegmentSpeech,
			Speaker:   "Sean",
			SpeakerID: "sean",
			Text:      "I draw my blade.",
			Player:    false,
		},
		{
			Kind:      entity.SegmentNarration,
			Text:      "The shadows lengthen.",
		},
	}

	result := attachPlayerSegment(existing, "say", "I draw my blade.", "sean", "Sean")
	if len(result) != 2 {
		t.Fatalf("expected 2 segments (no duplicate prepended), got %d", len(result))
	}
	if !result[0].Player {
		t.Errorf("expected result[0].Player to be true")
	}
	if result[0].Text != "I draw my blade." {
		t.Errorf("expected result[0].Text to be %q, got %q", "I draw my blade.", result[0].Text)
	}
}

func TestAttachPlayerSegment_PrependsWhenNoEcho(t *testing.T) {
	existing := []entity.TurnSegment{
		{
			Kind:    entity.SegmentSpeech,
			Speaker: "Goblin",
			Text:    "Who goes there?",
		},
	}

	result := attachPlayerSegment(existing, "say", "I draw my blade.", "sean", "Sean")
	if len(result) != 2 {
		t.Fatalf("expected 2 segments (player prepended), got %d", len(result))
	}
	if !result[0].Player || result[0].SpeakerID != "sean" {
		t.Errorf("expected result[0] to be player segment, got %+v", result[0])
	}
	if result[1].Speaker != "Goblin" {
		t.Errorf("expected result[1] to be goblin segment, got %+v", result[1])
	}
}
