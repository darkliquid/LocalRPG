package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/dialogue"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media/opus"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// narratorSpeaker is the cache namespace for lines read in the narrator voice.
const narratorSpeaker = "narrator"

// audioExtensions are the names a cached clip may carry. Every clip is stored as
// Ogg/Opus, so there is exactly one.
var audioExtensions = []string{".opus"}

// AudioExtension names a clip from its bytes, because a provider returns whatever
// its engine produces rather than what the configuration implies.
func AudioExtension(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("OggS")):
		return ".opus"
	case bytes.HasPrefix(data, []byte("RIFF")):
		return ".wav"
	case bytes.HasPrefix(data, []byte("ID3")):
		return ".mp3"
	case len(data) > 1 && data[0] == 0xFF && data[1]&0xE0 == 0xE0:
		return ".mp3"
	case bytes.HasPrefix(data, []byte("fLaC")):
		return ".flac"
	default:
		return ".wav"
	}
}

// AudioContentType is the MIME type for a clip's bytes.
func AudioContentType(data []byte) string {
	switch AudioExtension(data) {
	case ".opus":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	default:
		return "audio/wav"
	}
}

// LegacySegments parses pre-segment turns at playback time. Speaker names are
// resolved permissively: legacy records never carried entity IDs, so any speaker
// the prose names is accepted.
func LegacySegments(narration string) []entity.TurnSegment {
	parsed := dialogue.Parse(narration, func(candidate string) (string, bool) {
		if strings.TrimSpace(candidate) == "" {
			return "", false
		}
		return "", true
	})

	segments := make([]entity.TurnSegment, 0, len(parsed))
	for _, segment := range parsed {
		kind := entity.SegmentNarration
		if segment.IsSpeech {
			kind = entity.SegmentSpeech
		}
		segments = append(segments, entity.TurnSegment{
			Kind:    kind,
			Speaker: segment.Speaker,
			Text:    segment.Text,
		})
	}
	return segments
}

type TTSClient interface {
	Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error)
}

// SpeechCueCapabilities describes the steering hints a TTS engine can interpret.
type SpeechCueCapabilities struct {
	AudioTags        bool     `json:"audio_tags"`
	MarkdownEmphasis bool     `json:"markdown_emphasis"`
	SupportedTags    []string `json:"supported_tags,omitempty"`
	PromptGuidance   string   `json:"prompt_guidance,omitempty"`
}

// SpeechCueAdvertiser is an optional interface implemented by TTS clients that
// declare vocal steering and performance cue capabilities.
type SpeechCueAdvertiser interface {
	SpeechCueCapabilities() SpeechCueCapabilities
}

// ResolveSpeechCueCapabilities resolves effective speech cue capabilities by combining
// provider-advertised capabilities with user configuration overrides.
func ResolveSpeechCueCapabilities(cfg config.TTSConfig, client TTSClient) SpeechCueCapabilities {
	var caps SpeechCueCapabilities
	if adv, ok := client.(SpeechCueAdvertiser); ok {
		caps = adv.SpeechCueCapabilities()
	} else if aware, ok := client.(MarkdownAware); ok && aware.SupportsMarkdown() {
		caps.MarkdownEmphasis = true
	}

	if !cfg.SpeechCues.Enabled && cfg.SpeechCues.AudioTags == nil && cfg.SpeechCues.MarkdownEmphasis == nil {
		caps.AudioTags = false
		caps.MarkdownEmphasis = false
	} else {
		if cfg.SpeechCues.AudioTags != nil {
			caps.AudioTags = *cfg.SpeechCues.AudioTags
		}
		if cfg.SpeechCues.MarkdownEmphasis != nil {
			caps.MarkdownEmphasis = *cfg.SpeechCues.MarkdownEmphasis
		}
	}
	return caps
}

type TTSPipeline struct {
	client      TTSClient
	cache       *ContentCache
	logger      trace.Logger
	policy      TextPolicy
	opusBitrate int
	// groupCaps are the capabilities grouping is planned against: the client's
	// own declaration, overlaid with any configured limits.
	groupCaps TTSCapabilities

	// flights serialize synthesis per cache key, so concurrent requests for the
	// same utterance synthesize and encode once instead of racing.
	flightMu sync.Mutex
	flights  map[string]*sync.Mutex
	// lastUsage is what the most recent synthesis consumed; a cache hit reports
	// zero so the caller records nothing.
	lastUsage Usage
	// repairs records cached clips that were not in the cache's format: either one that
	// was re-encoded in place, or one that was unreadable and discarded.
	repairs []string
}

