package gui

import (
	"context"
	"sync"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/trace"
)

type fakeSentenceTTSClient struct {
	mu    sync.Mutex
	calls int
}

func (c *fakeSentenceTTSClient) Synthesize(context.Context, string, *entity.VoiceConfig) ([]byte, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return media.GenerateToneWAV(440, 0.01), nil
}

func (c *fakeSentenceTTSClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestSentenceStreamerSynthesizesCompleteSentencesOnly(t *testing.T) {
	client := &fakeSentenceTTSClient{}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(t.TempDir()))
	streamer := newSentenceStreamer(context.Background(), pipeline, &entity.VoiceConfig{VoiceID: "v1"}, trace.Nop(), 1)

	streamer.Feed("The hall is quiet. Garrick")
	streamer.Feed(" steps inside.")
	streamer.Close()

	if got := client.callCount(); got != 2 {
		t.Fatalf("calls = %d, want 2 complete sentences", got)
	}
}

func TestSentenceStreamerNilIsSafe(t *testing.T) {
	var streamer *sentenceStreamer
	streamer.Feed("text")
	streamer.Close()
}
