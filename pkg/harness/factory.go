package harness

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider"
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
	return &GenerateResponse{Text: "Echo: " + req.PromptText()}, nil
}

func (b *builtinEchoModelProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)
	out <- StreamChunk{Text: "Echo: " + req.PromptText(), Done: true}
	return nil
}

func NewModelProvider(id string, cfg ProviderConfig) (ModelProvider, error) {
	switch cfg.Type {
	case "disabled":
		return &disabledModelProvider{id: id}, nil
	case "builtin":
		if cfg.BuiltinName == "echo" {
			return &builtinEchoModelProvider{id: id}, nil
		}
		return BuildModelFor(id, cfg)
	case "cli", "http", "gemini", "inworld", "":
		return BuildModelFor(id, cfg)
	default:
		return nil, fmt.Errorf("unknown model provider type: %s", cfg.Type)
	}
}

// ModelBuildPayload is what BuildModelFor hands a provider package: the family
// config plus the role id the caller wants the provider named.
type ModelBuildPayload struct {
	ID     string         `json:"id"`
	Config ProviderConfig `json:"config"`
}

// BuildModelFor builds the provider for cfg's registry id, named id, so role
// routing keeps working when construction goes through the registry.
func BuildModelFor(id string, cfg ProviderConfig) (ModelProvider, error) {
	key, ok := KeyFor(cfg)
	if !ok {
		return nil, fmt.Errorf("harness: no registry provider for type %q", cfg.Type)
	}
	regID := string(key.Parent())
	reg, ok := provider.Lookup(regID)
	if !ok {
		return nil, fmt.Errorf("harness: provider %q is not registered", regID)
	}
	raw, err := json.Marshal(ModelBuildPayload{ID: id, Config: cfg})
	if err != nil {
		return nil, fmt.Errorf("harness: encode %s config: %w", regID, err)
	}
	built, err := reg.Build(context.Background(), raw)
	if err != nil {
		return nil, err
	}
	model, ok := built.(ModelProvider)
	if !ok {
		return nil, fmt.Errorf("harness: provider %q is not a model provider", regID)
	}
	return model, nil
}

// BuildModel constructs a model provider from the registry by descriptor ID.
func BuildModel(descriptorID string, cfg ProviderConfig) (ModelProvider, error) {
	return BuildModelFor(descriptorID, cfg)
}

