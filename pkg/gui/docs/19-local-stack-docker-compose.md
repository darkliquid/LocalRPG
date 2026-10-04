---
id: 19-local-stack-docker-compose
title: Fully Local AI Stack with Docker Compose
category: Local AI & Self-Hosting
order: 19
description: One Docker Compose project that runs Ollama, Kokoro, Faster-Whisper, and ComfyUI together, with recommended models, system requirements, and the matching LocalRPG configuration.
---

# Fully Local AI Stack with Docker Compose

The earlier guides each start a service. This one wires all
four into one Compose project, so a machine with no outbound network access can
still run the Game Master, narrate dialogue, transcribe your voice, and illustrate
the scene. Every container binds to `127.0.0.1`, which matches LocalRPG's
local-first design: nothing on the stack is reachable from the local network, and
no API key is required for any of it.

## 1. Services in the stack

| Service | Image | Host port | LocalRPG role |
| --- | --- | --- | --- |
| Ollama | `ollama/ollama:latest` | `11434` | GM, Narrator, and Extractor LLM |
| Kokoro-FastAPI | `ghcr.io/remsky/kokoro-fastapi-cpu:latest` | `8880` | Text-to-speech for narration and dialogue |
| Faster-Whisper | `fedirz/faster-whisper-server:latest-cpu` | `8000` | Speech-to-text for voice dictation |
| ComfyUI | `yanwk/comfyui-boot:cpu` | `8188` | Scene, portrait, and item illustration |

Ready-to-run files live in the repository under `docker/`:

- `docker/compose.local.yml` runs the CPU baseline.
- `docker/compose.gpu.yml` is an override that swaps in the CUDA images and
  reserves the host GPU.

## 2. System requirements

These services have different
appetites. Ollama scales with the model you pick, ComfyUI wants a GPU, and Kokoro
and Faster-Whisper are cheap either way. Pick the row that matches the hardware
you already have.

| Tier | CPU | System RAM | GPU (VRAM) | What it runs comfortably |
| --- | --- | --- | --- | --- |
| **Entry, CPU only** | 4 modern cores with AVX2 | 16 GB | none | `gemma3:4b` or `llama3.2` for text; Kokoro and Whisper on CPU; use the built-in procedural art generator instead of ComfyUI |
| **Recommended** | 8 cores | 32 GB | 8–12 GB (RTX 3060 12 GB, RTX 4060, Apple M-series 16 GB) | `gemma4:12b` or `llama3.1:8b`; Kokoro on GPU; Whisper `small` on GPU; Stable Diffusion XL in ComfyUI |
| **Comfortable** | 12+ cores | 64 GB | 16–24 GB (RTX 3090, RTX 4090, Apple M-series 32 GB+) | `gemma4:26b` or `gemma4:31b`; Whisper `large-v3`; Flux.1 or SD3.5 checkpoints |

> [!IMPORTANT]
> One 12 GB card can host Ollama, Kokoro, and Whisper at the same
> time, but Stable Diffusion XL wants most of that VRAM to itself. If ComfyUI
> starts returning out-of-memory errors, stop the other containers while you
> generate art, or move up to a 24 GB card.

Disk usage matters as much as memory, because the model weights are what you
actually download:

| Component | Disk footprint |
| --- | --- |
| Ollama models | 2–20 GB depending on the model and its quantization |
| Kokoro weights | ~350 MB |
| Faster-Whisper `small` | ~500 MB |
| ComfyUI base image plus one SDXL checkpoint | ~2 GB image plus 7 GB checkpoint |

Budget **15–35 GB** of free disk for a complete working stack, and note that the
ComfyUI CUDA image is by far the largest single pull.

## 3. The Compose project

The base file is the CPU stack. Save it as `docker/compose.local.yml` (it's
already there in a checkout of this repository):

```yaml
name: localrpg-local-ai

services:
  ollama:
    image: ollama/ollama:latest
    container_name: localrpg-ollama
    restart: unless-stopped
    ports:
      - "127.0.0.1:11434:11434"
    volumes:
      - ollama-models:/root/.ollama
    healthcheck:
      test: ["CMD-SHELL", "ollama list >/dev/null 2>&1 || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 30s

  kokoro:
    image: ghcr.io/remsky/kokoro-fastapi-cpu:latest
    container_name: localrpg-kokoro
    restart: unless-stopped
    ports:
      - "127.0.0.1:8880:8880"

  whisper:
    image: fedirz/faster-whisper-server:latest-cpu
    container_name: localrpg-whisper
    restart: unless-stopped
    ports:
      - "127.0.0.1:8000:8000"
    environment:
      WHISPER_MODEL: Systran/faster-whisper-small
      WHISPER__COMPUTE_TYPE: int8
    volumes:
      - whisper-models:/root/.cache/huggingface

  comfyui:
    image: yanwk/comfyui-boot:cpu
    container_name: localrpg-comfyui
    restart: unless-stopped
    ports:
      - "127.0.0.1:8188:8188"
    volumes:
      - comfyui-models:/root/ComfyUI/models
      - comfyui-output:/root/ComfyUI/output

volumes:
  ollama-models:
  whisper-models:
  comfyui-models:
  comfyui-output:
```

