package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/dialogue"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// narratorSpeaker is the cache namespace for lines read in the narrator voice.
const narratorSpeaker = "narrator"

// audioExtensions are the names a cached clip may carry, in the order they are
// looked for. The legacy .wav name is last so a cache written before clips were
// named honestly stays warm.
var audioExtensions = []string{".mp3", ".ogg", ".flac", ".wav"}

// AudioExtension names a clip from its bytes, because a provider returns whatever
// its engine produces rather than what the configuration implies.
func AudioExtension(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("RIFF")):
		return ".wav"
	case bytes.HasPrefix(data, []byte("ID3")):
		return ".mp3"
	case len(data) > 1 && data[0] == 0xFF && data[1]&0xE0 == 0xE0:
		return ".mp3"
	case bytes.HasPrefix(data, []byte("OggS")):
		return ".ogg"
	case bytes.HasPrefix(data, []byte("fLaC")):
		return ".flac"
	default:
		return ".wav"
	}
}

// AudioContentType is the MIME type for a clip's bytes.
func AudioContentType(data []byte) string {
	switch AudioExtension(data) {
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
	client TTSClient
	cache  *ContentCache
	logger trace.Logger
	policy TextPolicy
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
	speakerID, voice, spoken := p.prepareSegment(segment, narratorVoice, voiceFor)
	if strings.TrimSpace(spoken) == "" {
		return "", ErrNoSpeakableText
	}
	if spoken != segment.Text {
		p.logger = trace.OrNil(p.logger)
		p.logger.Event("media.tts.reduced", map[string]interface{}{
			"chars_raw":    len([]rune(segment.Text)),
			"chars_spoken": len([]rune(spoken)),
		})
	}
	return p.SynthesizeUtterance(ctx, speakerID, voice, spoken)
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
		client: client,
		cache:  cache,
	}
}

func (p *TTSPipeline) SynthesizeUtterance(ctx context.Context, speakerID string, voice *entity.VoiceConfig, text string) (string, error) {
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
	p.logger = trace.OrNil(p.logger)
	p.logger.Event("media.tts.request", map[string]interface{}{
		"speaker":   speakerID,
		"voice_id":  voiceID,
		"provider":  provider,
		"model":     model,
		"pitch":     pitch,
		"rate":      rate,
		"chars":     len([]rune(text)),
		"cache_key": base,
	})

	if path, ok := p.cachedClip(base); ok {
		p.logger.Event("media.tts.result", map[string]interface{}{
			"cache_hit":   true,
			"duration_ms": time.Since(start).Milliseconds(),
		})
		return path, nil
	}

	audioBytes, err := p.client.Synthesize(ctx, text, voice)
	if err != nil {
		return "", fmt.Errorf("synthesize utterance: %w", err)
	}

	p.logger.Event("media.tts.result", map[string]interface{}{
		"cache_hit":    false,
		"bytes":        len(audioBytes),
		"content_type": AudioContentType(audioBytes),
		"duration_ms":  time.Since(start).Milliseconds(),
	})

	return p.cache.Put("audio", base+AudioExtension(audioBytes), audioBytes)
}

// cachedClip finds a clip under any known extension, so a cache written under an
// older naming scheme is reused rather than regenerated.
func (p *TTSPipeline) cachedClip(base string) (string, bool) {
	for _, ext := range audioExtensions {
		if p.cache.Exists("audio", base+ext) {
			return filepath.Join(p.cache.Subdir("audio"), base+ext), true
		}
	}
	return "", false
}
