package gui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

var errQueueDropped = errors.New("queue full")

// sentenceQueueDepth bounds the sentences waiting for a worker. Generation is
// never blocked: a queue this deep means the provider is far behind, and a
// dropped sentence is simply synthesized by the finalise path instead.
const sentenceQueueDepth = 64

// provisionalSpeech is one unit the streamer synthesized: its ordinal within the
// turn and the clip that was written for it. A client plays it while the prose
// is still arriving, and the turn's own clip list is the same audio.
type provisionalSpeech struct {
	Index    int
	Text     string
	AudioKey string
	AudioURL string
}

// speechUnit is one sentence to speak, with the attribution its voice needs. A
// narration unit carries no speaker and is read by the narrator.
type speechUnit struct {
	Kind      string
	SpeakerID string
	Text      string
}

// synthesisJob is one worker request: a single sentence, or a group of lines the
// streamer folded together so they share one provider call.
type synthesisJob struct {
	seq   uint64
	unit  speechUnit
	group []media.SpeakerLine
}

// jobResult is the finished output of one synthesis job, sequenced before emit.
type jobResult struct {
	seq  uint64
	text string
	key  string
	err  error
}

// sentenceStreamer synthesizes a turn's audio as the model streams it, so a
// beat's audio is often already cached by the time the turn's segments are
// finalised. In its grouping mode it folds consecutive same-speaker segments into
// one request, under the same fold and capabilities the turn's clip plan uses, so
// the streamed clips are the clips the turn records.
type sentenceStreamer struct {
	ctx      context.Context
	pipeline *media.TTSPipeline
	narrator *entity.VoiceConfig
	// voiceFor resolves a speaker's voice, so streamed speech is read in the
	// character's own voice rather than the narrator's.
	voiceFor func(string) *entity.VoiceConfig
	logger   trace.Logger
	queue    chan synthesisJob
	wg       sync.WaitGroup
	closeOne sync.Once
	mu       sync.Mutex
	buf      strings.Builder
	// grouping and folder fold consecutive same-speaker lines into one request.
	grouping bool
	folder   *media.GroupFolder
	// emit reports a completed unit in submission order.
	emit func(provisionalSpeech)
	// sequencer maintains in-order announcements across concurrent workers.
	nextSeq      uint64
	announcedSeq uint64
	results      map[uint64]jobResult
	turnNumber       int
	readyCount       int
	progressObserver func(AudioProgressDTO)
	// stopped suppresses emission once the turn is authoritative. From then on the
	// played set is the client's, so a late unit is synthesized and played with the
	// rest of the turn rather than announced out of order.
	stopped atomic.Bool
}

// newSentenceStreamer builds a streamer with workers consuming the queue, calling
// emit once per completed unit in submission order. workers below one becomes one; emit may be nil.
func newSentenceStreamer(ctx context.Context, pipeline *media.TTSPipeline, voice *entity.VoiceConfig, logger trace.Logger, workers int, emit func(provisionalSpeech)) *sentenceStreamer {
	if workers < 1 {
		workers = 1
	}
	streamer := &sentenceStreamer{
		ctx:      ctx,
		pipeline: pipeline,
		narrator: voice,
		logger:   trace.OrNil(logger),
		queue:    make(chan synthesisJob, sentenceQueueDepth),
		emit:     emit,
		results:  make(map[uint64]jobResult),
	}
	for i := 0; i < workers; i++ {
		streamer.wg.Add(1)
		go streamer.worker()
	}
	return streamer
}

// SetTurnNumber informs the streamer which turn it is generating audio for.
func (s *sentenceStreamer) SetTurnNumber(turnNumber int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnNumber = turnNumber
}

// SetProgressObserver sets the callback for audio synthesis lifecycle stages.
func (s *sentenceStreamer) SetProgressObserver(observer func(AudioProgressDTO)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progressObserver = observer
}

func (s *sentenceStreamer) emitProgress(seq uint64, stage, key, url string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	observer := s.progressObserver
	readyCount := s.readyCount
	total := int(s.nextSeq)
	turnNum := s.turnNumber
	s.mu.Unlock()

	if observer != nil {
		observer(AudioProgressDTO{
			TurnNumber:    turnNum,
			Sequence:      int(seq),
			TotalSegments: total,
			Stage:         stage,
			ReadyCount:    readyCount,
			AudioKey:      key,
			AudioURL:      url,
		})
	}
}

// SetVoiceResolver supplies the per-speaker voice lookup speech is read with. A
// nil resolver leaves every unit in the narrator's voice.
func (s *sentenceStreamer) SetVoiceResolver(voiceFor func(string) *entity.VoiceConfig) {
	if s != nil {
		s.voiceFor = voiceFor
	}
}

