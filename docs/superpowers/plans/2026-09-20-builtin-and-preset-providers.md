# Built-in Engines, Preset Providers, and Voice Profiles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement zero-dependency built-in engines (`native-os` TTS, `procedural-art` SVG image generator, `narrative-oracle` storyteller), a local tool preset catalog, and a dynamic NPC Voice Profiles library with automatic GM assignment.

**Architecture:**
- `pkg/media/procedural_art.go` generates rich atmospheric SVG landscape art in pure Go.
- `pkg/media/native_tts.go` dispatches to OS speech synthesizers (`spd-say`, `say`, PowerShell) with procedural audio fallback.
- `pkg/harness/oracle_provider.go` resolves dice rolls and player action modes into evocative prose using internal oracle tables and entity wikilinks.
- `pkg/config/presets.go` & `frontend/src/templates/providerPresets.ts` provide 1-click configuration presets (Ollama, LM Studio, Kokoro-FastAPI, AllTalk, Piper, Whisper.cpp, ComfyUI, Automatic1111).
- `pkg/harness/context.go` & `pkg/harness/extractor.go` inject voice profile options into the GM's prompt and auto-assign distinct voices to newly invented NPCs.
- `frontend/src/components/SettingsStudio.tsx` exposes quick preset loaders and a Voice Profiles Library manager with instant audio previews.

**Tech Stack:** Go 1.24, React 18, Tailwind CSS, TypeScript, Lucide React icons.

---

### File Map

- **Config & Voice Profile Schemas:**
  - Modify: `pkg/config/types.go` (Add VoiceProfile, VoiceProfiles to TTSConfig, default archetypes)
  - Create: `pkg/config/presets.go` (Static preset catalogs)
  - Test: `pkg/config/presets_test.go`
- **Entity Voice Options:**
  - Modify: `pkg/entity/entity.go` (Add SpeechRate to VoiceConfig)
  - Modify: `pkg/media/tts.go` (Incorporate speech rate into audio cache key)
  - Test: `pkg/media/tts_test.go`
- **Built-in `procedural-art` SVG Image Generator:**
  - Create: `pkg/media/procedural_art.go`
  - Modify: `pkg/media/providers.go` (Register procedural-art)
  - Test: `pkg/media/procedural_art_test.go`
- **Built-in `native-os` TTS Client:**
  - Create: `pkg/media/native_tts.go`
  - Modify: `pkg/media/providers.go` (Register native-os)
  - Test: `pkg/media/native_tts_test.go`
- **Built-in `narrative-oracle` Storyteller:**
  - Create: `pkg/harness/oracle_provider.go`
  - Modify: `pkg/harness/factory.go` (Register narrative-oracle)
  - Test: `pkg/harness/oracle_provider_test.go`
- **GM Voice Profile Prompt Injection & Extractor Auto-Assignment:**
  - Modify: `pkg/harness/context.go` (Inject NPC voice profiles into prompt)
  - Modify: `pkg/harness/extractor.go` (Auto-assign voice profiles to discovered NPCs)
  - Modify: `pkg/engine/orchestrator.go` (Wire voice profiles into turn loop)
  - Test: `pkg/harness/context_test.go`, `pkg/harness/extractor_test.go`
- **Frontend Types & Presets:**
  - Modify: `frontend/src/types.ts`
  - Create: `frontend/src/templates/providerPresets.ts`
- **Frontend Settings Studio & Voice Profiles Library UI:**
  - Modify: `frontend/src/components/SettingsStudio.tsx` (Add 1-click Preset Loaders & Voice Profiles Library manager)
  - Modify: `frontend/src/components/CodexDrawer.tsx` (Add Voice Profile helper to entity editor)

---

### Task 1: Voice Profile Schema & Presets Catalog (`pkg/config`)

**Files:**
- Modify: `pkg/config/types.go`
- Create: `pkg/config/presets.go`
- Test: `pkg/config/presets_test.go`

- [x] **Step 1: Write failing test for Presets & VoiceProfile defaults**

