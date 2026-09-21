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
		clip, err := p.SynthesizeSegment(ctx, segment, narratorVoice, voiceFor)
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

	return p.SynthesizeUtterance(ctx, speakerID, voice, segment.Text)
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
