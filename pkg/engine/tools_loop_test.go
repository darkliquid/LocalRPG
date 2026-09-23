package engine

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// toolScriptProvider returns a different scripted reply per call, so a test can
// script a tool round followed by a final answer.
type toolScriptProvider struct {
	mu       sync.Mutex
	replies  []toolReply
	calls    int
	requests []harness.GenerateRequest
}

type toolReply struct {
	text  string
	tools []harness.ToolCall
}

func (p *toolScriptProvider) ID() string { return "tool-script" }

func (p *toolScriptProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	reply, _ := p.next(req)
	return &harness.GenerateResponse{Text: reply.text}, nil
}

func (p *toolScriptProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	reply, ok := p.next(req)
	if !ok {
		return nil
	}
	if reply.text != "" {
		out <- harness.StreamChunk{Text: reply.text}
	}
	out <- harness.StreamChunk{Done: true, FinishReason: "stop", ToolCalls: reply.tools}
	return nil
}

func (p *toolScriptProvider) next(req harness.GenerateRequest) (toolReply, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)
	if p.calls >= len(p.replies) {
		p.calls++
		return toolReply{}, false
	}
	reply := p.replies[p.calls]
	p.calls++
	return reply, true
}

// fakeExecutor records the calls it was given and returns scripted results.
type fakeExecutor struct {
	mu      sync.Mutex
	calls   []harness.ToolCall
	results []string
}

func (f *fakeExecutor) Execute(ctx context.Context, call harness.ToolCall) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	if len(f.results) == 0 {
		return "no results", true
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result, true
}

func toolLoopOrchestrator(t *testing.T, provider harness.ModelProvider) (*TurnOrchestrator, *Timeline) {
	t.Helper()
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)
	return orchestrator, timeline
}

func TestToolCapabilityResolution(t *testing.T) {
	cases := []struct {
		name       string
		capability string
		caller     bool
		want       bool
	}{
		{"auto with a tool-calling provider", "auto", true, true},
		{"auto without one", "auto", false, false},
		{"yes forces it", "yes", false, true},
		{"no suppresses it", "no", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orchestrator, _ := toolLoopOrchestrator(t, &scriptedStreamProvider{})
			orchestrator.SetTools(&fakeExecutor{}, tc.capability)
			if got := orchestrator.offersTools(tc.caller); got != tc.want {
				t.Errorf("offersTools = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestToolLoopRunsACallThenAnswers(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{text: "let me check", tools: []harness.ToolCall{{ID: "1", Name: "search_entities", Arguments: `{"query":"warden"}`}}},
		{text: "The Warden keeps the eastern gate."},
	}}
	executor := &fakeExecutor{results: []string{"The Warden (character, id warden): a grim guard."}}
	orchestrator, timeline := toolLoopOrchestrator(t, provider)
	orchestrator.SetTools(executor, "yes")

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "who guards the gate?", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The Warden keeps the eastern gate." {
		t.Errorf("Narration = %q", turn.Narration)
	}
	if len(executor.calls) != 1 || executor.calls[0].Name != "search_entities" {
		t.Errorf("executor calls = %+v", executor.calls)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "search_entities" {
		t.Errorf("turn provenance = %+v", turn.ToolCalls)
	}

	// The second call must carry the tool result as a tool message.
	provider.mu.Lock()
	last := provider.requests[len(provider.requests)-1]
	provider.mu.Unlock()
	found := false
	for _, message := range last.Messages {
		if message.Role == "tool" && message.ToolCallID == "1" {
			found = true
		}
	}
	if !found {
		t.Errorf("the second request did not carry the tool result: %+v", last.Messages)
	}

	if turns, err := timeline.history.LoadHistory(); err != nil || len(turns) != 1 {
		t.Errorf("recorded turns = %d, err = %v", len(turns), err)
	}
}

func TestToolLoopStopsAtTheRoundLimit(t *testing.T) {
	always := make([]toolReply, 0, 8)
	for i := 0; i < 8; i++ {
		always = append(always, toolReply{tools: []harness.ToolCall{{ID: "1", Name: "search_entities", Arguments: `{}`}}})
	}
	provider := &toolScriptProvider{replies: always}
	executor := &fakeExecutor{}
	orchestrator, _ := toolLoopOrchestrator(t, provider)
	orchestrator.SetTools(executor, "yes")
	orchestrator.SetToolRounds(2)

	// Every round asks for a tool, so the loop runs out and the turn fails for
	// want of narration rather than looping forever.
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "keep looking", nil); err == nil {
		t.Fatalf("expected the turn to fail when no narration was ever produced")
	}
	if len(executor.calls) != 2 {
		t.Errorf("executed %d calls, want the round cap of 2", len(executor.calls))
	}
}

func TestToolLoopWithdrawsToolsUnderBudget(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "search_entities", Arguments: `{"query":"warden"}`}}},
		{text: "Answering from what I have."},
	}}
	executor := &fakeExecutor{results: []string{strings.Repeat("x", 500000)}}
	orchestrator, _ := toolLoopOrchestrator(t, provider)
	orchestrator.SetTools(executor, "yes")
	// The prompt fits, but the oversized tool result crosses the budget, so the
	// next round withdraws tools.
	orchestrator.SetContextLimits(harness.ContextLimits{TokenBudget: 100000})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "who guards the gate?", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration == "" {
		t.Errorf("expected an answer from what the model had")
	}

	provider.mu.Lock()
	last := provider.requests[len(provider.requests)-1]
	provider.mu.Unlock()
	if len(last.Tools) != 0 {
		t.Errorf("the withdrawn round must be sent without tools")
	}
	withdrawn := false
	for _, message := range last.Messages {
		if message.Role == "tool" && len(message.Content) > 0 && message.ToolCallID == "" {
			withdrawn = true
		}
	}
	if !withdrawn {
		t.Errorf("the refusal must be a readable tool result: %+v", last.Messages)
	}
}

func TestToolLoopWithoutToolsIsUnchanged(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{{text: "The gate stands open."}}}
	orchestrator, _ := toolLoopOrchestrator(t, provider)

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open." {
		t.Errorf("Narration = %q", turn.Narration)
	}
}
