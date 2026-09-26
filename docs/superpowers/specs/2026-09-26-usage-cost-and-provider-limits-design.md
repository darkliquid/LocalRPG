# Usage, Cost, and Provider Limits Design

**Date:** 2026-09-26
**Status:** Proposed
**Scope:** Per-provider usage and cost accounting, a per-campaign ledger with a global aggregate, a rate-limit / insufficient-funds failure taxonomy, a provider+role block, and the Usage UI
**Related:** `pkg/harness` (router, failures), `pkg/provider`, `pkg/media`, `pkg/storage`, `pkg/gui`, `pkg/config`, `frontend/`

## 1. Overview & Goals

Metered providers are billed, but the application has no idea what a turn costs.
`metered` is only a per-provider boolean (`pkg/config/types.go:150-152`;
`pkg/media/catalog.go:51-53`), no provider reports usage to a common sink (only
`geminillm` parses a usage field, `pkg/provider/geminillm/provider.go:211`), and
rate limits and funding failures are handled ad hoc by string-matching inside
individual providers (`pkg/provider/geminillm/provider.go:588`,
`pkg/provider/ttsgemini/client.go:254`, `pkg/provider/ttselevenlabs/client.go:184-214`).
`FailureCode` has no rate-limit or funds case (`pkg/harness/failure.go:16-22`),
so a 429 and a broken endpoint look the same to the user.

This specification adds usage and cost accounting broken down by provider,
campaign, and turn; distinguishes rate limits from insufficient funds; blocks a
provider+role for the length of a server-advertised backoff; and surfaces all of
it in the UI.

**Goals:**

- Record every provider call's usage (tokens, characters, requests) into a
  per-campaign ledger keyed by turn, role, provider, and model.
- Compute cost at write time from a built-in price table with user overrides, so
  historical cost does not drift when prices change.
- Show spend by provider, campaign, and turn, plus a global aggregate.
- Add `rate_limited` and `insufficient_funds` failure codes, carrying the
  server's `Retry-After` when present.
- Block new work that would use a rate-limited provider+role until the deadline,
  lifting the block immediately when that role's provider changes.
- Never block on insufficient funds; make those failures unmistakable and clear
  them as soon as a later call succeeds.

**Non-Goals:**

- Estimating provider billing exactly for providers that expose no usage. Usage
  is reported where available and counted otherwise; cost is shown only when a
  price is known.
- Retrying the user's work automatically after a rate limit; the user resumes.
- Currency conversion or locale formatting beyond the configured currency.
- Per-token price discovery from provider APIs (prices are configured or built
  in, not fetched).

**Success Criteria:**

- After a turn on a metered provider, the Usage view shows that turn's provider,
  model, tokens/characters, requests, and cost.
- A 429 with `Retry-After: 30` blocks new LLM work on that provider for ~30 s;
  TTS and image on the same vendor keep working; changing the LLM provider
  clears the block at once.
- An insufficient-funds failure is labelled as such in the turn error surface,
  is not treated as retryable, and does not block anything.
- A provider that reports no usage still contributes request counts, and its
  rows show no cost rather than a wrong one.
- Existing campaigns migrate their index.db forward without a rebuild.

## 2. Investigation Findings

- No usage sink. `StreamChunk` and `GenerateResponse` carry no usage
  (`pkg/harness/types.go:8-15`); `ModelProvider` is
  `ID/Generate/Stream` (`pkg/harness/types.go:107-111`).
- The Router is the natural choke point: it knows the role and provider
  (`pkg/harness/router.go:94,138,185`).
- `GenerationFailure` carries a code, message, attempts, and elapsed time but no
  retry hint (`pkg/harness/failure.go:29-45,66-106`).
- Providers already import `pkg/harness`, so shared usage and error types can
  live there without a cycle. `pkg/media` does not import harness and defines its
  own optional capability interfaces (`MeteredProvider`, `pkg/media/catalog.go:51-53`).
- Storage migrations are ordered and idempotent, currently at version 7
  (`pkg/storage/migrate.go:14-24`); a new table is a version 8 step.
- Settings Studio has tabs `paths/providers/agents/media/preferences/debug`
  (`frontend/src/components/SettingsStudio.tsx:108,354-399`); a `usage` tab fits.

## 3. Design

### 3.1 Usage reporting

Add a shared usage value and an optional reporting surface.

