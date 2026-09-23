package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
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
	return &SherpaTTSClient{modelDir: modelDir, modelID: KokoroModelV019}
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
		sid = ResolveKokoroSpeakerID(s.modelID, voice.VoiceID)
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
		s.logger.Event("tts.synthesize", map[string]interface{}{
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