// Repairs reports the cached clips that were not Ogg/Opus, so a caller can say what it had
// to fix rather than quietly serving something a browser might refuse.
func (p *TTSPipeline) Repairs() []string {
	p.flightMu.Lock()
	defer p.flightMu.Unlock()
	return append([]string(nil), p.repairs...)
}

// noteRepair records one clip that was not in the cache's format.
func (p *TTSPipeline) noteRepair(note string) {
	p.flightMu.Lock()
	defer p.flightMu.Unlock()
	if len(p.repairs) < maxReportedRepairs {
		p.repairs = append(p.repairs, note)
	}
}

// maxReportedRepairs bounds the report: a cache written by an older version could hold
// hundreds of clips in another format.
const maxReportedRepairs = 10

// SetOpusBitrate selects the on-disk Opus bitrate. Out-of-range values fall back
// to the default.
func (p *TTSPipeline) SetOpusBitrate(bitrate int) {
	if bitrate < opus.MinBitrate || bitrate > opus.MaxBitrate {
		bitrate = opus.DefaultBitrate
	}
	p.opusBitrate = bitrate
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (p *TTSPipeline) SetLogger(logger trace.Logger) {
	p.logger = trace.OrNil(logger)
}

// SetTextPolicy selects how narration Markdown is treated before synthesis. The
// zero value reduces Markdown unless the client is MarkdownAware.
func (p *TTSPipeline) SetTextPolicy(policy TextPolicy) {
	p.policy = policy
}

// SynthesizeSegments renders every segment as a list of clips, skipping a segment
// that reduces to no speakable text rather than treating it as a failure. Cached
// clips are reused; audio references stay out of the turn record because the cache
// key is a pure function of speaker, voice, prosody, and text.
func (p *TTSPipeline) SynthesizeSegments(ctx context.Context, segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) ([][]string, error) {
	clips := make([][]string, 0, len(segments))

	for _, segment := range segments {
		list, err := p.SynthesizeSegmentClips(ctx, segment, narratorVoice, voiceFor, false)
		if errors.Is(err, ErrNoSpeakableText) {
			continue
		}
		if err != nil {
			return nil, err
		}
		clips = append(clips, list)
	}

	return clips, nil
}

// clipUnits resolves a segment to the units synthesis reads: one sentence of the
// text this client is sent, or the whole text when splitting could corrupt it. It
// is the single definition of what a clip reads, shared by key computation and
// synthesis so the two can never disagree about which clip a segment needs.
func (p *TTSPipeline) clipUnits(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (speakerID string, voice *entity.VoiceConfig, units []string, err error) {
	speakerID, voice, spoken := p.prepareSegment(segment, narratorVoice, voiceFor)
	if strings.TrimSpace(spoken) == "" {
		return speakerID, voice, nil, ErrNoSpeakableText
	}
	if spoken != segment.Text {
		logger := trace.OrNil(p.logger)
		logger.Event("media.tts.reduced", map[string]interface{}{
			"chars_raw":    len([]rune(segment.Text)),
			"chars_spoken": len([]rune(spoken)),
		})
	}
	return speakerID, voice, p.sentencesFor(spoken), nil
}

// SegmentClipKeys reports the keys a segment's clips will have, without
// synthesizing anything, so a caller can name a clip before it exists.
func (p *TTSPipeline) SegmentClipKeys(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) ([]string, error) {
	speakerID, voice, units, err := p.clipUnits(segment, narratorVoice, voiceFor)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(units))
	for _, unit := range units {
		keys = append(keys, ComputeAudioCacheKeyForVoice(speakerID, voice, unit))
	}
	return keys, nil
}

// SynthesizeSegmentClips renders a segment as one clip per unit, in order. A unit
// that fails is skipped so its neighbours still play; only a segment that produced
// no clip at all reports the failure.
func (p *TTSPipeline) SynthesizeSegmentClips(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig, force bool) ([]string, error) {
	speakerID, voice, units, err := p.clipUnits(segment, narratorVoice, voiceFor)
	if err != nil {
		return nil, err
	}

	clips := make([]string, 0, len(units))
	var firstErr error
	for _, unit := range units {
		clip, err := p.SynthesizeUtteranceForce(ctx, speakerID, voice, unit, force)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		clips = append(clips, clip)
	}
	if len(clips) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return clips, nil
}

// prepareSegment resolves who reads a segment and in what voice, without
// synthesising. Both synthesis and the uncached count use it, so the two can
// never disagree about which clip a segment needs.
func (p *TTSPipeline) prepareSegment(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (speakerID string, voice *entity.VoiceConfig, spoken string) {
	spoken = SpeakableTextFor(p.policy, p.client, segment.Text)
	voice = narratorVoice
	speakerID = narratorSpeaker

	if segment.Kind == entity.SegmentSpeech {
		speakerID = segment.SpeakerID
		if speakerID == "" {
			speakerID = segment.Speaker
		}
		if speakerID == "" {
			speakerID = narratorSpeaker
		}
		if voiceFor != nil {
			if resolved := voiceFor(speakerID); resolved != nil {
				voice = resolved
			}
		}
	}
	return speakerID, voice, spoken
}

// SynthesizeProvisional renders a sentence of prose before the turn's final
// segments exist. It reads the same unit the finaliser will, so when the finished
// segment's text contains that sentence the final synthesis is a cache hit rather
// than a second provider call. Text that reduces to nothing returns
// ErrNoSpeakableText.
func (p *TTSPipeline) SynthesizeProvisional(ctx context.Context, kind, speakerID, text string, voice *entity.VoiceConfig) (string, error) {
	segment := entity.TurnSegment{Kind: kind, SpeakerID: speakerID, Text: text}
	speaker, resolved, spoken := p.prepareSegment(segment, voice, nil)
	if strings.TrimSpace(spoken) == "" {
		return "", ErrNoSpeakableText
	}
	return p.SynthesizeUtteranceForce(ctx, speaker, resolved, spoken, false)
}

// sentencesFor splits reduced text into sentences when doing so cannot change
// what a client hears. A client that is being sent Markdown keeps the whole
// segment, because emphasis or an audio tag can span what the splitter sees as a
// sentence boundary.
func (p *TTSPipeline) sentencesFor(reduced string) []string {
	if p.markdownPreserved() {
		return []string{reduced}
	}
	return SplitSentences(reduced)
}

// markdownPreserved reports whether the client receives the text's Markdown. It
// mirrors SpeakableTextFor's policy decision.
func (p *TTSPipeline) markdownPreserved() bool {
	switch p.policy {
	case TextPolicyKeep:
		return true
	case TextPolicyStrip:
		return false
	}
	aware, ok := p.client.(MarkdownAware)
	return ok && aware.SupportsMarkdown()
}

// CountUncached reports how many speakable segments already have every clip and
// how many would need synthesis, so a bulk operation can warn before spending
// money on a metered provider. It mutates nothing.
func (p *TTSPipeline) CountUncached(segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (cached, uncached int) {
	for _, segment := range segments {
		keys, err := p.SegmentClipKeys(segment, narratorVoice, voiceFor)
		if err != nil || len(keys) == 0 {
			continue
		}
		complete := true
		for _, key := range keys {
			if _, ok := p.cachedClip(key); !ok {
				complete = false
				break
			}
		}
		if complete {
			cached++
		} else {
			uncached++
		}
	}
	return cached, uncached
}

// ClipPath names the file a clip key is stored under, so a caller that holds a
// group key can find its audio.
func (p *TTSPipeline) ClipPath(key string) string {
	if key == "" {
		return ""
	}
	return filepath.Join(p.cache.Subdir("audio"), key+".opus")
}

// CountUncachedGroups reports how many groups already have their clip and how
// many would need synthesis, the grouped counterpart of CountUncached.
func (p *TTSPipeline) CountUncachedGroups(groups []ClipGroup) (cached, uncached int) {
	for _, group := range groups {
		if _, ok := p.cachedClip(group.Key); ok {
			cached++
		} else {
			uncached++
		}
	}
	return cached, uncached
}

// LastUsage reports what the most recent synthesis consumed. A cache hit or a
// provider that reports nothing yields the zero value.
func (p *TTSPipeline) LastUsage() Usage {	p.flightMu.Lock()
	defer p.flightMu.Unlock()
	return p.lastUsage
}

func (p *TTSPipeline) setLastUsage(u Usage) {
	p.flightMu.Lock()
	defer p.flightMu.Unlock()
	p.lastUsage = u
}

func NewTTSPipeline(client TTSClient, cache *ContentCache) *TTSPipeline {
	return &TTSPipeline{
		client:      client,
		cache:       cache,
		opusBitrate: opus.DefaultBitrate,
		flights:     map[string]*sync.Mutex{},
		groupCaps:   ClientCapabilities(client),
	}
}

// keyLock returns the mutex for one cache key, creating it on first use.
func (p *TTSPipeline) keyLock(key string) *sync.Mutex {
	p.flightMu.Lock()
	defer p.flightMu.Unlock()
	if p.flights == nil {
		p.flights = map[string]*sync.Mutex{}
	}
	lock, ok := p.flights[key]
	if !ok {
		lock = &sync.Mutex{}
		p.flights[key] = lock
	}
	return lock
}

func (p *TTSPipeline) SynthesizeUtterance(ctx context.Context, speakerID string, voice *entity.VoiceConfig, text string) (string, error) {
	return p.SynthesizeUtteranceForce(ctx, speakerID, voice, text, false)
}

func (p *TTSPipeline) SynthesizeUtteranceForce(ctx context.Context, speakerID string, voice *entity.VoiceConfig, text string, force bool) (string, error) {
	base := ComputeAudioCacheKeyForVoice(speakerID, voice, text)
	start := time.Now()

	voiceID, pitch, rate := "", 0.0, 0.0
	provider, model := "", ""
	if voice != nil {
		voiceID, pitch, rate = voice.VoiceID, voice.Pitch, voice.SpeechRate
		provider = voice.Provider
		if value, ok := voice.Options["model"].(string); ok {
			model = value
		}
	}
	logger := trace.OrNil(p.logger)
	logger.Event("media.tts.request", map[string]interface{}{
		"speaker":   speakerID,
		"voice_id":  voiceID,
		"provider":  provider,
		"model":     model,
		"pitch":     pitch,
		"rate":      rate,
		"chars":     len([]rune(text)),
		"cache_key": base,
	})

	if !force {
		if path, ok := p.cachedClip(base); ok {
			mediaMetrics().ttsCache.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("localrpg.cache.result", "hit")))
			mediaMetrics().ttsDuration.Record(ctx, float64(time.Since(start).Milliseconds()),
				otelmetric.WithAttributes(attribute.Bool("localrpg.cache.hit", true)))
			logger.Event("media.tts.result", map[string]interface{}{
				"cache_hit":   true,
				"duration_ms": time.Since(start).Milliseconds(),
			})
			p.setLastUsage(Usage{})
			return path, nil
		}
	}

	// Serialize per cache key: a concurrent request for the same utterance waits,
	// then takes the clip the first one wrote instead of synthesizing again.
	keyLock := p.keyLock(base)
	keyLock.Lock()
	defer keyLock.Unlock()
	if !force {
		if path, ok := p.cachedClip(base); ok {
			p.setLastUsage(Usage{})
			return path, nil
		}
	}

	audioBytes, err := p.client.Synthesize(ctx, text, voice)
	if err != nil {
		code := harness.ClassifyProviderError(err)
		logger.Event("media.tts.error", map[string]interface{}{
			"speaker":  speakerID,
			"provider": provider,
			"code":     string(code),
			"error":    err.Error(),
		})
		mediaMetrics().providerErrors.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.role", "tts"),
			attribute.String("error.kind", string(code)),
			attribute.String("gen_ai.system", provider),
		))
		return "", &harness.GenerationFailure{
			Code:    code,
			Message: fmt.Sprintf("synthesize utterance: %v", err),
			Cause:   err,
		}
	}

	// Normalise whatever the provider returned into Ogg/Opus, so the cache holds
	// exactly one format and playback decodes one codec.
	pcm, inRate, inChannels, decodeErr := DecodeProviderAudio(audioBytes, "")
	if decodeErr != nil {
		return "", &harness.GenerationFailure{
			Code:    harness.FailureProviderError,
			Message: fmt.Sprintf("normalise speech: %v", decodeErr),
			Cause:   decodeErr,
		}
	}
	encoded, encodeErr := opus.Encode(pcm, inRate, inChannels, p.opusBitrate)
	if encodeErr != nil {
		return "", &harness.GenerationFailure{
			Code:    harness.FailureProviderError,
			Message: fmt.Sprintf("encode speech: %v", encodeErr),
			Cause:   encodeErr,
		}
	}

	mediaMetrics().ttsCache.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("localrpg.cache.result", "miss")))
	mediaMetrics().ttsBytes.Record(ctx, int64(len(encoded)), otelmetric.WithAttributes(attribute.String("localrpg.tts.provider", provider)))
	mediaMetrics().ttsDuration.Record(ctx, float64(time.Since(start).Milliseconds()),
		otelmetric.WithAttributes(attribute.Bool("localrpg.cache.hit", false)))

	logger.Event("media.tts.result", map[string]interface{}{
		"cache_hit":    false,
		"bytes":        len(encoded),
		"content_type": "audio/ogg",
		"duration_ms":  time.Since(start).Milliseconds(),
	})

	p.setLastUsage(p.usageFor(text))
	return p.cache.Put("audio", base+".opus", encoded)
}

