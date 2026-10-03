package gui

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

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
	unit  speechUnit
	group []media.SpeakerLine
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
	// emit reports a completed unit and next is the monotonic ordinal it carries.
	// Both are optional: a streamer with no consumer still caches audio.
	emit func(provisionalSpeech)
	next uint64
	// stopped suppresses emission once the turn is authoritative. From then on the
	// played set is the client's, so a late unit is synthesized and played with the
	// rest of the turn rather than announced out of order.
	stopped atomic.Bool
}

// newSentenceStreamer builds a streamer with workers consuming the queue, calling
// emit once per completed unit. workers below one becomes one; emit may be nil.
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
	}
	for i := 0; i < workers; i++ {
		streamer.wg.Add(1)
		go streamer.worker()
	}
	return streamer
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
		if job.group != nil {
			s.synthesizeGroup(job.group)
			continue
		}
		s.synthesizeUnit(job.unit)
	}
}

// synthesizeUnit renders one sentence and announces its clip.
func (s *sentenceStreamer) synthesizeUnit(unit speechUnit) {
	path, err := s.pipeline.SynthesizeProvisional(s.ctx, unit.Kind, unit.SpeakerID, unit.Text, s.voiceForUnit(unit))
	if err != nil {
		s.logger.Event("media.tts.provisional_error", map[string]interface{}{"error": err.Error()})
		return
	}
	if !s.emitting() || path == "" {
		return
	}
	key := media.ClipKeyForPath(path)
	s.announce(unit.Text, key)
}

// synthesizeGroup renders one folded group with a single provider call, so its
// clip is keyed by the group and the finalise pass is a cache hit.
func (s *sentenceStreamer) synthesizeGroup(lines []media.SpeakerLine) {
	groups, err := s.pipeline.SynthesizeGroups(s.ctx, []media.ClipGroup{{Lines: lines}})
	if err != nil || len(groups) == 0 {
		s.logger.Event("media.tts.provisional_error", map[string]interface{}{"error": "synthesize group"})
		return
	}
	group := groups[0]
	if !s.emitting() || group.Key == "" {
		return
	}
	s.announce(media.GroupText(lines), group.Key)
}

// announce reports a finished clip, if a consumer is listening.
func (s *sentenceStreamer) announce(text, key string) {
	s.emit(provisionalSpeech{
		Index:    int(atomic.AddUint64(&s.next, 1) - 1),
		Text:     text,
		AudioKey: key,
		AudioURL: clipURL(key),
	})
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
		for _, group := range s.folder.Add(line) {
			s.enqueueGroup(group)
		}
		return
	}
	s.enqueueUnit(speechUnit{Kind: kind, SpeakerID: speakerID, Text: text})
}

// enqueueUnit sends a sentence without blocking, dropping it when the worker is
// far behind; the finalise pass synthesizes whatever was dropped.
func (s *sentenceStreamer) enqueueUnit(unit speechUnit) {
	select {
	case s.queue <- synthesisJob{unit: unit}:
	default:
	}
}

// enqueueGroup sends a folded group without blocking, on the same terms.
func (s *sentenceStreamer) enqueueGroup(group []media.SpeakerLine) {
	select {
	case s.queue <- synthesisJob{group: group}:
	default:
	}
}

// Close flushes the pending group, stops accepting work, and waits for the queue
// to drain.
func (s *sentenceStreamer) Close() {
	if s == nil {
		return
	}
	if s.grouping && s.folder != nil {
		if group := s.folder.Flush(); group != nil {
			s.enqueueGroup(group)
		}
	}
	s.closeOne.Do(func() { close(s.queue) })
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
