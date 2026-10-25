package anthropicllm

import (
	"encoding/json"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestBuildRequestShape(t *testing.T) {
	p := NewAnthropicProvider("gm", Options{Model: "claude-sonnet-5-5"})
	req := harness.GenerateRequest{
		System: "You are the GM.",
		Messages: []harness.Message{
			{Role: "system", Content: "Rules: be dramatic."},
			{Role: "user", Content: "look around"},
			{Role: "assistant", Content: "A cellar.", ToolCalls: []harness.ToolCall{{ID: "t1", Name: "roll", Arguments: `{"d":"d20"}`}}},
			{Role: "tool", ToolCallID: "t1", Content: "14"},
			{Role: "user", Content: "climb down"},
		},
		Tools:      []harness.ToolSpec{{Name: "roll", Description: "roll dice", Parameters: map[string]any{"type": "object"}}},
		ToolChoice: "required",
	}
	out := p.buildRequest(req)

	if out.System != "You are the GM.\n\nRules: be dramatic." {
		t.Errorf("system = %q", out.System)
	}
	if out.MaxTokens != 1024 {
		t.Errorf("max_tokens = %d, want the 1024 default", out.MaxTokens)
	}
	// system folded out, tool folded into a user block, the two user turns merged:
	// user(look around), assistant(text + tool_use), user(tool_result + climb down)
	if len(out.Messages) != 3 {
		t.Fatalf("messages = %d, want 3: %v", len(out.Messages), out.Messages)
	}
	assistant := out.Messages[1]
	if assistant.Role != "assistant" || len(assistant.Content) != 2 {
		t.Fatalf("assistant = %+v", assistant)
	}
	if assistant.Content[0].Type != "text" || assistant.Content[0].Text != "A cellar." {
		t.Errorf("assistant text block = %+v", assistant.Content[0])
	}
	use := assistant.Content[1]
	if use.Type != "tool_use" || use.ToolUseID != "t1" || use.Name != "roll" || use.Input["d"] != "d20" {
		t.Errorf("tool_use block = %+v", use)
	}
	merged := out.Messages[2]
	if merged.Role != "user" || len(merged.Content) != 2 {
		t.Fatalf("merged user = %+v", merged)
	}
	if merged.Content[0].Type != "tool_result" || merged.Content[0].ToolUseID != "t1" || merged.Content[0].Text != "14" {
		t.Errorf("tool_result block = %+v", merged.Content[0])
	}
	if merged.Content[1].Type != "text" || merged.Content[1].Text != "climb down" {
		t.Errorf("trailing user text = %+v", merged.Content[1])
	}
	if len(out.Tools) != 1 || out.Tools[0].Name != "roll" || out.ToolChoice == nil || out.ToolChoice.Type != "any" {
		t.Errorf("tools = %+v tool_choice = %+v", out.Tools, out.ToolChoice)
	}
	if out.CacheControl == nil || out.CacheControl.Type != "ephemeral" {
		t.Errorf("cache_control = %+v, want ephemeral under the auto default", out.CacheControl)
	}
}

func TestBuildRequestSamplingCascade(t *testing.T) {
	p := NewAnthropicProvider("gm", Options{Model: "m", MaxTokens: 0, PromptCache: "off"})
	out := p.buildRequest(harness.GenerateRequest{Prompt: "hi", MaxTokens: 512})
	if out.MaxTokens != 512 {
		t.Errorf("max_tokens = %d, want 512", out.MaxTokens)
	}
	if out.CacheControl != nil {
		t.Errorf("cache_control sent with prompt_cache off")
	}
	if out.Thinking != nil {
		t.Errorf("thinking sent without a budget: %+v", out.Thinking)
	}
}

func TestBuildRequestThinkingBudget(t *testing.T) {
	budget := 2048
	p := NewAnthropicProvider("gm", Options{Model: "m", ThinkingBudget: &budget})
	out := p.buildRequest(harness.GenerateRequest{Prompt: "hi"})
	if out.Thinking == nil || out.Thinking.Type != "enabled" || out.Thinking.BudgetTokens != 2048 {
		t.Errorf("thinking = %+v, want enabled/2048", out.Thinking)
	}
	withTools := p.buildRequest(harness.GenerateRequest{Prompt: "hi", Tools: []harness.ToolSpec{{Name: "t"}}})
	if withTools.Thinking != nil {
		t.Errorf("thinking sent on a tool request: %+v", withTools.Thinking)
	}
	zero := 0
	off := NewAnthropicProvider("gm", Options{Model: "m", ThinkingBudget: &zero})
	if got := off.buildRequest(harness.GenerateRequest{Prompt: "hi"}); got.Thinking != nil {
		t.Errorf("thinking sent with a zero budget: %+v", got.Thinking)
	}
}

func TestMappedInputMalformedJSON(t *testing.T) {
	in := mappedInput("not json")
	if in["arguments"] != "not json" {
		t.Errorf("mappedInput = %v", in)
	}
	ok := mappedInput(`{"d":"d20"}`)
	if ok["d"] != "d20" {
		t.Errorf("mappedInput(valid) = %v", ok)
	}
}

func TestBuildRequestWarnsOnceAndOmitsTemperature(t *testing.T) {
	var body map[string]any
	p := NewAnthropicProvider("gm", Options{Model: "m", Temperature: 0.7})
	req := p.buildRequest(harness.GenerateRequest{Prompt: "hi"})
	b, _ := json.Marshal(req)
	_ = json.Unmarshal(b, &body)
	if _, sent := body["temperature"]; sent {
		t.Errorf("temperature sent on the wire: %v", body)
	}
	if !p.warned {
		t.Errorf("expected the unsupported-parameter warn to fire once")
	}
	p.buildRequest(harness.GenerateRequest{Prompt: "again"})
	if !p.warned {
		t.Errorf("warn flag lost")
	}
}