// usageFor reports what a synthesis consumed: the client's own report when it
// has one, otherwise an estimate from the spoken text.
func (p *TTSPipeline) usageFor(text string) Usage {
	if reporter, ok := p.client.(UsageReporter); ok {
		if u := reporter.LastUsage(); u != (Usage{}) {
			return u
		}
	}
	return Usage{Characters: len([]rune(text)), Requests: 1, Estimated: true}
}

// SetGroupCaps overrides the provider capabilities the pipeline plans groups
// with, so a caller can apply configured limits on top of the provider's own
// declaration.
func (p *TTSPipeline) SetGroupCaps(caps TTSCapabilities) {
	p.groupCaps = normalizeCaps(caps)
}

// GroupCaps reports the capabilities the pipeline plans groups with.
func (p *TTSPipeline) GroupCaps() TTSCapabilities {
	return p.groupCaps
}

// GroupClipKeys reports the clip keys a turn's groups will have, without
// synthesizing anything, so a caller can name a clip before it exists. It
// mirrors SegmentClipKeys for the grouped path.
func (p *TTSPipeline) GroupClipKeys(segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) []ClipGroup {
	return p.GroupClipKeysWithCaps(segments, narratorVoice, voiceFor, p.groupCaps)
}

// GroupClipKeysWithCaps is GroupClipKeys under an explicit capability set, so a
// caller can name the clips a live, single-speaker fold would write and match the
// audio the streamer already produced.
func (p *TTSPipeline) GroupClipKeysWithCaps(segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig, caps TTSCapabilities) []ClipGroup {
	groups := p.GroupPlan(segments, narratorVoice, voiceFor, caps)
	for i := range groups {
		provider, model := groupKeyProvider(groups[i].Lines)
		groups[i].Key = ComputeGroupCacheKey(provider, model, groups[i].Lines)
		if _, ok := p.cachedClip(groups[i].Key); ok {
			groups[i].Cached = true
		}
	}
	return groups
}

