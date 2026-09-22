# Design Specification: Agentic Turns and Internal Tools

**Date:** 2026-09-22
**Status:** Draft — pending review
**Topic:** Let a turn make more than one model call, and let the GM call internal tools (entity search, graph traversal, timeline search) mid-turn to build a coherent reply

---

## 1. Problem Statement & Motivation

A turn is currently one model call. Everything the GM might need is crammed into a single prompt up front, which is why the coherence work spends so much effort deciding what to include and what to trim. The GM cannot ask a question of its own world, so the only way to know anything is to have been told, and the only recourse when a detail is missing is to invent it.

The pieces that make this fixable are already here, and the gap is specific:

1. **The provider contract cannot represent a tool round.** `GenerateRequest` has a single `Prompt` field and no tools; `StreamChunk` carries only `Text`. There is no way to send a message history, offer tools, or receive a call back.
2. **There is no search.** `pkg/storage` has no FTS5, no `MATCH`, and no `LIKE` anywhere. The index can fetch an entity by exact id and list every entity, and that is all.
3. **There are no embeddings.** Nothing in the codebase produces or stores a vector.
4. **A turn has no loop.** `ProcessActionStream` assembles once, generates once, parses once.

Multi-call turns make coherence far less dependent on pre-injection, because the GM can look something up instead of being handed the whole world and asked to find the relevant part.

## 2. Goals & Non-Goals

**Goals**

1. A turn may call the model again after a tool result, up to a bounded number of rounds.
2. The GM can search entities, read one, walk the graph, and search the timeline.
3. Tool activity is visible while it happens, and never looks like a stall.
4. A turn cannot become an unbounded agent loop or an unbounded spend.
5. Tools degrade honestly: a provider that cannot call them still plays coherently.

**Non-Goals**

- Write tools. The GM does not mutate canon; the extractor owns that path.
- Shell, filesystem, network, or code-execution tools. The tool surface is internal queries against the campaign's own index and never more.
- Tools for the extractor or the summariser. Only `gm` gets them.
- A general agent framework. One bounded loop in one place.
- Embeddings in v1 (see section 8 for the seam they must fit).

---

## 3. Decisions

Settled across two review rounds.

| Area | Decision |
| --- | --- |
| Home | This spec. The coherence spec gains a pointer only |
| Sequencing | After the trace and canon increments, so the instrument exists and the GM already sees state before tools are added |
| Pre-injected context | Stays. Tools complement recall rather than replace it, so coherence never depends on the model choosing to ask |
| Capability | Declared per role (`supports_tools: auto\|yes\|no`); function calling for capable HTTP providers, today's behaviour for everything else |
| Retrieval stack | SQLite FTS5 and graph traversal first; embeddings later **behind the same tool interface** |
| Bounds | `agents.tool_rounds` 4, `agents.tool_result_chars` 4000, the whole conversation counted against `context_token_budget` |
| Budget refusal | Delivered as a readable tool result, so the model answers with what it has instead of retrying the same call |
| Transcript | In memory for the turn, recorded in the **trace**, never in `history.jsonl` |
| Tool set | `search_entities`, `get_entity`, `graph_neighbours`, `search_timeline`, all read-only |
| Tool failure | Returned to the model as an error result it can read and adapt to |
| Streaming | A `{"type":"tool"}` event while a call runs |
| Watchdog | Every tool event resets the idle watchdog, server and client |
| Index | An FTS5 virtual table over `entities` and `turns`, with triggers, backfilled from the content tables |
| Tool-call assembly | In the provider. `StreamChunk` never carries partial fragments, so no vendor's wire format reaches the engine |
| Multiple calls per message | Executed sequentially, because local reads are microseconds and ordering makes the trace and tests reproducible |
| Roles with tools | `gm` only, and the opening turn is included |
| Tool surface | A permanent ceiling: read-only, internal, four tools, and no write tool at any point |
| Trace detail | `summary` records name, outcome, size, and duration; arguments and results are `full`, like prompts |
| Embeddings | Deferred to their own spec, with the `search_semantic` interface fixed now so the model's contract does not change later |

---

## 4. The Turn as a Bounded Loop

`ProcessActionStream` keeps its shape: load history, handle commands, assemble context, generate, parse, record. Only the generation step changes, from one call into a loop.

