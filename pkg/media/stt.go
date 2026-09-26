package media

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"

	"github.com/darkliquid/localrpg/pkg/harness"
)

type STTClient interface {
	Transcribe(ctx context.Context, audioData []byte) (string, error)
}

type STTProvider struct {
	client STTClient
}

func NewSTTProvider(client STTClient) *STTProvider {
	return &STTProvider{client: client}
}

// TranscribeAudio turns a provider error into a bounded GenerationFailure so the
// caller can log the provider's own words and show them to the user, and records
// the failure against localrpg.provider.errors.
func (s *STTProvider) TranscribeAudio(ctx context.Context, audioData []byte) (string, error) {
	if len(audioData) == 0 {
		return "", &harness.GenerationFailure{Code: harness.FailureInvalidRequest, Message: "empty audio data"}
	}
	text, err := s.client.Transcribe(ctx, audioData)
	if err != nil {
		code := harness.ClassifyProviderError(err)
		mediaMetrics().providerErrors.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.role", "stt"),
			attribute.String("error.kind", string(code)),
		))
		return "", &harness.GenerationFailure{Code: code, Message: err.Error(), Cause: err}
	}
	return text, nil
}