Named volumes keep every downloaded weight outside the containers, so
`docker compose down` never costs you a re-download.

The GPU override is a second file rather than a second stack, so the two never
drift. It swaps the image tags and reserves the GPU:

```yaml
x-nvidia-gpu: &nvidia-gpu
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: all
            capabilities: [gpu]

services:
  ollama:
    <<: *nvidia-gpu

  kokoro:
    image: ghcr.io/remsky/kokoro-fastapi-gpu:latest
    <<: *nvidia-gpu

  whisper:
    image: fedirz/faster-whisper-server:latest-cuda
    environment:
      WHISPER_MODEL: Systran/faster-whisper-small
      WHISPER__COMPUTE_TYPE: float16
    <<: *nvidia-gpu

  comfyui:
    image: yanwk/comfyui-boot:cu130-slim-v2
    <<: *nvidia-gpu
```

`WHISPER__COMPUTE_TYPE` uses a double underscore because it maps to the nested
`whisper.compute_type` setting. `int8` is the right choice on CPU and `float16`
on a GPU.

## 4. Starting and stopping the stack

CPU hosts:

```bash
docker compose -f docker/compose.local.yml up -d
```

NVIDIA hosts (Linux with the NVIDIA Container Toolkit, or Windows with the WSL2
backend):

```bash
docker compose -f docker/compose.local.yml -f docker/compose.gpu.yml up -d
```

Check that everything came up, and follow one service's log while it
downloads its weights:

```bash
docker compose -f docker/compose.local.yml ps
docker compose -f docker/compose.local.yml logs -f ollama
```

Stop the stack without losing anything, or remove the containers and networks
while keeping the model volumes:

```bash
docker compose -f docker/compose.local.yml stop
docker compose -f docker/compose.local.yml down
```

Add `-v` to `down` only when you actually want to re-download every model.

## 5. Pulling and pinning models

The image doesn't include an Ollama model, so pull at least one before you play:

```bash
docker exec -it localrpg-ollama ollama pull gemma4:12b
docker exec -it localrpg-ollama ollama pull llama3.2
docker exec -it localrpg-ollama ollama list
```

Faster-Whisper fetches `Systran/faster-whisper-small` on the first transcription
request and caches it in the `whisper-models` volume, so the first dictation is
slower than the ones that follow. To pre-load it instead, set `PRELOAD_MODELS` in
the service's `environment` block.

Kokoro works out of the box: the container includes the ONNX weights and all eleven
voice embeddings. ComfyUI needs a checkpoint, which you place in the
`comfyui-models` volume under `checkpoints/`. One way is a bind mount
instead of a named volume, pointing at a folder you can drop files into:

```yaml
  comfyui:
    volumes:
      - ~/comfyui/models:/root/ComfyUI/models
      - ~/comfyui/output:/root/ComfyUI/output
```

Then download a checkpoint such as `sd_xl_base_1.0.safetensors`, or a fantasy
fine-tune from a site like Civitai, into `~/comfyui/models/checkpoints/` and
restart the container.

## 6. Recommended models

Model choice is the biggest lever on turn latency and prose quality. The
Extractor runs on every turn and only needs structured output, so give it
something small and keep the bigger model for the GM and Narrator.

| Role | Model | Disk | Notes |
| --- | --- | --- | --- |
| GM and Narrator, baseline | `gemma4:12b` | ~8 GB | Good balance of reasoning, prose, and context length at 12 GB VRAM |
| GM and Narrator, light | `gemma4:latest` | ~7 GB | The edge variant, usable on 8 GB VRAM or CPU-only at 16 GB RAM |
| GM and Narrator, heavy | `gemma4:26b` | ~17 GB | Mixture-of-experts, fast for its size, wants 24 GB VRAM |
| GM and Narrator, alternative | `llama3.1:8b` or `qwen2.5:7b` | 5–6 GB | Strong instruction following and reliable Markdown |
| Extractor | `llama3.2` | ~2 GB | Fast, cheap, and good enough for entity and mention extraction |
| Speech-to-text | `Systran/faster-whisper-small` | ~500 MB | Good accuracy on fantasy names at near-real-time speed |
| Speech-to-text, CPU fallback | `Systran/faster-whisper-base.en` | ~150 MB | Use when `small` is too slow without a GPU |
| Text-to-speech | Kokoro `af_bella`, `am_adam`, `bm_george` | ~350 MB | Warm, commanding, and distinguished voices respectively |

