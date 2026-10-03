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

// provisionalSpeech is one sentence the streamer synthesized: its ordinal within
// the turn and the clip that was written for it. A client plays it while the prose
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

// sentenceStreamer synthesizes sentences as the model streams them, so a beat's
// audio is often already cached by the time the turn's segments are finalised.
type sentenceStreamer struct {
	ctx      context.Context
	pipeline *media.TTSPipeline
	narrator *entity.VoiceConfig
	// voiceFor resolves a speaker's voice, so streamed speech is read in the
	// character's own voice rather than the narrator's.
	voiceFor func(string) *entity.VoiceConfig
	logger   trace.Logger
	queue    chan speechUnit
	wg       sync.WaitGroup
	closeOne sync.Once
	mu       sync.Mutex
	buf      strings.Builder
	// emit reports a completed sentence and next is the monotonic ordinal it
	// carries. Both are optional: a streamer with no consumer still caches audio.
	emit func(provisionalSpeech)
	next uint64
	// stopped suppresses emission once the turn is authoritative. From then on the
	// played set is the client's, so a late sentence is synthesized and played with
	// the rest of the turn rather than announced out of order.
	stopped atomic.Bool
}

// newSentenceStreamer builds a streamer with workers consuming the queue, calling
// emit once per completed sentence. workers below one becomes one; emit may be nil.
func newSentenceStreamer(ctx context.Context, pipeline *media.TTSPipeline, voice *entity.VoiceConfig, logger trace.Logger, workers int, emit func(provisionalSpeech)) *sentenceStreamer {
	if workers < 1 {
		workers = 1
	}
	streamer := &sentenceStreamer{
		ctx:      ctx,
		pipeline: pipeline,
		narrator: voice,
		logger:   trace.OrNil(logger),
		queue:    make(chan speechUnit, sentenceQueueDepth),
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

// worker synthesizes queued sentences until the queue closes, each in its own
// speaker's voice.
func (s *sentenceStreamer) worker() {
	defer s.wg.Done()
	for unit := range s.queue {
		path, err := s.pipeline.SynthesizeProvisional(s.ctx, unit.Kind, unit.SpeakerID, unit.Text, s.voiceForUnit(unit))
		if err != nil {
			s.logger.Event("media.tts.provisional_error", map[string]interface{}{"error": err.Error()})
			continue
		}
		if !s.emitting() || path == "" {
			continue
		}
		key := media.ClipKeyForPath(path)
		s.emit(provisionalSpeech{
			Index:    int(atomic.AddUint64(&s.next, 1) - 1),
			Text:     unit.Text,
			AudioKey: key,
			AudioURL: clipURL(key),
		})
	}
}

// emitting reports whether a completed sentence should be announced.
func (s *sentenceStreamer) emitting() bool {
	return s.emit != nil && !s.stopped.Load()
}

// StopEmitting stops announcing sentences, which a session does once the turn is
// recorded: the finalise pass then plays whatever the stream produced. A sentence
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
		s.enqueue(speechUnit{Kind: entity.SegmentNarration, Text: sentence})
	}
}

// FeedSegment queues every sentence of one parsed segment, attributed to its
// speaker, so streamed speech is voiced with the character's profile.
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
	complete, remainder := media.SplitCompleteSentences(text)
	if strings.TrimSpace(remainder) != "" {
		complete = append(complete, remainder)
	}
	for _, sentence := range complete {
		s.enqueue(speechUnit{Kind: kind, SpeakerID: event.SpeakerID, Text: sentence})
	}
}

// enqueue sends a unit without blocking, dropping it when the worker is far
// behind; the finalise pass synthesizes whatever was dropped.
func (s *sentenceStreamer) enqueue(unit speechUnit) {
	select {
	case s.queue <- unit:
	default:
	}
}

// Close stops accepting sentences and waits for the queued ones to finish.
func (s *sentenceStreamer) Close() {
	if s == nil {
		return
	}
	s.closeOne.Do(func() { close(s.queue) })
	s.wg.Wait()
}

// sentenceStreamerFor builds a sentence pre-synthesiser for a turn, or nil when
// the configuration disables it or no TTS provider is configured. emit receives
// each completed sentence's clip; it may be nil.
func (s *Service) sentenceStreamerFor(ctx context.Context, gameID string, cfg *config.Config, emit func(provisionalSpeech)) *sentenceStreamer {
	if !cfg.TTSStreamSentences() {
		return nil
	}
	// Grouping and sentence streaming are alternative strategies: a streamed
	// sentence is a cache miss for a group, so only one runs for a turn.
	if cfg.TTSGrouping() == "always" {
		return nil
	}
	if cfg.Media.TTS.Type == "" || cfg.Media.TTS.Type == "disabled" {
		return nil
	}
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil
	}
	streamer := newSentenceStreamer(ctx, pipeline, s.narratorVoiceFor(gameID, cfg), s.logger, 2, emit)
	streamer.SetVoiceResolver(s.voiceFor(gameID))
	return streamer
}
