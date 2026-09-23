package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// replyProvider returns a different scripted reply per call, so a test can give
// the narrator one reply and the completion role another.
type replyProvider struct {
	id      string
	replies []*scriptedStreamProvider
	calls   int
}

func (p *replyProvider) ID() string { return p.id }

func (p *replyProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return nil, fmt.Errorf("replyProvider does not implement Generate")
}

func (p *replyProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	if p.calls >= len(p.replies) {
		defer close(out)
		return fmt.Errorf("no scripted reply for call %d", p.calls)
	}
	reply := p.replies[p.calls]
	p.calls++
	return reply.Stream(ctx, req, out)
}

func recoveryOrchestrator(t *testing.T, gm *scriptedStreamProvider, completion *replyProvider, policy CompletionPolicy) (*TurnOrchestrator, *Timeline) {
	t.Helper()
	orchestrator, timeline, _ := streamingOrchestrator(t, gm)
	if completion != nil {
		orchestrator.SetCompletionProvider(completion)
	}
	orchestrator.SetCompletionPolicy(policy)
	return orchestrator, timeline
}

func TestCompleteReplyIsRecordedUnchanged(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all."}}
	orchestrator, _ := recoveryOrchestrator(t, gm, nil, CompletionPolicy{})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open before us all." {
		t.Errorf("Narration = %q", turn.Narration)
	}
	if turn.Recovery != "" || turn.Truncated {
		t.Errorf("complete reply should not be recovered: recovery=%q truncated=%v", turn.Recovery, turn.Truncated)
	}
}

func TestStructuralCutIsContinued(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all and the hinges groan "}}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{chunks: []string{"in the rising wind."}},
	}}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	want := "The gate stands open before us all and the hinges groan in the rising wind."
	if turn.Narration != want {
		t.Errorf("Narration = %q, want %q", turn.Narration, want)
	}
	if turn.Recovery != string(RecoveryContinued) {
		t.Errorf("Recovery = %q, want continued", turn.Recovery)
	}
	if turn.Truncated {
		t.Errorf("a continued reply is not truncated")
	}
}

func TestInterruptedStreamKeepsTextAndContinues(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all. The old hinge"}, err: errors.New("stream died")}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{chunks: []string{"s groan in the wind."}},
	}}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	want := "The gate stands open before us all. The old hinges groan in the wind."
	if turn.Narration != want {
		t.Errorf("Narration = %q, want %q", turn.Narration, want)
	}
	if turn.Recovery != string(RecoveryContinued) {
		t.Errorf("Recovery = %q, want continued", turn.Recovery)
	}
}

func TestContinuationFailureTrimsToBoundary(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all. The hinges groan and then"}}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{err: errors.New("completion failed")},
	}}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open before us all." {
		t.Errorf("Narration = %q, want the trimmed sentence", turn.Narration)
	}
	if turn.Recovery != string(RecoveryTrimmed) {
		t.Errorf("Recovery = %q, want trimmed", turn.Recovery)
	}
	if turn.Truncated {
		t.Errorf("a trimmed reply is not truncated")
	}
}

func TestShortIncompleteReplyIsKept(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"Mid-sentence cut"}}
	completion := &replyProvider{id: "completion"}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "Mid-sentence cut" {
		t.Errorf("Narration = %q, want the partial unchanged", turn.Narration)
	}
	if turn.Recovery != string(RecoveryKept) || !turn.Truncated {
		t.Errorf("expected kept and truncated, got recovery=%q truncated=%v", turn.Recovery, turn.Truncated)
	}
	if completion.calls != 0 {
		t.Errorf("too-short reply should not call completion, calls = %d", completion.calls)
	}
}

func TestModeOffKeepsRawText(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all. The hinges groan and then"}}
	completion := &replyProvider{id: "completion"}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{Mode: "off"})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open before us all. The hinges groan and then" {
		t.Errorf("Narration = %q, want the raw reply", turn.Narration)
	}
	if turn.Recovery != "" || !turn.Truncated {
		t.Errorf("mode off should keep the raw reply: recovery=%q truncated=%v", turn.Recovery, turn.Truncated)
	}
	if completion.calls != 0 {
		t.Errorf("mode off must not call completion, calls = %d", completion.calls)
	}
}

func TestModeTrimNeverCallsCompletion(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all. The hinges groan and then"}}
	completion := &replyProvider{id: "completion"}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{Mode: "trim"})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open before us all." {
		t.Errorf("Narration = %q, want the trimmed sentence", turn.Narration)
	}
	if turn.Recovery != string(RecoveryTrimmed) {
		t.Errorf("Recovery = %q, want trimmed", turn.Recovery)
	}
	if completion.calls != 0 {
		t.Errorf("mode trim must not call completion, calls = %d", completion.calls)
	}
}

func TestModeContinueKeepsPartialWhenContinuationFails(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all. The hinges groan and then"}}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{err: errors.New("completion failed")},
	}}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{Mode: "continue"})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open before us all. The hinges groan and then" {
		t.Errorf("Narration = %q, want the partial kept", turn.Narration)
	}
	if turn.Recovery != string(RecoveryKept) || !turn.Truncated {
		t.Errorf("expected kept and truncated, got recovery=%q truncated=%v", turn.Recovery, turn.Truncated)
	}
}

func TestContinuationDeltaIsStreamed(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all and the hinges groan "}}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{chunks: []string{"in the rising wind."}},
	}}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	var received []string
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", func(text string) error {
		received = append(received, text)
		return nil
	}); err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(received) != 2 || received[1] != "in the rising wind." {
		t.Errorf("streamed chunks = %v, want the continuation forwarded", received)
	}
}

func TestRecoveryOutcomeIsPersisted(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all and the hinges groan "}}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{chunks: []string{"in the rising wind."}},
	}}
	orchestrator, timeline := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil); err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected one recorded turn, got %d", len(turns))
	}
	if turns[0].Recovery != string(RecoveryContinued) {
		t.Errorf("recorded recovery = %q, want continued", turns[0].Recovery)
	}
	if turns[0].Truncated {
		t.Errorf("a continued reply should not be recorded truncated")
	}
}
