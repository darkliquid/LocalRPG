package media

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type UtteranceSegment struct {
	Speaker    string
	Text       string
	IsNarrator bool
}

var attributedSpeechRegex = regexp.MustCompile(`^([^:\n]+):\s*"([^"]+)"`)
var quotedSpeechRegex = regexp.MustCompile(`"([^"]+)"`)

func ParseDialogueSegments(text, defaultNarrator string) []UtteranceSegment {
	lines := strings.Split(text, "\n")
	var segments []UtteranceSegment
	lastSpeaker := defaultNarrator

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for "Speaker: "..." pattern
		if match := attributedSpeechRegex.FindStringSubmatch(line); len(match) == 3 {
			speaker := strings.TrimSpace(match[1])
			quote := match[2]
			lastSpeaker = speaker
			segments = append(segments, UtteranceSegment{
				Speaker:    speaker,
				Text:       quote,
				IsNarrator: false,
			})
			continue
		}

		// Check for quoted speech inside prose
		if match := quotedSpeechRegex.FindStringSubmatch(line); len(match) == 2 {
			quote := match[1]
			// Dialogue part
			segments = append(segments, UtteranceSegment{
				Speaker:    lastSpeaker,
				Text:       quote,
				IsNarrator: false,
			})
			continue
		}

		// Default to narrator prose
		segments = append(segments, UtteranceSegment{
			Speaker:    defaultNarrator,
			Text:       line,
			IsNarrator: true,
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

func NewTTSPipeline(client TTSClient, cache *ContentCache) *TTSPipeline {
	return &TTSPipeline{
		client: client,
		cache:  cache,
	}
}

func (p *TTSPipeline) SynthesizeUtterance(ctx context.Context, speakerID string, voice *entity.VoiceConfig, text string) (string, error) {
	voiceHash := "default"
	if voice != nil {
		voiceHash = fmt.Sprintf("%s:%s:%.2f", voice.Provider, voice.VoiceID, voice.Pitch)
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
