# Agentic Turns: The Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn one model call into a bounded loop that can run tool calls between calls, stream tool activity to the player, and record what a turn looked up.

**Architecture:** Only the generation step of `ProcessActionStream` changes. A `ToolExecutor` interface keeps `pkg/engine` free of any tool implementation, an optional `ToolCaller` interface on the provider makes capability inference concrete, and the loop appends a conversation rather than replacing the single prompt. Everything downstream of generation (segmentation, mentions, extraction, recording, recovery) is untouched.

**Tech Stack:** Go 1.27.1 (standard library only for tests), the contract and tools delivered by `agentic-turns-contract-and-search`, React 19 + TypeScript for the activity line.

**Spec:** `docs/superpowers/specs/2026-09-22-agentic-turns-and-tools-design.md` (§4, §6, §9, §10, §11, §12, §16)
**Depends on:** `docs/superpowers/plans/2026-09-23-agentic-turns-contract-and-search.md` (implemented), which delivered `Message`/`ToolSpec`/`ToolCall`, HTTP tools and accumulation, `supports_tools`, `tool_rounds`, `tool_result_chars`, `ToolSpecs`, and `pkg/tools.Executor`.

## Scope

This is increment 3 of the spec: the loop, its bounds, the `tool` stream event, the watchdog behaviour, `Turn.ToolCalls` provenance, and the UI that renders them.

Two notes on the existing code:
- **There is no client-side inactivity guard to reset.** The server's per-call chunk watchdog in `pkg/engine` is the only one. Because that watchdog is started per generation call and stopped when the call returns, tool execution between calls is inherently outside it: a slow tool cannot stall a turn. This plan adds no client watchdog, and the earlier frontend plan's "reset the client inactivity guard" line does not apply.
- Increment 4 (embeddings / `search_semantic`) is out of scope; the tool surface stays as the previous plan fixed it.

## Global Constraints

- Standard library only for tests. Use `interface{}`, never `any`. Errors wrapped with `fmt.Errorf("...: %w", err)`. `go vet ./...` clean.
- A turn always terminates: rounds are bounded, and once they are spent or the budget is reached, tools are withdrawn and the model is told to answer.
- A tool error is result text, never a failed turn. A failed tool never loses the turn.
- Prose emitted in a round that also contains tool calls is discarded and traced, never concatenated.
- A stray call in the final, tool-less round is ignored while its text is kept.
- Arguments and results live in the trace; `Turn.ToolCalls` carries only name and result size.
- Never write em dashes in source code.
- Conventional Commits with a scope, subject under 72 characters.
- Verification: `mise run test` and `mise run lint`; frontend gate `cd frontend && npx tsc --noEmit`.
- Single Go test example: `go test -run TestToolLoop ./pkg/engine/`.

### File Map

| Action | Path | Responsibility |
| :--- | :--- | :--- |
| Modify | `pkg/engine/orchestrator.go` | `streamResult.ToolCalls`, `generateRequest`, the loop, withdrawal, budget, trace rows, observer |
| Modify | `pkg/harness/http_provider.go` | `ToolCaller` interface implementation |
| Modify | `pkg/engine/history.go` | `Turn.ToolCalls []ToolCallRecord` |
| Create | `pkg/engine/tools_loop_test.go` | A scripted tool-calling provider and the loop tests |
| Modify | `pkg/engine/history_test.go` | Provenance round-trip |
| Modify | `pkg/gui/types.go` | `TurnEvent` tool fields, `ToolCallDTO`, `TurnDTO.ToolCalls` |
| Modify | `pkg/gui/service.go` | Observer wiring, `turnDTO` copy, `prepareTurn` builds the executor |
| Modify | `pkg/gui/tool_loop_test.go` (create) | The tool event reaches the stream |
| Modify | `cmd/localrpg/play.go` | Wire the executor for the TUI |
| Modify | `frontend/src/types.ts` | `ToolCall`, `TurnEvent` tool fields |
| Modify | `frontend/src/App.tsx` | Tool activity line while a call runs |
| Modify | `frontend/src/components/ChronicleView.tsx` | The quiet provenance line |
| Modify | `frontend/src/components/SettingsStudio.tsx` | Tool limits and the capability control |

---

## Task 1: Carry tool calls out of a stream

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/engine/orchestrator_stream_test.go`

**Interfaces:**
- Consumes: `harness.StreamChunk.ToolCalls` (contract plan).
- Produces: `streamResult.ToolCalls []harness.ToolCall`; `(*TurnOrchestrator).generateRequest(ctx, req harness.GenerateRequest, onChunk) (streamResult, error)`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/engine/orchestrator_stream_test.go`:

