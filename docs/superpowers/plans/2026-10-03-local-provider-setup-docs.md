# Local Provider Setup Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Author dedicated step-by-step setup guides for local AI engines (Ollama, Kokoro-FastAPI, Fish Audio S2, Faster-Whisper, ComfyUI) in a new `Local AI & Self-Hosting` documentation category and integrate them with the provider overview and test suite.

**Architecture:** Add five standalone Markdown articles with standardized YAML frontmatter under `pkg/gui/docs/` following a uniform 5-part structure (Hardware, Running via Docker/Commands, Verification curl, LocalRPG configuration, and Alternatives). Refactor `pkg/gui/docs/05-providers.md` to remove the inline Fish Audio S2 guide and cross-link out to each dedicated guide. Update `pkg/gui/docs_test.go` to assert discovery and link resolution across the new category.

**Tech Stack:** Go (embed.FS, testing), Markdown (GitHub-flavored, frontmatter, markdownlint-cli2), Docker & curl commands, tools/sitegen.

---

### File Map

- Modify: `pkg/gui/docs_test.go` (category assertions, article retrieval assertions)
- Create: `pkg/gui/docs/14-local-llm-ollama.md` (Ollama LLM guide)
- Create: `pkg/gui/docs/15-local-tts-kokoro.md` (Kokoro-FastAPI TTS guide)
- Create: `pkg/gui/docs/16-local-tts-fish-audio.md` (Fish Audio S2 TTS guide)
- Create: `pkg/gui/docs/17-local-stt-whisper.md` (Faster-Whisper STT guide)
- Create: `pkg/gui/docs/18-local-image-comfyui.md` (ComfyUI Image guide)
- Modify: `pkg/gui/docs/05-providers.md` (remove inline 80-line Fish Audio guide, add links to 14–18)

---

### Task 1: Update Docs Test Suite for New Category and Article Verification

**Files:**
- Modify: `pkg/gui/docs_test.go:40-75`

- [x] **Step 1: Update `pkg/gui/docs_test.go` with category and article assertions**

Edit `pkg/gui/docs_test.go` to add `"Local AI & Self-Hosting"` to `expectedCategories` in `TestDocsService_GetDocsList`, and add test cases in `TestDocsService_GetDocArticle` asserting that articles `14-local-llm-ollama` through `18-local-image-comfyui` are retrievable and non-empty.

```go
	expectedCategories := []string{
		"Core Concepts",
		"Configuration & Providers",
		"Studio Guides",
		"Codex & Content Reference",
		"Local AI & Self-Hosting",
	}
	for _, cat := range expectedCategories {
		if !foundCategories[cat] {
			t.Errorf("missing expected category %q", cat)
		}
	}
```

And in `TestDocsService_GetDocArticle`:

```go
	localDocIDs := []string{
		"14-local-llm-ollama",
		"15-local-tts-kokoro",
		"16-local-tts-fish-audio",
		"17-local-stt-whisper",
		"18-local-image-comfyui",
	}
	for _, docID := range localDocIDs {
		article, err := svc.GetDocArticle(context.Background(), docID)
		if err != nil {
			t.Errorf("GetDocArticle(%q) failed: %v", docID, err)
			continue
		}
		if article.Category != "Local AI & Self-Hosting" {
			t.Errorf("article %q expected category 'Local AI & Self-Hosting', got %q", docID, article.Category)
		}
		if len(article.Content) == 0 {
			t.Errorf("article %q has empty content", docID)
		}
	}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestDocsService_GetDocsList ./pkg/gui/`
Expected: FAIL with `missing expected category "Local AI & Self-Hosting"`

- [x] **Step 3: Commit test changes**

```bash
git add pkg/gui/docs_test.go
git commit -m "test(docs): assert Local AI & Self-Hosting category and local guides"
```

---

### Task 2: Author Ollama Setup Guide (`14-local-llm-ollama.md`)

**Files:**
- Create: `pkg/gui/docs/14-local-llm-ollama.md`

- [x] **Step 1: Write `pkg/gui/docs/14-local-llm-ollama.md`**

Create the document with full instructions following the 5-part structure:

