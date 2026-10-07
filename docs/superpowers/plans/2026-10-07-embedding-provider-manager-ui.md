# Embedding Provider Manager UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the embeddings family the same provider-manager treatment as TTS, STT, and image: choose the built-in projection, the built-in ONNX encoder, or embeddings from Gemini, OpenAI, or Ollama, and download the encoder from the interface.

**Architecture:** Embeddings stay at `config.Embeddings` and gain config helpers that mirror `MediaConfig`'s; `InspectMedia` learns the `embedding` family; the frontend gains an embeddings adapter and the manager handles a new family with a type-driven editor.

**Tech Stack:** Go (stdlib tests, no testify); React 19, Tailwind v4, Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-07-embedding-provider-manager-ui-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- The config shape does not change; an existing `config.yaml` loads unchanged.
- `frontend` has `strict`, `noUnusedLocals`, `noUnusedParameters`: `tsc --noEmit` is the lint gate.
- Update `Service`, `server.go`, `frontend/src/types.ts`, and `client.ts` together when an endpoint changes.
- Conventional Commits, subject under 72 chars.

## File Map

- Create: `pkg/config/embeddings.go`, `pkg/config/embeddings_test.go`
- Create: `frontend/src/lib/embeddingProviders.ts`, `frontend/src/lib/embeddingProviders.test.ts`
- Modify: `pkg/gui/media_inspect.go`, `pkg/gui/types.go`, `pkg/gui/media_inspect_test.go`
- Modify: `pkg/provider/openaiembedding/openai.go`, `pkg/provider/geminiembedding/gemini.go`
- Modify: `frontend/src/types.ts`, `frontend/src/lib/mediaProviders.ts`
- Modify: `frontend/src/components/ProviderManager.tsx`, `frontend/src/components/SettingsStudio.tsx`
- Modify: `pkg/gui/docs/20-local-embeddings.md` (point at the UI)

---

### Task 1: The embeddings config helpers

**Files:**
- Create: `pkg/config/embeddings.go`
- Test: `pkg/config/embeddings_test.go`

**Interfaces:**
- Consumes: `Config.Embeddings`, `providerNames`.
- Produces: `Config.EmbeddingNames() []string`, `Config.EmbeddingFor(name string) EmbeddingProviderConfig`.

- [ ] **Step 1: Write the failing test**

```go
func TestEmbeddingForResolvesDefaultAndNamed(t *testing.T) {
	cfg := &Config{Embeddings: EmbeddingsConfig{
		Provider:   "local",
		Dimensions: 384,
		Providers: map[string]EmbeddingProviderConfig{
			"local": {Type: "onnx"},
			"oa":    {Type: "http", Endpoint: "https://api.openai.com/v1"},
		},
	}}
	if got := cfg.EmbeddingFor("oa"); got.Type != "http" {
		t.Fatalf("EmbeddingFor(oa) = %+v", got)
	}
	if got := cfg.EmbeddingFor(""); got.Type != "onnx" {
		t.Fatalf("EmbeddingFor(default) = %+v", got)
	}
	if names := cfg.EmbeddingNames(); names[0] != "default" || len(names) != 3 {
		t.Fatalf("EmbeddingNames() = %v", names)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/config/ -run TestEmbeddingFor -v`
Expected: FAIL (undefined).

- [ ] **Step 3: Write the minimal implementation**

