package media

import (
	"context"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// TTSCapabilities describes what a speech provider can render in one request. A
// zero value is the conservative single-voice, one-utterance provider that every
// TTSClient already is.
type TTSCapabilities struct {
	// MaxSpeakers is the number of distinct speakers one request may address.
	// Zero is treated as one.
	MaxSpeakers int
	// MaxCharsPerRequest bounds the speakable characters in one request. Zero
	// means unbounded or unknown.
	MaxCharsPerRequest int
	// MaxTokensPerRequest bounds the tokens in one request. Zero means unbounded
	// or unknown.
	MaxTokensPerRequest int
	// SupportsGrouping reports that the provider accepts a multi-segment,
	// single-speaker group in one request.
	SupportsGrouping bool
	// SupportsBatch reports that the provider implements BatchTTSClient.
	SupportsBatch bool
	// SupportsStreaming reports that the provider can stream audio within one
	// request. It is advertised but not yet used.
	SupportsStreaming bool
}

// TTSCapabilityReporter is implemented by a TTSClient that declares what it can
// render in one request. A client that does not implement it is treated as a
// single-voice, one-utterance provider.
type TTSCapabilityReporter interface {
	TTSCapabilities() TTSCapabilities
}

// SpeakerLine is one speaker's line within a group. Label is the display name a
// multi-speaker provider addresses the speaker by; it is part of the group key
// because it changes the provider prompt.
type SpeakerLine struct {
	SpeakerID string
	Label     string
	Voice     *entity.VoiceConfig
	Text      string
}

// GroupTTSClient is implemented by a TTSClient that can render a multi-speaker
// group in one request. A client that does not implement it is sent groups one
// speaker at a time through Synthesize.
type GroupTTSClient interface {
	SynthesizeGroup(ctx context.Context, lines []SpeakerLine) ([]byte, error)
	TTSCapabilities() TTSCapabilities
}

// ClipGroup is a maximal run of adjacent segments a provider renders in one
// request. SegmentIndexes index the turn's segment slice; Lines are the resolved
// speakers and text; Key is the content-addressed clip name.
type ClipGroup struct {
	SegmentIndexes []int
	Lines          []SpeakerLine
	Key            string
	Cached         bool
}

// ClientCapabilities reports a client's declared capabilities, defaulting to the
// single-voice provider every TTSClient is when it declares nothing.
func ClientCapabilities(client TTSClient) TTSCapabilities {
	if reporter, ok := client.(TTSCapabilityReporter); ok {
		return normalizeCaps(reporter.TTSCapabilities())
	}
	return normalizeCaps(TTSCapabilities{})
}

// normalizeCaps clamps a capability set to the invariants the pipeline relies on:
// at least one speaker, and no negative limits.
func normalizeCaps(caps TTSCapabilities) TTSCapabilities {
	if caps.MaxSpeakers < 1 {
		caps.MaxSpeakers = 1
	}
	if caps.MaxCharsPerRequest < 0 {
		caps.MaxCharsPerRequest = 0
	}
	if caps.MaxTokensPerRequest < 0 {
		caps.MaxTokensPerRequest = 0
	}
	return caps
}
