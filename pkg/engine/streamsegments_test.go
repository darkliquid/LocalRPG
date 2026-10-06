package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/trace"
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

// streamTestRoster is a minimal roster for parser tests.
type streamTestRoster map[string]string

func (m streamTestRoster) Resolve(name string) (string, bool) { id, ok := m[name]; return id, ok }
func (m streamTestRoster) Declare(name, id string)            { m[name] = id }

func TestRepairedRollEndsTurn(t *testing.T) {
	o := &TurnOrchestrator{parser: turnstream.NewParser(streamTestRoster{})}
	o.parser.Feed("@roll {\"actor\":\"x\",\"check_kind\":\"do\",}\n")
	req, ok := o.pendingRoll()
	if !ok {
		t.Fatal("a repaired @roll should be pending")
	}
	if req.Actor != "x" {
		t.Fatalf("actor = %q, want x", req.Actor)
	}
}

// recordingLogger captures event names so a test can assert one was emitted.
type recordingLogger struct{ events []string }

func (l *recordingLogger) Enabled(trace.Level) bool { return true }
func (l *recordingLogger) Event(name string, _ map[string]interface{}) {
	l.events = append(l.events, name)
}
func (l *recordingLogger) SetGame(string) {}

func (l *recordingLogger) sawEvent(name string) bool {
	for _, event := range l.events {
		if event == name {
			return true
		}
	}
	return false
}

func TestTurnEmitsRepairTrace(t *testing.T) {
	logger := &recordingLogger{}
	o := &TurnOrchestrator{parser: turnstream.NewParser(streamTestRoster{}), logger: logger}
	o.parser.Feed("@roll {\"actor\":\"x\",\"check_kind\":\"do\",}\n")
	o.logRepairReport()
	if !logger.sawEvent("turn.records_repaired") {
		t.Fatal("expected a turn.records_repaired trace event")
	}
}
