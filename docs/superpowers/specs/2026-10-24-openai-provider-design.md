# OpenAI Provider Family Design

**Date:** 2026-10-24
**Status:** Proposed
**Bead:** LocalRPG-3lz
**Scope:** `pkg/provider`, `pkg/harness`, `pkg/media`, `pkg/config`, `pkg/pricing`, `frontend/src`, `pkg/gui/docs`
**Research:** API surface verified against official OpenAI documentation on 2026-10-24; sources in section 12.

## 1. Problem

OpenAI is reachable today, but only through generic adapters that hide what the platform offers:

| Capability | How it works today | What is wrong with it |
|---|---|---|
| Chat | `type: http` maps to `llm:openaichat` (`pkg/provider/openaichat`) | Chat Completions wire format only; no model catalogue or OpenAI presets; tool calling with reasoning models is unavailable at `reasoning_effort` beyond `none` |
| TTS | `type: http` → `tts:http` (`pkg/provider/ttshttp`) | No voice catalogue, no `instructions` (tone/accent/emotion control), no model choice surfaced |
| STT | `stt:whisper-http` ships an OpenAI preset | Works; preset still defaults to `whisper-1` (shutdown 2027-02-26) |
| Image | `image:http` parses OpenAI `b64_json` responses | No model presets, no size/quality mapping (`aspect_ratio` is a Gemini-shaped knob) |
| Embeddings | `embedding:openai` (`pkg/provider/openaiembedding`) | Already native; nothing to build |

Also, the belief that Anthropic offers image generation, TTS, or embeddings is incorrect; that
correction is recorded in the Anthropic spec (`2026-10-24-anthropic-provider-design.md`), not here.

## 2. What OpenAI offers (verified 2026-10-24)

| Capability | Models | Endpoint / wire | Notes |
|---|---|---|---|
| Chat | `gpt-6-astra` ($10/$50 per MTok), `gpt-6.1-sol` ($2/$10), `gpt-6-luna` ($0.1/$0.5), `gpt-5.6-*`, `gpt-5.5` | `POST /v1/responses` (**recommended**) or `/v1/chat/completions` (still supported) | 1.05M context; from GPT-5.4 onward, Chat Completions does not support tool calling at `reasoning_effort` above `none`; the codex model line (`gpt-5.3-codex` et al.) is deprecated, and the Codex agent now invokes mainline model IDs |
| Image generation | `gpt-image-2.5-sunburst`, `gpt-image-2.5-flare`, `gpt-image-2` | `POST /v1/images/generations` (b64 response); also a Responses `image_generation` tool with `partial_images` streaming | Sizes `1024x1024`, `1536x1024`, `1024x1536` plus custom `WxH` (multiples of 16, max edge 3840); quality `low/medium/high/xhigh/max/auto`; transparent background |
| TTS | `gpt-4o-mini-tts` (plus legacy `tts-1`) | `POST /v1/audio/speech` | 13 named voices (`alloy`, `ash`, `ballad`, `coral`, `echo`, `fable`, `nova`, `onyx`, `sage`, `shimmer`, `verse`, `marin`, `cedar`); `instructions` for tone; `speed`; `mp3/opus/aac/flac/wav/pcm`; chunked streaming. The endpoint is deprecated with shutdown 2027-01-06, migrating to the Realtime API (`gpt-realtime-2.1-mini` is not a `/v1/audio/speech` model) |
| STT | `gpt-transcribe`, `gpt-live-transcribe` | `POST /v1/audio/transcriptions` (multipart, ≤25 MB) | `whisper-1` and the `gpt-4o-transcribe` family shut down 2027-02-26 |
| Embeddings | `text-embedding-3-small` (1536-d), `text-embedding-3-large` (3072-d) | `POST /v1/embeddings` | `dimensions` truncation supported; no newer model exists |
| Auth | (not a model) | `Authorization: Bearer $OPENAI_API_KEY` | Rate-limited per org/project, tiered (RPM/TPM/IPM) |

Sign in with ChatGPT (subscription auth, "openai/codex" in other tools) is an officially documented
OAuth 2.0 code+PKCE flow with a loopback redirect and dynamic client registration; it routes through
the normal `/v1/responses` endpoint with `store: false` and `stream: true` mandatory. It is scoped
out of this spec (section 3) and tracked as a possible spike.

## 3. Non-goals