```go
func TestStreamSurfacesToolCalls(t *testing.T) {
	provider := &scriptedStreamProvider{
		chunks:    []string{"let me check"},
		toolCalls: []harness.ToolCall{{ID: "1", Name: "search_entities", Arguments: `{"query":"warden"}`}},
	}
	orchestrator, _, _ := streamingOrchestrator(t, provider)

	result, err := orchestrator.generateRequest(context.Background(), harness.GenerateRequest{
		Messages: []harness.Message{{Role: "user", Content: "who is the warden?"}},
	}, nil)
	if err != nil {
		t.Fatalf("generateRequest: %v", err)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "search_entities" {
		t.Errorf("ToolCalls = %+v, want the stream's call", result.ToolCalls)
	}
}
```

Add a `toolCalls []harness.ToolCall` field to `scriptedStreamProvider` and emit them with the terminal chunk in its `Stream` (the provider is defined at the top of this file):

```go
	for _, chunk := range p.chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- harness.StreamChunk{Text: chunk}:
		}
	}
	out <- harness.StreamChunk{Done: true, ToolCalls: p.toolCalls}
	return p.err
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestStreamSurfacesToolCalls ./pkg/engine/ -v`
Expected: FAIL with "unknown field toolCalls" or "undefined: generateRequest".

- [ ] **Step 3: Implement**

In `pkg/engine/orchestrator.go`, extend `streamResult`:

```go
type streamResult struct {	Text         string
	FinishReason string
	Interrupted  error
	ToolCalls    []harness.ToolCall
	// Provenance is what the turn looked up, compactly: name and result size.
	// Arguments and results live in the trace, not in the campaign's history.
	Provenance []ToolCallRecord
}
```

In `stream`, capture them from the terminal chunk:

```go
			if !ok {
				if err := <-streamErr; err != nil {
					...
				}
				return streamResult{Text: sb.String(), FinishReason: finishReason, ToolCalls: chunkToolCalls}, nil
			}
```

The terminal chunk is the one that closes the channel; track it as it arrives. Store the calls as they are seen, so the final result carries them:

```go
	var sb strings.Builder
	finishReason := ""
	var toolCalls []harness.ToolCall
	...
			if len(chunk.ToolCalls) > 0 {
				toolCalls = chunk.ToolCalls
			}
```

In `pkg/engine/history.go`, add the record type this result carries:

```go
// ToolCallRecord is one tool a turn called, kept compact on purpose: the
// arguments and the result live in the trace, not in the campaign's history.
type ToolCallRecord struct {
	Name        string `json:"name"`
	ResultChars int    `json:"result_chars"`
}
```

Add `generateRequest` and make `generate` a wrapper:

```go
// generate streams the gm reply through stream, falling back to the configured
// fallback provider when the primary fails before producing any text.
func (o *TurnOrchestrator) generate(ctx context.Context, prompt string, onChunk func(string) error) (streamResult, error) {
	return o.generateRequest(ctx, harness.GenerateRequest{Prompt: prompt}, onChunk)
}

// generateRequest streams one request through the gm role, falling back to the
// configured fallback provider when the primary fails before producing any text.
func (o *TurnOrchestrator) generateRequest(ctx context.Context, req harness.GenerateRequest, onChunk func(string) error) (streamResult, error) {
	provider, err := o.router.GetProviderForRole("gm")
	if err != nil {
		return streamResult{}, err
	}

	result, err := o.stream(ctx, provider, req, onChunk)
	if err == nil || errors.Is(err, errStreamListener) || ctx.Err() != nil {
		return result, err
	}
	if fallback, ok := o.router.FallbackForRole("gm"); ok {
		return o.stream(ctx, fallback, req, onChunk)
	}
	return result, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_stream_test.go
git commit -m "feat(engine): carry tool calls out of a provider stream"
```

---

## Task 2: Tool execution seams

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/harness/http_provider.go`
- Create: `pkg/engine/tools_loop_test.go` (the shared fixture for Tasks 2-4)

**Interfaces:**
- Produces: `engine.ToolExecutor`, `engine.ToolActivity`, `(*TurnOrchestrator).SetTools(executor ToolExecutor, capability string)`, `(*TurnOrchestrator).SetToolRounds(rounds int)`, `(*TurnOrchestrator).SetToolObserver(observer func(ToolActivity))`, `harness.ToolCaller`.

- [ ] **Step 1: Write the failing test**

Create `pkg/engine/tools_loop_test.go` with the fixture and a capability test:

```go
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
	mu     sync.Mutex
	replies []toolReply
	calls  int
	requests []harness.GenerateRequest
}

type toolReply struct {
	text  string
	tools []harness.ToolCall
}

func (p *toolScriptProvider) ID() string { return "tool-script" }

