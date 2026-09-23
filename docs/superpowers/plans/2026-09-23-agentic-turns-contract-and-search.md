# Agentic Turns: Contract and Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the provider contract the vocabulary a tool round needs, add FTS5 search to the index, and implement the four read-only tools.

**Architecture:** The contract is additive: `GenerateRequest` grows `Messages` and `Tools`, `StreamChunk` grows whole `ToolCalls`, and `Prompt` stays as a derived string for providers that only accept one, so every existing provider keeps working unchanged. Search is a derived FTS5 index kept in step by triggers and backfilled on open. The tools are plain functions over the store, with their schemas in one table so the offered list and the dispatcher cannot drift.

**Tech Stack:** Go 1.27.1 (standard library plus the existing `modernc.org/sqlite`), `net/http/httptest` for provider tests, existing `pkg/storage`, `pkg/harness`, `pkg/entity`.

**Spec:** `docs/superpowers/specs/2026-09-22-agentic-turns-and-tools-design.md`

## Scope

This spec is three delivery increments (its §17). This plan is **increments 1 and 2: the contract and the search**, which are testable with no model involved and touch no UI.

Deferred to the second plan, `agentic-turns-loop`:
- the bounded loop in `ProcessActionStream`, round and budget limits, tool withdrawal
- `agents.tool_rounds` (the per-turn round cap; this plan adds `agents.tool_result_chars`, which the tools need)
- the `{"type":"tool"}` stream event, its `TurnEvent` framing, the server-side watchdog reset, and the console activity line
- `Turn.ToolCalls` provenance and the chronicle line
- the `tool.round`, `tool.call`, `tool.result` trace rows (the loop emits them)
- the Settings controls for the new limits

Nothing here is user-visible on its own, which the spec anticipates: a contract with no tools, and tools with no caller.

## Global Constraints

- Standard library only for tests; `modernc.org/sqlite` is already the engine.
- Use `interface{}`, never `any`. Errors wrapped with `fmt.Errorf("...: %w", err)`. `go vet ./...` clean.
- Never write em dashes in source code.
- A provider that ignores `Messages`, `Tools`, and `ToolCalls` must behave exactly as it does today. This is the one compatibility property the whole plan rests on.
- A tool error is result text, never a failed turn.
- The tool surface is a permanent ceiling: read-only, internal, four tools, no write tool at any point.
- Conventional Commits with a scope, subject under 72 characters.
- Verification: `mise run test` and `mise run lint`. Frontend gate (nothing here changes it, but keep it green): `cd frontend && npx tsc --noEmit`.
- Single Go test example: `go test -run TestMessagesPrompt ./pkg/harness/`.

### File Map

| Action | Path | Responsibility |
| :--- | :--- | :--- |
| Modify | `pkg/harness/types.go` | `Message`, `ToolSpec`, `ToolCall`, `GenerateRequest.Messages`/`Tools`, `StreamChunk.ToolCalls`, `PromptText` |
| Modify | `pkg/harness/types_test.go` | `MessagesPrompt` and `PromptText` tests |
| Modify | `pkg/harness/http_provider.go` | `tools` field, role mapping, `tool_calls` accumulation, one-shot degradation |
| Modify | `pkg/harness/http_provider_test.go` | Fragmented accumulation and rejection-degradation tests |
| Modify | `pkg/harness/cli_provider.go`, `oracle_provider.go`, `factory.go` | Use `PromptText()`; ignore tools |
| Modify | `pkg/harness/cli_provider_test.go` (or `oracle_provider_test.go`) | A messages-only request still produces a prompt |
| Modify | `pkg/config/types.go` | `AgentRoleConfig.SupportsTools`, `AgentsConfig.ToolRounds`/`ToolResultChars`, accessors |
| Modify | `pkg/config/types_test.go` | Capability and bound defaults |
| Create | `pkg/storage/fts.go` | FTS5 tables, triggers, backfill, rebuild |
| Create | `pkg/storage/fts_test.go` | Trigger, backfill, and search tests |
| Modify | `pkg/storage/db.go` | Call `EnsureFTS` from `OpenDB` |
| Create | `pkg/harness/tools.go` | `ToolSpecs`, schemas, `UnknownToolMessage` |
| Create | `pkg/harness/tools_test.go` | Schema table and unknown-tool tests |
| Create | `pkg/tools/tools.go` | `Executor` and the four tool implementations |
| Create | `pkg/tools/query.go` | FTS query construction |
| Create | `pkg/tools/tools_test.go`, `pkg/tools/query_test.go` | Tool behaviour and query building |

---

## Task 1: The provider contract

**Files:**
- Modify: `pkg/harness/types.go`
- Modify: `pkg/harness/types_test.go`

**Interfaces:**
- Produces: `harness.Message`, `harness.ToolSpec`, `harness.ToolCall`, `GenerateRequest.Messages`/`Tools`, `StreamChunk.ToolCalls`, `GenerateRequest.PromptText()`, `harness.MessagesPrompt(messages []Message) string`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/types_test.go`:

```go
func TestMessagesPromptLabelsEveryRole(t *testing.T) {
	prompt := MessagesPrompt([]Message{
		{Role: "system", Content: "You are the GM."},
		{Role: "user", Content: "I open the gate."},
		{Role: "assistant", Content: "The hinges protest."},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "1", Name: "get_entity"}}},
		{Role: "tool", ToolCallID: "1", Content: "Aldric: a mercenary."},
	})

	for _, want := range []string{"You are the GM.", "Player: I open the gate.", "Narrator: The hinges protest.", "Tool result:", "Aldric: a mercenary."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q:\n%s", want, prompt)
		}
	}
}

func TestPromptTextPrefersAnExplicitPrompt(t *testing.T) {
	explicit := GenerateRequest{Prompt: "just this"}
	if got := explicit.PromptText(); got != "just this" {
		t.Errorf("PromptText = %q, want the explicit prompt", got)
	}

	derived := GenerateRequest{Messages: []Message{{Role: "user", Content: "hello"}}}
	if got := derived.PromptText(); !strings.Contains(got, "hello") {
		t.Errorf("PromptText = %q, want the conversation flattened", got)
	}
}
```

Add `"strings"` to the test imports if absent.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestMessagesPrompt|TestPromptText' ./pkg/harness/ -v`
Expected: FAIL with "undefined: MessagesPrompt".

- [ ] **Step 3: Implement**

In `pkg/harness/types.go`, add `"strings"` to the imports and add:

```go
// Message is one turn of the conversation a tool-capable provider is given.
// Role is "system", "user", "assistant", or "tool".
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant messages only
	ToolCallID string     `json:"tool_call_id,omitempty"` // tool messages only
}

// ToolSpec is one tool offered to a model, with its JSON Schema parameters.
type ToolSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall is a model's request to run a tool. Arguments is the raw JSON the
// model produced, because a malformed payload is the model's to fix, not ours.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// MessagesPrompt renders a conversation as one prompt for a provider that only
// accepts a string. Roles are labelled so a tool result is never mistaken for
// narration.
func MessagesPrompt(messages []Message) string {
	var sb strings.Builder
	for _, message := range messages {
		switch message.Role {
		case "system":
			sb.WriteString(message.Content)
			sb.WriteString("\n\n")
		case "user":
			sb.WriteString("Player: ")
			sb.WriteString(message.Content)
			sb.WriteString("\n")
		case "assistant":
			if strings.TrimSpace(message.Content) != "" {
				sb.WriteString("Narrator: ")
				sb.WriteString(message.Content)
				sb.WriteString("\n")
			}
		case "tool":
			sb.WriteString("Tool result:\n")
			sb.WriteString(message.Content)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String())
}
```

