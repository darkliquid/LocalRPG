# LocalRPG Media Pipelines Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the media generation and multimodal pipelines for LocalRPG: dialogue utterance parsing with speaker attribution, multi-voice TTS (Kokoro/OpenAI/ElevenLabs), ComfyUI/SD WebUI image generation, Whisper Speech-to-Text (STT), and state-aware content caching for audio and art.

**Architecture:** A decoupled `pkg/media` package containing:
1. `ContentCache`: Deterministic hashing for speech clips `SHA256(speaker_id + voice_hash + text)` and image assets `SHA256(entity_id + appearance_hash + style_hash)` so character changes (e.g. voice injury, new scars) invalidate caches predictably while avoiding redundant re-generation.
2. `TTSPipeline`: Parses raw GM narrative into *Narrator Prose* and *Attributed Character Dialogue*, resolves per-entity voice settings from Markdown frontmatter, and synthesizes audio clips.
3. `ImagePipeline`: Generates scene illustrations and character portraits by blending world art style tags with entity physical traits via ComfyUI / SD WebUI / OpenAI APIs.
4. `STTPipeline`: Speech-to-Text transcriber for player voice dictation.

**Tech Stack:** Go 1.27, standard library (`crypto/sha256`, `net/http`, `os`, `io`), `github.com/darkliquid/localrpg/pkg/entity`, `github.com/darkliquid/localrpg/pkg/storage`.

---

### File Structure Map

```text
LocalRPG/
├── cmd/
│   └── localrpg/
│       ├── main.go               # Updated with "tts" and "image" subcommands
│       ├── media.go              # CLI handlers for tts and image commands
│       └── media_test.go         # CLI media command integration tests
├── pkg/
│   └── media/
│       ├── cache.go              # Content-addressable file cache & state-aware hashers
│       ├── cache_test.go         # Cache key generation & invalidation tests
│       ├── tts.go                # Dialogue segmenter, speaker resolver, & TTS provider
│       ├── tts_test.go           # Utterance splitting & speech synthesis tests
│       ├── image.go              # Image generator interface, ComfyUI/WebUI client
│       ├── image_test.go         # Image prompt synthesis & generation tests
│       ├── stt.go                # Whisper Speech-to-Text provider interface
│       └── stt_test.go           # Audio transcription tests
```

---

### Task 1: State-Aware Content Caching (Audio & Art)

**Files:**
- Create: `pkg/media/cache.go`
- Test: `pkg/media/cache_test.go`

- [ ] **Step 1: Write the failing test for Content Cache**

```go
// pkg/media/cache_test.go
package media

import (
	"path/filepath"
	"testing"
)

func TestStateAwareAudioCacheKey(t *testing.T) {
	speakerID := "lady-evelyn"
	voiceConfigA := "kokoro:bf_emma:1.0"
	voiceConfigB := "kokoro:bf_emma:0.8:raspy" // voice damaged/altered
	text := "Thank you, traveler."

	keyA1 := ComputeAudioCacheKey(speakerID, voiceConfigA, text)
	keyA2 := ComputeAudioCacheKey(speakerID, voiceConfigA, text)
	keyB := ComputeAudioCacheKey(speakerID, voiceConfigB, text)

	if keyA1 != keyA2 {
		t.Errorf("expected deterministic cache key for identical parameters")
	}
	if keyA1 == keyB {
		t.Errorf("expected altered voice config to produce different cache key")
	}
}

func TestStateAwareArtCacheKey(t *testing.T) {
	entityID := "alden-tavern"
	worldStyle := "oil painting, dark fantasy"
	appearanceA := "Cozy wooden tavern with glowing hearth"
	appearanceB := "Burned-out ruins of wooden tavern" // altered after fire

	keyA := ComputeArtCacheKey(entityID, appearanceA, worldStyle)
	keyB := ComputeArtCacheKey(entityID, appearanceB, worldStyle)

	if keyA == keyB {
		t.Errorf("expected altered appearance to produce different cache key")
	}
}

func TestContentCacheFileStorage(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewContentCache(tempDir)

	data := []byte("audio payload bytes")
	path, err := cache.Put("audio", "sample-key.ogg", data)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if !cache.Exists("audio", "sample-key.ogg") {
		t.Errorf("expected file to exist at %s", path)
	}

	loaded, err := cache.Get("audio", "sample-key.ogg")
	if err != nil || string(loaded) != string(data) {
		t.Errorf("cache read mismatch: got %v, err=%v", string(loaded), err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/... -v -run TestStateAwareAudioCacheKey`  
