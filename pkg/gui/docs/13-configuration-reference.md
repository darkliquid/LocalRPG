---
id: 13-configuration-reference
title: Configuration Reference
category: Configuration & Providers
order: 7
description: Every key LocalRPG reads from config.yaml, with the accepted values for the enumerated ones.
---

# Configuration Reference

This page is generated from the configuration structs, so it always lists the
keys the loader accepts. The file is `config.yaml` in the user config
directory, merged with an optional `./localrpg.yaml` in the project root.

## Keys

A `[]` suffix marks a list of tables. `<key>` and `<role>` are names you
choose; the ledger and provider identifiers are listed in the
[Provider & Model Catalogue](12-provider-catalogue).

### `version`

| Key | Type |
| --- | --- |
| `version` | string |

### `paths`

| Key | Type |
| --- | --- |
| `paths.systems` | string |
| `paths.worlds` | string |
| `paths.games` | string |
| `paths.cache` | string |

### `providers`

| Key | Type |
| --- | --- |
| `providers.gemini.api_key` | string |
| `providers.currency` | string |
| `providers.prices[].provider` | string |
| `providers.prices[].model` | string |
| `providers.prices[].per_million_input` | int |
| `providers.prices[].per_million_output` | int |
| `providers.prices[].per_character` | int |
| `providers.prices[].per_request` | int |

### `agents`

| Key | Type |
| --- | --- |
| `agents.default_role` | string |
| `agents.roles.<key>.type` | string |
| `agents.roles.<key>.inherit_from` | string |
| `agents.roles.<key>.builtin_name` | string |
| `agents.roles.<key>.command` | string |
| `agents.roles.<key>.args` | []string |
| `agents.roles.<key>.endpoint` | string |
| `agents.roles.<key>.model` | string |
| `agents.roles.<key>.api_key` | string |
| `agents.roles.<key>.temperature` | float |
| `agents.roles.<key>.max_tokens` | int |
| `agents.roles.<key>.supports_tools` | string |
| `agents.roles.<key>.thinking_budget` | int |
| `agents.roles.<key>.top_p` | float |
| `agents.roles.<key>.top_k` | int |
| `agents.fallbacks` | map<string, string> |
| `agents.turn_timeout_seconds` | int |
| `agents.chunk_timeout_seconds` | int |
| `agents.context_token_budget` | int |
| `agents.recent_turn_window` | int |
| `agents.recent_turn_char_limit` | int |
| `agents.trace_payload_chars` | int |
| `agents.trace_max_bytes` | int |
| `agents.trace_max_files` | int |
| `agents.trace_rotate_check` | int |
| `agents.trace_chunk_limit` | int |
| `agents.scene_recall_turns` | int |
| `agents.scene_recall_chars` | int |
| `agents.retrieval_turns` | int |
| `agents.retrieval_chars` | int |
| `agents.retrieval_halflife_turns` | int |
| `agents.summary_every` | int |
| `agents.summary_char_limit` | int |
| `agents.thread_idle_turns` | int |
| `agents.threads_max` | int |
| `agents.continuity_checks` | bool |
| `agents.action_echo` | bool |
| `agents.completion.mode` | string |
| `agents.completion.max_attempts` | int |
| `agents.completion.tail_chars` | int |
| `agents.completion.min_incomplete_chars` | int |
| `agents.completion.timeout_seconds` | int |
| `agents.tool_rounds` | int |
| `agents.tool_result_chars` | int |

### `media`

