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
}

// enqueueClip sends one clip to the player once, reporting whether it was sent.
func (a *turnAudioPlan) enqueueClip(key, path string) bool {
	if a == nil || a.queue == nil || path == "" {
		return false
	}
	if !a.played.take(key) {
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