Extend `GenerateRequest`:

```go
	// Messages is authoritative when set. Prompt remains for providers that only
	// accept a single string.
	Messages []Message  `json:"messages,omitempty"`
	Tools    []ToolSpec `json:"tools,omitempty"`
```

Extend `StreamChunk`:

```go
	// ToolCalls carries whole calls only; a provider reassembles its own wire
	// format and never leaks fragments to the engine.
	ToolCalls []ToolCall
```

Add:

```go
// PromptText is the request as a single string: the explicit Prompt when a caller
// set one, otherwise the conversation flattened for a provider that cannot take
// messages.
func (r GenerateRequest) PromptText() string {
	if strings.TrimSpace(r.Prompt) != "" {
		return r.Prompt
	}
	return MessagesPrompt(r.Messages)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/types.go pkg/harness/types_test.go
git commit -m "feat(harness): add the tool-round vocabulary to the provider contract"
```

---

## Task 2: HTTP provider tools and streamed accumulation

**Files:**
- Modify: `pkg/harness/http_provider.go`
- Modify: `pkg/harness/http_provider_test.go`

**Interfaces:**
- Consumes: `Message`, `ToolSpec`, `ToolCall` (Task 1).
- Produces: an OpenAI-compatible `tools` request field; whole `ToolCalls` on the terminal chunk; a single retry without tools when the server rejects the field, traced as `provider.tools`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/http_provider_test.go`:

```go
func TestHTTPProviderAccumulatesStreamedToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["tools"]; !ok {
			t.Errorf("expected a tools field, got %v", body)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		// Arguments arrive split across frames, as OpenAI-compatible servers do.
		frames := []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"search_entities","arguments":"{\"que"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ry\":\"Kae"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"l\"}"}}]}}],"finish_reason":"tool_calls"}`,
		}
		for _, frame := range frames {
			fmt.Fprintf(w, "data: %s\n\n", frame)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	provider := NewHTTPProvider("gm", server.URL, "test", "")
	provider.logger = trace.Nop()

	out := make(chan StreamChunk, 20)
	var calls []ToolCall
	done := make(chan struct{})
	go func() {
		for chunk := range out {
			if len(chunk.ToolCalls) > 0 {
				calls = chunk.ToolCalls
			}
		}
		close(done)
	}()

	req := GenerateRequest{
		Messages: []Message{{Role: "user", Content: "who is Kael?"}},
		Tools:    []ToolSpec{{Name: "search_entities", Description: "search", Parameters: map[string]interface{}{"type": "object"}}},
	}
	if err := provider.Stream(context.Background(), req, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	<-done

	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want one assembled call", calls)
	}
	if calls[0].ID != "call_1" || calls[0].Name != "search_entities" {
		t.Errorf("call = %+v", calls[0])
	}
	if calls[0].Arguments != `{"query":"Kael"}` {
		t.Errorf("arguments = %q, want the reassembled JSON", calls[0].Arguments)
	}
}

func TestHTTPProviderDegradesOnceWhenToolsAreRejected(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["tools"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"unknown field tools"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Fine.\"},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	provider := NewHTTPProvider("gm", server.URL, "test", "")
	memory := trace.NewMemory(trace.LevelFull)
	provider.SetLogger(memory)

	out := make(chan StreamChunk, 20)
	go func() {
		for range out {
		}
	}()

	req := GenerateRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
		Tools:    []ToolSpec{{Name: "search_entities"}},
	}
	if err := provider.Stream(context.Background(), req, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want one retry without tools", attempts)
	}
	event, ok := memory.Find("provider.tools")
	if !ok {
		t.Fatalf("expected a provider.tools trace event")
	}
	if event.Fields["rejected"] != true {
		t.Errorf("rejected = %v, want true", event.Fields["rejected"])
	}
}
```

Match the existing test file's imports (`fmt`, `encoding/json`, `net/http`, `net/http/httptest`, `context`, `testing`, `github.com/darkliquid/localrpg/pkg/trace`) and add any that are missing.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestHTTPProviderAccumulates|TestHTTPProviderDegrades' ./pkg/harness/ -v`
Expected: FAIL: no `tools` field is sent, and no accumulation happens.

- [ ] **Step 3: Implement**

In `pkg/harness/http_provider.go`, extend the wire types:

```go
type openAIToolSpec struct {
	Type     string                  `json:"type"`
	Function openAIFunctionSpec      `json:"function"`
}

type openAIFunctionSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type openAIMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content,omitempty"`
	ToolCalls  []openAIToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
```

Add `Tools []openAIToolSpec \`json:"tools,omitempty"\`` to `openAIChatRequest`, and to `openAIChatChunk`'s delta:

```go
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
```

Add the accumulator:

```go
// toolCallAccumulator reassembles streamed tool calls. Arguments arrive split
// across frames and are identified only by index, so the engine is handed whole
// calls and never a vendor's fragment format.
type toolCallAccumulator struct {
	order []int
	calls map[int]*ToolCall
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{calls: make(map[int]*ToolCall)}
}

func (a *toolCallAccumulator) add(index int, id, name, arguments string) {
	call, ok := a.calls[index]
	if !ok {
		call = &ToolCall{}
		a.calls[index] = call
		a.order = append(a.order, index)
	}
	if id != "" {
		call.ID = id
	}
	if name != "" {
		call.Name = name
	}
	call.Arguments += arguments
}

func (a *toolCallAccumulator) result() []ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	calls := make([]ToolCall, 0, len(a.order))
	for _, index := range a.order {
		calls = append(calls, *a.calls[index])
	}
	return calls
}
```

Rewrite the request-building and stream body. Keep `Stream` as a thin wrapper that degrades once:

```go
func (h *HTTPProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	return h.streamOnce(ctx, req, out, true)
}

// streamOnce sends one request. When a server rejects the tools field it is
// retried once without it, because a rejection is a provider limitation rather
// than a turn failure; the trace records why.
func (h *HTTPProvider) streamOnce(ctx context.Context, req GenerateRequest, out chan<- StreamChunk, allowTools bool) error {
	defer close(out)

	messages := h.buildMessages(req)
	tools := h.buildTools(req, allowTools)
	...
```

`buildMessages` maps `req.Messages` when present, else `System` and `PromptText()`:

