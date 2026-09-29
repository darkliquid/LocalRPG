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

// sentenceStreamer synthesizes sentences as the model streams them, so a beat's
// audio is often already cached by the time the turn's segments are finalised.
type sentenceStreamer struct {
	ctx      context.Context
	pipeline *media.TTSPipeline
	voice    *entity.VoiceConfig
	logger   trace.Logger
	queue    chan string
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
		voice:    voice,
		logger:   trace.OrNil(logger),
		queue:    make(chan string, sentenceQueueDepth),
		emit:     emit,
	}
	for i := 0; i < workers; i++ {
		streamer.wg.Add(1)
		go streamer.worker()
	}
	return streamer
}

// worker synthesizes queued sentences until the queue closes. Narration is
// provisionally read in the narrator voice; dialogue cannot be attributed until
// submit_turn parses the segments, so it is left to the finalise path.
func (s *sentenceStreamer) worker() {
	defer s.wg.Done()
	for sentence := range s.queue {
		path, err := s.pipeline.SynthesizeProvisional(s.ctx, entity.SegmentNarration, "", sentence, s.voice)
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
			Text:     sentence,
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

// Feed adds streamed text and queues every complete sentence.
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
		select {
		case s.queue <- sentence:
		default:
		}
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
	if cfg.Media.TTS.Type == "" || cfg.Media.TTS.Type == "disabled" {
		return nil
	}
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil
	}
	return newSentenceStreamer(ctx, pipeline, s.narratorVoiceFor(gameID, cfg), s.logger, 2, emit)
}
