package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// formatTTSClient returns MP3 bytes, which is what a real engine may hand back
// whatever the configuration implies.
type formatTTSClient struct {
	calls int
}

func (c *formatTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	c.calls++
	return []byte("ID3\x04\x00" + text), nil
}

func TestAudioExtensionSniffsTheBytes(t *testing.T) {
	cases := map[string]string{
		"RIFF....WAVEfmt ":           ".wav",
		"ID3\x04\x00":                ".mp3",
		"\xff\xfb\x90\x00":           ".mp3",
		"OggS\x00\x02":               ".ogg",
		"fLaC\x00\x00":               ".flac",
		"surprise bytes from a host": ".wav",
	}

	for body, want := range cases {
		if got := AudioExtension([]byte(body)); got != want {
			t.Errorf("AudioExtension(%q) = %q, want %q", body, got, want)
		}
	}

	if got := AudioContentType([]byte("ID3\x04\x00")); got != "audio/mpeg" {
		t.Errorf("AudioContentType = %q, want audio/mpeg", got)
	}
	if got := AudioContentType([]byte("OggS\x00\x02")); got != "audio/ogg" {
		t.Errorf("AudioContentType = %q, want audio/ogg", got)
	}
	if got := AudioContentType([]byte("fLaC\x00\x00")); got != "audio/flac" {
		t.Errorf("AudioContentType = %q, want audio/flac", got)
	}
	if got := AudioContentType([]byte("RIFF")); got != "audio/wav" {
		t.Errorf("AudioContentType = %q, want audio/wav", got)
	}
}

func TestSynthesizeUtteranceReusesALegacyWavNamedClip(t *testing.T) {
	client := &recordingTTSClient{}
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(client, cache)

	first, err := pipeline.SynthesizeUtterance(context.Background(), "narrator", nil, "hello")
	if err != nil {
		t.Fatalf("SynthesizeUtterance failed: %v", err)
	}

	// A cache written before clips were named honestly still holds a .wav, and a
	// second run must reuse it rather than synthesising again.
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, data, 0644); err != nil {
		t.Fatal(err)
	}

	second, err := pipeline.SynthesizeUtterance(context.Background(), "narrator", nil, "hello")
	if err != nil {
		t.Fatalf("second SynthesizeUtterance failed: %v", err)
	}
	if second != first {
		t.Errorf("expected the cached clip %q, got %q", first, second)
	}
	if client.calls != 1 {
		t.Errorf("expected 1 synthesis call, got %d", client.calls)
	}
}

func TestSynthesizeUtteranceNamesAClipFromItsBytes(t *testing.T) {
	client := &formatTTSClient{}
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(client, cache)

	path, err := pipeline.SynthesizeUtterance(context.Background(), "narrator", nil, "hello")
	if err != nil {
		t.Fatalf("SynthesizeUtterance failed: %v", err)
	}
	if ext := filepath.Ext(path); ext != ".mp3" {
		t.Errorf("clip path = %q, want an .mp3 name for MP3 bytes", path)
	}

	// The honest name is found on the next run, so nothing is regenerated.
	again, err := pipeline.SynthesizeUtterance(context.Background(), "narrator", nil, "hello")
	if err != nil {
		t.Fatalf("second SynthesizeUtterance failed: %v", err)
	}
	if again != path {
		t.Errorf("expected the cached clip %q, got %q", path, again)
	}
	if client.calls != 1 {
		t.Errorf("expected 1 synthesis call, got %d", client.calls)
	}
}

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

func TestSynthesizeSegmentPicksTheRightVoice(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	narrator := &entity.VoiceConfig{VoiceID: "narrator-voice"}
	voices := map[string]*entity.VoiceConfig{
		"garrick": {VoiceID: "bm_george"},
		"Sean":    {VoiceID: "player-voice"},
	}
	voiceFor := func(key string) *entity.VoiceConfig { return voices[key] }

	if _, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentNarration, Text: "The hall is quiet."}, narrator, voiceFor); err != nil {
		t.Fatalf("SynthesizeSegment failed: %v", err)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "narrator-voice" {
		t.Errorf("narration should use the narrator voice, got %+v", client.lastVoice)
	}

	if _, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentSpeech, SpeakerID: "garrick", Text: "Keep walking."}, narrator, voiceFor); err != nil {
		t.Fatalf("SynthesizeSegment failed: %v", err)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "bm_george" {
		t.Errorf("speech should use the speaker's voice, got %+v", client.lastVoice)
	}

	// A legacy record has a name but no ID; the name still resolves a voice.
	if _, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentSpeech, Speaker: "Sean", Text: "Hello."}, narrator, voiceFor); err != nil {
		t.Fatalf("SynthesizeSegment failed: %v", err)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "player-voice" {
		t.Errorf("legacy speech should resolve by name, got %+v", client.lastVoice)
	}
}

func TestTTSPipelineTracesCacheHitsAndMisses(t *testing.T) {
	dir := t.TempDir()
	memory := trace.NewMemory(trace.LevelFull)
	client := &mockTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(dir))
	pipeline.SetLogger(memory)

	voice := &entity.VoiceConfig{Provider: "kokoro", VoiceID: "af_bella", Pitch: 1, SpeechRate: 1}
	if _, err := pipeline.SynthesizeUtterance(context.Background(), "elena", voice, "Hello."); err != nil {
		t.Fatalf("SynthesizeUtterance failed: %v", err)
	}
	if _, err := pipeline.SynthesizeUtterance(context.Background(), "elena", voice, "Hello."); err != nil {
		t.Fatalf("second SynthesizeUtterance failed: %v", err)
	}

	results := make([]trace.Event, 0)
	for _, event := range memory.Events() {
		if event.Name == "media.tts.result" {
			results = append(results, event)
		}
	}
	if len(results) != 2 {
		t.Fatalf("expected two TTS results, got %d", len(results))
	}
	if results[0].Fields["cache_hit"] != false {
		t.Errorf("first synthesis should be a miss, got %+v", results[0].Fields)
	}
	if results[1].Fields["cache_hit"] != true {
		t.Errorf("second synthesis should be a hit, got %+v", results[1].Fields)
	}

	request, ok := memory.Find("media.tts.request")
	if !ok {
		t.Fatalf("expected a TTS request event, got %v", memory.Names())
	}
	if request.Fields["voice_id"] != "af_bella" || request.Fields["chars"] != 6 {
		t.Errorf("unexpected request fields: %+v", request.Fields)
	}
	if request.Fields["cache_key"] == nil {
		t.Errorf("expected the cache key so a clip can be found, got %+v", request.Fields)
	}
}
