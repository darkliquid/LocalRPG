package media

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type toneClient struct{}

func (toneClient) Synthesize(context.Context, string, *entity.VoiceConfig) ([]byte, error) {
	return GenerateToneWAV(440, 0.02), nil
}

// TestStreamedAndPlannedGroupsShareAKey proves the streamer's fold and the turn's
// clip plan name the same clip, so the finalise pass is a cache hit rather than a
// second provider call.
func TestStreamedAndPlannedGroupsShareAKey(t *testing.T) {
	pipeline := NewTTSPipeline(toneClient{}, NewContentCache(t.TempDir()))
	caps := TTSCapabilities{MaxSpeakers: 1}

	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The hall is quiet."},
		{Kind: entity.SegmentNarration, Text: "Cold air rushes in."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
	}

	planned := pipeline.GroupClipKeysWithCaps(segments, nil, nil, caps)

	folder := NewGroupFolder(caps, 0)
	var streamed []ClipGroup
	for _, segment := range segments {
		line, ok := pipeline.SegmentLine(segment, nil, nil)
		if !ok {
			continue
		}
		for _, group := range folder.Add(line) {
			streamed = append(streamed, ClipGroup{Lines: group})
		}
	}
	if group := folder.Flush(); group != nil {
		streamed = append(streamed, ClipGroup{Lines: group})
	}

	if len(streamed) != len(planned) {
		t.Fatalf("streamed %d groups, planned %d", len(streamed), len(planned))
	}
	for i := range planned {
		provider, model := groupKeyProvider(streamed[i].Lines)
		if got := ComputeGroupCacheKey(provider, model, streamed[i].Lines); got != planned[i].Key {
			t.Fatalf("group %d key = %q, planned %q", i, got, planned[i].Key)
		}
	}
}
