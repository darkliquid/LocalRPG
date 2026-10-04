# Content Editor Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace every raw markdown textarea in the studios and the codex with one CodeMirror 6 editor that has highlighting, line numbers and keyboard save, and add a full-screen Content Studio where a document has room to be written.

**Architecture:** One uncontrolled `MarkdownEditor` component owns a CodeMirror 6 document and is keyed per document identity, so the parent keeps the existing `markDirty()` / Save / Discard flow untouched. Language support is a three-way switch (`markdown`, `markdown-frontmatter`, `yaml`) over `@codemirror/lang-markdown` and `@codemirror/lang-yaml`. The Content Studio is a lazy-loaded full-screen surface, mirroring how `DocsModal` is opened from `MenuBar`.

**Tech Stack:** CodeMirror 6 (`codemirror`, `@codemirror/lang-markdown`, `@codemirror/lang-yaml`, `@codemirror/view`, `@codemirror/state`, `@codemirror/commands`), React 19, TypeScript, Tailwind v4, Vite 6.

**Spec:** `docs/superpowers/specs/2026-10-04-content-editor-core-design.md`

**Depends on:** `docs/superpowers/plans/2026-10-04-content-organisation.md` (Task 9 provides `EntityTree`, Task 8 provides the folder client methods). Task 7 of this plan uses both.

## Global Constraints

- The editor is **uncontrolled**: `value` seeds the document and `onChange` reports every transaction. The parent must pass a `key` that changes with the document identity.
- Explicit save stays. Do not add autosave; `markDirty()` gates the Save button and `DiscardDraftConfirm` guards an unsaved draft.
- The editor never writes to disk. `onSave` calls the parent's existing save handler.
- The app must keep building with **no CDN, no web worker, and no network access**. CodeMirror 6 is bundled, not fetched.
- The export player (`frontend/vite.player.config.ts`, entry `player.html`) must not contain CodeMirror. Its input graph is separate from `main.tsx`, so this holds unless a player component imports an editor; Task 8 asserts it.
- `tsconfig.json` sets `strict`, `noUnusedLocals`, `noUnusedParameters`. `npm run build` fails on an unused import.
- No spellcheck (deferred), no WYSIWYG, no rendered preview, no JavaScript editing.
- `go vet ./...` and `go test ./...` must stay clean.
- Commits are Conventional Commits with a scope; subject under 72 characters.

---

### File Map

- **`frontend/package.json`** — the CodeMirror dependencies and the player-bundle check script.
- **`frontend/scripts/checkPlayerBundle.mjs`** (new) — fails when CodeMirror reaches the player output.
- **`frontend/src/components/editor/theme.ts`** (new) — the `EditorView.theme` built from the app's palette.
- **`frontend/src/components/editor/languages.ts`** (new) — the pure language switch.
- **`frontend/src/components/editor/MarkdownEditor.tsx`** (new) — the component.
- **`frontend/src/components/CodexDrawer.tsx`** — note editor swaps in; dirty indicator and Mod-S.
- **`frontend/src/components/WorldsStudio.tsx`** — lore prompt and world entity template swap in.
- **`frontend/src/components/SystemsStudio.tsx`** — rules prompt swaps in.
- **`frontend/src/components/ContentStudio.tsx`** (new) — the full-screen surface.
- **`frontend/src/App.tsx`**, **`frontend/src/components/MenuBar.tsx`** — open the Content Studio.

---

### Task 1: Add the CodeMirror dependencies

**Files:**
- Modify: `frontend/package.json`, `frontend/package-lock.json`

**Interfaces:**
- Produces: `codemirror`, `@codemirror/lang-markdown`, `@codemirror/lang-yaml`, `@codemirror/view`, `@codemirror/state`, `@codemirror/commands`, `@codemirror/language`, `@codemirror/autocomplete`, `@codemirror/lint` available to the app.

- [ ] **Step 1: Install the dependencies**

Run:

```bash
cd frontend && npm install codemirror @codemirror/lang-markdown @codemirror/lang-yaml @codemirror/view @codemirror/state @codemirror/commands @codemirror/language @codemirror/autocomplete @codemirror/lint
```