```go
// pkg/harness
type Usage struct {
	Provider     string `json:"provider"`
	Model        string `json:"model,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Characters   int    `json:"characters,omitempty"`
	Requests     int    `json:"requests,omitempty"`
	// Estimated marks a value derived from request shape rather than reported by
	// the provider, so the UI can qualify it.
	Estimated bool `json:"estimated,omitempty"`
}
```

- `StreamChunk` gains `Usage *Usage`, set on the final chunk by providers that
  report it (additive, so providers that do not are unaffected).
- `GenerateResponse` gains `Usage *Usage`.
- `pkg/media` gains an optional `UsageReporter` (`LastUsage() Usage`) on TTS,
  STT, and image clients, mirroring how `MeteredProvider` is discovered
  (`pkg/media/capabilities.go:33-34`). Providers that cannot report set
  `Requests: 1` and `Estimated: true`; TTS sets `Characters` from the spoken
  text.

The Router records LLM usage after every call, tagging the role
(`pkg/harness/router.go`). Recording is best-effort and never fails a turn.

#### 3.1.1 What each provider can actually report

Researched against the provider implementations and their upstream API
reference docs. "Parse" is what this spec must add; today none of these fields
are read.

| Provider | Call | Reportable | Source field | Notes |
|---|---|---|---|---|
| `openaichat` (LLM) | stream | Yes (tokens) | final SSE chunk `usage`, needs `stream_options.include_usage` | Add the flag; the final chunk may be lost if the stream is cancelled. |
| `geminillm` (LLM) | stream + non-stream | Yes (tokens) | `UsageMetadata` (`promptTokenCount`, `candidatesTokenCount`, `cachedContentTokenCount`) | Non-stream genai path and the interactions REST path already read cached tokens only; streaming carries metadata on the **final** chunk. |
| `clillm` (LLM) | stream + non-stream | No | — | Local subprocess; record `Requests: 1`, estimate tokens from prompt/output length. |
| `narrative-oracle` (LLM builtin) | both | No | — | Estimate from text length. |
| mock/echo (LLM) | both | No | — | Estimate. |
| `ttsgemini` | non-stream | Yes (tokens) | `GenerateContentResponse.UsageMetadata` | TTS is token-billed (audio tokens); `ResponseModalities:["AUDIO"]` still returns metadata. |
| `ttselevenlabs` | non-stream | Yes (characters) | response header `character-cost` (+ `request-id`); account usage via `GET /v1/user/subscription` | The header is the per-request meter; the subscription endpoint gives remaining quota for a panel. |
| `ttshttp` | non-stream | No | — | Estimate characters from the spoken text. |
| `ttsnativeos`, `ttssherpa` | non-stream | No | — | Local; estimate characters. |
| `imagegemini` (Imagen) | non-stream | No | — | Imagen returns no usage; estimate from image count/size. `gemini-*-image` (Nano Banana) responses do carry `usageMetadata` through `generateContent`. |
| `imagehttp` | non-stream | Partial | OpenAI-compatible image responses may carry `usage` for `gpt-image-*` | A1111/ComfyUI carry none; read `usage` when present, else one request. |
| `sttwhisperhttp` | non-stream | Yes (duration or tokens) | response `usage` (`{type:"duration",seconds}` for `whisper-1`, tokens otherwise) | Requires requesting a JSON body with usage (e.g. `response_format=verbose_json` for `whisper-1`); currently sends no `response_format`. |
| `geminiembedding` | non-stream | No | — | No usage read; count requests. |
| `openaiembedding` | non-stream | Yes (tokens) | response `usage.prompt_tokens` | Currently ignored. |

Consequences for the design:

- Every LLM provider can contribute at least request counts and, where the API
  supports it, tokens; only `clillm` and the in-process builtins cannot, and they
  are the ones we mark `Estimated`.
- TTS usage is characters for ElevenLabs and tokens for Gemini; the `Usage`
  value carries both, and pricing resolves whichever the provider reports.
- Imagen has no usage at all, so its cost is estimated per generated image
  (documented as an estimate in the UI).
- Whisper usage needs a request change (a response format that returns it), so
  the STT task includes that.

### 3.2 Pricing

New package `pkg/pricing`:

```go
type Price struct {
	// PerMillionInput/Output are used for token-billed models.
	PerMillionInput  Micros
	PerMillionOutput Micros
	// PerCharacter and PerRequest cover speech and image providers.
	PerCharacter Micros
	PerRequest   Micros
}

