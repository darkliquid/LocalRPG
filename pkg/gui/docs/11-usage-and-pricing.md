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
the **ledger key** the adapter records usage under, which is fixed by LocalRPG:

| Family | Ledger key |
| --- | --- |
| LLM | the adapter ID, e.g. `openaichat` or `gemini` |
| Speech (TTS) | `gemini:tts`, `builtin:<builtin_name>`, `cli:<command>`, or `http:<host>` |
| Transcription (STT) | `<builtin_name>`, else `<type>` |
| Image | `<builtin_name>`, else `<type>` |

An unpriced row in the Usage tab shows exactly the key to use, and the
[Provider & Model Catalogue](12-provider-catalogue) lists them all. Keys are
shared where adapters are shared: `provider: http` prices every HTTP image
endpoint, and `provider: gemini` covers both Gemini tokens (LLM) and Gemini
requests (image).

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
    - provider: openaichat
      per_million_input: 2500000   # 2.50 USD per 1M input tokens
      per_million_output: 10000000 # 10.00 USD per 1M output tokens

    # One specific model wins over the adapter-wide entry above.
    - provider: openaichat
      model: gpt-4o-mini
      per_million_input: 150000
      per_million_output: 600000

    # Speech is billed per character; the key is builtin:<name>.
    - provider: builtin:elevenlabs
      per_character: 30            # 0.00003 USD per character

    # Request-billed image endpoints share the http key.
    - provider: http
      per_request: 40000           # 0.04 USD per image
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

1. A config entry with the same provider **and** model.
2. A config entry with the same provider and no model.
3. A built-in default shipped in `pkg/pricing` (for the common presets).
4. Nothing, which yields a zero cost and the **no price configured** badge.

Costs are computed and stored at write time. Editing a price therefore changes
future turns only; existing rows keep the figure they were recorded with, so
historical totals do not drift.

## Notes

- A missing or zero price yields a zero cost. LocalRPG never fabricates a rate.
- Providers that report no usage metadata still get an **estimated** row where a
  count can be derived (for example characters spoken), so spend is at least
  visible.
- Local, unmetered providers (procedural art, native OS speech) have no price
  and need none.
