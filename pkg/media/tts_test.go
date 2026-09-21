package media

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type mockTTSClient struct {
	lastVoice string
	lastText  string
}

func (m *mockTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	m.lastText = text
	if voice != nil {
		m.lastVoice = voice.VoiceID
	}
	return []byte("mock-wav-bytes"), nil
}

type recordingTTSClient struct {
	calls     int
	lastVoice *entity.VoiceConfig
}

func (c *recordingTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	c.calls++
	c.lastVoice = voice
	return []byte("RIFF" + text), nil
}

func TestLegacySegmentsKeepProseAndAttributeObviousSpeakers(t *testing.T) {
	segments := LegacySegments("The hall is quiet.\nGarrick: \"Keep walking.\"")

	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %#v", segments)
	}
	if segments[0].Kind != entity.SegmentNarration || segments[0].Text != "The hall is quiet." {
		t.Errorf("unexpected first segment %#v", segments[0])
	}
	if segments[1].Kind != entity.SegmentSpeech || segments[1].Speaker != "Garrick" {
		t.Errorf("unexpected second segment %#v", segments[1])
	}
}

func TestSynthesizeSegmentsUsesPerSpeakerVoicesAndCaches(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The hall is quiet."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
	}

	voices := map[string]*entity.VoiceConfig{
		"garrick": {VoiceID: "bm_george", Pitch: 0.85, SpeechRate: 0.9},
	}
	voiceFor := func(speakerID string) *entity.VoiceConfig { return voices[speakerID] }

	first, err := pipeline.SynthesizeSegments(context.Background(), segments, nil, voiceFor)
	if err != nil {
		t.Fatalf("SynthesizeSegments failed: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("expected 2 clips, got %d", len(first))
	}

	second, err := pipeline.SynthesizeSegments(context.Background(), segments, nil, voiceFor)
	if err != nil {
		t.Fatalf("second SynthesizeSegments failed: %v", err)
	}
	if first[1] != second[1] {
		t.Errorf("expected the cached clip to be reused, got %q then %q", first[1], second[1])
	}
	if client.calls != 2 {
		t.Errorf("expected 2 synthesis calls across both runs, got %d", client.calls)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "bm_george" {
		t.Errorf("expected the speaker's voice to be used, got %+v", client.lastVoice)
	}
}

func TestTTSSynthesisWithCache(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewContentCache(tempDir)
	client := &mockTTSClient{}

	pipeline := NewTTSPipeline(client, cache)

	voice := &entity.VoiceConfig{Provider: "kokoro", VoiceID: "bf_emma"}
	path, err := pipeline.SynthesizeUtterance(context.Background(), "lady-evelyn", voice, "Thank you.")
	if err != nil {
		t.Fatalf("SynthesizeUtterance failed: %v", err)
	}

	if path == "" {
		t.Errorf("expected non-empty audio path")
	}

	// Verify cached
	if client.lastVoice != "bf_emma" || client.lastText != "Thank you." {
		t.Errorf("synthesis parameters mismatch: %+v", client)
	}
}

type testTTSClient struct {
	onSynthesize func(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error)
}

func (t *testTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	if t.onSynthesize != nil {
		return t.onSynthesize(ctx, text, voice)
	}
	return []byte("test-wav"), nil
}

func TestTTSPipeline_PerCharacterVoiceAndSpeed(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewContentCache(tmpDir)

	var lastSynthesizedVoice *entity.VoiceConfig
	client := &testTTSClient{
		onSynthesize: func(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
			lastSynthesizedVoice = voice
			return []byte("WAV_DATA_FOR_" + text), nil
		},
	}

	pipeline := NewTTSPipeline(client, cache)

	charVoice := &entity.VoiceConfig{
		VoiceID:    "af_bella",
		Pitch:      1.10,
		SpeechRate: 0.95,
	}

	path, err := pipeline.SynthesizeUtterance(context.Background(), "elena", charVoice, "I hear footsteps.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Errorf("expected non-empty cache path")
	}

	if lastSynthesizedVoice == nil || lastSynthesizedVoice.VoiceID != "af_bella" || lastSynthesizedVoice.SpeechRate != 0.95 {
		t.Errorf("expected character voice config with af_bella and 0.95 rate, got %+v", lastSynthesizedVoice)
	}
}

func TestSynthesizeSegmentsResolvesLegacySpeakersByName(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	segments := LegacySegments(`Garrick: "Keep walking."`)
	voices := map[string]*entity.VoiceConfig{"Garrick": {VoiceID: "bm_george"}}
	voiceFor := func(key string) *entity.VoiceConfig { return voices[key] }

	if _, err := pipeline.SynthesizeSegments(context.Background(), segments, nil, voiceFor); err != nil {
		t.Fatalf("SynthesizeSegments failed: %v", err)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "bm_george" {
		t.Errorf("expected a legacy speaker name to resolve a voice, got %+v", client.lastVoice)
	}
}