`@codemirror/autocomplete` and `@codemirror/lint` are not used until the intelligence plan; installing them now keeps that plan to code changes only.

- [ ] **Step 2: Confirm the versions landed**

Run: `cd frontend && npm ls codemirror @codemirror/lang-markdown @codemirror/lang-yaml`
Expected: each listed with a resolved version and no `UNMET DEPENDENCY`.

- [ ] **Step 3: Confirm the app still builds and bundles offline**

Run: `cd frontend && npm run build`
Expected: `tsc` clean, vite writes `../pkg/gui/dist`, and the script touches `pkg/gui/dist/.gitkeep` so `git status` stays clean.

- [ ] **Step 4: Commit**

```bash
git add frontend/package.json frontend/package-lock.json
git commit -m "build(frontend): add the CodeMirror editor dependencies"
```

---

### Task 2: The `MarkdownEditor` component

**Files:**
- Create: `frontend/src/components/editor/theme.ts`
- Create: `frontend/src/components/editor/languages.ts`
- Create: `frontend/src/components/editor/MarkdownEditor.tsx`

**Interfaces:**
- Produces: `type EditorLanguage = 'markdown' | 'markdown-frontmatter' | 'yaml'`; `function languageExtensions(language: EditorLanguage): Extension[]`; `const editorTheme: Extension`; default export `MarkdownEditor` with props `{ value, onChange, language, onSave?, readOnly?, placeholder?, minHeight?, ariaLabel }`.

- [ ] **Step 1: Write the pure language switch**

Create `frontend/src/components/editor/languages.ts`:

```ts
import type { Extension } from '@codemirror/state';
import { markdown } from '@codemirror/lang-markdown';
import { yaml, yamlFrontmatter } from '@codemirror/lang-yaml';

export type EditorLanguage = 'markdown' | 'markdown-frontmatter' | 'yaml';

// languageExtensions maps a document kind to its parser. It is a plain function
// with no CodeMirror instance, so the mapping is reviewable on its own.
export function languageExtensions(language: EditorLanguage): Extension[] {
  switch (language) {
    case 'markdown-frontmatter':
      // A note is markdown whose head is a YAML block, so the frontmatter gets
      // YAML parsing and the body keeps markdown parsing.
      return [yamlFrontmatter({ content: markdown() })];
    case 'yaml':
      return [yaml()];
    case 'markdown':
    default:
      return [markdown()];
  }
}
```

- [ ] **Step 2: Write the theme**

Create `frontend/src/components/editor/theme.ts`:

```ts
import { EditorView } from '@codemirror/view';

// editorTheme keeps the editor inside the app's glassmorphic palette rather than
// looking like a foreign widget. The values mirror the stone and purple tokens in
// frontend/src/index.css.
export const editorTheme = EditorView.theme(
  {
    '&': {
      color: '#e7e5e4',
      backgroundColor: 'transparent',
      fontSize: '0.75rem',
      height: '100%',
    },
    '.cm-content': {
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
      padding: '0.75rem',
      caretColor: '#c084fc',
    },
    '.cm-scroller': { lineHeight: '1.6', overflow: 'auto' },
    '.cm-gutters': {
      backgroundColor: 'rgba(0,0,0,0.35)',
      color: '#78716c',
      border: 'none',
    },
    '.cm-activeLine': { backgroundColor: 'rgba(168,85,247,0.06)' },
    '.cm-activeLineGutter': { backgroundColor: 'rgba(168,85,247,0.10)', color: '#c084fc' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground': {
      backgroundColor: 'rgba(147,51,234,0.35)',
    },
    '.cm-cursor': { borderLeftColor: '#c084fc' },
    '.cm-placeholder': { color: '#78716c' },
    '.cm-tooltip': {
      backgroundColor: '#1c1917',
      border: '1px solid rgba(255,255,255,0.1)',
      borderRadius: '0.5rem',
    },
  },
  { dark: true },
);
```

- [ ] **Step 3: Write the component**

Create `frontend/src/components/editor/MarkdownEditor.tsx`:

