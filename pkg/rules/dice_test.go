package rules

import (
	"fmt"
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

// A client draws a die from its face, so the roll has to carry the faces that
// landed rather than only their total: two dice totalling four could be 1+3 or
// 2+2, and neither is the die's size.
func TestEvaluateRollCarriesTheFacesThatLanded(t *testing.T) {
	res, err := EvaluateRoll("2d6")
	if err != nil {
		t.Fatalf("EvaluateRoll: %v", err)
	}
	if len(res.Dice) != 2 {
		t.Fatalf("dice = %#v, want one face per die", res.Dice)
	}
	if res.RollCount != len(res.Dice) {
		t.Errorf("roll count = %d, faces = %d", res.RollCount, len(res.Dice))
	}

	sum := 0
	for _, die := range res.Dice {
		if die.Value < 1 || die.Value > 6 {
			t.Errorf("face %d is not a d6", die.Value)
		}
		if die.Symbol != fmt.Sprintf("%d", die.Value) {
			t.Errorf("face %d shows %q, want the number itself", die.Value, die.Symbol)
		}
		sum += die.Value
	}
	if sum != res.Total {
		t.Errorf("faces %#v total %d, want the roll's total %d", res.Dice, sum, res.Total)
	}

	// A modifier changes the total, not the dice.
	modified, err := EvaluateRoll("2d6+3")
	if err != nil {
		t.Fatalf("EvaluateRoll: %v", err)
	}
	if len(modified.Dice) != 2 {
		t.Fatalf("dice = %#v, want the two dice", modified.Dice)
	}
	diceTotal := modified.Dice[0].Value + modified.Dice[1].Value
	if modified.Total != diceTotal+3 {
		t.Errorf("total = %d, want the faces %#v plus the modifier", modified.Total, modified.Dice)
	}
}

// A Fate die is not a number to show, so the face carries the symbol its own
// rules use for it.
func TestEvaluateRollKeepsAFateDiceSymbol(t *testing.T) {
	res, err := EvaluateRoll("3dF")
	if err != nil {
		t.Fatalf("EvaluateRoll: %v", err)
	}
	if len(res.Dice) != 3 {
		t.Fatalf("dice = %#v, want three faces", res.Dice)
	}
	for _, die := range res.Dice {
		if die.Symbol == "" || die.Symbol == fmt.Sprintf("%d", die.Value) {
			t.Errorf("face %+v has no symbol of its own", die)
		}
	}
}

func TestRollSummaryCarriesTheFaces(t *testing.T) {
	res, err := EvaluateRoll("2d6")
	if err != nil {
		t.Fatalf("EvaluateRoll: %v", err)
	}

	// The summary reports the total the caller passes, which a schema resolver may
	// have adjusted with the actor's stat bonus.
	summary := res.Summary(res.Total + 2)
	if summary.Total != res.Total+2 {
		t.Errorf("total = %d, want the adjusted total", summary.Total)
	}
	if len(summary.Dice) != len(res.Dice) {
		t.Fatalf("summary dice = %#v, want the faces", summary.Dice)
	}

	var nilRoll *RollResult
	if nilRoll.Summary(0) != nil {
		t.Error("a nil roll must have no summary")
	}
}
