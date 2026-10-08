---
id: 23-ai-world-generation
title: AI World Generation
category: Studio Guides
order: 23
description: Draft a world from a sentence, a folder, or a set of pages, then review it before it is committed.
---

# AI World Generation

The Worlds Studio drafts whole worlds from a short brief, fills an existing world with new entities, proposes lore and story hooks for it, and builds one from source material you already have. Each of those paths produces a **draft** you review first. LocalRPG writes into `worlds/<id>/` only after you accept the draft.

## Draft contents

A draft contains a world's identity (name, genre, description, art style, tags), its lore split into sections, and a list of entity notes. It lives in `worlds/.drafts/<id>.yaml`, which the indexer skips. The studio never treats a draft as a world. Committing or discarding deletes the file.

Every generated entity is a real note: frontmatter with an `id`, `name`, and `type`, and a body that may include `[[wikilinks]]` to the other entities in the draft. LocalRPG flattens a link that resolves to nothing into plain text instead of leaving it dangling, and the review lists it as a dropped link.

## The brief

Open the Worlds Studio and click **Generate**. The dialog collects:

- **Premise**: the sentence the world grows from.
- **Name** and **Genre**: optional, and filled in by the model when you leave them empty.
- **Locations**, **Factions**, and **Characters**: how many of each to produce, up to 10.

## Pipeline steps

A generation runs as four structured model calls, and each one reports progress as it finishes:

1. **Outline** produces the world's identity and lore.
2. **Places and factions** produces the locations and the factions based there.
3. **Characters** produces the people, seeded with what the earlier steps wrote.
4. **Cross-link** writes each entity's body with `[[wikilinks]]` to the others.

Each step is a separate call, so a failure at step three keeps the outline and the places. Cancelling closes the stream, aborts the call in flight, and saves nothing.

## Reviewing and committing

The draft opens in a review with a count of what you have accepted:

- **Lore sections** and **entities** each have an accept or reject toggle.
- You can **edit** an entity in place before accepting it, and the edit is what LocalRPG writes.
- **Commit** writes the accepted set. With nothing accepted, the Commit button is unavailable.
- **Discard** deletes the draft.

Committing a draft for a new world writes `world.yaml`, `prompts/lore.md`, and the accepted entity notes. LocalRPG stages the write in a temporary directory and renames it into place. A failed commit discards the staging directory. Committing into an existing world appends the accepted lore and writes the accepted entities. An entity id that is already taken is refused rather than overwritten.

## Importing a source

The **Source** row in the dialog has three modes:

| Source | What it does |
| --- | --- |
| **Brief** | Generates from the premise alone. |
| **Folder** | Reads `.md`, `.markdown`, and `.txt` files under a directory. Entirely local, with no network call. |
| **URLs** | Fetches the pages you list, one per line, and reduces each to readable text. |

Folder ingestion splits each file into chunks at headings and paragraph boundaries, and skips binary files and dot-directories. An empty folder is an error rather than an empty world.

URL ingestion is a network action you opt into. It sends a descriptive user agent, fetches `robots.txt` once per host and honours it, caps each page at 2 MB, and fetches at most 20 URLs. It does **not** crawl: LocalRPG fetches only the URLs you name. A page that fails is reported, and the rest of the batch still ingests. A JavaScript-only page yields little or no text, and LocalRPG reports that as a likely render issue.

Each entity built from a source records the chunk it came from in its `source` frontmatter field, so you can trace it. You are responsible for the licensing of anything you ingest.

## Generating entities for an existing world

Select a world and click **Generate entities**. The dialog takes an instruction ("three rival factions in the south"), a kind, a count, and an optional focus that anchors the batch to an existing entity.

LocalRPG seeds the batch with the world's lore and its existing entities. The new entities fit the established tone. Links resolve against the batch **and** the existing world. An unresolved link is dropped and listed in the preview. The preview is read-only. **Accept** writes each note and renames it to its frontmatter id. When an id is already taken, the accept is refused unless you tick **Rename on an id clash**, which suffixes the new note instead.

## Enhancing a world

Select a world and click **Enhance**. The dialog takes an instruction and returns a list of proposals:

| Kind | What applying it does |
| --- | --- |
| **Lore** | Appends a Markdown section to `prompts/lore.md`, under a named heading when one exists. |
| **Hook** | Appends a story seed under a `## Hooks` section, and creates that section when the lore has none. |
| **New entity** | Writes a new entity note, linked to the world's existing entities. |

Each proposal includes the reason the model suggests it. Reject the ones you do not want, then click **Apply accepted**, and LocalRPG writes only those. It appends to existing prose and never replaces it, and it leaves every existing entity untouched.

## Cost controls

A generation costs several model calls, and the dialog reports the price before you spend it. It runs a dry run, which reports the call count and, when the provider has a published rate, the estimated cost. An unpriced provider is labelled `unpriced` rather than shown as free.

When the estimate reaches 10 calls, **Generate** asks for a second click to confirm. A per-generation call cap, `generation.max_calls`, stops a runaway. When the cap trips, the error states the setting to raise. After a generation runs, the calls and tokens it used are recorded in the usage ledger under the `generator` role, and the Usage view shows what generation cost.

## Working offline

World generation works offline. When no agent role can generate, LocalRPG uses a deterministic template generator that fills the same fields from fixed pools, and the review states that plainly. Treat the result as a fallback that produces something usable without a provider.

LocalRPG prefers an explicit `generator` role when you define one, and otherwise the `gm` role. The default configurations (the `echo` command and the built-in narrative oracle) count as unavailable, because neither one can generate.

```yaml
agents:
  roles:
    generator:
      type: http
      endpoint: "http://localhost:11434/v1"
      model: "llama3.1:8b"

generation:
  max_calls: 30
```

## Related

- [Building Worlds Studio Guide](08-worlds-studio) for authoring a world by hand.
- [Editing Content Outside the App](22-editing-content) for the on-disk layout a commit produces.
- [Usage, Cost & Provider Limits](11-usage-and-pricing) for the ledger the `generator` role records into.
