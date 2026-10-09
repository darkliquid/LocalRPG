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
	// Segments are the turn-segment indexes this unit covers, so the turn plan can
	// record what the player has heard.
	Segments []int
}

// speechUnit is one sentence to speak, with the attribution its voice needs. A
// narration unit carries no speaker and is read by the narrator.
type speechUnit struct {
	Kind      string
	Speaker   string
	SpeakerID string
	Text      string
}

// synthesisJob is one worker request: a single sentence, or a group of lines the
// streamer folded together so they share one provider call.
type synthesisJob struct {
	seq      uint64
	unit     speechUnit
	group    []media.SpeakerLine
	segments []int
}

// jobResult is the finished output of one synthesis job, sequenced before emit.
type jobResult struct {
	seq      uint64
	text     string
	key      string
	segments []int
	err      error
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
	// segQueue holds the segment indexes of lines fed to the folder, in order, so
	// an emitted group can be attributed the segments it covers. lastSeg is the
	// most recently popped index, reused when a split line yields more groups than
	// indexes. unframedSegment indexes the folded paragraphs of the unframed path.
	segQueue        []int
	lastSeg         int
	unframedSegment int
	// emit reports a completed unit in submission order.
	emit func(provisionalSpeech)
	// sequencer maintains in-order announcements across concurrent workers.
	nextSeq          uint64
	announcedSeq     uint64
	results          map[uint64]jobResult
	turnNumber       int
	readyCount       int
	failedCount      int
	progressObserver func(AudioProgressDTO)
	owner            string
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
	failedCount := s.failedCount
	total := int(s.nextSeq)
	turnNum := s.turnNumber
	owner := s.owner
	s.mu.Unlock()

	if observer != nil {
		observer(AudioProgressDTO{
			TurnNumber:    turnNum,
			Sequence:      int(seq),
			TotalSegments: total,
			Stage:         stage,
			ReadyCount:    readyCount,
			FailedCount:   failedCount,
			AudioKey:      key,
			AudioURL:      url,
			Owner:         owner,
		})
	}
}