```go
messages := []harness.Message{
    {Role: "system", Content: contextPrompt},
    {Role: "user", Content: action},
}

var final string
for round := 0; round <= limits.ToolRounds; round++ {
    offerTools := supportsTools && round < limits.ToolRounds && !overBudget(messages)
    reply := generate(messages, toolsFor(offerTools))

    if len(reply.ToolCalls) == 0 {
        final = reply.Text
        break
    }

    messages = append(messages, reply.Message())
    for _, call := range reply.ToolCalls {
        result := executeTool(call)
        messages = append(messages, harness.Message{
            Role: "tool", ToolCallID: call.ID, Content: result,
        })
    }
}
```

Properties that matter:

- **The loop always terminates.** Round count is bounded, and once the rounds are spent or the budget is reached, tools are withdrawn and the model is told in the prompt that it must answer now. A model that keeps asking cannot loop forever.
- **The last assistant message with no tool calls is the turn.** Everything downstream, segmentation, mention resolution, extraction, recording, is unchanged.
- **Tool rounds are sequential.** Local reads are microseconds, so concurrency buys nothing, and ordering keeps results, trace, and tests reproducible.
- **`overBudget` counts the live conversation**, not the assembled prompt. The prompt was already trimmed to fit; each tool result grows the conversation, and the check is what stops that growth from exceeding the model's real window.

## 5. Provider Contract

`pkg/harness/types.go` gains the vocabulary a tool round needs:

```go
type Message struct {
    Role       string     // "system", "user", "assistant", "tool"
    Content    string
    ToolCalls  []ToolCall // assistant messages only
    ToolCallID string     // tool messages only
}

type ToolSpec struct {
    Name        string
    Description string
    Parameters  map[string]interface{} // JSON Schema
}

type ToolCall struct {
    ID        string
    Name      string
    Arguments string // raw JSON as the model produced it
}

type GenerateRequest struct {
    Messages    []Message
    Tools       []ToolSpec
    Temperature float64
    MaxTokens   int
    Extra       map[string]interface{}
    Prompt      string // derived, for providers that only accept a string
}

type StreamChunk struct {
    Text         string
    ToolCalls    []ToolCall
    Done         bool
    FinishReason string
    Error        error
}
```

`Messages` is authoritative. `Prompt` is kept and synthesised for providers that take a single string, so the CLI provider and the built-in oracle keep working unchanged. That is the whole compatibility story: a provider that ignores tools and messages behaves exactly as it does today.

The HTTP provider gains the OpenAI-compatible `tools` field and **accumulates the streamed `tool_calls` deltas itself** (they arrive fragmented by index, with the name in one delta and the arguments split across many). Accumulation lives in the provider so `StreamChunk.ToolCalls` only ever carries whole calls: partial fragments would leak one vendor's wire format into the engine and make every future provider responsible for the same reassembly.

## 6. Capability

`agents.roles.<role>.supports_tools` defaults to `auto`:

- `auto` — an HTTP provider gets tools; every other type does not.
- `yes` — force the attempt, for an endpoint whose type does not imply tool support.
- `no` — suppress it, for a server known to mishandle the `tools` field.

Inference alone is wrong in both directions: some OpenAI-compatible servers accept `tools` and ignore it, returning a plain completion, which is harmless; others reject the field outright with a 400. A rejection is handled as a turn-level degradation: the provider is retried once without tools, the trace records why, and the turn continues with pre-injected context.

Only `gm` calls tools, and **the opening turn is included**. The opening turn is the most world-knowledge-hungry turn there is, and excluding it would make the campaign's most context-dependent turn the only one that cannot look anything up. The summariser and extractor stay tool-free: their work is bounded, and giving the summariser search would let it become a second, unbounded agent.

## 7. Tools v1

All four are internal reads against the campaign's own index and store. None of them can write.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `search_entities` | `query`, optional `type`, optional `limit` | Matching entities as `id`, `name`, `type`, and a body snippet |
| `get_entity` | `id_or_name` | The note's frontmatter and state, plus the body up to the result cap |
| `graph_neighbours` | `id`, optional direction, optional `limit` | Adjacent entities and the relation that connects them |
| `search_timeline` | `query`, optional `limit`, optional `entity` | Turn numbers with a narration snippet |