```tsx
import { useEffect, useRef } from 'react';
import { EditorState } from '@codemirror/state';
import { EditorView, keymap, placeholder as placeholderExt } from '@codemirror/view';
import { basicSetup } from 'codemirror';
import { indentWithTab } from '@codemirror/commands';
import { languageExtensions, type EditorLanguage } from './languages';
import { editorTheme } from './theme';

export interface MarkdownEditorProps {
  value: string;
  onChange: (next: string) => void;
  language: EditorLanguage;
  onSave?: () => void;
  readOnly?: boolean;
  placeholder?: string;
  minHeight?: string;
  ariaLabel: string;
}

// MarkdownEditor owns its CodeMirror document. It is uncontrolled on purpose:
// value seeds the editor and onChange reports every transaction. The parent must
// pass a key that changes with the document identity (the entity id, the file
// path), which is what keeps undo history and the cursor correct and avoids the
// cursor-jump that a controlled CodeMirror causes.
export default function MarkdownEditor({
  value,
  onChange,
  language,
  onSave,
  readOnly,
  placeholder,
  minHeight = '240px',
  ariaLabel,
}: MarkdownEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const viewRef = useRef<EditorView | null>(null);

  // The callbacks are read through refs so a parent re-render does not tear the
  // editor down and lose the cursor.
  const onChangeRef = useRef(onChange);
  const onSaveRef = useRef(onSave);
  onChangeRef.current = onChange;
  onSaveRef.current = onSave;

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const extensions = [
      basicSetup,
      languageExtensions(language),
      editorTheme,
      EditorView.lineWrapping,
      EditorState.readOnly.of(!!readOnly),
      EditorView.contentAttributes.of({ 'aria-label': ariaLabel }),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) onChangeRef.current(update.state.doc.toString());
      }),
      keymap.of([
        {
          key: 'Mod-s',
          preventDefault: true,
          run: () => {
            onSaveRef.current?.();
            return true;
          },
        },
        indentWithTab,
      ]),
    ];
    if (placeholder) extensions.push(placeholderExt(placeholder));

    const view = new EditorView({
      parent: host,
      state: EditorState.create({ doc: value, extensions }),
    });
    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // Intentionally created once: the parent remounts this component via key when
    // a different document is loaded.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div
      ref={hostRef}
      className="w-full min-h-0 overflow-hidden rounded-xl border border-stone-800 bg-stone-950 focus-within:border-purple-500/50 transition-colors"
      style={{ minHeight, height: '100%' }}
    />
  );
}
```

- [ ] **Step 4: Verify the type gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors. If `basicSetup` is not exported from `codemirror` in the installed version, import it from `codemirror` still — that is its documented export — and check `npm ls codemirror` resolves.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/editor/
git commit -m "feat(frontend): add the CodeMirror markdown editor component"
```

---

### Task 3: The codex note editor

**Files:**
- Modify: `frontend/src/components/CodexDrawer.tsx`

**Interfaces:**
- Consumes: `MarkdownEditor` (Task 2)
- Produces: the codex note body is edited in `MarkdownEditor` with `language="markdown-frontmatter"`.

- [ ] **Step 1: Replace the textarea**

In `frontend/src/components/CodexDrawer.tsx`, replace the `<textarea>` at line 424 with:

```tsx
            <MarkdownEditor
              key={entity?.id ?? 'no-note'}
              value={markdown}
              onChange={setMarkdown}
              language="markdown-frontmatter"
              onSave={() => {
                if (entity) void handleSave(entity.id);
              }}
              ariaLabel="Entity note markdown"
              minHeight="360px"
            />
```

`handleSave` is the component's existing save routine; if it is an inline arrow in the JSX today, extract it to a `const handleSave = async (id: string) => { await onSave(id, markdown); }` above the return so both the Save button and Mod-S call it.

- [ ] **Step 2: Add the import**

Add beside the other component imports in that file:

```tsx
import MarkdownEditor from './editor/MarkdownEditor';
```

- [ ] **Step 3: Verify the type gate and the build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: clean, and `pkg/gui/dist` is written.

- [ ] **Step 4: Verify by hand that the editor is live**

Run: `mise run dev:gui` in one terminal and `mise run dev:frontend` in another, open the codex, and confirm: line numbers are visible, the frontmatter block is coloured as YAML, the body is coloured as markdown, and typing marks the drawer dirty.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/CodexDrawer.tsx
git commit -m "feat(frontend): edit codex notes in the CodeMirror editor"
```

