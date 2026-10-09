package rules

import (
	"fmt"
	"regexp"
	"strconv"

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

// comparisonPattern matches the threshold ProfileNotation appends to a pool
// notation, such as the ">=8" in "5d10>=8".
var comparisonPattern = regexp.MustCompile(`(>=|<=|>|<|=)\s*(\d+)\s*$`)

// ManualRoll builds a roll from the dice a player entered, so a manual entry is
// recorded as the dice that landed rather than as a bare total. Successes are
// counted against the notation's own threshold, exactly as a rolled pool would
// be, so a manual pool entry resolves the same way a rolled one does.
func ManualRoll(notation string, faces []int) *RollResult {
	total := 0
	dice := make([]harness.DieFace, 0, len(faces))
	for _, face := range faces {
		total += face
		dice = append(dice, harness.DieFace{Value: face, Symbol: strconv.Itoa(face)})
	}
	return &RollResult{
		Notation:  notation,
		Total:     total,
		Successes: countSuccesses(notation, faces),
		RollCount: len(faces),
		Dice:      dice,
	}
}

// countSuccesses counts the entered dice meeting the notation's comparison.
func countSuccesses(notation string, faces []int) int {
	match := comparisonPattern.FindStringSubmatch(notation)
	if match == nil {
		return 0
	}
	threshold, err := strconv.Atoi(match[2])
	if err != nil {
		return 0
	}
	count := 0
	for _, face := range faces {
		switch match[1] {
		case ">=":
			if face >= threshold {
				count++
			}
		case ">":
			if face > threshold {
				count++
			}
		case "<=":
			if face <= threshold {
				count++
			}
		case "<":
			if face < threshold {
				count++
			}
		case "=":
			if face == threshold {
				count++
			}
		}
	}
	return count
}
