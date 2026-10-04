---
id: 17-local-stt-whisper
title: Setting Up Faster-Whisper for Speech Recognition
category: Local AI & Self-Hosting
order: 17
description: OpenAI-compatible speech-to-text server setup via Docker, hardware considerations, and LocalRPG voice input configuration.
---

# Setting Up Faster-Whisper for Speech Recognition

LocalRPG supports voice dictation, allowing you to speak your character's actions and dialogue directly into your microphone. Rather than sending microphone audio to third-party cloud services, you can run a local OpenAI-compatible transcription server powered by **faster-whisper** (CTranslate2).

## 1. Hardware Requirements

Faster-Whisper is optimized for high-speed inference on both CPUs and GPUs:

| Model | Memory (VRAM / RAM) | Accuracy | Relative Speed |
| --- | --- | --- | --- |
| `tiny` / `base` | < 1 GB | Basic commands | ~16x real-time on GPU, ~3x on CPU |
| `small` (Recommended) | ~1.5 GB | Excellent for RPG prose & character dialogue | ~10x real-time on GPU, ~1.5x on CPU |
| `medium` / `large-v3` | ~3–5 GB | Near-perfect multilingual transcription | ~5x real-time on GPU |

> [!TIP]
> The `small` or `base.en` models provide instantaneous transcription and rarely mishear fantasy terms.

## 2. Running via Docker (Recommended)

The community-standard `fedirz/faster-whisper-server` provides an OpenAI-compatible `/v1/audio/transcriptions` endpoint on port `8000`.

### GPU Acceleration (NVIDIA CUDA)

```bash
docker run -d \
  --name faster-whisper \
  --restart always \
  --gpus all \
  -p 8000:8000 \
  -e WHISPER_MODEL=Systran/faster-whisper-small \
  fedirz/faster-whisper-server:latest-cuda
```

### CPU Execution (No GPU required)

```bash
docker run -d \
  --name faster-whisper \
  --restart always \
  -p 8000:8000 \
  -e WHISPER_MODEL=Systran/faster-whisper-small \
  fedirz/faster-whisper-server:latest-cpu
```

### Docker Compose (`docker-compose.yml`)

```yaml
services:
  whisper:
    image: fedirz/faster-whisper-server:latest-cuda # or latest-cpu
    container_name: faster-whisper
    restart: always
    ports:
      - "8000:8000"
    environment:
      - WHISPER_MODEL=Systran/faster-whisper-small
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
```

## 3. Verify Server Health

Test transcribing an audio file:

```bash
curl -s -X POST http://localhost:8000/v1/audio/transcriptions \
  -H "Content-Type: multipart/form-data" \
  -F file=@test.wav \
  -F model="whisper-1"
```

Verify that the returned JSON contains the transcription text:

```json
{"text":"Greetings, adventurer! The local speech engine is online and ready."}
```

## 4. Connecting in LocalRPG

### Via Settings Studio (GUI)

1. Open **Settings Studio** -> **Media** -> **STT**.
2. Click **Load STT Preset…** and select **Faster-Whisper**.
3. Confirm the **Endpoint** is set to `http://localhost:8000/v1/audio/transcriptions` and **Model** to `whisper-1`.
4. Click **Save Settings**.
5. When playing a campaign, click the microphone button next to the prompt bar to speak your action.

### Via Configuration File (`config.yaml` or `localrpg.yaml`)

```yaml
media:
  stt:
    type: http
    endpoint: "http://localhost:8000/v1/audio/transcriptions"
    model: "whisper-1"
```

## 5. Alternatives: Whisper.cpp (`whisper-cli`)

If you prefer a standalone command-line binary without a long-running HTTP server, LocalRPG also supports `whisper-cli` from the `whisper.cpp` project:

```yaml
media:
  stt:
    type: cli
    command: "whisper-cli"
    args:
      - "-m"
      - "models/ggml-base.bin"
      - "-f"
      - "%INPUT%"
      - "-nt"
```