// SynthesizeGroups renders each uncached group with one provider call, in order.
// A group that fails is returned uncached rather than aborting the turn, so a
// caller can fall back to per-segment synthesis for just that group. It is the
// grouped counterpart of SynthesizeSegmentClips.
func (p *TTSPipeline) SynthesizeGroups(ctx context.Context, groups []ClipGroup) ([]ClipGroup, error) {
	return p.SynthesizeGroupsForce(ctx, groups, false)
}

// SynthesizeGroupsForce is SynthesizeGroups with an option to re-render a group
// even when its clip is cached, which is what a regenerate does.
func (p *TTSPipeline) SynthesizeGroupsForce(ctx context.Context, groups []ClipGroup, force bool) ([]ClipGroup, error) {
	rendered := make([]ClipGroup, 0, len(groups))
	var firstErr error
	for _, group := range groups {
		result, err := p.synthesizeGroup(ctx, group, force)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		rendered = append(rendered, result)
	}
	return rendered, firstErr
}

// SynthesizeTurn plans a turn's groups and renders them, the turn-level entry
// point for the grouped path.
func (p *TTSPipeline) SynthesizeTurn(ctx context.Context, segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) ([]ClipGroup, error) {
	return p.SynthesizeGroups(ctx, p.GroupClipKeys(segments, narratorVoice, voiceFor))
}

