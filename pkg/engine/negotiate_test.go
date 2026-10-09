package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// scriptedProvider answers with a fixed reply, so an adjudication can be tested
// without a model.
type scriptedProvider struct {
	text string
	err  error
}

func (p *scriptedProvider) ID() string { return "scripted" }

func (p *scriptedProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &harness.GenerateResponse{Text: p.text}, nil
}

func (p *scriptedProvider) Stream(_ context.Context, _ harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	if p.err != nil {
		return p.err
	}
	out <- harness.StreamChunk{Text: p.text, Done: true}
	return nil
}

func testPending() harness.PendingCheck {
	return harness.PendingCheck{
		Ref: "check-1",
		Request: harness.CheckRequest{
			Actor:      "player",
			CheckKind:  "do",
			Stat:       "agility",
			Stakes:     "the lock gives",
			Difficulty: "desperate",
			Outcomes:   map[string]string{"pass": "open", "fail": "stuck"},
		},
	}
}

func TestAdjudicateAcceptsAReasonableCounter(t *testing.T) {
	provider := &scriptedProvider{text: `{"ruling":"accept","stakes":"the bar lifts free","difficulty":"risky","reason":"the bar is not a lock"}`}
	got, err := Adjudicate(context.Background(), provider, testPending(), harness.CounterProposal{Approach: "lift the bar, not pick the lock"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Ruling != harness.RulingAccept || got.Difficulty != "risky" || !got.Agreed() {
		t.Fatalf("adjudication = %+v", got)
	}
}

func TestAdjudicateAdjusts(t *testing.T) {
	provider := &scriptedProvider{text: "```json\n{\"ruling\":\"adjust\",\"difficulty\":\"risky\",\"reason\":\"partly right\"}\n```"}
	got, err := Adjudicate(context.Background(), provider, testPending(), harness.CounterProposal{Approach: "quieter approach"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Ruling != harness.RulingAdjust || got.Reason != "partly right" {
		t.Fatalf("adjudication = %+v", got)
	}
}

// TestAdjudicateHoldKeepsTheTerms guards that a hold never carries new terms,
// whatever the model echoed back.
func TestAdjudicateHoldKeepsTheTerms(t *testing.T) {
	provider := &scriptedProvider{text: `{"ruling":"hold","difficulty":"trivial","reason":"the lock is the lock"}`}
	got, err := Adjudicate(context.Background(), provider, testPending(), harness.CounterProposal{Approach: "just open it"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Ruling != harness.RulingHold || got.Agreed() {
		t.Fatalf("adjudication = %+v", got)
	}
	if got.Difficulty != "" {
		t.Fatalf("a hold must not carry terms, got %q", got.Difficulty)
	}
	if got.Reason == "" {
		t.Fatal("a hold should explain itself")
	}
}

func TestAdjudicateDefaultsToHoldOnAnUnknownRuling(t *testing.T) {
	provider := &scriptedProvider{text: `{"ruling":"maybe","reason":"unclear"}`}
	got, err := Adjudicate(context.Background(), provider, testPending(), harness.CounterProposal{Approach: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Ruling != harness.RulingHold {
		t.Fatalf("ruling = %q, want a hold", got.Ruling)
	}
}

func TestAdjudicateFailsOnAnUnparseableReply(t *testing.T) {
	provider := &scriptedProvider{text: "I think the player is right, probably."}
	if _, err := Adjudicate(context.Background(), provider, testPending(), harness.CounterProposal{Approach: "x"}); err == nil {
		t.Fatal("a reply that is not an adjudication should fail")
	}
}

func TestAdjudicateSurfacesAProviderError(t *testing.T) {
	provider := &scriptedProvider{err: errors.New("no model")}
	if _, err := Adjudicate(context.Background(), provider, testPending(), harness.CounterProposal{Approach: "x"}); err == nil {
		t.Fatal("a provider error should surface")
	}
	if _, err := Adjudicate(context.Background(), nil, testPending(), harness.CounterProposal{}); err == nil {
		t.Fatal("no provider should fail")
	}
}

func TestAdjudicationPromptStatesBothSides(t *testing.T) {
	prompt := adjudicationPrompt(testPending(), harness.CounterProposal{Approach: "lift the bar", Difficulty: "risky"})
	for _, want := range []string{"the lock gives", "desperate", "lift the bar", "risky", "agility"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt omits %q:\n%s", want, prompt)
		}
	}
}
