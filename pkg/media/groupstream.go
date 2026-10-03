package media

import "strings"

// GroupFolder folds a turn's speaker lines into groups incrementally, applying
// the same rules as planGroups: a group ends when the speaker changes, when the
// speaker budget is reached, or when the request limits would be exceeded, and a
// line too large for one request is split at sentence boundaries. A positive
// budget additionally forces a flush so live audio is not held until a long block
// ends, which is the only way the streaming fold differs from the batch plan.
type GroupFolder struct {
	caps    TTSCapabilities
	budget  int
	pending []SpeakerLine
	chars   int
}

// NewGroupFolder builds a folder. A budget of zero flushes only on a boundary or
// the provider limits, which makes it identical to planGroups.
func NewGroupFolder(caps TTSCapabilities, budget int) *GroupFolder {
	return &GroupFolder{caps: normalizeCaps(caps), budget: budget}
}

// Add appends a line and returns every group it closed. A line that cannot join
// the pending group flushes it first, and a line too large for one request is
// split at sentence boundaries, exactly as planGroups does.
func (f *GroupFolder) Add(line SpeakerLine) [][]SpeakerLine {
	// A line too large for one request is split, mirroring planGroups.
	if !linesFit([]SpeakerLine{line}, f.caps) {
		out := f.flushGroups()
		for _, part := range splitLineToFit(line, f.caps) {
			out = append(out, []SpeakerLine{part})
		}
		return out
	}

	if len(f.pending) > 0 && !canJoinGroup(ClipGroup{Lines: f.pending}, line, f.caps) {
		out := f.flushGroups()
		f.append(line)
		if f.budget > 0 && f.chars >= f.budget {
			out = append(out, f.flushGroups()...)
		}
		return out
	}

	f.append(line)
	if f.budget > 0 && f.chars >= f.budget {
		return f.flushGroups()
	}
	return nil
}

// Flush returns the pending group and clears it.
func (f *GroupFolder) Flush() []SpeakerLine {
	if len(f.pending) == 0 {
		return nil
	}
	out := f.pending
	f.pending = nil
	f.chars = 0
	return out
}

// flushGroups returns the pending group as a one-element slice, or nil.
func (f *GroupFolder) flushGroups() [][]SpeakerLine {
	if group := f.Flush(); group != nil {
		return [][]SpeakerLine{group}
	}
	return nil
}

// append adds a line and tracks its speakable length against the budget.
func (f *GroupFolder) append(line SpeakerLine) {
	f.pending = append(f.pending, line)
	f.chars += len([]rune(strings.TrimSpace(line.Text))) + 1
}
