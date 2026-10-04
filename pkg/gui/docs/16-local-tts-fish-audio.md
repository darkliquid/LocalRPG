---
id: 16-local-tts-fish-audio
title: Setting Up Fish Audio S2 with vLLM-Omni
category: Local AI & Self-Hosting
order: 16
description: GPU requirements, Docker/vLLM-Omni container setup, voice cloning, and emotion tag performance steering.
---

# Setting Up Fish Audio S2 with vLLM-Omni

Fish Audio S2 Pro is an expressive, multilingual 4-billion-parameter Dual-AR neural voice model that runs entirely offline. When paired with **vLLM-Omni**, it exposes an OpenAI-compatible speech endpoint on port `8091` with Triton decode acceleration, sub-second latency, and support for expressive emotion tags and zero-shot voice cloning.

## 1. Hardware Requirements

- **VRAM**:
  - **FP16 (Full Precision)**: ~17–24 GB VRAM (NVIDIA RTX 3090, RTX 4090, A800).
  - **Quantized (FP8 or INT4)**: ~8–12 GB VRAM (NVIDIA RTX 3060, RTX 4070).
- **CUDA**: 12.1+ compatible NVIDIA GPU driver.

## 2. Running via Docker (Recommended)

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
    container_name: fish-audio-s2
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

## 3. Running via Python (Host pip)

If running directly on the host with Python 3.10+:

```bash
pip install vllm-omni fish-speech
vllm serve fishaudio/s2-pro --omni --port 8091
```

## 4. Verify Server Health

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

Play `test.wav` to confirm voice generation and emotional delivery.

## 5. Connecting in LocalRPG

### Via Settings Studio (GUI)

1. Open **Settings Studio** -> **Media** -> **TTS**.
2. Click **Load TTS Preset…** and choose **Fish Audio S2 Pro (Local vLLM-Omni)** (or select **Fish Audio S2 (Local vLLM-Omni)** in the **TTS Engine** dropdown).
3. The endpoint defaults to `http://localhost:8091` and model to `fishaudio/s2-pro`.
4. Click **Save Settings**.

### Vocal Performance Steering

Bracketed delivery tags (`[whisper]`, `[excited]`, `[angry]`, `[sad]`, `[laugh]`, `[sigh]`, `[gasp]`, `[cough]`, `[cry]`, `[screaming]`, `[shouting]`) are automatically understood and preserved in spoken narration.

### Zero-Shot Voice Cloning

Under **Voice Profiles**, configure `ref_audio` (such as `assets/voices/guard.wav` or a file path) and `ref_text` (transcript of the clip). LocalRPG automatically reads and converts the clip to a data URI for the server.

### Via Configuration File (`config.yaml` or `localrpg.yaml`)

```yaml
media:
  tts:
    type: http
    endpoint: "http://localhost:8091"
    model: "fishaudio/s2-pro"
    default_voice: "default"
    pitch: 1.0
    speech_rate: 1.0
    master_volume: 1.0
    auto_play: true
```
