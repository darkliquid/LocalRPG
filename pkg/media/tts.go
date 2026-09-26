package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

	// flights serialize synthesis per cache key, so concurrent requests for the
	// same utterance synthesize and encode once instead of racing.
	flightMu sync.Mutex
	flights  map[string]*sync.Mutex
}

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

// SynthesizeSegments renders every segment with its speaker's voice, falling back
// to the narrator voice for narration and unresolved speech. Cached clips are
// reused; audio references stay out of the turn record because the cache key is a
// pure function of speaker, voice, prosody, and text. A segment that reduces to no
// speakable text is skipped rather than treated as a failure.
func (p *TTSPipeline) SynthesizeSegments(ctx context.Context, segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) ([]string, error) {
	clips := make([]string, 0, len(segments))

	for _, segment := range segments {
		clip, err := p.SynthesizeSegment(ctx, segment, narratorVoice, voiceFor)
		if errors.Is(err, ErrNoSpeakableText) {
			continue
		}
		if err != nil {
			return nil, err
		}
		clips = append(clips, clip)
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

// SynthesizeSegment renders one segment: narration and unresolved speech read in
// the narrator voice, resolved speech in the speaker's own. Legacy records carry a
// speaker name but no entity ID, so the name is tried as a voice key too.
func (p *TTSPipeline) SynthesizeSegment(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (string, error) {
	return p.SynthesizeSegmentForce(ctx, segment, narratorVoice, voiceFor, false)
}

// SynthesizeSegmentForce renders one segment with optional force cache bypass.
func (p *TTSPipeline) SynthesizeSegmentForce(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig, force bool) (string, error) {
	speakerID, voice, spoken := p.prepareSegment(segment, narratorVoice, voiceFor)
	if strings.TrimSpace(spoken) == "" {
		return "", ErrNoSpeakableText
	}
	if spoken != segment.Text {
		logger := trace.OrNil(p.logger)
		logger.Event("media.tts.reduced", map[string]interface{}{
			"chars_raw":    len([]rune(segment.Text)),
			"chars_spoken": len([]rune(spoken)),
		})
	}
	return p.SynthesizeUtteranceForce(ctx, speakerID, voice, spoken, force)
}

// CountUncached reports how many speakable segments already have a clip and how
// many would need synthesis, so a bulk operation can warn before spending money
// on a metered provider. It mutates nothing.
func (p *TTSPipeline) CountUncached(segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (cached, uncached int) {
	for _, segment := range segments {
		speakerID, voice, spoken := p.prepareSegment(segment, narratorVoice, voiceFor)
		if strings.TrimSpace(spoken) == "" {
			continue
		}
		if _, ok := p.cachedClip(ComputeAudioCacheKeyForVoice(speakerID, voice, spoken)); ok {
			cached++
		} else {
			uncached++
		}
	}
	return cached, uncached
}

func NewTTSPipeline(client TTSClient, cache *ContentCache) *TTSPipeline {
	return &TTSPipeline{
		client:      client,
		cache:       cache,
		opusBitrate: opus.DefaultBitrate,
		flights:     map[string]*sync.Mutex{},
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

	return p.cache.Put("audio", base+".opus", encoded)
}

// cachedClip finds a clip under any known extension, so a cache written under an
// older naming scheme is reused rather than regenerated.
func (p *TTSPipeline) cachedClip(base string) (string, bool) {
	for _, ext := range audioExtensions {
		if p.cache.Exists("audio", base+ext) {
			path := filepath.Join(p.cache.Subdir("audio"), base+ext)
			if !clipHasValidHeader(path, ext) {
				// A clip written before format sniffing (or by a provider that
				// changed its output) would fail to decode; drop it so it is
				// re-synthesized rather than served as broken audio.
				_ = os.Remove(path)
				continue
			}
			return path, true
		}
	}
	return "", false
}

// clipHasValidHeader reports whether a cached clip's leading bytes match its
// extension, so a headerless or mistyped file is never served as audio.
func clipHasValidHeader(path, ext string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	head := make([]byte, 12)
	n, _ := io.ReadFull(f, head)
	if n < 4 {
		return false
	}
	head = head[:n]

	switch ext {
	case ".opus":
		return bytes.HasPrefix(head, []byte("OggS"))
	default:
		return false
	}
}