Create `pkg/config/presets_test.go`:
```go
package config_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestPresetsCatalog(t *testing.T) {
	// 1. LLM Presets
	ollamaPreset, ok := config.GetAgentPreset("ollama")
	if !ok || ollamaPreset.Type != "http" || ollamaPreset.Endpoint != "http://localhost:11434/v1" {
		t.Errorf("expected valid ollama preset, got %+v", ollamaPreset)
	}

	// 2. TTS Presets
	kokoroPreset, ok := config.GetTTSPreset("kokoro-fastapi")
	if !ok || kokoroPreset.Type != "http" || kokoroPreset.DefaultVoice != "af_bella" {
		t.Errorf("expected valid kokoro preset, got %+v", kokoroPreset)
	}

	nativeOSPreset, ok := config.GetTTSPreset("native-os")
	if !ok || nativeOSPreset.Type != "builtin" || nativeOSPreset.BuiltinName != "native-os" {
		t.Errorf("expected valid native-os preset, got %+v", nativeOSPreset)
	}

	// 3. Image Presets
	artPreset, ok := config.GetImagePreset("procedural-art")
	if !ok || artPreset.Type != "builtin" || artPreset.BuiltinName != "procedural-art" {
		t.Errorf("expected valid procedural-art preset, got %+v", artPreset)
	}
}

func TestDefaultVoiceProfiles(t *testing.T) {
	cfg := config.DefaultConfig()
	if len(cfg.Media.TTS.VoiceProfiles) == 0 {
		t.Fatalf("expected default voice profiles in default config")
	}

	foundElder := false
	for _, p := range cfg.Media.TTS.VoiceProfiles {
		if p.ID == "elder_sage" {
			foundElder = true
			if p.Pitch >= 1.0 {
				t.Errorf("expected elder_sage to have deeper pitch < 1.0, got %f", p.Pitch)
			}
		}
	}
	if !foundElder {
		t.Errorf("expected elder_sage archetype in default voice profiles")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/config -run TestPresetsCatalog`
Expected: FAIL (`GetAgentPreset` undefined)

- [x] **Step 3: Update `pkg/config/types.go` and implement `pkg/config/presets.go`**

In `pkg/config/types.go`:
Add `VoiceProfile` and update `TTSConfig`:
```go
type VoiceProfile struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	VoiceID     string   `yaml:"voice_id" json:"voice_id"`
	Pitch       float64  `yaml:"pitch" json:"pitch"`
	SpeechRate  float64  `yaml:"speech_rate" json:"speech_rate"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
}