- **Realtime API audio.** Speech synthesis and realtime transcription move to WebSocket sessions.
  That is a session-oriented client of a different shape than `Synthesize(ctx, text) ([]byte, error)`.
  The `/v1/audio/speech` route ships here with the deprecation date recorded; a Realtime TTS adapter
  is a follow-up bead, decided when OpenAI publishes the migrated non-realtime path (or the date forces one).
- **Sign in with ChatGPT subscription auth.** LocalRPG is open source, which today qualifies for the
  documented flow, but it adds OAuth, PKCE, loopback redirect, token refresh and per-user consent to
  the provider surface. It deserves a separate spike bead; the adapter here takes standard API keys
  (`api_key: "env:OPENAI_API_KEY"`), and `store: false` is already the adapter's default so the
  Responses requests stay SIWC-compatible if the spike lands.
- **Image edits and portrait character consistency.** The cross-provider `ImageClient` interface is
  prompt-only; `/v1/images/edits` needs an input image and mask. Layered/scene-aware output stays
  with `SceneHintProvider` implementations (Gemini shapes).
- **Replacing `llm:openaichat`.** It stays the adapter for OpenAI-compatible local servers
  (Ollama, LM Studio, LocalAI, vLLM presets), which do not implement `/v1/responses`.
- **Responses `image_generation` tool / server-side tools (web search, file search, code interpreter).**
  The engine's tool loop is local (`pkg/tools`); remote hosted tools are a separate decision.

## 4. Design

### 4.1 Family layout

One provider identity, four adapters. `type: openai` selects the dedicated key per family:

| Config family | New key | New package | Config trigger |
|---|---|---|---|
| LLM | `llm:openai` | `pkg/provider/openairesponses` | `agents.roles.<role>.type: openai` |
| TTS | `tts:openai` | `pkg/provider/ttsopenai` | `media.tts.type: openai` |
| Image | `image:openai` | `pkg/provider/imageopenai` | `media.image.type: openai` |
| STT | `stt:whisper-http` (existing) | none | `media.stt.type: http` with the OpenAI preset |
| Embeddings | `embedding:openai` (existing) | none | `embeddings.provider: openai` (name already resolves to it) |

New candidates registered in `pkg/provider/keys.go` and `AllKeys()`, blank-imported by
`pkg/provider/all/all.go` so `provider.List()` serves the catalogue. `harness.KeyFor` (`pkg/harness/exports.go`)
and the `NewModelProvider` switch (`pkg/harness/factory.go`) gain the `openai` case for LLM; the media
resolvers gain it in `pkg/media/exports.go` (`TTSKeyFor`, `ImageKeyFor`).

### 4.2 LLM adapter (`openairesponses`)

`POST {endpoint}/v1/responses` (default endpoint `https://api.openai.com/v1`); headers
`Authorization: Bearer <key>`; SSE `data:` lines only (typed events carry their name in `data.type`).
`store: false` and `stream: true` on every request. One request per engine call, matching `openaichat`.

| LocalRPG (`GenerateRequest`) | Responses API |
|---|---|
| `System` | top-level `instructions` |
| `Messages` role `user` | input item `{type:"message", role:"user", content:[{type:"input_text", text}]}` |
| `Messages` role `assistant` + `ToolCalls` | items `{type:"message", role:"assistant", content:[...]}` plus `{type:"function_call", call_id, name, arguments}` per call |
| `Messages` role `tool` | `{type:"function_call_output", call_id, output}` |
| `Tools` | `tools: [{type:"function", name, description, parameters, strict: false}]` |
| `ToolChoice "required"` | `tool_choice: "required"`; `"none"` drops the tools array; `""` omits the key (auto) |
| `MaxTokens` | `max_output_tokens` |
| `Temperature` | `temperature`, **omitted whenever `effort` is set** (reasoning models reject it) |
| `ProviderConfig.Effort` (new, shared field) | `reasoning: {effort: <low|medium|high|...>}` |
| `Prompt` (string-only callers) | single input item, as `openaichat` does today |

Streaming event mapping:

| Event | Chunk behaviour |
|---|---|
| `response.output_text.delta` | `StreamChunk{Text: delta}` |
| `response.output_item.added/done` (type `function_call`) | accumulate by item `id` (arguments carry `delta` in the arguments stream) |
| `response.completed` | final chunk `StreamChunk{Done: true, Usage}`; check `response.status` (`incomplete`, `failed` surface as errors with the truncated body, `provider.MaxProviderDetailBytes`) |
| `error` event | `StreamChunk{Error}` |
| anything else (`response.created`, in-progress, content-part events) | ignored |

