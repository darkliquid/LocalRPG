package gui

import "sync"

// Ledger records what the player has heard, and how much of it, so no clip plays
// twice and a clip cut short resumes rather than restarting. It is keyed by clip
// key. The server owns the authoritative ledger for a turn and the client keeps a
// mirror; the two reconcile with Merge, which takes the furthest offset and never
// downgrades a completed clip, so a repeated or reordered merge is harmless.
//
// A nil *Ledger is an empty ledger that discards writes, so a caller without one
// needs no guard.
type Ledger struct {
	mu      sync.Mutex
	entries map[string]Entry
}

// Entry is a clip's heard state.
type Entry struct {
	// PlayedMS is how far into the clip playback reached, in milliseconds.
	PlayedMS int `json:"played_ms"`
	// TotalMS is the clip's length, when it is known.
	TotalMS int `json:"total_ms,omitempty"`
	// Complete reports that the clip was heard to its end.
	Complete bool `json:"complete,omitempty"`
}

// NewLedger returns an empty ledger.
func NewLedger() *Ledger {
	return &Ledger{entries: map[string]Entry{}}
}

// Record notes how far a clip has played. It merges with what is already known,
// so a late, smaller offset never rewinds the clip.
func (l *Ledger) Record(key string, playedMS, totalMS int, complete bool) {
	if l == nil || key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.mergeLocked(key, Entry{PlayedMS: playedMS, TotalMS: totalMS, Complete: complete})
}

// Merge folds another ledger's entries into this one. Each clip keeps the larger
// offset and length, and a clip complete on either side stays complete, so the
// merge is a max: commutative, idempotent, and safe to repeat on a reconnect.
func (l *Ledger) Merge(other map[string]Entry) {
	if l == nil || len(other) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, entry := range other {
		if key == "" {
			continue
		}
		l.mergeLocked(key, entry)
	}
}

// mergeLocked applies one entry. The caller holds the lock.
func (l *Ledger) mergeLocked(key string, in Entry) {
	if l.entries == nil {
		l.entries = map[string]Entry{}
	}
	in.PlayedMS = max(in.PlayedMS, 0)
	in.TotalMS = max(in.TotalMS, 0)
	cur := l.entries[key]
	cur.PlayedMS = max(cur.PlayedMS, in.PlayedMS)
	cur.TotalMS = max(cur.TotalMS, in.TotalMS)
	cur.Complete = cur.Complete || in.Complete
	l.entries[key] = cur
}

// Entry returns a clip's heard state, and whether the ledger knows the clip.
func (l *Ledger) Entry(key string) (Entry, bool) {
	if l == nil {
		return Entry{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	return e, ok
}

// Complete reports whether a clip was heard to its end, which makes a later play
// of it a no-op.
func (l *Ledger) Complete(key string) bool {
	e, _ := l.Entry(key)
	return e.Complete
}

// Partial reports whether a clip was started but not finished: its remainder is
// owed to the player, from PlayedMS.
func (l *Ledger) Partial(key string) bool {
	e, _ := l.Entry(key)
	return !e.Complete && e.PlayedMS > 0
}

// Snapshot copies the ledger's entries, so a caller can serialise them without
// holding the lock or racing a later merge.
func (l *Ledger) Snapshot() map[string]Entry {
	out := map[string]Entry{}
	if l == nil {
		return out
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, entry := range l.entries {
		out[key] = entry
	}
	return out
}
