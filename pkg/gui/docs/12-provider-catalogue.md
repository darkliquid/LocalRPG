---
id: 12-provider-catalogue
title: Provider & Model Catalogue
category: Configuration & Providers
order: 7
description: Every registered provider ID, its presets and models, and the ledger key that prices match against.
---

# Provider & Model Catalogue

This page is generated from the provider registry, so it always lists the IDs
LocalRPG actually ships. Use it when writing a `providers.<id>` block, assigning
an agent role, or adding a `providers.prices` entry.

The **Ledger key** column is the exact `provider` value a metered call records
and therefore the value a price must use. It is also the value shown in the
Provider column of the Usage tab. A price with no `model` matches every model of
that key.

> [!NOTE]
> A provider ID such as `llm:openaichat` is the built-in adapter name, not the key
> you choose for `providers.<your-id>`. The ledger key is fixed by the adapter;
> the config key under `providers:` is yours to name.

## Ledger key rules

| Family | Ledger key | Example |
| --- | --- | --- |
| LLM | the adapter key | `llm:openaichat`, `llm:gemini` |
| Speech (TTS) | `tts:gemini`, `tts:<name>`, `tts:piper@<command>`, `tts:http@<host>` | `tts:http@localhost:8880` |
| Transcription (STT) | `stt:whisper-http@<host>`, `stt:whisper-cli@<command>` | `stt:whisper-http@localhost:8000` |
| Image | `image:gemini`, `image:http@<host>`, `image:cli@<command>` | `image:http@127.0.0.1:8188` |
| Embedding | `embedding:openai`, `embedding:gemini`, `embedding:builtin` | `embedding:gemini@default` |

## LLM providers

| Provider ID | Source | Ledger key | Presets and models |
| --- | --- | --- | --- |
| `llm:cli` | cli | `llm:cli@llama-cli` | `claude-cli`, `llama-cli` |
| `llm:gemini` | gemini | `llm:gemini` | `gemini-3.8-flash` |
| `llm:narrative-oracle` | builtin | `llm:narrative-oracle` | `narrative-oracle` |
| `llm:openaichat` | http | `llm:openaichat@localhost:11434` | `default`, `gpt-4`, `llama3.2` |

## Speech (TTS) providers

| Provider ID | Source | Ledger key | Presets and models |
| --- | --- | --- | --- |
| `tts:elevenlabs` | builtin | `tts:elevenlabs` | `eleven_multilingual_v2` |
| `tts:gemini` | gemini | `tts:gemini` | `gemini-2.5-flash-preview-tts`, `gemini-2.5-pro-preview-tts`, `gemini-3.1-flash-tts-preview`, `gemini-3.8-flash-lite-tts`, `gemini-3.8-flash-tts` |
| `tts:http` | http | `tts:http@localhost:8880` | `alltalk`, `kokoro`, `tts-1` |
| `tts:native-os` | builtin | `tts:native-os` | `native-os` |
| `tts:piper` | cli | `tts:piper@piper` | `piper` |
| `tts:sherpa-onnx` | builtin | `tts:sherpa-onnx` | `sherpa-onnx` |

## Transcription (STT) providers

| Provider ID | Source | Ledger key | Presets and models |
| --- | --- | --- | --- |
| `stt:web-speech` | builtin | `not reported` | `web-speech` |
| `stt:whisper-cli` | cli | `stt:whisper-cli@whisper-cli` | `whisper-cli` |
| `stt:whisper-http` | http | `stt:whisper-http@localhost:8000` | `whisper-1` |

## Image providers

| Provider ID | Source | Ledger key | Presets and models |
| --- | --- | --- | --- |
| `image:cli` | cli | `image:cli@sd` | `sd-cli` |
| `image:gemini` | gemini | `image:gemini` | `gemini-2.5-flash-image`, `gemini-3-pro-image`, `gemini-3.1-flash-image`, `gemini-3.1-flash-lite-image`, `imagen-3.0-fast-generate-001`, `imagen-3.0-generate-002` |
| `image:http` | http | `image:http@127.0.0.1:8188` | `automatic1111`, `comfyui`, `dall-e-3`, `stablediffusion` |
| `image:procedural-art` | builtin | `image:procedural-art` | `procedural-art` |

## Embedding providers

| Provider ID | Source | Ledger key | Presets and models |
| --- | --- | --- | --- |
| `embedding:gemini` | gemini | `embedding:gemini` | - |
| `embedding:openai` | http | `embedding:openai` | - |

## Built-in default prices

These rates apply when no `providers.prices` entry matches, and only when the
ledger key matches exactly. Add a config entry to override or extend them.

| Ledger key | Input (per 1M) | Output (per 1M) | Per character | Per request |
| --- | --- | --- | --- | --- |
| `llm:gemini` | 125000 | 500000 | 0 | 0 |
| `llm:openaichat` | 150000 | 600000 | 0 | 0 |

Prices are micros: one millionth of a currency unit, so `2500000` is 2.50.
