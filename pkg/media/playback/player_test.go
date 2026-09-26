package playback

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/media/opus"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// useFakeDevice replaces the audio boundary with a draining sink so tests need
// no sound card. It drains faster than real time, so clips finish quickly.
func useFakeDevice(t *testing.T) {
	t.Helper()
	prev := startDevice
	startDevice = func(rate int, fill func([]float32)) error {
		buf := make([]float32, 1024)
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
					fill(buf)
					time.Sleep(time.Millisecond)
				}
			}
		}()
		t.Cleanup(func() { close(stop) })
		return nil
	}
	t.Cleanup(func() { startDevice = prev })
}

// writeOpusClip lays down a mono Ogg/Opus clip, the shape the player reads.
func writeOpusClip(t *testing.T, samples int) string {
	t.Helper()

	pcm := make([]int16, samples)
	for frame := range pcm {
		pcm[frame] = int16(math.Sin(2*math.Pi*330*float64(frame)/float64(opus.SampleRate)) * 0.3 * 32767)
	}
	data, err := opus.Encode(pcm, opus.SampleRate, 1, opus.DefaultBitrate)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	path := filepath.Join(t.TempDir(), "clip.opus")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
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

func TestOpenReportsUnavailableWhenDeviceFails(t *testing.T) {
	prev := startDevice
	startDevice = func(int, func([]float32)) error { return errors.New("no device") }
	t.Cleanup(func() { startDevice = prev })

	if _, err := Open(1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open = %v, want ErrUnavailable", err)
	}
}

func TestPlayerPlaysAQueue(t *testing.T) {
	useFakeDevice(t)
	player, err := Open(0.5)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !player.Available() {
		t.Fatal("expected the player to be available")
	}

	first := writeOpusClip(t, opus.SampleRate/10) // 100ms
	second := writeOpusClip(t, opus.SampleRate/10)
	if err := player.PlayFiles([]string{first, second}); err != nil {
		t.Fatalf("PlayFiles: %v", err)
	}
	if !player.Playing() {
		t.Fatal("expected playback to be reported as running")
	}

	deadline := time.Now().Add(5 * time.Second)
	for player.Playing() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if player.Playing() {
		t.Errorf("expected the queue to finish")
	}
}

func TestPlayerStopEndsTheQueueEarly(t *testing.T) {
	useFakeDevice(t)
	player, err := Open(0.5)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	path := writeOpusClip(t, opus.SampleRate*5) // 5s
	if err := player.PlayFiles([]string{path}); err != nil {
		t.Fatalf("PlayFiles: %v", err)
	}

	player.Stop()

	if player.Playing() {
		t.Errorf("expected Stop to end the queue")
	}
}

func TestPlayFilesSkipsUndecodableClips(t *testing.T) {
	useFakeDevice(t)
	player, err := Open(0.5)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	junk := filepath.Join(t.TempDir(), "junk.bin")
	if err := os.WriteFile(junk, []byte("OggS\x00\x02not-a-container"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := player.PlayFiles([]string{junk}); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestPlayFilesPlaysTheGoodClipsWhenOneIsBad(t *testing.T) {
	useFakeDevice(t)
	player, err := Open(0.5)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	dir := t.TempDir()
	good := writeOpusClip(t, opus.SampleRate/10)
	junk := filepath.Join(dir, "junk.bin")
	if err := os.WriteFile(junk, []byte("OggS\x00\x02"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := player.PlayFiles([]string{junk, good}); err != nil {
		t.Fatalf("expected the playable clip to be queued: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for player.Playing() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if player.Playing() {
		t.Errorf("expected the queue to finish")
	}
}

func TestPlayerTracesWhatItPlayed(t *testing.T) {
	useFakeDevice(t)
	player, err := Open(0.5)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	memory := trace.NewMemory(trace.LevelSummary)
	player.SetLogger(memory)

	path := writeOpusClip(t, opus.SampleRate/10)
	if err := player.PlayFiles([]string{path}); err != nil {
		t.Fatalf("PlayFiles: %v", err)
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
