package media

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// STTClient is what every transcription backend implements.
type STTClient interface {
	Transcribe(ctx context.Context, audioData []byte) (string, error)
}

// STTProvider wraps a client with what a transcription needs around it: the cache
// its result lands in and the usage it reports.
type STTProvider struct {
	client STTClient
}

// NewSTTProvider wraps a transcription client.
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
