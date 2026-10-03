---
id: 15-local-tts-kokoro
title: Setting Up Kokoro-FastAPI for Neural TTS
category: Local AI & Self-Hosting
order: 15
description: Running Kokoro-FastAPI via Docker (CPU/GPU), voice profile selection, and connecting to LocalRPG.
---

# Setting Up Kokoro-FastAPI for Neural TTS

Kokoro is an 82-million parameter neural text-to-speech model that delivers remarkable voice quality, natural cadence, and accurate pronunciation while remaining lightweight enough to run in real time on modern CPUs.

When run via the community **Kokoro-FastAPI** container, it exposes an OpenAI-compatible speech endpoint on port `8880` (`/v1/audio/speech`), allowing LocalRPG to stream dialogue and narration audio with near-zero latency.

## 1. Hardware Requirements

- **CPU Mode**: Runs in real time (< 0.2x real-time factor) on modern x86_64 and ARM64 CPUs. Requires ~1 GB of system RAM.
- **GPU Mode**: Requires ~1.5 GB of VRAM on NVIDIA GPUs (CUDA 12+).
- **Disk Space**: ~350 MB for the base ONNX weights and voice embedding tensors.

## 2. Running via Docker (Recommended)

### CPU Execution (No GPU required)

```bash
docker run -d \
  --name kokoro-fastapi \
  --restart always \
  -p 8880:8880 \
  ghcr.io/remsky/kokoro-fastapi-cpu:latest
```

### GPU Acceleration (NVIDIA CUDA)

```bash
docker run -d \
  --name kokoro-fastapi \
  --restart always \
  --gpus all \
  -p 8880:8880 \
  ghcr.io/remsky/kokoro-fastapi-gpu:latest
```

### Docker Compose (`docker-compose.yml`)

```yaml
services:
  kokoro-fastapi:
    image: ghcr.io/remsky/kokoro-fastapi-cpu:latest # or ghcr.io/remsky/kokoro-fastapi-gpu:latest
    container_name: kokoro-fastapi
    restart: always
    ports:
      - "8880:8880"
```

## 3. Verify Server Health

Synthesize a test audio clip to verify that the container is ready:

```bash
curl -X POST http://localhost:8880/v1/audio/speech \
  -H "Content-Type: application/json" \
  -d '{
    "model": "kokoro",
    "input": "Greetings, adventurer! The local Kokoro speech engine is online and ready.",
    "voice": "af_bella",
    "response_format": "wav"
  }' --output test.wav
```

Play `test.wav` with your system audio player (e.g. `aplay test.wav`, `afplay test.wav`, or `mpv test.wav`) to confirm clean speech output.

## 4. Connecting in LocalRPG

### Via Settings Studio (GUI)

1. Open **Settings Studio** -> **Media** -> **TTS**.
2. In the **TTS Engine** dropdown, choose **HTTP Endpoint (Kokoro-FastAPI, AllTalk, OpenAI Speech)**.
3. Or click **Load TTS Preset...** and select **Kokoro-FastAPI**.
4. Set the **Endpoint** to `http://localhost:8880` and **Model** to `kokoro`.
5. Under **Voice Profiles**, click the **Load Kokoro Voices (11 Profiles)** button. This immediately installs the full catalog of archetyped voices with appropriate gender, region, and personality tags:
   - `af_bella` (Warm, approachable American female)
   - `af_sarah` (Poised, narrative American female)
   - `af_nicole` (Youthful American female)
   - `af_sky` (Gentle, breathy American female)
   - `am_adam` (Deep, commanding American male)
   - `am_michael` (Formal, disciplined American male)
   - `bf_emma` (Poised, elegant British female)
   - `bf_isabella` (Noble British female)
   - `bm_george` (Distinguished British male)
   - `bm_lewis` (Refined British male)
6. Click **Save Settings**.

### Via Configuration File (`config.yaml` or `localrpg.yaml`)

```yaml
media:
  tts:
    type: http
    endpoint: "http://localhost:8880"
    model: "kokoro"
    default_voice: "af_bella"
    pitch: 1.0
    speech_rate: 1.0
    master_volume: 1.0
    auto_play: true
```

## 5. Voice Blending & Customization

Kokoro-FastAPI supports blending multiple voices using a plus sign in the voice identifier, for example `af_bella+af_sarah`. You can author custom voice profiles in LocalRPG's Voice Profiles table using blended voice IDs to create distinctive tones for specific NPC archetypes.
