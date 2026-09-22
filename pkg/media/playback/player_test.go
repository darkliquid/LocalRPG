package playback

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/darkliquid/mago"
)

// toneWAV builds a short mono 16-bit WAV, which is the shape the mixer decodes.
func toneWAV(t *testing.T, duration time.Duration) []byte {
	t.Helper()

	const sampleRate = 48000
	frameCount := int(float64(sampleRate) * duration.Seconds())

	pcm := bytes.Buffer{}
	for frame := 0; frame < frameCount; frame++ {
		sample := int16(math.Sin(2*math.Pi*330*float64(frame)/sampleRate) * 0.3 * 32767)
		if err := binary.Write(&pcm, binary.LittleEndian, sample); err != nil {
			t.Fatal(err)
		}
	}
	return wrapPCMAsWAV(pcm.Bytes(), 1, sampleRate, 16)
}

func TestToWAVPassesWAVThrough(t *testing.T) {
	wav := toneWAV(t, 20*time.Millisecond)

	got, err := ToWAV(wav)
	if err != nil {
		t.Fatalf("ToWAV failed: %v", err)
	}
	if !bytes.Equal(got, wav) {
		t.Errorf("expected WAV bytes unchanged")
	}
}

func TestToWAVRejectsAnUnknownContainer(t *testing.T) {
	_, err := ToWAV([]byte("OggS\x00\x02"))
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestIsMP3DetectsFramesAndTags(t *testing.T) {
	if !isMP3([]byte("ID3\x04\x00")) {
		t.Errorf("expected an ID3 tag to be treated as MP3")
	}
	if !isMP3([]byte{0xFF, 0xFB, 0x90}) {
		t.Errorf("expected a frame sync to be treated as MP3")
	}
	if isMP3([]byte("RIFF")) {
		t.Errorf("did not expect RIFF to be treated as MP3")
	}
}

func TestPlayerPlaysAQueueOnTheNullBackend(t *testing.T) {
	player, err := OpenWithBackends(0.5, []mago.Backend{mago.BackendNull})
	if err != nil {
		t.Fatalf("open player: %v", err)
	}
	defer func() { _ = player.Close() }()

	if !player.Available() {
		t.Fatal("expected the null backend to be available")
	}

	clips := [][]byte{toneWAV(t, 40*time.Millisecond), toneWAV(t, 40*time.Millisecond)}
	if err := player.PlayClips(clips); err != nil {
		t.Fatalf("PlayClips failed: %v", err)
	}
	if !player.Playing() {
		t.Errorf("expected playback to be reported as running")
	}

	deadline := time.Now().Add(5 * time.Second)
	for player.Playing() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if player.Playing() {
		t.Errorf("expected the queue to finish")
	}
}

func TestPlayerStopEndsTheQueueEarly(t *testing.T) {
	player, err := OpenWithBackends(0.5, []mago.Backend{mago.BackendNull})
	if err != nil {
		t.Fatalf("open player: %v", err)
	}
	defer func() { _ = player.Close() }()

	if err := player.PlayClips([][]byte{toneWAV(t, 3*time.Second)}); err != nil {
		t.Fatalf("PlayClips failed: %v", err)
	}

	player.Stop()

	if player.Playing() {
		t.Errorf("expected Stop to end the queue")
	}
}

func TestPlayerWithoutADeviceReportsUnavailable(t *testing.T) {
	player := &Player{}
	if player.Available() {
		t.Errorf("expected a bare player to be unavailable")
	}
	if err := player.PlayClips(nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}
