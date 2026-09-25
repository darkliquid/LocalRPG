package geminiembedding

import (
	"testing"
)

func TestGeminiEmbeddingConfig(t *testing.T) {
	client := NewClient(ClientConfig{
		APIKey: "fake-key",
		Model:  "text-embedding-004",
	})
	if client.ID() != "gemini-embedding" {
		t.Errorf("expected ID gemini-embedding, got %s", client.ID())
	}
	if client.Dimensions() != 768 {
		t.Errorf("expected 768 dimensions for text-embedding-004, got %d", client.Dimensions())
	}
}
