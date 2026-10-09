package gui

import (
	"regexp"
	"sync"

	"github.com/darkliquid/localrpg/pkg/media"
)

// clipKeyPattern is the whole shape of a clip key: a content-addressed name is
// 64 lowercase hex characters, so no client-supplied path can reach the cache.
var clipKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// clipURL names a clip by its key, which is what makes a clip's URL stable
// without a turn or a segment index.
func clipURL(key string) string {
	return "/api/audio/clip/" + key
}

// clipURLs names each clip in the order the segment plays them.
func clipURLs(clips []string) []string {
	urls := make([]string, 0, len(clips))
	for _, clip := range clips {
		urls = append(urls, clipURL(media.ClipKeyForPath(clip)))
	}
	return urls
}

// speechEvent frames a streamed sentence for a client-authority session, which
// plays the clip while the prose is still arriving.
func speechEvent(speech provisionalSpeech) TurnEvent {
	return TurnEvent{
		Type:     "speech",
		Index:    speech.Index,
		Text:     speech.Text,
		AudioKey: speech.AudioKey,
		AudioURL: speech.AudioURL,
	}
}

// clipSet records which clips have already been sent to the player. With one clip
// per unit it is exact: a unit heard is a unit skipped, and no unit is heard twice.
type clipSet struct {
	mu   sync.Mutex
	keys map[string]bool
}

func newClipSet() *clipSet {
	return &clipSet{keys: map[string]bool{}}
}

// take records a key and reports whether it was new.
func (c *clipSet) take(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.keys[key] {
		return false
	}
	c.keys[key] = true
	return true
}

// turnAudioPlan is a turn's playback state: the queue the streamer appends to when
// application playback is running, and the set of clips it has already sent. A plan
// with no queue is a client-authority or silent session, where clips are warmed but
// not played here.
type turnAudioPlan struct {
	queue  chan string
	played *clipSet
	// heard tracks the segment indexes the player has been sent audio for. It is
	// the authority for the finalise skip: a group whose segments are all heard is
	// not re-enqueued, whatever its key.
	heard map[int]bool
	// ledger is the turn's playback ledger: how much of each clip the player has
	// actually heard, merged from the client's offsets. Where heard answers "was
	// this segment sent", the ledger answers "how far did it play", so a clip the
	// browser finished is skipped and one it cut short is left for the client to
	// resume. Nil means no offsets are known, which leaves RB-2's skip in charge.
	ledger *Ledger
	mu     sync.Mutex
}

// clipVerdict is what the finalise pass does with one clip.
type clipVerdict int

const (
	// clipPlay means the clip is owed in full.
	clipPlay clipVerdict = iota
	// clipHeard means the clip was heard to its end and must not play again.
	clipHeard
	// clipPartial means the clip was cut short: the device cannot seek, so it is
	// suppressed here and the client resumes it from the ledger's offset.
	clipPartial
)

// verdict reads the ledger for one clip key.
func (a *turnAudioPlan) verdict(key string) (clipVerdict, Entry) {
	if a == nil || key == "" {
		return clipPlay, Entry{}
	}
	entry, ok := a.ledger.Entry(key)
	switch {
	case !ok:
		return clipPlay, Entry{}
	case entry.Complete:
		return clipHeard, entry
	case entry.PlayedMS > 0:
		return clipPartial, entry
	default:
		return clipPlay, entry
	}
}

// newTurnAudioPlan builds a plan with an optional playback queue. A nil queue
// warms clips without playing them here.
func newTurnAudioPlan(queue chan string) *turnAudioPlan {
	return &turnAudioPlan{queue: queue, played: newClipSet(), heard: map[int]bool{}}
}

// markHeard records that the player has been sent audio for each segment index.
func (a *turnAudioPlan) markHeard(indexes []int) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.heard == nil {
		a.heard = map[int]bool{}
	}
	for _, index := range indexes {
		a.heard[index] = true
	}
}

// heardAll reports whether every index has been heard. An empty set is trivially
// heard.
func (a *turnAudioPlan) heardAll(indexes []int) bool {
	all, _ := a.heardState(indexes)
	return all
}

// heardState reports whether all and whether any of the indexes have been heard.
// It is the one place the ledger is read, so all and any cannot disagree.
func (a *turnAudioPlan) heardState(indexes []int) (all, any bool) {
	if a == nil || len(indexes) == 0 {
		return true, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	all = true
	for _, index := range indexes {
		if a.heard[index] {
			any = true
			continue
		}
		all = false
	}
	return all, any
}

// enqueueClip sends one clip to the player once, reporting whether it was sent.
func (a *turnAudioPlan) enqueueClip(key, path string) bool {
	if a == nil || a.queue == nil || path == "" {
		return false
	}
	if a.played == nil || !a.played.take(key) {
		return false
	}
	select {
	case a.queue <- path:
		return true
	default:
		// The player is behind by more than a queue's depth; a dropped clip is
		// heard late rather than blocking the turn.
		return false
	}
}

// close ends the turn's queue, so the player stops when it has played everything.
func (a *turnAudioPlan) close() {
	if a != nil && a.queue != nil {
		close(a.queue)
	}
}
