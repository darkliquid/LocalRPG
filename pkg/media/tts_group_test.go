package media

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// countingGroupClient counts how many single-voice requests a turn issues.
type countingGroupClient struct {
	calls int
	caps  TTSCapabilities
}

func (c *countingGroupClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	c.calls++
	return GenerateToneWAV(440, 0.02), nil
}

func (c *countingGroupClient) TTSCapabilities() TTSCapabilities { return c.caps }

func groupPipeline(t *testing.T, client TTSClient, caps TTSCapabilities) *TTSPipeline {
	t.Helper()
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	pipeline.SetGroupCaps(caps)
	return pipeline
}

func TestSynthesizeTurnIssuesOneRequestPerGroup(t *testing.T) {
	client := &countingGroupClient{caps: TTSCapabilities{MaxSpeakers: 1}}
	pipeline := groupPipeline(t, client, client.caps)

	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The door opens."},
		{Kind: entity.SegmentNarration, Text: "Cold air rushes in."},
		{Kind: entity.SegmentNarration, Text: "He shivers."},
	}
	groups, err := pipeline.SynthesizeTurn(context.Background(), segments, nil, nil)
	if err != nil {
		t.Fatalf("SynthesizeTurn: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if client.calls != 1 {
		t.Errorf("expected 1 synthesis call for the whole narration, got %d", client.calls)
	}
	if !groups[0].Cached {
		t.Errorf("expected the group to be cached after synthesis")
	}
}

func TestSynthesizeTurnSeparatesSpeakersWhenSingleVoice(t *testing.T) {
	client := &countingGroupClient{caps: TTSCapabilities{MaxSpeakers: 1}}
	pipeline := groupPipeline(t, client, client.caps)

	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The door opens."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
	}
	groups, err := pipeline.SynthesizeTurn(context.Background(), segments, nil, nil)
	if err != nil {
		t.Fatalf("SynthesizeTurn: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if client.calls != 2 {
		t.Errorf("expected 2 synthesis calls, got %d", client.calls)
	}
}

func TestSynthesizeTurnReusesCachedGroup(t *testing.T) {
	client := &countingGroupClient{caps: TTSCapabilities{MaxSpeakers: 1}}
	pipeline := groupPipeline(t, client, client.caps)
	segments := []entity.TurnSegment{{Kind: entity.SegmentNarration, Text: "The door opens."}}

	if _, err := pipeline.SynthesizeTurn(context.Background(), segments, nil, nil); err != nil {
		t.Fatalf("first SynthesizeTurn: %v", err)
	}
	if _, err := pipeline.SynthesizeTurn(context.Background(), segments, nil, nil); err != nil {
		t.Fatalf("second SynthesizeTurn: %v", err)
	}
	if client.calls != 1 {
		t.Errorf("expected the second turn to reuse the cached group, got %d calls", client.calls)
	}
}

func TestSynthesizeTurnUsesGroupClientForTwoSpeakers(t *testing.T) {
	client := &capGroupClient{caps: TTSCapabilities{MaxSpeakers: 2, SupportsGrouping: true}}
	pipeline := groupPipeline(t, client, client.caps)

	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The door opens."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
	}
	// Two speakers must sound different to share a multi-speaker request.
	voiceFor := func(speakerID string) *entity.VoiceConfig {
		if speakerID == "garrick" {
			return &entity.VoiceConfig{VoiceID: "Kore"}
		}
		return nil
	}
	groups, err := pipeline.SynthesizeTurn(context.Background(), segments, &entity.VoiceConfig{VoiceID: "Aoede"}, voiceFor)
	if err != nil {
		t.Fatalf("SynthesizeTurn: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(client.group) != 1 {
		t.Fatalf("expected one SynthesizeGroup call, got %d", len(client.group))
	}
	if len(client.group[0]) != 2 {
		t.Errorf("expected two speaker lines, got %#v", client.group[0])
	}
}

func TestGroupClipKeysMatchSynthesizedKeys(t *testing.T) {
	client := &countingGroupClient{caps: TTSCapabilities{MaxSpeakers: 1}}
	pipeline := groupPipeline(t, client, client.caps)

	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "A."},
		{Kind: entity.SegmentNarration, Text: "B."},
	}
	keys := pipeline.GroupClipKeys(segments, nil, nil)
	groups, err := pipeline.SynthesizeTurn(context.Background(), segments, nil, nil)
	if err != nil {
		t.Fatalf("SynthesizeTurn: %v", err)
	}
	if len(keys) != len(groups) {
		t.Fatalf("GroupClipKeys returned %d groups, synthesis returned %d", len(keys), len(groups))
	}
	for i := range keys {
		if keys[i].Key != groups[i].Key {
			t.Errorf("group %d: predicted key %q, synthesized key %q", i, keys[i].Key, groups[i].Key)
		}
	}
}

func TestSynthesizeTurnGroupsByCharacterLimit(t *testing.T) {
	client := &countingGroupClient{caps: TTSCapabilities{MaxSpeakers: 1, MaxCharsPerRequest: 20}}
	pipeline := groupPipeline(t, client, client.caps)

	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "One two three."},
		{Kind: entity.SegmentNarration, Text: "Four five six."},
		{Kind: entity.SegmentNarration, Text: "Seven eight."},
	}
	groups, err := pipeline.SynthesizeTurn(context.Background(), segments, nil, nil)
	if err != nil {
		t.Fatalf("SynthesizeTurn: %v", err)
	}
	if len(groups) < 2 {
		t.Fatalf("expected the character limit to split the run, got %d groups", len(groups))
	}
	if client.calls != len(groups) {
		t.Errorf("expected one call per group (%d), got %d", len(groups), client.calls)
	}
}