```markdown
---
id: 14-local-llm-ollama
title: Setting Up Ollama for Local LLMs
category: Local AI & Self-Hosting
order: 14
description: Hardware requirements, native/Docker setup, recommended models for tabletop RPGs, and connecting Ollama to LocalRPG.
---

# Setting Up Ollama for Local LLMs

Ollama is a lightweight, cross-platform runner for open-weights large language models. It provides out-of-the-box hardware acceleration (CUDA, ROCm, Apple Metal) and exposes an OpenAI-compatible HTTP API on port `11434`, making it the easiest way to power LocalRPG's GM, Narrator, and Extractor agents without sending data to cloud APIs.

## 1. Hardware Requirements

LLM performance depends directly on parameter count and quantization:

| Model Tier | Representative Models | VRAM / RAM | Target Hardware |
| --- | --- | --- | --- |
| **Small & Fast** | `llama3.2:3b`, `qwen2.5:3b` | 4–6 GB | Modern CPUs, entry-level laptops, Apple Silicon (M1/M2 8GB+) |
| **Recommended Baseline** | `llama3.1:8b`, `qwen2.5:7b` | 8–12 GB | NVIDIA RTX 3060/4060, Apple Silicon (16GB+), mid-tier GPUs |
| **High Fidelity** | `qwen2.5:14b`, `mistral-small` | 16–24 GB | NVIDIA RTX 3090/4090, Apple Silicon (24GB+) |

> [!TIP]
> For turn-based narrative roleplaying, 8B models like `llama3.1:8b` or `qwen2.5:7b` provide the optimal balance between creative roleplay, markdown formatting compliance, and low turn latency.

## 2. Running Ollama

### Option A: Native Installation (Recommended for Desktop)

Install Ollama directly on your operating system for optimal GPU detection:

- **macOS / Windows**: Download the installer from [ollama.com](https://ollama.com).
- **Linux**: Run the official installation script:

```bash
curl -fsSL https://ollama.com/install.sh | sh
```

Once installed, pull your chosen model from your terminal:

```bash
ollama pull llama3.2
```

Ollama automatically starts as a background service on `http://localhost:11434`.

### Option B: Running via Docker

If you prefer containerized isolation or are running on a headless home server:

```bash
docker run -d \
  --name ollama \
  --restart always \
  --gpus all \
  -v ollama:/root/.ollama \
  -p 11434:11434 \
  ollama/ollama:latest
```

Pull the model inside the container:

```bash
docker exec -it ollama ollama run llama3.2
```

Or run via `docker-compose.yml`:

```yaml
services:
  ollama:
    image: ollama/ollama:latest
    container_name: ollama
    restart: always
    ports:
      - "11434:11434"
    volumes:
      - ollama:/root/.ollama
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]

volumes:
  ollama:
```

## 3. Verify Server Health

Test that Ollama's OpenAI-compatible endpoint responds:

```bash
curl -s http://localhost:11434/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [
      {"role": "user", "content": "Respond with: Ready for adventure."}
    ],
    "temperature": 0.7
  }'
```

Verify that the response JSON contains `Ready for adventure.`.

## 4. Connecting in LocalRPG

### Via Settings Studio (GUI)

1. Open **Settings Studio** -> **Agents**.
2. Under **Agent Roles** (e.g. **GM** or **Narrator**), open the preset dropdown and choose **Ollama**.
3. The endpoint defaults to `http://localhost:11434/v1` and the model to `llama3.2`.
4. If you pulled a different model (e.g. `llama3.1:8b` or `qwen2.5:7b`), type its tag into the **Model** field.
5. Click **Save Settings**.

### Via Configuration File (`config.yaml` or `localrpg.yaml`)

```yaml
agents:
  roles:
    gm:
      type: http
      endpoint: "http://localhost:11434/v1"
      model: "llama3.2"
      temperature: 0.7
      max_tokens: 1024
    narrator:
      type: http
      endpoint: "http://localhost:11434/v1"
      model: "llama3.2"
      temperature: 0.7
      max_tokens: 1024
```

## 5. Alternatives: LM Studio & vLLM

LocalRPG's OpenAI HTTP client works seamlessly with any standard OpenAI-compatible local server:

