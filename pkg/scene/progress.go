package scene

import "time"

// Progress reports how far a long-running export has come. Phase is a stable
// machine name ("compile", "frames", "encode", "done"); Done and Total are the
// phase's own unit, which for a render is frames. Message carries a human-readable
// note for a degraded step, such as a scene without art, and is empty for
// ordinary progress.
//
// The remaining fields are the detail a progress bar needs to say what the work
// actually is. They are all known before a render starts, so a bar can be exact
// from the first frame rather than creeping towards an unknown total.
type Progress struct {
	Phase   string
	Done    int
	Total   int
	Message string

	// Frames counts the frames rendered. ImageFrames are frames that show new
	// imagery; RepeatFrames reuse the frame before them, which is the heartbeat
	// that keeps a still beat alive.
	Frames       int
	ImageFrames  int
	RepeatFrames int

	// Audio counts the Opus packets muxed and the bytes they took. The totals are
	// the campaign's own clip sizes, read before the render begins.
	AudioPackets      int
	TotalAudioPackets int
	AudioBytes        int64
	TotalAudioBytes   int64

	// Elapsed is how long the render has run and Length is the finished video's
	// duration, so a caller can estimate what is left.
	Elapsed time.Duration
	Length  time.Duration
}

// ProgressFunc receives progress updates. Implementations must not block: the
// caller emits from the rendering goroutine.
type ProgressFunc func(Progress)

// emitProgress calls fn unless it is nil, so callers never guard each emit.
func emitProgress(fn ProgressFunc, p Progress) {
	if fn != nil {
		fn(p)
	}
}
