package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

// TestAudioPipelineIsBuiltOnce checks the shared pipeline: two requests must not
// construct two TTS clients (which for built-in TTS reloads the model).
func TestAudioPipelineIsBuiltOnce(t *testing.T) {
	_, svc := turnFixture(t)

	builds := 0
	svc.newTTSClient = func(cfg config.TTSConfig) (media.TTSClient, error) {
		builds++
		return &bareClient{}, nil
	}
	svc.configMgr.Get().Media.TTS.Type = "cli"
	svc.configMgr.Get().Media.TTS.Command = "true"

	first, err := svc.audioPipeline()
	if err != nil {
		t.Fatalf("first audioPipeline: %v", err)
	}
	second, err := svc.audioPipeline()
	if err != nil {
		t.Fatalf("second audioPipeline: %v", err)
	}
	if first != second {
		t.Fatal("audioPipeline rebuilt the pipeline for the same config")
	}
	if builds != 1 {
		t.Fatalf("tts client builds = %d, want 1", builds)
	}
}