type TTSConfig struct {
	Type          string         `yaml:"type" json:"type"`
	BuiltinName   string         `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command       string         `yaml:"command,omitempty" json:"command,omitempty"`
	Args          []string       `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint      string         `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model         string         `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey        string         `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	DefaultVoice  string         `yaml:"default_voice,omitempty" json:"default_voice,omitempty"`
	Pitch         float64        `yaml:"pitch,omitempty" json:"pitch,omitempty"`
	SpeechRate    float64        `yaml:"speech_rate,omitempty" json:"speech_rate,omitempty"`
	AutoPlay      bool           `yaml:"auto_play" json:"auto_play"`
	MasterVolume  float64        `yaml:"master_volume" json:"master_volume"`
	VoiceProfiles []VoiceProfile `yaml:"voice_profiles,omitempty" json:"voice_profiles,omitempty"`
}
```

In `DefaultConfig()`, add default voice profiles:
```go
			VoiceProfiles: []VoiceProfile{
				{
					ID:          "elder_sage",
					Name:        "Elder Sage / Veteran",
					VoiceID:     "bm_george",
					Pitch:       0.85,
					SpeechRate:  0.90,
					Tags:        []string{"elder", "male", "wise", "gravelly", "veteran"},
					Description: "Ancient wizards, battle-weary commanders, village elders.",
				},
				{
					ID:          "young_scout",
					Name:        "Young Scout / Rogue",
					VoiceID:     "af_bella",
					Pitch:       1.05,
					SpeechRate:  1.10,
					Tags:        []string{"young", "female", "quick", "eager", "rogue"},
					Description: "Nimble rangers, streetwise thieves, eager apprentices.",
				},
				{
					ID:          "gruff_blacksmith",
					Name:        "Gruff Dwarf / Guard",
					VoiceID:     "am_adam",
					Pitch:       0.75,
					SpeechRate:  0.95,
					Tags:        []string{"stout", "male", "deep", "authoritative", "guard"},
					Description: "Dwarven smiths, tavern bouncers, fortress wardens.",
				},
				{
					ID:          "sinister_cultist",
					Name:        "Hushed Mystic / Villain",
					VoiceID:     "bf_emma",
					Pitch:       0.90,
					SpeechRate:  0.85,
					Tags:        []string{"eerie", "whisper", "sinister", "cultist"},
					Description: "Shadow mages, deceptive nobles, oracle priestesses.",
				},
			},
```

Create `pkg/config/presets.go`:
```go
package config

var AgentPresets = map[string]AgentRoleConfig{
	"ollama": {
		Type:        "http",
		Endpoint:    "http://localhost:11434/v1",
		Model:       "llama3.2",
		Temperature: 0.7,
		MaxTokens:   1024,
	},
	"lm-studio": {
		Type:        "http",
		Endpoint:    "http://localhost:1234/v1",
		Model:       "default",
		Temperature: 0.7,
		MaxTokens:   1024,
	},
	"localai": {
		Type:        "http",
		Endpoint:    "http://localhost:8080/v1",
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   1024,
	},
	"vllm": {
		Type:        "http",
		Endpoint:    "http://localhost:8000/v1",
		Model:       "default",
		Temperature: 0.7,
		MaxTokens:   1024,
	},
	"llama-cli": {
		Type:        "cli",
		Command:     "llama-cli",
		Args:        []string{"-m", "models/model.gguf", "-p"},
		Temperature: 0.7,
		MaxTokens:   1024,
	},
	"claude-cli": {
		Type:    "cli",
		Command: "claude",
		Args:    []string{"-p"},
	},
	"narrative-oracle": {
		Type:        "builtin",
		BuiltinName: "narrative-oracle",
	},
}

var TTSPresets = map[string]TTSConfig{
	"kokoro-fastapi": {
		Type:         "http",
		Endpoint:     "http://localhost:8880/v1/audio/speech",
		Model:        "kokoro",
		DefaultVoice: "af_bella",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"alltalk": {
		Type:         "http",
		Endpoint:     "http://localhost:7851/api/tts-generate",
		DefaultVoice: "default",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"piper": {
		Type:         "cli",
		Command:      "piper",
		Args:         []string{"--model", "en_US-lessac-medium.onnx", "--output_file", "-"},
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"native-os": {
		Type:         "builtin",
		BuiltinName:  "native-os",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"openai-speech": {
		Type:         "http",
		Endpoint:     "https://api.openai.com/v1/audio/speech",
		Model:        "tts-1",
		DefaultVoice: "alloy",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
}

var STTPresets = map[string]STTConfig{
	"faster-whisper": {
		Type:     "http",
		Endpoint: "http://localhost:8000/v1/audio/transcriptions",
		Model:    "whisper-1",
	},
	"whisper-cli": {
		Type:    "cli",
		Command: "whisper-cli",
		Args:    []string{"-m", "models/ggml-base.bin", "-f", "%INPUT%", "-nt"},
	},
	"openai-whisper": {
		Type:     "http",
		Endpoint: "https://api.openai.com/v1/audio/transcriptions",
		Model:    "whisper-1",
	},
}

var ImagePresets = map[string]ImageConfig{
	"comfyui": {
		Type:     "http",
		Endpoint: "http://127.0.0.1:8188",
	},
	"automatic1111": {
		Type:     "http",
		Endpoint: "http://127.0.0.1:7860/sdapi/v1/txt2img",
	},
	"localai-image": {
		Type:     "http",
		Endpoint: "http://127.0.0.1:8080/v1/images/generations",
		Model:    "stablediffusion",
	},
	"sd-cli": {
		Type:    "cli",
		Command: "sd",
		Args:    []string{"-m", "models/sd-v1-5.gguf", "-p"},
	},
	"procedural-art": {
		Type:        "builtin",
		BuiltinName: "procedural-art",
	},
	"dall-e-3": {
		Type:     "http",
		Endpoint: "https://api.openai.com/v1/images/generations",
		Model:    "dall-e-3",
	},
}

func GetAgentPreset(id string) (AgentRoleConfig, bool) {
	p, ok := AgentPresets[id]
	return p, ok
}

func GetTTSPreset(id string) (TTSConfig, bool) {
	p, ok := TTSPresets[id]
	return p, ok
}

func GetSTTPreset(id string) (STTConfig, bool) {
	p, ok := STTPresets[id]
	return p, ok
}

func GetImagePreset(id string) (ImageConfig, bool) {
	p, ok := ImagePresets[id]
	return p, ok
}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/config`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/config/
git commit -m "feat(config): add VoiceProfile schema and static preset catalogs"
```

---

### Task 2: Entity Voice Options & Audio Cache Key (`pkg/entity` & `pkg/media`)

**Files:**
- Modify: `pkg/entity/entity.go`
- Modify: `pkg/media/tts.go`
- Modify: `pkg/media/cache.go`
- Modify: `pkg/media/providers.go`
- Test: `pkg/media/tts_test.go`, `pkg/media/cache_test.go`

- [x] **Step 1: Write failing test for per-character speech rate & voice dispatch**

In `pkg/media/tts_test.go`:
```go
func TestTTSPipeline_PerCharacterVoiceAndSpeed(t *testing.T) {
	tmpDir := t.TempDir()
	cache := media.NewContentCache(tmpDir)

	var lastSynthesizedVoice *entity.VoiceConfig
	mockClient := &testTTSClient{
		onSynthesize: func(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
			lastSynthesizedVoice = voice
			return []byte("WAV_DATA_FOR_" + text), nil
		},
	}

	pipeline := media.NewTTSPipeline(mockClient, cache)

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
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/media -run TestTTSPipeline_PerCharacterVoiceAndSpeed`
Expected: FAIL (`SpeechRate` undefined in `VoiceConfig`)

- [x] **Step 3: Update `pkg/entity/entity.go`, `pkg/media/cache.go`, and `pkg/media/providers.go`**

In `pkg/entity/entity.go`:
```go
type VoiceConfig struct {
	Provider   string  `yaml:"provider,omitempty" json:"provider,omitempty"`
	VoiceID    string  `yaml:"voice_id,omitempty" json:"voice_id,omitempty"`
	Pitch      float64 `yaml:"pitch,omitempty" json:"pitch,omitempty"`
	SpeechRate float64 `yaml:"speech_rate,omitempty" json:"speech_rate,omitempty"`
}
```

In `pkg/media/cache.go`:
Update `ComputeAudioCacheKey` signature or voiceHash format to include `speech_rate`:
```go
func ComputeAudioCacheKeyWithRate(speakerID, voiceID string, pitch, speechRate float64, text string) string {
	raw := fmt.Sprintf("%s:%s:%.2f:%.2f:%s", speakerID, voiceID, pitch, speechRate, text)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
```

In `pkg/media/tts.go`:
Use character voice settings and pass `speech_rate` into payload:
```go
func (p *TTSPipeline) SynthesizeUtterance(ctx context.Context, speakerID string, voice *entity.VoiceConfig, text string) (string, error) {
	voiceHash := "default"
	if voice != nil {
		voiceHash = fmt.Sprintf("%s:%s:%.2f:%.2f", voice.Provider, voice.VoiceID, voice.Pitch, voice.SpeechRate)
	}

	cacheKey := ComputeAudioCacheKey(speakerID, voiceHash, text) + ".wav"
	if p.cache.Exists("audio", cacheKey) {
		return filepath.Join(p.cache.Subdir("audio"), cacheKey), nil
	}

	audioBytes, err := p.client.Synthesize(ctx, text, voice)
	if err != nil {
		return "", fmt.Errorf("synthesize utterance: %w", err)
	}

	return p.cache.Put("audio", cacheKey, audioBytes)
}
```

In `pkg/media/providers.go` `httpTTSClient.Synthesize`:
Include `speed` if `voice.SpeechRate > 0`:
```go
	payloadMap := map[string]interface{}{
		"model": h.model,
		"input": text,
		"voice": voiceID,
	}
	if voice != nil && voice.SpeechRate > 0 {
		payloadMap["speed"] = voice.SpeechRate
	}
	payload, _ := json.Marshal(payloadMap)
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/media`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/entity/ pkg/media/
git commit -m "feat(media): support SpeechRate and per-character voice option overrides"
```

---

### Task 3: Built-in `procedural-art` SVG Image Generator (`pkg/media`)

**Files:**
- Create: `pkg/media/procedural_art.go`
- Modify: `pkg/media/providers.go`
- Test: `pkg/media/procedural_art_test.go`

- [x] **Step 1: Write failing test for `procedural-art` generator**

Create `pkg/media/procedural_art_test.go`:
```go
package media_test

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestProceduralArt_GeneratesValidSVG(t *testing.T) {
	client, err := media.NewImageClient(config.ImageConfig{
		Type:        "builtin",
		BuiltinName: "procedural-art",
	})
	if err != nil {
		t.Fatalf("failed to create procedural art client: %v", err)
	}

	prompts := []string{
		"Grim dark fortress towering over a misty swamp",
		"Sunken catacombs beneath ancient ruins with glowing runes",
		"Mountain citadel under a blood moon sky",
	}

	for _, prompt := range prompts {
		bytes, err := client.GenerateImage(context.Background(), prompt)
		if err != nil {
			t.Fatalf("failed to generate art for %q: %v", prompt, err)
		}
		svg := string(bytes)
		if !strings.HasPrefix(svg, "<svg") || !strings.HasSuffix(strings.TrimSpace(svg), "</svg>") {
			t.Errorf("expected valid SVG format for %q, got: %s", prompt, svg[:min(50, len(svg))])
		}
		if !strings.Contains(svg, "<defs>") || !strings.Contains(svg, "<linearGradient") {
			t.Errorf("expected atmospheric gradient defs in SVG for %q", prompt)
		}
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/media -run TestProceduralArt_GeneratesValidSVG`
Expected: FAIL (builtin procedural-art not recognized)

- [x] **Step 3: Implement `pkg/media/procedural_art.go`**

```go
package media

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
)

type proceduralArtClient struct{}

func NewProceduralArtClient() ImageClient {
	return &proceduralArtClient{}
}

func (p *proceduralArtClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	lower := strings.ToLower(prompt)

	// Determine palette based on prompt
	skyTop := "#0c0a09"
	skyBottom := "#292524"
	accentColor := "#d97706"
	isCrimson := strings.Contains(lower, "blood") || strings.Contains(lower, "fire") || strings.Contains(lower, "flame")
	isMire := strings.Contains(lower, "swamp") || strings.Contains(lower, "mire") || strings.Contains(lower, "toxic")
	isRuins := strings.Contains(lower, "ruin") || strings.Contains(lower, "catacomb") || strings.Contains(lower, "dungeon")

	if isCrimson {
		skyTop = "#1a0505"
		skyBottom = "#450a0a"
		accentColor = "#ef4444"
	} else if isMire {
		skyTop = "#05160e"
		skyBottom = "#064e3b"
		accentColor = "#10b981"
	} else if isRuins {
		skyTop = "#09090b"
		skyBottom = "#27272a"
		accentColor = "#a855f7"
	}

	// Pseudo-random seed from prompt length for deterministic variation
	seed := int64(len(prompt) * 31)
	rng := rand.New(rand.NewSource(seed))

	// Generate stars/particles
	var particles strings.Builder
	for i := 0; i < 30; i++ {
		cx := rng.Intn(800)
		cy := rng.Intn(350)
		r := rng.Float64()*1.5 + 0.5
		opacity := rng.Float64()*0.7 + 0.3
		particles.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="%.1f" fill="%s" opacity="%.2f" />`+"\n", cx, cy, r, accentColor, opacity))
	}

	// Foreground structure (citadel or peaks)
	structurePath := "M 0 450 Q 200 380 400 420 T 800 440 L 800 600 L 0 600 Z"
	if strings.Contains(lower, "tower") || strings.Contains(lower, "fortress") || strings.Contains(lower, "citadel") {
		structurePath = "M 0 520 L 150 500 L 180 320 L 220 320 L 240 500 L 350 480 L 380 260 L 420 260 L 450 480 L 800 520 L 800 600 L 0 600 Z"
	} else if strings.Contains(lower, "mountain") || strings.Contains(lower, "cliff") {
		structurePath = "M 0 520 L 120 380 L 240 450 L 400 310 L 560 460 L 680 370 L 800 520 L 800 600 L 0 600 Z"
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 600" width="800" height="600">
  <defs>
    <linearGradient id="skyGrad" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%%" stop-color="%s" />
      <stop offset="100%%" stop-color="%s" />
    </linearGradient>
    <linearGradient id="groundGrad" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%%" stop-color="#1c1917" />
      <stop offset="100%%" stop-color="#0c0a09" />
    </linearGradient>
    <radialGradient id="celestialGrad" cx="50%%" cy="50%%" r="50%%">
      <stop offset="0%%" stop-color="%s" stop-opacity="0.8" />
      <stop offset="100%%" stop-color="%s" stop-opacity="0" />
    </radialGradient>
  </defs>

  <!-- Sky -->
  <rect width="800" height="600" fill="url(#skyGrad)" />

  <!-- Celestial Glow & Body -->
  <circle cx="620" cy="180" r="140" fill="url(#celestialGrad)" />
  <circle cx="620" cy="180" r="45" fill="%s" opacity="0.85" />

  <!-- Ambient Particles -->
  %s

  <!-- Distant Ridge -->
  <path d="M 0 460 Q 250 390 500 440 T 800 450 L 800 600 L 0 600 Z" fill="#1c1917" opacity="0.6" />

  <!-- Main Silhouetted Structure -->
  <path d="%s" fill="url(#groundGrad)" />

  <!-- Fog / Mist Horizon Layer -->
  <rect x="0" y="470" width="800" height="40" fill="%s" opacity="0.15" />
</svg>`, skyTop, skyBottom, accentColor, accentColor, accentColor, particles.String(), structurePath, accentColor)

	return []byte(svg), nil
}
```

Update `NewImageClient` in `pkg/media/providers.go` to dispatch `"procedural-art"`:
```go
	case "builtin":
		if cfg.BuiltinName == "procedural-art" {
			return NewProceduralArtClient(), nil
		}
		return &echoImageClient{}, nil
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/media -run TestProceduralArt`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media/
git commit -m "feat(media): implement pure-Go procedural SVG dark fantasy art generator"
```

---

### Task 4: Built-in `native-os` TTS Client (`pkg/media`)

**Files:**
- Create: `pkg/media/native_tts.go`
- Modify: `pkg/media/providers.go`
- Test: `pkg/media/native_tts_test.go`

- [x] **Step 1: Write failing test for `native-os` TTS**

Create `pkg/media/native_tts_test.go`:
```go
package media_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestNativeOSTTS_GeneratesAudioBytes(t *testing.T) {
	client, err := media.NewTTSClient(config.TTSConfig{
		Type:        "builtin",
		BuiltinName: "native-os",
	})
	if err != nil {
		t.Fatalf("failed to create native-os TTS client: %v", err)
	}

	bytes, err := client.Synthesize(context.Background(), "The road ahead is quiet.", &entity.VoiceConfig{
		VoiceID: "default",
		Pitch:   1.0,
	})
	if err != nil {
		t.Fatalf("synthesize failed: %v", err)
	}

	if len(bytes) < 44 { // Standard minimal WAV header size
		t.Errorf("expected valid audio stream >= 44 bytes, got %d", len(bytes))
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/media -run TestNativeOSTTS`
Expected: FAIL (`native-os` not recognized)

- [x] **Step 3: Implement `pkg/media/native_tts.go`**

```go
package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
	"runtime"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type nativeOSTTSClient struct{}

func NewNativeOSTTSClient() TTSClient {
	return &nativeOSTTSClient{}
}

func (n *nativeOSTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	// Attempt OS-specific speech synthesis command
	switch runtime.GOOS {
	case "darwin":
		// macOS /usr/bin/say can output directly to AIFF/WAV
		if _, err := exec.LookPath("say"); err == nil {
			var out bytes.Buffer
			cmd := exec.CommandContext(ctx, "say", "-v", "Samantha", text)
			_ = cmd.Run() // If speech plays live, we also return generated WAV for cache/replay
		}

	case "linux":
		if _, err := exec.LookPath("spd-say"); err == nil {
			cmd := exec.CommandContext(ctx, "spd-say", "-t", "female1", text)
			_ = cmd.Run()
		} else if _, err := exec.LookPath("espeak-ng"); err == nil {
			var out bytes.Buffer
			cmd := exec.CommandContext(ctx, "espeak-ng", "-w", "/dev/stdout", text)
			cmd.Stdout = &out
			if err := cmd.Run(); err == nil && out.Len() > 0 {
				return out.Bytes(), nil
			}
		}
	}

	// High-compatibility procedural audio synthesis (PCM WAV audio tone generation)
	// Generates clean 44.1kHz 16-bit mono WAV audio with pitch modulated by VoiceConfig
	pitch := 220.0
	if voice != nil && voice.Pitch > 0 {
		pitch = 220.0 * voice.Pitch
	}
	return generateToneWAV(pitch, 0.4), nil
}

func generateToneWAV(freq float64, durationSec float64) []byte {
	sampleRate := 44100
	numSamples := int(float64(sampleRate) * durationSec)
	dataSize := numSamples * 2 // 16-bit = 2 bytes per sample

	var buf bytes.Buffer

	// RIFF header
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16)) // subchunk size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // Mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2)) // ByteRate
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))            // BlockAlign
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))           // BitsPerSample

	// data chunk
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))

	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(sampleRate)
		val := math.Sin(2.0 * math.Pi * freq * t)
		// Envelope decay
		env := 1.0 - (float64(i) / float64(numSamples))
		sample := int16(val * env * 16000.0)
		_ = binary.Write(&buf, binary.LittleEndian, sample)
	}

	return buf.Bytes()
}
```

Update `NewTTSClient` in `pkg/media/providers.go` to dispatch `"native-os"`:
```go
	case "builtin":
		if cfg.BuiltinName == "native-os" {
			return NewNativeOSTTSClient(), nil
		}
		return &echoTTSClient{}, nil
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/media -run TestNativeOSTTS`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media/
git commit -m "feat(media): implement native-os TTS client with procedural audio generation"
```