Rules:

- Every result is compact text with a header naming what was found, because the model reads it as a tool message and anchors on structure.
- Results are capped at `agents.tool_result_chars`; a cap that bites says so, so the model knows to narrow its query rather than conclude the world is small.
- A tool error is returned as the result text (`"error: no entity matching \"Kael\""`), never as a failure of the turn.
- An unknown or hallucinated tool name returns a readable error listing the available tools.
- The parameter schemas are generated from one table in Go, so the tool list, its documentation, and its dispatch cannot drift apart.
- The surface is a **permanent ceiling**, not a v1 convenience: read-only, internal, and never a filesystem, shell, network, or code-execution tool. There is also no write tool, at any point, because canon changes belong to the extractor, where they are proposed rather than asserted.

## 8. Retrieval Stack

**v1: FTS5 and the graph.** A virtual table over `entities` (name, body, tags) and `turns` (input, narration), with triggers on insert, update, and delete so search follows the tables. Because `entities` has a text primary key and `turns` an integer one, both use their implicit `rowid` as the external content rowid.

Migration follows the index's existing promise: created idempotently on open, and **backfilled from the content tables** rather than by rescanning Markdown, so opening an old campaign does not re-read every note. If the virtual table is missing or out of step, it is dropped and rebuilt from the tables.

**Later: embeddings.** Out of scope for v1, and deliberately designed for. `search_semantic` will be a fifth tool with the same shape, so the model's contract does not change when it arrives. Its **interface is fixed now** (name, arguments, result format), while the provider axis, the vector storage, and the re-embed trigger get their own spec. The trigger is nearly free when it comes: the index already tracks a file hash per note, which is exactly the signal that a note's vector is stale. Coupling this spec to an embedding design would delay tools that need no new dependency at all.

## 9. Bounds and Failure Modes

| Bound | Default | Effect |
| --- | --- | --- |
| `agents.tool_rounds` | 4 | Tool rounds per turn, so at most five generations |
| `agents.tool_result_chars` | 4000 | Per tool result, truncating with a marker that says so |
| `context_token_budget` | unbounded | The whole conversation. Once reached, tools are withdrawn and the model is told to answer |
| `turn_timeout_seconds` | 300 | Wall clock for the whole loop, unchanged |

The withdrawal message is a **tool result**, not a silent stop, because a model whose tool call vanishes will simply try the same call again until the round limit is spent. Withdrawal is also the last thing that happens: the next generation is sent without a `tools` field at all, so a well-behaved model cannot ask a question that will not be answered.

## 10. Streaming and the Watchdog

A tool round can take seconds during which no narration is produced. Two consequences:

1. A new `{"type":"tool"}` stream event carries the tool name, a summary of its arguments, and its status. The console renders a small activity line (`looking up Guard Kael…`) so the turn reads as thinking rather than stalled.
2. **Every tool event resets the idle watchdog**, server-side in `pkg/engine` and client-side in the inactivity guard. Without this, any tool round slower than `agents.chunk_timeout_seconds` is killed as a stall, which would make the feature unusable on exactly the local models most likely to need it.

The existing `TurnEvent` framing carries the new event type; no new transport is introduced.

## 11. Trace

The coherence spec's event catalogue is the canonical list; this feature adds rows to it.

| Event | Fields |
| --- | --- |
| `tool.round` | round, offered, conversation_tokens, budget |
| `tool.call` | round, name, arguments (full), arguments_chars |
| `tool.result` | name, ok, bytes, duration_ms, result (full, capped) |
| `provider.tools` | offered[], rejected, degradation reason |

Tool events count as turn activity for the watchdog, so they are emitted as they happen rather than buffered.

## 12. Config Changes

| Key | Default | Meaning |
| --- | --- | --- |
| `agents.roles.<role>.supports_tools` | `auto` | `auto`, `yes`, or `no` |
| `agents.tool_rounds` | 4 | Tool rounds per turn |
| `agents.tool_result_chars` | 4000 | Per tool result cap |

Both are exposed in the Settings Studio beside the other limits, and both are meaningless for a role whose provider cannot call tools.

## 13. Testing Strategy