// synthesizeGroup renders one group, reusing a cached clip when present. The
// per-key single-flight lock means two callers requesting the same group
// synthesize it once.
func (p *TTSPipeline) synthesizeGroup(ctx context.Context, group ClipGroup, force bool) (ClipGroup, error) {
	start := time.Now()
	provider, model := groupKeyProvider(group.Lines)
	if group.Key == "" {
		group.Key = ComputeGroupCacheKey(provider, model, group.Lines)
	}

	logger := trace.OrNil(p.logger)
	logger.Event("media.tts.group_request", map[string]interface{}{
		"speaker":   groupSpeakerLabel(group.Lines),
		"provider":  provider,
		"model":     model,
		"speakers":  distinctSpeakers(group.Lines),
		"segments":  len(group.SegmentIndexes),
		"chars":     len([]rune(groupText(group.Lines))),
		"cache_key": group.Key,
	})

	keyLock := p.keyLock(group.Key)
	keyLock.Lock()
	defer keyLock.Unlock()

	if !force {
		if path, ok := p.cachedClip(group.Key); ok {
			_ = path
			group.Cached = true
			p.setLastUsage(Usage{})
			return group, nil
		}
	}

	audio, err := p.renderGroupAudio(ctx, group)
	if err != nil {
		code := harness.ClassifyProviderError(err)
		logger.Event("media.tts.group_error", map[string]interface{}{
			"provider": provider,
			"code":     string(code),
			"error":    err.Error(),
		})
		mediaMetrics().providerErrors.Add(ctx, 1, otelmetric.WithAttributes(
			attribute.String("localrpg.role", "tts"),
			attribute.String("error.kind", string(code)),
			attribute.String("gen_ai.system", provider),
		))
		return group, &harness.GenerationFailure{
			Code:    code,
			Message: fmt.Sprintf("synthesize group: %v", err),
			Cause:   err,
		}
	}

	if _, err := p.storeGroupClip(ctx, group.Key, audio, start, provider, groupText(group.Lines)); err != nil {
		return group, err
	}
	group.Cached = true
	return group, nil
}

