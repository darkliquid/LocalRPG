---
id: 14-migrating-provider-keys
title: Migrating Provider Keys
category: Configuration & Providers
order: 7
description: What changed in config version 2, how to update providers.prices, and what happens to old spend.
---

# Migrating Provider Keys

LocalRPG names every provider with one canonical key, `<family>:<adapter>`, and
prices are matched against that key. Earlier versions used several different
names for the same provider, which is why prices often appeared not to apply.

A configuration written before version 2 is reported as a problem when it loads,
and the log names any entry the loader rejected. Update the `provider` value of
each `providers.prices` entry using the table below.

## Old key to new key

| Old key | New key |
| --- | --- |
| `openaichat` | `llm:openaichat` |
| `gemini` (LLM) | `llm:gemini` |
| `gemini:tts` | `tts:gemini` |
| `builtin:elevenlabs` | `tts:elevenlabs` |
| `builtin:sherpa-onnx` | `tts:sherpa-onnx` |
| `builtin:native-os` | `tts:native-os` |
| `cli:piper` | `tts:piper@piper` |
| `http:<host>` (speech) | `tts:http@<host>` |
| `http` (transcription) | `stt:whisper-http@<host>` |
| `http` (image) | `image:http@<host>` |
| `gemini` (image) | `image:gemini` |
| `procedural-art` | `image:procedural-art` |

Speech, transcription, and image keys that name an endpoint include the host, so
`tts:http@localhost:8880` is the key for a server on that address. The
[Provider & Model Catalogue](12-provider-catalogue) lists every key, and the
Provider column of the Usage tab shows the key each row was recorded under.

## A worked example

```yaml
providers:
  prices:
    # Before: keys that never matched a recorded row.
    - provider: openaichat
      per_million_input: 2500000
    - provider: builtin:elevenlabs
      per_character: 30

    # After: the keys the ledger actually uses.
    - provider: llm:openaichat
      per_million_input: 2500000
    - provider: tts:elevenlabs
      per_character: 30
```

## Existing spend

Rows recorded before the change keep the key they were written with. That key is
no longer valid, so those rows show **no price configured** and are never
re-priced. They are history, and nothing needs to be done about them.

## What else changed

- Embedding calls are recorded now, under `embedding:openai`,
  `embedding:gemini`, or `embedding:builtin`.
- A provider configured with no registered adapter (an unnamed builtin, or an
  unknown type) records no usage rather than a guessed provider name.
- A price can target one endpoint, such as `tts:http@localhost:8880`, and still
  fall back to the adapter-wide `tts:http` price for every other endpoint. See
  [Usage, Cost & Pricing](11-usage-and-pricing) for the lookup ladder.