// SetGrouping makes the streamer fold consecutive same-speaker lines into one
// request under caps, which must be the caps the turn's clip plan uses.
func (s *sentenceStreamer) SetGrouping(enabled bool, caps media.TTSCapabilities) {
	if s == nil {
		return
	}
	s.grouping = enabled
	if enabled {
		s.folder = media.NewGroupFolder(caps, 0)
	}
}

// voiceForUnit resolves the voice a unit is read in: the speaker's own for
// speech, the narrator's otherwise.
func (s *sentenceStreamer) voiceForUnit(unit speechUnit) *entity.VoiceConfig {
	if unit.Kind == entity.SegmentSpeech && unit.SpeakerID != "" && s.voiceFor != nil {
		if voice := s.voiceFor(unit.SpeakerID); voice != nil {
			return voice
		}
	}
	return s.narrator
}

// worker synthesizes queued jobs until the queue closes.
func (s *sentenceStreamer) worker() {
	defer s.wg.Done()
	for job := range s.queue {
		s.emitProgress(job.seq, "synthesizing", "", "")
		if job.group != nil {
			s.synthesizeGroup(job)
			continue
		}
		s.synthesizeUnit(job)
	}
}

// synthesizeUnit renders one sentence and sequences its clip.
func (s *sentenceStreamer) synthesizeUnit(job synthesisJob) {
	unit := job.unit
	path, err := s.pipeline.SynthesizeProvisional(s.ctx, unit.Kind, unit.SpeakerID, unit.Text, s.voiceForUnit(unit))
	if err != nil {
		s.logger.Event("media.tts.provisional_error", map[string]interface{}{"error": err.Error()})
		s.completeJob(jobResult{seq: job.seq, err: err})
		s.emitProgress(job.seq, "failed", "", "")
		return
	}
	key := ""
	if path != "" {
		key = media.ClipKeyForPath(path)
	}
	s.emitProgress(job.seq, "encoding", key, clipURL(key))
	s.completeJob(jobResult{seq: job.seq, text: unit.Text, key: key})
}

// synthesizeGroup renders one folded group with a single provider call, so its
// clip is keyed by the group and the finalise pass is a cache hit.
func (s *sentenceStreamer) synthesizeGroup(job synthesisJob) {
	lines := job.group
	groups, err := s.pipeline.SynthesizeGroups(s.ctx, []media.ClipGroup{{Lines: lines}})
	if err != nil || len(groups) == 0 {
		s.logger.Event("media.tts.provisional_error", map[string]interface{}{"error": "synthesize group"})
		s.completeJob(jobResult{seq: job.seq, err: errors.New("synthesize group failed")})
		s.emitProgress(job.seq, "failed", "", "")
		return
	}
	group := groups[0]
	s.emitProgress(job.seq, "encoding", group.Key, clipURL(group.Key))
	s.completeJob(jobResult{seq: job.seq, text: media.GroupText(lines), key: group.Key})
}

// completeJob records a finished job and drains any in-sequence completed results
// to emit, guaranteeing that units are announced in submission order.
func (s *sentenceStreamer) completeJob(res jobResult) {
	s.mu.Lock()
	if res.err == nil {
		s.readyCount++
	}
	s.completeJobLocked(res)
	s.mu.Unlock()
	if res.err == nil {
		s.emitProgress(res.seq, "ready", res.key, clipURL(res.key))
	}
}

func (s *sentenceStreamer) completeJobLocked(res jobResult) {
	s.results[res.seq] = res
	for {
		item, ok := s.results[s.announcedSeq]
		if !ok {
			break
		}
		delete(s.results, s.announcedSeq)
		s.announcedSeq++

		if item.err == nil && item.key != "" && s.emitting() {
			s.emit(provisionalSpeech{
				Index:    int(item.seq),
				Text:     item.text,
				AudioKey: item.key,
				AudioURL: clipURL(item.key),
			})
		}
	}
}

// emitting reports whether a completed unit should be announced.
func (s *sentenceStreamer) emitting() bool {
	return s.emit != nil && !s.stopped.Load()
}

// StopEmitting stops announcing units, which a session does once the turn is
// recorded: the finalise pass then plays whatever the stream produced. A unit
// still in flight is synthesized either way, so its clip is never wasted.
func (s *sentenceStreamer) StopEmitting() {
	if s == nil {
		return
	}
	s.stopped.Store(true)
}

// Feed adds raw narration text and queues every complete sentence, holding a
// partial sentence until more arrives. It is how a provider that emits no framing
// still gets live narration audio.
func (s *sentenceStreamer) Feed(text string) {
	if s == nil || s.pipeline == nil {
		return
	}

	s.mu.Lock()
	s.buf.WriteString(text)
	complete, remainder := media.SplitCompleteSentences(s.buf.String())
	s.buf.Reset()
	s.buf.WriteString(remainder)
	s.mu.Unlock()

	for _, sentence := range complete {
		s.feed(entity.SegmentNarration, "", sentence)
	}
}

