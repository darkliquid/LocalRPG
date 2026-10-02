package media

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
)

// plainGroupClient implements only TTSClient, declaring no capabilities.
type plainGroupClient struct{}

func (plainGroupClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return GenerateToneWAV(440, 0.01), nil
}

// capGroupClient declares capabilities and implements GroupTTSClient.
type capGroupClient struct {
	caps  TTSCapabilities
	group [][]SpeakerLine
}

func (c *capGroupClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return GenerateToneWAV(440, 0.01), nil
}

func (c *capGroupClient) SynthesizeGroup(ctx context.Context, lines []SpeakerLine) ([]byte, error) {
	c.group = append(c.group, lines)
	return GenerateToneWAV(440, 0.01), nil
}

func (c *capGroupClient) TTSCapabilities() TTSCapabilities { return c.caps }

func TestClientCapabilitiesDefaultsToSingleVoice(t *testing.T) {
	caps := ClientCapabilities(plainGroupClient{})
	if caps.MaxSpeakers != 1 {
		t.Errorf("MaxSpeakers = %d, want 1", caps.MaxSpeakers)
	}
	if caps.SupportsGrouping || caps.SupportsBatch || caps.SupportsStreaming {
		t.Errorf("unexpected capabilities %#v", caps)
	}
}

func TestClientCapabilitiesReadsTheReporter(t *testing.T) {
	client := &capGroupClient{caps: TTSCapabilities{MaxSpeakers: 2, MaxCharsPerRequest: 4000, SupportsGrouping: true}}
	caps := ClientCapabilities(client)
	if caps.MaxSpeakers != 2 || caps.MaxCharsPerRequest != 4000 || !caps.SupportsGrouping {
		t.Errorf("unexpected capabilities %#v", caps)
	}
}

func TestClientCapabilitiesClampsSpeakersAndLimits(t *testing.T) {
	client := &capGroupClient{caps: TTSCapabilities{MaxSpeakers: 0, MaxCharsPerRequest: -5, MaxTokensPerRequest: -1}}
	caps := ClientCapabilities(client)
	if caps.MaxSpeakers != 1 || caps.MaxCharsPerRequest != 0 || caps.MaxTokensPerRequest != 0 {
		t.Errorf("unexpected capabilities %#v", caps)
	}
}

func TestResolveGroupCapsOverlaysConfiguredLimits(t *testing.T) {
	client := &capGroupClient{caps: TTSCapabilities{MaxSpeakers: 2, MaxCharsPerRequest: 4000, SupportsGrouping: true}}
	cfg := config.TTSConfig{Limits: &config.TTSLimits{MaxChars: 1000, MaxSpeakers: 1}}
	caps := ResolveGroupCaps(cfg, client)

	if caps.MaxCharsPerRequest != 1000 {
		t.Errorf("MaxCharsPerRequest = %d, want 1000", caps.MaxCharsPerRequest)
	}
	if caps.MaxSpeakers != 1 {
		t.Errorf("MaxSpeakers = %d, want 1", caps.MaxSpeakers)
	}
	if !caps.SupportsGrouping {
		t.Errorf("expected the provider's grouping capability to survive the overlay")
	}
}

func TestComputeGroupCacheKeyIsSegmentationStable(t *testing.T) {
	one := []SpeakerLine{{SpeakerID: "narrator", Label: "Narrator", Text: "A. B."}}
	two := []SpeakerLine{
		{SpeakerID: "narrator", Label: "Narrator", Text: "A."},
		{SpeakerID: "narrator", Label: "Narrator", Text: "B."},
	}
	if ComputeGroupCacheKey("tts:gemini", "gemini-3.8-flash-tts", one) != ComputeGroupCacheKey("tts:gemini", "gemini-3.8-flash-tts", two) {
		t.Errorf("expected the same key for the same text under different segmentation")
	}
}

func TestComputeGroupCacheKeyPreservesSpeakerOrder(t *testing.T) {
	a := []SpeakerLine{
		{SpeakerID: "narrator", Label: "Narrator", Text: "The door opens."},
		{SpeakerID: "garrick", Label: "Garrick", Text: "Keep walking."},
	}
	b := []SpeakerLine{
		{SpeakerID: "garrick", Label: "Garrick", Text: "Keep walking."},
		{SpeakerID: "narrator", Label: "Narrator", Text: "The door opens."},
	}
	if ComputeGroupCacheKey("tts:gemini", "gemini-3.8-flash-tts", a) == ComputeGroupCacheKey("tts:gemini", "gemini-3.8-flash-tts", b) {
		t.Errorf("expected a different key when speaker order changes")
	}
}

