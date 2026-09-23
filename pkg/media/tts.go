package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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

// SynthesizeSegment renders one segment: narration and unresolved speech read in
// the narrator voice, resolved speech in the speaker's own. Legacy records carry a
// speaker name but no entity ID, so the name is tried as a voice key too.
func (p *TTSPipeline) SynthesizeSegment(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (string, error) {
	spoken := SpeakableTextFor(p.policy, p.client, segment.Text)
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

	voice := narratorVoice
	speakerID := narratorSpeaker

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

	return p.SynthesizeUtterance(ctx, speakerID, voice, spoken)
}

func NewTTSPipeline(client TTSClient, cache *ContentCache) *TTSPipeline {
	return &TTSPipeline{
		client: client,
		cache:  cache,
	}
}

func (p *TTSPipeline) SynthesizeUtterance(ctx context.Context, speakerID string, voice *entity.VoiceConfig, text string) (string, error) {
	voiceHash := "default"
	if voice != nil {
		voiceHash = fmt.Sprintf("%s:%s:%.2f:%.2f", voice.Provider, voice.VoiceID, voice.Pitch, voice.SpeechRate)
	}

	base := ComputeAudioCacheKey(speakerID, voiceHash, text)
	start := time.Now()

	voiceID, pitch, rate := "", 0.0, 0.0
	if voice != nil {
		voiceID, pitch, rate = voice.VoiceID, voice.Pitch, voice.SpeechRate
	}
	p.logger = trace.OrNil(p.logger)
	p.logger.Event("media.tts.request", map[string]interface{}{
		"speaker":   speakerID,
		"voice_id":  voiceID,
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
