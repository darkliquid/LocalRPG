# Anthropic Provider Design

**Date:** 2026-10-24
**Status:** Proposed
**Bead:** LocalRPG-00i
**Scope:** `pkg/provider`, `pkg/harness`, `pkg/config`, `pkg/pricing`, `frontend/src`, `pkg/gui/docs`
**Research:** API surface verified against official Anthropic (Claude platform) documentation on
2026-10-24; sources in section 11.

## 1. Problem

Anthropic is reachable today only as a workaround: a `claude-cli` preset wraps the Claude Code CLI
(`type: cli`, `command: claude`, `args: [-p]`), which spawns Node and flattens the conversation into a
one-shot prompt. There is no native HTTP speaking provider for the Messages API, and the generic
`llm:openaichat` adapter cannot speak Anthropic's wire: a different content-block message shape
(`tool_use`/`tool_result` blocks), different SSE event names, auth via `x-api-key` plus a required
`anthropic-version` header, a required `max_tokens`, and sampling parameters that are now invalid.

A native adapter is the plan the 2026-09-22 speech-and-LLM-provider proposal already anticipated (P7).

## 2. What Anthropic offers (verified 2026-10-24)

| Capability | Offering | Verdict |
|---|---|---|
| Chat (LLM) | Messages API `POST api.anthropic.com/v1/messages`; models `claude-fable-5-1` ($10/$50 per MTok), `claude-opus-5-5` ($4/$20), `claude-sonnet-5-5` ($2/$10), `claude-haiku-5-5` (from $0.10/$0.50), all 1M context, vision input, tool use, adaptive thinking | **This spec** |
| Image generation | none; the vision FAQ is explicit that Claude is an image understanding model only, text and image *input*, text output | impossible; image roles keep Gemini/tts-style providers (OpenAI per the sibling spec) |
| TTS/STT | none; no `/v1/audio` endpoints exist anywhere in the API reference | impossible |
| Embeddings | none ("Anthropic does not offer its own embedding model"); docs point to third-party Voyage AI | impossible natively; Voyage could join the embeddings registry later as its own provider |
| Prompt caching | top-level automatic `cache_control: {"type":"ephemeral"}` (breakpoint auto-moves to the last cacheable block); explicit breakpoints (max 4) supported too; reads at 10% of base input (2.5% Fable/Mythos, 5% Opus/Sonnet 5.5); writes +25%; 5m TTL | **in this adapter** |
| Structured outputs | `output_config.format = {type: "json_schema", schema}`, GA | follow-up, non-goal v1 |
| Web search / code execution / MCP connector | server-side tools priced separately | non-goal (the engine's tools are local) |
| Streaming | SSE: `message_start`, `content_block_start/delta/stop`, `message_delta`, `message_stop`, `ping`, `error` | **in this adapter** |
| Auth | `Authorization: Bearer <key>` or legacy `x-api-key`; `anthropic-version: 2026-10-21` required | **in this adapter** |

Two API facts shape the adapter more than anything else:

1. **`temperature`, `top_p` and `top_k` are removed.** As of API version `2026-10-21` a request that
   sends any of them is a 400 on every model. Behaviour control moved to `output_config.effort`
   (`low|medium|high|xhigh|max`) and the `thinking` config. LocalRPG's default role config carries
   `temperature: 0.7`, so the adapter must never forward it.
2. **`max_tokens` is required** on every message request.

## 3. Goals

- A first-class Anthropic LLM provider: `type: anthropic`, registry key `llm:anthropic`, package
  `pkg/provider/anthropicllm`, wired through the same `harness.ModelProvider` interface, router,
  fallbacks and role inheritance (`inherit`) every other provider uses.
- Full tool-loop participation: the engine's multi-round tool loop (`harness.ToolSpecs()`,
  `ToolChoice "required"` cadence force) must work, including `tool_use`/`tool_result` conversion.
- Streaming through the engine's `Stream` path with honest `FinishReason` and `Usage`.
- Prompt caching on by default, since the engine re-sends a growing narration context every turn
  (4-layer assembled prompt; cache reads at 2.5 to 10 percent of base input are a large saving).
- Presets, pricing rows, catalogue entry, config reference and frontend editor options, like every
  other provider.

