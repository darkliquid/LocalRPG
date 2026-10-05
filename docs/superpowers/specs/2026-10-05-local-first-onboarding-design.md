# Local-First Onboarding Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#36 LF-5](https://github.com/darkliquid/LocalRPG/issues/36)
**Epic:** [#17 Zero-GPU and local-first offerings](https://github.com/darkliquid/LocalRPG/issues/17)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §2 (LF-5)
**Depends on:** [#34 LF-3](https://github.com/darkliquid/LocalRPG/issues/34)
**Scope:** `pkg/gui/docs`, `tools/sitegen`, `frontend`

---

## 1. Problem

The local-first story is scattered across six articles (`14-local-llm-ollama` through
`19-local-stack-docker-compose`), each about one tool. There is no single page that answers the
question a new user actually has: **what can I run, with what hardware, and what will it be like?**

LF-3 labels each provider with a tier and a caveat in the catalogue. That is per-provider; the user
needs the overview, before they open the catalogue, to decide what to install.

## 2. Goals

- One page that presents a **capability matrix**: for each of LLM, TTS, STT, image, and embeddings,
  what runs with no GPU, with a GPU, needs a local server, or needs a key, with a plain note on
  quality and speed.
- A launcher entry that opens it, so a new user finds it before configuring anything.
- Consistent with LF-3's tiers and caveats, so the page and the catalogue agree.
- Rendered on the site and linted, like every other article.

## 3. Non-goals

- Setup instructions; the existing per-tool articles cover those, and the page links to them.
- A new behaviour or provider.
- Benchmarks; the notes are qualitative and honest, not measured numbers.

## 4. Design

### 4.1 The page

A new `pkg/gui/docs/21-local-first.md`:

1. **The short answer.** One paragraph: the app runs fully offline with the basic providers; better
   output needs a local model or a key.
2. **The matrix.** A table:

| Capability | No GPU, no model | No GPU, small model | GPU / local server | Cloud |
| --- | --- | --- | --- | --- |
| LLM (narrator) | Narrative Oracle — template prose | — | Ollama, llama.cpp, LM Studio | Gemini, OpenAI |
| TTS | `native-os` — an OS voice | Kokoro (sherpa-onnx) | Kokoro-FastAPI, Piper | ElevenLabs, Gemini, … |
| STT | browser speech (not in the desktop app) | — | faster-whisper | Cartesia, Inworld |
| Image | procedural art | — | ComfyUI, A1111, stable-diffusion.cpp | Gemini Imagen |
| Embeddings | hash projection | ONNX encoder (LF-1) | — | OpenAI, Gemini |

Each cell names the provider (linking the provider catalogue or the relevant article) and a short
quality/speed note.
3. **What each tier means.** LF-3's four tiers and their caveats, restated.
4. **The offline preset.** How to get the fully offline stack in one action (LF-4).
5. **Links** to the per-tool articles.

### 4.2 Consistency with LF-3

The tier names and caveats on the page are the strings from `provider.TierCaveat` (LF-3), so the page
and the catalogue cannot disagree. Where practical, a test asserts the page contains each tier's
label.

### 4.3 The launcher entry

The launcher's new-user path (and the About/Docs affordance) links to the page, so it is reachable
before any provider is configured. The app already renders embedded docs in a viewer
(`MarkdownDocViewer`), so this is a link, not a new surface.

### 4.4 Registration

Add the article to `tools/sitegen/content.go` and to `scripts/lint-prose.sh`, so it renders on the
site and is linted.

## 5. Behaviour

Not applicable; this is documentation. The outcome: a new user reads one page and knows what to
install.

## 6. Testing

- `mise run lint:docs` and `mise run lint:prose` pass for the new article.
- `mise run site:build` renders it.
- A test asserts the article contains each LF-3 tier label, so the two cannot drift.

## 7. Rollout

A new docs article, a launcher link, and two list entries. No behaviour change.

## 8. Risks

- **Stale matrix.** Providers change. The page is part of the repo and reviewed like the catalogue; the
  tier test catches a label drift, and the matrix is small enough to keep current.
- **Over-promising.** Qualitative notes ("well below a large model") are honest; measured numbers
  would age badly.
- **Discoverability.** A launcher link is the whole point; if it is buried, the page is not found. Put
  it in the new-user path and the docs list.