// CostMicros returns the cost of one usage record. A zero price yields zero.
func CostMicros(u harness.Usage, p Price) Micros
```

`Micros` is an integer (1e-6 currency units) so costs are exact and sortable.
Built-in defaults cover the models LocalRPG ships presets for; config overrides
win:

```yaml
providers:
  prices:
    - provider: builtin:gemini
      model: gemini-2.5-pro
      per_million_input: 1250000    # 1.25 per 1M, in micros
      per_million_output: 10000000
    - provider: builtin:elevenlabs
      per_character: 30
  currency: USD
```

A price with no model matches every model of that provider. An unknown provider
yields a zero price and therefore no cost, never a guess.

### 3.3 Ledger

A usage row is written for every recorded call, through a storage migration
(version 8).

```sql
CREATE TABLE usage_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  turn_number INTEGER NOT NULL DEFAULT 0,   -- 0 for work outside a turn (previews)
  role TEXT NOT NULL,                        -- gm | extractor | completion | tts | stt | image
  provider TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  characters INTEGER NOT NULL DEFAULT 0,
  requests INTEGER NOT NULL DEFAULT 0,
  estimated INTEGER NOT NULL DEFAULT 0,
  cost_micros INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_usage_turn ON usage_records(turn_number);
CREATE INDEX idx_usage_provider ON usage_records(provider, role);
```

Store methods `SaveUsage(record)` and queries `UsageByTurn(turn)`,
`UsageSummary(by provider|role|turn)`, `UsageTotal()`. The global view is a
**sum** across campaign databases: it opens each `index.db` read-only and adds
the rows, cached in memory with a short TTL because it is off the turn path. The
global view is a total plus a per-campaign drilldown (campaign, then turn),
never a second stored ledger.

Rows are written with cost already computed, so a later price change does not
rewrite history.

### 3.4 Failure taxonomy

Add two codes (`pkg/harness/failure.go`):

```go
FailureRateLimited      FailureCode = "rate_limited"
FailureInsufficientFunds FailureCode = "insufficient_funds"
```

`GenerationFailure` gains `RetryAfterMS int64` (0 when unknown) and `Role`,
`Provider` (already present in `Attempt`).

Providers raise typed errors, and classification maps both the type and the
existing string markers, so behaviour is uniform while provider wording stays
free:

```go
// pkg/provider (imported by every provider, imports nothing from harness)
type RateLimitedError struct { RetryAfter time.Duration; Message string }
type InsufficientFundsError struct { Message string }
```

`harness.ClassifyProviderError` maps `RateLimitedError` → `FailureRateLimited`,
`InsufficientFundsError` → `FailureInsufficientFunds`, and keeps the current
marker scan as a fallback for providers that return plain errors (the existing
"quota exceeded or rate limit" and "RESOURCE_EXHAUSTED" strings,
`geminillm/provider.go:588`, `ttsgemini/client.go:254`,
`ttselevenlabs/client.go:256`; `ttselevenlabs`' `Retry-After` parsing at
`client.go:212-218` is the model for the typed error).

Distinguishing the two is provider-specific, and the upstream docs give a
reliable signal:

- ElevenLabs returns HTTP **429** with `type: rate_limit_error` (codes
  `rate_limit_exceeded`, `concurrent_limit_exceeded`, `system_busy`) and HTTP
  **402** with `type: payment_required`, `code: insufficient_credits` for no
  funds. `402`/`payment_required`/`insufficient_credits` map to
  `FailureInsufficientFunds`; `429` maps to `FailureRateLimited`.
- OpenAI uses `429` for rate limits and for insufficient quota; the error body
  (`code`/`type`) is the discriminator, with `insufficient_quota` meaning funds.
- Gemini `RESOURCE_EXHAUSTED` is ambiguous and is treated as a rate limit
  (retryable) unless the body names billing/quota; guessing "out of funds"
  wrongly would wrongly reassure.

`Retry-After` is **not guaranteed**: ElevenLabs documents none (it prescribes
exponential backoff), and OpenAI sends rate-limit header hints rather than a
documented `Retry-After` for every case. So the block uses the header when
present and otherwise a default window.

### 3.5 Provider + role block

A small in-process registry:

```go
// pkg/harness (or pkg/limits)
type LimitRegistry struct { /* keyed by provider key + role */ }
func (r *LimitRegistry) Block(providerKey, role string, until time.Time)
func (r *LimitRegistry) Blocked(providerKey, role string) (time.Time, bool)
func (r *LimitRegistry) Clear(providerKey, role string)
```

Key is the provider identity from `media.ProviderKey` for media and the router's
provider ID for LLMs. Rules:

- A `FailureRateLimited` with `RetryAfterMS > 0` blocks that provider+role until
  the deadline. With no header, a default window applies (30 s, or a longer
  window derived from the provider's own hint headers when it sends them).
- A `concurrent_limit_exceeded` (ElevenLabs) is a rate limit, not a funds
  failure, and blocks the same way.
- The block is consulted before a turn starts, before a TTS/STT/image call, and
  before a preview or audition. A blocked request returns a typed
  `ErrRateLimitedUntil{Until}` that the API maps to HTTP 429 with `Retry-After`.
- Changing the provider for that role clears the block immediately. This is
  detected by re-reading the role's provider identity on each check (and, once
  the config-revision work lands, by revision).
- Blocks are in-memory only; a restart is a fresh chance and rate-limit windows
  are short.
- Insufficient funds never blocks: it records a `lastFundsFailure` for the
  provider+role, surfaced in the UI and cleared by the next success.

### 3.6 Attribution

- LLM usage is recorded with the turn number and role (`gm`, `extractor`,
  `completion`) by the orchestrator, which already owns the turn number.
- TTS/STT/image usage is recorded by the service: with the turn number when the
  work belongs to a turn (segment audio, portraits for a campaign entity), and
  with turn `0` for previews and auditions.
- A call is attributed to exactly one row; a cache hit records no usage (it
  cost nothing and spent no provider quota).

### 3.7 UI

**Settings → Usage tab** (`frontend/src/components/SettingsStudio.tsx`):

- Totals for the current campaign and a global total, filterable by provider and
  role, with a per-turn table (turn number, mode, providers used, tokens/chars,
  cost, estimated marker).
- A cost figure appears only for priced providers; otherwise the row shows usage
  units and "no price configured".
- A "metered" badge so unpriced-but-billable providers are still obvious.
- A global total with a **drilldown**: campaign → turn, from the summed view
  (§3.3). The tab opens on the current campaign and offers "All campaigns".

**Inline status:**

- The play header shows a chip while a provider+role is blocked, with a live
  countdown from `Retry-After`; sending a turn while blocked is prevented with a
  clear message.
- The story theater and preview/audition surfaces show the same chip.
- `TurnEvent` error payloads gain `code` (already present) and `retry_after_ms`
  so the client can render the countdown and the funds message
  (`pkg/gui/types.go:404-422`).
- An insufficient-funds failure renders as a distinct, persistent banner:
  "Provider X rejected the request: insufficient funds/credits. Add funds and
  retry — no other work is blocked." It clears on the next success.

**API:**

- `GET /api/game/{id}/usage` → per-campaign summary and per-turn rows.
- `GET /api/usage` → global aggregate across campaigns.
- `GET /api/limits` → current blocks (`provider`, `role`, `until`) and funds
  failures, so the UI can show the chip without waiting for a failure.

### 3.8 Interfaces

```go
// pkg/harness
type Usage struct { /* §3.1 */ }
// StreamChunk.Usage, GenerateResponse.Usage added
func ClassifyProviderError(err error) FailureCode // extended
type GenerationFailure struct { /* …; RetryAfterMS int64 */ }

