package playback

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/media/opus"
)

func decodedFrames(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pcm, _, _, err := opus.Decode(data)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return len(pcm)
}

func drainQueue(t *testing.T, q *queueStreamer) int {
	t.Helper()
	buf := make([][2]float64, 256)
	total := 0
	for i := 0; i < 1_000_000; i++ {
		n, ok := q.Stream(buf)
		total += n
		if !ok {
			return total
		}
	}
	t.Fatal("queueStreamer did not terminate")
	return total
}

func TestQueueStreamerPullsClipsLazilyInOrder(t *testing.T) {
	dir := t.TempDir()
	first := writeToneWAV(t, dir, "one.wav", deviceSampleRate, 40*time.Millisecond)
	second := writeToneWAV(t, dir, "two.wav", deviceSampleRate, 40*time.Millisecond)
	want := decodedFrames(t, first) + decodedFrames(t, second)

	clips := make(chan string, 2)
	clips <- first
	clips <- second
	close(clips)

	q := newQueueStreamer(clips)
	defer q.Close()

	if total := drainQueue(t, q); total != want {
		t.Fatalf("streamed %d frames, want %d across both clips", total, want)
	}
}

func TestQueueStreamerSkipsUndecodableClips(t *testing.T) {
	dir := t.TempDir()
	good := writeToneWAV(t, dir, "good.wav", deviceSampleRate, 30*time.Millisecond)
	want := decodedFrames(t, good)

	bad := filepath.Join(dir, "bad.bin")
	if err := os.WriteFile(bad, []byte("not audio"), 0644); err != nil {
		t.Fatal(err)
	}

	clips := make(chan string, 2)
	clips <- bad
	clips <- good
	close(clips)

	q := newQueueStreamer(clips)
	defer q.Close()

	if total := drainQueue(t, q); total != want {
		t.Fatalf("streamed %d frames, want the %d from the good clip", total, want)
	}
}