// renderGroupAudio issues the one provider call a group needs: a single speaker
// is sent through Synthesize as the concatenated text, and a multi-speaker group
// through SynthesizeGroup when the provider supports it.
func (p *TTSPipeline) renderGroupAudio(ctx context.Context, group ClipGroup) ([]byte, error) {
	lines := canonicalGroupLines(group.Lines)
	if distinctSpeakers(lines) <= 1 {
		text := groupText(lines)
		var voice *entity.VoiceConfig
		if len(lines) > 0 {
			voice = lines[0].Voice
		}
		return p.client.Synthesize(ctx, text, voice)
	}

	groupClient, ok := p.client.(GroupTTSClient)
	if !ok {
		return nil, fmt.Errorf("tts: provider cannot render %d speakers in one request", distinctSpeakers(lines))
	}
	return groupClient.SynthesizeGroup(ctx, lines)
}

// storeGroupClip normalises provider audio to the cache's one format and writes
// it under a group key, mirroring the per-utterance path.
func (p *TTSPipeline) storeGroupClip(ctx context.Context, key string, audio []byte, start time.Time, provider, text string) (string, error) {
	pcm, inRate, inChannels, decodeErr := DecodeProviderAudio(audio, "")
	if decodeErr != nil {
		return "", &harness.GenerationFailure{
			Code:    harness.FailureProviderError,
			Message: fmt.Sprintf("normalise speech: %v", decodeErr),
			Cause:   decodeErr,
		}
	}
	encoded, encodeErr := opus.Encode(pcm, inRate, inChannels, p.opusBitrate)
	if encodeErr != nil {
		return "", &harness.GenerationFailure{
			Code:    harness.FailureProviderError,
			Message: fmt.Sprintf("encode speech: %v", encodeErr),
			Cause:   encodeErr,
		}
	}

	mediaMetrics().ttsCache.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("localrpg.cache.result", "miss")))
	mediaMetrics().ttsBytes.Record(ctx, int64(len(encoded)), otelmetric.WithAttributes(attribute.String("localrpg.tts.provider", provider)))
	mediaMetrics().ttsDuration.Record(ctx, float64(time.Since(start).Milliseconds()),
		otelmetric.WithAttributes(attribute.Bool("localrpg.cache.hit", false)))

	trace.OrNil(p.logger).Event("media.tts.result", map[string]interface{}{
		"cache_hit":    false,
		"bytes":        len(encoded),
		"content_type": "audio/ogg",
		"duration_ms":  time.Since(start).Milliseconds(),
		"group":        true,
	})

	p.setLastUsage(p.usageFor(text))
	return p.cache.Put("audio", key+".opus", encoded)
}

// groupKeyProvider names the provider and model a group's key is namespaced
// under, taken from the first line that carries a voice.
func groupKeyProvider(lines []SpeakerLine) (provider, model string) {
	for _, line := range lines {
		if line.Voice == nil {
			continue
		}
		provider = line.Voice.Provider
		if value, ok := line.Voice.Options["model"].(string); ok {
			model = value
		}
		return provider, model
	}
	return "", ""
}

// groupText is the speakable text a group sends, the canonical lines joined.
func groupText(lines []SpeakerLine) string {
	canonical := canonicalGroupLines(lines)
	parts := make([]string, 0, len(canonical))
	for _, line := range canonical {
		parts = append(parts, line.Text)
	}
	return strings.Join(parts, " ")
}

