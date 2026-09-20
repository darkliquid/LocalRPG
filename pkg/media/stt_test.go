package media

import (
	"context"
	"testing"
)

type mockSTTClient struct {
	transcription string
}

func (m *mockSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	return m.transcription, nil
}

func TestSTTTranscription(t *testing.T) {
	client := &mockSTTClient{transcription: "I draw my bow and aim at the scout."}
	provider := NewSTTProvider(client)

	text, err := provider.TranscribeAudio(context.Background(), []byte("wav bytes"))
	if err != nil {
		t.Fatalf("TranscribeAudio failed: %v", err)
	}

	if text != "I draw my bow and aim at the scout." {
		t.Errorf("transcription mismatch: %s", text)
	}
}
