package oracle

import (
	"testing"
)

func TestParsePrompt(t *testing.T) {
	prompt := "[MECHANICS RESULT: strong]\nPlayer Action: pick the lock\n" +
		"Stakes: the alarm sounds\nLocation: Saltmarch\nNear [[Garrick]] and [[Kaelen]].\n" +
		"Player stats: Edge 3, Grit 1.\n"
	p := parsePrompt(prompt)
	if p.Tier != "strong" || p.Action != "pick the lock" || p.Location != "Saltmarch" {
		t.Fatalf("parts = %+v", p)
	}
	if len(p.Entities) != 2 || p.Entities[0] != "Garrick" {
		t.Fatalf("entities = %v", p.Entities)
	}
	if len(p.Stats) != 2 || p.Stats[0].Name != "Edge" || p.Stats[0].Value != 3 {
		t.Fatalf("stats = %+v", p.Stats)
	}
}

func TestParsePromptAdditionalFormats(t *testing.T) {
	// Mode=... Tier=Success format
	prompt1 := "[MECHANICS RESULT: Mode=attack Roll=11 Tier=Success]\n" +
		"**Current Location:** [[The Sunken Outpost]]\n" +
		"Near [[Elena]].\n" +
		"## PLAYER ACTION\nSean: strike the beast\n"
	p1 := parsePrompt(prompt1)
	if p1.Tier != "Success" {
		t.Errorf("expected Tier=Success, got %q", p1.Tier)
	}
	if p1.Location != "The Sunken Outpost" {
		t.Errorf("expected Location=The Sunken Outpost, got %q", p1.Location)
	}
	if p1.Action != "strike the beast" {
		t.Errorf("expected Action=strike the beast, got %q", p1.Action)
	}
	if len(p1.Entities) != 1 || p1.Entities[0] != "Elena" {
		t.Errorf("expected Entities=[Elena], got %v", p1.Entities)
	}

	// ROLL RESULT format with stakes
	prompt2 := "[ROLL RESULT: miss, the bridge collapses]\n"
	p2 := parsePrompt(prompt2)
	if p2.Tier != "miss" {
		t.Errorf("expected Tier=miss, got %q", p2.Tier)
	}
	if p2.Stakes != "the bridge collapses" {
		t.Errorf("expected Stakes='the bridge collapses', got %q", p2.Stakes)
	}
}