// groupSpeakerLabel names a group's speakers for a trace event.
func groupSpeakerLabel(lines []SpeakerLine) string {
	seen := make([]string, 0, len(lines))
	known := map[string]bool{}
	for _, line := range lines {
		key := speakerKey(line)
		if known[key] {
			continue
		}
		known[key] = true
		seen = append(seen, key)
	}
	return strings.Join(seen, "+")
}

// cachedClip finds a clip under any known extension, so a cache written under an
// older naming scheme is reused rather than regenerated. A clip that is not in the cache's
// format is repaired rather than thrown away: it may be the only copy of that audio, and
// decoding it needs no provider. Only a clip that cannot be repaired is discarded, and
// then it is re-synthesized.
func (p *TTSPipeline) cachedClip(base string) (string, bool) {
	for _, ext := range audioExtensions {
		if p.cache.Exists("audio", base+ext) {
			path := filepath.Join(p.cache.Subdir("audio"), base+ext)
			if !clipIsValid(path) {
				repaired, err := p.NormalizeClip(path)
				if err != nil {
					p.noteRepair(fmt.Sprintf("%s was not Ogg/Opus and could not be re-encoded (%v), so it was discarded", filepath.Base(path), err))
					_ = os.Remove(path)
					continue
				}
				p.noteRepair(fmt.Sprintf("%s was not Ogg/Opus and was re-encoded in place", filepath.Base(path)))
				return repaired, true
			}
			return path, true
		}
	}
	return "", false
}

// IsOpusClip reports whether data is an Ogg/Opus stream, which is the one format the
// cache stores. A file that is Ogg but not Opus (a Vorbis clip from an older cache, say)
// is not one, so it is repaired rather than served as something it is not.
func IsOpusClip(data []byte) bool {
	if !bytes.HasPrefix(data, []byte("OggS")) {
		return false
	}
	head := data[:min(len(data), opusHeadWindow)]
	return bytes.Contains(head, []byte("OpusHead"))
}

// opusHeadWindow is how far into an Ogg stream its Opus identification header can be: the
// first page carries it, well inside this.
const opusHeadWindow = 1024

// oggEndOfStream is the page header flag that marks the last page of a logical stream.
const oggEndOfStream = 0x04

// oggHeaderSize is one Ogg page header: capture pattern, version, flags, granule position,
// stream serial, page sequence, CRC, and the segment count.
const oggHeaderSize = 27

// IsCompleteOpusStream reports whether data is a single, whole, uncorrupted Ogg/Opus stream:
// every page is present and its checksum matches, all pages belong to one logical stream, and
// the page sequence runs from zero without gaps. A clip that fails this is refused by a
// browser - sometimes with "could not be decoded" - so it is treated as broken rather than
// served or exported. A write cut short by a killed process, and two writers interleaving
// their pages into one file, are both caught here.
//
// The end-of-stream flag is deliberately not required: this pipeline's own encoder does not
// set it, and browsers play those clips.
func IsCompleteOpusStream(data []byte) bool {
	if !IsOpusClip(data) {
		return false
	}

	var serial, sequence uint32
	for offset := 0; offset < len(data); {
		page := data[offset:]
		if len(page) < oggHeaderSize || !bytes.HasPrefix(page, []byte("OggS")) {
			return false
		}

		pageSerial := binary.LittleEndian.Uint32(page[14:18])
		pageSequence := binary.LittleEndian.Uint32(page[18:22])
		if offset == 0 {
			serial = pageSerial
		} else if pageSerial != serial {
			// A second logical stream in one file: a browser sees a chimeric stream.
			return false
		}
		if pageSequence != sequence {
			return false
		}
		sequence++

		segments := int(page[26])
		tableEnd := oggHeaderSize + segments
		if len(page) < tableEnd {
			return false
		}

		body := 0
		for _, lacing := range page[oggHeaderSize:tableEnd] {
			body += int(lacing)
		}
		pageEnd := tableEnd + body
		if len(page) < pageEnd {
			return false
		}

		// The checksum covers this page alone, with its own field zeroed, which is how
		// the muxer writes it.
		declared := binary.LittleEndian.Uint32(page[22:26])
		checksummed := append([]byte(nil), page[:pageEnd]...)
		for i := 22; i < 26; i++ {
			checksummed[i] = 0
		}
		if opus.OggCRC(checksummed) != declared {
			return false
		}

		offset += pageEnd
	}

	// The walk consumed the buffer exactly, so the stream ends on a page boundary.
	return true
}