```go
func (h *HTTPProvider) buildMessages(req GenerateRequest) []openAIMessage {
	if len(req.Messages) == 0 {
		messages := make([]openAIMessage, 0, 2)
		if req.System != "" {
			messages = append(messages, openAIMessage{Role: "system", Content: req.System})
		}
		messages = append(messages, openAIMessage{Role: "user", Content: req.PromptText()})
		return messages
	}

	messages := make([]openAIMessage, 0, len(req.Messages))
	for _, message := range req.Messages {
		mapped := openAIMessage{Role: message.Role, Content: message.Content, ToolCallID: message.ToolCallID}
		for _, call := range message.ToolCalls {
			wire := openAIToolCall{ID: call.ID, Type: "function"}
			wire.Function.Name = call.Name
			wire.Function.Arguments = call.Arguments
			mapped.ToolCalls = append(mapped.ToolCalls, wire)
		}
		messages = append(messages, mapped)
	}
	return messages
}

// buildTools maps the offered tools, or returns none when tools are not allowed.
func (h *HTTPProvider) buildTools(req GenerateRequest, allowTools bool) []openAIToolSpec {
	if !allowTools || len(req.Tools) == 0 {
		return nil
	}
	tools := make([]openAIToolSpec, 0, len(req.Tools))
	for _, spec := range req.Tools {
		wire := openAIToolSpec{Type: "function"}
		wire.Function.Name = spec.Name
		wire.Function.Description = spec.Description
		wire.Function.Parameters = spec.Parameters
		tools = append(tools, wire)
	}
	return tools
}
```

In the request payload, set `Tools: tools`. Replace `req.Prompt` in the `provider.request` trace with `req.PromptText()`.

On a non-200 response, before returning, add the degradation:

```go
	if resp.StatusCode != http.StatusOK {
		h.logger.Event("provider.error", map[string]interface{}{
			"role":   h.id,
			"status": resp.Status,
			"url":    url,
		})
		if allowTools && len(req.Tools) > 0 && resp.StatusCode == http.StatusBadRequest {
			h.logger.Event("provider.tools", map[string]interface{}{
				"role":     h.id,
				"offered":  len(req.Tools),
				"rejected": true,
				"reason":   "the provider rejected the tools field",
			})
			return h.streamOnce(ctx, req, out, false)
		}
		return fmt.Errorf("http error %s from %s", resp.Status, url)
	}
```

In the scanner loop, feed the accumulator:

```go
		accumulator := newToolCallAccumulator()
		...
		if len(chunk.Choices) > 0 {
			for _, call := range chunk.Choices[0].Delta.ToolCalls {
				accumulator.add(call.Index, call.ID, call.Function.Name, call.Function.Arguments)
			}
			if chunk.Choices[0].Delta.Content != "" {
				...
			}
			if chunk.Choices[0].FinishReason != "" {
				finishReason = chunk.Choices[0].FinishReason
			}
		}
```

and the terminal chunk:

```go
	out <- StreamChunk{Done: true, FinishReason: finishReason, ToolCalls: accumulator.result()}
	return nil
```

Note: `defer close(out)` now lives in `streamOnce`, and the retry re-enters it; that is correct because the first attempt closes the channel only after returning, and the retry is a fresh stream on the same channel. Do not also close `out` in `Stream`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/harness/ -count=1`
Expected: PASS, including the existing HTTP provider tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/http_provider.go pkg/harness/http_provider_test.go
git commit -m "feat(harness): offer tools and reassemble streamed tool calls"
```

---

## Task 3: String-only providers keep working

**Files:**
- Modify: `pkg/harness/cli_provider.go`, `pkg/harness/oracle_provider.go`, `pkg/harness/factory.go`
- Test: `pkg/harness/cli_provider_test.go` (or `oracle_provider_test.go`)

