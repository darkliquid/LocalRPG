---
id: 05-providers
title: AI & Media Providers
category: Configuration & Providers
order: 5
description: Configuring LLMs, voice synthesis, speech recognition, image generation, and spend tracking.
---

# AI & Media Providers

LocalRPG uses a uniform provider configuration architecture for all AI models, voice synthesizers, speech recognition, and image generators.

## Provider Architecture

Every provider in `config.yaml` follows the same schema:

```yaml
providers:
  my-provider-id:
    type: http # builtin | cli | http | mock | disabled
    builtin_name: "" # Used when type is builtin
    base_url: "https://api.openai.com/v1"
    api_key: "env:OPENAI_API_KEY"
    model: "gpt-4o"
```

### Supported Provider Types

- **`builtin`**: Runs directly in the LocalRPG process without external dependencies or GPU requirements.
- **`http`**: Connects via HTTP/REST to local daemons (Ollama, LM Studio, Sherpa, ComfyUI) or cloud APIs (OpenAI, Gemini, Anthropic, ElevenLabs).
- **`cli`**: Spawns command-line binaries (e.g. `whisper.cpp`, `spd-say`, custom scripts).
- **`mock`**: Returns deterministic placeholder responses for offline testing and development.
- **`disabled`**: Explicitly disables the capability.

## LLM Providers

### Ollama (Local)

Run models locally with zero external network access:

```yaml
providers:
  local-llama:
    type: http
    base_url: "http://localhost:11434"
    model: "llama3.1:8b"
```

### Gemini API (Cloud)

```yaml
providers:
  gemini-flash:
    type: http
    base_url: "https://generativelanguage.googleapis.com"
    api_key: "env:GEMINI_API_KEY"
    model: "gemini-2.0-flash"
```

### Narrative Oracle (Built-in)

Zero-setup built-in fallback model that generates narrative choices using procedural oracle tables.

## Media Providers

### Voice (TTS)

- **ElevenLabs**: High-fidelity AI speech (`type: http`, `base_url: https://api.elevenlabs.io`).
- **Gemini Voice**: Multimodal speech synthesis.
- **Fish Audio S2 (Local vLLM-Omni)**: 4B Dual-AR multilingual neural voice synthesis at 44.1 kHz with fine-grained emotional tags (e.g. `[whisper]`, `[excited]`, `[angry]`) and zero-shot voice cloning from reference audio.
- **Native OS (`builtin_name: native-os`)**: Built-in speech using your operating system's native synthesizer (`say` on macOS, `spd-say` on Linux, PowerShell SAPI on Windows).
- **Sherpa / Piper**: High quality local neural speech synthesis.

#### Setting Up Fish Audio S2 with vLLM-Omni (Docker & Local)

Fish Audio S2 Pro is an expressive, multilingual text-to-speech model that runs entirely offline. When paired with **vLLM-Omni**, it exposes an OpenAI-compatible speech endpoint on port 8091 with Triton decode acceleration.

##### 1. Hardware Requirements

- **VRAM**:
  - **FP16 (Full Precision)**: ~17–24 GB VRAM (RTX 3090, RTX 4090, A800).
  - **Quantized (FP8 or INT4)**: ~8–12 GB VRAM (RTX 3060, RTX 4070).
- **CUDA**: 12.1+ compatible NVIDIA GPU driver.

##### 2. Running via Docker (Recommended)

Running with Docker isolates all CUDA, Triton, and audio codec dependencies:

```bash
docker run --gpus all \
  -p 8091:8091 \
  --ipc=host \
  -v ~/.cache/huggingface:/root/.cache/huggingface \
  vllm/vllm-omni:latest \
  vllm serve fishaudio/s2-pro --omni --port 8091
```

Or using `docker-compose.yml`:

```yaml
services:
  fish-speech:
    image: vllm/vllm-omni:latest
    ports:
      - "8091:8091"
    ipc: host
    volumes:
      - ~/.cache/huggingface:/root/.cache/huggingface
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    command: vllm serve fishaudio/s2-pro --omni --port 8091
```

> [!NOTE]
> If your GPU has 12–16 GB of VRAM, add `--gpu-memory-utilization 0.9` or use an FP8/quantized variant of the checkpoint to prevent out-of-memory errors during context caching.

##### 3. Running via Python (pip)

If running directly on the host with Python 3.10+:

```bash
pip install vllm-omni fish-speech
vllm serve fishaudio/s2-pro --omni --port 8091
```

##### 4. Verify Server Health

Test that the endpoint is synthesizing audio properly before configuring LocalRPG:

```bash
curl -X POST http://localhost:8091/v1/audio/speech \
  -H "Content-Type: application/json" \
  -d '{
    "model": "fishaudio/s2-pro",
    "input": "[excited] Greetings, adventurer! The local Fish Audio voice server is ready.",
    "voice": "default",
    "response_format": "wav"
  }' --output test.wav
```

##### 5. Connecting in LocalRPG

1. Open **Settings Studio** -> **Media** -> **TTS**.
2. Click **Load TTS Preset...** and choose **Fish Audio S2 Pro (Local vLLM-Omni)** (or select **Fish Audio S2 (Local vLLM-Omni)** in the **TTS Engine** dropdown).
3. The endpoint defaults to `http://localhost:8091` and model to `fishaudio/s2-pro`.
4. **Vocal Performance Steering**: Bracketed delivery tags (`[whisper]`, `[excited]`, `[angry]`, `[sad]`, `[laugh]`, `[sigh]`, `[gasp]`, `[cough]`, `[cry]`, `[screaming]`, `[shouting]`) are automatically understood and preserved in spoken narration.
5. **Zero-Shot Voice Cloning**: Under **Voice Profiles**, configure `ref_audio` (e.g. `assets/voices/guard.wav` or a file path) and `ref_text` (transcript of the clip). LocalRPG automatically reads and converts the clip to a data URI for the server.

### Image Generation

- **Procedural Art (`builtin_name: procedural-art`)**: Pure-Go SVG generator creating heraldic banners, landscape silhouettes, and item icons without a GPU.
- **ComfyUI / Automatic1111**: Local Stable Diffusion web APIs.
- **Google Imagen**: Cloud image synthesis.

## Spend Ledger & Rate Limit Handling

LocalRPG protects you from runaway API costs and service interruptions:

- **Rate Limits (HTTP 429)**: The engine automatically applies exponential backoff with jitter.
- **Insufficient Funds (HTTP 402)**: Instantly pauses background generation and displays a warning chip in the interface.
- **Spend Ledger**: Tracks exact token usage, estimated costs, and requests per model, viewable in Global Settings.

See [Usage, Cost & Pricing](11-usage-and-pricing) for the ledger keys and price
fields, the [Provider & Model Catalogue](12-provider-catalogue) for the full
list of provider IDs, presets, and models, and the
[Configuration Reference](13-configuration-reference) for every config key.