func (p *toolScriptProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)
	reply := toolReply{}
	if p.calls < len(p.replies) {
		reply = p.replies[p.calls]
	}
	p.calls++
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
			orchestrator.SetTools(&fakeExecutor{}, tc.capability)
			if got := orchestrator.offersTools(tc.caller); got != tc.want {
				t.Errorf("offersTools = %v, want %v", got, tc.want)
			}
		})
	}
}
```

Adjust the capability helper's exact shape while implementing Step 3; the test is the contract: `auto` follows `ToolCaller`, `yes` forces, `no` suppresses.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestToolCapabilityResolution ./pkg/engine/ -v`
Expected: FAIL with "undefined: SetTools".

- [ ] **Step 3: Implement**

In `pkg/engine/orchestrator.go`, add to the struct:

```go
	toolExecutor  ToolExecutor
	toolCapability string
	toolRounds    int
	toolObserver  func(ToolActivity)
```

Add the seams:

```go
// ToolExecutor runs one tool call and returns the text a model will read. A
// failed call still returns readable text, so a tool error never loses a turn.
type ToolExecutor interface {
	Execute(ctx context.Context, call harness.ToolCall) (result string, ok bool)
}

// ToolActivity is one step of tool activity a client can render as it happens.
type ToolActivity struct {
	Round   int
	Name    string
	Status  string // "running" or "done"
	Summary string
}

// SetTools attaches the executor and the role's declared capability: "auto",
// "yes", or "no".
func (o *TurnOrchestrator) SetTools(executor ToolExecutor, capability string) {
	o.toolExecutor = executor
	o.toolCapability = capability
}

// SetToolRounds caps the tool rounds in one turn.
func (o *TurnOrchestrator) SetToolRounds(rounds int) {
	if rounds <= 0 {
		rounds = 4
	}
	o.toolRounds = rounds
}

// SetToolObserver receives tool activity as it happens, so a client can show it
// rather than waiting in silence.
func (o *TurnOrchestrator) SetToolObserver(observer func(ToolActivity)) {
	o.toolObserver = observer
}

func (o *TurnOrchestrator) toolRoundCap() int {
	if o.toolRounds <= 0 {
		return 4
	}
	return o.toolRounds
}

// offersTools decides whether to offer a tool surface to a provider. isCaller is
// whether the provider implements harness.ToolCaller; capability "auto" follows
// it, "yes" forces, and "no" suppresses.
func (o *TurnOrchestrator) offersTools(isCaller bool) bool {
	if o.toolExecutor == nil {
		return false
	}
	switch o.toolCapability {
	case "yes":
		return true
	case "no":
		return false
	default:
		return isCaller
	}
}
```

The test builds its own orchestrator; the fixture does not need a provider for this case.

In `pkg/harness/http_provider.go`, implement the capability interface:

```go
// ToolCaller marks a provider that can accept a tools field and return tool
// calls. It is what "auto" checks, so a provider type that cannot is never
// offered a surface it would ignore or reject.
func (h *HTTPProvider) ToolCallerCapable() bool { return true }
```

and declare the interface beside `ModelProvider` in `pkg/harness/types.go`:

```go
// ToolCaller is implemented by providers that can be offered tools.
type ToolCaller interface {
	ToolCallerCapable() bool
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ ./pkg/harness/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/tools_loop_test.go pkg/harness/http_provider.go pkg/harness/types.go
git commit -m "feat(engine): add the tool execution seams"
```

---

## Task 3: The bounded loop

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/engine/tools_loop_test.go`

**Interfaces:**
- Consumes: `generateRequest`, the seams (Tasks 1-2), `harness.ToolSpecs`, `recoverReply`.
- Produces: the loop, with rounds, budget withdrawal, readable refusal, and the `tool.round`/`tool.call`/`tool.result` trace rows.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/engine/tools_loop_test.go`:

```go
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
	if "let me check" == turn.Narration {
		t.Errorf("the preamble prose must not become narration")
	}
	if len(executor.calls) != 1 || executor.calls[0].Name != "search_entities" {
		t.Errorf("executor calls = %+v", executor.calls)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "search_entities" {
		t.Errorf("turn provenance = %+v", turn.ToolCalls)
	}

	// The second call must carry the tool result as a tool message.
	provider.mu.Lock()
	defer provider.mu.Unlock()
	last := provider.requests[len(provider.requests)-1]
	found := false
	for _, message := range last.Messages {
		if message.Role == "tool" && message.ToolCallID == "1" {
			found = true
		}
	}
	if !found {
		t.Errorf("the second request did not carry the tool result: %+v", last.Messages)
	}
	if len(last.Tools) != 0 {
		t.Errorf("the final round must be sent without tools")
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

	// Every round asks for a tool, so the loop runs out and the final call fails
	// for want of narration rather than looping forever.
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
	defer provider.mu.Unlock()
	last := provider.requests[len(provider.requests)-1]
	if len(last.Tools) != 0 {
		t.Errorf("the withdrawn round must be sent without tools")
	}
	withdrawn := false
	for _, message := range last.Messages {
		if message.Role == "tool" && strings.Contains(message.Content, "no longer available") {
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
```

