package media

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media/opus"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// formatTTSClient returns MP3 bytes, which is what a real engine may hand back
// whatever the configuration implies.
type formatTTSClient struct {
	calls int
}

func (c *formatTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	c.calls++
	return GenerateToneWAV(440, 0.02), nil
}

func TestAudioExtensionSniffsTheBytes(t *testing.T) {
	cases := map[string]string{
		"RIFF....WAVEfmt ":           ".wav",
		"ID3\x04\x00":                ".mp3",
		"\xff\xfb\x90\x00":           ".mp3",
		"OggS\x00\x02":               ".opus",
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

func TestSynthesizeUtteranceReusesTheCachedClip(t *testing.T) {
	client := &recordingTTSClient{}
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(client, cache)

	first, err := pipeline.SynthesizeUtterance(context.Background(), "narrator", nil, "hello")
	if err != nil {
		t.Fatalf("SynthesizeUtterance failed: %v", err)
	}

	// A second run reuses the cached Opus clip rather than synthesising again.
	second, err := pipeline.SynthesizeUtterance(context.Background(), "narrator", nil, "hello")
	if err != nil {
		t.Fatalf("second SynthesizeUtterance failed: %v", err)
	}
	if second != first {
		t.Errorf("expected the cached clip %q, got %q", first, second)
	}
	if filepath.Ext(first) != ".opus" {
		t.Errorf("clip path = %q, want an .opus name", first)
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
	if ext := filepath.Ext(path); ext != ".opus" {
		t.Errorf("clip path = %q, want an .opus name for normalised audio", path)
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
	return GenerateToneWAV(440, 0.01), nil
}

type recordingTTSClient struct {
	calls     int
	lastVoice *entity.VoiceConfig
}

func (c *recordingTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	c.calls++
	c.lastVoice = voice
	return GenerateToneWAV(440, 0.02), nil
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
		t.Fatalf("expected a clip list per segment, got %d", len(first))
	}
	if len(first[1]) != 1 {
		t.Fatalf("expected one clip for the single-sentence line, got %d", len(first[1]))
	}

	second, err := pipeline.SynthesizeSegments(context.Background(), segments, nil, voiceFor)
	if err != nil {
		t.Fatalf("second SynthesizeSegments failed: %v", err)
	}
	if len(second) != 2 || len(second[1]) != 1 || first[1][0] != second[1][0] {
		t.Errorf("expected the cached clip to be reused, got %#v then %#v", first, second)
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
	return GenerateToneWAV(440, 0.01), nil
}

func TestTTSPipeline_PerCharacterVoiceAndSpeed(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewContentCache(tmpDir)

	var lastSynthesizedVoice *entity.VoiceConfig
	client := &testTTSClient{
		onSynthesize: func(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
			lastSynthesizedVoice = voice
			return GenerateToneWAV(440, 0.02), nil
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

	if _, err := pipeline.SynthesizeSegmentClips(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentNarration, Text: "The hall is quiet."}, narrator, voiceFor, false); err != nil {
		t.Fatalf("SynthesizeSegmentClips failed: %v", err)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "narrator-voice" {
		t.Errorf("narration should use the narrator voice, got %+v", client.lastVoice)
	}

	if _, err := pipeline.SynthesizeSegmentClips(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentSpeech, SpeakerID: "garrick", Text: "Keep walking."}, narrator, voiceFor, false); err != nil {
		t.Fatalf("SynthesizeSegmentClips failed: %v", err)
	}
	if client.lastVoice == nil || client.lastVoice.VoiceID != "bm_george" {
		t.Errorf("speech should use the speaker's voice, got %+v", client.lastVoice)
	}

	// A legacy record has a name but no ID; the name still resolves a voice.
	if _, err := pipeline.SynthesizeSegmentClips(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentSpeech, Speaker: "Sean", Text: "Hello."}, narrator, voiceFor, false); err != nil {
		t.Fatalf("SynthesizeSegmentClips failed: %v", err)
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

func TestTTSPipelineNamesTheProviderInTheTrace(t *testing.T) {
	pipeline := NewTTSPipeline(&mockTTSClient{}, NewContentCache(t.TempDir()))
	memory := trace.NewMemory(trace.LevelFull)
	pipeline.SetLogger(memory)

	voice := &entity.VoiceConfig{
		Provider: "builtin:elevenlabs",
		VoiceID:  "v1",
		Options:  map[string]interface{}{"model": "eleven_turbo_v2_5"},
	}
	if _, err := pipeline.SynthesizeUtterance(context.Background(), "elena", voice, "Hello."); err != nil {
		t.Fatalf("SynthesizeUtterance: %v", err)
	}

	event, ok := memory.Find("media.tts.request")
	if !ok {
		t.Fatalf("no media.tts.request event recorded")
	}
	if event.Fields["provider"] != "builtin:elevenlabs" {
		t.Errorf("provider = %v", event.Fields["provider"])
	}
	if event.Fields["model"] != "eleven_turbo_v2_5" {
		t.Errorf("model = %v", event.Fields["model"])
	}
}

func TestSynthesizeUtteranceForceBypassesCache(t *testing.T) {
	client := &recordingTTSClient{}
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(client, cache)

	first, err := pipeline.SynthesizeUtterance(context.Background(), "speaker-1", nil, "Hello world")
	if err != nil {
		t.Fatalf("first synthesize: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("expected 1 call, got %d", client.calls)
	}

	// Normal call without force hits cache
	second, err := pipeline.SynthesizeUtterance(context.Background(), "speaker-1", nil, "Hello world")
	if err != nil {
		t.Fatalf("second synthesize: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("expected cached hit, but client was called %d times", client.calls)
	}
	if first != second {
		t.Errorf("expected same path for cached hit, got %s and %s", first, second)
	}

	// Forced call bypasses cache and increments calls
	third, err := pipeline.SynthesizeUtteranceForce(context.Background(), "speaker-1", nil, "Hello world", true)
	if err != nil {
		t.Fatalf("forced synthesize: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("expected 2 calls after force, got %d", client.calls)
	}
	if third == "" {
		t.Fatal("expected non-empty third path")
	}
}

// The cache holds one format. A clip that reached it another way - a campaign cached
// before the Opus migration, or a file placed by hand - is decoded and re-encoded, so a
// bundle never carries audio a browser refuses.
func TestNormalizeClipRepairsAFileThatIsNotOpus(t *testing.T) {
	dir := t.TempDir()
	cache := NewContentCache(dir)
	pipeline := NewTTSPipeline(&recordingTTSClient{}, cache)

	// A WAV clip sitting in the audio cache under an Opus key, as an older version left it.
	wav := GenerateToneWAV(440, 0.02)
	path, err := cache.Put("audio", "legacy-clip.opus", wav)
	if err != nil {
		t.Fatal(err)
	}
	if IsOpusClip(wav) {
		t.Fatal("the fixture is already Opus")
	}

	repaired, err := pipeline.NormalizeClip(path)
	if err != nil {
		t.Fatalf("NormalizeClip: %v", err)
	}

	data, err := os.ReadFile(repaired)
	if err != nil {
		t.Fatal(err)
	}
	if !IsOpusClip(data) {
		t.Errorf("repaired clip is not Ogg/Opus")
	}
	if filepath.Ext(repaired) != ".opus" {
		t.Errorf("repaired clip %q is not named for its format", repaired)
	}
}

func TestNormalizeClipLeavesAnOpusClipAlone(t *testing.T) {
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(&recordingTTSClient{}, cache)

	path, err := pipeline.SynthesizeUtterance(context.Background(), "narrator", nil, "hello")
	if err != nil {
		t.Fatal(err)
	}

	again, err := pipeline.NormalizeClip(path)
	if err != nil {
		t.Fatalf("NormalizeClip: %v", err)
	}
	if again != path {
		t.Errorf("NormalizeClip rewrote an Opus clip: %q", again)
	}
}

// A clip whose write was interrupted keeps its header and loses its body: a browser refuses
// it with "could not be decoded" while every header check passes. It must not be served.
func TestATruncatedClipIsReplacedRatherThanServed(t *testing.T) {
	dir := t.TempDir()
	cache := NewContentCache(dir)

	// A whole clip, cut short the way a killed write leaves it.
	writer := NewTTSPipeline(&recordingTTSClient{}, cache)
	whole, err := writer.SynthesizeUtterance(context.Background(), "garrick", &entity.VoiceConfig{VoiceID: "bm_george"}, "Keep your hood up.")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(whole)
	if err != nil {
		t.Fatal(err)
	}
	if !IsCompleteOpusStream(data) {
		t.Fatal("the fixture clip is not whole")
	}
	if err := os.WriteFile(whole, data[:len(data)*2/3], 0644); err != nil {
		t.Fatal(err)
	}

	// The same key, read again: the truncated file is not a clip, so it is replaced by a
	// fresh synthesis rather than handed to a player.
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, cache)
	replaced, err := pipeline.SynthesizeUtterance(context.Background(), "garrick", &entity.VoiceConfig{VoiceID: "bm_george"}, "Keep your hood up.")
	if err != nil {
		t.Fatalf("SynthesizeUtterance: %v", err)
	}
	if client.calls != 1 {
		t.Errorf("synthesis calls = %d, want the truncated clip to be replaced", client.calls)
	}

	replacedData, err := os.ReadFile(replaced)
	if err != nil {
		t.Fatal(err)
	}
	if !IsCompleteOpusStream(replacedData) {
		t.Error("the replaced clip is not whole")
	}
	if _, _, _, err := opus.Decode(replacedData); err != nil {
		t.Errorf("the replaced clip does not decode: %v", err)
	}
}

// The last page of a stream must mark the end of it (RFC 3533). Chrome plays a stream
// without the flag; stricter demuxers refuse the file, so an export re-encodes it.
func TestClipsEndProperly(t *testing.T) {
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(&recordingTTSClient{}, cache)
	path, err := pipeline.SynthesizeUtterance(context.Background(), "npc", &entity.VoiceConfig{VoiceID: "bm_george"}, "Keep your hood up.")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !hasEndOfStream(fresh) {
		t.Fatal("a freshly written clip does not mark the end of its stream")
	}
	if err := ClipProblem(fresh); err != nil {
		t.Errorf("a freshly written clip is not acceptable: %v", err)
	}

	// What the previous muxer wrote: the same stream with the flag cleared.
	older := append([]byte(nil), fresh...)
	last := lastPageOffset(older)
	older[last+5] &^= oggEndOfStream
	if hasEndOfStream(older) {
		t.Fatal("the fixture still marks the end of its stream")
	}
	if err := ClipProblem(older); err == nil {
		t.Error("a stream that does not end properly should be reported")
	}
}

// pageExtent reports where the page starting at offset ends.
func pageExtent(data []byte, offset int) int {
	segments := int(data[offset+26])
	body := 0
	for _, lacing := range data[offset+oggHeaderSize : offset+oggHeaderSize+segments] {
		body += int(lacing)
	}
	return offset + oggHeaderSize + segments + body
}

// lastPageOffset finds the start of the final page in a stream.
func lastPageOffset(data []byte) int {
	last := 0
	for offset := 0; offset < len(data); {
		segments := int(data[offset+26])
		body := 0
		for _, lacing := range data[offset+oggHeaderSize : offset+oggHeaderSize+segments] {
			body += int(lacing)
		}
		pageEnd := offset + oggHeaderSize + segments + body
		if pageEnd >= len(data) {
			return offset
		}
		last = pageEnd
		offset = pageEnd
	}
	return last
}

// A campaign cached by an older build holds streams that do not mark their end. An export
// must be able to repair those without a provider: the audio is already there, it only needs
// re-muxing, which is what makes a bundle playable in a browser that is strict about it.
func TestNormalizeClipRepairsAStreamThatDoesNotEndWithoutAProvider(t *testing.T) {
	cache := NewContentCache(t.TempDir())
	writer := NewTTSPipeline(&recordingTTSClient{}, cache)
	path, err := writer.SynthesizeUtterance(context.Background(), "npc", &entity.VoiceConfig{VoiceID: "bm_george"}, "Keep your hood up.")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Exactly what the older muxer wrote: the flag absent and the checksum still valid.
	older := append([]byte(nil), fresh...)
	last := lastPageOffset(older)
	older[last+5] &^= oggEndOfStream
	pageEnd := pageExtent(older, last)
	declared := append([]byte(nil), older[last:pageEnd]...)
	for i := 22; i < 26; i++ {
		declared[i] = 0
	}
	binary.LittleEndian.PutUint32(older[last+22:last+26], opus.OggCRC(declared))
	if err := os.WriteFile(path, older, 0644); err != nil {
		t.Fatal(err)
	}

	// A pipeline whose provider cannot synthesize anything: only re-muxing can fix it.
	repair := NewTTSPipeline(&deadTTS{}, cache)
	fixed, err := repair.NormalizeClip(path)
	if err != nil {
		t.Fatalf("NormalizeClip: %v", err)
	}

	data, err := os.ReadFile(fixed)
	if err != nil {
		t.Fatal(err)
	}
	if err := ClipProblem(data); err != nil {
		t.Errorf("the repaired clip is still not playable: %v", err)
	}
	if _, _, _, err := opus.Decode(data); err != nil {
		t.Errorf("the repaired clip does not decode: %v", err)
	}
}

// deadTTS cannot synthesize anything, so only re-muxing an existing clip can succeed.
type deadTTS struct{}

func (deadTTS) Synthesize(context.Context, string, *entity.VoiceConfig) ([]byte, error) {
	return nil, errors.New("no provider available")
}