- **LM Studio**: Run the LM Studio desktop application, load any GGUF model, and click the **Local Server** icon to start serving on `http://localhost:1234/v1`. Select preset `lm-studio` in LocalRPG.
- **vLLM**: For multi-turn throughput and high-concurrency batching on Linux with NVIDIA GPUs, run `vllm serve meta-llama/Llama-3.1-8B-Instruct --port 8000`. Use endpoint `http://localhost:8000/v1` in LocalRPG.
```

- [x] **Step 2: Run linter and tests**

Run: `npm run lint:docs`
Expected: `0 issues in 14 files`

Run: `go test -v -run "TestDocsService_GetDocArticle" ./pkg/gui/`

- [x] **Step 3: Commit**

```bash
git add pkg/gui/docs/14-local-llm-ollama.md
git commit -m "docs(providers): add Ollama local LLM setup guide"
```

---

### Task 3: Author Kokoro-FastAPI Setup Guide (`15-local-tts-kokoro.md`)

**Files:**
- Create: `pkg/gui/docs/15-local-tts-kokoro.md`

- [x] **Step 1: Write `pkg/gui/docs/15-local-tts-kokoro.md`**

Create the document with full instructions following the 5-part structure:

```markdown
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
```

- [x] **Step 2: Run linter and tests**

Run: `npm run lint:docs`
Expected: `0 issues in 15 files`

- [x] **Step 3: Commit**

```bash
git add pkg/gui/docs/15-local-tts-kokoro.md
git commit -m "docs(providers): add Kokoro-FastAPI local TTS setup guide"
```

---

### Task 4: Author Fish Audio S2 Setup Guide (`16-local-tts-fish-audio.md`)

**Files:**
- Create: `pkg/gui/docs/16-local-tts-fish-audio.md`

- [x] **Step 1: Write `pkg/gui/docs/16-local-tts-fish-audio.md`**

Create the document relocating and refining the Fish Audio S2 vLLM-Omni content:

```markdown
---
id: 16-local-tts-fish-audio
title: Setting Up Fish Audio S2 with vLLM-Omni
category: Local AI & Self-Hosting
order: 16
description: GPU requirements, Docker/vLLM-Omni container setup, voice cloning, and emotion tag performance steering.
---

# Setting Up Fish Audio S2 with vLLM-Omni

Fish Audio S2 Pro is an expressive, multilingual 4B Dual-AR neural voice model that runs entirely offline. When paired with **vLLM-Omni**, it exposes an OpenAI-compatible speech endpoint on port `8091` with Triton decode acceleration, sub-second latency, and support for expressive emotion tags and zero-shot voice cloning.

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
2. Click **Load TTS Preset...** and choose **Fish Audio S2 Pro (Local vLLM-Omni)** (or select **Fish Audio S2 (Local vLLM-Omni)** in the **TTS Engine** dropdown).
3. The endpoint defaults to `http://localhost:8091` and model to `fishaudio/s2-pro`.
4. Click **Save Settings**.

### Vocal Performance Steering

Bracketed delivery tags (`[whisper]`, `[excited]`, `[angry]`, `[sad]`, `[laugh]`, `[sigh]`, `[gasp]`, `[cough]`, `[cry]`, `[screaming]`, `[shouting]`) are automatically understood and preserved in spoken narration.

### Zero-Shot Voice Cloning

