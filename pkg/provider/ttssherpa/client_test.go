//go:build sherpa

package ttssherpa

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/darkliquid/localrpg/pkg/media"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestKokoroVoiceMapping(t *testing.T) {
	tests := []struct {
		voiceID string
		wantSid int
	}{
		{"af", 0},
		{"af_bella", 1},
		{"af_nicole", 2},
		{"am_adam", 5},
		{"am_michael", 6},
		{"bf_emma", 7},
		{"bm_george", 9},
		{"bm_lewis", 10},
		{"unknown_voice", 0}, // fallback
		{"", 0},
	}

	for _, tt := range tests {
		got := media.ResolveKokoroSpeakerID(media.KokoroModelV019, tt.voiceID)
		if got != tt.wantSid {
			t.Errorf("media.ResolveKokoroSpeakerID(%q) = %d; want %d", tt.voiceID, got, tt.wantSid)
		}
	}
}

func TestEncodePCMToWAV(t *testing.T) {
	samples := []float32{0.0, 0.5, -0.5, 1.0, -1.0}
	sampleRate := 24000

	wavBytes, err := EncodePCMFloatToWAV(samples, sampleRate)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}

	if len(wavBytes) != 44+len(samples)*2 {
		t.Errorf("expected %d bytes, got %d", 44+len(samples)*2, len(wavBytes))
	}

	// Verify header tags
	if string(wavBytes[0:4]) != "RIFF" {
		t.Errorf("expected RIFF header, got %q", string(wavBytes[0:4]))
	}
	if string(wavBytes[8:12]) != "WAVE" {
		t.Errorf("expected WAVE format, got %q", string(wavBytes[8:12]))
	}

	var rate uint32
	_ = binary.Read(bytes.NewReader(wavBytes[24:28]), binary.LittleEndian, &rate)
	if rate != 24000 {
		t.Errorf("expected 24000 sample rate, got %d", rate)
	}
}

func TestSherpaTTSMissingModelReturnsError(t *testing.T) {
	client := NewSherpaTTSClient(t.TempDir())
	_, err := client.Synthesize(context.Background(), "Hello test", &entity.VoiceConfig{VoiceID: "af_bella"})
	if err != ErrModelNotLoaded {
		t.Errorf("expected ErrModelNotLoaded, got %v", err)
	}
}