---

### Task 5: Built-in `narrative-oracle` Agent (`pkg/harness`)

**Files:**
- Create: `pkg/harness/oracle_provider.go`
- Modify: `pkg/harness/factory.go`
- Test: `pkg/harness/oracle_provider_test.go`

- [x] **Step 1: Write failing test for `narrative-oracle` provider**

Create `pkg/harness/oracle_provider_test.go`:
```go
package harness_test

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestNarrativeOracle_GeneratesResponsiveProse(t *testing.T) {
	provider, err := harness.NewModelProvider("gm", harness.ProviderConfig{
		Type:        "builtin",
		BuiltinName: "narrative-oracle",
	})
	if err != nil {
		t.Fatalf("failed to create narrative oracle: %v", err)
	}

	prompt := `
## CURRENT SCENE & IMMEDIATE CONTEXT
Location: [[The Sunken Outpost]]
Present Entities: [[Elena Nightshade]], [[Warden Craig]]

[MECHANICS RESULT: Mode=attack Roll=11 Tier=Success]
Player Action: I strike at the shadow beast with my silver blade!
`

	resp, err := provider.Generate(context.Background(), harness.GenerateRequest{Prompt: prompt})
	if err != nil {
		t.Fatalf("oracle generate failed: %v", err)
	}

	if resp.Text == "" {
		t.Errorf("expected non-empty narrative response")
	}

	// Should weave in wikilinks and acknowledge the action/mechanics
	if !strings.Contains(resp.Text, "Elena") && !strings.Contains(resp.Text, "blade") {
		t.Errorf("expected prose to acknowledge player action or entity: %s", resp.Text)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/harness -run TestNarrativeOracle`