---

### Task 4: The world studio editors

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`

**Interfaces:**
- Consumes: `MarkdownEditor` (Task 2)
- Produces: the lore prompt uses `language="markdown"`; the world entity template uses `language="markdown-frontmatter"`.

- [ ] **Step 1: Replace the lore prompt textarea**

At `frontend/src/components/WorldsStudio.tsx:983`, replace the `<textarea value={lorePrompt} ...>` with:

```tsx
            <MarkdownEditor
              key={`${savedID || slugID || 'draft'}-lore`}
              value={lorePrompt}
              onChange={(next) => {
                setLorePrompt(next);
                markDirty();
              }}
              language="markdown"
              ariaLabel="World lore prompt"
              placeholder="Describe the sensory tone, factions and conflicts the storyteller should hold in mind..."
            />
```

- [ ] **Step 2: Replace the entity template textarea**

At `frontend/src/components/WorldsStudio.tsx:1062`, replace the `<textarea value={entityMarkdown} ...>` with:

```tsx
                  <MarkdownEditor
                    key={`${savedID || slugID || 'draft'}-${selectedEntityID}`}
                    value={entityMarkdown}
                    onChange={(next) => {
                      setEntityMarkdown(next);
                      markDirty();
                    }}
                    language="markdown-frontmatter"
                    ariaLabel="World entity template markdown"
                  />
```

- [ ] **Step 3: Add the import**

```tsx
import MarkdownEditor from './editor/MarkdownEditor';
```

- [ ] **Step 4: Verify the type gate and the build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx
git commit -m "feat(frontend): edit world lore and templates in the CodeMirror editor"
```

---

### Task 5: The system rules prompt

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx`

**Interfaces:**
- Consumes: `MarkdownEditor` (Task 2)
- Produces: the rules prompt uses `language="markdown"`.

- [ ] **Step 1: Replace the rules prompt textarea**

At `frontend/src/components/SystemsStudio.tsx:674`, replace the `<textarea value={rulesPrompt} ...>` with:

```tsx
            <MarkdownEditor
              key={`${savedID || slugID || 'draft'}-rules`}
              value={rulesPrompt}
              onChange={(next) => {
                setRulesPrompt(next);
                markDirty();
              }}
              language="markdown"
              ariaLabel="System rules prompt"
              placeholder="Describe the resolution philosophy, dice mechanics and character stats the engine should follow..."
            />
```

Leave the `mechanics.js` textarea at line 658 and the short description textarea at line 489 alone: JavaScript and short YAML scalars are out of scope.

- [ ] **Step 2: Add the import**

```tsx
import MarkdownEditor from './editor/MarkdownEditor';
```

- [ ] **Step 3: Verify the type gate and the build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/SystemsStudio.tsx
git commit -m "feat(frontend): edit system rules in the CodeMirror editor"
```

---

### Task 6: Dirty indicator and keyboard save in the codex

**Files:**
- Modify: `frontend/src/components/CodexDrawer.tsx`

**Interfaces:**
- Consumes: `MarkdownEditor`'s `onSave` (Task 2)
- Produces: a visible unsaved marker beside the Save button, and Mod-S saving the note.

- [ ] **Step 1: Track the dirty state**

In `CodexDrawer.tsx`, beside the existing `const [markdown, setMarkdown] = useState('');`, add:

```tsx
  const [savedMarkdown, setSavedMarkdown] = useState('');
```

Set both when a note is loaded, in the existing `useEffect` that runs on `entity`:

```tsx
  useEffect(() => {
    if (entity) {
      setMarkdown(entity.markdown);
      setSavedMarkdown(entity.markdown);
    } else {
      setMarkdown('');
      setSavedMarkdown('');
    }
  }, [entity]);

  const isDirty = markdown !== savedMarkdown;
```

