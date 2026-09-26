package gui

import (
	"context"
	"testing"

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
