package oracle

import (
	"testing"
)

const realAssembledPromptFixture = `[MECHANICS RESULT: Mode=attack Roll=11 Tier=Success]

## SYSTEM RULES & RESOLUTION MECHANICS
Resolution: 10+ Success, 7-9 Mixed, 6- Failure.
Player stats: Edge 3, Grit 1.

## IMMEDIATE SCENE
**Current Location:** Alden Tavern
A warm tavern smelling of ale.

**Player Character:** Sean (the protagonist, played by the user)
Edge 3, Grit 1.

## PRESENT CHARACTERS & NOTABLE BEINGS
Lady Evelyn: Guarded former lieutenant. [[Lady Evelyn]]

## PLAYER ACTION
Sean: I speak with Evelyn
`

func TestOracleParsesARealAssembledPrompt(t *testing.T) {
	p := parsePrompt(realAssembledPromptFixture)
	if p.Tier == "" || p.Action == "" {
		t.Fatalf("parts = %+v", p)
	}
	if p.Tier != "Success" {
		t.Errorf("expected Tier=Success, got %q", p.Tier)
	}
	if p.Action != "I speak with Evelyn" {
		t.Errorf("expected Action='I speak with Evelyn', got %q", p.Action)
	}
	if p.Location != "Alden Tavern" {
		t.Errorf("expected Location='Alden Tavern', got %q", p.Location)
	}
	if len(p.Entities) != 1 || p.Entities[0] != "Lady Evelyn" {
		t.Errorf("expected Entities=[Lady Evelyn], got %v", p.Entities)
	}
	if len(p.Stats) != 2 || p.Stats[0].Name != "Edge" || p.Stats[0].Value != 3 {
		t.Errorf("expected Stats=[Edge 3, Grit 1], got %+v", p.Stats)
	}

	prose := craftProse(realAssembledPromptFixture)
	if prose == "" {
		t.Fatal("expected non-empty crafted prose")
	}
}
