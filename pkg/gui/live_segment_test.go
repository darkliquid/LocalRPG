package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/turnstream"
)

func TestLiveSegmentDTOMapsSpeechAndNarration(t *testing.T) {
	speech, ok := liveSegmentDTO(turnstream.Event{
		Kind: turnstream.KindSpeech, Speaker: "Kaelen", SpeakerID: "kaelen", Text: "Keep walking.",
	})
	if !ok || speech.Kind != "speech" || speech.SpeakerID != "kaelen" {
		t.Fatalf("speech = %#v, %v", speech, ok)
	}

	narration, ok := liveSegmentDTO(turnstream.Event{Kind: turnstream.KindNarration, Text: "The hall is quiet."})
	if !ok || narration.Kind != "narration" {
		t.Fatalf("narration = %#v, %v", narration, ok)
	}

	if _, ok := liveSegmentDTO(turnstream.Event{Kind: turnstream.KindRecord}); ok {
		t.Fatal("a record must not render as a segment")
	}
	if _, ok := liveSegmentDTO(turnstream.Event{Kind: turnstream.KindNarration, Text: "   "}); ok {
		t.Fatal("an empty narration must not render")
	}
}