// sharedKeyFor picks the provider-wide credential a role inherits when it has
// none of its own: an Inworld role gets providers.inworld.api_key, every other
// role keeps the Gemini key it has always used.
func sharedKeyFor(cfg *config.Config, key provider.Key, hasKey bool) string {
	if hasKey && key.Parent() == provider.KeyLLMInworld {
		return cfg.Providers.Inworld.APIKey
	}
	return cfg.Providers.Gemini.APIKey
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
	router.SetLogger(logger)

	// roleTiers and roleFeatures let a chain order its members by the tier and
	// features of each member's registered adapter, without the chain code
	// reaching into the registry itself.
	roleTiers := make(map[string]provider.Tier, len(cfg.Agents.Roles))
	roleFeatures := make(map[string][]provider.Feature, len(cfg.Agents.Roles))

	gmConfigured := false
	for role, roleCfg := range cfg.Agents.Roles {
		if roleCfg.Type == "inherit" {
			continue
		}
		if role == config.RoleGM {
			gmConfigured = true
		}

		key, hasKey := KeyFor(ProviderConfig{
			Type:        roleCfg.Type,
			BuiltinName: roleCfg.BuiltinName,
			Command:     roleCfg.Command,
			Endpoint:    roleCfg.Endpoint,
			Instance:    roleCfg.Instance,
		})
		if hasKey {
			if reg, ok := provider.Lookup(string(key.Parent())); ok {
				roleTiers[role] = reg.Descriptor.Tier
				roleFeatures[role] = reg.Descriptor.Features
			}
		}

		provider, err := NewModelProviderWithLogger(role, ProviderConfig{
			Type:           roleCfg.Type,
			BuiltinName:    roleCfg.BuiltinName,
			Command:        roleCfg.Command,
			Args:           roleCfg.Args,
			Endpoint:       roleCfg.Endpoint,
			Model:          roleCfg.Model,
			APIKey:         roleCfg.APIKey,
			Temperature:    roleCfg.Temperature,
			MaxTokens:      roleCfg.MaxTokens,
			ThinkingBudget: roleCfg.ThinkingBudget,
			TopP:           roleCfg.TopP,
			TopK:           roleCfg.TopK,
			SharedAPIKey:   sharedKeyFor(cfg, key, hasKey),
			Instance:       roleCfg.Instance,
		}, logger)
		if err != nil {
			name := roleCfg.BuiltinName
			if name == "" {
				name = roleCfg.Command
			}
			router.recordBuildError(RoleBuildError{
				Role: role, Type: roleCfg.Type, Name: name, Err: err,
			})
			continue
		}
		setProviderChunkLimit(provider, cfg.TraceChunkLimit())

		router.RegisterProvider(provider)
		router.AssignRole(role, role)
		if hasKey {
			router.AssignRoleKey(role, key)
		}
	}

	for role, fallback := range cfg.Agents.Fallbacks {
		if fallback != "" {
			router.SetFallback(role, fallback)
		}
	}

	// A declared chain names other configured roles, which are registered as
	// providers above, so the router can try them in the rule's order.
	for role, roleCfg := range cfg.Agents.Roles {
		if len(roleCfg.Chain) > 0 {
			router.SetChain(role, roleCfg.Chain, roleCfg.ChainConfig().Rule(), roleCfg.Tag)
		}
	}
	router.SetChainTier(func(id string) (provider.Tier, []provider.Feature) {
		return roleTiers[id], roleFeatures[id]
	})

	// The echo default exists for an intentionally unconfigured gm, not to mask a
	// gm that was configured and then failed to build.
	if !gmConfigured {
		if _, err := router.GetProviderForRole(config.RoleGM); err != nil {
			router.RegisterProvider(&builtinEchoModelProvider{id: "default-echo"})
			router.AssignRole(config.RoleGM, "default-echo")
		}
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
		if key, ok := router.ProviderKeyForRole(source); ok {
			extractor.SetProviderKey(key)
		}
		return extractor
	}

	provider, err := NewModelProviderWithLogger(config.RoleExtractor, ProviderConfig{
		Type:           roleCfg.Type,
		BuiltinName:    roleCfg.BuiltinName,
		Command:        roleCfg.Command,
		Args:           roleCfg.Args,
		Endpoint:       roleCfg.Endpoint,
		Model:          roleCfg.Model,
		APIKey:         roleCfg.APIKey,
		Temperature:    roleCfg.Temperature,
		MaxTokens:      roleCfg.MaxTokens,
		ThinkingBudget: roleCfg.ThinkingBudget,
		TopP:           roleCfg.TopP,
		TopK:           roleCfg.TopK,
		SharedAPIKey:   cfg.Providers.Gemini.APIKey,
		Instance:       roleCfg.Instance,
	}, logger)
	if err != nil {
		return nil
	}
	extractor := NewExtractor(provider)
	extractor.SetLogger(logger)
	if key, ok := KeyFor(ProviderConfig{
		Type:        roleCfg.Type,
		BuiltinName: roleCfg.BuiltinName,
		Command:     roleCfg.Command,
		Endpoint:    roleCfg.Endpoint,
		Instance:    roleCfg.Instance,
	}); ok {
		extractor.SetProviderKey(key)
	}
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
		Type:           roleCfg.Type,
		BuiltinName:    roleCfg.BuiltinName,
		Command:        roleCfg.Command,
		Args:           roleCfg.Args,
		Endpoint:       roleCfg.Endpoint,
		Model:          roleCfg.Model,
		APIKey:         roleCfg.APIKey,
		Temperature:    roleCfg.Temperature,
		MaxTokens:      roleCfg.MaxTokens,
		ThinkingBudget: roleCfg.ThinkingBudget,
		TopP:           roleCfg.TopP,
		TopK:           roleCfg.TopK,
		SharedAPIKey:   cfg.Providers.Gemini.APIKey,
		Instance:       roleCfg.Instance,
	}, logger)
	if err != nil {
		return nil
	}
	setProviderChunkLimit(provider, cfg.TraceChunkLimit())
	return provider
}
