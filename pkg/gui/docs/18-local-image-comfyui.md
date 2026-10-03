---
id: 18-local-image-comfyui
title: Setting Up ComfyUI for Local Image Generation
category: Local AI & Self-Hosting
order: 18
description: ComfyUI API configuration, model setup (SDXL/Flux/SD1.5), and connecting LocalRPG for scene and entity illustration.
---

# Setting Up ComfyUI for Local Image Generation

LocalRPG can generate scene illustrations, NPC portraits, and item icons directly as your tabletop campaign unfolds. For high-quality offline image generation, LocalRPG connects directly to **ComfyUI** over its local HTTP API on port `8188`.

## 1. Hardware Requirements

Image synthesis is GPU-intensive and benefits heavily from dedicated VRAM:

| Model Architecture | VRAM | Recommended GPUs |
| --- | --- | --- |
| **Stable Diffusion 1.5** | 4–6 GB | NVIDIA GTX 1060, RTX 2060, Apple Silicon (8GB+) |
| **SDXL (Recommended)** | 8–12 GB | NVIDIA RTX 3060, RTX 4060, Apple Silicon (16GB+) |
| **Flux.1 / SD3** | 12–24 GB | NVIDIA RTX 3090, RTX 4080/4090, Apple Silicon (32GB+) |

## 2. Running ComfyUI

### Option A: Standalone Python Setup

Clone and install ComfyUI according to the official documentation, then launch it allowing local network listening:

```bash
python main.py --listen 127.0.0.1 --port 8188
```

Ensure your target checkpoint (for example `sd_xl_base_1.0.safetensors` or a fantasy fine-tune) is placed inside `ComfyUI/models/checkpoints/`.

### Option B: Running via Docker

```bash
docker run -d \
  --name comfyui \
  --restart always \
  --gpus all \
  -p 8188:8188 \
  -v ~/comfyui/models:/app/models \
  -v ~/comfyui/output:/app/output \
  yanwk/comfyui-boot:latest
```

## 3. Verify Server Health

Test that ComfyUI is active and serving its API:

```bash
curl -s http://127.0.0.1:8188/system_stats
```

Verify that the response returns system metadata and GPU devices:

```json
{"system": {"os": "linux", "python_version": "..."}, "devices": [{"name": "NVIDIA GeForce RTX 4090", "vram_total": 25757220864}]}
```

## 4. Connecting in LocalRPG

### Via Settings Studio (GUI)

1. Open **Settings Studio** -> **Media** -> **Image**.
2. Click **Load Image Preset...** and select **ComfyUI**.
3. Confirm the **Endpoint** is set to `http://127.0.0.1:8188`.
4. Click **Save Settings**.
5. In your campaign or Codex entity view, click **Generate Image** to request an illustration.

### Via Configuration File (`config.yaml` or `localrpg.yaml`)

```yaml
media:
  image:
    type: http
    endpoint: "http://127.0.0.1:8188"
```

## 5. Alternatives: Automatic1111 & Stable Diffusion WebUI

If you use **Automatic1111** or **SD-WebUI-Forge**, launch the web UI with the `--api` argument (e.g. `COMMANDLINE_ARGS="--api"`):

```yaml
media:
  image:
    type: http
    endpoint: "http://127.0.0.1:7860/sdapi/v1/txt2img"
```

Select preset **Automatic1111** in Settings Studio to connect.
