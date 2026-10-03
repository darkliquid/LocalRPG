package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

func TestSegmentsFromEventsKeepsOrderAndSpeakers(t *testing.T) {
	events := []turnstream.Event{
		{Kind: turnstream.KindNarration, Text: "The hall is quiet."},
		{Kind: turnstream.KindSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
		{Kind: turnstream.KindRecord, Record: &turnstream.Record{Type: turnstream.RecordRoll}},
	}
	got := segmentsFromEvents(events)

	want := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The hall is quiet."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
	}
	if len(got) != len(want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestStripRecordLinesKeepsProseAndSpeech(t *testing.T) {
	text := "The docks are quiet.\n@persona {\"name\":\"Kae\"}\n> Kaelen: Keep walking.\n\nA gull cries."
	got := stripRecordLines(text)
	if got != "The docks are quiet.\n> Kaelen: Keep walking.\n\nA gull cries." {
		t.Fatalf("stripRecordLines = %q", got)
	}
}
