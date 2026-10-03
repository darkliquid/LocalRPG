// Package inworldllm registers the Inworld AI LLM router provider.
//
// Inworld's chat completions endpoint is OpenAI-compatible, so this reuses
// openaichat's HTTP provider (streaming, streamed tool-call accumulation,
// tracing, the tools-rejected retry) with an Inworld descriptor, preset, and
// Basic authentication rather than a second copy of the wire protocol.
package inworldllm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/provider/openaichat"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// DefaultEndpoint is Inworld's OpenAI-compatible chat completions gateway.
const DefaultEndpoint = "https://api.inworld.ai/v1/chat/completions"

// DefaultModel routes a prompt across Inworld's frontier models.
const DefaultModel = "inworld/compare-frontier-models"

// ErrMissingAPIKey is returned when no credential resolves.
var ErrMissingAPIKey = errors.New("inworld: an API key is required; set providers.inworld.api_key, agents.roles.<role>.api_key, or INWORLD_API_KEY")

// ResolveAPIKey applies the credential precedence: an explicit role key, then
// the shared providers.inworld.api_key, then INWORLD_API_KEY.
func ResolveAPIKey(explicit, shared string) (string, error) {
	for _, candidate := range []string{explicit, shared, os.Getenv("INWORLD_API_KEY")} {
		if key := strings.TrimSpace(candidate); key != "" {
			return key, nil
		}
	}
	return "", ErrMissingAPIKey
}

// NewInworldLLMClient builds the Inworld chat provider. The endpoint defaults to
// the public gateway and the model to the frontier router, so a role only has to
// name `type: inworld`.
func NewInworldLLMClient(cfg config.AgentRoleConfig, sharedKey string) (*openaichat.HTTPProvider, error) {
	apiKey, err := ResolveAPIKey(cfg.APIKey, sharedKey)
	if err != nil {
		return nil, err
	}
	trace.RegisterSecret(apiKey)

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}

	return openaichat.NewHTTPProviderWithAuth("inworld", endpoint, model, apiKey, "Basic", harness.GenerationOptions{
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	}), nil
}

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyLLMInworld),
			Family:      provider.FamilyLLM,
			Label:       "Inworld LLM Router",
			Description: "Gateway routing prompts across frontier models with automatic fallbacks and tool calling.",
			Source:      "http",
			Features: []provider.Feature{
				provider.FeatureStreaming,
				provider.FeatureTools,
				provider.FeatureKeyRequired,
				provider.FeatureMetered,
			},
			Presets: []provider.Preset{
				{
					ID:          "inworld-frontier",
					Order:       5,
					Label:       "Inworld Frontier Router",
					Description: "Routes prompts across frontier LLMs with automatic fallbacks.",
					Config: map[string]interface{}{
						"type":         "builtin",
						"builtin_name": "inworld",
						"model":        "inworld/compare-frontier-models",
						"temperature":  0.7,
						"max_tokens":   2048,
					},
				},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload harness.ModelBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewInworldLLMClient(config.AgentRoleConfig{
				Endpoint:    payload.Config.Endpoint,
				Model:       payload.Config.Model,
				APIKey:      payload.Config.APIKey,
				Temperature: payload.Config.Temperature,
				MaxTokens:   payload.Config.MaxTokens,
			}, payload.Config.SharedAPIKey)
		},
	})
}
