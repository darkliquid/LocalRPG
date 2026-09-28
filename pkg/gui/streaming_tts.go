package gui

import (
	"context"
	"strings"
	"sync"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// sentenceQueueDepth bounds the sentences waiting for a worker. Generation is
// never blocked: a queue this deep means the provider is far behind, and a
// dropped sentence is simply synthesized by the finalise path instead.
const sentenceQueueDepth = 64

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
}

// newSentenceStreamer builds a streamer with workers consuming the queue.
// workers below one becomes one.
func newSentenceStreamer(ctx context.Context, pipeline *media.TTSPipeline, voice *entity.VoiceConfig, logger trace.Logger, workers int) *sentenceStreamer {
	if workers < 1 {
		workers = 1
	}
	streamer := &sentenceStreamer{
		ctx:      ctx,
		pipeline: pipeline,
		voice:    voice,
		logger:   trace.OrNil(logger),
		queue:    make(chan string, sentenceQueueDepth),
	}
	for i := 0; i < workers; i++ {
		streamer.wg.Add(1)
		go streamer.worker()
	}
	return streamer
}

// worker synthesizes queued sentences until the queue closes.
func (s *sentenceStreamer) worker() {
	defer s.wg.Done()
	for sentence := range s.queue {
		// Narration is provisionally read in the narrator voice; dialogue cannot
		// be attributed until submit_turn parses the segments, so it is left to
		// the finalise path.
		if _, err := s.pipeline.SynthesizeProvisional(s.ctx, entity.SegmentNarration, "", sentence, s.voice); err != nil {
			s.logger.Event("media.tts.provisional_error", map[string]interface{}{"error": err.Error()})
		}
	}
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
// the configuration disables it or no TTS provider is configured.
func (s *Service) sentenceStreamerFor(ctx context.Context, gameID string, cfg *config.Config) *sentenceStreamer {
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
	return newSentenceStreamer(ctx, pipeline, s.narratorVoiceFor(gameID, cfg), s.logger, 2)
}
