package media_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestNativeOSTTS_GeneratesAudioBytes(t *testing.T) {
	client, err := media.NewTTSClient(config.TTSConfig{
		Type:        "builtin",
		BuiltinName: "native-os",
	})
	if err != nil {
		t.Fatalf("failed to create native-os TTS client: %v", err)
	}

	bytes, err := client.Synthesize(context.Background(), "The road ahead is quiet.", &entity.VoiceConfig{
		VoiceID: "default",
		Pitch:   1.0,
	})
	if err != nil {
		t.Fatalf("synthesize failed: %v", err)
	}

	if len(bytes) < 1000 || string(bytes[:4]) != "RIFF" || string(bytes[8:12]) != "WAVE" {
		t.Errorf("expected valid generated audio stream >= 1000 bytes with RIFF/WAVE header, got %d", len(bytes))
	}
}
