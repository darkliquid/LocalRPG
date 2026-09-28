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
> Provider IDs such as `openaichat` are the built-in adapter names, not the keys
> you choose for `providers.<your-id>`. The ledger key is fixed by the adapter;
> the config key under `providers:` is yours to name.

## Ledger key rules

| Family | Ledger key | Example |
| --- | --- | --- |
| LLM | the provider ID, for adapters that report usage | `openaichat`, `gemini` |
| Speech (TTS) | `gemini:tts`, `builtin:<name>`, `cli:<command>`, `http:<host>` | `builtin:elevenlabs` |
| Transcription (STT) | `<builtin_name>`, else `<type>` | `http` |
| Image | `<builtin_name>`, else `<type>` | `gemini` |

## LLM providers

| Provider ID | Source | Ledger key | Presets and models |
| --- | --- | --- | --- |
| `cli` | cli | `not reported` | `claude-cli`, `llama-cli` |
| `gemini` | gemini | `gemini` | `gemini-3.8-flash` |
| `narrative-oracle` | builtin | `not reported` | `narrative-oracle` |
| `openaichat` | http | `openaichat` | `default`, `gpt-4`, `llama3.2` |

## Speech (TTS) providers

| Provider ID | Source | Ledger key | Presets and models |
| --- | --- | --- | --- |
| `tts-elevenlabs` | builtin | `builtin:elevenlabs` | `eleven_multilingual_v2` |
| `tts-gemini` | gemini | `gemini:tts` | `gemini-2.5-flash-preview-tts`, `gemini-2.5-pro-preview-tts`, `gemini-3.1-flash-tts-preview`, `gemini-3.8-flash-lite-tts`, `gemini-3.8-flash-tts` |
| `tts-native-os` | builtin | `builtin:native-os` | `native-os` |
| `tts-openai-http` | http | `http:localhost:8880` | `alltalk`, `kokoro`, `tts-1` |
| `tts-piper` | cli | `cli:piper` | `piper` |
| `tts-sherpa-onnx` | builtin | `builtin:sherpa-onnx` | `sherpa-onnx` |

## Transcription (STT) providers

| Provider ID | Source | Ledger key | Presets and models |
| --- | --- | --- | --- |
| `stt-webspeech` | builtin | `web-speech` | `web-speech` |
| `stt-whisper-cli` | cli | `cli` | `whisper-cli` |
| `stt-whisper-http` | http | `http` | `whisper-1` |

## Image providers

| Provider ID | Source | Ledger key | Presets and models |
| --- | --- | --- | --- |
| `image-cli` | cli | `cli` | `sd-cli` |
| `image-gemini` | gemini | `gemini` | `gemini-2.5-flash-image`, `gemini-3-pro-image`, `gemini-3.1-flash-image`, `gemini-3.1-flash-lite-image`, `imagen-3.0-fast-generate-001`, `imagen-3.0-generate-002` |
| `image-http` | http | `http` | `automatic1111`, `comfyui`, `dall-e-3`, `stablediffusion` |
| `image-procedural-art` | builtin | `procedural-art` | `procedural-art` |

## Built-in default prices

These rates apply when no `providers.prices` entry matches, and only when the
ledger key matches exactly. Add a config entry to override or extend them.

| Ledger key | Input (per 1M) | Output (per 1M) | Per character | Per request |
| --- | --- | --- | --- | --- |
| `gemini` | 125000 | 500000 | 0 | 0 |
| `openaichat` | 150000 | 600000 | 0 | 0 |
| `elevenlabs` | 0 | 0 | 0 | 0 |

Prices are micros: one millionth of a currency unit, so `2500000` is 2.50.
