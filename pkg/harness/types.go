package harness

import (
	"context"
	"strings"
)

type StreamChunk struct {
	Text         string
	ToolCalls    []ToolCall
	Done         bool
	FinishReason string
	Error        error
	// Usage is set on the final chunk by a provider that reports it.
	Usage *Usage
}

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
	Signature []byte `json:"signature,omitempty"`
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

// GenerationOptions are the sampling parameters a provider applies to a call.
// They come from the role's configuration and are merged with any per-request
// values, which win.
type GenerationOptions struct {
	Temperature float64
	MaxTokens   int
	Stop        []string
}

type GenerateRequest struct {
	Prompt      string                 `json:"prompt"`
	System      string                 `json:"system,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
	// Messages is authoritative when set. Prompt remains for providers that only
	// accept a single string.
	Messages []Message  `json:"messages,omitempty"`
	Tools    []ToolSpec `json:"tools,omitempty"`
	// ToolChoice asks the provider to force a tool call: "required" forces at
	// least one, "none" forbids them, and "" leaves it to the model. Providers
	// that cannot force tool use ignore it.
	ToolChoice string `json:"tool_choice,omitempty"`
}

// PromptText is the request as a single string: the explicit Prompt when a caller
// set one, otherwise the conversation flattened for a provider that cannot take
// messages.
func (r GenerateRequest) PromptText() string {
	if strings.TrimSpace(r.Prompt) != "" {
		return r.Prompt
	}
	return MessagesPrompt(r.Messages)
}

type GenerateResponse struct {
	Text         string `json:"text"`
	CachedTokens int    `json:"cached_tokens,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	// Usage is set by a provider that reports token usage.
	Usage *Usage
}

type ModelProvider interface {
	ID() string
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error
}

// ToolCaller is implemented by providers that can be offered tools.
type ToolCaller interface {
	ToolCallerCapable() bool
}

type ProviderConfig struct {
	Type           string   `yaml:"type"` // "builtin", "cli", "http", "mock", "disabled"
	BuiltinName    string   `yaml:"builtin_name,omitempty"`
	Command        string   `yaml:"command,omitempty"`
	Args           []string `yaml:"args,omitempty"`
	Endpoint       string   `yaml:"endpoint,omitempty"`
	Model          string   `yaml:"model,omitempty"`
	APIKey         string   `yaml:"api_key,omitempty"`
	Temperature    float64  `yaml:"temperature,omitempty"`
	MaxTokens      int      `yaml:"max_tokens,omitempty"`
	ThinkingBudget *int     `yaml:"thinking_budget,omitempty"`
	TopP           *float64 `yaml:"top_p,omitempty"`
	TopK           *int     `yaml:"top_k,omitempty"`
	SharedAPIKey   string   `yaml:"shared_api_key,omitempty"`
}

type RoleRoutingConfig struct {
	Roles map[string]ProviderConfig `yaml:"roles"`
}
