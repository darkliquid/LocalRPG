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