| Key | Type |
| --- | --- |
| `media.tts.type` | string |
| `media.tts.builtin_name` | string |
| `media.tts.model_path` | string |
| `media.tts.command` | string |
| `media.tts.args` | []string |
| `media.tts.endpoint` | string |
| `media.tts.model` | string |
| `media.tts.api_key` | string |
| `media.tts.default_voice` | string |
| `media.tts.pitch` | float |
| `media.tts.speech_rate` | float |
| `media.tts.auto_play` | bool |
| `media.tts.master_volume` | float |
| `media.tts.voice_profiles[].id` | string |
| `media.tts.voice_profiles[].name` | string |
| `media.tts.voice_profiles[].voice_id` | string |
| `media.tts.voice_profiles[].provider` | string |
| `media.tts.voice_profiles[].pitch` | float |
| `media.tts.voice_profiles[].speech_rate` | float |
| `media.tts.voice_profiles[].tags` | []string |
| `media.tts.voice_profiles[].description` | string |
| `media.tts.voice_profiles[].options` | map<string, any> |
| `media.tts.markdown` | string |
| `media.tts.metered` | bool |
| `media.tts.options` | map<string, any> |
| `media.tts.speech_cues.enabled` | bool |
| `media.tts.speech_cues.audio_tags` | bool |
| `media.tts.speech_cues.markdown_emphasis` | bool |
| `media.tts.speech_cues.display_mode` | string |
| `media.tts.opus_bitrate` | int |
| `media.stt.type` | string |
| `media.stt.builtin_name` | string |
| `media.stt.command` | string |
| `media.stt.args` | []string |
| `media.stt.endpoint` | string |
| `media.stt.model` | string |
| `media.stt.api_key` | string |
| `media.image.type` | string |
| `media.image.builtin_name` | string |
| `media.image.command` | string |
| `media.image.args` | []string |
| `media.image.endpoint` | string |
| `media.image.model` | string |
| `media.image.api_key` | string |
| `media.image.auto_generate` | bool |
| `media.image.builtin_fallback` | bool |
| `media.image.aspect_ratio` | string |
| `media.image.person_generation` | string |

### `embeddings`

| Key | Type |
| --- | --- |
| `embeddings.enabled` | bool |
| `embeddings.provider` | string |
| `embeddings.model` | string |
| `embeddings.dimensions` | int |
| `embeddings.batch_size` | int |
| `embeddings.providers.<key>.type` | string |
| `embeddings.providers.<key>.builtin_name` | string |
| `embeddings.providers.<key>.endpoint` | string |
| `embeddings.providers.<key>.url` | string |
| `embeddings.providers.<key>.api_key` | string |
| `embeddings.providers.<key>.model` | string |

### `preferences`

| Key | Type |
| --- | --- |
| `preferences.streaming` | bool |
| `preferences.typing_speed_ms` | int |
| `preferences.cinematic_effects` | bool |
| `preferences.font_scale` | string |
| `preferences.trace_level` | string |

### `telemetry`

| Key | Type |
| --- | --- |
| `telemetry.enabled` | bool |
| `telemetry.endpoint` | string |
| `telemetry.protocol` | string |
| `telemetry.insecure` | bool |
| `telemetry.sample_ratio` | float |
| `telemetry.traces` | bool |
| `telemetry.metrics` | bool |
| `telemetry.logs` | bool |
| `telemetry.headers` | map<string, string> |
| `telemetry.service_name` | string |

### `mechanics`

| Key | Type |
| --- | --- |
| `mechanics.engagement` | string |
| `mechanics.cadence_turns` | int |

## Value sets

Keys typed `string` above accept the following values where noted. An
unlisted value is either rejected or falls back to the documented default.

| Key | Accepted values |
| --- | --- |
| `agents.default_role` | `gm`, `narrator`, `extractor`, `completion`, or any role defined in `agents.roles` |
| `agents.roles.<role>.type` | `builtin`, `http`, `cli`, `gemini`, `inherit`, `disabled` |
| `agents.roles.<role>.supports_tools` | `auto`, `yes`, `no` |
| `agents.completion.mode` | `auto`, `continue`, `trim`, `off` |
| `media.tts.type` | `builtin`, `http`, `cli`, `gemini`, `disabled` |
| `media.tts.markdown` | `auto`, `strip`, `keep` |
| `media.stt.type` | `builtin`, `http`, `cli`, `web-speech`, `disabled` |
| `media.image.type` | `builtin`, `http`, `cli`, `comfyui`, `gemini`, `disabled` |
| `embeddings.provider` | `builtin-local`, `openai`, `gemini`, `disabled` |
| `embeddings.providers.<id>.type` | `builtin`, `http`, `gemini`, `disabled` |
| `preferences.font_scale` | `small`, `medium`, `large` |
| `preferences.trace_level` | `off`, `summary`, `full` |
| `mechanics.engagement` | `off`, `auto`, `ask` |
