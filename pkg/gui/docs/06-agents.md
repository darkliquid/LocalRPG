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
   - Best suited for reasoning-heavy models (e.g. `claude-3-5-sonnet`, `gemini-2.0-pro`, `gpt-4o`, `qwen2.5:14b`).

2. **`narrator`**
   - Responsible for literary description, evocative dialogue, sensory immersion, and atmospheric stage directions.
   - Best suited for creative writing models (e.g. `gemini-2.0-flash`, `mistral-large`, `llama3.1:8b`).

3. **`extractor`**
   - Runs in the background at turn completion to parse entities, character introductions, inventory changes, and relationship tags into the campaign Codex.
   - Best suited for fast, structured-output models (e.g. `gemini-2.0-flash-lite`, `gpt-4o-mini`, `llama3.2:3b`).

## Role Mapping in Configuration

Configure agent role assignments under `agents.roles` in your `config.yaml`:

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

## Token Budgets & Context Management

The orchestrator dynamically fits prompt layers into the configured `context_window` limit:
- Priority is given to system rules and character sheet state.
- Entity memories and living-world notes are prioritized based on proximity in the knowledge graph.
- Historical turns are compressed or truncated using sliding-window recaps when context headroom is low.
