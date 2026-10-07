---
id: 21-local-first
title: Local-First and Offline Play
category: Local AI & Self-Hosting
order: 21
description: What runs locally in LocalRPG, what hardware each tier requires, and how to configure a zero-network tabletop stack.
---

# Local-First and Offline Play

LocalRPG runs offline on any modern laptop using built-in, zero-model providers. You can play tabletop campaigns with zero network connectivity and no GPU; richer narrative prose, neural voices, and high-resolution scene artwork require either self-hosted local models or cloud provider API keys.

## 1. Capability Matrix

The table below outlines what runs across each capability family, comparing fully model-free operation, small CPU models, self-hosted GPU daemons, and cloud endpoints.

| Capability | No GPU, No Model | No GPU, Small Model | GPU / Local Server | Cloud |
| --- | --- | --- | --- | --- |
| **LLM (Storyteller)** | [Narrative Oracle](12-provider-catalogue) · Fast deterministic templates | — | [Ollama](14-local-llm-ollama), llama.cpp, [LM Studio](14-local-llm-ollama) · Rich interactive prose | Gemini, OpenAI · Frontier intelligence |
| **TTS (Speech)** | [`native-os`](12-provider-catalogue) · System speech synthesizer | [Kokoro via Sherpa ONNX](15-local-tts-kokoro) · Natural speech on CPU | [Kokoro-FastAPI](15-local-tts-kokoro), [Piper](12-provider-catalogue), [Fish Audio](16-local-tts-fish-audio) · Fast expressive audio | ElevenLabs, Gemini · Studio quality |
| **STT (Voice Input)** | Web Speech API · In browser mode only | — | [Faster-Whisper](17-local-stt-whisper) · Accurate transcription | Cartesia, Inworld · Low-latency streaming |
| **Image Generation** | [Procedural Art](12-provider-catalogue) · Instant procedural SVG portraits | — | [ComfyUI](18-local-image-comfyui), [Automatic1111](18-local-image-comfyui) · Detailed diffusion art | Gemini Imagen · High-fidelity scenes |
| **Embeddings** | Built-in Projection · Fast hash vectors | [ONNX Encoder](20-local-embeddings) · Small neural embedding model | — | OpenAI, Gemini · Cloud vectors |

### Notes on Speed and Quality

- **No GPU, no model**: Does not require downloading files or running extra installation steps. Narrative text uses structured templates, speech uses the voice engine of your operating system, and scene visuals use geometric SVG art.
- **No GPU, small model**: Runs lightweight neural weights directly on your CPU. Kokoro synthesises voice lines in near real-time, and the ONNX encoder groups related entities semantically without needing an external service.
- **GPU or local server**: Delivers full tabletop immersion with generative prose and diffusion portraits. Needs local server software such as Ollama or ComfyUI running on your network or local machine.
- **Cloud**: Offers the highest text complexity and rapid audio generation, but requires third-party API credentials and sends requests over the public internet.

## 2. Capability Tiers

LocalRPG classifies every registered provider into one of four capability tiers. The catalogue and settings panels display these badges so you always know how a provider runs:

- **`offline-basic`**: Deterministic and simple, and it runs entirely on your machine. Its output is more limited and repetitive than a model's.
- **`offline-neural`**: Runs a small model on your CPU, entirely on your machine. Quality is well below a large local or cloud model.
- **`local-server`**: Needs a server you run yourself. Local, but only offline while that server is.
- **`cloud`**: Sends your text to a remote provider and needs an API key. Metered in most cases.

## 3. One-Action Offline Preset

If you want a verified zero-network stack immediately, LocalRPG provides an automated offline preset bundle:

1. Open **Settings Studio** and choose the **Providers** tab.
2. Select **Offline preset** in the Offline Operations panel.
3. Review the affected providers and pick your speech synthesizer (`native-os` for zero downloads or `sherpa-onnx` for local neural voice).
4. Select **Apply preset** to configure the stack.

You can also apply this bundle from the terminal:

```bash
localrpg config offline-preset --tts native-os
```

To verify that your configuration does not make external network requests, choose **Check offline** in Settings Studio or run:

```bash
localrpg config check-offline
```

## 4. Self-Hosting Guides

To set up local servers for generative models, refer to the dedicated guides:

- [Local LLMs with Ollama and LM Studio](14-local-llm-ollama)
- [Local Voice Synthesis with Kokoro](15-local-tts-kokoro)
- [Voice Synthesis with Fish Audio](16-local-tts-fish-audio)
- [Speech Recognition with Faster-Whisper](17-local-stt-whisper)
- [Image Generation with ComfyUI and Automatic1111](18-local-image-comfyui)
- [Unified Local Stack with Docker Compose](19-local-stack-docker-compose)
- [Local Semantic Search with a Built-in Encoder](20-local-embeddings)