Under **Voice Profiles**, configure `ref_audio` (e.g. `assets/voices/guard.wav` or a file path) and `ref_text` (transcript of the clip). LocalRPG automatically reads and converts the clip to a data URI for the server.

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
```

- [x] **Step 2: Run linter and tests**

Run: `npm run lint:docs`
Expected: `0 issues in 16 files`

- [x] **Step 3: Commit**

```bash
git add pkg/gui/docs/16-local-tts-fish-audio.md
git commit -m "docs(providers): add Fish Audio S2 vLLM-Omni setup guide"
```

---

### Task 5: Author Faster-Whisper Setup Guide (`17-local-stt-whisper.md`)

**Files:**
- Create: `pkg/gui/docs/17-local-stt-whisper.md`

- [x] **Step 1: Write `pkg/gui/docs/17-local-stt-whisper.md`**

Create the document with full instructions following the 5-part structure:

```markdown
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
> The `small` or `base.en` models provide instantaneous transcription with virtually zero misheard fantasy terms.

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
2. Click **Load STT Preset...** and select **Faster-Whisper**.
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
```

- [x] **Step 2: Run linter and tests**

Run: `npm run lint:docs`
Expected: `0 issues in 17 files`

- [x] **Step 3: Commit**

```bash
git add pkg/gui/docs/17-local-stt-whisper.md
git commit -m "docs(providers): add Faster-Whisper local STT setup guide"
```

---

### Task 6: Author ComfyUI Setup Guide (`18-local-image-comfyui.md`)

**Files:**
- Create: `pkg/gui/docs/18-local-image-comfyui.md`

- [x] **Step 1: Write `pkg/gui/docs/18-local-image-comfyui.md`**

Create the document with full instructions following the 5-part structure:

```markdown
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
```

- [x] **Step 2: Run linter and tests**

Run: `npm run lint:docs`
Expected: `0 issues in 18 files`

- [x] **Step 3: Commit**

```bash
git add pkg/gui/docs/18-local-image-comfyui.md
git commit -m "docs(providers): add ComfyUI local image generation setup guide"
```

---

### Task 7: Refactor `05-providers.md` to Decouple Inline Guide & Add Cross-Links

**Files:**
- Modify: `pkg/gui/docs/05-providers.md`

- [x] **Step 1: Edit `pkg/gui/docs/05-providers.md`**

Replace the 80 lines of inline Fish Audio vLLM guide in `pkg/gui/docs/05-providers.md` with a clean summary and direct markdown cross-references to all five local guides:
- In `## LLM Providers`: reference `[Setting Up Ollama for Local LLMs](14-local-llm-ollama)`.
- In `### Voice (TTS)`: reference `[Setting Up Kokoro-FastAPI](15-local-tts-kokoro)` and `[Setting Up Fish Audio S2](16-local-tts-fish-audio)`.
- In `### Speech Recognition (STT)`: reference `[Setting Up Faster-Whisper](17-local-stt-whisper)`.
- In `### Image Generation`: reference `[Setting Up ComfyUI](18-local-image-comfyui)`.
- In footer links: add reference to the new Local AI & Self-Hosting guides.

- [x] **Step 2: Run linter and internal link resolution test**

Run: `npm run lint:docs`
Expected: `0 issues in 18 files`

Run: `go test -v -run "TestDocsService_InternalLinksResolve" ./pkg/gui/`
Expected: PASS with all internal links verified.

- [x] **Step 3: Commit**

```bash
git add pkg/gui/docs/05-providers.md
git commit -m "docs(providers): decouple inline Fish Audio guide and link to local guides"
```

---

### Task 8: Full Verification & Static Site Generation

**Files:**
- Verify: Full repo tests and documentation sync

- [x] **Step 1: Sync provider catalogue and configuration reference**

Run: `go test ./pkg/gui -update-docs`
Expected: PASS

- [x] **Step 2: Run full documentation linter**

Run: `mise run lint:docs`
Expected: `Summary: 0 issues in 0 files`

- [x] **Step 3: Run full docs test suite**

Run: `go test -v -count=1 ./pkg/gui/ -run "TestDocsService_.*|TestServer_DocsEndpoints"`
Expected: All tests pass, including:
- `TestDocsService_GetDocsList` (category `"Local AI & Self-Hosting"` present)
- `TestDocsService_GetDocArticle` (articles 14–18 retrieved successfully)
- `TestDocsService_InternalLinksResolve` (all internal links resolve)

- [x] **Step 4: Build static website via `tools/sitegen`**

Run: `mise run site:build`
Expected: Site generated cleanly in `website/dist` including the new `Local AI & Self-Hosting` navigation group and rendered articles.

- [x] **Step 5: Run git status and commit if catalogue changed**

```bash
git status -s
```
If clean, proceed. If any generated files changed, commit them with:
```bash
git commit -am "docs: update generated provider catalogue and site assets"
```
