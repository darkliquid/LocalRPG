# Frontmatter and Wikilink Intelligence Design

**Date:** 2026-10-04
**Status:** Approved
**Scope:** the generated frontmatter schema, schema-driven completion, hover and
lint in the editor, structured parse errors, and wikilink completion
**Related:** `2026-10-04-content-editor-core-design.md` (the editor these
extensions attach to), `2026-10-04-content-organisation-design.md` (the `folder`
field wikilink completion shows)
**Feature branch:** `feat/content-authoring`

---

## 1. Problem

The editor spec gives a real editor but no knowledge of what a note *is*. Two
problems remain, and both were named in the original request.

**The frontmatter is invisible and easy to break.** `entity.EntityFrontmatter`
(`pkg/entity/entity.go:27-45`) is the only place the accepted keys exist, and
nothing surfaces it. There is no way to see which keys are legal while editing,
and no warning before a save fails. `SaveEntity` (`pkg/gui/service.go:974`)
already parses before writing, so invalid YAML already blocks the save — but the
author gets a raw Go error:

```
save entity "silver-hand": parse frontmatter: yaml: line 3: did not find expected key
```

which names no key, points at no line in the editor, and suggests nothing.

**Cross-linking requires knowing an id in advance.** `[[silver-hand]]` only works
if the author already knows the slug. There is no completion over the notes that
exist, so in practice people write prose and hope an entity is later created with
a matching name.

## 2. Design

### 2.1 One generated schema

The reflection walker that already generates the configuration reference
(`walkSchema`, `yamlFieldName`, `typeLabel`, `pkg/gui/docs_schema_test.go:16-166`)
moves into shared non-test code and gains a second consumer: a committed, embedded
`pkg/gui/schema/entity-frontmatter.json` generated from
`entity.EntityFrontmatter`.

```json
{
  "allowUnknown": true,
  "keys": [
    {"name": "id", "type": "string", "required": true, "description": "..."},
    {"name": "tags", "type": "[]string", "required": false, "description": "..."}
  ]
}
```

- **`required` is free.** A field without `omitempty` is required, which is
  exactly `id`, `name` and `type`. No hand-maintained list to drift.
- **`type`** comes from the existing `typeLabel`.
- **`description`** lives beside the generator, and a test asserts every field has
  one, so a new field cannot ship undocumented.
- **`allowUnknown` is `true`** because of the `ExtraMeta` inline catch-all
  (`pkg/entity/entity.go:44`). The engine is schema-agnostic by design — the
  schema describes, it does not gate.

Staleness is enforced the way the documentation already is: `go test ./pkg/gui
-update-docs` regenerates the artifact, and the test fails with that instruction
when it is out of date.

Served at `GET /api/schema/entity-frontmatter`, added across `Service`,
`server.go`, `frontend/src/types.ts` and `frontend/src/api/client.ts` together,
which is the route convention this repo already follows.

### 2.2 Frontmatter intelligence

One module, `frontend/src/components/editor/frontmatter.ts`, holding the
schema-driven CM6 extensions:

- **Key completion.** A `CompletionSource` that fires only when the
  `yamlFrontmatter` syntax tree puts the cursor at a key position. It offers every
  known key not already present, each with its type as `detail` and its
  description as `info`.
- **Value completion.** Booleans and the enumerated sets the schema declares. This
  is what makes `type:` safe: `character`, `location`, `item` and the rest are
  offered rather than guessed.
- **Hover.** The key's type and description.
- **Lint.** A `linter()` over the frontmatter region reporting unparseable YAML
  (error, with a line), duplicate keys (error), and unknown keys as **info only**
  — "kept as extra metadata". An unknown key is never an error, because the
  engine promises to carry it.

### 2.3 Legible save errors

`entity.ParseMarkdownEntity` gains a typed `*entity.FrontmatterError` carrying the
byte offset of the failure. Because the parser knows where the frontmatter block
starts, the offset converts to a line and column without parsing the YAML library's
error string. The save route returns them in the JSON body, so the editor
highlights the offending line and shows the message inline.

The client linter is an early warning, **not a gate**: Save stays enabled. The JS
YAML parser and `gopkg.in/yaml.v3` will not agree on every edge case, and a
false positive must not trap the author with a file the server would have
accepted.

### 2.4 Wikilink intelligence

- **Completion.** A `CompletionSource` gated on
  `context.matchBefore(/\[\[[^[\]]*$/)`, offering entities matched fuzzily on
  name, id, aliases and tags, each labelled by display name with the id as `detail`
  and the folder as a breadcrumb. It fires in the body *and* inside the
  frontmatter's `location` and `faction` values, which are wikilink-bearing
  (`pkg/entity/entity.go:100-115` scans them for links).
- **Insert form is smart.** `[[silver-hand]]` when the display name slugifies to
  the id, and `[[silver-hand|Silver Hand]]` only when the label adds something.
  The machinery already supports this: `entity.WikilinkTarget` unwraps `|label`
  (`pkg/entity/entity.go:161`) and `rewriteInboundLinks` preserves the label group
  when it rewrites a merged link.
- **Data.** `GET /api/game/{id}/entities`, now carrying `folder` from the
  organisation spec, fetched once per editor session into a small cached store and
  refreshed after a save, a merge, a delete, or a completed turn. Filtering is
  client-side; a campaign holds tens to low hundreds of notes.
  `EntitySummaryDTO` gains `aliases` so alias matching has a source.

Backlinks already render in the codex (`CodexDrawer.tsx:437`) and are unchanged.

## 3. Non-Goals

- **A properties panel.** The document stays the truth; there is no second writer
  to the same frontmatter.
- **Create-on-miss** for an unmatched `[[target]]`. It needs an id, a name and a
  type, and it is its own feature.
- **Spellcheck, rendered preview, WYSIWYG.** Settled in the editor spec.
- **Changing a note's `id` from the editor.** An id change is an identity change,
  not an edit.
- **Preserving YAML comments and key order across a save.** `SaveEntity` normalises
  through `SerializeMarkdown`, and that is unchanged.
- **A schema for `state`** beyond "arbitrary keys are allowed".

## 4. Success Criteria

- At a key position inside `---`, every known key not already present is offered
  with its type and description.
- An unknown key shows an info hint, not an error, and saves without complaint.
- Unparseable frontmatter squiggles while typing; if it is saved anyway, the
  server names the same line and the editor highlights it.
- Typing `[[` lists entities by display name and picking one inserts the smart
  form; this works in the body and in `location` and `faction`.
- Adding a field to `entity.EntityFrontmatter` and running
  `go test ./pkg/gui -update-docs` makes it appear in completion with no frontend
  change.
- `go test ./...` is clean and `npx tsc --noEmit` passes.

## 5. Testing

- **`pkg/entity`** — `FrontmatterError` carries the right offset, and offset →
  line/column is correct for a document broken at a known place.
- **`pkg/gui`** — the schema artifact's staleness test; every
  `EntityFrontmatter` field has a description; the save route returns a line and
  column for a parse failure; the schema route serves the artifact.
- **Frontend** — `tsc --noEmit` is the type gate; driver tests cover `[[`
  inserting the smart form and the error-line highlight. The pure helpers
  (schema → completion options, smart insert form) live in plain modules so they
  stay reviewable without a test runner.
