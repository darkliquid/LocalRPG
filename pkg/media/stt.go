package media

import (
	"context"
	"fmt"
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

func (s *STTProvider) TranscribeAudio(ctx context.Context, audioData []byte) (string, error) {
	if len(audioData) == 0 {
		return "", fmt.Errorf("empty audio data")
	}
	return s.client.Transcribe(ctx, audioData)
}
