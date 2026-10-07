# Embedding Provider Manager UI Design

**Date:** 2026-10-07
**Status:** Proposed
**Issue:** [#97](https://github.com/darkliquid/LocalRPG/issues/97)
**Epic:** [#16 Multiple providers of the same type](https://github.com/darkliquid/LocalRPG/issues/16)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` (topic 1) and
[#32 LF-1](https://github.com/darkliquid/LocalRPG/issues/32)
**Scope:** `pkg/config`, `pkg/gui`, `pkg/provider`, `frontend`

---

## 1. Problem

Embeddings is the only provider family with no settings interface. A user who
wants semantic recall switches on the built-in BGE encoder by hand-editing
`config.yaml` and downloading the model from the command line, because:

- The provider manager covers LLM, TTS, STT, and image
  (`frontend/src/components/ProviderManager.tsx`), but not embeddings.
- `InspectMedia` answers for `tts`, `stt`, and `image` only
  (`pkg/gui/media_inspect.go:15-44`); asking for `embedding` returns an empty
  list.
- The frontend `AppConfig` type has no `embeddings` field at all
  (`frontend/src/types.ts:981`), so the settings studio cannot read or write it.
- The generic download modal exists (`ModelDownloadModal.tsx`), but nothing
  triggers it for the encoder: the Kokoro pack is offered from the TTS panel,
  and the encoder has no panel.

The result is that LF-1's provider, and the OpenAI, Gemini, and Ollama
embedding paths that already exist in config, are reachable only by editing YAML.

## 2. Goals

- An **Embeddings** section in the provider manager, beside TTS, STT, and image.
- Choose the embeddings system from one place: the built-in hash projection, the
  built-in ONNX encoder (LF-1), or embeddings from an LLM provider (Gemini,
  OpenAI, Ollama, or any OpenAI-compatible endpoint).
- The full named-provider treatment the other families already have: list,
  add, duplicate, rename, remove, and choose the default.
- A **download** affordance for the ONNX encoder, showing installed state and
  reusing the existing model download modal.
- Enable or disable embeddings, and set dimensions and batch size, from the UI.
- Capability tier and caveat shown per entry, from the descriptor (LF-3).

## 3. Non-goals

- New embedding adapters. LF-1 adds `embedding:onnx`; `embedding:openai` (type
  `http`) and `embedding:gemini` already exist, and Ollama is reached through the
  OpenAI-compatible `http` type.
- Re-embedding stored data eagerly when the model changes. The worker keys
  vectors by model id, so a switch re-indexes lazily on the next turn.
- The one-click offline bundle (LF-4) and the onboarding page (LF-5). This is the
  family editor those build on, not a replacement.
- Per-purpose embedding routing. Embeddings have one use; there is no purpose map.

## 4. Design

### 4.1 Embeddings is a config family, not a media family

`MediaConfig` holds TTS, STT, and image, each with a singleton default, a named
map, and a purpose map (`pkg/config/media.go:42-67`). Embeddings live at
`config.Embeddings` (`pkg/config/types.go:331`) and already carry a named map
(`Providers map[string]EmbeddingProviderConfig`) plus a selector
(`Provider string`); MP-1 copied *their* shape for the other families
(`docs/proposals/2026-10-05-next-phase-deep-dive.md:164`). They are the
precedent, so the UI adapter mirrors the media one without moving the config.

Add to `pkg/config` (a new `embeddings.go`, beside `media.go`):

- `func (e EmbeddingsConfig) ProviderNames() []string` — the named entries,
  sorted. Unlike the media families there is no reserved `default` row: the
  top-level selector names the active entry, so the default is not a name.
- `func (e EmbeddingsConfig) ProviderFor(name string) EmbeddingProviderConfig` —
  the named entry; an empty name or the reserved `default` resolves to the entry
  the top-level selector names; an unknown name falls back to the built-in
  projection, so a caller always gets a usable entry.
- `func (e EmbeddingsConfig) SelectedProvider() string` — the active entry name,
  or the reserved `default` when none is chosen.

The manager renders the named entries and marks the selected one, rather than
copying a named entry into a singleton the way the media families do.

### 4.2 The frontend family adapter

`frontend/src/lib/mediaProviders.ts` defines `ProviderFamily = 'tts' | 'stt' |
'image'` and the helpers `providersKey`, `mediaEntryValue`, `setMediaEntry`, and
`entryNames`. Add a parallel `frontend/src/lib/embeddingProviders.ts` with the
same four helpers over `config.embeddings`, and widen the manager's family type
to include `'embedding'`.

`ProviderManager` (`ProviderManager.tsx`) is already family-generic: it lists,
adds, duplicates, renames, removes, selects a default, and delegates the editor
to a `renderEditor` callback. Embeddings need no purpose map, so the manager's
`purposesFor` returns none for the family, exactly as STT does today.

### 4.3 Backend inspection

`InspectMedia` gains a `case "embedding"` that walks `ProviderNames()`, resolves
each with `embeddings.KeyFor` (`pkg/embeddings/factory.go`), and reports the same
`MediaInspectEntryDTO` fields the other families use (key, tier, key required,
metered, key present). Add one embeddings-only field to the DTO:

- `ModelID string` — the model manager id a local entry needs, empty otherwise.
- `ModelInstalled bool` — whether that model is present, from
  `models.Manager.Status`.

For `embedding:onnx`, `ModelID` is `models.EmbeddingEncoderModelID` and
`ModelInstalled` comes from the manager, so the UI knows whether to show a
download button.

### 4.4 The editor and its presets

The editor is a small form whose fields follow the selected `type`:

| Type | Fields |
| --- | --- |
| `builtin` | dimensions |
| `onnx` | dimensions, model path (optional), model download |
| `http` | endpoint, api key, model, dimensions |
| `gemini` | model, api key (falls back to the shared Gemini key) |

To make the add menu useful, register descriptors' presets so the "add" flow
offers the common entries ready-made. `openaiembedding` and `geminiembedding`
declare none today; add:

- **Built-in ONNX encoder** — `{type: onnx, dimensions: 384}`.
- **OpenAI** — `{type: http, endpoint: "https://api.openai.com/v1", model:
  "text-embedding-3-small"}`.
- **Ollama** — `{type: http, endpoint: "http://localhost:11434/v1", model:
  "nomic-embed-text"}`.
- **Gemini** — `{type: gemini, model: "text-embedding-004"}`.

The presets ride the existing descriptor preset mechanism
(`pkg/provider/descriptor.go:55-61`) and the frontend's `presetsToMap`, so the
manager's add menu is populated without new plumbing.

### 4.5 The model download

Reuse `ModelDownloadModal`, which already takes a model id, name, and size and
subscribes to model status events. The ONNX entry renders a download button when
`ModelInstalled` is false and a progress bar while a download runs. The model
list, status events, and the download route already exist
(`/api/models/...`, `Service.GetModelsStatus`, `Service.SubscribeModelEvents`),
so this is wiring, not new backend.

### 4.6 Enabling and disabling

The top-level `embeddings.enabled` switch and the default selector sit at the
head of the section. Disabling hides the entries and leaves the config in place,
matching how `media` families treat an unused provider.

## 5. Behaviour

| State | Result |
| --- | --- |
| embeddings disabled | the section shows the switch only; recall uses nothing |
| builtin selected | no download; the hash projection answers searches |
| onnx selected, model present | the encoder answers; tier badge reads offline-neural |
| onnx selected, model absent | the entry offers a download; recall falls back to the projection until it completes |
| a cloud entry selected | the key field is required and the tier badge reads cloud |
| a name removed while it is the default | the selector falls back to `default` |

## 6. Testing

- `pkg/config`: `EmbeddingNames` and `EmbeddingFor` resolve the default, a named
  entry, and an unknown name to the singleton.
- `pkg/gui`: `InspectMedia{Family: "embedding"}` lists the default and named
  entries with the right key and tier, and reports `ModelInstalled` for onnx.
- `pkg/provider`: the new presets are present on their descriptors and merge into
  a valid config.
- `frontend` (Vitest): the embedding helpers round-trip a config; `ProviderManager`
  renders the embedding family and calls `renderEditor`; a removed default falls
  back.
- An e2e pass (`pkg/e2e`) that the Embeddings section renders and a provider can
  be added, following the existing settings-studio e2e tests.

## 7. Rollout

Additive. The section appears in the providers tab next to the existing families.
The config shape does not change, so an existing `config.yaml` loads unchanged
and the default embeddings provider stays what it is until the user changes it.

## 8. Risks

- **A family that is not a `MediaConfig`.** Embeddings sit at the top level, so
  the frontend adapter cannot reuse `mediaEntryValue` directly. The adapter is
  small and the shape is close, but it is a second code path to keep in step with
  the media one.
- **Key sharing.** Gemini and OpenAI embeddings can share the LLM provider's key.
  The editor must show that inheritance (as the TTS panel does) rather than
  demanding a duplicate key.
- **Download size.** The encoder is ~34 MB. The modal already reports size and
  progress, so the cost is visible before the user commits.
- **Two sources of truth for `enabled`.** The switch, the selector, and a named
  entry all imply whether embeddings run. The section must make the effective
  state obvious, the way the media families do.
