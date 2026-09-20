# Built-in Engines, Preset Providers, and Per-Character Customization Specification

- **Date:** 2026-09-20
- **Status:** Approved
- **Scope:** Pure-Go built-in providers (native-os TTS, procedural-art SVG image generator, narrative-oracle storyteller), quick-load preset catalog for local tools, and per-character voice/option overrides.

---

## 1. Overview & Goals

LocalRPG requires first-class support for running completely offline and locally without mandatory external cloud services or complex initial setup.

Key capabilities:
1. **Zero-Dependency Built-In Engines**:
   - **`native-os` TTS**: Uses the host OS speech synthesizer directly from Go (`spd-say` on Linux, `/usr/bin/say` on macOS, PowerShell on Windows) with procedural audio fallback.
   - **`procedural-art` Image Gen**: Pure-Go multi-layered SVG scene and sigil generator based on scene prompt keywords and world genre/tone.
   - **`narrative-oracle` AI Agent**: Pure-Go table-and-rule-driven procedural story orchestrator for offline gameplay and rule verification without an LLM server.
2. **Local Tool Preset Catalog**:
   - One-click presets for popular local servers and CLI binaries: Ollama, LM Studio, LocalAI, vLLM, Kokoro-FastAPI, AllTalk, Piper TTS, Whisper.cpp, ComfyUI, Automatic1111, and sd-cli.
3. **Per-Character Customization**:
   - Entity frontmatter `voice:` overrides (voice ID e.g. `af_bella`, pitch, speed rate) with global fallback hierarchy.
   - Voice preview helper in Codex and Worlds Studio.

---

## 2. Built-in Engines Architecture

### 2.1 Native OS Voice Synthesizer (`native-os`)
- **Package**: `pkg/media/native_tts.go`
- **Behavior**:
  - Auto-detects `runtime.GOOS`:
    - **Linux**: Executes `spd-say` or `espeak-ng` if installed; falls back to generating a synthesized WAV waveform tone.
    - **Darwin (macOS)**: Executes `/usr/bin/say` with `-o <temp.wav> --data-format=LEF32@22050` or pipe to generate audio bytes.
    - **Windows**: Executes PowerShell script invoking `System.Speech.Synthesis.SpeechSynthesizer`.
  - Implements `media.TTSClient` returning audio bytes.
  - Caches audio in `ContentCache` using `ComputeAudioCacheKey`.

### 2.2 Procedural Fantasy SVG Generator (`procedural-art`)
- **Package**: `pkg/media/procedural_art.go`
- **Behavior**:
  - Implements `media.ImageClient`.
  - Keyword parser extracts themes: terrain (`mountains`, `dungeon`, `ruins`, `forest`, `swamp`), lighting (`night`, `moon`, `dusk`, `dawn`), and mood (`fog`, `ember`, `void`).
  - Emits self-contained SVG (`<svg viewBox="0 0 800 600" ...>`) containing:
    - Radial / linear sky gradients.
    - Silhouette polygon paths for mountains, ruins, and trees.
    - Moon, sun, or mystic sigil overlays.
    - Atmospheric particle dust / stars.
  - Generates SVG bytes and returns clean asset URLs (`.svg`), natively rendered by both WebKit WebView and standard browsers.

### 2.3 Deterministic Narrative Oracle (`narrative-oracle`)
- **Package**: `pkg/harness/oracle_provider.go`
- **Behavior**:
  - Implements `harness.ModelProvider`.
  - Parses player action mode (`do`, `attack`, `say`, `roll`) and mechanics tag `[MECHANICS RESULT: ...]`.
  - Maps results to 4 outcome tiers:
    - *Full Success with Momentum*: Heroic accomplishment, bonus narrative advantage.
    - *Success*: Direct achievement of intent.
    - *Mixed Success*: Progress achieved with complication or cost.
    - *Failure*: Threat escalation, environmental shift, or adversary reaction.
  - Weaves in contextual entities from the turn prompt (locations, NPCs) with bracketed wikilinks `[[...]]`.
  - Emits streaming or non-streaming responses.