> [!TIP]
> Keep one model resident for play and pull alternates for experiments. Switching
> models mid-campaign changes the narrator's voice noticeably, which is fine for
> testing but jarring mid-session.

## 7. Pointing LocalRPG at the stack

Because every service speaks an OpenAI-compatible dialect on localhost, one
configuration block covers the whole stack. Put this in `config.yaml` in your
user config directory, or in `./localrpg.yaml` for one project:

```yaml
agents:
  roles:
    gm:
      type: http
      endpoint: "http://localhost:11434/v1"
      model: "gemma4:12b"
      temperature: 0.7
      max_tokens: 1024
    narrator:
      type: http
      endpoint: "http://localhost:11434/v1"
      model: "gemma4:12b"
      temperature: 0.8
      max_tokens: 1024
    extractor:
      type: http
      endpoint: "http://localhost:11434/v1"
      model: "llama3.2"
      temperature: 0.1
      max_tokens: 512

media:
  tts:
    type: http
    endpoint: "http://localhost:8880"
    model: "kokoro"
    default_voice: "af_bella"
    auto_play: true
  stt:
    type: http
    endpoint: "http://localhost:8000/v1/audio/transcriptions"
    model: "whisper-1"
  image:
    type: http
    endpoint: "http://127.0.0.1:8188"
    auto_generate: false
```

The same path through the GUI is **Settings Studio** -> **Agents** for the
three roles and **Settings Studio** -> **Media** for the three media services. Each
panel offers a preset dropdown (**Ollama**, **Kokoro-FastAPI**, **Faster-Whisper**,
**ComfyUI**) that fills in the endpoint for you.

To confirm the wiring without opening the app, run a turn from the terminal:

```bash
localrpg play <game-id>
```

If the GM replies, Ollama is reachable. If narration audio plays, Kokoro is
reachable. The microphone button and the **Generate Image** action cover the other
two.

## 8. Verifying each service

Each service responds to a cheap health request, which is a quick way to tell a
slow model download from a broken container:

```bash
curl -s http://localhost:11434/api/tags
curl -s http://localhost:8880/v1/audio/voices
curl -s http://localhost:8000/health
curl -s http://127.0.0.1:8188/system_stats
```

`/api/tags` lists the Ollama models you have pulled, `/v1/audio/voices` lists the
Kokoro voices, `/health` returns the transcription server's status, and
`/system_stats` reports the GPU ComfyUI can see.

## 9. CPU-only, AMD, and Apple Silicon

- **CPU-only** works for the whole stack except image generation. Ollama,
  Kokoro, and Faster-Whisper are all usable without a GPU; ComfyUI on CPU is
  technically functional but slow enough that the built-in procedural art
  generator (`builtin_name: procedural-art`) is the better default. Leave
  `media.image.type` as `builtin` for pure-Go SVG art with no GPU at all.
- **AMD GPUs** on Linux use the ROCm tags instead of CUDA:
  `ghcr.io/remsky/kokoro-fastapi-rocm` for Kokoro and `yanwk/comfyui-boot:rocm7`
  for ComfyUI. Ollama's ROCm support depends on your card, so check its
  documentation before assuming it's covered.
- **Apple Silicon** can't pass Metal through a container, so no GPU reservation
  works there. Run Ollama and ComfyUI natively (both use Metal directly
  and are much faster for it) and keep the CPU Kokoro and Whisper containers.
  Point the `endpoint` values at the native servers, which listen on the same
  ports.

## 10. Troubleshooting and shrinking the footprint

- **A service is unhealthy but the logs look fine.** Give Ollama up to a minute
  on first start; the healthcheck's `start_period` covers it.
- **Ollama ignores the GPU.** Confirm the container can access the device with
  `docker exec localrpg-ollama nvidia-smi`. If that fails, the NVIDIA Container
  Toolkit is missing on the host.
- **Turns are slow.** Move the Extractor to a smaller model, lower
  `max_tokens`, or set `agents.roles.extractor: disabled` to record deterministic
  entity mentions with no model call at all.
- **ComfyUI runs out of memory.** Stop Kokoro and Ollama while generating art, or
  switch from SDXL to a Stable Diffusion 1.5 checkpoint.
- **Disk pressure.** `ollama rm <model>` drops a model you aren't using, and the
  `comfyui-models` volume is the next biggest item.

## 11. Per-service deep dives

Each of these covers the native installation path, the full preset list, and
provider-specific tuning:

- [Setting Up Ollama for Local LLMs](14-local-llm-ollama)
- [Setting Up Kokoro-FastAPI for Neural TTS](15-local-tts-kokoro)
- [Setting Up Fish Audio S2 with vLLM-Omni](16-local-tts-fish-audio)
- [Setting Up Faster-Whisper for Speech Recognition](17-local-stt-whisper)
- [Setting Up ComfyUI for Local Image Generation](18-local-image-comfyui)