func TestComputeGroupCacheKeyChangesWithVoice(t *testing.T) {
	base := []SpeakerLine{{SpeakerID: "narrator", Label: "Narrator", Voice: &entity.VoiceConfig{VoiceID: "Aoede", Pitch: 1, SpeechRate: 1}, Text: "Hello."}}
	baseKey := ComputeGroupCacheKey("tts:gemini", "gemini-3.8-flash-tts", base)

	cases := map[string][]SpeakerLine{
		"voice id": {{SpeakerID: "narrator", Label: "Narrator", Voice: &entity.VoiceConfig{VoiceID: "Kore", Pitch: 1, SpeechRate: 1}, Text: "Hello."}},
		"pitch":    {{SpeakerID: "narrator", Label: "Narrator", Voice: &entity.VoiceConfig{VoiceID: "Aoede", Pitch: 1.2, SpeechRate: 1}, Text: "Hello."}},
		"options":  {{SpeakerID: "narrator", Label: "Narrator", Voice: &entity.VoiceConfig{VoiceID: "Aoede", Pitch: 1, SpeechRate: 1, Options: map[string]interface{}{"direction": "weary"}}, Text: "Hello."}},
		"label":    {{SpeakerID: "narrator", Label: "The Narrator", Voice: &entity.VoiceConfig{VoiceID: "Aoede", Pitch: 1, SpeechRate: 1}, Text: "Hello."}},
		"text":     {{SpeakerID: "narrator", Label: "Narrator", Voice: &entity.VoiceConfig{VoiceID: "Aoede", Pitch: 1, SpeechRate: 1}, Text: "Goodbye."}},
	}
	for name, lines := range cases {
		if ComputeGroupCacheKey("tts:gemini", "gemini-3.8-flash-tts", lines) == baseKey {
			t.Errorf("expected the %s change to alter the key", name)
		}
	}

	if ComputeGroupCacheKey("tts:gemini", "gemini-3.8-flash-tts-other", base) == baseKey {
		t.Errorf("expected a model change to alter the key")
	}
	if ComputeGroupCacheKey("tts:elevenlabs", "gemini-3.8-flash-tts", base) == baseKey {
		t.Errorf("expected a provider change to alter the key")
	}
}

func TestComputeGroupCacheKeyHandlesNilVoice(t *testing.T) {
	lines := []SpeakerLine{{SpeakerID: "narrator", Label: "Narrator", Text: "Hello."}}
	if ComputeGroupCacheKey("tts:gemini", "gemini-3.8-flash-tts", lines) == "" {
		t.Errorf("expected a key for a nil voice")
	}
}

// simpleResolver mirrors the pipeline's speaker and label resolution without
// needing a client or a text policy.
func simpleResolver(segment entity.TurnSegment) (SpeakerLine, bool) {
	if strings.TrimSpace(segment.Text) == "" {
		return SpeakerLine{}, false
	}
	speakerID := narratorSpeaker
	label := narratorLabel
	if segment.Kind == entity.SegmentSpeech {
		speakerID = segment.SpeakerID
		if speakerID == "" {
			speakerID = segment.Speaker
		}
		label = segment.Speaker
		if label == "" {
			label = speakerID
		}
	}
	return SpeakerLine{SpeakerID: speakerID, Label: label, Text: segment.Text}, true
}

func narration(text string) entity.TurnSegment {
	return entity.TurnSegment{Kind: entity.SegmentNarration, Text: text}
}

func speech(speaker, text string) entity.TurnSegment {
	return entity.TurnSegment{Kind: entity.SegmentSpeech, Speaker: speaker, SpeakerID: strings.ToLower(speaker), Text: text}
}

func TestPlanGroupsMergesAdjacentSameSpeaker(t *testing.T) {
	segments := []entity.TurnSegment{narration("A."), narration("B."), narration("C.")}
	groups := planGroups(segments, TTSCapabilities{MaxSpeakers: 1}, simpleResolver)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d: %#v", len(groups), groups)
	}
	if len(groups[0].SegmentIndexes) != 3 {
		t.Errorf("expected 3 segments in the group, got %v", groups[0].SegmentIndexes)
	}
}

