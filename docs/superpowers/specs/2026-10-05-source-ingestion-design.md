# Source Ingestion Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#81 WG-4](https://github.com/darkliquid/LocalRPG/issues/81)
**Epic:** [#25 AI world generation](https://github.com/darkliquid/LocalRPG/issues/25)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §8 (WG-4)
**Depends on:** [#78 WG-1](https://github.com/darkliquid/LocalRPG/issues/78), [#82 WG-5](https://github.com/darkliquid/LocalRPG/issues/82)
**Scope:** new `pkg/ingest`, `pkg/gui`, `frontend`

---

## 1. Problem

A user with a folder of setting notes, or a wiki page, or a pile of exported lore text, has no way to
turn it into a world. They must read it and retype it into the studio. The model could read it and
propose entities, but nothing feeds it the source, and there is no fetching of external text at all
(`chromedp` is used only for the app driver, `pkg/driver/driver.go:17-18`).

## 2. Goals

- **Folder ingestion**: read a directory of Markdown and text, extract lore and entities, and build a
  draft world.
- **URL ingestion**: fetch a set of URLs, extract readable text, and run the same pipeline.
- Offline-first: folder ingestion works with no network; URL ingestion is explicitly a network
  action.
- Produce a **draft** reviewed before commit (WG-5), never a live world.
- Be honest about copyright and provenance: the user supplies the source and is responsible for it.

## 3. Non-goals

- A web crawler. A bounded set of URLs the user names, not a crawl.
- JavaScript-rendered sites. A plain fetch and an HTML-to-text reduction; a browser render is a
  possible spike, not part of this.
- Whole-world generation from nothing (WG-1).

## 4. Design

### 4.1 Two sources, one pipeline

```go
// Source is one thing to ingest.
type Source struct {
	Kind string // "folder" | "url"
	Path string // for a folder
	URLs []string // for urls
}

// Extract reads a source into bounded text chunks.
func Extract(ctx context.Context, src Source) ([]Chunk, error)

// Build turns chunks into a draft world, reusing WG-1's generator.
func Build(ctx context.Context, gen Generator, chunks []Chunk, brief Brief) (Draft, error)
```

A `Chunk` is `{Source, Title, Text}` with `Text` bounded (for example 4 KB), so a large folder is
split into many small inputs the model can handle.

### 4.2 Folder ingestion

Walk the directory, read `.md` and `.txt` files, split each into chunks at headings and paragraph
boundaries, and keep the file path as the chunk's title. Skip binary files and dot-directories. This
is entirely local.

### 4.3 URL ingestion

For each URL:

1. Fetch with a plain `http.Client`, a descriptive `User-Agent`, a size cap, and a timeout.
2. Respect `robots.txt` for the host (fetch it once per host, cache it in memory).
3. Reduce HTML to text: strip tags, scripts, and styles, collapse whitespace, and keep the `<title>`
   and headings as the chunk title. A small, dependency-free reduction is enough for prose pages; it
   will not handle a JavaScript app.
4. Chunk as above.

The fetch is bounded (N URLs, total bytes) and reports per-URL failures without failing the batch.
The UI states plainly that URL ingestion fetches the network.

**Spike**: whether to render JavaScript-heavy pages with the existing `chromedp` is deferred. The
spec's baseline is a plain fetch; a follow-up can add a render mode if the plain fetch proves
insufficient.

### 4.4 Extraction and draft assembly

The model reads the chunks (batched, with the brief) and produces a draft world: an outline, lore,
and entities, as in WG-1. Where a chunk maps cleanly to an entity (a page titled "Saltmarch"), the
model is prompted to keep the mapping, so the entity's name reflects the source.

The draft is the same `Draft` type WG-1 produces, so WG-5 reviews it identically.

### 4.5 Provenance and honesty

- The draft records each entity's **source** (the chunk it came from) in a comment or a `source`
  frontmatter field, so the user can trace it.
- The UI says the user is responsible for the source's licensing; the app does not fetch copyrighted
  material on the user's behalf beyond what they name.
- Nothing is committed without review.

### 4.6 Surfaces

- **API**: `POST /api/world/ingest` (source → NDJSON steps → draft), mirroring WG-1.
- **Studio**: an "Import a source" option in the world-generation flow that takes a folder path or a
  list of URLs.

## 5. Behaviour

| Source | Result |
| --- | --- |
| a folder of Markdown | a draft world with entities from the files |
| a single page URL | a draft with lore and entities from the page |
| a URL that fails | reported; the other URLs still ingest |
| a JavaScript-only page | little or no text; reported as a likely render issue |
| an empty folder | an error, not an empty world |
| a large folder | chunked; the model is called per batch |

## 6. Testing

- `pkg/ingest`: folder extraction reads `.md`/`.txt`, skips binary and dot-dirs, and chunks at
  headings; URL extraction parses a fixture HTML into text and titles; robots disallow is honoured;
  a fetch failure does not fail the batch.
- `pkg/ingest`: a stub generator turns chunks into a draft; a chunk-to-entity mapping is preserved.
- `pkg/gui`: the endpoint streams steps and a draft; no live world is written.
- A regression guard: folder ingestion performs no network call.

## 7. Rollout

Additive: a new package, an endpoint, and a studio option. Folder ingestion is offline; URL ingestion
is a network action the user opts into.

## 8. Risks

- **Copyright and provenance.** The app is a tool; the user names the source. The provenance field and
  the notice are the guardrails. The app must not crawl.
- **Fetch reliability.** A plain fetch fails on many modern sites. The baseline is honest about it;
  the render spike is the escape hatch.
- **Prompt size.** A big source is many chunks and many model calls. The chunk cap and a batch limit
  bound it; WG-6's controls apply.
- **Politeness.** A descriptive user agent, robots.txt, one fetch per URL, and a size cap keep it
  well-behaved. No crawling means no load spike.