---

## 3. Preset Catalog Specification

### 3.1 Presets Data Structure (`frontend/src/templates/providerPresets.ts` and `pkg/config/presets.go`)

- **LLM / Agents**:
  - `ollama`: `http://localhost:11434/v1`, model: `llama3.2`, temp: 0.7
  - `lm-studio`: `http://localhost:1234/v1`, model: `default`
  - `localai`: `http://localhost:8080/v1`, model: `gpt-4`
  - `vllm`: `http://localhost:8000/v1`
  - `llama-cli`: command: `llama-cli`, args: `["-m", "models/model.gguf", "-p"]`
  - `claude-cli`: command: `claude`, args: `["-p"]`
  - `narrative-oracle`: type: `builtin`, builtin_name: `narrative-oracle`

- **Text-to-Speech (TTS)**:
  - `kokoro-fastapi`: `http://localhost:8880/v1/audio/speech`, model: `kokoro`, default_voice: `af_bella`
  - `alltalk`: `http://localhost:7851/api/tts-generate`
  - `piper`: command: `piper`, args: `["--model", "en_US-lessac-medium.onnx", "--output_file", "-"]`
  - `native-os`: type: `builtin`, builtin_name: `native-os`
  - `openai-speech`: `https://api.openai.com/v1/audio/speech`, model: `tts-1`, default_voice: `alloy`

- **Speech-to-Text (STT)**:
  - `faster-whisper`: `http://localhost:8000/v1/audio/transcriptions`, model: `whisper-1`
  - `whisper-cli`: command: `whisper-cli`, args: `["-m", "models/ggml-base.bin", "-f", "%INPUT%", "-nt"]`
  - `openai-whisper`: `https://api.openai.com/v1/audio/transcriptions`, model: `whisper-1`

- **Image Generation**:
  - `comfyui`: `http://127.0.0.1:8188`
  - `automatic1111`: `http://127.0.0.1:7860/sdapi/v1/txt2img`
  - `localai-image`: `http://127.0.0.1:8080/v1/images/generations`, model: `stablediffusion`
  - `sd-cli`: command: `sd`, args: `["-m", "models/sd-v1-5.gguf", "-p"]`
  - `procedural-art`: type: `builtin`, builtin_name: `procedural-art`
  - `dall-e-3`: `https://api.openai.com/v1/images/generations`, model: `dall-e-3`

---

## 4. Per-Character Voice & Option Customization

### 4.1 Schema Expansion (`pkg/entity/entity.go`)
```go
type VoiceConfig struct {
    Provider   string  `yaml:"provider,omitempty" json:"provider,omitempty"`
    VoiceID    string  `yaml:"voice_id,omitempty" json:"voice_id,omitempty"`
    Pitch      float64 `yaml:"pitch,omitempty" json:"pitch,omitempty"`
    SpeechRate float64 `yaml:"speech_rate,omitempty" json:"speech_rate,omitempty"`
}
```

### 4.2 Resolution Logic
1. Speaker entity `voice_id` takes precedence over global settings `default_voice`.
2. Speaker `pitch` and `speech_rate` multiply global baseline settings.
3. Audio cache key incorporates speaker ID, resolved voice ID, pitch, speech rate, and text content.
4. HTTP TTS client sends the character's voice in the JSON request body `{"model": "kokoro", "voice": "af_bella", "input": "..."}`.

---

## 5. Testing & Verification

1. Unit tests for `procedural-art` SVG output (valid SVG tags, proper XML escaping).
2. Unit tests for `narrative-oracle` response formatting and mechanics result handling.
3. Unit tests for `native-os` TTS execution and fallback.
4. Unit tests for per-character voice resolution in `TTSPipeline`.
5. Frontend TypeScript verification of preset catalog and voice selection helpers.