## 4. Non-goals

- **Media families.** Anthropic has no image/TTS/STT endpoints; nothing to adapt. A `type: anthropic`
  on `media.tts` is not defined and errors with a clear message like any unknown type.
- **Voyage AI embeddings**, which Anthropic's docs recommend for retrieval. Worth its own small bead for
  the `pkg/embeddings` registry only if the campaign corpus outgrows `embedding:builtin`/ONNX.
- **Structured outputs (`output_config.format`).** The extractor and completion roles parse
  prompt-requested JSON the same way on every provider today; strict JSON would be a separate, shared
  decision (sibling spec's `strict` knob) rather than an Anthropic-only feature.
- **Server-side tools** (web search, code execution, MCP connector): web search is priced per search; local-first means the corpus is the knowledge source. Non-goal.
- **Extended-thinking round-trips in tool loops** (section 7 limitation) and managed agents/Sessions.
- **The legacy `/v1/complete` completions API.** Deprecated; Messages is the sole supported endpoint.
- **Server-sent thinking to the trace.** Raw thinking is never returned by the models anyway; the
  `display` summary is dropped rather than surfaced.

## 5. Design

### 5.1 Registration and wiring

- `pkg/provider/anthropicllm` self-registers under `llm:anthropic` (`ProviderConfig`-described, tier
  Cloud, features streaming/tools/key_required/model_catalogue), blank-imported via
  `pkg/provider/all/all.go`.
- `pkg/provider/keys.go`: `KeyLLMAnthropic Key = "llm:anthropic"` plus `AllKeys()` entry.
- `harness.KeyFor` gains `case "anthropic"` (an instance discriminator only when a user names one; the
  endpoint is fixed to the public API and no host discriminator is derived).
- `harness.NewModelProvider`'s type switch gains `"anthropic"` alongside `http`/`gemini` so construction
  goes through the registry like the other HTTP-shaped providers.
- `ProvidersConfig.Anthropic` (struct shape of `InworldProviderConfig`) holds
  `providers.anthropic.api_key`; `sharedKeyFor` resolves it the same way it resolves Gemini.

### 5.2 Request construction

`POST {endpoint}/v1/messages` (default endpoint `https://api.anthropic.com`); headers
`x-api-key: <key>` and `anthropic-version: 2026-10-21` (a compile-time constant; no per-request
version knob). `stream: true`.

| LocalRPG (`GenerateRequest`) | Messages API |
|---|---|
| `System` | top-level `system` string |
| `Messages` role `system` | appended into `system`, separated by a blank line |
| `Messages` role `user` | `{role: "user", content: [{type: "text", text}]}` |
| `Messages` role `assistant` (text only) | `{role: "assistant", content: [{type: "text", text}]}` |
| `Messages` role `assistant` + `ToolCalls` | text block (if any) plus `{type: "tool_use", id, name, input}` per call (`arguments` parsed as JSON; malformed arguments pass through verbatim, as `toolCallAccumulator` does) |
| `Messages` role `tool` | `{role: "user", content: [{type: "tool_result", tool_use_id, content: [{type:"text", text}]}]}` |
| `Tools` | `tools: [{name, description, input_schema}]` |
| `ToolChoice "required"` | `tool_choice: {type: "any"}`; `"none"` drops the tools array; `""` omits the key |
| `Temperature`, `TopP`, `TopK` | **never sent**; a warn-once log when a role config set one |
| `ProviderConfig.Effort` (new shared field) | `output_config: {effort: <low|medium|high|xhigh|max>}` |
| `ProviderConfig.ThinkingBudget` | 5.5 models: ignored (adaptive thinking is always on; `type: "disabled"` would 400 on Fable/Opus). 4.x-era models: `{type: "enabled", budget_tokens: max(1024, budget)}`, only when the request carries no tools |
| `MaxTokens` | required `max_tokens`: the request value when set, else the config value, else 1024 |
| `ProviderConfig.PromptCache` (new: `auto|off`) | `auto` (default for this adapter): top-level `cache_control: {type: "ephemeral"}`; `off`: omitted |

### 5.3 Streaming

Frames are `event: <name>` + `data: {...}` pairs, so the reader parses the `event` line (unlike
`openaichat`, which keys off `data.type` alone), tolerates blank-line keep-alives, and reuses the
`SetLogger`/`SetChunkLimit` optional interfaces.

| Event | Chunk behaviour |
|---|---|
| `message_start` | capture the message `id`; record `usage.input_tokens` for the final chunk |
| `content_block_start` (index, type `tool_use`) | open a tool-call accumulator keyed by block index, seeded with `id`/`name` |
| `content_block_delta` type `text_delta` | `StreamChunk{Text: delta}` |
| `content_block_delta` type `input_json_delta` | append `partial_json` to the accumulator for that index |
| `content_block_delta` type `thinking_delta` | count tokens towards the future `thinking_tokens` note; emit no chunk |
| `content_block_stop` | if the closed block was a `tool_use`, emit the accumulated `ToolCall` |
| `message_delta` | collect `stop_reason` and `usage.output_tokens` |
| `message_stop` | final `StreamChunk{Done: true, FinishReason, ToolCalls?, Usage}` |
| `ping` | ignored |
| `error` event / non-200 | `StreamChunk{Error}` or request error with status plus truncated body (`provider.MaxProviderDetailBytes`) |

Finish reason mapping: `end_turn`/`stop_sequence` → `stop`, `max_tokens` → `length`, `tool_use` →
`tool_calls`, `refusal` → `refusal` (with the refusal detail in the error path, HTTP 200 still).

Usage: `usage.input_tokens + cache_read_input_tokens` report as input tokens;
`usage.cache_read_input_tokens` → `GenerateResponse.CachedTokens`;
`usage.output_tokens` (thinking included) → output tokens; `cache_creation_input_tokens` is reported
as input but is not yet priced separately (section 7).

### 5.4 Presets, pricing, catalogue

- Descriptor presets, all disabled until a key is configured:
  1. *Claude Sonnet 5.5* (recommended gm; $2/$10, `effort: high` omitted by default),
  2. *Claude Haiku 5.5* (extractor/completion and budget gm),
  3. *Claude Opus 5.5* (high-stakes narration),
  4. *Claude Fable 5.1* (max quality, long-horizon turns).
  Each preset sets `type: anthropic` and the model; no temperature key appears in any preset.
- `pkg/pricing/pricing.go` gains model rows for the four IDs, captured 2026-10-24 (the old 4.x-era
  models keep the adapter-wide row only). Cache-read discount is not representable in the current
  `Price` shape; costs are computed at base input rate, which over-charges cache hits rather than
  under-charging. An extension to price cache reads separately belongs to the cost-guardrails bead (P5).
- The frontend provider-type unions and `LLMRoleEditor.tsx` gain `"anthropic"`; at the same
  regeneration `06-agents.md`'s `claude-3-5-sonnet` mention updates to a current model.
- Catalogue and docs regenerate from the registry (`go test ./pkg/gui -update-docs`, `-update-dto`);
  `05-providers.md` mentions Anthropic as an `http` target today; that paragraph becomes the native path.

## 6. Behaviour

- Every engine role except media roles can be Anthropic: `gm`, `narrator`, `extractor`, `completion`,
  and any user-defined role; `supports_tools` and `inherit` behave as with `http` roles.
- Turn narration is cheaper per turn than direct token cost implies: the system/game rules block and
  world lore sit in the cached prefix; the growing history inherits the automatic breakpoint.
- A tool-cadence turn still forces a tool call (`tool_choice: any`), and the model's `tool_use` block
  round-trips through the same accumulator contract `openaichat` uses, keyed by OpenAI's array index
  there and by content-block index here.
- The extractor's JSON-minded prompt works unchanged; a schema-valid guarantee would require the
  structured-outputs follow-up, not a per-provider trick.

## 7. Limitations (recorded, not hidden)

- **Thinking blocks are not round-tripped in tool loops.** The harness `Message` shape carries text,
  tool calls and tool results, not thinking blocks with their signatures. Omission is tolerated by
  adaptive-thinking models (they re-reason when histories lack the block; cache value is lost);
  manual-thinking models on 4.x reject tool use with thinking mid-loop, so the adapter never sends
  `thinking` on a request that carries tools (warn-once log). Closing this means giving `harness.Message`
  a thinking-block carrier, which is its own decision.
- **Cost precision lags caching.** `harness.Usage` has no cache-write field, so a turn's cached prefix
  is priced at the base rate. Under-costing is impossible; over-costing until P5 extends the shape.
- **Strict tool use.** Anthropic's `strict: true` (grammar-constrained) tools are not used in v1 for
  parity with the other adapters' `strict: false` behaviour.
- **The `2026-10-21` version constant.** A pinned version is required; new regional/beta features
  (workspace IDs, fast mode) need their own headers and are simply not sent.

## 8. File Map

```text
pkg/provider/anthropicllm/{anthropicllm.go,http.go,sse.go}    new
pkg/provider/keys.go                  +KeyLLMAnthropic, +AllKeys
pkg/provider/all/all.go               +blank import
pkg/harness/exports.go                +KeyFor case "anthropic"
pkg/harness/factory.go                +type clause, +sharedKeyFor
pkg/harness/types.go                  +ProviderConfig.Effort, +ProviderConfig.PromptCache
pkg/config/types.go                   +ProvidersConfig.Anthropic, +AgentRoleConfig mirrors
pkg/pricing/pricing.go                +rows (captured 2026-10-24)
frontend/src/types.ts                 +provider type union entries
frontend/src/components/providers/LLMRoleEditor.tsx  +"anthropic" option
pkg/gui/testdata/dto.json             regenerate (-update-dto)
pkg/gui/docs/12-provider-catalogue.md regenerate (-update-docs)
pkg/gui/docs/05-providers.md          prose
cfg taken care of by the same shared field additions as the OpenAI spec
```

## 9. Testing

- httptest fake Anthropic server: request-body shape (no `temperature`, required `max_tokens`,
  `system` at top level, `tool_choice` mapping, `cache_control` present under `auto` and absent under
  `off`), recorded `anthropic-version`/`x-api-key` headers, SSE fixtures for streaming text, a
  tool-call round (`input_json_delta` accumulation by block index), `refusal` stop reason, and 4xx
  error surfaces.
- Warn-once behaviour for a configured temperature (exactly one log per provider instance).
- Preset/key resolution: role key without `api_key` resolves `providers.anthropic.api_key`; the
  `env:` convention is covered by existing config tests.
- Catalogue and docs regeneration tests; DTO contract test with the two exception maps unchanged.
- `mise run test:backend:race` for the SSE reader; `mise run lint` at the end (new exported symbols
  carry doc comments so `lint:revive` does not grow).

## 10. Rollout

One pull request: adapter plus wiring, then a second for presets/pricing/frontend if the first is
already large. The bead stays open through both. The `claude-cli` preset stays for users who prefer the
CLI.

## 11. Risks and sources

- **Model churn.** `claude-sonnet-4-5` is EOL 2026-11-30 and the 4.x line will retire; model IDs are
  user-configurable so retirement is a preset refresh. Re-verify prices at implementation.
- **A response carrying `refusal`.** Surfaced as an error-level failure through the standard failure
  plumbing (`useFailures` on the GUI side), worded so the player knows the narrator declined, not that
  the app broke.
- **SDK.** The Vendor Go SDK is not vendored; the adapter is hand-rolled HTTP like `openaichat` (the
  only SDK dependency in the tree today is Gemini's `genai`). No Go SDK adoption decision is needed;
  if one happens later it is contained by the package.

Sources:

- Models: `https://platform.claude.com/docs/en/models/overview`
- Messages API: `https://platform.claude.com/docs/en/api/messages/create`
- Versioning (temperature removal): `https://platform.claude.com/docs/en/api/versioning`
- Streaming: `https://platform.claude.com/docs/en/build-with-claude/streaming`
- Prompt caching: `https://platform.claude.com/docs/en/build-with-claude/prompt-caching`
- Thinking: `https://platform.claude.com/docs/en/build-with-claude/thinking`
- Vision FAQ ("cannot generate images"): `https://platform.claude.com/docs/en/build-with-claude/vision`
- Embeddings (Voyage): `https://platform.claude.com/docs/en/build-with-claude/embeddings`
- API overview (auth headers, endpoint list): `https://platform.claude.com/docs/en/api/overview`
