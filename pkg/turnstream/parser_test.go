package turnstream

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/jsonrepair"
)

// mapRoster is a fixed roster for tests.
type mapRoster map[string]string

func (m mapRoster) Resolve(name string) (string, bool) {
	id, ok := m[name]
	return id, ok
}

func (m mapRoster) Declare(name, id string) { m[name] = id }

func TestFeedClassifiesLines(t *testing.T) {
	p := NewParser(mapRoster{"Kaelen": "kaelen"})
	events := p.Feed("The docks are quiet.\n\n> Kaelen: You didn't see me here.\n\nA gull cries.\n")
	events = append(events, p.Flush()...)

	want := []Event{
		{Kind: KindNarration, Text: "The docks are quiet."},
		{Kind: KindSpeech, Speaker: "Kaelen", SpeakerID: "kaelen", Text: "You didn't see me here."},
		{Kind: KindNarration, Text: "A gull cries."},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	for i := range want {
		if events[i].Kind != want[i].Kind || events[i].Text != want[i].Text ||
			events[i].SpeakerID != want[i].SpeakerID {
			t.Fatalf("event %d = %#v, want %#v", i, events[i], want[i])
		}
	}
}

func TestFeedIsChunkInvariant(t *testing.T) {
	input := "One.\n\n> Kaelen: Keep walking.\n\nTwo.\n"
	whole := NewParser(mapRoster{"Kaelen": "kaelen"})
	want := append(whole.Feed(input), whole.Flush()...)

	for split := 1; split < len(input); split++ {
		p := NewParser(mapRoster{"Kaelen": "kaelen"})
		got := append(p.Feed(input[:split]), p.Feed(input[split:])...)
		got = append(got, p.Flush()...)
		if len(got) != len(want) {
			t.Fatalf("split %d: %#v, want %#v", split, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("split %d event %d: %#v, want %#v", split, i, got[i], want[i])
			}
		}
	}
}

func TestUnresolvedSpeakerStaysNarration(t *testing.T) {
	p := NewParser(mapRoster{})
	events := append(p.Feed("> Nobody: Who said that?\n"), p.Flush()...)
	if len(events) != 1 || events[0].Kind != KindNarration || events[0].Text != "Nobody: Who said that?" {
		t.Fatalf("events = %#v", events)
	}
}

func TestNarrationParagraphsCoalesce(t *testing.T) {
	p := NewParser(mapRoster{})
	events := append(p.Feed("The docks are quiet.\nA gull cries overhead.\n\n"), p.Flush()...)
	if len(events) != 1 || events[0].Text != "The docks are quiet.\nA gull cries overhead." {
		t.Fatalf("events = %#v", events)
	}
}

func TestQuotedSpeechIsStripped(t *testing.T) {
	p := NewParser(mapRoster{"Kaelen": "kaelen"})
	events := append(p.Feed("> Kaelen: \"You didn't see me here.\"\n"), p.Flush()...)
	if len(events) != 1 || events[0].Text != "You didn't see me here." {
		t.Fatalf("events = %#v", events)
	}
}

func TestLegacyQuotedSpeechIsAttributed(t *testing.T) {
	p := NewParser(mapRoster{"Kaelen": "kaelen"})
	events := append(p.Feed(`Kaelen: "Keep walking."`+"\n"), p.Flush()...)
	if len(events) != 1 || events[0].Kind != KindSpeech || events[0].SpeakerID != "kaelen" {
		t.Fatalf("events = %#v", events)
	}
}

func TestLegacyUnquotedSpeechIsAttributed(t *testing.T) {
	p := NewParser(mapRoster{"Kaelen": "kaelen"})
	events := append(p.Feed("Kaelen: Keep walking.\n"), p.Flush()...)
	if len(events) != 1 || events[0].Kind != KindSpeech || events[0].SpeakerID != "kaelen" || events[0].Text != "Keep walking." {
		t.Fatalf("events = %#v", events)
	}
}

func TestLegacyQuoteForAnUnknownSpeakerStaysNarration(t *testing.T) {
	p := NewParser(mapRoster{})
	events := append(p.Feed(`As you declare: "I draw my blade."`+"\n"), p.Flush()...)
	if len(events) != 1 || events[0].Kind != KindNarration {
		t.Fatalf("events = %#v", events)
	}
}

func TestRecordsAreParsedAndPersonaeDeclared(t *testing.T) {
	roster := mapRoster{}
	p := NewParser(roster)
	events := p.Feed("@persona {\"name\":\"Kae\",\"type\":\"character\"}\n> Kae: Well met.\n")
	events = append(events, p.Flush()...)

	if len(events) != 2 || events[0].Kind != KindRecord || events[1].Kind != KindSpeech {
		t.Fatalf("events = %#v", events)
	}
	if events[1].SpeakerID != "kae" {
		t.Fatalf("a declared persona must be attributable: %#v", events[1])
	}
	records := p.Records()
	if len(records) != 1 || records[0].Type != RecordPersona || records[0].Err != nil {
		t.Fatalf("records = %#v", records)
	}
}

func TestRecordPersonaRevealsMapsPreviousIdentity(t *testing.T) {
	roster := mapRoster{}
	p := NewParser(roster)
	events := p.Feed("@persona {\"name\":\"Doctor Cain\",\"type\":\"character\",\"reveals\":\"Unknown Voice\"}\n> Unknown Voice: I am here.\n")
	events = append(events, p.Flush()...)

	if len(events) != 2 || events[0].Kind != KindRecord || events[1].Kind != KindSpeech {
		t.Fatalf("events = %#v", events)
	}
	if events[1].SpeakerID != "doctor-cain" {
		t.Fatalf("revealed persona must map previous identity to doctor-cain, got %#v", events[1])
	}
	if id, ok := roster.Resolve("Unknown Voice"); !ok || id != "doctor-cain" {
		t.Fatalf("roster must resolve previous identity to doctor-cain, got %q, %v", id, ok)
	}
}

func TestMalformedRecordIsKeptButDoesNotFailTheStream(t *testing.T) {
	p := NewParser(mapRoster{})
	events := p.Feed("@bogus {not json}\nStill narrated.\n")
	events = append(events, p.Flush()...)

	records := p.Records()
	if len(records) != 1 || records[0].Err == nil {
		t.Fatalf("a bad record must be reported, got %#v", records)
	}
	if len(events) != 1 || events[0].Kind != KindNarration || events[0].Text != "Still narrated." {
		t.Fatalf("prose must survive a bad record, got %#v", events)
	}
}

func TestRecordWithoutPayloadIsReported(t *testing.T) {
	p := NewParser(mapRoster{})
	p.Feed("@roll\n")
	records := p.Records()
	if len(records) != 1 || records[0].Err == nil {
		t.Fatalf("records = %#v", records)
	}
}

func TestParserRepairsMalformedRecord(t *testing.T) {
	p := NewParser(mapRoster{})
	p.Feed("@roll {\"actor\":\"x\",\"check_kind\":\"do\",}\n")
	recs := p.Records()
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	if recs[0].Err != nil {
		t.Fatalf("record errored: %v", recs[0].Err)
	}
	if recs[0].Repaired != jsonrepair.KindTrailingComma {
		t.Fatalf("Repaired = %q, want %q", recs[0].Repaired, jsonrepair.KindTrailingComma)
	}
	if req, err := recs[0].DecodeRoll(); err != nil || req.Actor != "x" {
		t.Fatalf("decode repaired roll: %+v %v", req, err)
	}
}

func TestParserKeepsUnrepairableRecord(t *testing.T) {
	p := NewParser(mapRoster{})
	p.Feed("@roll not json at all\n")
	recs := p.Records()
	if len(recs) != 1 || recs[0].Err == nil {
		t.Fatalf("expected one errored record, got %+v", recs)
	}
	if recs[0].Repaired != jsonrepair.KindNone {
		t.Fatalf("Repaired = %q, want empty", recs[0].Repaired)
	}
}

func TestParserAssemblesMultiLineRecord(t *testing.T) {
	p := NewParser(mapRoster{})
	p.Feed("@roll {\n")
	p.Feed("  \"actor\": \"x\",\n")
	p.Feed("  \"check_kind\": \"do\"\n")
	p.Feed("}\n")
	recs := p.Records()
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	if recs[0].Err != nil {
		t.Fatalf("record errored: %v", recs[0].Err)
	}
	if recs[0].Repaired != jsonrepair.KindNone {
		t.Fatalf("Repaired = %q, want empty", recs[0].Repaired)
	}
	if req, err := recs[0].DecodeRoll(); err != nil || req.Actor != "x" {
		t.Fatalf("decode multi-line roll: %+v %v", req, err)
	}
}

func TestParserFlushAssemblesUnterminatedRecord(t *testing.T) {
	p := NewParser(mapRoster{})
	p.Feed("@roll {\"actor\":\"x\"\n")
	p.Flush()
	recs := p.Records()
	if len(recs) != 1 || recs[0].Err != nil {
		t.Fatalf("expected one repaired record, got %+v", recs)
	}
	if recs[0].Repaired != jsonrepair.KindClose {
		t.Fatalf("Repaired = %q, want close", recs[0].Repaired)
	}
}

func TestParserFlushReportsUnrepairableRecord(t *testing.T) {
	p := NewParser(mapRoster{})
	p.Feed("@roll {not json\n")
	p.Flush()
	recs := p.Records()
	if len(recs) != 1 || recs[0].Err == nil {
		t.Fatalf("expected one errored record, got %+v", recs)
	}
}

func TestParserRepairReport(t *testing.T) {
	p := NewParser(mapRoster{})
	p.Feed("@roll {\"actor\":\"x\",}\n")
	p.Feed("@persona {\"name\":\"Vex\"}\n")
	p.Feed("@roll not json at all\n")
	r := p.RepairReport()
	if r.Total != 3 || r.Repaired != 1 || r.Failed != 1 {
		t.Fatalf("report = %+v, want total 3 repaired 1 failed 1", r)
	}
	if r.Kinds[jsonrepair.KindTrailingComma] != 1 {
		t.Fatalf("kinds = %+v, want one trailing_comma", r.Kinds)
	}
}
