package media

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestCountUncached(t *testing.T) {
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(&echoTTSClient{}, cache)

	narrator := &entity.VoiceConfig{VoiceID: "af_bella"}
	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The gate stands open."},
		{Kind: entity.SegmentSpeech, SpeakerID: "aldric", Text: "Hold the line."},
		{Kind: entity.SegmentNarration, Text: "***"},
	}

	cached, uncached := pipeline.CountUncached(segments, narrator, nil)
	if cached != 0 || uncached != 2 {
		t.Fatalf("cached = %d, uncached = %d, want 0 and 2", cached, uncached)
	}

	// Synthesising one segment brings the count down by exactly one.
	if _, err := pipeline.SynthesizeSegment(context.Background(), segments[0], narrator, nil); err != nil {
		t.Fatalf("SynthesizeSegment: %v", err)
	}
	cached, uncached = pipeline.CountUncached(segments, narrator, nil)
	if cached != 1 || uncached != 1 {
		t.Errorf("cached = %d, uncached = %d, want 1 and 1", cached, uncached)
	}
}
