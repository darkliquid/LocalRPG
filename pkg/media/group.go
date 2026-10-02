package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

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

// groupKeyLine is the per-line payload a group key hashes. Every field that
// changes the provider prompt or the audio belongs here.
type groupKeyLine struct {
	Label      string                 `json:"label"`
	SpeakerID  string                 `json:"speaker_id"`
	Provider   string                 `json:"provider"`
	VoiceID    string                 `json:"voice_id"`
	Pitch      float64                `json:"pitch"`
	SpeechRate float64                `json:"speech_rate"`
	Options    map[string]interface{} `json:"options,omitempty"`
	Text       string                 `json:"text"`
}

// canonicalGroupLines coalesces consecutive lines of the same speaker into one,
// joining their text with a space. The provider receives the concatenation
// either way, so this makes the group key segmentation-stable for the common
// single-speaker run without reordering a multi-speaker transcript.
func canonicalGroupLines(lines []SpeakerLine) []SpeakerLine {
	canonical := make([]SpeakerLine, 0, len(lines))
	for _, line := range lines {
		if len(canonical) > 0 {
			last := &canonical[len(canonical)-1]
			if sameSpeaker(*last, line) {
				last.Text = strings.TrimSpace(last.Text + " " + line.Text)
				continue
			}
		}
		canonical = append(canonical, line)
	}
	return canonical
}

// sameSpeaker reports whether two lines belong to the same speaker. The speaker
// ID is authoritative when both carry one; otherwise the labels are compared.
func sameSpeaker(a, b SpeakerLine) bool {
	if a.SpeakerID != "" && b.SpeakerID != "" {
		return a.SpeakerID == b.SpeakerID
	}
	return a.Label == b.Label && a.Label != ""
}

// ComputeGroupCacheKey hashes the effective lines of a group. It is
// segmentation-stable: the same text in the same order under the same voices
// yields the same key regardless of how the turn was segmented, because
// consecutive same-speaker lines are coalesced first.
func ComputeGroupCacheKey(provider, model string, lines []SpeakerLine) string {
	canonical := canonicalGroupLines(lines)
	payload := make([]groupKeyLine, 0, len(canonical))
	for _, line := range canonical {
		entry := groupKeyLine{Label: line.Label, SpeakerID: line.SpeakerID, Text: line.Text}
		if line.Voice != nil {
			entry.Provider = line.Voice.Provider
			entry.VoiceID = line.Voice.VoiceID
			entry.Pitch = line.Voice.Pitch
			entry.SpeechRate = line.Voice.SpeechRate
			entry.Options = line.Voice.Options
		}
		payload = append(payload, entry)
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		// A slice of canonical scalars cannot fail to marshal; degrade rather
		// than panic so a hand-edited note never loses a turn.
		var builder strings.Builder
		for _, entry := range payload {
			builder.WriteString(entry.SpeakerID)
			builder.WriteByte('\x1f')
			builder.WriteString(entry.VoiceID)
			builder.WriteByte('\x1f')
			builder.WriteString(entry.Text)
			builder.WriteByte('\x1e')
		}
		encoded = []byte(builder.String())
	}

	hash := sha256.Sum256([]byte("v4:" + provider + "\x00" + model + "\x00" + string(encoded)))
	return hex.EncodeToString(hash[:])
}
