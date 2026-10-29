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

### OpenAI

The **`openai`** type is OpenAI's native path: chat runs over the Responses API
with tool calling and reasoning effort, speech comes from a fixed 13-voice
catalogue with delivery `instructions`, and images come from the gpt-image
models with size and quality mapping. The key can be set once for every
capability:

```yaml
providers:
  openai:
    api_key: "env:OPENAI_API_KEY"
agents:
  roles:
    gm:
      type: openai
      model: gpt-6.1-sol
      effort: medium # optional; reasoning models reject temperature when set
media:
  tts:
    type: openai
    default_voice: marin
  image:
    type: openai
    quality: auto # low | medium | high | auto
```

The speech endpoint is deprecated by OpenAI (shutdown announced for
2027-01-06); the provider catalogue marks the caveat. Requests are never stored
on OpenAI's side (`store: false`).

### Anthropic

The **`anthropic`** type is Anthropic's native LLM path over the Messages API.
It supports tool calling, reasoning effort, and prompt caching on by default
(the growing narration context reads at a fraction of base input). Anthropic
doesn't offer image generation, speech, or embeddings APIs, so this type applies to
roles only:

```yaml
providers:
  anthropic:
    api_key: "env:ANTHROPIC_API_KEY"
agents:
  roles:
    gm:
      type: anthropic
      model: claude-sonnet-5-5
      prompt_cache: auto # auto (default) | off
```

The Messages API removed `temperature`, `top_p` and `top_k`; configure behaviour
with `effort` instead, and note that `max_tokens` is always required, so the
role's `max_tokens` or a default of 1024 is used.

### How providers run

LocalRPG labels providers with one of four capability tiers, so an offline
template is not mistaken for a frontier model. A **capability tier** describes how
a provider runs and how good its output can be, not how it is configured. The tier
and its caveat appear beside each provider in the catalogue and the preset picker.

| Tier | Label | What it means |
| --- | --- | --- |
| `offline-basic` | Offline · basic | Deterministic and simple, and it runs entirely on your machine. Its output is more limited and repetitive than a model's. |
| `offline-neural` | Offline · small model | Runs a small model on your CPU, entirely on your machine. Quality is well below a large local or cloud model. |
| `local-server` | Local server | Needs a server you run yourself. Local, but only offline while that server is. |
| `cloud` | Cloud | Sends your text to a remote provider and needs an API key. Metered in most cases. |

The tier describes the architecture. A `cli` provider is `local-server` because
the binary it runs may reach the network. The
[Provider & Model Catalogue](12-provider-catalogue.md) lists the tier of every
provider.

### Offline Preset and Verification

To configure a fully offline stack in one step, choose the **Offline** sub-tab under Providers in Settings Studio, then **Offline preset**, or run `localrpg config offline-preset`. This switches the GM role to the Narrative Oracle, speech synthesis to Native OS, image generation to Procedural Art, and vector search to built-in projections.

To verify that all active providers operate offline, choose **Check offline** in Settings Studio or run `localrpg config check-offline`.

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

For hardware requirements, container deployment, and configuration walkthroughs, see [Setting Up Ollama for Local LLMs](14-local-llm-ollama.md).

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

Zero-setup built-in offline storyteller. It reads character stats, outcome tiers, stakes, entities, and location to compose responsive prose deterministically without requiring a model, server, or network connection.

## Media Providers

### Voice (TTS)

- **ElevenLabs**: High-fidelity AI speech (`type: http`, `base_url: https://api.elevenlabs.io`).
- **Gemini Voice**: Multimodal speech synthesis.
- **Fish Audio S2 (Local vLLM-Omni)**: 4-billion-parameter Dual-AR multilingual voice synthesis at 44.1 kHz with fine-grained emotional tags and zero-shot voice cloning. See [Setting Up Fish Audio S2](16-local-tts-fish-audio.md).
- **Kokoro-FastAPI**: High-speed local neural voice synthesis on CPU/GPU. See [Setting Up Kokoro-FastAPI](15-local-tts-kokoro.md).
- **Native OS (`builtin_name: native-os`)**: Built-in speech using your operating system's native synthesizer (`say` on macOS, `spd-say` on Linux, PowerShell SAPI on Windows).
- **Sherpa / Piper**: High quality local neural speech synthesis.

### Speech Recognition (STT)

- **Faster-Whisper**: High-speed local transcription server via CTranslate2. See [Setting Up Faster-Whisper](17-local-stt-whisper.md).
- **Whisper CLI**: Standalone local transcription via `whisper.cpp`.
- **Web Speech API**: In-browser speech recognition supported on Chromium-based browsers.

### Image Generation

- **Procedural Art (`builtin_name: procedural-art`)**: Pure-Go SVG generator creating heraldic banners, landscape silhouettes, and item icons without a GPU.
- **ComfyUI / Automatic1111**: Local Stable Diffusion web APIs. See [Setting Up ComfyUI](18-local-image-comfyui.md).
- **Google Imagen**: GCP image synthesis.

### Purpose Chains

A media purpose can declare an ordered chain, mirroring a role's. Each member
names an entry in the matching family map (`tts_providers` or `image_providers`)
or the reserved `default`:

```yaml
media:
  purposes:
    scene: hero
  purpose_chains:
    scene:
      chain: [hero, default]
      select: first
```

The registry uses the first member that builds, so a premium provider can lead a
purpose while a local one stands by. The rules that order the chain are the same
four a role chain uses.

## Spend Ledger & Rate Limit Handling

LocalRPG protects you from runaway API costs and service interruptions:

- **Rate Limits (HTTP 429)**: The engine automatically applies exponential backoff with jitter.
- **Insufficient Funds (HTTP 402)**: Instantly pauses background generation and displays a warning chip in the interface.
- **Spend Ledger**: Tracks exact token usage, estimated costs, and requests per model, viewable in Global Settings.

For step-by-step local service installation guides, see:

- [Fully Local AI Stack with Docker Compose](19-local-stack-docker-compose.md) (all four services in one Compose project)
- [Setting Up Ollama for Local LLMs](14-local-llm-ollama.md)
- [Setting Up Kokoro-FastAPI for Neural TTS](15-local-tts-kokoro.md)
- [Setting Up Fish Audio S2 with vLLM-Omni](16-local-tts-fish-audio.md)
- [Setting Up Faster-Whisper for Speech Recognition](17-local-stt-whisper.md)
- [Setting Up ComfyUI for Local Image Generation](18-local-image-comfyui.md)

See [Usage, Cost & Pricing](11-usage-and-pricing.md) for the ledger keys and price
fields, the [Provider & Model Catalogue](12-provider-catalogue.md) for the full
list of provider IDs, presets, and models, and the
[Configuration Reference](13-configuration-reference.md) for every config key.
