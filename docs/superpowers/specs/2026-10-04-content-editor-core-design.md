# Content Editor Core Design

**Date:** 2026-10-04
**Status:** Approved
**Scope:** the editor component, its language wiring, the studio and codex
surfaces it replaces, the new Content Studio surface, and the frontend bundle
**Related:** `2026-10-04-content-organisation-design.md` (the `EntityTree` this
surface hosts), `2026-10-04-frontmatter-and-wikilink-intelligence-design.md`
**Feature branch:** `feat/content-authoring`

---

## 1. Problem

Every markdown and YAML surface in the app is a bare `<textarea>`:

| Surface | File |
| --- | --- |
| Codex entity note | `frontend/src/components/CodexDrawer.tsx:424` |
| World entity template | `frontend/src/components/WorldsStudio.tsx:1062` |
| World lore prompt | `frontend/src/components/WorldsStudio.tsx:983` |
| System rules prompt | `frontend/src/components/SystemsStudio.tsx:674` |
| System `mechanics.js` | `frontend/src/components/SystemsStudio.tsx:658` |

They are `font-mono`, `spellCheck={false}`, `resize-none`, and offer no line
numbers, no syntax highlighting, no search, no indentation handling and no
keyboard save. Authoring a note means editing raw text with none of the
affordances a modern editor gives, in a panel that is often a few hundred pixels
wide.

The fix is one real editor component, used everywhere a document is edited, plus
a full-screen surface where there is actually room to write.

## 2. Design

### 2.1 Component and API

One component, `frontend/src/components/editor/MarkdownEditor.tsx`:

```ts
interface MarkdownEditorProps {
  value: string;                 // initial document; see §2.5
  onChange: (next: string) => void;
  language: 'markdown' | 'markdown-frontmatter' | 'yaml';
  onSave?: () => void;           // wired to Mod-S
  readOnly?: boolean;
  placeholder?: string;
  minHeight?: string;
  ariaLabel: string;
}
```

CodeMirror 6 owns its document, so the component is deliberately
**uncontrolled**: `value` seeds the editor and `onChange` fires on every
transaction. The parent must therefore pass a `key` that changes with the
document's identity — the entity id, or the file path. Keying on identity is what
keeps undo history and the cursor correct and avoids the well-known controlled-CM6
cursor-jump bug.

The existing call sites already have exactly this shape, which is why the swap is
mechanical: `onChange={(e) => { setMarkdown(e.target.value); markDirty(); }}`
becomes `onChange={(next) => { setMarkdown(next); markDirty(); }}`.

### 2.2 Language wiring

`basicSetup` from the `codemirror` meta-package covers line numbers, active-line
highlight, bracket matching, history, in-document search and indentation. The
component adds the language and the theme:

- `markdown` → `markdown()` (`@codemirror/lang-markdown`)
- `markdown-frontmatter` → `yamlFrontmatter({ content: markdown() })`
  (`@codemirror/lang-yaml`), so a note is parsed as markdown whose head is a YAML
  block
- `yaml` → `yaml()`

The theme is a single `EditorView.theme` built from the app's existing
stone/purple tokens in `frontend/src/index.css`, so the editor is not a foreign
widget dropped into the design. `EditorView.lineWrapping` is on and `indentWithTab`
is bound.

### 2.3 Surfaces replaced

| Surface | Language | Change |
| --- | --- | --- |
| `CodexDrawer.tsx:424` entity note | `markdown-frontmatter` | replace the textarea |
| `WorldsStudio.tsx:1062` world entity template | `markdown-frontmatter` | replace the textarea |
| `WorldsStudio.tsx:983` lore prompt | `markdown` | replace the textarea |
| `SystemsStudio.tsx:674` rules prompt | `markdown` | replace the textarea |
| `SystemsStudio.tsx:658` `mechanics.js` | `javascript` | replace the textarea |
| `WorldsStudio.tsx:832`, `SystemsStudio.tsx:489` description fields | — | unchanged; these are short `*.yaml` scalars edited as form fields |

### 2.4 Content Studio

A new full-screen surface, the home for everything the studios cannot show at
once: the `EntityTree` from the organisation spec on the left, `MarkdownEditor` on
the right, and tabs for the three document kinds — entity notes, prompts
(`prompts/lore.md`, `prompts/rules.md`), and the manifest documents
(`world.yaml`, `system.yaml`, `game.yaml`) as raw `yaml`.

It is deliberately built after the in-place swaps, so something useful ships
first, and it is the surface the intelligence spec's frontmatter panel and
wikilink completion plug into.

### 2.5 Save and lifecycle

Explicit save is preserved. This is not a stylistic choice: the studios' draft
model depends on it. `markDirty()` gates the Save button and `DiscardDraftConfirm`
guards an unsaved draft, and autosave would write half-typed frontmatter to disk
and fight the discard guard.

- `onChange` sets the parent's dirty flag exactly as the textarea did.
- `onSave` invokes the existing save handler.
- `Mod-S` is bound to `onSave` and prevented from reaching the browser.
- A dirty indicator sits beside the existing Save button.
- The editor never writes to disk itself.

### 2.6 Bundle constraint

The editor is app-only. `frontend/vite.player.config.ts` builds the separate
single-file export player, which has no editing at all, so CM6 must not be
reachable from it. Every editor import lives under app-only components, and the
build check in §4 asserts CodeMirror is absent from the player output. The app
bundle grows by roughly 200-250 KB gzipped; **the player bundle must not grow at
all**.

## 3. Non-Goals

- Live-preview, WYSIWYG, or any rendered preview pane. The editor is source-only.
- Spellcheck. The native path is broken on a CM6 editing host in Chromium — the
  `spellcheck` attribute silently does nothing — so a bundled dictionary
  extension is the only real option and it is deferred.
- JavaScript editing for world hooks. `mechanics.js` is covered; a world hook
  file has no editor surface in the app yet, so there is nothing to swap.
- Structured frontmatter forms, frontmatter key completion, and wikilink
  completion. All three are the intelligence spec.
- Autosave, vim keymaps, and collaborative editing.

## 4. Success Criteria

- Every textarea in §2.3 is the CM6 editor; no raw markdown textarea remains in
  the studios or the codex.
- Typing `[[` in a note is inert — no crash, and no completion until the
  intelligence spec lands.
- The player bundle build output contains no CodeMirror reference and does not
  grow.
- The app builds and runs with no CDN, no web worker, and no network access; the
  offline `go:embed` guarantee holds.
- The dirty, save and discard flows behave exactly as they did before.
- `npx tsc --noEmit` is clean and `go test ./...` passes.

## 5. Testing

This repo has no frontend test runner, so `tsc --noEmit` is the type gate and
behaviour is exercised through the Go `pkg/driver` browser tests, which skip when
`driver.Available` is false rather than failing on a host without a browser.

- **Type gate** — `npx tsc --noEmit` (the existing `mise run test:frontend`).
- **Driver** — open the codex, select a note, type, and assert the dirty state and
  the save round-trip through `POST`/`PUT` still work.
- **Bundle assertion** — build `vite.player.config.ts` and grep the output for
  CodeMirror; the check is what holds §2.6.
- Extension assembly and language selection are kept in plain modules so they
  stay small and reviewable without a test runner.