Usage: `usage.input_tokens` / `output_tokens` → `harness.Usage`; `input_tokens_details.cached_tokens`
→ `GenerateResponse.CachedTokens`.

### 4.3 TTS adapter (`ttsopenai`)

`POST /v1/audio/speech` with `{model, input, voice, speed, instructions?, response_format}`. `opus`
is requested so the pipeline's normalise step is a pass-through; `Characters` and per-character
pricing match `tts:http@api.openai.com` rows.

- **VoiceCatalog** (the September P1 interface, already shipped with options/caching): static list of
  the 13 named voices with `recommended` tags on `marin`/`cedar`, so `harness.AssignVoiceProfile`
  scores them like any other catalogue.
- **VoiceOptions**: `instructions` (string, free text on tone and accent), `model` (reserved key: enum
  `gpt-4o-mini-tts`), `format`/`speed` passthrough. Voice-profile `options` already live in the audio
  cache key (`ComputeAudioCacheKeyForVoice`), so re-tuning never collides.
- `deprecation` caveat on the descriptor: endpoint shutdown 2027-01-06.
- `metered: true` presets; `grouping: auto` applies (OpenAI has no native multi-speaker clip).

### 4.4 Image adapter (`imageopenai`)

`POST /v1/images/generations` `{model, prompt, size, quality, n: 1, output_format}`; response
`data[0].b64_json` (the parsing shape `image:http` already handles). Mapping: `ImageConfig.AspectRatio`
1:1 → `1024x1024`, 3:2 → `1536x1024`, 2:3 → `1024x1536`; a new `media.image.quality` field
(`low|medium|high|auto`, default `auto`) maps to the `quality` parameter. `person_generation` is a
Gemini concept and reads as a no-op here (documented in the config reference). Pricing rows for
`gpt-image-2.5-*` are added at implementation time only if the current price page can be verified
against the API's billing unit; until then the model stays unpriced like the table's local adapters,
with the usual `providers.prices` override available.

### 4.5 Auth and shared key

- `ProvidersConfig.OpenAI` (`pkg/config/types.go`, struct shape of `InworldProviderConfig`) holds
  `providers.openai.api_key`.
- `sharedKeyFor` (`pkg/harness/factory.go:99`) and `SharedProviderKey` (`pkg/media/exports.go:64`)
  resolve `providers.openai.api_key` when a role/config has no key of its own.
- The `env:` / `keyfile:` prefix convention the config parser already implements is what the presets use
  (`api_key: "env:OPENAI_API_KEY"`); no default key is invented.

### 4.6 Catalogue, presets, pricing, GUI

- Descriptor presets (disabled by default, like ElevenLabs/by design): `openai` for LLM
  (models `gpt-6.1-sol` recommended, `gpt-6-luna` budget, `gpt-6-astra` max), `tts:openai`,
  `image:openai`, plus the STT preset (`gpt-transcribe`) on `sttwhisperhttp`.
- `pkg/pricing/pricing.go` rows, endpoint-scoped like the existing OpenAI rows: `llm:openai` per-model
  rows captured from the model catalogue on 2026-10-24, `tts:openai` per character.
- Frontend: `types.ts` provider-type unions gain `"openai"`; `LLMRoleEditor.tsx`,
  `TTSProviderEditor.tsx`, `STTProviderEditor.tsx`, `ImageProviderEditor.tsx` option lists; the DTO
  contract test regenerates `pkg/gui/testdata/dto.json` (`go test ./pkg/gui -update-dto`).
- The provider catalogue endpoint, `12-provider-catalogue.md` and the config reference regenerate
  from the registry (`go test ./pkg/gui -update-docs`); `05-providers.md` gains the OpenAI prose note.

## 5. Behaviour

- An OpenAI role streams a turn the same way an `http` role does; the player sees no difference beyond
  which name the config carries.
- Reasoning-effort setting is one knob: `effort` on the LLM role config, rolled via `reasoning.effort`.
- Tool calling works with reasoning models, which Chat Completions cannot carry above
  `reasoning_effort: none`; the tool loop rides Responses.
