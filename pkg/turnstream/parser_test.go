package turnstream

import "testing"

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
