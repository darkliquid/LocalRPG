---
id: 20-local-embeddings
title: Local Semantic Search with a Built-in Encoder
category: Local AI & Self-Hosting
order: 20
description: How LocalRPG turns entities and turns into searchable vectors, why the default projection cannot tell "sword" from "cabbage", and how to switch on the built-in BGE encoder that can.
---

# Local Semantic Search with a Built-in Encoder

Recall and semantic search both rest on embeddings: every entity and turn is
turned into a vector, and a search finds the vectors closest to your query. How
good that search is depends entirely on the encoder behind it, and LocalRPG ships
two: a fast built-in projection and a small neural encoder.

## 1. What embeddings do here

When you record a turn, LocalRPG indexes the entities it touches. Each indexed
text becomes a fixed-length vector of 384 numbers. A search embeds your query the
same way and ranks everything by cosine similarity, so a question about a *blade*
can surface a note that only ever says *sword*.

That only works if the encoder understands that those two words mean similar
things. The default projection does not, and the built-in encoder does.

## 2. The two encoders

| Provider | Tier | What it is | Semantic generalisation |
| --- | --- | --- | --- |
| `embedding:builtin` | offline-basic | A hash projection over words and character n-grams. Deterministic, instant, no model. | None. Related words are no closer than unrelated ones. |
| `embedding:onnx` | offline-neural | A small BGE encoder run in-process on the CPU. | Yes. "blade" is closer to "sword" than to "cabbage". |

The built-in projection is the default and needs nothing. The encoder is opt-in,
because it downloads a model the first time you use it.

## 3. Switching on the encoder

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

Then download the encoder from the model manager, the same way the Kokoro voice
pack is downloaded. The encoder is about 34 MB and is verified against a pinned
checksum before it is used. No network call is made when it embeds.

If you already downloaded the model elsewhere, point at its directory with
`model_path` instead of relying on the default cache:

```yaml
    local:
      type: onnx
      model_path: /home/you/models/bge-small-en-v1.5
```

## 4. What happens when the model is missing

Nothing breaks. When the encoder or its runtime is not available, LocalRPG falls
back to the built-in projection and reports a missing model, so a search still
returns results and the interface can prompt you to download the encoder. The two
encoders store their vectors under different model identifiers, so switching
between them never mixes incompatible vectors.

## 5. What it is not

The encoder runs on your CPU and is small. It generalises far better than the
projection, and it is not in the same league as a large cloud embedding model. It
is the right choice when you want semantic recall with no key, no GPU, and no
network call.