Add `EmbeddingNames` (reserved `default` first, then named entries sorted, reusing
`providerNames`) and `EmbeddingFor` (the named entry, else the default entry
synthesised from the top-level selector and `Embeddings.Providers[Provider]`).

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./pkg/config/ -run TestEmbeddingFor -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/embeddings.go pkg/config/embeddings_test.go
git commit -m "feat(config): resolve embedding providers by name"
```

---

### Task 2: Inspection for the embedding family

**Files:**
- Modify: `pkg/gui/media_inspect.go`, `pkg/gui/types.go`
- Test: `pkg/gui/media_inspect_test.go` (append)

**Interfaces:**
- Consumes: `Config.EmbeddingNames`/`EmbeddingFor` (Task 1), `embeddings.KeyFor`, `models.Manager.Status`.
- Produces: `InspectMedia{Family: "embedding"}` answers; `MediaInspectEntryDTO.ModelID` and `.ModelInstalled`.

- [ ] **Step 1: Write the failing test**

```go
func TestMediaInspectListsEmbeddingEntries(t *testing.T) {
	svc := newInspectService(t) // existing helper, config with embeddings enabled
	resp, err := svc.InspectMedia(context.Background(), MediaInspectRequestDTO{Family: "embedding"})
	if err != nil {
		t.Fatalf("InspectMedia: %v", err)
	}
	if len(resp.Entries) == 0 || resp.Entries[0].Name != "default" {
		t.Fatalf("entries = %+v", resp.Entries)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/gui/ -run TestMediaInspectListsEmbedding -v`
Expected: FAIL (empty list).

- [ ] **Step 3: Write the minimal implementation**

Add a `case "embedding"` to `InspectMedia` that walks `cfg.EmbeddingNames()`,
resolves each with `embeddings.KeyFor`, and sets `ModelID` to
`models.EmbeddingEncoderModelID` and `ModelInstalled` from
`s.modelsManager.Status` when the entry is an ONNX one. Extend the DTO.

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./pkg/gui/ -run TestMediaInspect -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/media_inspect.go pkg/gui/types.go pkg/gui/media_inspect_test.go
git commit -m "feat(gui): describe the embedding family for the manager"
```

---

### Task 3: Ready-made embedding presets

**Files:**
- Modify: `pkg/provider/openaiembedding/openai.go`, `pkg/provider/geminiembedding/gemini.go`, `pkg/provider/embeddingonnx/embeddingonnx.go`
- Test: the matching `*_test.go` files

**Interfaces:**
- Consumes: `provider.Preset`.
- Produces: presets on `embedding:onnx`, `embedding:openai`, `embedding:gemini`.

- [ ] **Step 1: Write the failing tests**

```go
func TestEmbeddingPresets(t *testing.T) {
	for _, key := range []provider.Key{provider.KeyEmbeddingONNX, provider.KeyEmbeddingOpenAI, provider.KeyEmbeddingGemini} {
		reg, ok := provider.Lookup(string(key))
		if !ok || len(reg.Descriptor.Presets) == 0 {
			t.Fatalf("%s has no preset", key)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/provider/... -run TestEmbeddingPresets -v`
Expected: FAIL.

- [ ] **Step 3: Write the minimal implementation**

Add the four presets from the spec (ONNX encoder, OpenAI, Ollama, Gemini) to the
matching descriptors.

- [ ] **Step 4: Run them to verify they pass**

Run: `go test ./pkg/provider/... -run TestEmbeddingPresets -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider
git commit -m "feat(provider): offer ready-made embedding presets"
```

---

### Task 4: The frontend embeddings adapter

**Files:**
- Modify: `frontend/src/types.ts` (add `embeddings` to `AppConfig`)
- Create: `frontend/src/lib/embeddingProviders.ts`, `frontend/src/lib/embeddingProviders.test.ts`

**Interfaces:**
- Produces: `EmbeddingsConfig`; `embeddingEntryValue`, `setEmbeddingEntry`, `embeddingEntryNames`.

- [ ] **Step 1: Write the failing test**

```ts
it('round-trips a named embedding entry', () => {
  const cfg = { ...baseConfig, embeddings: { enabled: true, provider: 'default', providers: {} } } as AppConfig;
  const next = setEmbeddingEntry(cfg, 'oa', { type: 'http', endpoint: 'https://api.openai.com/v1' });
  expect(embeddingEntryValue(next, 'oa').type).toBe('http');
  expect(embeddingEntryNames(next)).toEqual(['default', 'oa']);
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd frontend && npx vitest run src/lib/embeddingProviders.test.ts`
Expected: FAIL.

- [ ] **Step 3: Write the minimal implementation**

Add the `EmbeddingsConfig` type and the three helpers, mirroring
`mediaProviders.ts` but over `config.embeddings`.

- [ ] **Step 4: Run it to verify it passes**

Run: `cd frontend && npx vitest run src/lib/embeddingProviders.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/lib/embeddingProviders.ts frontend/src/lib/embeddingProviders.test.ts
git commit -m "feat(frontend): add an embeddings config adapter"
```

---

### Task 5: The manager handles the embedding family

**Files:**
- Modify: `frontend/src/lib/mediaProviders.ts`, `frontend/src/components/ProviderManager.tsx`
- Test: `frontend/src/components/ProviderManager.test.tsx`

**Interfaces:**
- Consumes: the adapter (Task 4), the presets (Task 3), the inspect DTO (Task 2).
- Produces: `family="embedding"` renders, with a type-driven editor and a download button for the encoder.

- [ ] **Step 1: Write the failing tests**

```tsx
it('renders the embedding family and its editor', async () => {
  render(<ProviderManager family="embedding" config={cfg} onChange={() => {}} />);
  expect(await screen.findByText(/embedding/i)).toBeInTheDocument();
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd frontend && npx vitest run src/components/ProviderManager.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Write the minimal implementation**

Widen the family type, branch the address helpers to the embeddings adapter, and
render the type-driven editor (builtin/onnx/http/gemini) with the download
button wired to `ModelDownloadModal` when `ModelInstalled` is false.

- [ ] **Step 4: Run them to verify they pass**

Run: `cd frontend && npx vitest run src/components/ProviderManager.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/mediaProviders.ts frontend/src/components/ProviderManager.tsx frontend/src/components/ProviderManager.test.tsx
git commit -m "feat(frontend): manage the embedding family"
```

---

### Task 6: Wire the section and verify

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`, `pkg/gui/docs/20-local-embeddings.md`
- Test: `pkg/e2e` (append a settings-studio case)

- [ ] **Step 1: Render the section**

Add the Embeddings section to the providers tab beside TTS, STT, and image.

- [ ] **Step 2: Add the e2e case**

A test that the Embeddings section renders and a provider can be added, following
the existing settings-studio e2e tests.

- [ ] **Step 3: Point the guide at the UI**

Update `pkg/gui/docs/20-local-embeddings.md` to describe the settings section as
the way to switch on the encoder, keeping the `config.yaml` form for reference.

- [ ] **Step 4: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 5: Confirm the acceptance criteria**

- The Embeddings section lists, adds, and removes entries.
- The built-in encoder, OpenAI, Ollama, and Gemini are offered as presets.
- The encoder shows a download button when its model is absent.
- Tier and caveat show per entry.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(frontend): add the embeddings section to settings"
```
