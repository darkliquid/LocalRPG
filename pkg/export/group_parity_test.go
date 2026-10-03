package export

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

// The export and the app must resolve the same clip keys for a turn, or a bundle
// re-synthesizes audio the app already has and the two disagree about what a beat
// sounds like.
func TestTurnAudioMatchesPipelineGroupKeys(t *testing.T) {
	cache := media.NewContentCache(t.TempDir())
	pipeline := media.NewTTSPipeline(toneTTS{}, cache)
	narrator := &entity.VoiceConfig{VoiceID: "Aoede"}
	resolver := NewSpeechResolver(pipeline, nil, narrator).(*speechResolver)
	resolver.SetGrouped(true)

	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The door opens."},
		{Kind: entity.SegmentNarration, Text: "Cold air rushes in."},
	}

	groups, err := resolver.TurnAudio(context.Background(), segments)
	if err != nil {
		t.Fatalf("TurnAudio: %v", err)
	}
	want := pipeline.GroupClipKeys(segments, narrator, nil)
	if len(groups) != len(want) {
		t.Fatalf("TurnAudio returned %d groups, the pipeline planned %d", len(groups), len(want))
	}
	for i := range want {
		if groups[i].Key != want[i].Key {
			t.Errorf("group %d: export key %q, pipeline key %q", i, groups[i].Key, want[i].Key)
		}
	}
	if len(groups) != 1 {
		t.Errorf("expected the two narration segments to share one group, got %d", len(groups))
	}
	if len(groups) == 1 && len(groups[0].AudioPaths) == 0 {
		t.Errorf("expected the group to carry a clip")
	}
}

// A resolver that is not grouped must not render groups, so the export falls back
// to per-beat resolution.
func TestTurnAudioIsNilWhenUngrouped(t *testing.T) {
	cache := media.NewContentCache(t.TempDir())
	resolver := NewSpeechResolver(media.NewTTSPipeline(toneTTS{}, cache), nil, nil).(*speechResolver)

	groups, err := resolver.TurnAudio(context.Background(), []entity.TurnSegment{{Kind: entity.SegmentNarration, Text: "A."}})
	if err != nil {
		t.Fatalf("TurnAudio: %v", err)
	}
	if groups != nil {
		t.Errorf("expected no groups when ungrouped, got %#v", groups)
	}
}
