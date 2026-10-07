# ONNX Embedding Runtime Spike

**Date:** 2026-10-05
**Status:** Decision
**Issue:** [#32 LF-1](https://github.com/darkliquid/LocalRPG/issues/32)
**Spec:** `docs/superpowers/specs/2026-10-05-onnx-embedding-design.md` §4.1

---

## 1. Question

The design spec asks for a local, CPU-only text embedding provider with real
semantic generalisation, and §4.1 asks for a spike to choose the runtime before
any of it is built. Three options were on the table:

1. **`onnxruntime_go` (CGO).** The standard Go binding for ONNX Runtime, with a
   per-platform shared library.
2. **A pure-Go encoder.** A small transformer written in Go.
3. **A precomputed projection.** A static embedding table computed in pure Go.

This note records what each costs and which one the implementation uses.

## 2. What each option costs

| Option | Per-OS packaging | Model size | Speed (short text) | Licence to carry | Effort to own |
| --- | --- | --- | --- | --- | --- |
| 1. `onnxruntime_go` | **none new**: reuses the `libonnxruntime` Sherpa already ships | ~34 MB int8, ~130 MB fp32 | ~0.5 ms/embed, warm (measured) | ONNX Runtime (MIT); model MIT | low: tokenizer plus tensor plumbing |
| 2. pure-Go encoder | none | same weights, if converted | far slower (no SIMD kernels) | model MIT | very high: own the kernels and the graph |
| 3. precomputed projection | none | a static table, megabytes | microseconds | table's own licence | medium, and it is a distillation, not a model |

**Option 2 is out.** A correct transformer in Go is weeks of work and would be
slower than ONNX Runtime on the same weights.

**Option 3 is the fallback, not the choice.** A static word table generalises
better than the FNV hash projection, but it is a fixed vocabulary and cannot
embed a sentence it has not seen; it would not reach the quality bar the spec
sets for recall.

**Option 1 is the choice.** It gives a real encoder, it is fast on CPU, and the
model is small.

### The packaging objection, answered

The proposal (`docs/proposals/2026-10-05-next-phase-deep-dive.md`, topic 2) warns
that a second ONNX runtime is bloat and prefers reusing Sherpa's. The spec
(§4.1) assumed Sherpa could not be reused because it is a speech library. Both
are half right, and the resolution is better than either expected:

- Sherpa's **Go API** cannot run a text encoder. It is TTS, ASR, VAD and speaker
  ID only.
- Sherpa's **runtime** can. The `sherpa-onnx-go-linux`, `-macos` and `-windows`
  modules each bundle a full `libonnxruntime.{so,dylib}` / `onnxruntime.dll`
  (1.28.2) per architecture, and the app links it already: `pkg/provider/all`
  imports the Sherpa TTS provider, so `libonnxruntime` is a `DT_NEEDED` of every
  build and its directory is on the binary's `DT_RUNPATH`.

So no second runtime is added. Only the pure-Go binding `onnxruntime_go` is
added, and it contains no runtime of its own: it loads one with
`dlopen`/`LoadLibrary` at run time. Pointing it at `libonnxruntime.so` resolves
to the library Sherpa already ships.

This was verified on Linux: a binary that blank-imports `sherpa_onnx` and calls
`onnxruntime_go`'s `SetSharedLibraryPath("libonnxruntime.so")` then
`InitializeEnvironment()` initialises successfully against the bundled 1.28.2
runtime. The binding's header declares `ORT_API_VERSION 25`, and the bundled
library answers `GetApi(25)` (it supports 1 through 28), so the two agree. The
same layout exists for macOS (`libonnxruntime.dylib`) and Windows
(`onnxruntime.dll`).

The library is still resolved at run time and the provider still degrades to the
hash projection when it cannot be loaded (design §4.3, §5), so a machine without
Sherpa's libraries loses the neural path, not recall itself. The library name
can be overridden with `LOCALRPG_ONNXRUNTIME_LIB`.

## 3. The model

**`Xenova/bge-small-en-v1.5`**, the ONNX export of **`BAAI/bge-small-en-v1.5`**.

- **Architecture:** BERT, 384 hidden dimensions, 33 M parameters.
- **Licence:** MIT (the base model's card declares `license: mit`).
- **Files, and their pinned hashes:**
  - `onnx/model_quantized.onnx` — int8, 34,014,426 bytes,
    `sha256:6c9c6101a956d62dfb5e7190c538226c0c5bb9cb27b651234b6df063ee7dbfe4`
  - `vocab.txt` — 231,508 bytes,
    `sha256:07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3`
- **Interface:** inputs `input_ids`, `attention_mask`, `token_type_ids`
  (`int64`, dynamic batch and sequence); output `last_hidden_state`
  `[batch, sequence, 384]`.
- **Pooling:** the CLS token (index 0), then L2 normalise, which is what BGE
  documents for retrieval.

384 dimensions match `EmbeddingsConfig.Dimensions`, so the storage schema does
not change.

## 4. The measurement

A throwaway program loaded the quantized model through `onnxruntime_go`
(v1.29.0) against the system `libonnxruntime.so.1.29.0`, tokenised four short
texts, ran one batched session, and pooled the CLS token. It is not kept.

```text
onnxruntime version: 1.29.0
batch of 4, seq 7, run took 1.98ms
  cos("a blade", "a sword")              = 0.8581
  cos("a blade", "a cabbage")            = 0.6587
  cos("a blade", "the knight drew his blade") = 0.7692
per-run (batch of 4) over 20 runs: 1.83ms
```

The semantic check the hash projection fails holds: *blade* is closer to
*sword* (0.858) than to *cabbage* (0.659). Throughput is ample for a background
indexer.

## 5. Decision

- **Runtime:** reuse Sherpa's. Add only the pure-Go binding
  `github.com/yalue/onnxruntime_go` v1.28.0 (its header declares
  `ORT_API_VERSION 25`, which the bundled 1.28.2 runtime answers). The runtime
  itself is the `libonnxruntime` the `sherpa-onnx-go-*` modules already ship per
  platform; nothing new is packaged.
- **Model:** `Xenova/bge-small-en-v1.5` quantized, MIT, 384 dimensions, pinned by
  SHA-256, downloaded on demand through `pkg/models`.
- **Fallback:** when the library or the model is absent, the provider reports a
  load error and the factory returns `BuiltinHashProjectionProvider`, so recall
  still answers and the UI can prompt a download.
- **Library resolution:** `LOCALRPG_ONNXRUNTIME_LIB` when set, else the
  per-OS name Sherpa ships (`libonnxruntime.so`, `libonnxruntime.dylib`,
  `onnxruntime.dll`), resolved through the binary's `DT_RUNPATH`. The library is
  loaded at run time, so a build never depends on it.