Add `"strings"` to the test file's imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestToolLoop' ./pkg/engine/ -v`
Expected: FAIL: no loop runs, so the executor is never called and provenance is empty.

- [ ] **Step 3: Implement the loop**

In `pkg/engine/orchestrator.go`, replace the generation block in `ProcessActionStream`:

```go
	result, err := o.runGenerationLoop(ctx, contextPrompt, actionInput, onChunk)
	if err != nil {
		return nil, fmt.Errorf("gm generation failed: %w", err)
	}

	cause := o.classifyCut(result)
	narration, recovery, stillIncomplete := o.recoverReply(ctx, result.Text, cause, onChunk)
	if strings.TrimSpace(narration) == "" {
		return nil, fmt.Errorf("gm returned no narration")
	}
```

and add:

```go
// runGenerationLoop runs the turn as a bounded conversation. It offers tools only
// while the role can call them, the rounds are not spent, and the conversation is
// within budget; otherwise the model is told tools are unavailable and asked to
// answer. The result is the last reply that carried no tool calls.
func (o *TurnOrchestrator) runGenerationLoop(ctx context.Context, contextPrompt, action string, onChunk func(string) error) (streamResult, error) {
	messages := []harness.Message{
		{Role: "system", Content: contextPrompt},
		{Role: "user", Content: action},
	}

	provider, err := o.router.GetProviderForRole("gm")
	if err != nil {
		return streamResult{}, err
	}
	isCaller := false
	if caller, ok := provider.(harness.ToolCaller); ok {
		isCaller = caller.ToolCallerCapable()
	}
	canCallTools := o.offersTools(isCaller)

	var provenance []ToolCallRecord
	withdrawn := false

	for round := 0; round <= o.toolRoundCap(); round++ {
		offerTools := canCallTools && round < o.toolRoundCap() && !withdrawn && !o.overBudget(messages)

		// Prompt is kept for a caller or provider that only reads a string: it is
		// the assembled context, exactly as the single-prompt path sent it, so
		// existing behaviour and tests are unchanged. A provider that can call
		// tools reads Messages instead.
		request := harness.GenerateRequest{Messages: messages, Prompt: contextPrompt}
		if offerTools {
			request.Tools = harness.ToolSpecs()
		}
		o.logger.Event("tool.round", map[string]interface{}{
			"round":               round,
			"offered":             offerTools,
			"conversation_tokens": conversationTokens(messages),
			"budget":              o.contextBudget(),
		})

		result, err := o.generateRequest(ctx, request, onChunk)
		if err != nil {
			return streamResult{}, err
		}

		if len(result.ToolCalls) == 0 {
			// A stray call in the tool-less round is ignored; its text is kept.
			if len(result.ToolCalls) > 0 && !offerTools {
				o.logger.Event("tool.stray", map[string]interface{}{"round": round, "calls": len(result.ToolCalls)})
			}
			return result, nil
		}

		// Prose in a tool round is the model thinking out loud, and its order
		// relative to the result is undefined, so it is discarded and traced.
		if strings.TrimSpace(result.Text) != "" {
			o.logger.Event("tool.prose_discarded", map[string]interface{}{
				"round": round,
				"chars": len([]rune(result.Text)),
			})
		}

		messages = append(messages, harness.Message{Role: "assistant", ToolCalls: result.ToolCalls})
		for _, call := range result.ToolCalls {
			o.logger.Event("tool.call", map[string]interface{}{
				"round":           round,
				"name":            call.Name,
				"arguments":       call.Arguments,
				"arguments_chars": len([]rune(call.Arguments)),
			})
			o.notifyTool(ToolActivity{Round: round, Name: call.Name, Status: "running"})

			started := time.Now()
			output, ok := o.toolExecutor.Execute(ctx, call)
			o.logger.Event("tool.result", map[string]interface{}{
				"name":        call.Name,
				"ok":          ok,
				"bytes":       len(output),
				"duration_ms": time.Since(started).Milliseconds(),
				"result":      output,
			})
			o.notifyTool(ToolActivity{Round: round, Name: call.Name, Status: "done", Summary: toolSummary(call.Name, ok, output)})

			provenance = append(provenance, ToolCallRecord{Name: call.Name, ResultChars: len([]rune(output))})
			messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: output})
		}

		// After the final allowed round, or once the budget is crossed, tools are
		// withdrawn and the model is told so as a readable result rather than a
		// silent stop it would retry.
		if round+1 > o.toolRoundCap() || o.overBudget(messages) {
			withdrawn = true
			messages = append(messages, harness.Message{
				Role:    "tool",
				Content: "Tools are no longer available for this turn. Answer now with what you already know.",
			})
		}
	}

	return streamResult{}, fmt.Errorf("tool loop ended without an answer")
}

