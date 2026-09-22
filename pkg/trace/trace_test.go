package trace

import "testing"

func TestParseLevelAcceptsTheDocumentedNames(t *testing.T) {
	cases := map[string]Level{
		"":        LevelOff,
		"off":     LevelOff,
		"none":    LevelOff,
		"summary": LevelSummary,
		"full":    LevelFull,
		"debug":   LevelFull,
	}

	for input, want := range cases {
		got, err := ParseLevel(input)
		if err != nil {
			t.Errorf("ParseLevel(%q) failed: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", input, got, want)
		}
	}

	if _, err := ParseLevel("loud"); err == nil {
		t.Errorf("expected an unknown level to fail")
	}
}

func TestLevelsAreOrderedAndPrintable(t *testing.T) {
	if LevelOff >= LevelSummary || LevelSummary >= LevelFull {
		t.Fatalf("levels must be ordered off < summary < full")
	}
	for level, want := range map[Level]string{LevelOff: "off", LevelSummary: "summary", LevelFull: "full"} {
		if got := level.String(); got != want {
			t.Errorf("Level(%d).String() = %q, want %q", level, got, want)
		}
	}
}

func TestNopRecordsNothingAndNeverEnabled(t *testing.T) {
	if Nop().Enabled(LevelSummary) {
		t.Errorf("Nop must not be enabled at any level")
	}
}

func TestMemoryStampsTheCampaign(t *testing.T) {
	memory := NewMemory(LevelSummary)
	memory.SetGame("test-campaign")
	memory.Event("turn.begin", map[string]interface{}{"number": 1})

	event, ok := memory.Find("turn.begin")
	if !ok {
		t.Fatal("expected the event")
	}
	if event.Fields["game"] != "test-campaign" {
		t.Errorf("expected the campaign stamped on the event, got %+v", event.Fields)
	}
	if event.Fields["number"] != 1 {
		t.Errorf("stamping must not lose the caller's fields, got %+v", event.Fields)
	}
}

func TestMemoryOnlyRecordsEnabledEvents(t *testing.T) {
	memory := NewMemory(LevelSummary)

	if !memory.Enabled(LevelSummary) {
		t.Errorf("expected summary to be enabled at summary level")
	}
	if memory.Enabled(LevelFull) {
		t.Errorf("expected full to be disabled at summary level")
	}

	memory.Event("turn.begin", map[string]interface{}{"number": 1})
	memory.Event("context.assembled", map[string]interface{}{"tokens": 2610})

	if got := memory.Names(); len(got) != 2 || got[0] != "turn.begin" {
		t.Fatalf("Names() = %v, want the two events in order", got)
	}

	event, ok := memory.Find("context.assembled")
	if !ok {
		t.Fatalf("expected to find the assembled event")
	}
	if event.Fields["tokens"] != 2610 {
		t.Errorf("fields were not preserved: %+v", event.Fields)
	}
}