// hasEndOfStream reports whether a stream's last page marks the end of the stream.
func hasEndOfStream(data []byte) bool {
	for offset := 0; offset < len(data); {
		page := data[offset:]
		if len(page) < oggHeaderSize {
			return false
		}
		segments := int(page[26])
		if len(page) < oggHeaderSize+segments {
			return false
		}
		body := 0
		for _, lacing := range page[oggHeaderSize : oggHeaderSize+segments] {
			body += int(lacing)
		}
		pageEnd := oggHeaderSize + segments + body
		if len(page) < pageEnd {
			return false
		}
		if offset+pageEnd >= len(data) {
			return page[5]&oggEndOfStream != 0
		}
		offset += pageEnd
	}
	return false
}

// WithoutEndOfStreamMarker returns the same clip as a build that did not mark the end of a
// stream wrote it: the flag cleared and the page's checksum kept valid. It exists for tests
// that need a clip an export must re-mux.
func WithoutEndOfStreamMarker(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	last := 0
	for offset := 0; offset < len(data); {
		segments := int(data[offset+26])
		body := 0
		for _, lacing := range data[offset+oggHeaderSize : offset+oggHeaderSize+segments] {
			body += int(lacing)
		}
		pageEnd := offset + oggHeaderSize + segments + body
		if pageEnd >= len(data) {
			last = offset
			break
		}
		offset = pageEnd
	}

	data[last+5] &^= oggEndOfStream
	page := append([]byte(nil), data[last:]...)
	for i := 22; i < 26; i++ {
		page[i] = 0
	}
	binary.LittleEndian.PutUint32(data[last+22:last+26], opus.OggCRC(page))
	return data, nil
}

// NormalizeClip returns a clip in the cache's one format, rewriting the file when it holds
// something else. Every clip this pipeline writes is already Ogg/Opus; this is for a file
// that reached the cache another way, such as a campaign cached before the Opus migration.
// A bundle must never carry audio a browser will refuse, and the repaired file is written
// back under its key so the app is fixed too.
func (p *TTSPipeline) NormalizeClip(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read clip %q: %w", filepath.Base(path), err)
	}
	if ClipProblem(data) == nil {
		return path, nil
	}

	pcm, rate, channels, err := decodeClip(data)
	if err != nil {
		return "", fmt.Errorf("decode clip %q: %w", filepath.Base(path), err)
	}
	encoded, err := opus.Encode(pcm, rate, channels, p.opusBitrate)
	if err != nil {
		return "", fmt.Errorf("encode clip %q: %w", filepath.Base(path), err)
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ".opus"
	return p.cache.Put("audio", name, encoded)
}

// decodeClip reads whatever a clip file holds, so a file in another format and a truncated
// Opus stream are both handled: Opus decodes page by page and reports a stream that ends
// early, which is what makes an interrupted write visible.
func decodeClip(data []byte) ([]int16, int, int, error) {
	if bytes.HasPrefix(data, []byte("OggS")) {
		return opus.Decode(data)
	}
	return DecodeProviderAudio(data, "")
}

// clipIsValid reports whether a cached clip is the one format the cache stores, whole and
// ready to play. A header alone is not enough: a stream cut short by an interrupted write
// still starts with a valid Opus header, and a browser refuses it.
func clipIsValid(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// The playback path pays for the structural checks and the checksum, not for a full
	// decode: a clip that reaches a player is decoded there anyway, and a whole stream
	// that fails to decode is vanishingly rare beside a write that was cut short.
	return IsCompleteOpusStream(data)
}

// ClipProblem reports what is wrong with a clip's bytes, or nil when they are a whole,
// decodable Ogg/Opus clip. It is the one answer to "can this be played", shared by the
// cache, an export, and an inspector, so all three agree about what a clip is.
func ClipProblem(data []byte) error {
	switch {
	case len(data) == 0:
		return errors.New("empty clip")
	case !bytes.HasPrefix(data, []byte("OggS")):
		return errors.New("not an Ogg stream")
	case !IsOpusClip(data):
		return errors.New("Ogg, but not Opus")
	case !IsCompleteOpusStream(data):
		return errors.New("Ogg/Opus stream is incomplete or corrupt")
	case !hasEndOfStream(data):
		// RFC 3533 requires the last page to mark the end of the stream. Chrome plays a
		// stream without it; stricter demuxers refuse the file outright, so it is not
		// "properly encoded" and an export re-encodes it.
		return errors.New("Ogg/Opus stream does not end properly")
	}
	if _, _, _, err := opus.Decode(data); err != nil {
		return fmt.Errorf("Opus stream does not decode: %w", err)
	}
	return nil
}