// notifyTool reports activity if a client asked to see it.
func (o *TurnOrchestrator) notifyTool(activity ToolActivity) {
	if o.toolObserver != nil {
		o.toolObserver(activity)
	}
}

// toolSummary is the short human line a client renders, for example "3 matches".
func toolSummary(name string, ok bool, output string) string {
	if !ok {
		return "failed"
	}
	if len(output) == 0 {
		return "no result"
	}
	return fmt.Sprintf("%d characters", len([]rune(output)))
}

// conversationTokens estimates the live conversation's size. Four runes per token
// is deliberately crude: the budget is a guardrail, not an accounting ledger.
func conversationTokens(messages []harness.Message) int {
	runes := 0
	for _, message := range messages {
		runes += len([]rune(message.Content))
	}
	return runes / 4
}

// contextBudget is the configured token ceiling, or 0 for unbounded.
func (o *TurnOrchestrator) contextBudget() int {
	return o.assembler.Limits().TokenBudget
}

// overBudget reports whether the live conversation has crossed the budget.
func (o *TurnOrchestrator) overBudget(messages []harness.Message) bool {
	budget := o.contextBudget()
	if budget <= 0 {
		return false
	}
	return conversationTokens(messages) > budget
}
```

`Provenance` is a field on `streamResult` (added in Task 1) so the loop can carry it out to `ProcessActionStream` without a second return value. Task 4 copies it onto `Turn`.

**The stray-call case.** A reply carrying calls while tools were *not* offered is the protocol quirk §4 describes: its text is the answer, and the calls are dropped with a trace line. Handle it immediately after generating, before the no-calls return:

```go
		if len(result.ToolCalls) > 0 && !offerTools {
			o.logger.Event("tool.stray", map[string]interface{}{"round": round, "calls": len(result.ToolCalls)})
			result.ToolCalls = nil
			result.Provenance = provenance
			return result, nil
		}
		if len(result.ToolCalls) == 0 {
			result.Provenance = provenance
			return result, nil
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -count=1`
Expected: PASS. If `TestToolLoopStopsAtTheRoundLimit` fails because the loop returns the scripted empty reply rather than an error, make the scripted provider return no reply once its list is exhausted (already the fixture's behaviour) and assert on the executor's call count and the error instead.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/tools_loop_test.go
git commit -m "feat(engine): run a turn as a bounded tool loop"
```

---

## Task 4: Provenance on the turn

**Files:**
- Modify: `pkg/engine/history.go`
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/engine/history_test.go`
- Modify: `pkg/engine/tools_loop_test.go`

**Interfaces:**
- Produces: `Turn.ToolCalls []ToolCallRecord`, populated from the loop's provenance.

- [ ] **Step 1: Write the failing test**

Append to `pkg/engine/history_test.go`:

```go
func TestTurnToolCallsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	logger := NewHistoryLogger(filepath.Join(dir, "history.jsonl"))
	turn := Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "look",
		Narration: "You look.",
		ToolCalls: []ToolCallRecord{{Name: "search_entities", ResultChars: 42}},
	}
	if err := logger.AppendTurn(turn); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	turns, err := logger.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(turns) != 1 || len(turns[0].ToolCalls) != 1 {
		t.Fatalf("turns = %+v", turns)
	}
	if turns[0].ToolCalls[0].Name != "search_entities" || turns[0].ToolCalls[0].ResultChars != 42 {
		t.Errorf("provenance = %+v", turns[0].ToolCalls)
	}
}
```

Match the imports already in that file (`path/filepath`, `time`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestTurnToolCallsRoundTrip ./pkg/engine/ -v`
Expected: FAIL to compile: `Turn` has no `ToolCalls`.

- [ ] **Step 3: Implement**

In `pkg/engine/history.go`, add the field to `Turn` (`ToolCallRecord` was defined here by Task 1):

```go
	// ToolCalls records what the turn looked up, compactly: name and result size.
	// Arguments and results live in the trace, not in the campaign's history.
	ToolCalls []ToolCallRecord `json:"tool_calls,omitempty"`
```

