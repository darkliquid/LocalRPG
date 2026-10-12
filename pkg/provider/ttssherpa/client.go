package ttssherpa

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/darkliquid/localrpg/pkg/media"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/trace"
	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

var ErrModelNotLoaded = errors.New("sherpa-onnx model weights are not loaded")

type SherpaTTSClient struct {
	modelDir string
	modelID  string
	tts      *sherpa.OfflineTts
	mu       sync.Mutex
	logger   trace.Logger
}

func NewSherpaTTSClient(modelDir string) *SherpaTTSClient {
	return &SherpaTTSClient{modelDir: modelDir, modelID: media.KokoroModelV019}
}

func (s *SherpaTTSClient) SetLogger(logger trace.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger = trace.OrNil(logger)
}

func (s *SherpaTTSClient) ensureLoadedLocked() error {
	if s.tts != nil {
		return nil
	}

	modelPath := filepath.Join(s.modelDir, "model.onnx")
	voicesPath := filepath.Join(s.modelDir, "voices.bin")
	tokensPath := filepath.Join(s.modelDir, "tokens.txt")
	dataDir := filepath.Join(s.modelDir, "espeak-ng-data")

	if _, err := os.Stat(modelPath); err != nil {
		return ErrModelNotLoaded
	}
	if _, err := os.Stat(voicesPath); err != nil {
		return ErrModelNotLoaded
	}

	config := sherpa.OfflineTtsConfig{}
	config.Model.Kokoro.Model = modelPath
	config.Model.Kokoro.Voices = voicesPath
	config.Model.Kokoro.Tokens = tokensPath
	config.Model.Kokoro.DataDir = dataDir
	config.Model.Kokoro.LengthScale = 1.0

	tts := sherpa.NewOfflineTts(&config)
	if tts == nil {
		return fmt.Errorf("failed to initialize sherpa-onnx OfflineTts")
	}

	s.tts = tts
	return nil
}

func (s *SherpaTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureLoadedLocked(); err != nil {
		return nil, err
	}

	sid := 0
	speed := float32(1.0)
	if voice != nil {
		sid = media.ResolveKokoroSpeakerID(s.modelID, voice.VoiceID)
		if voice.SpeechRate > 0 {
			speed = float32(voice.SpeechRate)
		}
	}

	start := time.Now()
	audio := s.tts.Generate(text, sid, speed)
	if audio == nil || len(audio.Samples) == 0 {
		return nil, fmt.Errorf("sherpa-onnx audio generation returned empty audio")
	}

	wavBytes, err := EncodePCMFloatToWAV(audio.Samples, audio.SampleRate)
	if err != nil {
		return nil, fmt.Errorf("encode wav: %w", err)
	}

	if s.logger != nil {
		s.logger.Event("tts.synthesize", map[string]any{
			"engine":      "sherpa-onnx",
			"sid":         sid,
			"samples":     len(audio.Samples),
			"sample_rate": audio.SampleRate,
			"bytes":       len(wavBytes),
			"elapsed_ms":  time.Since(start).Milliseconds(),
		})
	}

	return wavBytes, nil
}

func (s *SherpaTTSClient) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tts != nil {
		sherpa.DeleteOfflineTts(s.tts)
		s.tts = nil
	}
}

// ListVoices enumerates the 11 known Kokoro speakers for the pinned model.
// It requires neither network access nor loaded model weights.
func (s *SherpaTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	speakers := media.KokoroSpeakersForModel(s.modelID)
	voices := make([]media.ProviderVoice, 0, len(speakers))

	profiles := map[string]struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{
		"af":          {"Default (American Female)", "female", "american", []string{"american", "female", "default", "neutral"}, "The model's stock American female voice."},
		"af_bella":    {"Bella (American Female)", "female", "american", []string{"american", "female", "warm", "friendly"}, "American female voice, warm, approachable, and pleasant."},
		"af_nicole":   {"Nicole (American Female)", "female", "american", []string{"american", "female", "youthful", "energetic"}, "American female voice, brisk, youthful, and direct."},
		"af_sarah":    {"Sarah (American Female)", "female", "american", []string{"american", "female", "poised", "narrative"}, "American female voice, polished, measured, and story-oriented."},
		"af_sky":      {"Sky (American Female)", "female", "american", []string{"american", "female", "light", "airy"}, "American female voice, light, gentle, and breathy."},
		"am_adam":     {"Adam (American Male)", "male", "american", []string{"american", "male", "deep", "authoritative"}, "American male voice, deep, steady, and commanding."},
		"am_michael":  {"Michael (American Male)", "male", "american", []string{"american", "male", "commanding", "formal"}, "American male voice, disciplined, authoritative, and formal."},
		"bf_emma":     {"Emma (British Female)", "female", "british", []string{"british", "female", "gentle", "poised"}, "British female voice, elegant, gentle, and softly spoken."},
		"bf_isabella": {"Isabella (British Female)", "female", "british", []string{"british", "female", "noble", "melodic"}, "British female voice, aristocratic, melodic, and graceful."},
		"bm_george":   {"George (British Male)", "male", "british", []string{"british", "male", "mature", "distinguished"}, "British male voice, mature, distinguished, and resonant."},
		"bm_lewis":    {"Lewis (British Male)", "male", "british", []string{"british", "male", "thoughtful", "refined"}, "British male voice, measured, polite, and reflective."},
	}

	for _, speaker := range speakers {
		meta, ok := profiles[speaker.Name]
		if !ok {
			meta.name = speaker.Name
		}
		voices = append(voices, media.ProviderVoice{
			ID:          speaker.Name,
			Name:        meta.name,
			Gender:      meta.gender,
			Accent:      meta.accent,
			Categories:  []string{"built-in"},
			Tags:        meta.tags,
			Description: meta.description,
		})
	}
	return voices, nil
}

// media.SpeechCueCapabilities advertises that SherpaTTSClient supports plain text only.
func (s *SherpaTTSClient) SpeechCueCapabilities() media.SpeechCueCapabilities {
	return media.SpeechCueCapabilities{
		AudioTags:        false,
		MarkdownEmphasis: false,
	}
}

// EncodePCMFloatToWAV serializes 32-bit float audio samples to a 16-bit mono WAV container.
func EncodePCMFloatToWAV(samples []float32, sampleRate int) ([]byte, error) {
	numChannels := uint16(1)
	bitsPerSample := uint16(16)
	byteRate := uint32(sampleRate) * uint32(numChannels) * uint32(bitsPerSample/8)
	blockAlign := numChannels * (bitsPerSample / 8)
	dataSize := uint32(len(samples) * int(bitsPerSample/8))
	chunkSize := 36 + dataSize

	var buf bytes.Buffer
	buf.Grow(int(44 + dataSize))

	// RIFF header
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, chunkSize)
	buf.WriteString("WAVE")

	// fmt subchunk
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16)) // Subchunk1Size (16 for PCM)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // AudioFormat (1 for PCM)
	_ = binary.Write(&buf, binary.LittleEndian, numChannels)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, byteRate)
	_ = binary.Write(&buf, binary.LittleEndian, blockAlign)
	_ = binary.Write(&buf, binary.LittleEndian, bitsPerSample)

	// data subchunk
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataSize)

	for _, s := range samples {
		clamped := math.Max(-1.0, math.Min(1.0, float64(s)))
		val := int16(clamped * 32767)
		_ = binary.Write(&buf, binary.LittleEndian, val)
	}

	return buf.Bytes(), nil
}
