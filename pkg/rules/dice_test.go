package rules

import (
	"testing"
)

func TestEvaluateRoll(t *testing.T) {
	// Standard polyhedral roll
	res, err := EvaluateRoll("1d20+5")
	if err != nil {
		t.Fatalf("EvaluateRoll failed: %v", err)
	}
	if res.Total < 6 || res.Total > 25 {
		t.Errorf("expected total in [6, 25], got %d", res.Total)
	}
	if res.Notation != "1d20+5" {
		t.Errorf("expected notation 1d20+5, got %s", res.Notation)
	}

	// Exploding dice
	resExploding, err := EvaluateRoll("2d6!")
	if err != nil {
		t.Fatalf("EvaluateRoll with exploding dice failed: %v", err)
	}
	if resExploding.Total < 2 {
		t.Errorf("unexpected total: %d", resExploding.Total)
	}

	// Invalid notation
	_, err = EvaluateRoll("invalid_notation_xyz")
	if err == nil {
		t.Errorf("expected error on invalid dice notation")
	}
}