After a successful save, update the baseline:

```tsx
      await onSave(entity.id, markdown);
      setSavedMarkdown(markdown);
```

- [ ] **Step 2: Show the marker**

Immediately before the Save button in the header row, add:

```tsx
              {isDirty && (
                <span className="text-[10px] font-sans uppercase tracking-wider text-amber-400">
                  Unsaved
                </span>
              )}
```

- [ ] **Step 3: Verify by hand**

Run the dev servers, open a note, type, and confirm: the marker appears, `Ctrl-S` saves, the marker clears, and the Discard flow is unchanged.

- [ ] **Step 4: Verify the type gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/CodexDrawer.tsx
git commit -m "feat(frontend): show an unsaved marker and save with the keyboard"
```

---

### Task 7: The Content Studio surface

**Files:**
- Create: `frontend/src/components/ContentStudio.tsx`
- Modify: `frontend/src/App.tsx`, `frontend/src/components/MenuBar.tsx`

**Interfaces:**
- Consumes: `MarkdownEditor` (Task 2), `EntityTree` and the folder client methods (organisation plan Tasks 9 and 8)
- Produces: `ContentStudio` with props `{ isOpen, onClose, gameID }`, opened from the menu bar.

- [ ] **Step 1: Write the surface**

Create `frontend/src/components/ContentStudio.tsx`:

```tsx
import { useEffect, useMemo, useState } from 'react';
import { X } from 'lucide-react';
import { APIClient } from '../api/client';
import type { EntityNote, EntitySummary, FolderNode } from '../types';
import EntityTree from './EntityTree';
import MarkdownEditor, { type MarkdownEditorProps } from './editor/MarkdownEditor';

type DocKind = 'entities' | 'prompts' | 'manifests';

interface ContentStudioProps {
  isOpen: boolean;
  onClose: () => void;
  gameID: string;
}

