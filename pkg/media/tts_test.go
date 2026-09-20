package media

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type mockTTSClient struct {
	lastVoice string
	lastText  string
}

func (m *mockTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	m.lastText = text
	if voice != nil {
		m.lastVoice = voice.VoiceID
	}
	return []byte("mock-wav-bytes"), nil
}

func TestParseDialogueSegments(t *testing.T) {
	narrative := `The cold wind whistles through the cracks in the door.
Lady Evelyn: "You shouldn't have come here alone, traveler."
You reach for your sword, but she shakes her head.
"Put that away," she whispers.`

	segments := ParseDialogueSegments(narrative, "Narrator")

	if len(segments) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(segments))
	}

	if segments[0].Speaker != "Narrator" || !segments[0].IsNarrator {
		t.Errorf("expected segment 0 to be Narrator, got %+v", segments[0])
	}
	if segments[1].Speaker != "Lady Evelyn" || segments[1].IsNarrator {
		t.Errorf("expected segment 1 to be Lady Evelyn, got %+v", segments[1])
	}
	if segments[2].Speaker != "Narrator" {
		t.Errorf("expected segment 2 to be Narrator, got %+v", segments[2])
	}
	if segments[3].Speaker != "Lady Evelyn" {
		t.Errorf("expected continued dialogue to attribute to Lady Evelyn, got %+v", segments[3])
	}
}

func TestTTSSynthesisWithCache(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewContentCache(tempDir)
	client := &mockTTSClient{}

	pipeline := NewTTSPipeline(client, cache)

	voice := &entity.VoiceConfig{Provider: "kokoro", VoiceID: "bf_emma"}
	path, err := pipeline.SynthesizeUtterance(context.Background(), "lady-evelyn", voice, "Thank you.")
	if err != nil {
		t.Fatalf("SynthesizeUtterance failed: %v", err)
	}

	if path == "" {
		t.Errorf("expected non-empty audio path")
	}

	// Verify cached
	if client.lastVoice != "bf_emma" || client.lastText != "Thank you." {
		t.Errorf("synthesis parameters mismatch: %+v", client)
	}
}
