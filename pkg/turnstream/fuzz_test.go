package turnstream

import (
	"reflect"
	"testing"
)

// FuzzParser asserts the parser's contract on arbitrary bytes: it never panics,
// every event it emits is well-formed, and parsing the same input twice yields
// the same events.
func FuzzParser(f *testing.F) {
	f.Add("A quiet hall.\n")
	f.Add("> Garrick: \"Keep walking.\"\n")
	f.Add("@roll {\"actor\":\"x\"\n}\n")
	f.Add("@roll {bad json\n")
	f.Add("")
	f.Add("\x00\xff\r\n@persona {\"name\":\"Evelyn\"}\n> Evelyn: hello\n")
	f.Fuzz(func(t *testing.T, data string) {
		// Each parser gets its own roster: the parser mutates it as persona
		// records arrive, so a shared roster would let the first run's
		// declarations change the second run's attribution.
		newRoster := func() mapRoster {
			return mapRoster{"Garrick": "garrick", "Evelyn": "evelyn"}
		}
		p := NewParser(newRoster())
		p.Feed(data)
		_ = p.Flush()
		events := p.Events()

		second := NewParser(newRoster())
		second.Feed(data)
		_ = second.Flush()
		if !reflect.DeepEqual(events, second.Events()) {
			t.Fatalf("parsing is not deterministic for %q", data)
		}

		for _, ev := range events {
			switch ev.Kind {
			case KindNarration, KindSpeech:
			case KindRecord:
				if ev.Record == nil {
					t.Fatal("a record event has no record")
				}
			default:
				t.Fatalf("unknown event kind %q", ev.Kind)
			}
			if ev.Kind == KindSpeech && ev.Speaker == "" && ev.SpeakerID == "" {
				t.Fatalf("a speech event has no speaker: %#v", ev)
			}
		}
	})
}
