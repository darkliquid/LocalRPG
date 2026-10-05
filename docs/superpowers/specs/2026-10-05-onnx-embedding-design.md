# ONNX Embedding Provider Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#32 LF-1](https://github.com/darkliquid/LocalRPG/issues/32)
**Epic:** [#17 Zero-GPU and local-first offerings](https://github.com/darkliquid/LocalRPG/issues/17)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §2 (LF-1)
**Scope:** `pkg/embeddings`, `pkg/models`, `pkg/provider`, `pkg/config`

---

## 1. Problem

Semantic recall is the app's only use of embeddings, and the only local option is a hash projection:
`BuiltinHashProjectionProvider` (`pkg/embeddings/builtin.go:13-108`) projects words and character
n-grams into a fixed vector. It is deterministic and free, and it has **no semantic generalisation**:
"blade" and "sword" are as unrelated as "blade" and "cabbage". So the recall feature that exists to
find related content cannot.

The only in-process neural model in the tree is Sherpa-ONNX Kokoro TTS
(`pkg/provider/ttssherpa/client.go`), managed by `pkg/models`. There is no text encoder.

## 2. Goals

- A local, CPU-only text embedding provider that generalises semantically.
- Registered as `embedding:onnx`, tier `offline-neural` (LF-3), feature `offline`.
- The model is downloaded and verified by `pkg/models`, with the same "model missing" event as Kokoro.
- Falls back to the hash projection when the model is absent, so recall still works.
- No network at inference.

## 3. Non-goals

- Replacing the cloud embedding providers (OpenAI, Gemini).
- A model catalogue; one small model is enough.
- GPU inference.

## 4. Design

### 4.1 The runtime question (a spike)

Sherpa-ONNX is a **speech** library (TTS, ASR, VAD, speaker ID). It cannot run a text encoder like
BGE or MiniLM. So a text embedding provider needs a runtime the app does not have. Three options:

1. **`onnxruntime_go` (CGO).** The standard Go binding for ONNX Runtime. Needs the `onnxruntime`
   shared library alongside the binary, per platform. The repo already ships CGO and per-OS
   libraries (sherpa, oto, vpx), so this is consistent, at the cost of another shared library in
   every release.
2. **A pure-Go encoder.** Implement a small transformer in Go. Feasible for a tiny model but slow and
   a large amount of code to own.
3. **A precomputed projection.** Ship a distilled word/char projection (a static embedding table) and
   compute in pure Go. Fast, no runtime, better than the hash projection but well below a real model.

The spec's recommendation is **option 1** if the packaging cost is acceptable, because it gives a
real model; **option 3** as the fallback if adding a shared library per platform is rejected. The
first task is a spike to decide, with the model size, the per-platform library, and the licence
checked.

### 4.2 The model

A small, permissively licensed encoder, 384 dimensions to match the existing schema
(`EmbeddingsConfig.Dimensions`, default 384):

- BGE-small-en-v1.5, or
- all-MiniLM-L6-v2.

Both are ~90 MB (float32) and CPU-fast. The choice is the spike's output; the spec assumes 384 dims so
the storage schema does not change.

### 4.3 The provider

`pkg/embeddings` gains an `onnxProvider` (or `projectionProvider` for option 3) implementing the
existing interface (`Provider{ID, Dimensions, Embed}`, `pkg/embeddings/provider.go:6-10`), so
`NewProviderFromConfig` (`pkg/embeddings/factory.go:13-80`) can select it and the worker and search
paths are unchanged.

The model file is resolved through `pkg/models`:

- a `ModelSpec` for the encoder, with a URL, a pinned SHA-256, a size, and the required files;
- downloaded on demand with the existing checksum-verified extraction
  (`pkg/models/manager.go:419-457`);
- a `model_missing` event when absent, as Kokoro already emits.

When the model is missing, the provider falls back to `BuiltinHashProjectionProvider`, so a search
still returns something and the UI can prompt a download.

### 4.4 Registration

`pkg/provider/embeddingonnx/` registers `embedding:onnx` with `FeatureOffline` and tier
`offline-neural`, and `pkg/provider/all` imports it. `EmbeddingsConfig` already supports a named
provider (`Providers map[string]EmbeddingProviderConfig`), so a user selects it by name.

### 4.5 Cost and cold start

The model loads once per process (cached in the provider), like Kokoro. The first embed after a cold
start pays the load; subsequent calls are fast. Batch size is the existing `EmbeddingsConfig.BatchSize`.

## 5. Behaviour

| State | Result |
| --- | --- |
| model present | real embeddings; recall generalises |
| model absent | the hash projection; a `model_missing` event prompts a download |
| a download in progress | the existing progress event |
| a corrupt model | a load error; falls back to the hash projection |
| a cloud provider configured | unchanged; this is an alternative |

## 6. Testing

- `pkg/embeddings`: the provider's dimensions match the config; the same text embeds identically
  twice; a missing model falls back to the hash projection; a load error is reported.
- `pkg/embeddings`: with the model present (a tiny fixture), "blade" is closer to "sword" than to
  "cabbage" (the semantic check the hash projection fails).
- `pkg/models`: the encoder's spec downloads, verifies, and reports status.
- `pkg/provider`: the descriptor registers with the right tier and feature.
- A skip: the semantic test skips when the model is not available, per the repo's hardware-skip
  convention.

## 7. Rollout

Additive: a new provider and a model spec. The default embedding provider is unchanged; a user opts
in by selecting `embedding:onnx`.

## 8. Risks

- **Packaging.** Option 1 adds a shared library per platform to every release. The spike must weigh
  this; option 3 avoids it at a quality cost.
- **Model licence.** Both candidate models are permissive; the spike confirms and records it in the
  model spec.
- **Cold start.** The first embed loads the model; the worker should warm it on startup when the
  provider is selected, as the TTS path warms Kokoro.
