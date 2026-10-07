# ONNX Embedding Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A local, CPU-only text embedding provider with real semantic generalisation, behind the existing embedding interface.

**Architecture:** A spike chooses the runtime; a model spec downloads and verifies the encoder; the provider implements `embeddings.Provider` and falls back to the hash projection when the model is absent.

**Tech Stack:** Go; the chosen ONNX runtime (or a pure-Go projection), `pkg/models`.

**Spec:** `docs/superpowers/specs/2026-10-05-onnx-embedding-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- 384 dimensions, matching the existing schema.
- The model is downloaded and checksum-verified by `pkg/models`; no network at inference.
- Tests that need the model skip when it is unavailable.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The runtime spike

**Files:**
- Create: `docs/superpowers/specs/2026-10-05-onnx-embedding-runtime-spike.md`

**Interfaces:**
- Consumes: nothing.
- Produces: a recorded decision.

- [ ] **Step 1: Evaluate the three options**

For each of `onnxruntime_go`, a pure-Go encoder, and a precomputed projection, record: the per-OS
packaging cost, the model size, CPU speed on a short text, the licence, and the effort to own.

- [ ] **Step 2: Measure a candidate**

Build a throwaway program that embeds a fixed set of texts with the chosen runtime and model, and
record the time per embed and the "blade"/"sword"/"cabbage" distances.

- [ ] **Step 3: Write the decision**

Record the choice, the model, its licence, and the packaging plan. This is a spike's output: a
recommendation, and throwaway code that is not kept.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/specs/2026-10-05-onnx-embedding-runtime-spike.md
git commit -m "docs: record the embedding runtime decision"
```

---

### Task 2: The model spec

**Files:**
- Modify: `pkg/models/manager.go`
- Test: `pkg/models/manager_test.go` (append)

**Interfaces:**
- Consumes: `ModelSpec`.
- Produces: an `embedding-encoder` spec with URL, SHA-256, size, and required files.

- [ ] **Step 1: Write the failing test**

```go
func TestEmbeddingModelSpecIsRegistered(t *testing.T) {
	m := NewManager(t.TempDir(), trace.OrNil(nil))
	found := false
	for _, s := range m.ListStatuses() {
		if s.ID == "embedding-encoder" {
			found = true
			if s.SHA256 == "" || s.TotalBytes == 0 {
				t.Fatalf("spec incomplete: %+v", s)
			}
		}
	}
	if !found {
		t.Fatal("the embedding model spec is not registered")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/models/ -run TestEmbeddingModelSpec -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Register the spec in `registerDefaultSpecs` (`pkg/models/manager.go:82-99`) with the chosen model's
URL, pinned SHA-256, size, and required files, and a variant constant.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/models/ -run TestEmbeddingModelSpec -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/models/manager.go pkg/models/manager_test.go
git commit -m "feat(models): add the embedding encoder spec"
```

---

### Task 3: The provider

**Files:**
- Create: `pkg/embeddings/onnx.go`
- Test: `pkg/embeddings/onnx_test.go`

**Interfaces:**
- Consumes: `embeddings.Provider`, `pkg/models`.
- Produces: `func NewONNXProvider(modelDir string) Provider`.

- [ ] **Step 1: Write the failing test**

```go
func TestONNXProviderIsDeterministic(t *testing.T) {
	p, err := NewONNXProvider(testModelDir(t))
	if err != nil {
		t.Skip("model unavailable")
	}
	a, _ := p.Embed(context.Background(), []string{"a sword"})
	b, _ := p.Embed(context.Background(), []string{"a sword"})
	if len(a) != 1 || len(a[0]) != 384 || a[0][0] != b[0][0] {
		t.Fatalf("embedding mismatch: %v %v", a, b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/embeddings/ -run TestONNXProvider -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement the provider: load the model from `modelDir` (lazily, cached), tokenise, run the encoder,
and L2-normalise. `Dimensions` returns 384; `ID` returns the model's id.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/embeddings/ -run TestONNXProvider -v`
Expected: PASS or SKIP.

- [ ] **Step 5: Commit**

```bash
git add pkg/embeddings/onnx.go pkg/embeddings/onnx_test.go
git commit -m "feat(embeddings): add the local ONNX provider"
```

---

### Task 4: The fallback and the factory

**Files:**
- Modify: `pkg/embeddings/factory.go`
- Test: `pkg/embeddings/factory_test.go` (append)

**Interfaces:**
- Consumes: the provider (Task 3), the hash projection.
- Produces: `KeyFor`/`NewProviderFromConfig` handle `type: onnx`, with a fallback.

- [ ] **Step 1: Write the failing tests**

```go
func TestFactorySelectsONNX(t *testing.T) {
	cfg := config.EmbeddingsConfig{Provider: "local", Providers: map[string]config.EmbeddingProviderConfig{
		"local": {Type: "onnx"}}}
	p := NewProviderFromConfig(cfg)
	if p == nil || p.ID() == "" {
		t.Fatal("expected an onnx provider")
	}
}
func TestFactoryFallsBackWhenModelMissing(t *testing.T) {
	// With no model present, the provider is the hash projection.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/embeddings/ -run TestFactory -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Handle `type: onnx` in `NewProviderFromConfig` and `KeyFor` (`pkg/embeddings/factory.go:13-110`), and
fall back to `BuiltinHashProjectionProvider` when the model is absent, logging a `model_missing`
event.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/embeddings/ -run TestFactory -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/embeddings/factory.go pkg/embeddings/factory_test.go
git commit -m "feat(embeddings): select the ONNX provider with a fallback"
```

---

### Task 5: Registration and the semantic test

**Files:**
- Create: `pkg/provider/embeddingonnx/embeddingonnx.go`
- Modify: `pkg/provider/all/all.go`, `pkg/provider/keys.go`
- Test: `pkg/provider/embeddingonnx/embeddingonnx_test.go`, `pkg/embeddings/semantic_test.go`

**Interfaces:**
- Consumes: `provider.Register`, `FeatureOffline`, `TierOfflineNeural`.
- Produces: the `embedding:onnx` descriptor.

- [ ] **Step 1: Write the failing tests**

```go
func TestONNXDescriptor(t *testing.T) {
	d, ok := provider.Lookup(provider.KeyEmbeddingONNX)
	if !ok || d.Descriptor.Tier != provider.TierOfflineNeural {
		t.Fatalf("descriptor = %+v ok %v", d, ok)
	}
}
func TestSemanticNeighbours(t *testing.T) {
	// Skip when the model is unavailable; assert "blade" is closer to "sword"
	// than to "cabbage".
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/provider/embeddingonnx/ ./pkg/embeddings/ -run 'TestONNXDescriptor|TestSemanticNeighbours' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the key `KeyEmbeddingONNX = "embedding:onnx"`, register the descriptor with `FeatureOffline` and
`TierOfflineNeural`, and import the package in `all`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/provider/... ./pkg/embeddings/ -v`
Expected: PASS or SKIP.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider pkg/embeddings/semantic_test.go
git commit -m "feat(provider): register the local ONNX embedding"
```

---

### Task 6: Verification

- [ ] **Step 1: Fallback guard**

Add a test that a missing model still answers a search via the hash projection.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- The provider embeds locally and deterministically.
- The model downloads and verifies through `pkg/models`.
- A missing model falls back.
- The descriptor is offline-neural with the right feature.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the embedding fallback"
```
