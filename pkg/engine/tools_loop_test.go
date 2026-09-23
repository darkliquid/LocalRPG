package engine

import (
	"context"
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
