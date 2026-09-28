package gui

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestTranscribeAudioUnconfiguredIsFailure(t *testing.T) {
	svc := NewService(t.TempDir())
	_, err := svc.TranscribeAudio(context.Background(), []byte("audio"))
	failure, ok := harness.FailureFrom(err)
	if !ok {
		t.Fatalf("err = %v, want a GenerationFailure", err)
	}
	if failure.Code != harness.FailureProviderUnavailable {
		t.Fatalf("code = %q, want provider_unavailable", failure.Code)
	}
}

func TestTranscribeAudioWebSpeechIsActionable(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.configMgr.Get().Media.STT.Type = "web-speech"

	_, err := svc.TranscribeAudio(context.Background(), []byte("audio"))
	failure, ok := harness.FailureFrom(err)
	if !ok {
		t.Fatalf("err = %v, want a GenerationFailure", err)
	}
	if failure.Code != harness.FailureProviderUnavailable {
		t.Fatalf("code = %q, want provider_unavailable", failure.Code)
	}
	if !strings.Contains(failure.Message, "Whisper") {
		t.Fatalf("message %q should name the Whisper fallback", failure.Message)
	}
}

func TestTestProviderWebSpeechIsActionable(t *testing.T) {
	svc := NewService(t.TempDir())

	res, err := svc.TestProvider(context.Background(), TestProviderRequestDTO{
		Category: "stt",
		Provider: config.STTConfig{Type: "web-speech"},
	})
	if err != nil {
		t.Fatalf("TestProvider: %v", err)
	}
	if res.Success {
		t.Fatalf("expected the web-speech probe to fail")
	}
	if !strings.Contains(res.Message, "Whisper") {
		t.Fatalf("message %q should name the Whisper fallback", res.Message)
	}
}