In `orchestrator.go`'s `ProcessActionStream`, after the loop and the recovery pass, set the turn's provenance from the loop result:

```go
	turn := Turn{
		...
		ToolCalls: result.Provenance,
	}
```

and in `TestToolLoopRunsACallThenAnswers`, the existing `turn.ToolCalls` assertion now exercises this.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/engine/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/history.go pkg/engine/orchestrator.go pkg/engine/history_test.go pkg/engine/tools_loop_test.go
git commit -m "feat(engine): record what a turn looked up"
```

---

## Task 5: The tool stream event and the DTO

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Create: `pkg/gui/tool_loop_test.go`

**Interfaces:**
- Consumes: `engine.ToolActivity`, `engine.Turn.ToolCalls`.
- Produces: `TurnEvent` tool fields, `ToolCallDTO`, `TurnDTO.ToolCalls`.

- [ ] **Step 1: Write the failing test**

Create `pkg/gui/tool_loop_test.go`:

```go
package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
)

func TestTurnDTOCarriesToolProvenance(t *testing.T) {
	dto := TurnDTO{
		TurnNumber: 1,
		ToolCalls:  []ToolCallDTO{{Name: "search_entities", ResultChars: 42}},
	}
	if len(dto.ToolCalls) != 1 || dto.ToolCalls[0].Name != "search_entities" {
		t.Errorf("ToolCalls = %+v", dto.ToolCalls)
	}
}

func TestTurnEventCarriesToolActivity(t *testing.T) {
	event := TurnEvent{Type: "tool", ToolName: "search_entities", ToolStatus: "done", ToolSummary: "3 matches"}
	if event.ToolName == "" || event.ToolStatus != "done" || event.ToolSummary == "" {
		t.Errorf("event = %+v", event)
	}
}

