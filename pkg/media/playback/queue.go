package playback

import (
	"io"
	"sync"

	"github.com/gopxl/beep"
)

// queueStreamer lazily decodes clips as the audio callback needs them, so
// playback can start on the first completed clip instead of waiting for all of
// them. A clip that cannot be decoded is skipped, as PlayFiles does. While the
// queue is open but empty it blocks the audio callback until the next clip
// arrives, which is a gap rather than silence.
//
// The queue ends when clips closes or the player is stopped. It is deliberately
// not bound to a caller's context: narration belongs to the application, so a
// request that asked for it finishing must not cut it short.
type queueStreamer struct {
	channels chan (<-chan string)

	mu        sync.Mutex
	currentCh <-chan string
	current   beep.Streamer
	closer    io.Closer
	done      bool
}

func newQueueStreamer(clips <-chan string) *queueStreamer {
	channels := make(chan (<-chan string), 64)
	if clips != nil {
		channels <- clips
	}
	return &queueStreamer{
		channels: channels,
	}
}

func (q *queueStreamer) enqueueChannel(clips <-chan string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.done {
		return false
	}
	select {
	case q.channels <- clips:
		return true
	default:
		return false
	}
}

func (q *queueStreamer) isDone() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.done
}

// next decodes the next usable clip.
func (q *queueStreamer) next() bool {
	for {
		q.mu.Lock()
		if q.done {
			q.mu.Unlock()
			return false
		}
		ch := q.currentCh
		if ch == nil {
			select {
			case nextCh, ok := <-q.channels:
				if !ok {
					q.done = true
					q.mu.Unlock()
					return false
				}
				q.currentCh = nextCh
				ch = nextCh
			default:
				q.done = true
				q.mu.Unlock()
				return false
			}
		}
		q.mu.Unlock()

		path, ok := <-ch
		if !ok {
			q.mu.Lock()
			if q.currentCh == ch {
				q.currentCh = nil
			}
			q.mu.Unlock()
			continue
		}
		streamer, closer, err := decodeFile(path)
		if err != nil {
			continue
		}

		q.mu.Lock()
		if q.done {
			if closer != nil {
				_ = closer.Close()
			}
			q.mu.Unlock()
			return false
		}
		q.current, q.closer = streamer, closer
		q.mu.Unlock()
		return true
	}
}

func (q *queueStreamer) Stream(samples [][2]float64) (int, bool) {
	filled := 0
	for filled < len(samples) {
		q.mu.Lock()
		if q.done {
			q.mu.Unlock()
			return filled, filled > 0
		}
		curr := q.current
		closer := q.closer
		q.mu.Unlock()

		if curr == nil {
			if !q.next() {
				q.mu.Lock()
				defer q.mu.Unlock()
				return filled, filled > 0
			}
			continue
		}

		n, ok := curr.Stream(samples[filled:])
		filled += n
		if !ok {
			if closer != nil {
				_ = closer.Close()
			}
			q.mu.Lock()
			if q.current == curr {
				q.current = nil
				q.closer = nil
			}
			q.mu.Unlock()
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
	q.currentCh = nil
	return nil
}
