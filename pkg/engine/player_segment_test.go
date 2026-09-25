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

func TestPlayerSegment_TrimsQuotes(t *testing.T) {
	got := playerSegment("say", `"Hey, err... Trixie? Think I could grab a ride?"`, "stretch-layabout", "Stretch Layabout")
	if got == nil {
		t.Fatal("expected player segment")
	}
	want := "Hey, err... Trixie? Think I could grab a ride?"
	if got.Text != want {
		t.Errorf("got.Text = %q, want %q", got.Text, want)
	}
}

func TestAttachPlayerSegment_DeduplicatesWithTypoOrFormattingDifferences(t *testing.T) {
	existing := []entity.TurnSegment{
		{
			Kind:      entity.SegmentSpeech,
			Speaker:   "Stretch Layabout",
			SpeakerID: "stretch-layabout",
			Text:      "Hey Trixie, that was one hell of a race. No hard feelings, ey? Say, when we cash this in and split it, you fancy hanging out?",
		},
		{
			Kind: entity.SegmentNarration,
			Text: "Trixie smiles.",
		},
	}

	input := `"Hey Trixie, that was one hell of a race. No hard feelings, ey? Say, when we cash this in a split it, you fancy hanging out?"`
	result := attachPlayerSegment(existing, "say", input, "stretch-layabout", "Stretch Layabout")
	if len(result) != 2 {
		t.Fatalf("expected 2 segments (no duplicate prepended despite minor typo fix), got %d: %+v", len(result), result)
	}
	if !result[0].Player {
		t.Errorf("expected result[0].Player to be true")
	}
}

func TestAttachPlayerSegment_DeduplicatesAfterOpeningNarration(t *testing.T) {
	existing := []entity.TurnSegment{
		{
			Kind: entity.SegmentNarration,
			Text: "You clear your throat across the cockpit.",
		},
		{
			Kind:      entity.SegmentSpeech,
			Speaker:   "Stretch Layabout",
			SpeakerID: "stretch-layabout",
			Text:      "Hey, err... Trixie? Think I could grab a ride?",
		},
		{
			Kind:      entity.SegmentSpeech,
			Speaker:   "Trixie Nitro",
			SpeakerID: "trixie-nitro",
			Text:      "No way!",
		},
	}

	input := `"Hey, err... Trixie? Think I could grab a ride?"`
	result := attachPlayerSegment(existing, "say", input, "stretch-layabout", "Stretch Layabout")
	if len(result) != 3 {
		t.Fatalf("expected 3 segments (no duplicate prepended when player speaks as segment 1), got %d: %+v", len(result), result)
	}
	if !result[1].Player {
		t.Errorf("expected result[1].Player to be true")
	}
}

