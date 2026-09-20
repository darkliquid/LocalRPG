package harness

import (
	"context"
)

type StreamChunk struct {
	Text  string
	Done  bool
	Error error
}

type GenerateRequest struct {
	Prompt      string                 `json:"prompt"`
	System      string                 `json:"system,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

type GenerateResponse struct {
	Text string `json:"text"`
}

type ModelProvider interface {
	ID() string
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error
}

type ProviderConfig struct {
	Type        string   `yaml:"type"` // "cli", "http", "mock"
	Command     string   `yaml:"command,omitempty"`
	Args        []string `yaml:"args,omitempty"`
	Endpoint    string   `yaml:"endpoint,omitempty"`
	Model       string   `yaml:"model,omitempty"`
	APIKey      string   `yaml:"api_key,omitempty"`
	Temperature float64  `yaml:"temperature,omitempty"`
}

type RoleRoutingConfig struct {
	Roles map[string]ProviderConfig `yaml:"roles"`
}