func TestToolActivityMapsToATurnEvent(t *testing.T) {
	event := toolEvent(engine.ToolActivity{Round: 1, Name: "get_entity", Status: "running"})
	if event.Type != "tool" || event.ToolName != "get_entity" || event.ToolStatus != "running" {
		t.Errorf("event = %+v", event)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestTurnDTOCarriesTool|TestTurnEventCarriesTool|TestToolActivityMaps' ./pkg/gui/ -v`
Expected: FAIL with "unknown field ToolCalls" and "undefined: toolEvent".

- [ ] **Step 3: Implement**

In `pkg/gui/types.go`, extend `TurnEvent` and add the DTO:

```go
type TurnEvent struct {
	Type    string   `json:"type"` // "chunk", "turn", "tool", "error", or "model_missing"
	...
	// Tool activity, present when Type is "tool".
	ToolName    string `json:"tool_name,omitempty"`
	ToolStatus  string `json:"tool_status,omitempty"`
	ToolSummary string `json:"tool_summary,omitempty"`
}

// ToolCallDTO is one tool a turn called, with only its name and result size.
type ToolCallDTO struct {
	Name        string `json:"name"`
	ResultChars int    `json:"result_chars"`
}
```

Add to `TurnDTO`:

```go
	ToolCalls []ToolCallDTO `json:"tool_calls,omitempty"`
```

In `pkg/gui/service.go`, copy it in `turnDTO`:

```go
		ToolCalls: turnToolCallDTOs(turn.ToolCalls),
```

with:

```go
func turnToolCallDTOs(records []engine.ToolCallRecord) []ToolCallDTO {
	if len(records) == 0 {
		return nil
	}
	dtos := make([]ToolCallDTO, 0, len(records))
	for _, record := range records {
		dtos = append(dtos, ToolCallDTO{Name: record.Name, ResultChars: record.ResultChars})
	}
	return dtos
}

// toolEvent maps engine tool activity onto the stream's event framing.
func toolEvent(activity engine.ToolActivity) TurnEvent {
	return TurnEvent{
		Type:        "tool",
		ToolName:    activity.Name,
		ToolStatus:  activity.Status,
		ToolSummary: activity.Summary,
	}
}
```

and in `TurnSession.Run`, before calling `ProcessActionStream`, attach the observer:

```go
	t.orchestrator.SetToolObserver(func(activity engine.ToolActivity) {
		_ = emit(toolEvent(activity))
	})
```

An emit failure is ignored here on purpose: the turn still records, and a disconnected client is handled by the existing chunk listener.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/tool_loop_test.go
git commit -m "feat(gui): stream a turn's tool activity and record its provenance"
```

---

## Task 6: Wire the executor into both turn pipelines

**Files:**
- Modify: `pkg/gui/service.go` (`prepareTurn`)
- Modify: `cmd/localrpg/play.go`

**Interfaces:**
- Consumes: `tools.NewExecutor`, `config.RoleSupportsTools`, `config.ToolRounds`, `config.ToolResultChars`.

- [ ] **Step 1: Wire the GUI pipeline**

In `prepareTurn`, after the completion policy is set:

```go
	orchestrator.SetTools(tools.NewExecutor(store, cfg.ToolResultChars()), cfg.RoleSupportsTools("gm"))
	orchestrator.SetToolRounds(cfg.ToolRounds())
```

Add `"github.com/darkliquid/localrpg/pkg/tools"` to `pkg/gui/service.go`'s imports.

- [ ] **Step 2: Wire the TUI pipeline**

In `cmd/localrpg/play.go`, after `SetCompletionPolicy`:

```go
	orchestrator.SetTools(tools.NewExecutor(store, cfg.ToolResultChars()), cfg.RoleSupportsTools("gm"))
	orchestrator.SetToolRounds(cfg.ToolRounds())
```

Add the `tools` import.

- [ ] **Step 3: Verify**

Run: `go build ./... && go test ./pkg/gui/ ./cmd/... -count=1`
Expected: build and tests pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/gui/service.go cmd/localrpg/play.go
git commit -m "feat: give the turn loop its tools in the GUI and the TUI"
```

---

## Task 7: The activity line and the provenance line

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/components/ChronicleView.tsx`

**Interfaces:**
- Consumes: the `tool` stream event and `TurnDTO.ToolCalls`.

- [ ] **Step 1: Add the types**

In `frontend/src/types.ts`:

```ts
export interface ToolCall {
  name: string;
  result_chars: number;
}

export interface TurnEvent {
  type: 'chunk' | 'turn' | 'tool' | 'error' | 'model_missing';
  text?: string;
  turn?: Turn;
  message?: string;
  model_id?: string;
  name?: string;
  size_bytes?: number;
  tool_name?: string;
  tool_status?: 'running' | 'done';
  tool_summary?: string;
}
```

and add to `interface Turn`:

```ts
  tool_calls?: ToolCall[];
```

- [ ] **Step 2: Render the activity line**

In `App.tsx`, add state beside `streamedProse`:

```tsx
  const [toolActivity, setToolActivity] = useState<string | null>(null);
```

In the stream callback, handle the event and clear the line when narration resumes:

```tsx
          if (event.type === 'chunk') {
            setToolActivity(null);
            setStreamedProse((prev) => prev + (event.text ?? ''));
          } else if (event.type === 'tool') {
            setToolActivity(
              event.tool_status === 'running'
                ? `${event.tool_name}...`
                : `${event.tool_name}: ${event.tool_summary ?? 'done'}`,
            );
          } else if (event.type === 'turn' && event.turn) {
```

and clear it in the `finally` block:

```tsx
      setToolActivity(null);
```

Render it where the drafting indicator is shown (near `pendingAction`), so a tool round reads as activity rather than a stall:

```tsx
        {toolActivity && (
          <div className="text-xs font-mono text-amber-400/80 px-1 pb-1">{toolActivity}</div>
        )}
```

Place it inside the same container that renders the streamed prose or the drafting indicator, and match that container's existing classes; the exact anchor is the `pendingAction` block.

- [ ] **Step 3: Render the provenance line**

In `ChronicleView.tsx`, after the recovery block, add one quiet line:

```tsx
            {turn.tool_calls && turn.tool_calls.length > 0 && (
              <div className="text-[11px] font-mono text-stone-500 pt-1">
                Looked up: {turn.tool_calls.map((call) => `${call.name} (${call.result_chars})`).join(', ')}
              </div>
            )}
```

- [ ] **Step 4: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/App.tsx frontend/src/components/ChronicleView.tsx
git commit -m "feat(frontend): show tool activity and what a turn looked up"
```

---

## Task 8: The Settings controls

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`

**Interfaces:**
- Consumes: `agents.tool_rounds`, `agents.tool_result_chars`, `agents.roles.<role>.supports_tools`.

- [ ] **Step 1: Add the role capability control**

In the agent role editor, beside the existing provider-type control, add a select bound to `config.agents.roles[selectedRole].supports_tools`:

```tsx
                    <div className="space-y-1.5">
                      <label className="text-xs font-cinzel uppercase text-stone-300">Tool Calling</label>
                      <select
                        value={config.agents.roles[selectedRole]?.supports_tools ?? 'auto'}
                        onChange={(e) =>
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: {
                                ...config.agents.roles,
                                [selectedRole]: {
                                  ...defaultRoleConfig(selectedRole),
                                  ...config.agents.roles[selectedRole],
                                  supports_tools: e.target.value,
                                },
                              },
                            },
                          })
                        }
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
                      >
                        <option value="auto">Auto (HTTP providers only)</option>
                        <option value="yes">Yes (force)</option>
                        <option value="no">No (suppress)</option>
                      </select>
                    </div>
