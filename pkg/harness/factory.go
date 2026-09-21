package harness

import (
	"context"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/config"
)

type disabledModelProvider struct {
	id string
}

func (d *disabledModelProvider) ID() string { return d.id }

func (d *disabledModelProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("provider %q is disabled", d.id)
}

func (d *disabledModelProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)
	return fmt.Errorf("provider %q is disabled", d.id)
}

type builtinEchoModelProvider struct {
	id string
}

func (b *builtinEchoModelProvider) ID() string { return b.id }

func (b *builtinEchoModelProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	return &GenerateResponse{Text: "Echo: " + req.Prompt}, nil
}

func (b *builtinEchoModelProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)
	out <- StreamChunk{Text: "Echo: " + req.Prompt, Done: true}
	return nil
}

func NewModelProvider(id string, cfg ProviderConfig) (ModelProvider, error) {
	switch cfg.Type {
	case "disabled":
		return &disabledModelProvider{id: id}, nil
	case "cli":
		return NewCLIProvider(id, cfg.Command, cfg.Args), nil
	case "http":
		return NewHTTPProvider(id, cfg.Endpoint, cfg.Model, cfg.APIKey), nil
	case "builtin", "mock", "":
		if cfg.BuiltinName == "narrative-oracle" {
			return NewNarrativeOracleProvider(id), nil
		}
		if cfg.Command != "" {
			return NewCLIProvider(id, cfg.Command, cfg.Args), nil
		}
		return &builtinEchoModelProvider{id: id}, nil
	default:
		return nil, fmt.Errorf("unknown model provider type: %s", cfg.Type)
	}
}

// RouterFromConfig builds the role-routed provider registry a turn needs: one
// provider per configured role, the configured fallbacks, and an echo default for
// gm when nothing else is set up. Inherited roles are skipped because they resolve
// through the role they name.
func RouterFromConfig(cfg *config.Config) (*Router, error) {
	if cfg == nil {
		return nil, fmt.Errorf("build router: no config")
	}

	router := NewRouter()

	for role, roleCfg := range cfg.Agents.Roles {
		if roleCfg.Type == "inherit" {
			continue
		}

		provider, err := NewModelProvider(role, ProviderConfig{
			Type:        roleCfg.Type,
			BuiltinName: roleCfg.BuiltinName,
			Command:     roleCfg.Command,
			Args:        roleCfg.Args,
			Endpoint:    roleCfg.Endpoint,
			Model:       roleCfg.Model,
			APIKey:      roleCfg.APIKey,
			Temperature: roleCfg.Temperature,
			MaxTokens:   roleCfg.MaxTokens,
		})
		if err != nil {
			continue
		}

		router.RegisterProvider(provider)
		router.AssignRole(role, role)
	}

	for role, fallback := range cfg.Agents.Fallbacks {
		if fallback != "" {
			router.SetFallback(role, fallback)
		}
	}

	if _, err := router.GetProviderForRole(config.RoleGM); err != nil {
		router.RegisterProvider(NewCLIProvider("default-echo", "echo", []string{}))
		router.AssignRole(config.RoleGM, "default-echo")
	}

	return router, nil
}
