# Provider Costs and Usage Reporting Research

**Date:** 2026-09-28
**Question:** Which metered providers can LocalRPG price from published rates, and does the Gemini embeddings API report anything billable that the ledger could record?

## Summary

- **Gemini embeddings return no token usage on the Gemini Developer API.** The
  `genai` Go SDK's `EmbedContentResponse` carries only `Embeddings`, an
  `SDKHTTPResponse`, and a `Metadata` field whose `BillableCharacterCount` is
  documented as "Gemini Enterprise Agent Platform only". So on the API this
  project calls (`genai.BackendGeminiAPI`) a request count is all that can be
  recorded; on Agent Platform the billable character count is available and is
  worth recording when present.
- **Gemini TTS has no published rate we can use.** The Gemini Developer API
  pricing page could not be fetched (see Gaps), and the Google Cloud page carries
  no native speech/TTS section.
- **Gemini embeddings cannot be priced with the current schema** even though a
  rate exists: Google publishes it per 1,000 *count* at $0.00015, which is 0.15
  micros per unit, and `config.PriceConfig.PerCharacter` is an integer micros
  field. Expressing it needs a per-million-character (or per-million-unit) field.
- Everything else in this note is a published list price that maps onto an
  existing `PriceConfig` field.

## Gemini text models

Source: Google Cloud "Agent Platform Pricing"
(<https://cloud.google.com/vertex-ai/generative-ai/pricing>), section "Gemini 3",
"Gemini 2.5", "Gemini 2.0". Prices are USD per 1M tokens, Standard tier, global
region.

| Model | Input | Output | Note |
| --- | --- | --- | --- |
| Gemini 3.8 Flash | $0.75 | $3.75 | introductory through 2026-12-31; $1.50 / $7.50 from 2027-01-01 |
| Gemini 3.5 Flash-Lite | $0.30 | $2.50 | |
| Gemini 3.1 Flash-Lite | $0.25 | $1.50 | |
| Gemini 2.5 Flash | $0.30 | $2.50 | |
| Gemini 2.5 Flash Lite | $0.10 | $0.40 | |
| Gemini 2.5 Pro | $1.25 | $10.00 | <=200K context; $2.50 / $15.00 above |
| Gemini 2.0 Flash | $0.15 | $0.60 | |
| Gemini 2.0 Flash Lite | $0.075 | $0.30 | |

`gemini-3.8-flash` is the model the `llm:gemini` preset ships, so its rate is the
one that matters most; the introductory rate is the one in force today.

## Gemini embeddings

Source: same page, section "Embedding costs". The column is printed as "Price /
1,000 count (USD)" and the unit is not further defined on the page.

| Model | Unit price per 1,000 | Derived per 1M |
| --- | --- | --- |
| Gemini Embedding (`gemini-embedding`) | $0.00015 | $0.15 |
| Embeddings for Text, excluding Gemini Embedding | $0.000025 | $0.025 |

`text-embedding-004` is not printed by that ID; the umbrella row covers the
`text-embedding-*` family.

## Gemini image models

Source: same page, section "Imagen", column "Price / 1 count (USD)".

| Model | Price per image |
| --- | --- |
| Imagen 4 Ultra | $0.06 |
| Imagen 4 | $0.04 |
| Imagen 4 Fast | $0.02 |
| Imagen 3 | $0.04 |
| Imagen 3 Fast | $0.02 |

The `image:gemini` presets ship `imagen-3.0-generate-002` and
`imagen-3.0-fast-generate-001`, so the $0.04 and $0.02 rows apply. No per-image
prices are published for the "nano-banana" / Gemini image models on either page
consulted.

## OpenAI

Source: <https://platform.openai.com/docs/pricing> (markdown form at
`/docs/pricing.md`). USD.

| Model | Rate |
| --- | --- |
| gpt-4o | $2.50 / $10.00 per 1M tokens (input / output) |
| gpt-4o-mini | $0.15 / $0.60 per 1M tokens |
| text-embedding-3-small | $0.02 per 1M tokens |
| text-embedding-3-large | $0.13 per 1M tokens |
| tts-1 | $15.00 per 1M characters |
| tts-1-hd | $30.00 per 1M characters |
| Whisper | $0.006 per minute (no per-token price printed) |

