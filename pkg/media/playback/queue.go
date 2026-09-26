package playback

import (
	"context"
	"io"
	"sync"

	"github.com/gopxl/beep"
)

// queueStreamer lazily decodes clips as the audio callback needs them, so
// playback can start on the first completed clip instead of waiting for all of
// them. A clip that cannot be decoded is skipped, as PlayFiles does. While the
// queue is open but empty it blocks the audio callback until the next clip
// arrives, which is a gap rather than silence.
type queueStreamer struct {
	ctx   context.Context
	clips <-chan string

	mu      sync.Mutex
	current beep.Streamer
	closer  io.Closer
	done    bool
}

func newQueueStreamer(ctx context.Context, clips <-chan string) *queueStreamer {
	return &queueStreamer{ctx: ctx, clips: clips}
}

// next decodes the next usable clip. It is called with mu held.
func (q *queueStreamer) next() bool {
	for {
		select {
		case <-q.ctx.Done():
			q.done = true
			return false
		case path, ok := <-q.clips:
			if !ok {
				q.done = true
				return false
			}
			streamer, closer, err := decodeFile(path)
			if err != nil {
				continue
			}
			q.current, q.closer = streamer, closer
			return true
		}
	}
}

func (q *queueStreamer) Stream(samples [][2]float64) (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	filled := 0
	for filled < len(samples) {
		if q.done {
			return filled, filled > 0
		}
		if q.current == nil && !q.next() {
			return filled, filled > 0
		}
		n, ok := q.current.Stream(samples[filled:])
		filled += n
		if !ok {
			if q.closer != nil {
				_ = q.closer.Close()
			}
			q.current, q.closer = nil, nil
			if n == 0 {
				continue
			}
		}
	}
	return filled, true
}

func (q *queueStreamer) Err() error { return nil }

// Close stops the queue and releases the clip currently open. It satisfies
// io.Closer so the player's existing cleanup path closes it.
func (q *queueStreamer) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.done = true
	if q.closer != nil {
		_ = q.closer.Close()
		q.closer = nil
	}
	q.current = nil
	return nil
}
