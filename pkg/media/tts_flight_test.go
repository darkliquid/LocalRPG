package media

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type countingTTS struct{ calls atomic.Int32 }

func (c *countingTTS) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	c.calls.Add(1)
	return GenerateToneWAV(440, 0.02), nil
}

// TestSynthesisIsSingleFlightPerKey covers the shared pipeline: concurrent
// requests for the same utterance must synthesize and Opus-encode once.
func TestSynthesisIsSingleFlightPerKey(t *testing.T) {
	client := &countingTTS{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{Provider: "counting", VoiceID: "v1"}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := pipeline.SynthesizeUtterance(context.Background(), "speaker", voice, "hello there"); err != nil {
				t.Errorf("SynthesizeUtterance: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := client.calls.Load(); got != 1 {
		t.Fatalf("synthesize calls = %d, want 1", got)
	}
}