Expected: FAIL (package/media not defined)

- [ ] **Step 3: Implement Content Cache**

Write `pkg/media/cache.go`:
```go
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

func ComputeAudioCacheKey(speakerID, voiceConfigHash, utteranceText string) string {
	hasher := sha256.New()
	hasher.Write([]byte(speakerID + ":" + voiceConfigHash + ":" + utteranceText))
	return hex.EncodeToString(hasher.Sum(nil))
}

func ComputeArtCacheKey(entityID, appearanceHash, worldStyleHash string) string {
	hasher := sha256.New()
	hasher.Write([]byte(entityID + ":" + appearanceHash + ":" + worldStyleHash))
	return hex.EncodeToString(hasher.Sum(nil))
}

type ContentCache struct {
	baseDir string
}

func NewContentCache(baseDir string) *ContentCache {
	return &ContentCache{baseDir: baseDir}
}

func (c *ContentCache) Subdir(category string) string {
	dir := filepath.Join(c.baseDir, category)
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func (c *ContentCache) Exists(category, filename string) bool {
	path := filepath.Join(c.Subdir(category), filename)
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (c *ContentCache) Put(category, filename string, data []byte) (string, error) {
	path := filepath.Join(c.Subdir(category), filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write cache file: %w", err)
	}
	return path, nil
}

func (c *ContentCache) Get(category, filename string) ([]byte, error) {
	path := filepath.Join(c.Subdir(category), filename)
	return os.ReadFile(path)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/... -v -run TestStateAware`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/cache.go pkg/media/cache_test.go
git commit -m "feat(media): implement state-aware content caching for audio and art"
```

---

### Task 2: Multi-Voice Dialogue Parser & TTS Pipeline

**Files:**
- Create: `pkg/media/tts.go`
- Test: `pkg/media/tts_test.go`

- [ ] **Step 1: Write failing test for Dialogue Segmenter and TTS Engine**

```go
// pkg/media/tts_test.go
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

func TestParseDialogueSegments(t *testing.T) {
	narrative := `The cold wind whistles through the cracks in the door.
Lady Evelyn: "You shouldn't have come here alone, traveler."
You reach for your sword, but she shakes her head.
"Put that away," she whispers.`

	segments := ParseDialogueSegments(narrative, "Narrator")

	if len(segments) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(segments))
	}

	if segments[0].Speaker != "Narrator" || !segments[0].IsNarrator {
		t.Errorf("expected segment 0 to be Narrator, got %+v", segments[0])
	}
	if segments[1].Speaker != "Lady Evelyn" || segments[1].IsNarrator {
		t.Errorf("expected segment 1 to be Lady Evelyn, got %+v", segments[1])
	}
	if segments[2].Speaker != "Narrator" {
		t.Errorf("expected segment 2 to be Narrator, got %+v", segments[2])
	}
	if segments[3].Speaker != "Lady Evelyn" {
		t.Errorf("expected continued dialogue to attribute to Lady Evelyn, got %+v", segments[3])
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/... -v -run TestParseDialogueSegments`  
Expected: FAIL (ParseDialogueSegments not defined)

- [ ] **Step 3: Implement Dialogue Parser & TTS Pipeline**

Write `pkg/media/tts.go`:
```go
package media

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type UtteranceSegment struct {
	Speaker    string
	Text       string
	IsNarrator bool
}

var attributedSpeechRegex = regexp.MustCompile(`^([^:\n]+):\s*"([^"]+)"`)
var quotedSpeechRegex = regexp.MustCompile(`"([^"]+)"`)

func ParseDialogueSegments(text, defaultNarrator string) []UtteranceSegment {
	lines := strings.Split(text, "\n")
	var segments []UtteranceSegment
	lastSpeaker := defaultNarrator

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for "Speaker: "..." pattern
		if match := attributedSpeechRegex.FindStringSubmatch(line); len(match) == 3 {
			speaker := strings.TrimSpace(match[1])
			quote := match[2]
			lastSpeaker = speaker
			segments = append(segments, UtteranceSegment{
				Speaker:    speaker,
				Text:       quote,
				IsNarrator: false,
			})
			continue
		}

		// Check for quoted speech inside prose
		if match := quotedSpeechRegex.FindStringSubmatch(line); len(match) == 2 {
			quote := match[1]
			// Dialogue part
			segments = append(segments, UtteranceSegment{
				Speaker:    lastSpeaker,
				Text:       quote,
				IsNarrator: false,
			})
			continue
		}

		// Default to narrator prose
		segments = append(segments, UtteranceSegment{
			Speaker:    defaultNarrator,
			Text:       line,
			IsNarrator: true,
		})
	}

	return segments
}

