package media

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/dialogue"
	"github.com/darkliquid/localrpg/pkg/entity"
)

// narratorSpeaker is the cache namespace for lines read in the narrator voice.
const narratorSpeaker = "narrator"

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
}

// SynthesizeSegments renders every segment with its speaker's voice, falling back
// to the narrator voice for narration and unresolved speech. Cached clips are
// reused; audio references stay out of the turn record because the cache key is a
// pure function of speaker, voice, prosody, and text.
func (p *TTSPipeline) SynthesizeSegments(ctx context.Context, segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) ([]string, error) {
	clips := make([]string, 0, len(segments))

	for _, segment := range segments {
		voice := narratorVoice
		speakerID := narratorSpeaker
		if segment.Kind == entity.SegmentSpeech {
			// Legacy records carry a speaker name but no entity ID, so fall back
			// to the name when resolving a voice.
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

		clip, err := p.SynthesizeUtterance(ctx, speakerID, voice, segment.Text)
		if err != nil {
			return nil, err
		}
		clips = append(clips, clip)
	}

	return clips, nil
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

	cacheKey := ComputeAudioCacheKey(speakerID, voiceHash, text) + ".wav"
	if p.cache.Exists("audio", cacheKey) {
		return filepath.Join(p.cache.Subdir("audio"), cacheKey), nil
	}

	audioBytes, err := p.client.Synthesize(ctx, text, voice)
	if err != nil {
		return "", fmt.Errorf("synthesize utterance: %w", err)
	}

	return p.cache.Put("audio", cacheKey, audioBytes)
}
