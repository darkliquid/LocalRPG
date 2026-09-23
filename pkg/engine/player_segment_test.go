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