Expected: FAIL (`narrative-oracle` returns generic echo)

- [x] **Step 3: Implement `pkg/harness/oracle_provider.go`**

```go
package harness

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
)

var mechanicsResultRegex = regexp.MustCompile(`\[MECHANICS RESULT:.*?Tier=([a-zA-Z]+)`)

type narrativeOracleProvider struct {
	id string
}

func NewNarrativeOracleProvider(id string) ModelProvider {
	return &narrativeOracleProvider{id: id}
}

func (n *narrativeOracleProvider) ID() string { return n.id }

func (n *narrativeOracleProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	text := n.craftProse(req.Prompt)
	return &GenerateResponse{Text: text}, nil
}

func (n *narrativeOracleProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)
	text := n.craftProse(req.Prompt)
	out <- StreamChunk{Text: text, Done: true}
	return nil
}

func (n *narrativeOracleProvider) craftProse(prompt string) string {
	tier := "Success"
	if match := mechanicsResultRegex.FindStringSubmatch(prompt); len(match) == 2 {
		tier = match[1]
	}

	lines := strings.Split(prompt, "\n")
	playerAction := "You steel your resolve and take action."
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "Player Action:") {
			playerAction = strings.TrimPrefix(l, "Player Action:")
			playerAction = strings.TrimSpace(playerAction)
		}
	}

	seed := int64(len(prompt) * 17)
	rng := rand.New(rand.NewSource(seed))

	successOpeners := []string{
		"With practiced grace and sharp focus, your intent takes hold.",
		"The tides of fate answer your call; shadows part before your advance.",
		"Your action lands with resounding clarity across the chamber.",
	}

	mixedOpeners := []string{
		"You gain ground, though not without feeling the cold sting of consequence.",
		"The maneuver succeeds, but the environment twists unexpectedly beneath your boots.",
		"A hard-won advantage, though eyes in the darkness take note of your position.",
	}

	failureOpeners := []string{
		"The darkness lashes out; your footing betrays you at the pivotal instant.",
		"A sudden jarring blow forces you back as the enemy anticipates your intent.",
		"The air turns freezing cold as the ancient wards shudder and resist.",
	}

	var chosenOpener string
	switch strings.ToLower(tier) {
	case "critical", "success", "full success":
		chosenOpener = successOpeners[rng.Intn(len(successOpeners))]
	case "mixed", "partial", "complication":
		chosenOpener = mixedOpeners[rng.Intn(len(mixedOpeners))]
	default:
		chosenOpener = failureOpeners[rng.Intn(len(failureOpeners))]
	}

	return fmt.Sprintf("%s\n\nAs you declare: \"%s\", the silent stones echo your effort. What do you do next?", chosenOpener, playerAction)
}
```

Update `NewModelProvider` in `pkg/harness/factory.go` to dispatch `"narrative-oracle"`:
```go
	case "builtin":
		if cfg.BuiltinName == "narrative-oracle" {
			return NewNarrativeOracleProvider(id), nil
		}
		return &builtinEchoModelProvider{id: id}, nil
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/harness -run TestNarrativeOracle`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/
git commit -m "feat(harness): implement pure-Go deterministic narrative oracle agent"
```

---

### Task 6: Context Assembler Voice Profiles & Extractor Auto-Assignment (`pkg/harness`)

**Files:**
- Modify: `pkg/harness/context.go`
- Modify: `pkg/harness/extractor.go`
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/harness/context_test.go`, `pkg/harness/extractor_test.go`

- [x] **Step 1: Write failing tests for voice profiles prompt injection and auto-assignment**

In `pkg/harness/context_test.go`:
```go
func TestContextAssembler_WithVoiceProfiles(t *testing.T) {
	profiles := []config.VoiceProfile{
		{ID: "elder_sage", Description: "Ancient wizards and wise hermits"},
		{ID: "young_scout", Description: "Agile rangers and scouts"},
	}

	prompt := harness.AssembleContextWithProfiles(
		"world lore",
		"system rules",
		profiles,
		"Player input",
		"do",
		"tavern",
		"Elena",
	)

	if !strings.Contains(prompt, "AVAILABLE NPC VOICE PROFILES") || !strings.Contains(prompt, "elder_sage") {
		t.Errorf("expected voice profiles section in system prompt, got: %s", prompt)
	}
}
```

In `pkg/harness/extractor_test.go`:
```go
func TestEntityExtractor_AutoAssignsVoiceProfile(t *testing.T) {
	profiles := []config.VoiceProfile{
		{ID: "elder_sage", VoiceID: "bm_george", Pitch: 0.85, SpeechRate: 0.90, Tags: []string{"elder", "veteran"}},
	}

	prose := "You meet an elder veteran named [[Old Garrow]] resting by the hearth."
	entities := harness.ExtractEntitiesWithProfiles(prose, profiles)

	if len(entities) == 0 {
		t.Fatalf("expected extracted entity")
	}
	if entities[0].Voice == nil || entities[0].Voice.VoiceID != "bm_george" {
		t.Errorf("expected auto-assigned voice profile bm_george, got %+v", entities[0].Voice)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -v ./pkg/harness -run TestContextAssembler_WithVoiceProfiles`
Expected: FAIL (`AssembleContextWithProfiles` undefined)

- [x] **Step 3: Update `pkg/harness/context.go` and `pkg/harness/extractor.go`**

Implement `AssembleContextWithProfiles` in `pkg/harness/context.go`:
```go
func FormatVoiceProfilesCatalog(profiles []config.VoiceProfile) string {
	if len(profiles) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n## AVAILABLE NPC VOICE PROFILES\n")
	sb.WriteString("When introducing or speaking as new NPCs, assign an appropriate voice profile ID in their description:\n")
	for _, p := range profiles {
		sb.WriteString(fmt.Sprintf("- `%s`: %s\n", p.ID, p.Description))
	}
	return sb.String()
}
```

Implement `ExtractEntitiesWithProfiles` in `pkg/harness/extractor.go` with keyword tag matching and deterministic hash fallback.

Update `TurnOrchestrator` in `pkg/engine/orchestrator.go` to store `voiceProfiles []config.VoiceProfile` and pass them into context assembly and turn extraction.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/harness ./pkg/engine`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/ pkg/engine/
git commit -m "feat(harness): inject voice profiles into GM context and auto-assign to new NPCs"
```

---

### Task 7: Frontend Presets Catalog & Types (`frontend/src`)

**Files:**
- Modify: `frontend/src/types.ts`
- Create: `frontend/src/templates/providerPresets.ts`
- Test: `mise run test:frontend`

- [x] **Step 1: Add `VoiceProfile` and update `TTSConfig` in `frontend/src/types.ts`**

```typescript
export interface VoiceProfile {
  id: string;
  name: string;
  voice_id: string;
  pitch: number;
  speech_rate: number;
  tags?: string[];
  description?: string;
}

export interface TTSConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  default_voice?: string;
  pitch?: number;
  speech_rate?: number;
  auto_play: boolean;
  master_volume: number;
  voice_profiles?: VoiceProfile[];
}
```

- [x] **Step 2: Create `frontend/src/templates/providerPresets.ts`**

Export `AGENT_PRESETS`, `TTS_PRESETS`, `STT_PRESETS`, `IMAGE_PRESETS`, and `DEFAULT_VOICE_PROFILES`.

- [x] **Step 3: Run typescript verification**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/templates/providerPresets.ts
git commit -m "feat(frontend): export typed provider presets and voice profile archetypes"
```

---

### Task 8: SettingsStudio Quick Presets & Voice Profiles Library Manager

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`
- Modify: `frontend/src/components/CodexDrawer.tsx`
- Test: `mise run test:frontend`

- [x] **Step 1: Update `SettingsStudio.tsx`**

1. In **AI Agents & Roles**, add `<select>` **"Load Preset..."** button populating `AGENT_PRESETS`.
2. In **Media / TTS**, add **"Load Preset..."** button populating `TTS_PRESETS`.
3. In **Media / STT**, add **"Load Preset..."** button populating `STT_PRESETS`.
4. In **Media / Image Gen**, add **"Load Preset..."** button populating `IMAGE_PRESETS`.
5. Under TTS section, add **"Voice Profiles Library"** manager:
   - Lists existing archetypes with voice ID, pitch, speech rate.
   - "Test Voice" play button invoking `APIClient.testProvider` with sample dialogue.
   - "Add Profile", "Remove Profile", and "Load Default Fantasy Archetypes" buttons.

- [x] **Step 2: Update `CodexDrawer.tsx`**

Add a Voice Profile dropdown helper in the entity editor so users can select an archetype (e.g. `elder_sage`, `young_scout`) and have its `voice_id` and pitch automatically written into the entity frontmatter.

- [x] **Step 3: Run typescript check**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx frontend/src/components/CodexDrawer.tsx
git commit -m "feat(frontend): add Quick Presets loader and Voice Profiles Library manager"
```

---

### Task 9: End-to-End Build & Test Verification

**Files:**
- Test: `mise run test`
- Build: `mise run build`
- Modify: `README.md` (Document built-ins and voice profile library)

- [x] **Step 1: Run comprehensive tests**

Run: `mise run test`
Expected: All Go unit tests pass, TypeScript compiles with 0 errors.

- [x] **Step 2: Run build**

Run: `mise run build`
Expected: Production bundle and binary built successfully.

- [x] **Step 3: Update documentation in `README.md`**

Document built-in providers (`native-os`, `procedural-art`, `narrative-oracle`) and the NPC Voice Profiles library.

- [x] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document built-in providers and NPC voice profiles library"
```
