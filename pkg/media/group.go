package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
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

// ResolveGroupCaps overlays the operator's configured limits on a client's
// declared capabilities, so a proxied or self-hosted endpoint whose limits
// cannot be queried can still be grouped safely.
func ResolveGroupCaps(cfg config.TTSConfig, client TTSClient) TTSCapabilities {
	caps := ClientCapabilities(client)
	if cfg.Limits != nil {
		if cfg.Limits.MaxSpeakers > 0 {
			caps.MaxSpeakers = cfg.Limits.MaxSpeakers
		}
		if cfg.Limits.MaxChars > 0 {
			caps.MaxCharsPerRequest = cfg.Limits.MaxChars
		}
		if cfg.Limits.MaxTokens > 0 {
			caps.MaxTokensPerRequest = cfg.Limits.MaxTokens
		}
	}
	// multi_speaker: off forces one speaker per request even where the provider
	// could render two.
	if strings.EqualFold(strings.TrimSpace(cfg.MultiSpeaker), "off") {
		caps.MaxSpeakers = 1
	}
	return normalizeCaps(caps)
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

// narratorLabel is the speaker label narration is addressed by in a
// multi-speaker transcript.
const narratorLabel = "Narrator"

// GroupPlan maps a turn's segments to groups under a provider's capabilities.
// It synthesizes nothing and is the one definition of which segments share a
// clip, shared by synthesis, the GUI DTO builder and export.
func (p *TTSPipeline) GroupPlan(segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig, caps TTSCapabilities) []ClipGroup {
	return planGroups(segments, caps, func(segment entity.TurnSegment) (SpeakerLine, bool) {
		return p.SegmentLine(segment, narratorVoice, voiceFor)
	})
}

// SegmentLine builds the line a segment is grouped and voiced as: its reduced
// text, its label, and its voice. It is the one definition of a line, so a
// streamed group and a finalised group share a cache key.
func (p *TTSPipeline) SegmentLine(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (SpeakerLine, bool) {
	speakerID, voice, spoken := p.prepareSegment(segment, narratorVoice, voiceFor)
	if strings.TrimSpace(spoken) == "" {
		return SpeakerLine{}, false
	}
	label := narratorLabel
	if segment.Kind == entity.SegmentSpeech {
		switch {
		case segment.Speaker != "":
			label = segment.Speaker
		case speakerID != "":
			label = speakerID
		}
	}
	return SpeakerLine{SpeakerID: speakerID, Label: label, Voice: voice, Text: spoken}, true
}

// GroupText is the text a group of lines is read as, joined with a space. It is
// what a streamed group reports as its display text.
func GroupText(lines []SpeakerLine) string { return groupText(lines) }

// planGroups is the pure grouping algorithm: it walks the segments in order,
// extending the current group while the speaker budget and the request limit
// allow, and splits a single oversized segment at sentence boundaries.
func planGroups(segments []entity.TurnSegment, caps TTSCapabilities, resolve func(entity.TurnSegment) (SpeakerLine, bool)) []ClipGroup {
	caps = normalizeCaps(caps)
	groups := make([]ClipGroup, 0, len(segments))
	var current *ClipGroup

	flush := func() {
		if current != nil {
			groups = append(groups, *current)
			current = nil
		}
	}

	for index, segment := range segments {
		line, ok := resolve(segment)
		if !ok {
			continue
		}

		// A segment too large for one request is split at sentence boundaries.
		if !linesFit([]SpeakerLine{line}, caps) {
			flush()
			for _, part := range splitLineToFit(line, caps) {
				groups = append(groups, ClipGroup{SegmentIndexes: []int{index}, Lines: []SpeakerLine{part}})
			}
			continue
		}

		if current != nil && !canJoinGroup(*current, line, caps) {
			flush()
		}
		if current == nil {
			current = &ClipGroup{}
		}
		current.SegmentIndexes = append(current.SegmentIndexes, index)
		current.Lines = append(current.Lines, line)
	}
	flush()
	return groups
}

// canJoinGroup reports whether a line may extend a group without exceeding the
// speaker budget or the provider's request limit.
func canJoinGroup(group ClipGroup, line SpeakerLine, caps TTSCapabilities) bool {
	if !groupHasSpeaker(group, line) {
		if distinctSpeakers(group.Lines) >= caps.MaxSpeakers {
			return false
		}
		// Two speakers sharing one voice cannot be told apart, so a multi-speaker
		// provider would read both in the same voice; keep them in separate groups.
		if sharesVoice(group.Lines, line) {
			return false
		}
	}
	candidate := make([]SpeakerLine, len(group.Lines), len(group.Lines)+1)
	copy(candidate, group.Lines)
	candidate = append(candidate, line)
	return linesFit(candidate, caps)
}

// sharesVoice reports whether a line's speaker voice is already used by a
// different speaker in the group.
func sharesVoice(lines []SpeakerLine, line SpeakerLine) bool {
	identity := voiceIdentity(line.Voice)
	for _, existing := range lines {
		if sameSpeaker(existing, line) {
			continue
		}
		if voiceIdentity(existing.Voice) == identity {
			return true
		}
	}
	return false
}

// voiceIdentity names a voice for the same-voice check: provider and voice ID, so
// two speakers configured with the same voice are recognised.
func voiceIdentity(voice *entity.VoiceConfig) string {
	if voice == nil {
		return ""
	}
	return voice.Provider + "\x00" + voice.VoiceID
}

// groupHasSpeaker reports whether a line's speaker is already in a group.
func groupHasSpeaker(group ClipGroup, line SpeakerLine) bool {
	for _, existing := range group.Lines {
		if sameSpeaker(existing, line) {
			return true
		}
	}
	return false
}

// distinctSpeakers counts the distinct speakers among a group's lines.
func distinctSpeakers(lines []SpeakerLine) int {
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		seen[speakerKey(line)] = true
	}
	return len(seen)
}

// speakerKey names a speaker for grouping: the entity ID when present, else the
// display label.
func speakerKey(line SpeakerLine) string {
	if line.SpeakerID != "" {
		return line.SpeakerID
	}
	return line.Label
}

// linesFit reports whether a run of lines is within the provider's request
// limits. A zero limit means unbounded.
func linesFit(lines []SpeakerLine, caps TTSCapabilities) bool {
	chars := 0
	tokens := 0
	for index, line := range lines {
		if index > 0 {
			chars++ // the space that joins one line to the next
		}
		chars += len([]rune(line.Text))
		tokens += estimateTokens(line.Text)
	}
	if caps.MaxCharsPerRequest > 0 && chars > caps.MaxCharsPerRequest {
		return false
	}
	if caps.MaxTokensPerRequest > 0 && tokens > caps.MaxTokensPerRequest {
		return false
	}
	return true
}

// estimateTokens approximates a text's token count for grouping only, at the
// common four-characters-per-token ratio. The provider's exact tokenizer is not
// available here, so this bounds a request conservatively.
func estimateTokens(text string) int {
	return (len([]rune(text)) + 3) / 4
}

// splitLineToFit divides an oversized line into the fewest sentence-aligned
// parts that each fit, never splitting mid-sentence. A single sentence larger
// than the limit is returned alone; the provider rejects it and the bisection
// failure path takes over.
func splitLineToFit(line SpeakerLine, caps TTSCapabilities) []SpeakerLine {
	sentences := SplitSentences(line.Text)
	if len(sentences) <= 1 {
		return []SpeakerLine{line}
	}

	parts := make([]SpeakerLine, 0, len(sentences))
	current := ""
	for _, sentence := range sentences {
		if current == "" {
			current = sentence
			continue
		}
		candidate := current + " " + sentence
		if !linesFit([]SpeakerLine{{Text: candidate}}, caps) {
			parts = append(parts, withText(line, current))
			current = sentence
			continue
		}
		current = candidate
	}
	if current != "" {
		parts = append(parts, withText(line, current))
	}
	return parts
}

// withText copies a line with replacement text.
func withText(line SpeakerLine, text string) SpeakerLine {
	line.Text = text
	return line
}

// GroupForSegment returns the group that covers a segment index, so a caller
// that holds a segment can find the clip it shares with its neighbours.
func GroupForSegment(groups []ClipGroup, index int) (ClipGroup, bool) {
	for _, group := range groups {
		for _, segmentIndex := range group.SegmentIndexes {
			if segmentIndex == index {
				return group, true
			}
		}
	}
	return ClipGroup{}, false
}
