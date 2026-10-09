package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/jsonrepair"
)

// adjudicationSystem is the standing instruction for a renegotiation. It biases
// toward a fair table rather than a pushover, because a model that accepts every
// counter makes the difficulty meaningless.
const adjudicationSystem = `You are the Game Master adjudicating a player's counter-proposal to a check you called.

Rule on the counter honestly:

- accept a counter that is reasonable and improves the fiction, adopting its terms;
- adjust a counter that is partly right, stating the terms you will use;
- hold when the counter does not change the difficulty honestly, and say why.

Adopting a good counter is correct. Adopting a bad one is a failure to adjudicate.
Reply with one JSON object and nothing else.`

// adjudicationShape is the reply the GM is asked for.
type adjudicationShape struct {
	Ruling     string `json:"ruling"`
	Stakes     string `json:"stakes"`
	Difficulty string `json:"difficulty"`
	Notation   string `json:"notation"`
	Profile    string `json:"profile"`
	Reason     string `json:"reason"`
}

// Adjudicate asks the GM to rule on a player's counter-proposal to a pending
// check. It is one short generation rather than a turn: the ruling updates the
// check in place, and the player then rolls the agreed terms. A model failure or
// an unparseable answer holds the original terms, so nothing is lost.
func Adjudicate(ctx context.Context, provider harness.ModelProvider, pending harness.PendingCheck, counter harness.CounterProposal) (harness.Adjudication, error) {
	if provider == nil {
		return harness.Adjudication{}, fmt.Errorf("adjudicate: no provider")
	}

	resp, err := provider.Generate(ctx, harness.GenerateRequest{
		System:      adjudicationSystem,
		Prompt:      adjudicationPrompt(pending, counter),
		Temperature: 0.2,
		MaxTokens:   512,
	})
	if err != nil {
		return harness.Adjudication{}, fmt.Errorf("adjudicate: %w", err)
	}

	ruling, ok := parseAdjudication(resp.Text)
	if !ok {
		return harness.Adjudication{}, fmt.Errorf("adjudicate: the reply was not an adjudication")
	}
	if !ruling.Agreed() {
		// A hold keeps the check as it was, so the terms it echoes are ignored.
		return harness.Adjudication{Ruling: harness.RulingHold, Reason: ruling.Reason}, nil
	}
	return ruling, nil
}

// adjudicationPrompt states the check as it stands and the counter, so the GM can
// compare them.
func adjudicationPrompt(pending harness.PendingCheck, counter harness.CounterProposal) string {
	req := pending.Request

	var b strings.Builder
	b.WriteString("The check as you called it:\n")
	writeField(&b, "stakes", req.Stakes)
	writeField(&b, "check", req.CheckKind)
	writeField(&b, "stat", req.Stat)
	writeField(&b, "skill", req.Skill)
	writeField(&b, "difficulty", req.Difficulty)
	writeField(&b, "notation", req.Notation)
	if len(req.Outcomes) > 0 {
		outcomes := make([]string, 0, len(req.Outcomes))
		for key, text := range req.Outcomes {
			outcomes = append(outcomes, key+": "+text)
		}
		writeField(&b, "outcomes", strings.Join(outcomes, "; "))
	}

	b.WriteString("\nThe player's counter-proposal:\n")
	writeField(&b, "approach", counter.Approach)
	writeField(&b, "proposed stakes", counter.Stakes)
	writeField(&b, "proposed difficulty", counter.Difficulty)

	b.WriteString("\nReply with one JSON object and nothing else:\n")
	b.WriteString(`{"ruling":"accept|adjust|hold","stakes":"the agreed stakes","difficulty":"the agreed difficulty id","notation":"the agreed notation","profile":"the agreed profile","reason":"why"}`)
	return b.String()
}

func writeField(b *strings.Builder, name, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "- %s: %s\n", name, strings.TrimSpace(value))
}

// parseAdjudication reads the GM's ruling, repairing a fenced or truncated reply
// the way every other structured reply is repaired. A reply with no ruling is not
// an adjudication.
func parseAdjudication(raw string) (harness.Adjudication, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return harness.Adjudication{}, false
	}
	payload := []byte(trimmed)
	if repaired := jsonrepair.Repair(payload); repaired.OK {
		payload = repaired.Payload
	}

	var shape adjudicationShape
	if err := json.Unmarshal(payload, &shape); err != nil {
		return harness.Adjudication{}, false
	}
	ruling := harness.NormalizeRuling(shape.Ruling)
	return harness.Adjudication{
		Ruling:     ruling,
		Stakes:     strings.TrimSpace(shape.Stakes),
		Difficulty: strings.TrimSpace(shape.Difficulty),
		Notation:   strings.TrimSpace(shape.Notation),
		Profile:    strings.TrimSpace(shape.Profile),
		Reason:     strings.TrimSpace(shape.Reason),
	}, true
}
