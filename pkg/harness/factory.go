package harness

import (
	"context"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/trace"
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
		return NewCLIProviderWithOptions(id, cfg.Command, cfg.Args, GenerationOptions{
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
		}), nil
	case "http":
		return NewHTTPProviderWithOptions(id, cfg.Endpoint, cfg.Model, cfg.APIKey, GenerationOptions{
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
		}), nil
	case "builtin", "mock", "":
		if cfg.BuiltinName == "narrative-oracle" {
			return NewNarrativeOracleProvider(id), nil
		}
		if cfg.Command != "" {
			return NewCLIProviderWithOptions(id, cfg.Command, cfg.Args, GenerationOptions{
				Temperature: cfg.Temperature,
				MaxTokens:   cfg.MaxTokens,
			}), nil
		}
		return &builtinEchoModelProvider{id: id}, nil
	default:
		return nil, fmt.Errorf("unknown model provider type: %s", cfg.Type)
	}
}

// NewModelProviderWithLogger is NewModelProvider with a trace sink attached, so a
// caller that has one does not have to know which provider types accept it.
func NewModelProviderWithLogger(id string, cfg ProviderConfig, logger trace.Logger) (ModelProvider, error) {
	provider, err := NewModelProvider(id, cfg)
	if err != nil {
		return nil, err
	}
	setProviderLogger(provider, logger)
	return provider, nil
}

// setProviderLogger attaches a logger to any provider that accepts one. Providers
// that do not are left alone rather than requiring the interface to grow.
func setProviderLogger(provider ModelProvider, logger trace.Logger) {
	if aware, ok := provider.(interface{ SetLogger(trace.Logger) }); ok {
		aware.SetLogger(logger)
	}
}

// setProviderChunkLimit caps wire events for providers that record them.
func setProviderChunkLimit(provider ModelProvider, limit int) {
	if aware, ok := provider.(interface{ SetChunkLimit(int) }); ok {
		aware.SetChunkLimit(limit)
	}
}

// RouterFromConfig builds the role-routed provider registry a turn needs: one
// provider per configured role, the configured fallbacks, and an echo default for
// gm when nothing else is set up. Inherited roles are skipped because they resolve
// through the role they name.
func RouterFromConfig(cfg *config.Config) (*Router, error) {
	return RouterFromConfigWithLogger(cfg, trace.Nop())
}

// RouterFromConfigWithLogger is RouterFromConfig with a trace sink attached to
// every provider it registers. It applies the configured chunk limit here because
// the router owns the providers, and a caller that reaches into them afterwards
// would be reaching into objects it did not build.
func RouterFromConfigWithLogger(cfg *config.Config, logger trace.Logger) (*Router, error) {
	if cfg == nil {
		return nil, fmt.Errorf("build router: no config")
	}

	router := NewRouter()

	for role, roleCfg := range cfg.Agents.Roles {
		if roleCfg.Type == "inherit" {
			continue
		}

		provider, err := NewModelProviderWithLogger(role, ProviderConfig{
			Type:        roleCfg.Type,
			BuiltinName: roleCfg.BuiltinName,
			Command:     roleCfg.Command,
			Args:        roleCfg.Args,
			Endpoint:    roleCfg.Endpoint,
			Model:       roleCfg.Model,
			APIKey:      roleCfg.APIKey,
			Temperature: roleCfg.Temperature,
			MaxTokens:   roleCfg.MaxTokens,
		}, logger)
		if err != nil {
			continue
		}
		setProviderChunkLimit(provider, cfg.TraceChunkLimit())

		router.RegisterProvider(provider)
		router.AssignRole(role, role)
	}

	for role, fallback := range cfg.Agents.Fallbacks {
		if fallback != "" {
			router.SetFallback(role, fallback)
		}
	}

	if _, err := router.GetProviderForRole(config.RoleGM); err != nil {
		router.RegisterProvider(NewCLIProviderWithLogger("default-echo", "echo", []string{}, GenerationOptions{}, logger))
		router.AssignRole(config.RoleGM, "default-echo")
	}

	return router, nil
}

// ExtractorFromConfig resolves the per-turn extractor role without a trace sink.
func ExtractorFromConfig(cfg *config.Config, router *Router) *Extractor {
	return ExtractorFromConfigWithLogger(cfg, router, trace.Nop())
}

// ExtractorFromConfigWithLogger resolves the per-turn extractor role. An absent
// role still inherits gm, so configuration written before the role existed keeps
// working; `disabled` opts out; `inherit` follows the named role, which is what
// stops extraction silently pointing at a stale copy of gm.
func ExtractorFromConfigWithLogger(cfg *config.Config, router *Router, logger trace.Logger) *Extractor {
	if cfg == nil || router == nil {
		return nil
	}

	roleCfg, configured := cfg.Agents.Roles[config.RoleExtractor]
	if !configured {
		roleCfg = config.AgentRoleConfig{Type: "inherit", InheritFrom: config.RoleGM}
	}

	switch roleCfg.Type {
	case "disabled":
		return nil
	case "inherit", "":
		source := roleCfg.InheritFrom
		if source == "" {
			source = config.RoleGM
		}

		provider, err := router.GetProviderForRole(source)
		if err != nil {
			return nil
		}
		extractor := NewExtractor(provider)
		extractor.SetLogger(logger)
		return extractor
	}

	provider, err := NewModelProviderWithLogger(config.RoleExtractor, ProviderConfig{
		Type:        roleCfg.Type,
		BuiltinName: roleCfg.BuiltinName,
		Command:     roleCfg.Command,
		Args:        roleCfg.Args,
		Endpoint:    roleCfg.Endpoint,
		Model:       roleCfg.Model,
		APIKey:      roleCfg.APIKey,
		Temperature: roleCfg.Temperature,
		MaxTokens:   roleCfg.MaxTokens,
	}, logger)
	if err != nil {
		return nil
	}
	extractor := NewExtractor(provider)
	extractor.SetLogger(logger)
	return extractor
}

// CompletionFromConfig resolves the role that finishes a cut-off reply. It
// inherits gm unless configured otherwise, and a nil result disables the
// continuation half of recovery, leaving trimming.
func CompletionFromConfig(cfg *config.Config, router *Router, logger trace.Logger) ModelProvider {
	if cfg == nil || router == nil {
		return nil
	}

	roleCfg, configured := cfg.Agents.Roles[config.RoleCompletion]
	if !configured {
		roleCfg = config.AgentRoleConfig{Type: "inherit", InheritFrom: config.RoleGM}
	}

	switch roleCfg.Type {
	case "disabled":
		return nil
	case "inherit", "":
		source := roleCfg.InheritFrom
		if source == "" {
			source = config.RoleGM
		}
		provider, err := router.GetProviderForRole(source)
		if err != nil {
			return nil
		}
		return provider
	}

	provider, err := NewModelProviderWithLogger(config.RoleCompletion, ProviderConfig{
		Type:        roleCfg.Type,
		BuiltinName: roleCfg.BuiltinName,
		Command:     roleCfg.Command,
		Args:        roleCfg.Args,
		Endpoint:    roleCfg.Endpoint,
		Model:       roleCfg.Model,
		APIKey:      roleCfg.APIKey,
		Temperature: roleCfg.Temperature,
		MaxTokens:   roleCfg.MaxTokens,
	}, logger)
	if err != nil {
		return nil
	}
	setProviderChunkLimit(provider, cfg.TraceChunkLimit())
	return provider
}