// FeedSegment queues one parsed segment: a folded line when grouping, or its
// sentences otherwise, in the speaker's own voice.
func (s *sentenceStreamer) FeedSegment(event turnstream.Event) {
	if s == nil || s.pipeline == nil {
		return
	}
	kind := entity.SegmentNarration
	if event.Kind == turnstream.KindSpeech {
		kind = entity.SegmentSpeech
	}
	text := strings.TrimSpace(event.Text)
	if text == "" {
		return
	}

	// Grouping folds whole segments, because the turn's clip plan folds whole
	// segments; splitting here would build a different line and a different key.
	if s.grouping {
		s.feed(kind, event.SpeakerID, text)
		if event.Player {
			s.Flush()
		}
		return
	}

	complete, remainder := media.SplitCompleteSentences(text)
	if strings.TrimSpace(remainder) != "" {
		complete = append(complete, remainder)
	}
	for _, sentence := range complete {
		s.feed(kind, event.SpeakerID, sentence)
	}
}

// Flush flushes any pending folded group into the queue immediately.
func (s *sentenceStreamer) Flush() {
	if s == nil {
		return
	}
	s.mu.Lock()
	var group []media.SpeakerLine
	if s.grouping && s.folder != nil {
		group = s.folder.Flush()
	}
	s.mu.Unlock()
	if group != nil {
		s.enqueueGroup(group)
	}
}

// feed routes one piece of text to the folder or the sentence queue.
func (s *sentenceStreamer) feed(kind, speakerID, text string) {
	if s.grouping {
		line, ok := s.pipeline.SegmentLine(entity.TurnSegment{
			Kind:      kind,
			SpeakerID: speakerID,
			Text:      text,
		}, s.narrator, s.voiceFor)
		if !ok {
			return
		}
		s.mu.Lock()
		groups := s.folder.Add(line)
		s.mu.Unlock()
		for _, group := range groups {
			s.enqueueGroup(group)
		}
		return
	}
	s.enqueueUnit(speechUnit{Kind: kind, SpeakerID: speakerID, Text: text})
}

// enqueueUnit sends a sentence without blocking, sequencing its ordinal.
func (s *sentenceStreamer) enqueueUnit(unit speechUnit) {
	s.mu.Lock()
	seq := s.nextSeq
	s.nextSeq++
	dropped := false
	select {
	case s.queue <- synthesisJob{seq: seq, unit: unit}:
	default:
		s.completeJobLocked(jobResult{seq: seq, err: errQueueDropped})
		dropped = true
	}
	s.mu.Unlock()

	if dropped {
		s.emitProgress(seq, "failed", "", "")
	} else {
		s.emitProgress(seq, "waiting", "", "")
	}
}

// enqueueGroup sends a folded group without blocking, on the same terms.
func (s *sentenceStreamer) enqueueGroup(group []media.SpeakerLine) {
	s.mu.Lock()
	seq := s.nextSeq
	s.nextSeq++
	dropped := false
	select {
	case s.queue <- synthesisJob{seq: seq, group: group}:
	default:
		s.completeJobLocked(jobResult{seq: seq, err: errQueueDropped})
		dropped = true
	}
	s.mu.Unlock()

	if dropped {
		s.emitProgress(seq, "failed", "", "")
	} else {
		s.emitProgress(seq, "waiting", "", "")
	}
}

// Close flushes the pending group and closes the work queue.
func (s *sentenceStreamer) Close() {
	if s == nil {
		return
	}
	s.closeOne.Do(func() {
		s.Flush()
		close(s.queue)
	})
}

// Wait blocks until all queued synthesis jobs have finished.
func (s *sentenceStreamer) Wait() {
	if s == nil {
		return
	}
	s.wg.Wait()
}

// sentenceStreamerFor builds a pre-synthesiser for a turn, or nil when the
// configuration disables it or no TTS provider is configured. emit receives each
// completed unit's clip; it may be nil.
func (s *Service) sentenceStreamerFor(ctx context.Context, gameID string, cfg *config.Config, emit func(provisionalSpeech)) *sentenceStreamer {
	if !s.streamerRuns(cfg) {
		return nil
	}
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil
	}
	streamer := newSentenceStreamer(ctx, pipeline, s.narratorVoiceFor(gameID, cfg), s.logger, 2, emit)
	streamer.SetVoiceResolver(s.voiceFor(gameID))
	streamer.SetGrouping(s.liveGrouping(cfg, pipeline), media.LiveGroupCaps(pipeline.GroupCaps()))
	return streamer
}
