# Systems Studio New System Draft Design

- **Date:** 2026-09-25
- **Status:** Approved
- **Scope:** Systems Studio UI (`frontend/src/components/SystemsStudio.tsx`)
- **Related:** `frontend/src/components/WorldsStudio.tsx`, `frontend/src/components/launcher/DiscardDraftConfirm.tsx`

---

## 1. Overview & Goals

In `SystemsStudio`, clicking the "New" button previously set `selectedID = null` and populated the form with the reference 2d6 Narrative template. However, no visual item was added to the sidebar for the new system, leaving the previously clicked system or the list state ambiguous and confusing to the user as to whether they were creating a new system or editing an existing one.

In contrast, `WorldsStudio` introduces a dedicated `draft` concept: when adding a new world, an unsaved draft tile is pinned at the top of the sidebar with a dashed border, displaying the live title (or "Untitled World") and an `unsaved` badge, with active selection highlighting.

This specification aligns `SystemsStudio` with `WorldsStudio`, ensuring:
1. Creating a new system adds a visible, blank draft item into the sidebar.
2. The form starts as a true blank slate.
3. Switching away from a dirty draft warns the user with the existing `DiscardDraftConfirm` dialog.

### 1.1 Goals

1. **Sidebar Draft Tile**: When creating a new system, display a dashed-border tile at the top of the sidebar showing `{name.trim() || 'Untitled System'}` and an `unsaved` badge.
2. **Clear Selection Highlighting**: Active selection is unambiguous: `selection.kind === 'draft'` highlights only the draft tile; saved systems are only highlighted when `selection.kind === 'saved' && selection.id === s.id`.
3. **Blank Slate Initialization**: Clicking "New" clears all fields to empty strings (with version defaulting to `1.0.0`), allowing users to craft their rules from scratch.
4. **Reference Template On Demand**: The existing "Reset to Reference" button remains available to populate the full 2d6 Narrative boilerplate when desired.
5. **Discard Protection**: Warn with `<DiscardDraftConfirm>` if the user attempts to switch to an existing system while draft edits are unsaved.

### 1.2 Non-Goals

1. Modifying backend system schemas or endpoints: `APIClient.saveSystem`, `getSystem`, and `listSystems` remain unchanged.
2. Multi-draft persistence: only one active draft is kept in memory at a time, exactly matching `WorldsStudio`.

---

## 2. Component State & Architecture

### 2.1 Types (`frontend/src/components/SystemsStudio.tsx`)

```typescript
type SystemSelection = { kind: 'saved'; id: string } | { kind: 'draft' } | null;

interface SystemDraft {
  localId: string;
  dirty: boolean;
}
```

### 2.2 State Additions

Replace `selectedID: string | null` with:
- `selection: SystemSelection` (defaults to null, initialized on load)
- `draft: SystemDraft | null` (non-null when a new system is in progress)
- `pendingSelection: SystemSelection` (holds target selection during discard confirmation)

### 2.3 Form Initialization (`handleNewSystem`)

When "New" is clicked or 0 systems exist:
```typescript
const handleNewSystem = () => {
  setSelection({ kind: 'draft' });
  setDraft({ localId: crypto.randomUUID(), dirty: false });
  setName('');
  setSlugID('');
  setVersion('1.0.0');
  setDescription('');
  setRulesPrompt('');
  setScript('');
  setCreationPreamble('');
  setCreationFields([]);
  setActiveTab('manifest');
};
```

---

## 3. Sidebar Rendering & Discard Flow

### 3.1 Sidebar Rendering

At the top of the sidebar list in `SystemsStudio.tsx`:
```tsx
{draft && (
  <div
    onClick={() => requestSelection({ kind: 'draft' })}
    className={`p-3 rounded-xl border transition-all cursor-pointer text-left border-dashed ${
      selection?.kind === 'draft'
        ? 'bg-purple-950/30 border-purple-500/50 shadow-[0_0_15px_rgba(168,85,247,0.15)]'
        : 'bg-stone-900/40 border-stone-700 hover:bg-stone-800/40'
    }`}
  >
    <div className="flex items-center justify-between gap-2">
      <h4 className="font-sans text-xs font-bold text-stone-200 truncate">
        {name.trim() || 'Untitled System'}
      </h4>
      <span className="text-[10px] font-mono text-amber-300 bg-amber-950/40 border border-amber-500/30 px-1.5 py-0.5 rounded">
        unsaved
      </span>
    </div>
  </div>
)}
```

For saved systems:
```tsx
onClick={() => requestSelection({ kind: 'saved', id: s.id })}
className={`p-3 rounded-xl border transition-all cursor-pointer text-left ${
  selection?.kind === 'saved' && selection.id === s.id
    ? 'bg-purple-950/30 border-purple-500/50 shadow-[0_0_15px_rgba(168,85,247,0.15)]'
    : 'bg-stone-900/40 border-stone-800/60 hover:bg-stone-800/40 hover:border-stone-700'
}`}
```

### 3.2 Selection Transition & Discard Dialog

```typescript
const markDirty = () => setDraft((d) => (d ? { ...d, dirty: true } : d));

const requestSelection = (target: SystemSelection) => {
  if (selection?.kind === 'draft' && draft?.dirty && target?.kind !== 'draft') {
    setPendingSelection(target);
    return;
  }
  applySelection(target);
};

const applySelection = (target: SystemSelection) => {
  if (target?.kind === 'saved') {
    loadSystemDetail(target.id);
  } else if (target?.kind === 'draft') {
    setSelection({ kind: 'draft' });
  }
};
```

When `pendingSelection` is non-null, render `<DiscardDraftConfirm>`:
- On confirm: discard draft (`setDraft(null)`), execute `applySelection(pendingSelection)`, and clear `pendingSelection`.
- On cancel: clear `pendingSelection`.

---

## 4. Verification

1. `mise run test:frontend` (`npx tsc --noEmit`) passes with 0 errors.
2. Clicking "New" displays an "Untitled System" draft card at the top of the sidebar with an `unsaved` badge and empty fields.
3. Typing into the name updates the draft card title in real time.
4. Clicking a saved system while the draft is clean switches without dialog; clicking while dirty shows the discard confirmation dialog.
5. Saving a system clears the draft and marks the newly saved system as selected.
