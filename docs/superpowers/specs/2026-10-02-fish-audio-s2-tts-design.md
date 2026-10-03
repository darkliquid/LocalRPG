# Design Spec: Fish Audio S2 TTS Provider

**Date:** 2026-10-02
**Status:** Approved
**Target:** `pkg/provider/ttsfishaudio`, `pkg/provider/keys.go`, `pkg/provider/all`, `pkg/media`, `pkg/gui`, `frontend`
**Related:** `docs/superpowers/specs/2026-09-22-elevenlabs-tts-provider-design.md`, `docs/superpowers/specs/2026-09-23-voice-speech-cues-and-steering-design.md`

---

## 1. Executive Summary

[Fish Audio S2 Pro](https://github.com/fishaudio/fish-speech) is a 4B parameter Dual-Autoregressive (Dual-AR) multilingual text-to-speech model featuring fine-grained, sub-word emotional and prosodic steering tags (e.g. `[whisper]`, `[excited]`, `[angry]`) and zero-shot voice cloning from reference audio clips.

This specification defines the integration of Fish Audio S2 Pro into LocalRPG as a dedicated first-class TTS provider (`tts:fish-audio`). LocalRPG connects via HTTP to a local **vLLM-Omni** serving instance, utilizing its high-throughput, Triton decode-only kvcache attention fast path and OpenAI-compatible `POST /v1/audio/speech` endpoint.

Key capabilities delivered by this provider:
1. **Dedicated Provider Identity & Presets:** Registered as `KeyTTSFishAudio = "tts:fish-audio"`, with a default local vLLM preset on `http://localhost:8091` targeting `fishaudio/s2-pro`.
2. **First-Class Vocal Steering & Speech Cues:** Implements `media.SpeechCueAdvertiser` with `AudioTags: true` and prompt guidance for S2 Pro's bracketed emotional tags, allowing the GM LLM to direct delivery naturally.
3. **Zero-Shot Voice Cloning & Voice Catalog:** Supports standard named voices via `GET /v1/audio/voices` and zero-shot cloning from reference audio files (`ref_audio`) and transcripts (`ref_text`) configured on character voice profiles. Local audio files are encoded as data URIs so that vLLM-Omni can ingest them without shared filesystem constraints.

---

## 2. Research & Upstream Findings

### 2.1 Model Architecture & Audio Output
*   **Dual-Autoregressive Architecture:**
    *   *Slow AR Backbone:* Qwen3-based backbone generating semantic speech tokens.
    *   *Fast AR Decoder:* Autoregressively predicts residual codebook tokens.
    *   *DAC Codec:* Decodes discrete tokens into high-fidelity 44.1 kHz mono audio.
*   **Audio Format:** Emits 44.1 kHz 16-bit PCM / WAV.
*   **Hardware Requirements:**
    *   Full-precision (FP16): ~17–24 GB VRAM (e.g. RTX 3090, RTX 4090, A800).
    *   Quantized (FP8, INT4 GPTQ, or GGUF via `s2.cpp`): 4–12 GB VRAM for consumer GPUs.

### 2.2 vLLM-Omni Serving Interface
The official [vLLM-Omni Fish Speech S2 Pro recipe](https://github.com/vllm-project/vllm-omni/blob/main/recipes/fishaudio/Fish-Speech-S2-Pro.md) documents online serving with:
```bash
vllm serve fishaudio/s2-pro --omni --port 8091
```

*   **Synthesis Endpoint:** `POST /v1/audio/speech`
    *   Standard fields: `input` (text), `model` (checkpoint), `voice` (speaker name), `response_format` (`wav`, `mp3`, `flac`, `pcm`), `speed` (float).
    *   Voice Cloning extension fields:
        *   `ref_audio`: String specifying reference audio via HTTP URL, base64 data URI (`data:audio/wav;base64,...`), or `file://` URI.
        *   `ref_text`: Transcript of the reference audio clip.
*   **Voices Endpoint:** `GET /v1/audio/voices`
    *   Returns JSON listing available preset speakers and uploaded voices.

### 2.3 Fine-Grained Vocal Steering & Emotional Tags
Unlike standard TTS models that require separate audio style conditioning, Fish Audio S2 Pro was trained on text with natural language delivery tags. Tags placed within brackets directly alter the prosody, pitch, and timbre of the spoken words:
*   Supported tags: `[whisper]`, `[excited]`, `[angry]`, `[sad]`, `[laugh]`, `[sigh]`, `[gasp]`, `[cough]`, `[cry]`, `[screaming]`, `[shouting]`.
*   These map directly to LocalRPG's `media.SpeechCueCapabilities` (`AudioTags: true`).

---

## 3. Architecture & Provider Registration

### 3.1 Canonical Provider Key
In `pkg/provider/keys.go`:
```go
const (
    // ...
    KeyTTSFishAudio Key = "tts:fish-audio"
)
```
Added to `AllKeys()`.

### 3.2 Package `pkg/provider/ttsfishaudio`
New package `pkg/provider/ttsfishaudio/ttsfishaudio.go` registering the descriptor:
*   `ID`: `"tts:fish-audio"`
*   `Family`: `provider.FamilyTTS`
*   `Label`: `"Fish Audio S2 (vLLM-Omni)"`
*   `Description`: `"Dual-AR speech synthesis with fine-grained emotional tags and zero-shot voice cloning."`
*   `Source`: `"http"`
*   `Features`: `[]provider.Feature{provider.FeatureVoiceCatalog, provider.FeatureVoiceOptions, provider.FeatureOffline}`
*   `Presets`:
    *   `ID`: `"fish-audio-s2-vllm"`
    *   `Order`: 5
    *   `Label`: `"Fish Audio S2 Pro (Local vLLM-Omni)"`
    *   `Description`: `"Local Fish Audio S2 Pro running via vLLM-Omni on port 8091."`
    *   `Config`:
        ```yaml
        type: "http"
        endpoint: "http://localhost:8091"
        model: "fishaudio/s2-pro"
        default_voice: "default"
        pitch: 1.0
        speech_rate: 1.0
        auto_play: true
        master_volume: 1.0
        ```
*   `Build`: Unmarshals `media.TTSBuildPayload` and invokes `NewFishAudioTTSClient(payload.Config)`.

### 3.3 Import & Resolution
*   `pkg/provider/all/all.go`: Blank-imports `_ "github.com/darkliquid/localrpg/pkg/provider/ttsfishaudio"`.
*   `pkg/media/keys.go`: In `TTSKeyFor(cfg config.TTSConfig)`, detects Fish Audio when `cfg.Type == "fish-audio"` or when `cfg.Type == "http"` and `cfg.Model` contains `"fishaudio"` or `"s2-pro"`, returning `(KeyTTSFishAudio, true)`.

---

## 4. Synthesis & Client Implementation

### 4.1 Client Definition
In `pkg/provider/ttsfishaudio/client.go`:
```go
type FishAudioTTSClient struct {
    endpoint string
    model    string
    apiKey   string
    client   *http.Client
}

func NewFishAudioTTSClient(cfg config.TTSConfig) *FishAudioTTSClient {
    return &FishAudioTTSClient{
        endpoint: cfg.Endpoint,
        model:    cfg.Model,
        apiKey:   cfg.APIKey,
        client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 120 * time.Second},
    }
}
```

### 4.2 Synthesis Flow (`Synthesize`)
1.  **Resolve URL:** Resolves `<endpoint>/v1/audio/speech`.
2.  **Voice Identification:**
    *   Reads `voice.VoiceID`. If empty, defaults to `"default"`.
3.  **Voice Cloning Check:**
    *   Inspects `voice.Options["ref_audio"]` and `voice.Options["ref_text"]`.
    *   If `ref_audio` is specified:
        *   If it is a local file path, reads file contents from disk and base64 encodes it into `data:<mime>;base64,<payload>`.
        *   If it is an HTTP URL or already a `data:` URI, passes it directly.
4.  **Payload Construction:**
    ```json
    {
        "model": "fishaudio/s2-pro",
        "input": "<text_with_speech_cues>",
        "voice": "<voice_id>",
        "response_format": "wav"
    }
    ```
    *   If `voice.SpeechRate > 0 && voice.SpeechRate != 1.0`, includes `"speed": voice.SpeechRate`.
    *   If cloning options are present, includes `"ref_audio"` and `"ref_text"`.
5.  **Execution & Response:**
    *   Sends `POST` request with JSON body.
    *   If HTTP status != 200, reads up to `provider.MaxProviderDetailBytes` and wraps error via `fmt.Errorf("fish-audio tts failed (%d): %s", resp.StatusCode, detail)`.
    *   Returns raw audio byte stream (`audio/wav`), which LocalRPG normalizes to Opus in its playback pipeline.

---

## 5. Speech Cues & Vocal Steering

`FishAudioTTSClient` implements `media.SpeechCueAdvertiser`:
```go
func (c *FishAudioTTSClient) SpeechCueCapabilities() media.SpeechCueCapabilities {
    return media.SpeechCueCapabilities{
        AudioTags:        true,
        MarkdownEmphasis: false,
        SupportedTags: []string{
            "whisper", "excited", "angry", "sad", "laugh",
            "sigh", "gasp", "cough", "cry", "screaming", "shouting",
        },
        PromptGuidance: "Use bracketed emotional cues like [whisper] or [excited] directly before dialogue lines to steer delivery and vocal expression.",
    }
}
```

*   **Integration with Engine:**
    *   `pkg/media/speakable.go`: Because `AudioTags` is `true`, bracketed vocal tags matching supported tags are preserved in dialogue lines sent to the TTS client.
    *   `pkg/harness/context.go`: The system prompt context informs the GM agent of available delivery cues.
    *   `SettingsStudio.tsx`: Displays provider capabilities badge indicating bracketed vocal cue support.

---

## 6. Voice Catalog & Tunables

### 6.1 Voice Catalog (`media.VoiceCatalog`)
`FishAudioTTSClient` implements `media.VoiceCatalog`:
```go
func (c *FishAudioTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error)
```
*   Sends `GET <endpoint>/v1/audio/voices`.
*   Parses returned `{ "voices": [...] }` or string array.
*   If the endpoint returns 404 or an empty list, returns a fallback default voice:
    *   `ID`: `"default"`
    *   `Name`: `"Default Speaker (Fish Audio S2)"`
    *   `Language`: `"Multi"`
    *   `Tags`: `[]string{"dual-ar", "44.1khz", "zero-shot-capable"}`
    *   `Defaults`: `map[string]interface{}{"pitch": 1.0, "speech_rate": 1.0}`

### 6.2 Tunable Options (`media.VoiceOptions`)
`FishAudioTTSClient` implements `media.VoiceOptions`:
```go
func (c *FishAudioTTSClient) VoiceOptions() []media.VoiceOption {
    return []media.VoiceOption{
        {
            Key:         "ref_audio",
            Label:       "Reference Audio (Voice Clone)",
            Kind:        "string",
            Description: "Local file path (e.g. assets/voices/hero.wav), URL, or base64 data URI for zero-shot cloning.",
        },
        {
            Key:         "ref_text",
            Label:       "Reference Audio Transcript",
            Kind:        "string",
            Description: "Exact transcript of the reference audio clip (required by S2 Pro for voice cloning).",
        },
    }
}
```

---

## 7. Frontend Integration

*   **Settings Studio (`frontend/src/components/SettingsStudio.tsx`):**
    *   The TTS engine selector includes `Fish Audio S2 (vLLM-Omni)`.
    *   Preset loader includes `Fish Audio S2 Pro (Local vLLM-Omni)`.
    *   Selecting the preset or engine populates `endpoint: "http://localhost:8091"`, `model: "fishaudio/s2-pro"`, `default_voice: "default"`.
    *   Voice Profiles can configure `ref_audio` and `ref_text` using the dynamic VoiceOptions control.

---

## 8. Local Setup & Serving Runbook

Documented in `pkg/gui/docs/05-providers.md` and embedded documentation:

### Prerequisites
*   NVIDIA GPU with CUDA 12 support (16GB+ VRAM for FP16, or 8GB–12GB with FP8/INT4 quantization).
*   Python 3.10+ or Docker.

### Running with vLLM-Omni (Native Python)
```bash
pip install vllm-omni
vllm serve fishaudio/s2-pro --omni --port 8091
```

### Running with Docker
```bash
docker run --gpus all \
    -p 8091:8091 \
    --ipc=host \
    vllm/vllm-omni:latest \
    vllm serve fishaudio/s2-pro --omni --port 8091
```

### Testing Connection
```bash
curl -X POST http://localhost:8091/v1/audio/speech \
    -H "Content-Type: application/json" \
    -d '{
        "model": "fishaudio/s2-pro",
        "input": "[excited] Hello! Fish Audio S2 Pro is operational.",
        "voice": "default",
        "response_format": "wav"
    }' --output test.wav
```

---

## 9. Verification & Testing Strategy

1.  **Unit Tests (`pkg/provider/ttsfishaudio/client_test.go`):**
    *   `TestFishAudioSynthesis_Standard`: Tests standard synthesis payload, headers, and audio return against an `httptest.Server`.
    *   `TestFishAudioSynthesis_VoiceCloning_LocalFile`: Tests that local audio file references are correctly read and encoded as base64 data URLs in `ref_audio`.
    *   `TestFishAudioSynthesis_VoiceCloning_DirectURL`: Tests that external URLs are passed through verbatim.
    *   `TestFishAudioSpeechCueCapabilities`: Verifies that `AudioTags` is `true` and expected tags are present.
    *   `TestFishAudioVoiceCatalog`: Tests parsing `/v1/audio/voices` and fallback to default speaker when endpoint returns 404.
    *   `TestFishAudioVoiceOptions`: Verifies option schema validation.
2.  **Provider Integration Tests (`pkg/provider/all/`):**
    *   Ensure `provider.List(provider.FamilyTTS)` includes `tts:fish-audio`.
    *   Ensure preset parity tests pass for `fish-audio-s2-vllm`.
3.  **Documentation Tests:**
    *   Run `go test ./pkg/gui -update-docs` to regenerate embedded provider catalogue documentation and verify no lint or schema regressions.