**Interfaces:**
- Consumes: `GenerateRequest.PromptText()` (Task 1).
- Produces: no behaviour change for a request that only sets `Messages`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/cli_provider_test.go`:

```go
func TestCLIProviderUsesMessagesAsAPrompt(t *testing.T) {
	provider := NewCLIProvider("gm", "sh", []string{"-c", "echo $1", "--"})
	out := make(chan StreamChunk, 20)

	req := GenerateRequest{Messages: []Message{{Role: "user", Content: "hello from messages"}}}
	if err := provider.Stream(context.Background(), req, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var text strings.Builder
	for chunk := range out {
		text.WriteString(chunk.Text)
	}
	if !strings.Contains(text.String(), "hello from messages") {
		t.Errorf("output = %q, want the flattened conversation", text.String())
	}
}
```

This follows the file's existing `sh -c ... --` pattern, so it works anywhere the current CLI tests do.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestCLIProviderUsesMessages ./pkg/harness/ -v`
Expected: FAIL because the provider reads `req.Prompt`, which is empty.

- [ ] **Step 3: Implement**

In `pkg/harness/cli_provider.go`, `pkg/harness/oracle_provider.go`, and the echo provider in `pkg/harness/factory.go`, replace `req.Prompt` with `req.PromptText()`. Find every use with `grep -rn "req.Prompt" pkg/harness/` and update each one that is a provider reading a request; leave the HTTP provider's `buildMessages` fallback as it is (it already uses `PromptText`).

The CLI and oracle providers ignore `req.Tools` entirely, which is the compatibility property: a provider that cannot call tools simply produces its normal reply.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/cli_provider.go pkg/harness/oracle_provider.go pkg/harness/factory.go pkg/harness/cli_provider_test.go
git commit -m "feat(harness): derive a prompt for providers that take only a string"
```

(Adjust the test file to wherever the test landed.)

---

## Task 4: Capability and tool bounds in configuration

**Files:**
- Modify: `pkg/config/types.go`
- Modify: `pkg/config/types_test.go`

**Interfaces:**
- Produces: `config.AgentRoleConfig.SupportsTools`, `config.AgentsConfig.ToolRounds`/`ToolResultChars`, `(*Config).RoleSupportsTools(role string) string`, `(*Config).ToolRounds() int`, `(*Config).ToolResultChars() int`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/config/types_test.go`:

```go
func TestToolCapabilityAndBounds(t *testing.T) {
	empty := &Config{}

	if got := empty.RoleSupportsTools("gm"); got != "auto" {
		t.Errorf("RoleSupportsTools = %q, want auto", got)
	}
	if got := empty.ToolRounds(); got != 4 {
		t.Errorf("ToolRounds = %d, want 4", got)
	}
	if got := empty.ToolResultChars(); got != 4000 {
		t.Errorf("ToolResultChars = %d, want 4000", got)
	}

	configured := &Config{Agents: AgentsConfig{
		Roles: map[string]AgentRoleConfig{"gm": {Type: "http", SupportsTools: "no"}},
	}}
	if got := configured.RoleSupportsTools("gm"); got != "no" {
		t.Errorf("RoleSupportsTools = %q, want the configured no", got)
	}

	// An unrecognised value falls back to auto rather than silently disabling.
	unknown := &Config{Agents: AgentsConfig{Roles: map[string]AgentRoleConfig{"gm": {SupportsTools: "banana"}}}}
	if got := unknown.RoleSupportsTools("gm"); got != "auto" {
		t.Errorf("RoleSupportsTools = %q, want auto for an unknown value", got)
	}

	bounded := &Config{Agents: AgentsConfig{ToolRounds: 2, ToolResultChars: 500}}
	if got := bounded.ToolRounds(); got != 2 {
		t.Errorf("ToolRounds = %d, want 2", got)
	}
	if got := bounded.ToolResultChars(); got != 500 {
		t.Errorf("ToolResultChars = %d, want 500", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestToolCapabilityAndBounds ./pkg/config/ -v`
Expected: FAIL to compile with "unknown field SupportsTools".

- [ ] **Step 3: Implement**

In `pkg/config/types.go`, add to `AgentRoleConfig`:

```go
	// SupportsTools is "auto", "yes", or "no". Empty means auto: an HTTP provider
	// gets tools and every other type does not.
	SupportsTools string `yaml:"supports_tools,omitempty" json:"supports_tools,omitempty"`
```

Add to `AgentsConfig`:

```go
	// ToolRounds caps how many times a turn may call tools before tools are
	// withdrawn. Zero means the default of four.
	ToolRounds int `yaml:"tool_rounds" json:"tool_rounds"`
	// ToolResultChars caps one tool result. Zero means the default of 4000.
	ToolResultChars int `yaml:"tool_result_chars" json:"tool_result_chars"`
```

Add accessors:

```go
// RoleSupportsTools is the tool capability for a role: "auto", "yes", or "no".
func (c *Config) RoleSupportsTools(role string) string {
	value := strings.ToLower(strings.TrimSpace(c.Agents.Roles[role].SupportsTools))
	switch value {
	case "yes", "no", "auto":
		return value
	default:
		return "auto"
	}
}

// ToolRounds caps how many times a turn may call tools.
func (c *Config) ToolRounds() int {
	if c.Agents.ToolRounds <= 0 {
		return 4
	}
	return c.Agents.ToolRounds
}

// ToolResultChars caps one tool result, so a broad query cannot flood the prompt.
func (c *Config) ToolResultChars() int {
	if c.Agents.ToolResultChars <= 0 {
		return 4000
	}
	return c.Agents.ToolResultChars
}
```

Add the keys to `DefaultConfig`'s `Agents` literal so a fresh config shows them:

```go
			ToolRounds:          4,
			ToolResultChars:     4000,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go
git commit -m "feat(config): declare tool capability and result bounds"
```

---

## Task 5: The FTS5 index

**Files:**
- Create: `pkg/storage/fts.go`
- Create: `pkg/storage/fts_test.go`
- Modify: `pkg/storage/db.go`

**Interfaces:**
- Produces: `storage.EnsureFTS(db *sql.DB) error`, which creates the FTS tables and triggers, mirrors any rows the index is missing, and removes rows whose content is gone.

**Design note (a deviation worth stating):** the spec asks for FTS5 over `entities(name, body, tags)`. Entity tags live inside `frontmatter_json`, and an FTS5 *external-content* table can only index columns that exist on the content table, so `tags` cannot be a column of an external-content table. This plan uses plain FTS5 tables keyed by the content table's `rowid` and kept in step by triggers, which is the same join and the same migration story. It is called out here because it is a real difference from the spec's wording.

- [ ] **Step 1: Write the failing test**

Create `pkg/storage/fts_test.go`:

```go
package storage

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestFTSTracksEntityWrites(t *testing.T) {
	store := openTestDB(t)

	warden := &entity.Entity{ID: "warden", Name: "The Warden", Type: "character", Body: "A grim warden of the eastern gate.", Tags: []string{"guard"}}
	if err := store.SaveEntity(warden); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}

	hits, err := store.SearchEntities("warden", "", 10)
	if err != nil {
		t.Fatalf("SearchEntities: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "warden" {
		t.Fatalf("hits = %+v, want the warden", hits)
	}

	// Porter stemming makes an inflected query find the same note.
	if hits, err := store.SearchEntities("wardens", "", 10); err != nil || len(hits) != 1 {
		t.Errorf("stemmed query hits = %+v, err = %v", hits, err)
	}

	// An update is reflected, not duplicated.
	warden.Body = "A kindly warden of the western gate."
	if err := store.SaveEntity(warden); err != nil {
		t.Fatalf("SaveEntity update: %v", err)
	}
	if hits, err := store.SearchEntities("kindly", "", 10); err != nil || len(hits) != 1 {
		t.Errorf("after update hits = %+v, err = %v", hits, err)
	}
	if hits, _ := store.SearchEntities("grim", "", 10); len(hits) != 0 {
		t.Errorf("the old body should no longer match, got %+v", hits)
	}

	if err := store.DeleteEntity("warden"); err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}
	if hits, _ := store.SearchEntities("warden", "", 10); len(hits) != 0 {
		t.Errorf("a deleted entity must not be searchable, got %+v", hits)
	}
}

func TestFSTSearchesTurnProse(t *testing.T) {
	store := openTestDB(t)
	if err := store.SaveTurn(TurnRecord{Number: 1, Mode: "Do", Input: "I look around", Narration: "The guttered lanterns flicker."}); err != nil {
		t.Fatalf("SaveTurn: %v", err)
	}

	hits, err := store.SearchTurns("guttering", "", 10)
	if err != nil {
		t.Fatalf("SearchTurns: %v", err)
	}
	if len(hits) != 1 || hits[0].Number != 1 {
		t.Fatalf("hits = %+v, want turn 1", hits)
	}
}

func TestEnsureFTSBackfillsAnExistingIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	// A database written before FTS5 existed: schema only, no virtual tables.
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.db.Exec(`DROP TABLE IF EXISTS entities_fts`); err != nil {
		t.Fatalf("drop fts: %v", err)
	}
	if _, err := store.db.Exec(`DROP TABLE IF EXISTS turns_fts`); err != nil {
		t.Fatalf("drop fts: %v", err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "old", Name: "Old Note", Type: "lore", Body: "An ancient bridge."}); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}
	_ = store.Close()

	// Reopening must restore search without rescanning Markdown.
	reopened, err := NewStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	hits, err := reopened.SearchEntities("ancient", "", 10)
	if err != nil {
		t.Fatalf("SearchEntities after backfill: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "old" {
		t.Errorf("hits = %+v, want the backfilled note", hits)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestFTS|TestEnsureFTS' ./pkg/storage/ -v`
Expected: FAIL with "store.SearchEntities undefined".

- [ ] **Step 3: Implement the FTS tables**

Create `pkg/storage/fts.go`:

```go
package storage

import (
	"database/sql"
	"fmt"
)

// ftsSchema creates the derived search tables and the triggers that keep them in
// step with the content tables. Tags are read out of the entity frontmatter JSON,
// which is why these are plain FTS5 tables keyed by rowid rather than
// external-content tables: an external-content table can only index columns that
// exist on its content table. The tokenizer is porter, so model-written prose
// matches on inflection.
const ftsSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS entities_fts USING fts5(
    name, body, tags, tokenize='porter'
);

CREATE VIRTUAL TABLE IF NOT EXISTS turns_fts USING fts5(
    input, narration, tokenize='porter'
);

CREATE TRIGGER IF NOT EXISTS entities_fts_insert AFTER INSERT ON entities BEGIN
    INSERT INTO entities_fts(rowid, name, body, tags)
    VALUES (new.rowid, new.name, new.body, coalesce(json_extract(new.frontmatter_json, '$.tags'), ''));
END;

CREATE TRIGGER IF NOT EXISTS entities_fts_update AFTER UPDATE ON entities BEGIN
    DELETE FROM entities_fts WHERE rowid = old.rowid;
    INSERT INTO entities_fts(rowid, name, body, tags)
    VALUES (new.rowid, new.name, new.body, coalesce(json_extract(new.frontmatter_json, '$.tags'), ''));
END;

CREATE TRIGGER IF NOT EXISTS entities_fts_delete AFTER DELETE ON entities BEGIN
    DELETE FROM entities_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS turns_fts_insert AFTER INSERT ON turns BEGIN
    INSERT INTO turns_fts(rowid, input, narration)
    VALUES (new.rowid, new.input, new.narration);
END;

CREATE TRIGGER IF NOT EXISTS turns_fts_update AFTER UPDATE ON turns BEGIN
    DELETE FROM turns_fts WHERE rowid = old.rowid;
    INSERT INTO turns_fts(rowid, input, narration)
    VALUES (new.rowid, new.input, new.narration);
END;

CREATE TRIGGER IF NOT EXISTS turns_fts_delete AFTER DELETE ON turns BEGIN
    DELETE FROM turns_fts WHERE rowid = old.rowid;
END;
`

// EnsureFTS makes the search tables agree with the content tables. It is
// idempotent and runs on every open: rows the index is missing are inserted from
// the tables, and rows whose content is gone are removed, so an index written
// before FTS5 existed is repaired without re-reading any Markdown.
func EnsureFTS(db *sql.DB) error {
	if _, err := db.Exec(ftsSchema); err != nil {
		return fmt.Errorf("apply fts schema: %w", err)
	}

	backfill := []string{
		`INSERT INTO entities_fts(rowid, name, body, tags)
		 SELECT e.rowid, e.name, e.body, coalesce(json_extract(e.frontmatter_json, '$.tags'), '')
		 FROM entities e
		 WHERE e.rowid NOT IN (SELECT rowid FROM entities_fts)`,
		`DELETE FROM entities_fts
		 WHERE rowid NOT IN (SELECT rowid FROM entities)`,
		`INSERT INTO turns_fts(rowid, input, narration)
		 SELECT t.rowid, t.input, t.narration
		 FROM turns t
		 WHERE t.rowid NOT IN (SELECT rowid FROM turns_fts)`,
		`DELETE FROM turns_fts
		 WHERE rowid NOT IN (SELECT rowid FROM turns)`,
	}
	for _, statement := range backfill {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("backfill fts: %w", err)
		}
	}
	return nil
}

