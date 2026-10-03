package gui

import (
	"context"
	"sync"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

type fakeSentenceTTSClient struct {
	mu     sync.Mutex
	calls  int
	voices []string
}

func (c *fakeSentenceTTSClient) Synthesize(_ context.Context, _ string, voice *entity.VoiceConfig) ([]byte, error) {
	c.mu.Lock()
	c.calls++
	if voice != nil {
		c.voices = append(c.voices, voice.VoiceID)
	} else {
		c.voices = append(c.voices, "")
	}
	c.mu.Unlock()
	return media.GenerateToneWAV(440, 0.01), nil
}

func (c *fakeSentenceTTSClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestSentenceStreamerVoicesSpeechWithItsSpeaker(t *testing.T) {
	client := &fakeSentenceTTSClient{}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(t.TempDir()))
	streamer := newSentenceStreamer(context.Background(), pipeline, &entity.VoiceConfig{VoiceID: "narrator"}, trace.Nop(), 1, nil)
	streamer.SetVoiceResolver(func(speakerID string) *entity.VoiceConfig {
		if speakerID == "kaelen" {
			return &entity.VoiceConfig{VoiceID: "kaelen-voice"}
		}
		return nil
	})

	streamer.FeedSegment(turnstream.Event{Kind: turnstream.KindSpeech, SpeakerID: "kaelen", Text: "Keep walking."})
	streamer.Close()

	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.voices) != 1 || client.voices[0] != "kaelen-voice" {
		t.Fatalf("voices = %#v, want the speaker's profile", client.voices)
	}
}

func TestSentenceStreamerSynthesizesCompleteSentencesOnly(t *testing.T) {
	client := &fakeSentenceTTSClient{}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(t.TempDir()))
	streamer := newSentenceStreamer(context.Background(), pipeline, &entity.VoiceConfig{VoiceID: "v1"}, trace.Nop(), 1, nil)

	streamer.Feed("The hall is quiet. Garrick")
	streamer.Feed(" steps inside.")
	streamer.Close()

	if got := client.callCount(); got != 2 {
		t.Fatalf("calls = %d, want 2 complete sentences", got)
	}
}

func TestSentenceStreamerEmitsOrderedSentences(t *testing.T) {
	client := &fakeSentenceTTSClient{}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(t.TempDir()))

	var mu sync.Mutex
	var got []provisionalSpeech
	streamer := newSentenceStreamer(context.Background(), pipeline, &entity.VoiceConfig{VoiceID: "v1"}, trace.Nop(), 1, func(speech provisionalSpeech) {
		mu.Lock()
		got = append(got, speech)
		mu.Unlock()
	})

	streamer.Feed("The hall is quiet. Garrick")
	streamer.Feed(" steps inside.")
	streamer.Close()

	if len(got) != 2 {
		t.Fatalf("events = %#v, want the two complete sentences", got)
	}
	if got[0].Text != "The hall is quiet." || got[1].Text != "Garrick steps inside." {
		t.Errorf("texts = %q, %q", got[0].Text, got[1].Text)
	}
	for i, speech := range got {
		if speech.Index != i {
			t.Errorf("event %d carries index %d, want its ordinal", i, speech.Index)
		}
		if speech.AudioKey == "" {
			t.Errorf("event %d names no clip key", i)
		}
		if speech.AudioURL != "/api/audio/clip/"+speech.AudioKey {
			t.Errorf("event %d url = %q, want the clip's content-addressed URL", i, speech.AudioURL)
		}
	}
}

func TestSentenceStreamerWithoutAConsumerIsSafe(t *testing.T) {
	client := &fakeSentenceTTSClient{}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(t.TempDir()))
	streamer := newSentenceStreamer(context.Background(), pipeline, &entity.VoiceConfig{VoiceID: "v1"}, trace.Nop(), 1, nil)

	streamer.Feed("The hall is quiet.")
	streamer.Close()

	if got := client.callCount(); got != 1 {
		t.Fatalf("calls = %d, want the sentence still synthesized", got)
	}
}

func TestSentenceStreamerStopsEmittingOnceTheTurnIsAuthoritative(t *testing.T) {
	client := &fakeSentenceTTSClient{}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(t.TempDir()))

	var mu sync.Mutex
	var got []provisionalSpeech
	streamer := newSentenceStreamer(context.Background(), pipeline, &entity.VoiceConfig{VoiceID: "v1"}, trace.Nop(), 1, func(speech provisionalSpeech) {
		mu.Lock()
		got = append(got, speech)
		mu.Unlock()
	})

	streamer.StopEmitting()
	streamer.Feed("The hall is quiet.")
	streamer.Close()

	if len(got) != 0 {
		t.Errorf("events = %#v, want none once the turn is authoritative", got)
	}
	if calls := client.callCount(); calls != 1 {
		t.Fatalf("calls = %d, want the sentence synthesized all the same", calls)
	}
}

func TestSentenceStreamerNilIsSafe(t *testing.T) {
	var streamer *sentenceStreamer
	streamer.Feed("text")
	streamer.StopEmitting()
	streamer.Close()
}
