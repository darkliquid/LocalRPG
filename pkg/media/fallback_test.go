package media

import (
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

type testImageClient struct {
	data []byte
	err  error
}

func (s testImageClient) GenerateImage(context.Context, string) ([]byte, error) {
	return s.data, s.err
}

func TestFallbackReportsBothFailures(t *testing.T) {
	c := &fallbackImageClient{
		primary:  testImageClient{err: errors.New("primary down")},
		fallback: testImageClient{err: errors.New("builtin down")},
	}
	_, err := c.GenerateImage(context.Background(), "prompt")
	failure, ok := harness.FailureFrom(err)
	if !ok {
		t.Fatalf("err = %v, want a GenerationFailure", err)
	}
	if len(failure.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(failure.Attempts))
	}
}

func TestFallbackSucceedsAfterPrimaryFailure(t *testing.T) {
	c := &fallbackImageClient{
		primary:  testImageClient{err: errors.New("primary down")},
		fallback: testImageClient{data: []byte("art")},
	}
	data, err := c.GenerateImage(context.Background(), "prompt")
	if err != nil || string(data) != "art" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

type testSTTClient struct {
	text string
	err  error
}

func (s testSTTClient) Transcribe(context.Context, []byte) (string, error) {
	return s.text, s.err
}

func TestSTTFailureCarriesProviderMessage(t *testing.T) {
	provider := NewSTTProvider(testSTTClient{err: errors.New("whisper: model file missing")})
	_, err := provider.TranscribeAudio(context.Background(), []byte("audio"))
	failure, ok := harness.FailureFrom(err)
	if !ok {
		t.Fatalf("err = %v, want a GenerationFailure", err)
	}
	if failure.Message != "whisper: model file missing" {
		t.Fatalf("message = %q, want the provider message", failure.Message)
	}
}
