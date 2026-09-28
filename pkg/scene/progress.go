package scene

// Progress reports how far a long-running export has come. Phase is a stable
// machine name ("compile", "frames", "encode", "done"); Done and Total may be
// zero when a phase has no measurable units. Message carries a human-readable
// note for a degraded step, such as a scene without art, and is empty for
// ordinary progress.
type Progress struct {
	Phase   string
	Done    int
	Total   int
	Message string
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