- **Provider**: streamed `tool_calls` deltas reassemble into whole calls, including arguments split across many frames; a `tools` rejection degrades once and is recorded.
- **Loop**: a scripted provider that calls a tool then answers; a provider that calls tools every round is stopped at the limit with a final answer; a provider that never calls tools is unaffected.
- **Budget**: the conversation crossing the budget withdraws tools, the refusal is readable, and the turn still records.
- **Tools**: each tool against a fixture store, including no-match, cap-exceeded, unknown-tool, and a malformed argument payload.
- **Watchdog**: a tool round longer than `chunk_timeout_seconds` does not stall the turn.
- **Compatibility**: a CLI provider and the built-in oracle play a turn unchanged with tools configured.
- **Migration**: an index created before FTS5 gains a populated search table on open.

## 14. Migration & Compatibility

- Providers that ignore `Messages`, `Tools`, and `ToolCalls` behave exactly as today; the compatibility surface is that one property.
- `supports_tools` defaults to `auto`, so no existing configuration changes behaviour beyond HTTP providers gaining a capability they can decline.
- The FTS5 table is derived data: it can be dropped and rebuilt, and an old campaign backfills on first open.
- No change to `history.jsonl`, so no timeline migration.

## 15. Open Questions

Settled in review:

- **Round 6** (shape): this spec is separate and lands after trace and canon; tools complement pre-injection; capability is declared per provider; FTS5 and graph before embeddings; bounds are four rounds, 4000 characters per result, and the turn budget.
- **Round 7**: the transcript is in-memory and traced, never in `history.jsonl`; four read-only tools; a `tool` stream event resets the idle watchdog on both sides; `supports_tools: auto|yes|no`; FTS5 as a virtual table with triggers, backfilled from the content tables.
- **Round 8**: the embeddings provider and storage are deferred to their own spec while the tool's interface is fixed now; streamed tool calls are reassembled in the provider; calls execute sequentially; `gm` only, and the opening turn is included; the read-only internal surface is a permanent ceiling; trace records name, outcome, size and duration at `summary`, and arguments and results at `full`.

Queued for the ninth round:

1. **FTS5 query handling.** Pass the model's words to `MATCH` directly, or build a sanitised term query.
2. **Tokenizer.** The default `unicode61`, or `porter` stemming.
3. **Provenance.** Does a replayed turn show that tools were used, and how much.
4. **A misbehaving last round.** What happens when the model asks for a tool in the round where tools were withdrawn.

## 16. File Map

**Create**

- `pkg/harness/tools.go` (+ test) — `ToolSpec`, `ToolCall`, the tool table and its schemas
- `pkg/tools/` (+ test) — tool implementations over the store, FTS, and edges
- `pkg/storage/fts.go` (+ test) — the FTS5 tables, triggers, backfill, and rebuild
- `docs/superpowers/plans/2026-09-22-agentic-turns.md` — the task plan

**Modify**

- `pkg/harness/types.go` — `Message`, `ToolSpec`, `ToolCall`, `GenerateRequest.Messages`, `StreamChunk.ToolCalls`
- `pkg/harness/http_provider.go` — the `tools` field, `tool_calls` accumulation, rejection degradation
- `pkg/harness/cli_provider.go`, `oracle_provider.go` — synthesise `Prompt` from `Messages`, ignore tools
- `pkg/harness/factory.go` — carry `supports_tools`
- `pkg/engine/orchestrator.go` — the loop, bounds, withdrawal, watchdog resets
- `pkg/gui/service.go`, `server.go`, `types.go` — the `tool` stream event
- `pkg/config/types.go` — the new keys
- `frontend/src/components/ActionConsole.tsx` — the activity line
- `frontend/src/components/SettingsStudio.tsx` — the new limits

## 17. Delivery Increments

1. **Contract** — messages, tools, tool calls, HTTP accumulation, capability, and the trace rows. Shippable with no tools defined, and it makes every later step observable.
2. **Search** — FTS5 plus the four tools, testable entirely through tool unit tests with no model involved.
3. **The loop** — rounds, bounds, withdrawal, `tool` stream events, watchdog resets, and the console activity line.
4. **Embeddings** — `search_semantic` behind the existing tool interface, if it warrants its own spec.
