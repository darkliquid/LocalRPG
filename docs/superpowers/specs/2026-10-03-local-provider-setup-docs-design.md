# Design: Local AI & Self-Hosting Provider Documentation

## Context & Motivation

LocalRPG is built around local-first tabletop roleplaying with modular AI providers for LLM agents, text-to-speech (TTS), speech-to-text (STT), and image generation. While cloud APIs (Gemini, OpenAI, ElevenLabs) work out of the box with an API key, running local inference engines completely offline requires specific container or environment setups, model selection, port configuration, and performance tuning.

Currently, Fish Audio S2 with vLLM-Omni has an in-depth local setup guide embedded directly inside `pkg/gui/docs/05-providers.md`. However, users running other supported local backends—such as Ollama for LLMs, Kokoro-FastAPI for TTS, Faster-Whisper for STT, and ComfyUI for image generation—lack dedicated end-to-end setup instructions covering hardware requirements, Docker/command-line execution, health checks, and LocalRPG GUI/config integration.

Furthermore, housing 80+ line setup guides inside `05-providers.md` clutters high-level architectural documentation.

## Goals

1. **Standardized Dedicated Guides**: Author standalone, step-by-step documentation articles for each primary local engine under a new category: `Local AI & Self-Hosting`.
2. **Consistent 5-Part Template**:
   - Hardware Requirements (CPU, RAM, VRAM, GPU acceleration)
   - Running via Docker & Standalone Commands (docker run, `docker-compose.yml`, native installers where applicable)
   - Health Check & Verification (`curl` command testing the API endpoint before connecting LocalRPG)
   - Connecting in LocalRPG (Settings Studio UI steps + YAML snippet)
   - Recommended Models, Configurations & Alternatives (e.g. LM Studio, Automatic1111, whisper-cli)
3. **Decouple `05-providers.md`**: Relocate Fish Audio S2 into a dedicated guide alongside the other local services, keeping `05-providers.md` concise with clear cross-references.
4. **Site & In-App Parity**: Ensure new documentation renders seamlessly in the desktop app's `DocsModal` and the `tools/sitegen` static documentation site.
5. **Rigorous Verification**: Update tests in `pkg/gui/docs_test.go` and verify markdown formatting with `mise run lint:docs`.

## Non-Goals

- Implementing an all-in-one multi-container `docker-compose.yml` orchestrating all services together (per user decision, each guide remains strictly standalone).
- Creating new backend provider implementations (this is documentation and docs verification for existing presets and provider configurations).

## Documentation Structure & Metadata

All articles reside in `pkg/gui/docs/` and adhere to standard frontmatter:

```yaml
---
id: <article-id>
title: <Human-Readable Title>
category: Local AI & Self-Hosting
order: <integer>
description: <Short summary>
---
```

### New and Updated Articles

| File Path | Doc ID | Title | Order | Summary |
| --- | --- | --- | --- | --- |
| `pkg/gui/docs/05-providers.md` | `05-providers` | AI & Media Providers | `5` | Refactored: removes inline Fish Audio guide, adds introductory local AI overview with links to articles 14–18. |
| `pkg/gui/docs/14-local-llm-ollama.md` | `14-local-llm-ollama` | Setting Up Ollama for Local LLMs | `14` | Local LLM setup with Ollama, recommended models (Llama 3.2, Qwen 2.5), context limits, and LM Studio / vLLM notes. |
| `pkg/gui/docs/15-local-tts-kokoro.md` | `15-local-tts-kokoro` | Setting Up Kokoro-FastAPI for Neural TTS | `15` | Fast CPU/GPU neural voice synthesis using Kokoro-FastAPI container, port 8880, and the 11 built-in voice profiles. |
| `pkg/gui/docs/16-local-tts-fish-audio.md` | `16-local-tts-fish-audio` | Setting Up Fish Audio S2 with vLLM-Omni | `16` | Relocated from 05-providers: 4B Dual-AR multilingual TTS, emotion tags (`[whisper]`, `[excited]`), and zero-shot voice cloning. |
| `pkg/gui/docs/17-local-stt-whisper.md` | `17-local-stt-whisper` | Setting Up Faster-Whisper for Speech Recognition | `17` | Local STT server using `faster-whisper-server`, OpenAI-compatible `/v1/audio/transcriptions`, and whisper-cli notes. |
| `pkg/gui/docs/18-local-image-comfyui.md` | `18-local-image-comfyui` | Setting Up ComfyUI for Local Image Generation | `18` | Local image generation using ComfyUI API on port 8188, workflow endpoints, and Automatic1111 notes. |

