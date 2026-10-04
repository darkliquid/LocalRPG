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
- **`cli`**: Spawns command-line binaries (such as `whisper.cpp`, `spd-say`, custom scripts).
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

For hardware requirements, container deployment, and configuration walkthroughs, see [Setting Up Ollama for Local LLMs](14-local-llm-ollama).

### Gemini API (GCP)

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
- **Fish Audio S2 (Local vLLM-Omni)**: 4-billion-parameter Dual-AR multilingual voice synthesis at 44.1 kHz with fine-grained emotional tags and zero-shot voice cloning. See [Setting Up Fish Audio S2](16-local-tts-fish-audio).
- **Kokoro-FastAPI**: High-speed local neural voice synthesis on CPU/GPU. See [Setting Up Kokoro-FastAPI](15-local-tts-kokoro).
- **Native OS (`builtin_name: native-os`)**: Built-in speech using your operating system's native synthesizer (`say` on macOS, `spd-say` on Linux, PowerShell SAPI on Windows).
- **Sherpa / Piper**: High quality local neural speech synthesis.

### Speech Recognition (STT)

- **Faster-Whisper**: High-speed local transcription server via CTranslate2. See [Setting Up Faster-Whisper](17-local-stt-whisper).
- **Whisper CLI**: Standalone local transcription via `whisper.cpp`.
- **Web Speech API**: In-browser speech recognition supported on Chromium-based browsers.

### Image Generation

- **Procedural Art (`builtin_name: procedural-art`)**: Pure-Go SVG generator creating heraldic banners, landscape silhouettes, and item icons without a GPU.
- **ComfyUI / Automatic1111**: Local Stable Diffusion web APIs. See [Setting Up ComfyUI](18-local-image-comfyui).
- **Google Imagen**: GCP image synthesis.

## Spend Ledger & Rate Limit Handling

LocalRPG protects you from runaway API costs and service interruptions:

- **Rate Limits (HTTP 429)**: The engine automatically applies exponential backoff with jitter.
- **Insufficient Funds (HTTP 402)**: Instantly pauses background generation and displays a warning chip in the interface.
- **Spend Ledger**: Tracks exact token usage, estimated costs, and requests per model, viewable in Global Settings.

For step-by-step local service installation guides, see:

- [Fully Local AI Stack with Docker Compose](19-local-stack-docker-compose) (all four services in one Compose project)
- [Setting Up Ollama for Local LLMs](14-local-llm-ollama)
- [Setting Up Kokoro-FastAPI for Neural TTS](15-local-tts-kokoro)
- [Setting Up Fish Audio S2 with vLLM-Omni](16-local-tts-fish-audio)
- [Setting Up Faster-Whisper for Speech Recognition](17-local-stt-whisper)
- [Setting Up ComfyUI for Local Image Generation](18-local-image-comfyui)

See [Usage, Cost & Pricing](11-usage-and-pricing) for the ledger keys and price
fields, the [Provider & Model Catalogue](12-provider-catalogue) for the full
list of provider IDs, presets, and models, and the
[Configuration Reference](13-configuration-reference) for every config key.
