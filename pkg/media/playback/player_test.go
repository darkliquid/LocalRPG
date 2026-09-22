package playback

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/mago"
)

// writeToneWAV lays down a mono 16-bit WAV, which is the shape the decoder reads.
func writeToneWAV(t *testing.T, dir, name string, sampleRate int, duration time.Duration) string {
	t.Helper()

	frameCount := int(float64(sampleRate) * duration.Seconds())

	pcm := bytes.Buffer{}
	for frame := 0; frame < frameCount; frame++ {
		sample := int16(math.Sin(2*math.Pi*330*float64(frame)/float64(sampleRate)) * 0.3 * 32767)
		if err := binary.Write(&pcm, binary.LittleEndian, sample); err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, wrapPCMAsWAV(pcm.Bytes(), 1, sampleRate, 16), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// wrapPCMAsWAV builds a WAV container around raw PCM. It exists so the tests can
// produce input without a fixture file.
func wrapPCMAsWAV(pcm []byte, channels, sampleRate, bitsPerSample int) []byte {
	blockAlign := channels * bitsPerSample / 8
	byteRate := sampleRate * blockAlign

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(bitsPerSample))

	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)

	return buf.Bytes()
}

func nullBackendPlayer(t *testing.T) *Player {
	t.Helper()

	player, err := OpenWithBackends(0.5, []mago.Backend{mago.BackendNull})
	if err != nil {
		t.Fatalf("open player: %v", err)
	}
	t.Cleanup(func() { _ = player.Close() })
	return player
}

func TestPlayerPlaysAQueueOnTheNullBackend(t *testing.T) {
	player := nullBackendPlayer(t)

	dir := t.TempDir()
	// Two clips at different rates, so the queue exercises resampling too. Kokoro
	// narrates at 24 kHz while the device runs at 48 kHz.
	first := writeToneWAV(t, dir, "first.wav", deviceSampleRate, 40*time.Millisecond)
	second := writeToneWAV(t, dir, "second.wav", 24000, 40*time.Millisecond)

	if err := player.PlayFiles([]string{first, second}); err != nil {
		t.Fatalf("PlayFiles failed: %v", err)
	}
	if !player.Playing() {
		t.Fatalf("expected playback to be reported as running")
	}

	deadline := time.Now().Add(10 * time.Second)
	for player.Playing() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if player.Playing() {
		t.Errorf("expected the queue to finish")
	}
}

func TestPlayerStopEndsTheQueueEarly(t *testing.T) {
	player := nullBackendPlayer(t)

	path := writeToneWAV(t, t.TempDir(), "long.wav", deviceSampleRate, 5*time.Second)
	if err := player.PlayFiles([]string{path}); err != nil {
		t.Fatalf("PlayFiles failed: %v", err)
	}

	player.Stop()

	if player.Playing() {
		t.Errorf("expected Stop to end the queue")
	}
}

func TestPlayFilesSkipsUndecodableClips(t *testing.T) {
	player := nullBackendPlayer(t)

	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.bin")
	if err := os.WriteFile(junk, []byte("OggS\x00\x02not-a-container"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := player.PlayFiles([]string{junk}); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestPlayFilesPlaysTheGoodClipsWhenOneIsBad(t *testing.T) {
	player := nullBackendPlayer(t)

	dir := t.TempDir()
	good := writeToneWAV(t, dir, "good.wav", deviceSampleRate, 30*time.Millisecond)
	junk := filepath.Join(dir, "junk.bin")
	if err := os.WriteFile(junk, []byte("OggS\x00\x02"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := player.PlayFiles([]string{junk, good}); err != nil {
		t.Fatalf("expected the playable clip to be queued: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for player.Playing() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if player.Playing() {
		t.Errorf("expected the queue to finish")
	}
}

func TestPlayerWithoutADeviceReportsUnavailable(t *testing.T) {
	player := &Player{}

	if player.Available() {
		t.Errorf("expected a bare player to be unavailable")
	}
	if err := player.PlayFiles(nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

func TestPlayerTracesWhatItPlayed(t *testing.T) {
	player, err := OpenWithBackends(0.5, []mago.Backend{mago.BackendNull})
	if err != nil {
		t.Fatalf("open player: %v", err)
	}
	defer func() { _ = player.Close() }()

	memory := trace.NewMemory(trace.LevelSummary)
	player.SetLogger(memory)

	path := writeToneWAV(t, t.TempDir(), "clip.wav", deviceSampleRate, 30*time.Millisecond)
	if err := player.PlayFiles([]string{path}); err != nil {
		t.Fatalf("PlayFiles failed: %v", err)
	}

	event, ok := memory.Find("audio.play")
	if !ok {
		t.Fatalf("expected an audio.play event, got %v", memory.Names())
	}
	if event.Fields["clips"] != 1 {
		t.Errorf("clips = %v, want 1", event.Fields["clips"])
	}
	if event.Fields["volume"] != 0.5 {
		t.Errorf("volume = %v, want the configured gain", event.Fields["volume"])
	}
}
