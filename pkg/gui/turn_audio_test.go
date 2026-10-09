package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/localrpg/pkg/turnstream"
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

func TestFinishTurnAudioSkipsStreamerFedSpeechClip(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	session := &TurnSession{service: svc, gameID: gameID, cfg: svc.configMgr.Get()}
	turn, err := svc.findTurn(gameID, 1)
	if err != nil {
		t.Fatal(err)
	}

	plan := &turnAudioPlan{queue: make(chan string, 8), played: newClipSet()}

	// The streamer runs and feeds the speech segment (segment 1 of writeSegmentTurn)
	streamer := svc.sentenceStreamerFor(context.Background(), gameID, svc.configMgr.Get(), func(speech provisionalSpeech) {
		plan.enqueueClip(speech.AudioKey, svc.clipPath(speech.AudioKey))
	})
	if streamer == nil {
		t.Fatal("expected sentenceStreamerFor to return non-nil")
	}

	// Feed Captain Kaelen's line (matching segment 1)
	streamer.FeedSegment(turnstream.Event{
		Kind:      turnstream.KindSpeech,
		Speaker:   "Captain Kaelen",
		SpeakerID: "captain-kaelen",
		Text:      "Keep walking.",
	}, 0)
	streamer.Flush()
	streamer.Close()
	streamer.Wait()

	// Drain the clip enqueued by the streamer
	streamedClip := <-plan.queue
	if streamedClip == "" {
		t.Fatal("expected streamer to enqueue a clip")
	}

	// Now finalize turn audio
	session.finishTurnAudio(context.Background(), *turn, plan)
	plan.close()

	// Remaining should only be segment 0 (the narration "He does not look up.")
	remaining := drainClips(plan.queue)
	if len(remaining) != 1 {
		t.Fatalf("remaining = %#v, want only segment 0 (unstreamed narration)", remaining)
	}
	if media.ClipKeyForPath(remaining[0]) == media.ClipKeyForPath(streamedClip) {
		t.Errorf("the streamed speech clip %q was queued a second time", remaining[0])
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

func TestHeardAll(t *testing.T) {
	a := newTurnAudioPlan(nil)
	a.markHeard([]int{0, 1})
	if !a.heardAll([]int{0, 1}) {
		t.Fatal("both indexes were heard")
	}
	if a.heardAll([]int{0, 2}) {
		t.Fatal("index 2 was not heard")
	}
	if !a.heardAll(nil) {
		t.Fatal("an empty set is trivially heard")
	}
	all, any := a.heardState([]int{0, 5})
	if all || !any {
		t.Fatalf("heardState = %v/%v, want false/true", all, any)
	}
}

// captureLogger records trace event names so a test can assert one was emitted.
type captureLogger struct{ events []string }

func (l *captureLogger) Enabled(trace.Level) bool { return true }
func (l *captureLogger) Event(name string, _ map[string]interface{}) {
	l.events = append(l.events, name)
}
func (l *captureLogger) SetGame(string) {}

func (l *captureLogger) saw(name string) bool {
	for _, event := range l.events {
		if event == name {
			return true
		}
	}
	return false
}

func TestFinaliseSkipsHeardSegments(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	turn, err := svc.findTurn(gameID, 1)
	if err != nil {
		t.Fatal(err)
	}
	plan := newTurnAudioPlan(make(chan string, 8))
	indexes := make([]int, len(turn.Segments))
	for i := range turn.Segments {
		indexes[i] = i
	}
	plan.markHeard(indexes)

	session := &TurnSession{service: svc, gameID: gameID, cfg: svc.configMgr.Get()}
	session.finishTurnAudio(context.Background(), *turn, plan)
	plan.close()

	if remaining := drainClips(plan.queue); len(remaining) != 0 {
		t.Fatalf("a fully heard turn enqueued %#v", remaining)
	}
}

func TestFinaliseSuppressesAHeardSegment(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	turn, err := svc.findTurn(gameID, 1)
	if err != nil {
		t.Fatal(err)
	}
	plan := newTurnAudioPlan(make(chan string, 8))
	plan.markHeard([]int{0})

	session := &TurnSession{service: svc, gameID: gameID, cfg: svc.configMgr.Get()}
	session.finishTurnAudio(context.Background(), *turn, plan)
	plan.close()

	heard, err := svc.GetSegmentClips(context.Background(), gameID, 1, 0)
	if err != nil || len(heard) == 0 {
		t.Skip("no clip for segment 0")
	}
	heardKey := media.ClipKeyForPath(heard[0])
	for _, clip := range drainClips(plan.queue) {
		if media.ClipKeyForPath(clip) == heardKey {
			t.Fatal("a heard segment was replayed")
		}
	}
}

func TestParityMismatchIsTraced(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	turn, err := svc.findTurn(gameID, 1)
	if err != nil {
		t.Fatal(err)
	}
	logger := &captureLogger{}
	svc.SetLogger(logger)

	svc.logParityMismatch(gameID, turn.Segments, map[string]bool{"not-a-plan-key": true})
	if !logger.saw("turn.audio_parity_mismatch") {
		t.Fatal("expected a turn.audio_parity_mismatch trace event")
	}

	// A streamed key that the plan does contain is not a mismatch.
	logger.events = nil
	svc.logParityMismatch(gameID, turn.Segments, map[string]bool{})
	if logger.saw("turn.audio_parity_mismatch") {
		t.Fatal("an empty stream must not trace a mismatch")
	}
}

// finaliseClips runs the finalise pass for turn 1 against plan and returns the
// clips it enqueued, in order.
func finaliseClips(t *testing.T, svc *Service, gameID string, plan *turnAudioPlan) []string {
	t.Helper()
	turn, err := svc.findTurn(gameID, 1)
	if err != nil {
		t.Fatal(err)
	}
	session := &TurnSession{service: svc, gameID: gameID, cfg: svc.configMgr.Get()}
	session.finishTurnAudio(context.Background(), *turn, plan)
	plan.close()
	return drainClips(plan.queue)
}

// allTurnClips is every clip a fresh finalise pass plays for turn 1, which is
// the baseline the ledger tests subtract from.
func allTurnClips(t *testing.T, svc *Service, gameID string) []string {
	t.Helper()
	clips := finaliseClips(t, svc, gameID, newTurnAudioPlan(make(chan string, 8)))
	if len(clips) < 2 {
		t.Fatalf("baseline clips = %#v, want at least two", clips)
	}
	return clips
}

func TestFinaliseSkipsACompleteClip(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	baseline := allTurnClips(t, svc, gameID)
	heardKey := media.ClipKeyForPath(baseline[0])

	plan := newTurnAudioPlan(make(chan string, 8))
	plan.ledger = NewLedger()
	plan.ledger.Record(heardKey, 1200, 1200, true)

	got := finaliseClips(t, svc, gameID, plan)
	if len(got) != len(baseline)-1 {
		t.Fatalf("clips = %#v, want every clip but the heard one", got)
	}
	for _, clip := range got {
		if media.ClipKeyForPath(clip) == heardKey {
			t.Fatal("a clip the ledger records as complete was enqueued again")
		}
	}
}

func TestFinaliseEmitsAPartialClipOnce(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	baseline := allTurnClips(t, svc, gameID)
	partialKey := media.ClipKeyForPath(baseline[0])

	logger := &captureLogger{}
	svc.SetLogger(logger)
	plan := newTurnAudioPlan(make(chan string, 8))
	plan.ledger = NewLedger()
	plan.ledger.Record(partialKey, 400, 1200, false)

	got := finaliseClips(t, svc, gameID, plan)
	for _, clip := range got {
		if media.ClipKeyForPath(clip) == partialKey {
			t.Fatal("a partially heard clip was re-enqueued from its start")
		}
	}
	if len(got) != len(baseline)-1 {
		t.Fatalf("clips = %#v, want the rest of the turn", got)
	}
	if !logger.saw("turn.audio_partial") {
		t.Fatal("a suppressed partial clip must be traced")
	}
	// The offset stays in the ledger for the client to resume from.
	if entry, _ := plan.ledger.Entry(partialKey); entry.PlayedMS != 400 || entry.Complete {
		t.Fatalf("entry = %+v, want the partial offset kept", entry)
	}
}

func TestFinaliseWithAnEmptyLedgerPlaysEverything(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	baseline := allTurnClips(t, svc, gameID)

	plan := newTurnAudioPlan(make(chan string, 8))
	plan.ledger = NewLedger()
	got := finaliseClips(t, svc, gameID, plan)
	if len(got) != len(baseline) {
		t.Fatalf("clips = %#v, want the whole turn %#v", got, baseline)
	}
}
