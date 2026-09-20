package media

import (
	"context"
	"testing"
)

type mockImageClient struct {
	lastPrompt string
}

func (m *mockImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	m.lastPrompt = prompt
	return []byte("mock-png-image-bytes"), nil
}

func TestImagePipelinePromptCompositionAndCache(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewContentCache(tempDir)
	client := &mockImageClient{}

	pipeline := NewImagePipeline(client, cache)

	entityID := "alden-tavern"
	appearance := "Misty tavern with dark wooden beams"
	worldStyle := "oil painting, dark fantasy, gritty"

	path, err := pipeline.GenerateSceneImage(context.Background(), entityID, appearance, worldStyle)
	if err != nil {
		t.Fatalf("GenerateSceneImage failed: %v", err)
	}

	if path == "" {
		t.Errorf("expected non-empty image path")
	}

	expectedPrompt := "Misty tavern with dark wooden beams, oil painting, dark fantasy, gritty"
	if client.lastPrompt != expectedPrompt {
		t.Errorf("expected prompt %q, got %q", expectedPrompt, client.lastPrompt)
	}
}
