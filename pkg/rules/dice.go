package rules

import (
	"fmt"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/roll"
)

type RollResult struct {
	Notation  string `json:"notation"`
	Total     int    `json:"total"`
	Successes int    `json:"successes"`
	RollCount int    `json:"roll_count"`
	// Dice are the faces that landed, in order, so a client can show the dice
	// rather than infer them from the total: two dice totalling four may be 1+3 or
	// 2+2, and neither is the die's size.
	Dice []harness.DieFace `json:"dice,omitempty"`
}

func EvaluateRoll(notation string) (*RollResult, error) {
	program, err := roll.CompileString(notation)
	if err != nil {
		return nil, fmt.Errorf("compile roll %q: %w", notation, err)
	}

	result, err := roll.EvaluateProgram(program)
	if err != nil {
		return nil, fmt.Errorf("evaluate roll %q: %w", notation, err)
	}

	faces := make([]harness.DieFace, 0, len(result.Results))
	for _, die := range result.Results {
		symbol := die.Symbol
		if symbol == "" {
			symbol = fmt.Sprintf("%d", die.Result)
		}
		faces = append(faces, harness.DieFace{Value: die.Result, Symbol: symbol})
	}

	return &RollResult{
		Notation:  notation,
		Total:     result.Total,
		Successes: result.Successes,
		RollCount: len(result.Results),
		Dice:      faces,
	}, nil
}

// Summary is harness's view of the roll, reporting total as the number that
// counts: a resolver may have adjusted it with the actor's stat bonus. The dice
// are always the faces that landed.
func (r *RollResult) Summary(total int) *harness.RollSummary {
	if r == nil {
		return nil
	}
	return &harness.RollSummary{
		Notation:  r.Notation,
		Total:     total,
		Successes: r.Successes,
		RollCount: r.RollCount,
		Dice:      r.Dice,
	}
}