// SearchEntityHit is one entity match, with a snippet that shows why it matched.
type SearchEntityHit struct {
	ID      string
	Name    string
	Type    string
	Snippet string
}

// SearchTurnHit is one turn match, with a narration snippet.
type SearchTurnHit struct {
	Number  int
	Snippet string
}

// SearchEntities runs an FTS5 MATCH expression. The expression is built by the
// caller, because building it from a model's words is a tool concern.
func (s *Store) SearchEntities(match, entityType string, limit int) ([]SearchEntityHit, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT e.id, e.name, e.type, snippet(entities_fts, 1, '[', ']', '...', 12)
		FROM entities_fts
		JOIN entities e ON e.rowid = entities_fts.rowid
		WHERE entities_fts MATCH ?
		  AND (? = '' OR e.type = ?)
		ORDER BY bm25(entities_fts)
		LIMIT ?`, match, entityType, entityType, limit)
	if err != nil {
		return nil, fmt.Errorf("search entities: %w", err)
	}
	defer rows.Close()

	hits := make([]SearchEntityHit, 0)
	for rows.Next() {
		var hit SearchEntityHit
		if err := rows.Scan(&hit.ID, &hit.Name, &hit.Type, &hit.Snippet); err != nil {
			return nil, fmt.Errorf("scan entity hit: %w", err)
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

// SearchTurns runs an FTS5 MATCH expression over turn prose, optionally limited
// to turns that mention one entity.
func (s *Store) SearchTurns(match, entityID string, limit int) ([]SearchTurnHit, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT t.number, snippet(turns_fts, 1, '[', ']', '...', 12)
		FROM turns_fts
		JOIN turns t ON t.rowid = turns_fts.rowid
		WHERE turns_fts MATCH ?
		  AND (? = '' OR EXISTS (
		      SELECT 1 FROM turn_entities te
		      WHERE te.turn_number = t.number AND te.entity_id = ?
		  ))
		ORDER BY bm25(turns_fts)
		LIMIT ?`, match, entityID, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("search turns: %w", err)
	}
	defer rows.Close()

	hits := make([]SearchTurnHit, 0)
	for rows.Next() {
		var hit SearchTurnHit
		if err := rows.Scan(&hit.Number, &hit.Snippet); err != nil {
			return nil, fmt.Errorf("scan turn hit: %w", err)
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}
```

In `pkg/storage/db.go`, call it after the existing migration:

```go
	if err := EnsureFTS(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure fts index: %w", err)
	}
```

`storage.TurnRecord`'s fields are `Number`, `Timestamp`, `Mode`, `Input`, `Narration`, `Location`, `Outcome`, `RollJSON`, and `Entities` (`pkg/storage/turn.go`); `SaveTurn` upserts by `Number`, so an update fires the update trigger rather than the insert one.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/storage/ -count=1`
Expected: PASS, including the existing migration test.

- [ ] **Step 5: Commit**

```bash
git add pkg/storage/fts.go pkg/storage/fts_test.go pkg/storage/db.go
git commit -m "feat(storage): add a derived FTS5 index kept in step by triggers"
```

---

## Task 6: The tool table and schemas

**Files:**
- Create: `pkg/harness/tools.go`
- Create: `pkg/harness/tools_test.go`

**Interfaces:**
- Consumes: `ToolSpec` (Task 1).
- Produces: `harness.ToolSpecs() []ToolSpec`, `harness.ToolNames() []string`, `harness.UnknownToolMessage(name string) string`.

- [ ] **Step 1: Write the failing test**

Create `pkg/harness/tools_test.go`:

```go
package harness

import (
	"strings"
	"testing"
)

func TestToolSpecsDescribeEveryTool(t *testing.T) {
	specs := ToolSpecs()
	names := make(map[string]ToolSpec, len(specs))
	for _, spec := range specs {
		names[spec.Name] = spec
		if spec.Description == "" {
			t.Errorf("%s has no description", spec.Name)
		}
		if spec.Parameters["type"] != "object" {
			t.Errorf("%s parameters are not a JSON object schema", spec.Name)
		}
	}

	for _, want := range []string{"search_entities", "get_entity", "graph_neighbours", "search_timeline"} {
		if _, ok := names[want]; !ok {
			t.Errorf("tool table is missing %q", want)
		}
	}

	search := names["search_entities"]
	required, _ := search.Parameters["required"].([]string)
	if len(required) == 0 || required[0] != "query" {
		t.Errorf("search_entities required = %v, want query", required)
	}
}

func TestUnknownToolMessageListsTheSurface(t *testing.T) {
	message := UnknownToolMessage("teleport")
	if !strings.Contains(message, "teleport") {
		t.Errorf("message = %q, want the offending name", message)
	}
	for _, want := range ToolNames() {
		if !strings.Contains(message, want) {
			t.Errorf("message is missing the available tool %q: %s", want, message)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestToolSpecs|TestUnknownTool' ./pkg/harness/ -v`
Expected: FAIL with "undefined: ToolSpecs".

- [ ] **Step 3: Implement**

Create `pkg/harness/tools.go`:

```go
package harness

import (
	"fmt"
	"strings"
)

// stringProperty is one JSON Schema string parameter.
func stringProperty(description string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": description}
}

// intProperty is one JSON Schema integer parameter.
func intProperty(description string) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "description": description}
}

// objectSchema is a JSON Schema object with the given required keys.
func objectSchema(properties map[string]interface{}, required ...string) map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

// ToolSpecs is the whole tool surface. It is deliberately one table, so the list
// offered to a model, its documentation, and the dispatcher cannot drift apart.
// The surface is a permanent ceiling: read-only, internal, and never a shell,
// filesystem, network, or code-execution tool.
func ToolSpecs() []ToolSpec {
	return []ToolSpec{
		{
			Name: "search_entities",
			Description: "Search the campaign's entities by words. Returns matching ids, names, types, and a body snippet.",
			Parameters: objectSchema(map[string]interface{}{
				"query": stringProperty("Words to search for, for example 'warden eastern gate'."),
				"type":  stringProperty("Optional entity type filter, for example 'character' or 'location'."),
				"limit": intProperty("Maximum matches to return. Defaults to 10."),
				"match": stringProperty("Optional raw FTS5 MATCH expression, for callers who know the syntax."),
			}, "query"),
		},
		{
			Name: "get_entity",
			Description: "Read one entity by id or name. Returns its frontmatter, state, and the start of its note.",
			Parameters: objectSchema(map[string]interface{}{
				"id_or_name": stringProperty("The entity's id or its display name."),
			}, "id_or_name"),
		},
		{
			Name: "graph_neighbours",
			Description: "List the entities connected to one entity, and the relation that connects them.",
			Parameters: objectSchema(map[string]interface{}{
				"id":        stringProperty("The entity's id."),
				"direction": stringProperty("Optional: 'from', 'to', or 'both'. Defaults to both."),
				"limit":     intProperty("Maximum neighbours to return. Defaults to 20."),
			}, "id"),
		},
		{
			Name: "search_timeline",
			Description: "Search the campaign's past turns by words. Returns turn numbers with a narration snippet.",
			Parameters: objectSchema(map[string]interface{}{
				"query":  stringProperty("Words to search for in past narration and player input."),
				"entity": stringProperty("Optional entity id to restrict the search to turns mentioning it."),
				"limit":  intProperty("Maximum matches to return. Defaults to 10."),
				"match":  stringProperty("Optional raw FTS5 MATCH expression, for callers who know the syntax."),
			}, "query"),
		},
	}
}

// ToolNames is the surface's names, in the order it is offered.
func ToolNames() []string {
	specs := ToolSpecs()
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return names
}

// UnknownToolMessage is the readable result for an unknown or hallucinated tool,
// so the model can correct itself rather than retry the same call.
func UnknownToolMessage(name string) string {
	return fmt.Sprintf("error: unknown tool %q. Available tools: %s.", name, strings.Join(ToolNames(), ", "))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestToolSpecs|TestUnknownTool' ./pkg/harness/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/tools.go pkg/harness/tools_test.go
git commit -m "feat(harness): declare the read-only tool surface"
```

---

## Task 7: FTS query construction

**Files:**
- Create: `pkg/tools/query.go`
- Create: `pkg/tools/query_test.go`

**Interfaces:**
- Produces: `tools.BuildMatch(query string) string`.

- [ ] **Step 1: Write the failing test**

Create `pkg/tools/query_test.go`:

```go
package tools

import "testing"

func TestBuildMatch(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"single word", "warden", `"warden"*`},
		{"two words", "guard kael", `"guard" AND "kael"*`},
		{"punctuation is stripped", "kael's oath", `"kaels" AND "oath"*`},
		{"a bare operator cannot break the query", "guard AND", `"guard" AND "AND"*`},
		{"NEAR is quoted like any word", "NEAR(gate", `"NEARgate"*`},
		{"extra whitespace collapses", "  ancient   bridge  ", `"ancient" AND "bridge"*`},
		{"empty is empty", "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildMatch(tc.query); got != tc.want {
				t.Errorf("BuildMatch(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestBuildMatch ./pkg/tools/ -v`
Expected: FAIL with "no Go files" or "undefined: BuildMatch".

- [ ] **Step 3: Implement**

Create `pkg/tools/query.go`:

```go
package tools

import "strings"

// BuildMatch turns a model's words into an FTS5 MATCH expression that cannot be a
// syntax error. Terms are stripped of FTS punctuation, quoted, joined with AND,
// and the last is given a prefix star so 'guard kae' finds 'Guard Kael'. FTS5's
// own grammar is unreachable from here on purpose: 'kael's oath' and 'guard AND'
// are both errors a model can neither see nor fix.
func BuildMatch(query string) string {
	words := strings.Fields(query)
	terms := make([]string, 0, len(words))
	for _, word := range words {
		cleaned := stripFTSPunctuation(word)
		if cleaned == "" {
			continue
		}
		terms = append(terms, `"`+cleaned+`"`)
	}
	if len(terms) == 0 {
		return ""
	}
	last := len(terms) - 1
	terms[last] = terms[last] + "*"
	return strings.Join(terms, " AND ")
}

// stripFTSPunctuation removes everything FTS5 gives meaning to, leaving letters,
// digits, and hyphens.
func stripFTSPunctuation(word string) string {
	var sb strings.Builder
	for _, r := range word {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			sb.WriteRune(r)
		default:
			continue
		}
	}
	return sb.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestBuildMatch ./pkg/tools/ -v`
Expected: PASS. Note `"kael's oath"` becomes `"kaels"`, which is what the test asserts.

- [ ] **Step 5: Commit**

```bash
git add pkg/tools/query.go pkg/tools/query_test.go
git commit -m "feat(tools): build FTS queries that cannot break the grammar"
```

---

## Task 8: The four tools

**Files:**
- Create: `pkg/tools/tools.go`
- Create: `pkg/tools/tools_test.go`

**Interfaces:**
- Consumes: `harness.ToolCall`, `harness.ToolSpecs`, `harness.UnknownToolMessage`, `storage.Store`, `tools.BuildMatch`.
- Produces: `tools.Executor`, `tools.NewExecutor(store *storage.Store, maxResultChars int) *Executor`, `(*Executor).Execute(ctx context.Context, call harness.ToolCall) (result string, ok bool)`.

- [ ] **Step 1: Write the failing test**

Create `pkg/tools/tools_test.go`:

```go
package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	warden := &entity.Entity{
		ID: "warden", Name: "The Warden", Type: "character", Body: "A grim warden of the eastern gate.",
		Location: "eastern-gate", Wikilinks: []string{"eastern-gate"},
	}
	gate := &entity.Entity{ID: "eastern-gate", Name: "Eastern Gate", Type: "location", Body: "Iron-bound and old."}
	for _, ent := range []*entity.Entity{warden, gate} {
		if err := store.SaveEntity(ent); err != nil {
			t.Fatalf("SaveEntity(%s): %v", ent.ID, err)
		}
	}
	return NewExecutor(store, 4000)
}

func call(name, arguments string) harness.ToolCall {
	return harness.ToolCall{ID: "1", Name: name, Arguments: arguments}
}

func TestSearchEntitiesTool(t *testing.T) {
	executor := newTestExecutor(t)

	result, ok := executor.Execute(context.Background(), call("search_entities", `{"query":"warden"}`))
	if !ok {
		t.Fatalf("Execute reported failure: %s", result)
	}
	if !strings.Contains(result, "warden") || !strings.Contains(result, "The Warden") {
		t.Errorf("result = %s", result)
	}

	// A type filter that excludes the match returns an honest empty result.
	result, ok = executor.Execute(context.Background(), call("search_entities", `{"query":"warden","type":"location"}`))
	if !ok || !strings.Contains(result, "No entities") {
		t.Errorf("filtered result = %q, ok = %v", result, ok)
	}
}

func TestGetEntityTool(t *testing.T) {
	executor := newTestExecutor(t)

	result, ok := executor.Execute(context.Background(), call("get_entity", `{"id_or_name":"The Warden"}`))
	if !ok || !strings.Contains(result, "A grim warden") {
		t.Errorf("result = %q, ok = %v", result, ok)
	}

	result, ok = executor.Execute(context.Background(), call("get_entity", `{"id_or_name":"Nobody"}`))
	if ok || !strings.Contains(result, "error:") {
		t.Errorf("a missing entity must be a readable error, got %q, ok = %v", result, ok)
	}
}

func TestGraphNeighboursTool(t *testing.T) {
	executor := newTestExecutor(t)

	result, ok := executor.Execute(context.Background(), call("graph_neighbours", `{"id":"warden"}`))
	if !ok || !strings.Contains(result, "eastern-gate") {
		t.Errorf("result = %q, ok = %v", result, ok)
	}
}

func TestSearchTimelineTool(t *testing.T) {
	executor := newTestExecutor(t)
	if err := executor.store.SaveTurn(storage.TurnRecord{Number: 1, Mode: "Do", Input: "I run", Narration: "The guttered lantern flared."}); err != nil {
		t.Fatalf("SaveTurn: %v", err)
	}

	result, ok := executor.Execute(context.Background(), call("search_timeline", `{"query":"guttering"}`))
	if !ok || !strings.Contains(result, "turn 1") {
		t.Errorf("result = %q, ok = %v", result, ok)
	}
}

func TestToolErrorsAreReadableResults(t *testing.T) {
	executor := newTestExecutor(t)

	// Unknown tool.
	result, ok := executor.Execute(context.Background(), call("teleport", `{}`))
	if ok || !strings.Contains(result, "search_entities") {
		t.Errorf("unknown tool result = %q, ok = %v", result, ok)
	}

	// Malformed arguments.
	result, ok = executor.Execute(context.Background(), call("search_entities", `{not json`))
	if ok || !strings.Contains(result, "error:") {
		t.Errorf("malformed arguments result = %q, ok = %v", result, ok)
	}

	// Missing required argument.
	result, ok = executor.Execute(context.Background(), call("search_entities", `{}`))
	if ok || !strings.Contains(result, "query") {
		t.Errorf("missing argument result = %q, ok = %v", result, ok)
	}
}

func TestToolResultsAreCapped(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	long := strings.Repeat("the ancient bridge ", 200)
	if err := store.SaveEntity(&entity.Entity{ID: "long", Name: "Long Note", Type: "lore", Body: long}); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}

	executor := NewExecutor(store, 200)
	result, ok := executor.Execute(context.Background(), call("get_entity", `{"id_or_name":"long"}`))
	if !ok {
		t.Fatalf("Execute reported failure: %s", result)
	}
	if len([]rune(result)) > 240 {
		t.Errorf("result is %d runes, want it capped near 200", len([]rune(result)))
	}
	if !strings.Contains(result, "truncated") {
		t.Errorf("a cap that bites must say so: %s", result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestSearchEntitiesTool|TestGetEntityTool|TestGraphNeighboursTool|TestSearchTimelineTool|TestToolErrors|TestToolResults' ./pkg/tools/ -v`
Expected: FAIL with "undefined: NewExecutor".

- [ ] **Step 3: Implement**

Create `pkg/tools/tools.go`:

```go
// Package tools implements the read-only tools the GM may call mid-turn. Every
// tool reads the campaign's own index and store; none of them can write.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// defaultLimit caps a modest tool result when the model does not ask for a limit.
const defaultLimit = 10

// Executor runs tool calls against one campaign's index.
type Executor struct {
	store    *storage.Store
	maxChars int
}

// NewExecutor builds an executor. maxChars is agents.tool_result_chars; a
// non-positive value falls back to 4000.
func NewExecutor(store *storage.Store, maxChars int) *Executor {
	if maxChars <= 0 {
		maxChars = 4000
	}
	return &Executor{store: store, maxChars: maxChars}
}

// Execute runs one call and returns the text the model will read. ok is false
// when the call failed, but the result is still a readable message: a tool error
// must never become a failed turn.
func (e *Executor) Execute(ctx context.Context, call harness.ToolCall) (string, bool) {
	arguments := map[string]interface{}{}
	if strings.TrimSpace(call.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &arguments); err != nil {
			return e.cap(fmt.Sprintf("error: could not parse the arguments for %s: %v", call.Name, err)), false
		}
	}

	switch call.Name {
	case "search_entities":
		return e.searchEntities(arguments)
	case "get_entity":
		return e.getEntity(arguments)
	case "graph_neighbours":
		return e.graphNeighbours(arguments)
	case "search_timeline":
		return e.searchTimeline(arguments)
	default:
		return e.cap(harness.UnknownToolMessage(call.Name)), false
	}
}

func (e *Executor) searchEntities(arguments map[string]interface{}) (string, bool) {
	match := stringArgument(arguments, "match")
	if match == "" {
		match = BuildMatch(stringArgument(arguments, "query"))
	}
	if match == "" {
		return "error: search_entities needs a query", false
	}

	hits, err := e.store.SearchEntities(match, stringArgument(arguments, "type"), intArgument(arguments, "limit", defaultLimit))
	if err != nil {
		return e.cap(fmt.Sprintf("error: search_entities failed: %v", err)), false
	}
	if len(hits) == 0 {
		return "No entities matched.", true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d matching entities:\n", len(hits))
	for _, hit := range hits {
		fmt.Fprintf(&sb, "- %s (%s, id %s): %s\n", hit.Name, hit.Type, hit.ID, hit.Snippet)
	}
	return e.cap(sb.String()), true
}

func (e *Executor) getEntity(arguments map[string]interface{}) (string, bool) {
	ref := stringArgument(arguments, "id_or_name")
	if ref == "" {
		return "error: get_entity needs id_or_name", false
	}

	ent, err := e.findEntity(ref)
	if err != nil {
		return e.cap(fmt.Sprintf("error: %v", err)), false
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%s, id %s)\n", ent.Name, ent.Type, ent.ID)
	if ent.Location != "" {
		fmt.Fprintf(&sb, "location: %s\n", ent.Location)
	}
	if ent.Faction != "" {
		fmt.Fprintf(&sb, "faction: %s\n", ent.Faction)
	}
	if len(ent.Aliases) > 0 {
		fmt.Fprintf(&sb, "aliases: %s\n", strings.Join(ent.Aliases, ", "))
	}
	if ent.State != nil {
		if raw, err := json.Marshal(ent.State.Raw()); err == nil {
			fmt.Fprintf(&sb, "state: %s\n", raw)
		}
	}
	sb.WriteString("\n")
	sb.WriteString(ent.Body)
	return e.cap(sb.String()), true
}

// findEntity resolves an exact id first, then a case-insensitive name.
func (e *Executor) findEntity(ref string) (*entity.Entity, error) {
	if ent, err := e.store.GetEntity(ref); err == nil && ent != nil {
		return ent, nil
	}
	summaries, err := e.store.ListEntities()
	if err != nil {
		return nil, err
	}
	for _, summary := range summaries {
		if strings.EqualFold(summary.Name, ref) || strings.EqualFold(summary.ID, ref) {
			return e.store.GetEntity(summary.ID)
		}
	}
	return nil, fmt.Errorf("no entity matching %q", ref)
}

func (e *Executor) graphNeighbours(arguments map[string]interface{}) (string, bool) {
	id := stringArgument(arguments, "id")
	if id == "" {
		return "error: graph_neighbours needs an id", false
	}
	direction := strings.ToLower(stringArgument(arguments, "direction"))
	if direction == "" {
		direction = "both"
	}
	limit := intArgument(arguments, "limit", 20)

	lines := make([]string, 0)
	if direction == "from" || direction == "both" {
		edges, err := e.store.GetEdgesFrom(id)
		if err != nil {
			return e.cap(fmt.Sprintf("error: graph_neighbours failed: %v", err)), false
		}
		for _, edge := range edges {
			lines = append(lines, fmt.Sprintf("- %s -> %s (%s)", id, edge.TargetID, edge.Relation))
		}
	}
	if direction == "to" || direction == "both" {
		edges, err := e.store.GetEdgesTo(id)
		if err != nil {
			return e.cap(fmt.Sprintf("error: graph_neighbours failed: %v", err)), false
		}
		for _, edge := range edges {
			lines = append(lines, fmt.Sprintf("- %s <- %s (%s)", id, edge.SourceID, edge.Relation))
		}
	}

	if len(lines) == 0 {
		return fmt.Sprintf("No neighbours for %q.", id), true
	}
	sort.Strings(lines)
	if len(lines) > limit {
		lines = lines[:limit]
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d neighbours of %s:\n", len(lines), id)
	sb.WriteString(strings.Join(lines, "\n"))
	return e.cap(sb.String()), true
}

func (e *Executor) searchTimeline(arguments map[string]interface{}) (string, bool) {
	match := stringArgument(arguments, "match")
	if match == "" {
		match = BuildMatch(stringArgument(arguments, "query"))
	}
	if match == "" {
		return "error: search_timeline needs a query", false
	}

	hits, err := e.store.SearchTurns(match, stringArgument(arguments, "entity"), intArgument(arguments, "limit", defaultLimit))
	if err != nil {
		return e.cap(fmt.Sprintf("error: search_timeline failed: %v", err)), false
	}
	if len(hits) == 0 {
		return "No turns matched.", true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d matching turns:\n", len(hits))
	for _, hit := range hits {
		fmt.Fprintf(&sb, "- turn %d: %s\n", hit.Number, hit.Snippet)
	}
	return e.cap(sb.String()), true
}

// cap truncates a result and says so, because a model that cannot tell a capped
// result from a small world will conclude the world is small.
func (e *Executor) cap(text string) string {
	runes := []rune(text)
	if len(runes) <= e.maxChars {
		return text
	}
	return string(runes[:e.maxChars]) + fmt.Sprintf("\n... (truncated at %d characters; narrow the query to see more)", e.maxChars)
}

func stringArgument(arguments map[string]interface{}, key string) string {
	value, _ := arguments[key].(string)
	return strings.TrimSpace(value)
}

func intArgument(arguments map[string]interface{}, key string, fallback int) int {
	switch value := arguments[key].(type) {
	case float64:
		if value > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	}
	return fallback
}
```

The test reaches `executor.store`, so the field stays unexported and the test lives in package `tools`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/tools/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/tools/tools.go pkg/tools/tools_test.go
git commit -m "feat(tools): implement the four read-only campaign tools"
```

---

## Task 9: Full gate

**Files:** none (verification only).

- [ ] **Step 1: Run the whole suite**

Run: `go test ./... -count=1`
Expected: every package ok, including every pre-existing provider and storage test, which is the compatibility check for a contract change.

- [ ] **Step 2: Vet and typecheck**

Run: `mise run lint` and `cd frontend && npx tsc --noEmit`
Expected: `go vet ./...` clean and no TypeScript errors.

- [ ] **Step 3: Confirm the increment's properties**

- A provider that ignores `Messages`, `Tools`, and `ToolCalls` behaves exactly as before (Task 9 Step 1, plus Task 3's test).
- Streamed tool calls arrive whole, never as fragments (Task 2).
- A `tools` rejection degrades once and is traced (Task 2).
- An index written before FTS5 gains a populated search table on open (Task 5).
- Every tool error is a readable result, not a failed turn, and a cap that bites says so (Task 8).

---

## Self-Review

**Spec coverage (increments 1 and 2):**

- §4 the loop: deferred to the loop plan (as scoped).
- §5 provider contract: Tasks 1-3 (`Message`, `ToolSpec`, `ToolCall`, `Messages`/`Tools`, `StreamChunk.ToolCalls`, `Prompt` derived, HTTP accumulation, string-only providers).
- §6 capability: Task 4 (`supports_tools`); the HTTP rejection degradation is Task 2. Role resolution in the engine is the loop plan's, since only the loop offers tools.
- §7 the four tools: Tasks 6-8, including the one-table schemas, readable errors, unknown-tool listing, and the result cap.
- §8 retrieval stack: Task 5 (FTS5, porter, triggers, backfill) and Task 7 (queries built from words, raw `MATCH` reachable). `search_semantic` is out of scope by design.
- §9 bounds: Task 4 adds both keys and the `tool_result_chars` cap is enforced in Task 8; `tool_rounds` is consumed by the loop plan.
- §10-11 streaming, watchdog, and loop trace rows: loop plan.
- §12 config: Task 4.
- §13 testing: provider accumulation and degradation (Task 2), tools (Task 8), query building (Task 7), migration (Task 5), compatibility (Task 3 and the full suite).

**Placeholder scan:** no "TBD"/"implement later" text; every code step carries real code. Task 3 names its test file conditionally and tells the implementer to land it beside the provider's existing test, and Task 5 tells the implementer to match `TurnRecord`'s real field names before writing the test.

**Type consistency:** `Message`, `ToolSpec`, `ToolCall` are defined in Task 1 and consumed in Tasks 2, 3, 6, 8. `PromptText` is defined in Task 1 and used in Tasks 2 and 3. `EnsureFTS`, `SearchEntities`, `SearchTurns`, `SearchEntityHit`, `SearchTurnHit` are Task 5 and used in Task 8. `ToolSpecs`, `ToolNames`, `UnknownToolMessage` are Task 6 and used in Task 8. `BuildMatch` is Task 7 and used in Task 8. `NewExecutor` and `Execute` are Task 8. `RoleSupportsTools`, `ToolRounds`, `ToolResultChars` are Task 4; `ToolResultChars` is consumed by Task 8's executor construction, `ToolRounds` by the loop plan.

**Known deviations to report:** the FTS tables are plain rather than external-content (tags live in frontmatter), documented in Task 5; and `pkg/harness/tools.go` holds the schemas while `pkg/tools` holds the implementations, matching the spec's File Map.
