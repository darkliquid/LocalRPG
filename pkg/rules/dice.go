package rules

import (
	"fmt"

	"github.com/darkliquid/roll"
)

type RollResult struct {
	Notation  string `json:"notation"`
	Total     int    `json:"total"`
	Successes int    `json:"successes"`
	RollCount int    `json:"roll_count"`
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

	return &RollResult{
		Notation:  notation,
		Total:     result.Total,
		Successes: result.Successes,
		RollCount: len(result.Results),
	}, nil
}