// ContentStudio is the full-screen authoring surface: the folder tree on the
// left, one document on the right. It reuses the codex's note endpoints rather
// than inventing a second write path, so a save here is the same save.
export default function ContentStudio({ isOpen, onClose, gameID }: ContentStudioProps) {
  const [kind, setKind] = useState<DocKind>('entities');
  const [folders, setFolders] = useState<FolderNode[]>([]);
  const [entities, setEntities] = useState<EntitySummary[]>([]);
  const [note, setNote] = useState<EntityNote | null>(null);
  const [draft, setDraft] = useState('');
  const [saved, setSaved] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    if (!isOpen || !gameID) return;
    const client = new APIClient(gameID);
    void client.listFolders().then(setFolders).catch(() => setFolders([]));
    void client.listEntities().then(setEntities).catch(() => setEntities([]));
  }, [isOpen, gameID]);

  useEffect(() => {
    if (!isOpen || !note) return;
    setDraft(note.markdown);
    setSaved(note.markdown);
  }, [isOpen, note]);

  const language: MarkdownEditorProps['language'] =
    kind === 'entities' ? 'markdown-frontmatter' : kind === 'manifests' ? 'yaml' : 'markdown';

  const isDirty = draft !== saved;
  const client = useMemo(() => new APIClient(gameID), [gameID]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-stone-950/98 backdrop-blur-sm">
      <header className="flex items-center justify-between border-b border-white/10 px-5 py-3">
        <div className="flex items-center gap-4">
          <h2 className="font-sans text-sm font-bold uppercase tracking-wider text-stone-200">
            Content Studio
          </h2>
          <nav className="flex gap-1">
            {(['entities', 'prompts', 'manifests'] as DocKind[]).map((tab) => (
              <button
                key={tab}
                onClick={() => setKind(tab)}
                className={`rounded-lg px-2.5 py-1 text-xs capitalize transition-colors cursor-pointer ${
                  kind === tab
                    ? 'bg-purple-600 text-white font-bold'
                    : 'text-stone-400 hover:bg-white/10 hover:text-stone-100'
                }`}
              >
                {tab}
              </button>
            ))}
          </nav>
        </div>
        <div className="flex items-center gap-3">
          {isDirty && <span className="text-[10px] uppercase tracking-wider text-amber-400">Unsaved</span>}
          <button onClick={onClose} className="cursor-pointer text-stone-400 hover:text-white">
            <X className="h-4 w-4" />
          </button>
        </div>
      </header>

      {error && (
        <div className="border-b border-red-500/40 bg-red-950/40 px-5 py-2 text-xs text-red-200">
          {error}
        </div>
      )}

      <div className="flex min-h-0 flex-1">
        <aside className="w-64 shrink-0 border-r border-white/10 p-3">
          <EntityTree
            folders={folders}
            entities={entities}
            selectedId={note?.id}
            onSelect={async (id) => {
              setError('');
              setNote(await client.getEntity(id));
            }}
            onMoveEntity={async (id, folder) => {
              const target = await client.getEntity(id);
              await client.saveEntity(id, target.markdown, folder);
              setEntities(await client.listEntities());
            }}
            onMoveFolder={async (from, to) => {
              await client.moveFolder(from, to);
              setFolders(await client.listFolders());
            }}
            onCreateFolder={async (path) => {
              await client.createFolder(path);
              setFolders(await client.listFolders());
            }}
            onDeleteFolder={async (path) => {
              await client.deleteFolder(path, false);
              setFolders(await client.listFolders());
            }}
          />
        </aside>

        <main className="min-h-0 min-w-0 flex-1 p-3">
          {note ? (
            <MarkdownEditor
              key={note.id}
              value={draft}
              onChange={setDraft}
              language={language}
              ariaLabel={`${kind} document`}
              minHeight="100%"
              onSave={async () => {
                try {
                  setError('');
                  await client.saveEntity(note.id, draft, note.folder);
                  setSaved(draft);
                } catch (err) {
                  setError(err instanceof Error ? err.message : String(err));
                }
              }}
            />
          ) : (
            <div className="flex h-full items-center justify-center text-xs font-mono text-stone-500">
              Select a note to edit.
            </div>
          )}
        </main>
      </div>
    </div>
  );
}
```

The `prompts` and `manifests` tabs select documents the codex endpoints do not serve yet. For this task they reuse the note endpoints and therefore show an entity note with the chosen language; wiring `prompts/lore.md` and `world.yaml` to their own routes is the follow-on noted in the spec's Content Studio paragraph, and is out of scope here.

- [ ] **Step 2: Open it from the menu bar**

In `frontend/src/components/MenuBar.tsx`, add `onOpenContentStudio: () => void` to `MenuBarProps`, accept it in the destructured props, and add an entry beside `Documentation` (line 175):

```tsx
        { label: 'Content Studio', icon: FolderTree, action: onOpenContentStudio },
```

Import `FolderTree` from `lucide-react` alongside the existing icons.

- [ ] **Step 3: Mount it in the app**

In `frontend/src/App.tsx`, add beside the other lazy modals at line 32:

```tsx
const ContentStudio = lazy(() => import('./components/ContentStudio'));
```

Add the state beside `const [isDocsOpen, setIsDocsOpen] = useState(false);`:

```tsx
  const [isContentStudioOpen, setIsContentStudioOpen] = useState(false);
```

Pass the handler to the menu bar at line 1188:

```tsx
      <MenuBar
        onOpenDocs={() => openDocs()}
        onOpenAbout={() => setIsAboutOpen(true)}
        onOpenContentStudio={() => setIsContentStudioOpen(true)}
      />
```

Render it beside the other modals, inside the existing `Suspense`:

```tsx
        <ContentStudio
          isOpen={isContentStudioOpen}
          onClose={() => setIsContentStudioOpen(false)}
          gameID={gameID}
        />
```

Use whatever the app's campaign-id variable is actually called where `gameID` appears; if the codex already receives it under another name, pass that.

- [ ] **Step 4: Verify the type gate and the build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: clean.

- [ ] **Step 5: Verify by hand**

Run the dev servers, open the Content Studio from the menu, select a note, edit it, press `Ctrl-S`, reopen it, and confirm the change persisted and the tree still shows the note in its folder.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/ContentStudio.tsx frontend/src/components/MenuBar.tsx frontend/src/App.tsx
git commit -m "feat(frontend): add the full-screen content studio"
```