// pkg/provider
type RateLimitedError struct { RetryAfter time.Duration; Message string }
type InsufficientFundsError struct { Message string }

// pkg/pricing
func CostMicros(u harness.Usage, p Price) Micros
func Resolve(providerKey, model string, cfg *config.Config) Price

// pkg/storage
func (s *Store) SaveUsage(rec UsageRecord) error
func (s *Store) UsageByTurn(turn int) ([]UsageRecord, error)
func (s *Store) UsageSummary() (UsageSummary, error)

// pkg/gui
func (s *Service) GameUsage(ctx, gameID) (*UsageDTO, error)
func (s *Service) GlobalUsage(ctx) (*UsageDTO, error)
func (s *Service) Limits(ctx) (*LimitsDTO, error)
```

## 4. Data Flow

```
provider call
  → provider returns usage (or estimate) and, on failure, a typed error
  → Router / media pipeline records harness.Usage with role + turn
  → pricing.CostMicros(usage, Resolve(provider, model, cfg))
  → storage.SaveUsage(row)
  → on FailureRateLimited: limits.Block(provider, role, until)
  → TurnEvent / API error carries code + retry_after_ms
```

## 5. Error Handling

- Recording usage never fails a turn; a store error is logged
  (`usage.record_error`) and dropped.
- A missing or zero price yields zero cost, never a guessed one; the row keeps
  its usage units so the panel is still useful.
- An unknown model for a known provider falls back to the provider's default
  price row, then to zero.
- A block expires lazily on read, so no timer is needed.
- A malformed `Retry-After` is treated as absent and the default window applies.
- Insufficient-funds clears only on a later successful call for that provider
  and role; it is never auto-expired.

## 6. Testing & Verification

Go (stdlib `testing`, `t.TempDir()`):

- `pkg/pricing`: cost from tokens, characters, and requests; unknown provider is
  zero; an override beats a built-in; micros are exact.
- `pkg/harness`: `ClassifyProviderError` maps `RateLimitedError` (with and
  without `Retry-After`) and `InsufficientFundsError`; markers still classify.
- `pkg/harness`: a `LimitRegistry` blocks and expires on read; `Clear` lifts it;
  a different role is unaffected.
- `pkg/storage`: migration to version 8 on an existing index; `SaveUsage` and the
  three queries; a campaign DB with no usage returns an empty summary.
- `pkg/gui`: a turn on a stub metered provider writes a usage row with the turn
  number and role; a rate-limited stub blocks the next turn and returns 429 with
  `Retry-After`; changing the provider clears it; an insufficient-funds stub does
  not block and reports the code.
- `pkg/media`: a TTS client reporting usage records characters; a cache hit
  records nothing.

Frontend: `tsc` only; the Usage tab and chip are checked by inspection.

Manual: run one turn against a metered provider, confirm the tab and the inline
chip; simulate a 429 with `Retry-After` and confirm the countdown and the
unblock.

## 7. Compatibility & Rollout

- The migration is additive; existing campaigns gain an empty usage table.
- `StreamChunk`/`GenerateResponse` usage fields are optional; providers that do
  not set them behave exactly as before and contribute request counts only.
- The new failure codes are additive to `FailureCode`; consumers that switch on
  the existing codes keep working.
- No price is configured by default for providers without built-in prices, so no
  cost is fabricated.

## 8. Open Questions

- Should blocks survive a restart for very long rate-limit windows?
- Do per-character prices need per-model granularity for speech, or is one price
  per provider enough?
- Should the `ttselevenlabs` panel show remaining quota from
  `GET /v1/user/subscription` alongside spend?
- Which Gemini TTS path is authoritative for usage (legacy `generateContent`
  `usageMetadata` vs the Interactions API `usage`), if both are supported?

## 9. References

- Findings: `pkg/config/types.go:150-152`; `pkg/media/catalog.go:51-53`,
  `pkg/media/capabilities.go:33-34`; `pkg/harness/types.go:8-15,107-111`;
  `pkg/harness/router.go:94,138,185`; `pkg/harness/failure.go:16-22,29-45,66-106`;
  `pkg/provider/geminillm/provider.go:211,588`;
  `pkg/provider/ttsgemini/client.go:254`;
  `pkg/provider/ttselevenlabs/client.go:184-214`;
  `pkg/storage/migrate.go:14-24`;
  `frontend/src/components/SettingsStudio.tsx:108,354-399`;
  `pkg/gui/types.go:404-422`

External (primary):

- OpenAI Chat Completions streaming usage — https://developers.openai.com/api/reference/resources/chat/subresources/completions/streaming-events and https://developers.openai.com/cookbook/examples/how_to_stream_completions
- OpenAI Responses API usage — https://developers.openai.com/api/reference/resources/responses.md
- OpenAI image usage (GPT image models) — https://developers.openai.com/api/reference/resources/images.md
- OpenAI speech (no usage) — https://developers.openai.com/api/reference/resources/audio/subresources/speech/methods/create.md
- OpenAI transcription usage (duration/tokens) — https://platform.openai.com/docs/api-reference/audio/createTranscription
- Gemini usageMetadata — https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/v1/GenerateContentResponse and https://ai.google.dev/api/generate-content
- Gemini TTS (token-billed) — https://ai.google.dev/gemini-api/docs/speech-generation and https://ai.google.dev/gemini-api/docs/pricing
- Imagen response has no usage — https://docs.cloud.google.com/vertex-ai/generative-ai/docs/model-reference/imagen-api
- ElevenLabs character cost header and subscription — https://elevenlabs.io/docs/api-reference/introduction and https://elevenlabs.io/docs/api-reference/user/subscription
- ElevenLabs errors (429 codes, 402 insufficient_credits, no Retry-After) — https://elevenlabs.io/docs/eleven-api/resources/errors
