package state

import (
	"reflect"
	"testing"
)

func TestStateGetSet(t *testing.T) {
	s := NewState(map[string]any{
		"hp": 100,
		"stats": map[string]any{
			"strength": 14,
			"agility":  12,
		},
		"tags": []any{"warrior", "veteran"},
	})

	// Direct get
	val, ok := s.Get("hp")
	if !ok || val != 100 {
		t.Errorf("expected hp=100, got %v (ok=%v)", val, ok)
	}

	// Nested dot-path get
	str, ok := s.Get("stats.strength")
	if !ok || str != 14 {
		t.Errorf("expected stats.strength=14, got %v (ok=%v)", str, ok)
	}

	// Set nested path
	if err := s.Set("stats.intelligence", 16); err != nil {
		t.Fatalf("failed to set stats.intelligence: %v", err)
	}
	intel, ok := s.Get("stats.intelligence")
	if !ok || intel != 16 {
		t.Errorf("expected stats.intelligence=16, got %v (ok=%v)", intel, ok)
	}

	// Set new root path
	if err := s.Set("mana", 50); err != nil {
		t.Fatalf("failed to set mana: %v", err)
	}
	mana, ok := s.Get("mana")
	if !ok || mana != 50 {
		t.Errorf("expected mana=50, got %v (ok=%v)", mana, ok)
	}

	// Snapshot
	raw := s.Raw()
	if !reflect.DeepEqual(raw["mana"], 50) {
		t.Errorf("expected raw state to reflect changes, got %+v", raw)
	}
}
