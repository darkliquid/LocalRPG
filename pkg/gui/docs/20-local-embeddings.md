---
id: 20-local-embeddings
title: Local Semantic Search with a Built-in Encoder
category: Local AI & Self-Hosting
order: 20
description: How LocalRPG turns entities and turns into searchable vectors, why the default projection ranks unrelated words together, and how to switch on the built-in BGE encoder that separates them.
---

# Local Semantic Search with a Built-in Encoder

Recall and semantic search both rest on embeddings: LocalRPG turns every entity
and turn into a vector, then ranks the vectors closest to your query. The encoder
behind that ranking determines the quality of the search, and LocalRPG offers two,
a fast built-in projection and a small neural encoder.

## 1. How LocalRPG uses embeddings

When you record a turn, LocalRPG indexes the entities it touches. Each indexed
text becomes a fixed-length vector of 384 numbers. A search embeds your query the
same way and ranks everything by cosine similarity, so a question about a *blade*
can surface a note that only ever says *sword*.

That ranking depends on the encoder recognising that those two words mean similar
things. The default projection treats them as unrelated, while the built-in
encoder places them near each other.

## 2. Choosing an encoder

| Provider | Tier | What it is | Semantic generalisation |
| --- | --- | --- | --- |
| `embedding:builtin` | offline-basic | A hash projection over words and character n-grams. Deterministic, instant, and self-contained. | Unrelated words rank as far apart as related ones. |
| `embedding:onnx` | offline-neural | A small BGE encoder that runs in-process on the CPU. | *blade* ranks closer to *sword* than to *cabbage*. |

The built-in projection is the default and works out of the box. The encoder is
opt-in, because it downloads a model the first time you use it.

## 3. Enabling the encoder

Name the provider in `config.yaml` and set its type to `onnx`:

```yaml
embeddings:
  enabled: true
  provider: local
  dimensions: 384
  batch_size: 32
  providers:
    local:
      type: onnx
```

Download the encoder from the model manager, the same way you download the Kokoro
voice pack. It is about 34 MB, and LocalRPG verifies it against a pinned checksum
before use. Embedding then runs entirely on your machine.

To use a copy you downloaded elsewhere, point `model_path` at its directory:

```yaml
    local:
      type: onnx
      model_path: /home/you/models/bge-small-en-v1.5
```

## 4. Fallback behaviour

If the encoder or its runtime is missing, LocalRPG falls back to the built-in
projection and reports the missing model, so results still appear and the
interface can prompt you to download the encoder. Each encoder records its
vectors under a different model identifier, so switching between them never mixes
incompatible vectors.

## 5. Quality and limits

The encoder runs on your CPU and is small. It generalises far better than the
projection, and it stays well below a large cloud embedding model. Choose it when
you want semantic search on your own hardware, without a key or a GPU.