---

## Detailed Content Specifications

### 1. `14-local-llm-ollama.md` (Ollama)
- **Introduction**: Overview of Ollama as the easiest local LLM runner supporting OpenAI-compatible chat completions on port `11434`.
- **Hardware Requirements**:
  - Small / Fast: 3B models (`llama3.2:3b`, `qwen2.5:3b`) requiring ~4–6 GB RAM/VRAM; excellent for CPUs or entry-level laptops.
  - Recommended Baseline: 8B models (`llama3.1:8b`, `qwen2.5:7b`) requiring ~8–12 GB RAM/VRAM; great narrative depth and instruction following.
  - High End: 14B–32B models requiring 16–32 GB VRAM.
- **Running Ollama**:
  - Native installer: macOS, Linux (`curl -fsSL https://ollama.com/install.sh | sh`), and Windows installer.
  - Standalone Docker:
    ```bash
    docker run -d --gpus=all -v ollama:/root/.ollama -p 11434:11434 --name ollama ollama/ollama
    docker exec -it ollama ollama run llama3.2
    ```
  - Standalone `docker-compose.yml` snippet.
- **Health & Verification**:
  ```bash
  curl http://localhost:11434/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d '{
      "model": "llama3.2",
      "messages": [{"role": "user", "content": "Say hello!"}]
    }'
  ```
- **LocalRPG Connection**:
  - Settings Studio -> Agents -> Roles (e.g. GM, Narrator).
  - Select preset **Ollama** (`http://localhost:11434/v1`).
  - Model set to `llama3.2` (or user model).
  - YAML configuration block for `config.yaml` / `localrpg.yaml`.
- **Alternatives**:
  - **LM Studio**: Run GUI, download model, start Local Server on port `1234` (`http://localhost:1234/v1`).
  - **vLLM**: For high-throughput batched serving (`vllm serve ... --port 8000`).

### 2. `15-local-tts-kokoro.md` (Kokoro-FastAPI)
- **Introduction**: Lightweight 82M-parameter neural TTS model running via the community-standard `kokoro-fastapi` container with OpenAI-compatible `/v1/audio/speech`.
- **Hardware Requirements**:
  - CPU: ~1 GB RAM, easily achieves < 0.2x real-time factor on modern quad-core CPUs.
  - GPU: ~1.5 GB VRAM with NVIDIA CUDA.
- **Running via Docker**:
  - Docker run command:
    ```bash
    docker run -d -p 8880:8880 --name kokoro-fastapi ghcr.io/remsky/kokoro-fastapi-cpu:latest
    ```
    (and GPU variant `ghcr.io/remsky/kokoro-fastapi-gpu:latest` with `--gpus all`).
  - Standalone `docker-compose.yml`.
- **Health & Verification**:
  ```bash
  curl -X POST http://localhost:8880/v1/audio/speech \
    -H "Content-Type: application/json" \
    -d '{
      "model": "kokoro",
      "input": "Greetings, adventurer! The Kokoro voice server is online.",
      "voice": "af_bella",
      "response_format": "wav"
    }' --output test.wav
  ```
- **LocalRPG Connection**:
  - Settings Studio -> Media -> TTS.
  - Click **Load TTS Preset...** -> **Kokoro-FastAPI** (endpoint defaults to `http://localhost:8880`, model `kokoro`).
  - Click **Load Kokoro Voices (11 Profiles)** to populate all 11 stock voice profiles (`af_bella`, `am_adam`, `bf_emma`, etc.) with archetype and gender tags.
  - YAML configuration block.

### 3. `16-local-tts-fish-audio.md` (Fish Audio S2 Pro)
- **Introduction**: Relocated from `05-providers.md`. High-fidelity 4B Dual-AR multilingual TTS with emotion tags and zero-shot voice cloning.
- **Hardware Requirements**: FP16 (17–24 GB VRAM) vs Quantized (8–12 GB VRAM).
- **Running via Docker & vLLM-Omni**: `docker run` and `docker-compose.yml` on port `8091`.
- **Python / Pip Host Setup**: `pip install vllm-omni fish-speech` and `vllm serve fishaudio/s2-pro --omni --port 8091`.
- **Health & Verification**: `curl` against `http://localhost:8091/v1/audio/speech`.
- **LocalRPG Connection**: Settings Studio preset, emotion delivery tags (`[whisper]`, `[excited]`, `[angry]`), voice cloning reference audio and text.

