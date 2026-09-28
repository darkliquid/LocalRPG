# Provider keys are now `<family>:<adapter>`

LocalRPG identifies every provider with one canonical key. `providers.prices`
entries must use the new form. An entry with an old key is reported as a
configuration problem when the config loads and matches nothing.

Prices are also matched most-specific-first, so an instance key such as
`tts:http@localhost:8880` can override an adapter-wide `tts:http` price. The same
rule applies to rate-limit and funds blocks.

## Migrating a price

Replace the `provider` value in each `providers.prices` entry using the table
below, then check the log for `config.problem` entries, which name any key the
loader rejected.

| Old key | New key |
| --- | --- |
| `openaichat` | `llm:openaichat` |
| `gemini` (LLM) | `llm:gemini` |
| `gemini:tts` | `tts:gemini` |
| `builtin:elevenlabs` | `tts:elevenlabs` |
| `builtin:sherpa-onnx` | `tts:sherpa-onnx` |
| `builtin:native-os` | `tts:native-os` |
| `cli:piper` | `tts:piper@piper` |
| `http:<host>` (TTS) | `tts:http@<host>` |
| `http` (STT) | `stt:whisper-http@<host>` |
| `http` (image) | `image:http@<host>` |
| `gemini` (image) | `image:gemini` |
| `procedural-art` | `image:procedural-art` |

Speech, transcription, and image keys that name an endpoint include the host, so
`tts:http@localhost:8880` is the key for a server on that address. The
[Provider & Model Catalogue](../../pkg/gui/docs/12-provider-catalogue.md) lists
every key, and the Usage tab shows the key on each row.

## What happens to existing spend

Historical rows keep the key they were recorded with. Because that key is no
longer valid, those rows show **no price configured** and are never re-priced.
Add nothing to fix them; they are history.

## Other changes

- Embedding calls are now recorded, under `embedding:openai`, `embedding:gemini`,
  or `embedding:builtin`.
- A provider configured with no registered adapter (an unnamed builtin, or an
  unknown type) records no usage rather than a guessed provider name.
