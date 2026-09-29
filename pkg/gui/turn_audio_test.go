package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
)

func TestTurnAudioPlanSendsEachClipOnce(t *testing.T) {
	plan := &turnAudioPlan{queue: make(chan string, 4), played: newClipSet()}

	if !plan.enqueueClip("k1", "/cache/k1.opus") {
		t.Fatal("the first clip was refused")
	}
	if plan.enqueueClip("k1", "/cache/k1.opus") {
		t.Error("a clip the stream already played was sent twice")
	}
	if got := <-plan.queue; got != "/cache/k1.opus" {
		t.Errorf("queue received %q", got)
	}
}

func TestTurnAudioPlanWithoutAQueueSwallowsClips(t *testing.T) {
	plan := &turnAudioPlan{played: newClipSet()}
	if plan.enqueueClip("k1", "/cache/k1.opus") {
		t.Error("a plan with no queue accepted a clip")
	}
	plan.close()
}

func TestFinishTurnAudioSkipsStreamedClips(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	session := &TurnSession{service: svc, gameID: gameID, cfg: svc.configMgr.Get()}
	turn, err := svc.findTurn(gameID, 1)
	if err != nil {
		t.Fatal(err)
	}

	plan := &turnAudioPlan{queue: make(chan string, 8), played: newClipSet()}
	// The stream already played the first segment's clip.
	played, err := svc.GetSegmentClips(context.Background(), gameID, 1, 0)
	if err != nil || len(played) == 0 {
		t.Fatalf("GetSegmentClips = %#v, %v", played, err)
	}
	plan.enqueueClip(media.ClipKeyForPath(played[0]), played[0])
	// The player consumes what the stream produced before the turn is recorded.
	if got := <-plan.queue; got != played[0] {
		t.Fatalf("queue received %q, want the streamed clip", got)
	}

	session.finishTurnAudio(context.Background(), *turn, plan)
	plan.close()

	remaining := drainClips(plan.queue)
	if len(remaining) != 1 {
		t.Fatalf("remaining = %#v, want only the clip the stream had not played", remaining)
	}
	if media.ClipKeyForPath(remaining[0]) == media.ClipKeyForPath(played[0]) {
		t.Errorf("the streamed clip %q was queued a second time", remaining[0])
	}
}

func drainClips(queue <-chan string) []string {
	var clips []string
	for clip := range queue {
		clips = append(clips, clip)
	}
	return clips
}

func TestSpeechEventNamesTheClip(t *testing.T) {
	event := speechEvent(provisionalSpeech{Index: 3, Text: "One.", AudioKey: "abc", AudioURL: "/api/audio/clip/abc"})
	if event.Type != "speech" || event.Index != 3 || event.Text != "One." {
		t.Errorf("event = %+v", event)
	}
	if event.AudioKey != "abc" || event.AudioURL != "/api/audio/clip/abc" {
		t.Errorf("event = %+v, want the clip named", event)
	}
}

func TestTurnClipStreamYieldsEveryClipInOrder(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	clips := drainClips(svc.turnClipStream(gameID, 1, false))
	if len(clips) != 2 {
		t.Fatalf("clips = %#v, want one per segment", clips)
	}
	for i, clip := range clips {
		if clip == "" {
			t.Errorf("clip %d is empty", i)
		}
	}

	// A second run is a cache hit, in the same order.
	again := drainClips(svc.turnClipStream(gameID, 1, false))
	if len(again) != 2 || again[0] != clips[0] || again[1] != clips[1] {
		t.Errorf("second run = %#v, want the cached clips %#v", again, clips)
	}
}

func TestTurnClipStreamForAnUnknownTurnEndsImmediately(t *testing.T) {
	_, svc := setupTestGame(t)

	if clips := drainClips(svc.turnClipStream("test-campaign", 42, false)); len(clips) != 0 {
		t.Errorf("clips = %#v, want an empty stream", clips)
	}
}
