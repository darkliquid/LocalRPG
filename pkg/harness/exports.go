package harness

// ProviderIDFor maps a role configuration to the registry ID a facade should
// build. An empty result means the configuration has no registry provider (a
// disabled role, or the debug echo fallback) and the caller handles it.
func ProviderIDFor(cfg ProviderConfig) string {
	switch cfg.Type {
	case "http":
		return "openaichat"
	case "cli":
		return "cli"
	case "gemini":
		return "gemini"
	case "builtin", "mock", "":
		switch cfg.BuiltinName {
		case "gemini":
			return "gemini"
		case "narrative-oracle":
			return "narrative-oracle"
		}
		if cfg.Command != "" {
			return "cli"
		}
		return ""
	default:
		return ""
	}
}

// NewOpenAIChatProvider builds the OpenAI-compatible HTTP provider.
func NewOpenAIChatProvider(id string, cfg ProviderConfig) ModelProvider {
	return NewHTTPProviderWithOptions(id, cfg.Endpoint, cfg.Model, cfg.APIKey, GenerationOptions{
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	})
}

// NewCLIModelProvider builds the command-line provider.
func NewCLIModelProvider(id string, cfg ProviderConfig) ModelProvider {
	return NewCLIProviderWithOptions(id, cfg.Command, cfg.Args, GenerationOptions{
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	})
}

// NewGeminiModelProvider builds the Gemini provider, resolving the API key from
// the role override, the shared key, or the environment.
func NewGeminiModelProvider(id string, cfg ProviderConfig) (ModelProvider, error) {
	apiKey, err := ResolveGeminiAPIKey(cfg.APIKey, cfg.SharedAPIKey)
	if err != nil {
		return nil, err
	}
	return NewGeminiProvider(id, GeminiProviderOptions{
		Model:          cfg.Model,
		APIKey:         apiKey,
		Temperature:    &cfg.Temperature,
		MaxTokens:      &cfg.MaxTokens,
		ThinkingBudget: cfg.ThinkingBudget,
		TopP:           cfg.TopP,
		TopK:           cfg.TopK,
	})
}