### 4. `17-local-stt-whisper.md` (Faster-Whisper)
- **Introduction**: Local speech-to-text allowing hands-free voice prompt entry during gameplay.
- **Hardware Requirements**:
  - Model sizes: `tiny`, `base`, `small`, `medium`.
  - CPU: ~1–2 GB RAM.
  - GPU: ~1.5–3 GB VRAM for near-instant transcription.
- **Running via Docker**:
  - Using `fedirz/faster-whisper-server:latest-cuda` or `latest-cpu`:
    ```bash
    docker run -d --gpus all -p 8000:8000 \
      -e WHISPER_MODEL=Systran/faster-whisper-small \
      fedirz/faster-whisper-server:latest-cuda
    ```
  - Standalone `docker-compose.yml`.
- **Health & Verification**:
  ```bash
  curl -X POST http://localhost:8000/v1/audio/transcriptions \
    -H "Content-Type: multipart/form-data" \
    -F file=@test.wav \
    -F model="whisper-1"
  ```
- **LocalRPG Connection**:
  - Settings Studio -> Media -> STT.
  - Select preset **Faster-Whisper** (endpoint `http://localhost:8000/v1/audio/transcriptions`, model `whisper-1`).
  - YAML configuration block.
- **Alternatives**:
  - Using `whisper-cli` binary with ggml models for direct CLI transcription.

### 5. `18-local-image-comfyui.md` (ComfyUI)
- **Introduction**: Node-based local Stable Diffusion / Flux image generation server for campaign scenes, item icons, and character portraits.
- **Hardware Requirements**:
  - SD 1.5: 4–6 GB VRAM.
  - SDXL: 8–12 GB VRAM.
  - Flux.1: 12–24 GB VRAM.
- **Running ComfyUI**:
  - Launch with `--listen 127.0.0.1 --port 8188` to allow API requests from LocalRPG.
  - Docker setup with GPU passthrough.
- **Health & Verification**:
  ```bash
  curl http://127.0.0.1:8188/system_stats
  ```
- **LocalRPG Connection**:
  - Settings Studio -> Media -> Image.
  - Select preset **ComfyUI** (endpoint `http://127.0.0.1:8188`).
  - YAML configuration block.
- **Alternatives**:
  - **Automatic1111 / Stable Diffusion WebUI**: Launch with `--api` on port `7860` (`http://127.0.0.1:7860/sdapi/v1/txt2img`).

---

## Refactoring `05-providers.md`

`05-providers.md` will be updated to:
1. Maintain its concise role as the architectural overview for all AI & media providers.
2. Remove the 80 lines of inline Fish Audio vLLM setup code.
3. Include prominent links under each modality:
   - LLMs: Link to `[Setting Up Ollama for Local LLMs](14-local-llm-ollama)`.
   - Voice (TTS): Link to `[Setting Up Kokoro-FastAPI](15-local-tts-kokoro)` and `[Setting Up Fish Audio S2](16-local-tts-fish-audio)`.
   - Speech (STT): Link to `[Setting Up Faster-Whisper](17-local-stt-whisper)`.
   - Image Generation: Link to `[Setting Up ComfyUI](18-local-image-comfyui)`.
4. Ensure summary tables and footer links properly guide readers to the `Local AI & Self-Hosting` section.

---

## Verification & Testing

1. **Category & Document Discovery (`pkg/gui/docs_test.go`)**:
   - Add `"Local AI & Self-Hosting"` to `expectedCategories` in `TestDocsService_GetDocsList`.
   - Add explicit assertions that articles `14-local-llm-ollama` through `18-local-image-comfyui` can be loaded via `GetDocArticle` and contain valid Markdown content.
   - Run `TestDocsService_InternalLinksResolve` to ensure every internal markdown link (`[Title](doc-id)`) resolves to a valid embedded article ID.
2. **Markdown Linting**:
   - Execute `mise run lint:docs` (`markdownlint-cli2 "../pkg/gui/docs/*.md"`). All files must pass with 0 warnings.
3. **Catalogue & Schema Tests**:
   - Run `go test ./pkg/gui -update-docs` to ensure embedded catalogue files are synced.
   - Run `go test -v -count=1 ./pkg/gui/...` to verify full GUI suite passes.
4. **Static Site Build**:
   - Run `mise run site:build` to confirm `tools/sitegen` processes the new category and articles into `website/dist` without errors.
