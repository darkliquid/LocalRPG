---
id: 06-agents
title: AI Agents & Roles
category: Configuration & Providers
order: 6
description: Assigning models to GM, Narrator, and Extractor roles, plus token budgets and fallback chains.
---

# AI Agents & Roles

In LocalRPG, distinct cognitive tasks are assigned to specialized **Agent Roles**. You can map different AI models to each role based on their strengths, speeds, and costs.

## Core Agent Roles

1. **`gm` (Game Master)**
   - Responsible for rules adjudication, difficulty checks, world logic, and pacing.
   - Suited to reasoning-heavy models (such as `claude-3-5-sonnet`, `gemini-2.0-pro`, `gpt-4o`, `qwen2.5:14b`).

2. **`narrator`**
   - Responsible for literary description, evocative dialogue, sensory immersion, and atmospheric stage directions.
   - Suited to creative writing models (such as `gemini-2.0-flash`, `mistral-large`, `llama3.1:8b`).

3. **`extractor`**
   - Runs in the background at turn completion to parse entities, character introductions, inventory changes, and relationship tags into the campaign Codex.
   - Suited to fast, structured-output models (such as `gemini-2.0-flash-lite`, `gpt-4o-mini`, `llama3.2:3b`).

## Role Mapping in Configuration

Assign agent roles under `agents.roles` in your `config.yaml`:

```yaml
agents:
  roles:
    gm:
      provider: gemini-pro
      temperature: 0.7
      max_tokens: 1024
      fallback: local-llama
    narrator:
      provider: gemini-flash
      temperature: 0.9
      max_tokens: 1500
    extractor:
      provider: local-llama
      temperature: 0.2
      max_tokens: 512
```

## Provider Chains

A role can declare an ordered chain of provider instances rather than one
provider and its fallback. Each member names another entry under `agents.roles`,
so a chain reuses the provider configurations you already have:

```yaml
agents:
  roles:
    gm:
      type: gemini
      model: gemini-3.8-flash
      chain: [gm, cheap, local]
      select: cheapest
    cheap:
      type: gemini
      model: gemini-2.5-flash-lite
    local:
      type: http
      endpoint: http://localhost:11434/v1
      model: llama3.1
```

The `select` rule orders the chain, and the chain is then tried in order, so a
selection rule and a fallback are the same mechanism:

| Rule | Order |
| --- | --- |
| `first` (the default) | the declared order |
| `cheapest` | by the spend ledger's price, cheapest first; an unpriced instance sorts last |
| `local-first` | offline instances first, then a local server, then cloud |
| `by-tag` | instances whose provider carries `tag` as a feature or tier name first |

An empty chain keeps the role's own provider and the `agents.fallbacks` entry,
so a configuration written before chains existed behaves as it did. An unknown
chain member or an unknown rule is reported as a validation warning rather than
silently changing the chain.

## Token Budgets & Context Management

The orchestrator fits prompt layers into the configured `context_window` limit:

- Priority is given to system rules and character sheet state.
- Entity memories and living-world notes are prioritized based on proximity in the knowledge graph.
- Historical turns are compressed or truncated using sliding-window recaps when context headroom is low.
