package harness

import (
	"context"
	"fmt"
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
		if cfg.Command != "" {
			return NewCLIProvider(id, cfg.Command, cfg.Args), nil
		}
		return &builtinEchoModelProvider{id: id}, nil
	default:
		return nil, fmt.Errorf("unknown model provider type: %s", cfg.Type)
	}
}