```

Match the surrounding role-editor markup; the anchor is the provider-type select in that pane. `AgentRoleConfig` in `frontend/src/types.ts` also needs the field:

```ts
  supports_tools?: 'auto' | 'yes' | 'no';
```

- [ ] **Step 2: Add the numeric limits**

In the media/agents limits area, beside the context-budget control, add two number inputs bound to `config.agents.tool_rounds` and `config.agents.tool_result_chars`:

```tsx
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-1.5">
                    <label className="text-xs font-cinzel uppercase text-stone-300">Tool Rounds / Turn</label>
                    <input
                      type="number"
                      min="0"
                      max="20"
                      value={config.agents.tool_rounds ?? 4}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          agents: { ...config.agents, tool_rounds: parseInt(e.target.value, 10) || 0 },
                        })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-xs font-cinzel uppercase text-stone-300">Tool Result Characters</label>
                    <input
                      type="number"
                      min="0"
                      max="50000"
                      step="500"
                      value={config.agents.tool_result_chars ?? 4000}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          agents: { ...config.agents, tool_result_chars: parseInt(e.target.value, 10) || 0 },
                        })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                    />
                  </div>
                </div>
```

Add `tool_rounds?: number;` and `tool_result_chars?: number;` to `AgentsConfig` in `frontend/src/types.ts`.

- [ ] **Step 3: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx frontend/src/types.ts
git commit -m "feat(frontend): expose tool capability and limits in Settings"
```

---

## Task 9: Full gate

**Files:** none (verification only).

- [ ] **Step 1: Run the whole suite**

Run: `go test ./... -count=1`
Expected: every package ok.

- [ ] **Step 2: Vet and build the frontend**

Run: `mise run lint` and `cd frontend && npm run build`, then restore the tracked placeholder if the build removed it: `git checkout -- pkg/gui/dist/.gitkeep`.

- [ ] **Step 3: Confirm the criteria this plan owns**

- A turn may call the model again after a tool result, bounded by `tool_rounds` (Task 3).
- Tools degrade honestly: a provider without `ToolCaller` or a `no` capability simply answers, and a rejected `tools` field degrades once (Task 2 plus the contract plan).
- Tool activity is visible while it happens and never looks like a stall (Task 5 and Task 7), and a slow tool cannot trip the watchdog because the watchdog runs per call (Scope).
- A turn cannot loop or spend without bound: rounds are capped and the budget withdraws tools with a readable refusal (Task 3).
- Provenance is compact on the turn and full in the trace (Tasks 3 and 4).

---

## Self-Review

**Spec coverage:**

- §4 bounded loop: Task 3, including prose discard, stray-call handling, re-execution (nothing caches), termination, and the sequential order.
- §6 capability: Task 2 (`ToolCaller`, `auto`/`yes`/`no`) and Task 6 (role resolution); the opening turn is included because the loop runs for every `ProcessActionStream` call, and only `gm` is given tools.
- §9 bounds: Task 3 (`tool_rounds`, budget withdrawal with a readable refusal); `tool_result_chars` was enforced in the contract plan's executor; `turn_output_tokens` stays unenforced by design and is not added.
- §10 streaming and watchdog: Task 5 (the `tool` event) and Task 7 (the activity line); the watchdog note is in Scope.
- §11 trace: Task 3 (`tool.round`, `tool.call`, `tool.result`, plus `tool.prose_discarded` and `tool.stray`); `provider.tools` landed in the contract plan.
- §12 config: the keys landed in the contract plan; Task 8 exposes them.
- §16 File Map: `pkg/engine/orchestrator.go`, `history.go`, `pkg/gui/*`, `frontend` components, and `cmd/localrpg/play.go` are all covered.

**Placeholder scan:** no "TBD"/"implement later" text; every code step carries real code.

**Type consistency:** `streamResult.ToolCalls` (Task 1) is read by the loop (Task 3). `Provenance` and `ToolCallRecord` (Tasks 3-4) feed `Turn.ToolCalls` and `ToolCallDTO` (Tasks 4-5). `ToolExecutor`, `ToolActivity`, `SetTools`, `SetToolRounds`, `SetToolObserver` (Task 2) are consumed in Tasks 3, 5, 6. `harness.ToolCaller` (Task 2) is implemented by `HTTPProvider`. `toolEvent` (Task 5) mirrors `TurnEvent`'s new fields (Task 5) read by `App.tsx` (Task 7). The Settings fields (Task 8) match `config.AgentRoleConfig.SupportsTools` and `AgentsConfig.ToolRounds`/`ToolResultChars` from the contract plan.
