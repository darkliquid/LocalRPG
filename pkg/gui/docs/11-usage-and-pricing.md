---
id: 11-usage-and-pricing
title: Usage, Cost & Pricing
category: Configuration & Providers
order: 7
description: Reading the spend ledger and setting the provider prices that turn usage into a cost figure.
---

# Usage, Cost & Pricing

LocalRPG records every metered provider call in a **spend ledger** and, when a
price is known, converts that usage into a cost. The ledger lives in the Usage
tab of Global Settings and is stored per campaign, so a turn's spend is never
lost when a price changes later.

## The Usage Tab

Open **Global Settings** and select the **Usage** tab. You can switch between
the active campaign and a global view, and filter by provider or role. Each row
shows the turn, the role it was attributed to, the provider and model, what was
consumed (tokens, characters, or requests), and the cost.

Rows that consumed something a provider bills for, but for which LocalRPG has no
price, show a **no price configured** badge instead of a figure. That is not an
error: LocalRPG never guesses a rate. It means the usage was recorded and you
can turn it into a cost at any time by adding a price below.

## Which provider key?

The `provider` field is not the name you gave the block under `providers:`. It is
the **canonical key** the adapter records usage under, which is fixed by
LocalRPG. A key is `<family>:<adapter>`, and an adapter that names an endpoint or
command also has an instance form:

- adapter: `llm:openaichat`, `tts:http`, `image:gemini`, `embedding:gemini`
- instance: `tts:http@localhost:8880`, `stt:whisper-http@localhost:8000`

| Family | Key |
| --- | --- |
| LLM | `llm:openaichat`, `llm:gemini`, `llm:cli@<command>` |
| Speech (TTS) | `tts:gemini`, `tts:elevenlabs`, `tts:piper@<command>`, `tts:http@<host>` |
| Transcription (STT) | `stt:whisper-http@<host>`, `stt:whisper-cli@<command>` |
| Image | `image:gemini`, `image:http@<host>`, `image:cli@<command>` |
| Embedding | `embedding:openai`, `embedding:gemini`, `embedding:builtin` |

An unpriced row in the Usage tab shows exactly the key to use, and the
[Provider & Model Catalogue](12-provider-catalogue) lists them all.

### The fallback ladder

A recorded key is matched most specific first, stopping at the first match:

1. the instance key and model, e.g. `tts:http@localhost:8880` + `kokoro`
2. the instance key, e.g. `tts:http@localhost:8880`
3. the adapter key and model, e.g. `tts:http` + `kokoro`
4. the adapter key, e.g. `tts:http`

So a price on `tts:http` covers every HTTP speech endpoint, and a price on
`tts:http@hostA` overrides it for that endpoint only. The same ladder decides
which rate-limit block applies: a block on one endpoint does not stop another,
while a block on the adapter stops them all.

The field names below are also listed, with every other config key, in the
[Configuration Reference](13-configuration-reference).

## Setting a Price

Prices live in `config.yaml` under `providers.prices`. They override the
built-in table, and a price with no `model` matches every model of that
provider:

```yaml
providers:
  currency: USD
  prices:
    # An LLM adapter: token rates for every model of the adapter.
    - provider: llm:openaichat
      per_million_input: 2500000   # 2.50 USD per 1M input tokens
      per_million_output: 10000000 # 10.00 USD per 1M output tokens

    # One specific model wins over the adapter-wide entry above.
    - provider: llm:openaichat
      model: gpt-4o-mini
      per_million_input: 150000
      per_million_output: 600000

    # Speech is billed per character.
    - provider: tts:elevenlabs
      per_character: 30            # 0.00003 USD per character

    # Every request-billed HTTP image endpoint shares the adapter key.
    - provider: image:http
      per_request: 40000           # 0.04 USD per image

    # One endpoint overrides the adapter-wide image price.
    - provider: image:http@127.0.0.1:8188
      per_request: 0
```

### Price fields

| Field | Applies to | Meaning |
| --- | --- | --- |
| `per_million_input` | token-billed LLMs | charge per 1,000,000 input tokens |
| `per_million_output` | token-billed LLMs | charge per 1,000,000 output tokens |
| `per_character` | speech synthesis | charge per character spoken |
| `per_request` | image / request-billed | charge per call |

All values are in **micros**: one millionth of a currency unit. To express a
price of `2.50`, write `2500000` (2.50 x 1,000,000). The display currency is
`providers.currency` (default `USD`); no exchange-rate conversion is performed.

### Resolution order

For each usage row LocalRPG looks for a price in this order, stopping at the
first match:

1. A config entry with the same **instance key** and model.
2. A config entry with the same instance key and no model.
3. A config entry with the same **adapter key** and model.
4. A config entry with the same adapter key and no model.
5. A built-in default shipped in `pkg/pricing`, matched the same way.
6. Nothing, which yields a zero cost and the **no price configured** badge.

A `providers.prices` entry whose `provider` is not a canonical key is reported as
a configuration problem on load and matches nothing.

Costs are computed and stored at write time. Editing a price therefore changes
future turns only; existing rows keep the figure they were recorded with, so
historical totals do not drift.

## Notes

- A missing or zero price yields a zero cost. LocalRPG never fabricates a rate.
- Providers that report no usage metadata still get an **estimated** row where a
  count can be derived (for example characters spoken), so spend is at least
  visible.
- Local, unmetered providers (procedural art, native OS speech, a local model
  server) have no price and need none. Built-in rates are keyed to the vendor
  endpoint for exactly this reason: `tts:http@api.openai.com` is priced, while
  `tts:http@localhost:8880` is not.
- Some metered services ship no built-in rate because none is published, or none
  the ledger can express. Gemini speech has no published character rate, and
  transcription is billed per minute while the ledger records requests. Add a
  `providers.prices` entry if you know your own rate.
- The [Provider & Model Catalogue](12-provider-catalogue) lists the built-in
  rates with their models, so you can see what is priced before adding anything.