---

### Task 8: Prove the player bundle is editor-free

**Files:**
- Create: `frontend/scripts/checkPlayerBundle.mjs`
- Modify: `frontend/package.json`

**Interfaces:**
- Produces: `npm run check:player-bundle`, which exits non-zero when CodeMirror reaches `pkg/gui/dist/player`.

- [ ] **Step 1: Write the check**

Create `frontend/scripts/checkPlayerBundle.mjs`:

```js
// The export player is built on its own and inlined into a single page, so it
// must stay small and offline. The app editor must never be reachable from it;
// this fails the build when CodeMirror appears in the player output.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';

const playerDir = fileURLToPath(new URL('../../pkg/gui/dist/player', import.meta.url));

function walk(dir) {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    return statSync(full).isDirectory() ? walk(full) : [full];
  });
}

let files;
try {
  files = walk(playerDir);
} catch (err) {
  console.error(`player bundle not found at ${playerDir}; run the frontend build first`);
  process.exit(1);
}

const offenders = files.filter((file) => /codemirror/i.test(readFileSync(file, 'utf8')));
if (offenders.length > 0) {
  console.error('the player bundle must not contain the editor:');
  for (const file of offenders) console.error(`  ${file}`);
  process.exit(1);
}

console.log('player bundle is editor-free');
```

- [ ] **Step 2: Add the script**

In `frontend/package.json`, add to `scripts`:

```json
    "check:player-bundle": "node scripts/checkPlayerBundle.mjs",
```

- [ ] **Step 3: Run it against the built output**

Run: `cd frontend && npm run build && npm run check:player-bundle`
Expected: `player bundle is editor-free`.

- [ ] **Step 4: Prove it can fail**

Temporarily add `import MarkdownEditor from './editor/MarkdownEditor';` to `frontend/src/components/story/StoryPlayer.tsx` and reference it so it is not tree-shaken, rebuild, and confirm the check exits non-zero and names the player file. Remove the temporary import and rebuild.

- [ ] **Step 5: Run the whole suite**

Run: `mise run test`
Expected: Go tests pass and `tsc --noEmit` is clean.

- [ ] **Step 6: Commit**

```bash
git add frontend/scripts/checkPlayerBundle.mjs frontend/package.json
git commit -m "test(frontend): assert the editor stays out of the player bundle"
```

---

## Self-Review

**Spec coverage.** §2.1 component and API → Task 2. §2.2 language wiring → Tasks 1, 2. §2.3 surfaces replaced → Tasks 3, 4, 5, with the JS and short-scalar textareas deliberately left alone. §2.4 Content Studio → Task 7. §2.5 save and lifecycle → Tasks 2 (`onSave`, Mod-S), 3 and 6 (dirty marker, unchanged Discard flow). §2.6 bundle constraint → Task 8. §5 testing → `tsc --noEmit` in every task, the driver and bundle assertions in Task 8.

**Placeholder scan.** No `TBD` or "add error handling". Where a plan step depends on a name that only exists inside a file the executor will open (`handleSave`, the campaign-id variable, the reload callback), the step names the existing symbol and says what to do when the name differs.

**Type consistency.** `EditorLanguage` is declared once in `languages.ts` (Task 2) and re-exported through `MarkdownEditorProps['language']`, which Task 7 uses. `MarkdownEditorProps` is exported from `MarkdownEditor.tsx` in Task 2 and imported as a type in Task 7. `EntityTree`'s props in Task 7 match the interface declared in the organisation plan's Task 9 exactly (`folders`, `entities`, `selectedId`, `onSelect`, `onMoveEntity`, `onMoveFolder`, `onCreateFolder`, `onDeleteFolder`). `APIClient` methods used in Task 7 (`listFolders`, `createFolder`, `moveFolder`, `deleteFolder`, `listEntities`, `getEntity`, `saveEntity`) are the ones the organisation plan's Task 8 declares.
