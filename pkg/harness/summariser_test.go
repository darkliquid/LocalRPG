package harness

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/trace"
)

// scriptedSummaryProvider records the prompt it was given, so a test can assert
// what the summariser asked for rather than only what it returned.
type scriptedSummaryProvider struct {
	reply     string
	lastReq   GenerateRequest
	callCount int
}

func (p *scriptedSummaryProvider) ID() string { return "summary-script" }

func (p *scriptedSummaryProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	p.callCount++
	p.lastReq = req
	return &GenerateResponse{Text: p.reply}, nil
}

func (p *scriptedSummaryProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	return nil
}

type failingSummaryProvider struct{}

func (p *failingSummaryProvider) ID() string { return "summary-fail" }

func (p *failingSummaryProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	return nil, errors.New("model unavailable")
}

func (p *failingSummaryProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	return nil
}

func TestSummariserAsksForTheThreadsAndForbidsInvention(t *testing.T) {
	provider := &scriptedSummaryProvider{reply: "The party reached the harbour."}
	summariser := NewSummariser(provider)

	summary, err := summariser.Summarise(context.Background(), "They left the tavern.", []SummaryTurn{
		{Number: 7, Mode: "Do", Input: "I ask about the oil", Narration: "Kael mentioned the oil was low."},
	})
	if err != nil {
		t.Fatalf("Summarise failed: %v", err)
	}
	if summary != "The party reached the harbour." {
		t.Errorf("summary = %q", summary)
	}

	prompt := provider.lastReq.Prompt
	for _, wanted := range []string{"They left the tavern.", "Kael mentioned the oil was low.", "Turn 7"} {
		if !strings.Contains(prompt, wanted) {
			t.Errorf("expected the prompt to carry %q:\n%s", wanted, prompt)
		}
	}
	lowered := strings.ToLower(prompt)
	if !strings.Contains(lowered, "never invent") {
		t.Errorf("the prompt must forbid invention:\n%s", prompt)
	}
	if !strings.Contains(lowered, "promises") || !strings.Contains(lowered, "unresolved threads") {
		t.Errorf("the prompt must ask for what a later turn needs:\n%s", prompt)
	}
}

func TestSummariserTruncatesToTheConfiguredLimit(t *testing.T) {
	provider := &scriptedSummaryProvider{reply: strings.Repeat("long ", 500)}
	summariser := NewSummariser(provider)
	summariser.SetCharLimit(40)

	summary, err := summariser.Summarise(context.Background(), "", []SummaryTurn{{Number: 1, Mode: "Do", Narration: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(summary)) > 41 {
		t.Errorf("summary was not capped: %d runes", len([]rune(summary)))
	}
}

func TestSummariserTracesTheRegeneration(t *testing.T) {
	memory := trace.NewMemory(trace.LevelSummary)
	provider := &scriptedSummaryProvider{reply: "A short summary."}
	summariser := NewSummariser(provider)
	summariser.SetLogger(memory)

	if _, err := summariser.Summarise(context.Background(), "", []SummaryTurn{{Number: 1, Mode: "Do", Narration: "x"}}); err != nil {
		t.Fatal(err)
	}

	event, ok := memory.Find("summary.regenerate")
	if !ok {
		t.Fatalf("expected a summary.regenerate event, got %v", memory.Names())
	}
	if event.Fields["turns"] != 1 {
		t.Errorf("expected the turn count, got %+v", event.Fields)
	}
}

func TestSummariserReportsAProviderFailure(t *testing.T) {
	summariser := NewSummariser(&failingSummaryProvider{})

	if _, err := summariser.Summarise(context.Background(), "", []SummaryTurn{{Number: 1, Mode: "Do", Narration: "x"}}); err == nil {
		t.Errorf("expected the failure to surface, so the caller can leave through_turn alone")
	}
}

func TestSummariserWithoutAProviderFails(t *testing.T) {
	summariser := NewSummariser(nil)

	if _, err := summariser.Summarise(context.Background(), "", nil); err == nil {
		t.Errorf("expected a missing provider to fail rather than return an empty summary")
	}
}