Not printed by that exact ID: `dall-e-3` (absent from the page entirely). Image
models that are listed are priced per 1M tokens (`gpt-image-1` at $10.00 image
input / $40.00 image output), which the ledger cannot express because it records
a request count, not image tokens.

Note on selection: the page also lists model families beyond anything this
project ships presets for. Only the rows above were used, because each
corresponds to a model a preset names.

## ElevenLabs

Sources: <https://elevenlabs.io/pricing> and <https://elevenlabs.io/pricing/api>.

The API page prints per-1K-character USD rates directly: **$0.08** for v4, v3, and
v2 Multilingual, and **$0.04** for v4 Turbo and Flash/Turbo, with promotional
discounts on top at the time of writing. The subscription page bills in credits
(1 character = 1 credit on the V2 Multilingual models), which derives to roughly
$0.165-$0.20 per 1K characters depending on tier.

The `tts:elevenlabs` preset ships `eleven_multilingual_v2`, so $0.08 per 1,000
characters = **80 micros per character**.

## What maps onto the current price schema

| Ledger key | Field | Value (micros) | Source row |
| --- | --- | --- | --- |
| `llm:gemini` + `gemini-3.8-flash` | per_million_input / output | 750000 / 3750000 | Gemini 3.8 Flash, introductory |
| `tts:elevenlabs` | per_character | 80 | ElevenLabs API, v2 Multilingual |
| `tts:http@api.openai.com` | per_character | 15 | OpenAI tts-1 |
| `image:gemini` + `imagen-3.0-generate-002` | per_request | 40000 | Imagen 3 |
| `image:gemini` + `imagen-3.0-fast-generate-001` | per_request | 20000 | Imagen 3 Fast |
| `embedding:openai` + `text-embedding-3-small` | per_million_input | 20000 | OpenAI embeddings |
| `embedding:openai` + `text-embedding-3-large` | per_million_input | 130000 | OpenAI embeddings |

Unpriceable with the current schema or unpublished:

- `embedding:gemini` — $0.15 per 1M count is 0.15 micros per unit, below the
  integer micros resolution of `PerCharacter`.
- Gemini TTS (`tts:gemini`) — no published rate found.
- Whisper transcription — billed per minute; the ledger has no duration field.
- Request-billed image endpoints and `dall-e-3` — no per-image rate published.
- Local and built-in adapters (`image:procedural-art`, `tts:native-os`,
  `tts:sherpa-onnx`, `llm:narrative-oracle`, the local embedding hash) — run
  locally and are not metered, so they must stay unpriced.

## Gaps and caveats

- **The Gemini Developer API pricing page could not be fetched.**
  `https://ai.google.dev/gemini-api/docs/pricing` redirected to a Google sign-in
  loop for the fetcher, so all Gemini figures here come from the Google Cloud
  page. The two pages can differ: they are different billing surfaces, and the
  Cloud page is titled "Agent Platform Pricing". Gemini TTS and the Gemini image
  models are the categories most likely to be priced only on the Developer API
  page.
- **Introductory pricing expires.** Gemini 3.8 Flash doubles on 2027-01-01, so a
  hard-coded rate for it goes stale on a known date.
- **Embedding unit ambiguity.** Google prints "count" for embeddings without
  defining it as characters or tokens; the code records requests for Gemini
  embeddings today, so a price cannot be applied regardless.
- **`ContentEmbedding.Statistics` and `EmbedContentMetadata.BillableCharacterCount`
  are Agent Platform only**, so a Developer API deployment reports nothing.

## Implementation implications

1. Record `EmbedContentMetadata.BillableCharacterCount` as `Usage.Characters`
   when the response carries it, and fall back to a request count marked
   `estimated` otherwise. This is the only usage Gemini embeddings can report.
2. Add the built-in prices in the table above, using per-model entries where the
   rate is per model and an instance key (`tts:http@api.openai.com`) where the
   adapter is shared and only one endpoint is metered.
3. Leave the unpublished categories unpriced; the Usage view already labels those
   rows, and inventing a rate would be worse than showing none.
