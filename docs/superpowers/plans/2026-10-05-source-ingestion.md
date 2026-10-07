# Source Ingestion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn a folder of text or a set of URLs into a draft world, offline for folders and network-opt-in for URLs.

**Architecture:** `pkg/ingest` extracts bounded chunks (folder walk or polite URL fetch) and reuses WG-1's generator to build a `Draft`; an endpoint streams it and the studio offers the source option.

**Tech Stack:** Go standard library (`net/http`, `html` or a small tag stripper, `path/filepath`).

**Spec:** `docs/superpowers/specs/2026-10-05-source-ingestion-design.md`
**Depends on:** WG-1, WG-5.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- Folder ingestion performs no network call.
- URL ingestion respects robots.txt, uses a descriptive user agent, and is bounded.
- No crawling; only the URLs the user names.
- Conventional Commits, subject under 72 chars.

---

### Task 1: Chunks and folder extraction

**Files:**
- Create: `pkg/ingest/ingest.go`
- Test: `pkg/ingest/ingest_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Source`, `Chunk`, `func ExtractFolder(dir string) ([]Chunk, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestExtractFolderReadsTextAndSkipsBinaries(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# Saltmarch\n\nA port.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("Notes.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.png"), []byte{0x89, 0x50}, 0o644)
	chunks, err := ExtractFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2", len(chunks))
	}
	if chunks[0].Title == "" {
		t.Fatal("a chunk should carry its source title")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/ingest/ -run TestExtractFolder -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Walk the directory, read `.md`/`.txt`, split at headings and paragraph boundaries, cap each chunk's
text (4 KB), and set the chunk title from the file path.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/ingest/ -run TestExtractFolder -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/ingest/ingest.go pkg/ingest/ingest_test.go
git commit -m "feat(ingest): extract chunks from a folder"
```

---

### Task 2: URL extraction with robots

**Files:**
- Create: `pkg/ingest/fetch.go`
- Test: `pkg/ingest/fetch_test.go`

**Interfaces:**
- Consumes: `Chunk`.
- Produces: `func ExtractURLs(ctx context.Context, urls []string, opts FetchOptions) ([]Chunk, []error)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestExtractURLsReducesHTMLToText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}
		w.Write([]byte("<html><head><title>Saltmarch</title></head><body><h1>Saltmarch</h1><p>A port.</p><script>x</script></body></html>"))
	}))
	defer srv.Close()
	chunks, errs := ExtractURLs(context.Background(), []string{srv.URL + "/page"}, FetchOptions{})
	if len(errs) != 0 || len(chunks) != 1 {
		t.Fatalf("chunks %d errs %v", len(chunks), errs)
	}
	if !strings.Contains(chunks[0].Text, "A port.") || strings.Contains(chunks[0].Text, "<script>") {
		t.Fatalf("text = %q", chunks[0].Text)
	}
}

func TestExtractURLsHonoursRobots(t *testing.T) { /* a Disallow yields an error, no chunk */ }
func TestExtractURLsOneFailureDoesNotFailTheBatch(t *testing.T) { /* other URLs still ingest */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/ingest/ -run TestExtractURLs -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

For each URL: fetch robots.txt once per host and honour it, fetch the page with a descriptive user
agent and a size cap, reduce HTML to text (strip tags/scripts/styles, keep the title and headings),
and chunk. Collect per-URL errors without failing the batch.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/ingest/ -run TestExtractURLs -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/ingest/fetch.go pkg/ingest/fetch_test.go
git commit -m "feat(ingest): fetch and reduce URLs"
```

---

### Task 3: Build a draft from chunks

**Files:**
- Modify: `pkg/ingest/ingest.go`
- Test: `pkg/ingest/build_test.go`

**Interfaces:**
- Consumes: WG-1's `Generator` and `Draft`.
- Produces: `func Build(ctx context.Context, gen Generator, chunks []Chunk, brief Brief) (Draft, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestBuildProducesADraftWithProvenance(t *testing.T) {
	chunks := []Chunk{{Source: "a.md", Title: "Saltmarch", Text: "A port."}}
	g := &jsonGen{responses: []string{`{"entities":[{"name":"Saltmarch","type":"location"}]}`}}
	d, err := Build(context.Background(), g, chunks, Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Entities) != 1 || d.Entities[0].Source != "a.md" {
		t.Fatalf("draft = %+v", d)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/ingest/ -run TestBuild -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Batch the chunks, prompt the generator to produce an outline, lore, and entities with each entity's
source recorded, and assemble a `Draft`. Reuse WG-1's link step so the draft's links resolve.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/ingest/ -run TestBuild -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/ingest/ingest.go pkg/ingest/build_test.go
git commit -m "feat(ingest): build a draft world from chunks"
```

---

### Task 4: The endpoint and the studio option

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/gui/types.go`
- Modify: `frontend/src/components/WorldGenerateDialog.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `pkg/gui/ingest_test.go`

**Interfaces:**
- Consumes: `Extract`, `Build`.
- Produces: `POST /api/world/ingest` (NDJSON) and a source option in the generation flow.

- [ ] **Step 1: Write the failing test**

```go
func TestIngestEndpointStreamsAndWritesNothing(t *testing.T) {
	// Folder ingest streams steps and a draft; the worlds dir is unchanged.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestIngestEndpoint -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the endpoint (extract → build → stream steps and the draft, persisting to `worlds/.drafts/`), and
a source option in the generation dialog that accepts a folder path or URLs, with a network notice
for URLs.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestIngestEndpoint -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "feat: ingest a source into a draft world"
```

---

### Task 5: Verification

- [ ] **Step 1: Offline guard**

Add a test asserting folder ingestion makes no HTTP call (a fake transport that fails the test if used).

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Folder ingestion is offline and chunked.
- URL ingestion honours robots and reports per-URL failures.
- A draft is produced with entity provenance and resolving links.
- Nothing is committed without review.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the offline folder path"
```