- TTS: a character voice can pin `instructions` ("breathy, elderly, faintly amused") through
  `VoiceProfile.Options` without a LocalRPG schema change.
- Cost screenshots: usage records carry tokens and characters; keyed rates flow into the existing
  usage UI unchanged.

## 6. File Map

```text
pkg/provider/openairesponses/{openairesponses.go,client.go}   new
pkg/provider/ttsopenai/{ttsopenai.go,voices.go}               new
pkg/provider/imageopenai/imageopenai.go                       new
pkg/provider/keys.go                  +keys, +AllKeys
pkg/provider/all/all.go               +blank imports
pkg/harness/exports.go                +KeyFor case "openai"
pkg/harness/factory.go                +type clause, +sharedKeyFor
pkg/harness/types.go                  +ProviderConfig.Effort
pkg/media/exports.go                  +TTSKeyFor / ImageKeyFor / SharedProviderKey
pkg/config/types.go                   +ProvidersConfig.OpenAI, +ImageConfig.Quality
pkg/pricing/pricing.go                +rows
pkg/provider/openaiembedding/openai.go +presets (models)
pkg/config/offline.go                 embeddingKey passthrough unchanged (openai already known)
frontend/src/types.ts                 +provider type unions
frontend/src/components/providers/*.tsx option lists
pkg/gui/testdata/dto.json             regenerate (-update-dto)
pkg/gui/docs/12-provider-catalogue.md regenerate (-update-docs)
pkg/gui/docs/05-providers.md          prose
```

## 7. Testing

- httptest fake OpenAI server per adapter (the `openaichat` test pattern): request-body shape
  (`store:false`, `instructions`, `reasoning.effort`, `tool_choice:"required"`), SSE fixtures for
  text and tool-call rounds, `b64_json` images, speech bytes, embedding JSON already covered.
- The temperature-omission rule, keyed rejection of the reasoning-model/temperature combination.
- Voice catalogue stability (13 voices, reserved keys) and the cache-key participation of `instructions`.
- `provider.Validate()` + catalogue test; DTO contract test; `go test ./pkg/gui -update-docs`.
- `mise run test:backend:race` for the stream consumers; `mise run lint` at the end.

## 8. Rollout

One pull request per adapter in this order: LLM (unblocks role work), TTS, image, then presets and
pricing. Each PR regenerates its catalogue artefacts. The bead stays open across them and records what
landed; the SIWC spike and Realtime TTS follow-up beads are created only when this family's first PR is
approved.

## 9. Risks

- **Model churn is severe.** GPT-5.x/o-series shut down between 2026-12-11 and 2027-01; image shifts
  fast too (`gpt-image-1` shut down 2026-10-23). Model IDs are user-configurable so a preset going stale
  is a preset refresh, not an adapter patch. Re-verify every price row at implementation.
- **TTS endpoint expiry.** `/v1/audio/speech` stops 2027-01-06. The `metered` presets already require
  an explicit opt-in; drop or migrate before the date, never after it breaks silently mid-session.
- **Rate-limit tiers.** 429s surface as normal provider errors (`x-ratelimit-remaining-*` and
  `Retry-After` pass through the error mapper); there is no second-tier retry policy.
- **Responses API field drift.** The adapter pins no version header (none exists on this API), so the
  fake-server tests are the contract: when OpenAI changes a field, the tests say so before players see it.

## 10. Sources

- Model catalog and deprecations: `https://developers.openai.com/api/docs/models`, `.../api/docs/deprecations`
- Responses guide and migration: `https://developers.openai.com/api/docs/guides/text`, `.../api/docs/guides/migrate-to-responses`, `.../api/docs/guides/streaming-responses`
- Image generation: `https://developers.openai.com/api/docs/guides/image-generation`
- TTS: `https://developers.openai.com/api/docs/guides/text-to-speech`
- STT: `https://developers.openai.com/api/docs/guides/speech-to-text`
- Embeddings: `https://developers.openai.com/api/docs/guides/embeddings`
- Function calling: `https://developers.openai.com/api/docs/guides/function-calling`
- Rate limits and auth: `https://developers.openai.com/api/docs/guides/rate-limits`, `.../api/reference/overview#authentication`
- SIWC (deferred): `https://developers.openai.com/siwc/token-sharing-open-source/`, `.../sign-in`, `.../models-and-inference`