// SetOwner sets the streamer's playback owner ("device" or "browser").
func (s *sentenceStreamer) SetOwner(owner string) {
	if s != nil {
		s.mu.Lock()
		s.owner = owner
		s.mu.Unlock()
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
	if unit.Kind == entity.SegmentSpeech && s.voiceFor != nil {
		if unit.SpeakerID != "" {
			if voice := s.voiceFor(unit.SpeakerID); voice != nil {
				return voice
			}
		}
		if unit.Speaker != "" {
			if voice := s.voiceFor(unit.Speaker); voice != nil {
				return voice
			}
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
		return
	}
	key := ""
	if path != "" {
		key = media.ClipKeyForPath(path)
	}
	s.emitProgress(job.seq, "encoding", key, clipURL(key))
	s.completeJob(jobResult{seq: job.seq, text: unit.Text, key: key, segments: job.segments})
}

// synthesizeGroup renders one folded group with a single provider call, so its
// clip is keyed by the group and the finalise pass is a cache hit.
func (s *sentenceStreamer) synthesizeGroup(job synthesisJob) {
	lines := job.group
	groups, err := s.pipeline.SynthesizeGroups(s.ctx, []media.ClipGroup{{Lines: lines}})
	if err != nil || len(groups) == 0 {
		s.logger.Event("media.tts.provisional_error", map[string]interface{}{"error": "synthesize group"})
		s.completeJob(jobResult{seq: job.seq, err: errors.New("synthesize group failed")})
		return
	}
	group := groups[0]
	s.emitProgress(job.seq, "encoding", group.Key, clipURL(group.Key))
	s.completeJob(jobResult{seq: job.seq, text: media.GroupText(lines), key: group.Key, segments: job.segments})
}

// completeJob records a finished job and drains any in-sequence completed results
// to emit, guaranteeing that units are announced in submission order.
func (s *sentenceStreamer) completeJob(res jobResult) {
	s.mu.Lock()
	if res.err == nil {
		s.readyCount++
	} else {
		s.failedCount++
	}
	s.completeJobLocked(res)
	s.mu.Unlock()
	if res.err == nil {
		s.emitProgress(res.seq, "ready", res.key, clipURL(res.key))
	} else {
		s.emitProgress(res.seq, "failed", "", "")
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
				Segments: item.segments,
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

// Feed adds raw narration text and queues it, holding a partial unit until its
// boundary arrives. When grouping it folds a whole paragraph into one line, so
// its keys match the turn's plan; otherwise it splits into sentences. It is how a
// provider that emits no framing still gets live narration audio.
func (s *sentenceStreamer) Feed(text string) {
	if s == nil || s.pipeline == nil {
		return
	}

	s.mu.Lock()
	s.buf.WriteString(text)
	buffered := s.buf.String()
	var units []string
	if s.grouping {
		units, buffered = splitCompleteParagraphs(buffered)
	} else {
		units, buffered = media.SplitCompleteSentences(buffered)
	}
	s.buf.Reset()
	s.buf.WriteString(buffered)
	s.mu.Unlock()

	for _, unit := range units {
		s.feed(entity.SegmentNarration, "", "", unit, s.nextUnframedIndex())
	}
}

// splitCompleteParagraphs returns the paragraphs completed by a blank line and the
// trailing partial paragraph.
func splitCompleteParagraphs(text string) ([]string, string) {
	var out []string
	for {
		index := strings.Index(text, "\n\n")
		if index < 0 {
			break
		}
		if paragraph := strings.TrimSpace(text[:index]); paragraph != "" {
			out = append(out, paragraph)
		}
		text = text[index+2:]
	}
	return out, text
}

// FeedSegment queues one parsed segment: a folded line when grouping, or its
// sentences otherwise, in the speaker's own voice. segmentIndex is the event's
// position in the turn's segment order, the same index space the clip plan uses.
func (s *sentenceStreamer) FeedSegment(event turnstream.Event, segmentIndex int) {
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
		s.feed(kind, event.Speaker, event.SpeakerID, text, segmentIndex)
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
		s.feed(kind, event.Speaker, event.SpeakerID, sentence, segmentIndex)
	}
}

// Flush flushes any pending folded group and the unframed paragraph buffer into
// the queue immediately.
func (s *sentenceStreamer) Flush() {
	if s == nil {
		return
	}
	s.mu.Lock()
	var group []media.SpeakerLine
	var indexes []int
	if s.grouping && s.folder != nil {
		group = s.folder.Flush()
		if group != nil {
			indexes = s.takeIndexesLocked(len(group))
		}
	}
	trailing := ""
	if s.grouping {
		trailing = strings.TrimSpace(s.buf.String())
		s.buf.Reset()
	}
	s.mu.Unlock()
	if group != nil {
		s.enqueueGroup(group, indexes)
	}
	if trailing != "" {
		s.feed(entity.SegmentNarration, "", "", trailing, s.nextUnframedIndex())
	}
}

// takeIndexesLocked pops the segment indexes for the next emitted group. The
// caller holds the mutex. A split line yields more groups than indexes, so the
// last index is reused; the parity assertion catches any residual mismatch.
func (s *sentenceStreamer) takeIndexesLocked(n int) []int {
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if len(s.segQueue) > 0 {
			s.lastSeg = s.segQueue[0]
			s.segQueue = s.segQueue[1:]
		}
		out = append(out, s.lastSeg)
	}
	return out
}

// nextUnframedIndex hands out the segment index for a folded unframed paragraph.
func (s *sentenceStreamer) nextUnframedIndex() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.unframedSegment
	s.unframedSegment++
	return index
}

// feed routes one piece of text to the folder or the sentence queue.
func (s *sentenceStreamer) feed(kind, speaker, speakerID, text string, segmentIndex int) {
	if s.grouping {
		line, ok := s.pipeline.SegmentLine(entity.TurnSegment{
			Kind:      kind,
			Speaker:   speaker,
			SpeakerID: speakerID,
			Text:      text,
		}, s.narrator, s.voiceFor)
		if !ok {
			return
		}
		s.mu.Lock()
		s.segQueue = append(s.segQueue, segmentIndex)
		groups := s.folder.Add(line)
		attributed := make([][]media.SpeakerLine, len(groups))
		indexes := make([][]int, len(groups))
		for i, group := range groups {
			attributed[i] = group
			indexes[i] = s.takeIndexesLocked(len(group))
		}
		s.mu.Unlock()
		for i := range attributed {
			s.enqueueGroup(attributed[i], indexes[i])
		}
		return
	}
	s.enqueueUnit(speechUnit{Kind: kind, Speaker: speaker, SpeakerID: speakerID, Text: text}, segmentIndex)
}

// enqueueUnit sends a sentence without blocking, sequencing its ordinal.
func (s *sentenceStreamer) enqueueUnit(unit speechUnit, segmentIndex int) {
	s.mu.Lock()
	seq := s.nextSeq
	s.nextSeq++
	dropped := false
	select {
	case s.queue <- synthesisJob{seq: seq, unit: unit, segments: []int{segmentIndex}}:
	default:
		s.completeJobLocked(jobResult{seq: seq, err: errQueueDropped})
		s.failedCount++
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
func (s *sentenceStreamer) enqueueGroup(group []media.SpeakerLine, indexes []int) {
	s.mu.Lock()
	seq := s.nextSeq
	s.nextSeq++
	dropped := false
	select {
	case s.queue <- synthesisJob{seq: seq, group: group, segments: indexes}:
	default:
		s.completeJobLocked(jobResult{seq: seq, err: errQueueDropped})
		s.failedCount++
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
	streamer.SetGrouping(s.liveGrouping(cfg), pipeline.GroupCaps())
	return streamer
}
