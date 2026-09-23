package harness

import (
	"strings"
	"testing"
)

func TestProviderConfigValidation(t *testing.T) {
	cfg := ProviderConfig{
		Type:     "cli",
		Command:  "claude",
		Args:     []string{"--print"},
		Endpoint: "",
	}

	if cfg.Type != "cli" || cfg.Command != "claude" {
		t.Errorf("unexpected config: %+v", cfg)
	}

	roleCfg := RoleRoutingConfig{
		Roles: map[string]ProviderConfig{
			"gm":        {Type: "cli", Command: "claude"},
			"extractor": {Type: "http", Endpoint: "http://localhost:11434", Model: "qwen2.5:7b"},
		},
	}

	if len(roleCfg.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roleCfg.Roles))
	}
}

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