type TTSClient interface {
	Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error)
}

type TTSPipeline struct {
	client TTSClient
	cache  *ContentCache
}

func NewTTSPipeline(client TTSClient, cache *ContentCache) *TTSPipeline {
	return &TTSPipeline{
		client: client,
		cache:  cache,
	}
}

func (p *TTSPipeline) SynthesizeUtterance(ctx context.Context, speakerID string, voice *entity.VoiceConfig, text string) (string, error) {
	voiceHash := "default"
	if voice != nil {
		voiceHash = fmt.Sprintf("%s:%s:%.2f", voice.Provider, voice.VoiceID, voice.Pitch)
	}

	cacheKey := ComputeAudioCacheKey(speakerID, voiceHash, text) + ".wav"
	if p.cache.Exists("audio", cacheKey) {
		return filepathJoin(p.cache.Subdir("audio"), cacheKey), nil
	}

	audioBytes, err := p.client.Synthesize(ctx, text, voice)
	if err != nil {
		return "", fmt.Errorf("synthesize utterance: %w", err)
	}

	return p.cache.Put("audio", cacheKey, audioBytes)
}

func filepathJoin(dir, file string) string {
	return dir + "/" + file
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/... -v -run TestParseDialogue`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/tts.go pkg/media/tts_test.go
git commit -m "feat(media): implement multi-voice dialogue parser and TTS synthesis pipeline"
```

---

### Task 3: Image Generation Pipeline (ComfyUI / SD WebUI / Remote)

**Files:**
- Create: `pkg/media/image.go`
- Test: `pkg/media/image_test.go`

- [ ] **Step 1: Write failing test for Image Generator**

```go
// pkg/media/image_test.go
package media

import (
	"context"
	"testing"
)

type mockImageClient struct {
	lastPrompt string
}

func (m *mockImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	m.lastPrompt = prompt
	return []byte("mock-png-image-bytes"), nil
}

func TestImagePipelinePromptCompositionAndCache(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewContentCache(tempDir)
	client := &mockImageClient{}

	pipeline := NewImagePipeline(client, cache)

	entityID := "alden-tavern"
	appearance := "Misty tavern with dark wooden beams"
	worldStyle := "oil painting, dark fantasy, gritty"

	path, err := pipeline.GenerateSceneImage(context.Background(), entityID, appearance, worldStyle)
	if err != nil {
		t.Fatalf("GenerateSceneImage failed: %v", err)
	}

	if path == "" {
		t.Errorf("expected non-empty image path")
	}

	expectedPrompt := "Misty tavern with dark wooden beams, oil painting, dark fantasy, gritty"
	if client.lastPrompt != expectedPrompt {
		t.Errorf("expected prompt %q, got %q", expectedPrompt, client.lastPrompt)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/... -v -run TestImagePipeline`  
Expected: FAIL (NewImagePipeline not defined)

- [ ] **Step 3: Implement Image Pipeline**

Write `pkg/media/image.go`:
```go
package media

import (
	"context"
	"fmt"
	"strings"
)

type ImageClient interface {
	GenerateImage(ctx context.Context, prompt string) ([]byte, error)
}

type ImagePipeline struct {
	client ImageClient
	cache  *ContentCache
}

func NewImagePipeline(client ImageClient, cache *ContentCache) *ImagePipeline {
	return &ImagePipeline{
		client: client,
		cache:  cache,
	}
}

func (p *ImagePipeline) BuildPrompt(appearance, worldStyle string) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(appearance) != "" {
		parts = append(parts, strings.TrimSpace(appearance))
	}
	if strings.TrimSpace(worldStyle) != "" {
		parts = append(parts, strings.TrimSpace(worldStyle))
	}
	return strings.Join(parts, ", ")
}

func (p *ImagePipeline) GenerateSceneImage(ctx context.Context, entityID, appearance, worldStyle string) (string, error) {
	cacheKey := ComputeArtCacheKey(entityID, appearance, worldStyle) + ".webp"
	if p.cache.Exists("images", cacheKey) {
		return filepathJoin(p.cache.Subdir("images"), cacheKey), nil
	}

	prompt := p.BuildPrompt(appearance, worldStyle)
	imgBytes, err := p.client.GenerateImage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("generate image for %q: %w", entityID, err)
	}

	return p.cache.Put("images", cacheKey, imgBytes)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/... -v -run TestImagePipeline`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/image.go pkg/media/image_test.go
git commit -m "feat(media): implement image generation pipeline with prompt composition and caching"
```

---

### Task 4: Speech-to-Text (STT) Player Voice Input

**Files:**
- Create: `pkg/media/stt.go`
- Test: `pkg/media/stt_test.go`

- [ ] **Step 1: Write failing test for STT Provider**

```go
// pkg/media/stt_test.go
package media

import (
	"context"
	"testing"
)

type mockSTTClient struct {
	transcription string
}

func (m *mockSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	return m.transcription, nil
}

func TestSTTTranscription(t *testing.T) {
	client := &mockSTTClient{transcription: "I draw my bow and aim at the scout."}
	provider := NewSTTProvider(client)

	text, err := provider.TranscribeAudio(context.Background(), []byte("wav bytes"))
	if err != nil {
		t.Fatalf("TranscribeAudio failed: %v", err)
	}

	if text != "I draw my bow and aim at the scout." {
		t.Errorf("transcription mismatch: %s", text)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/... -v -run TestSTTTranscription`  
Expected: FAIL (NewSTTProvider not defined)

- [ ] **Step 3: Implement STT Provider**

Write `pkg/media/stt.go`:
```go
package media

import (
	"context"
	"fmt"
)

type STTClient interface {
	Transcribe(ctx context.Context, audioData []byte) (string, error)
}

type STTProvider struct {
	client STTClient
}

func NewSTTProvider(client STTClient) *STTProvider {
	return &STTProvider{client: client}
}

func (s *STTProvider) TranscribeAudio(ctx context.Context, audioData []byte) (string, error) {
	if len(audioData) == 0 {
		return "", fmt.Errorf("empty audio data")
	}
	return s.client.Transcribe(ctx, audioData)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/... -v -run TestSTTTranscription`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/stt.go pkg/media/stt_test.go
git commit -m "feat(media): implement Speech-to-Text provider interface"
```

---

### Task 5: CLI Media Subcommands (`localrpg tts` and `localrpg image`)

**Files:**
- Create: `cmd/localrpg/media.go`
- Modify: `cmd/localrpg/main.go`
- Test: `cmd/localrpg/media_test.go`

- [ ] **Step 1: Write integration tests for CLI media commands**

```go
// cmd/localrpg/media_test.go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLITTSCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "tts", "Hello world")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v, output: %s", err, string(out))
	}

	if !strings.Contains(string(out), "Synthesizing TTS: Hello world") {
		t.Errorf("unexpected output: %s", string(out))
	}
}

func TestCLIImageCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "image", "A tavern in the mist")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v, output: %s", err, string(out))
	}

	if !strings.Contains(string(out), "Generating image: A tavern in the mist") {
		t.Errorf("unexpected output: %s", string(out))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/localrpg/... -v -run TestCLITTSCommand`  
Expected: FAIL (tts subcommand not handled)

- [ ] **Step 3: Implement CLI Media Handlers**

Write `cmd/localrpg/media.go`:
```go
package main

import (
	"flag"
	"fmt"
	"os"
)

func handleTTSCommand(args []string) {
	fs := flag.NewFlagSet("tts", flag.ExitOnError)
	speaker := fs.String("speaker", "narrator", "Speaker or character name")
	fs.Parse(args)

	text := fs.Arg(0)
	if text == "" {
		fmt.Fprintln(os.Stderr, "Usage: localrpg tts [--speaker <name>] <text to speak>")
		os.Exit(1)
	}

	fmt.Printf("Synthesizing TTS: %s (speaker: %s)\n", text, *speaker)
}

func handleImageCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: localrpg image <prompt>")
		os.Exit(1)
	}

	prompt := args[0]
	fmt.Printf("Generating image: %s\n", prompt)
}
```

Update `cmd/localrpg/main.go` to dispatch `case "tts": handleTTSCommand(args[1:])` and `case "image": handleImageCommand(args[1:])`.

- [ ] **Step 4: Run all package tests across workspace**

Run: `go test -count=1 ./... -v`  
Expected: All package tests PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/localrpg/media.go cmd/localrpg/main.go cmd/localrpg/media_test.go
git commit -m "feat(cli): add 'tts' and 'image' subcommands"
```
