package playback

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/media/opus"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// writeToneWAV lays down a mono Ogg/Opus clip, which is the shape the player reads.
func writeToneWAV(t *testing.T, dir, name string, sampleRate int, duration time.Duration) string {
	t.Helper()

	frameCount := int(float64(sampleRate) * duration.Seconds())
	pcm := make([]int16, frameCount)
	for frame := range pcm {
		pcm[frame] = int16(math.Sin(2*math.Pi*330*float64(frame)/float64(sampleRate)) * 0.3 * 32767)
	}

	data, err := opus.Encode(pcm, sampleRate, 1, opus.DefaultBitrate)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

var (
	testPlayerInitOnce sync.Once
	testPlayerInstance *Player
	testPlayerErr      error
)

func getTestPlayer(t *testing.T) *Player {
	t.Helper()

	testPlayerInitOnce.Do(func() {
		testPlayerInstance, testPlayerErr = Open(0.5)
	})

	if testPlayerErr != nil {
		if errors.Is(testPlayerErr, ErrUnavailable) {
			t.Skip("audio hardware unavailable on host; skipping device test")
		}
		t.Fatalf("open player: %v", testPlayerErr)
	}

	testPlayerInstance.Stop()
	testPlayerInstance.SetVolume(0.5)
	return testPlayerInstance
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

func TestPlayerPlaysAQueue(t *testing.T) {
	player := getTestPlayer(t)

	dir := t.TempDir()
	first := writeToneWAV(t, dir, "first.wav", deviceSampleRate, 40*time.Millisecond)
	second := writeToneWAV(t, dir, "second.wav", 24000, 40*time.Millisecond)

	if err := player.PlayFiles([]string{first, second}); err != nil {
		t.Fatalf("PlayFiles failed: %v", err)
	}
	if !player.Playing() {
		t.Fatalf("expected playback to be reported as running")
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
	player := getTestPlayer(t)

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
	player := getTestPlayer(t)

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
	player := getTestPlayer(t)

	dir := t.TempDir()
	good := writeToneWAV(t, dir, "good.wav", deviceSampleRate, 30*time.Millisecond)
	junk := filepath.Join(dir, "junk.bin")
	if err := os.WriteFile(junk, []byte("OggS\x00\x02"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := player.PlayFiles([]string{junk, good}); err != nil {
		t.Fatalf("expected the playable clip to be queued: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for player.Playing() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if player.Playing() {
		t.Errorf("expected the queue to finish")
	}
}

func TestPlayerTracesWhatItPlayed(t *testing.T) {
	player := getTestPlayer(t)

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
