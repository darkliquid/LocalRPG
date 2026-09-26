package gui

import (
	"context"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

// bareClient is a TTS client that declares no options.
type bareClient struct{}

func (bareClient) Synthesize(context.Context, string, *entity.VoiceConfig) ([]byte, error) {
	return nil, nil
}

// inspectingClient declares one tunable, for option-validation tests.
type inspectingClient struct{}

func (inspectingClient) Synthesize(context.Context, string, *entity.VoiceConfig) ([]byte, error) {
	return nil, nil
}

func (inspectingClient) VoiceOptions() []media.VoiceOption {
	return []media.VoiceOption{
		{Key: "stability", Label: "Stability", Kind: "float", Min: 0, Max: 1, Step: 0.05},
	}
}