func TestPlanGroupsStartsANewGroupPerSpeakerWhenSingleVoice(t *testing.T) {
	segments := []entity.TurnSegment{narration("The door opens."), speech("Garrick", "Keep walking."), narration("He points on.")}
	groups := planGroups(segments, TTSCapabilities{MaxSpeakers: 1}, simpleResolver)

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %#v", len(groups), groups)
	}
	if groups[0].Lines[0].SpeakerID != narratorSpeaker || groups[1].Lines[0].SpeakerID != "garrick" {
		t.Errorf("unexpected group speakers %#v", groups)
	}
}

func TestPlanGroupsPairsTwoSpeakers(t *testing.T) {
	segments := []entity.TurnSegment{narration("The door opens."), speech("Garrick", "Keep walking."), speech("Mira", "Wait.")}
	resolver := func(segment entity.TurnSegment) (SpeakerLine, bool) {
		line, ok := simpleResolver(segment)
		if ok {
			line.Voice = &entity.VoiceConfig{VoiceID: line.SpeakerID}
		}
		return line, ok
	}
	groups := planGroups(segments, TTSCapabilities{MaxSpeakers: 2}, resolver)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %#v", len(groups), groups)
	}
	if len(groups[0].SegmentIndexes) != 2 {
		t.Errorf("expected the narrator and Garrick to pair, got %v", groups[0].SegmentIndexes)
	}
	if len(groups[1].SegmentIndexes) != 1 || groups[1].Lines[0].SpeakerID != "mira" {
		t.Errorf("expected Mira to start a new group, got %#v", groups[1])
	}
}

func TestPlanGroupsSplitsAnOversizedSegmentAtSentenceBoundaries(t *testing.T) {
	segments := []entity.TurnSegment{narration("Alpha beta. Gamma delta. Epsilon zeta.")}
	caps := TTSCapabilities{MaxSpeakers: 1, MaxCharsPerRequest: 25}
	groups := planGroups(segments, caps, simpleResolver)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %#v", len(groups), groups)
	}
	if groups[0].Lines[0].Text != "Alpha beta. Gamma delta." {
		t.Errorf("first part = %q, want whole sentences", groups[0].Lines[0].Text)
	}
	if groups[1].Lines[0].Text != "Epsilon zeta." {
		t.Errorf("second part = %q, want whole sentences", groups[1].Lines[0].Text)
	}
	for _, group := range groups {
		if !strings.HasSuffix(group.Lines[0].Text, ".") {
			t.Errorf("part %q does not end a sentence", group.Lines[0].Text)
		}
	}
}

func TestPlanGroupsSkipsEmptySegmentsWithoutBreakingAdjacency(t *testing.T) {
	segments := []entity.TurnSegment{narration("A."), narration("   "), narration("B.")}
	groups := planGroups(segments, TTSCapabilities{MaxSpeakers: 1}, simpleResolver)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d: %#v", len(groups), groups)
	}
	if len(groups[0].SegmentIndexes) != 2 || groups[0].SegmentIndexes[0] != 0 || groups[0].SegmentIndexes[1] != 2 {
		t.Errorf("expected segments 0 and 2, got %v", groups[0].SegmentIndexes)
	}
}

func TestPlanGroupsSeparatesSpeakersSharingAVoice(t *testing.T) {
	shared := &entity.VoiceConfig{VoiceID: "Aoede"}
	segments := []entity.TurnSegment{narration("The door opens."), speech("Garrick", "Keep walking.")}
	resolver := func(segment entity.TurnSegment) (SpeakerLine, bool) {
		line, ok := simpleResolver(segment)
		line.Voice = shared
		return line, ok
	}
	groups := planGroups(segments, TTSCapabilities{MaxSpeakers: 2}, resolver)

	if len(groups) != 2 {
		t.Fatalf("expected two speakers sharing a voice to render separately, got %d groups", len(groups))
	}
}

func TestResolveGroupCapsHonoursMultiSpeakerOff(t *testing.T) {
	client := &capGroupClient{caps: TTSCapabilities{MaxSpeakers: 2, SupportsGrouping: true}}

	if caps := ResolveGroupCaps(config.TTSConfig{MultiSpeaker: "off"}, client); caps.MaxSpeakers != 1 {
		t.Errorf("MaxSpeakers = %d, want 1 when multi_speaker is off", caps.MaxSpeakers)
	}
	if caps := ResolveGroupCaps(config.TTSConfig{MultiSpeaker: "auto"}, client); caps.MaxSpeakers != 2 {
		t.Errorf("MaxSpeakers = %d, want 2 when multi_speaker is auto", caps.MaxSpeakers)
	}
}
