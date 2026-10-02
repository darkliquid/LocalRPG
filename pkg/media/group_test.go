package media

import (
	"context"
	"testing"

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
