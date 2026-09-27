# Systems Studio New System Draft Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure adding a new system in Systems Studio adds an unsaved blank system draft tile into the sidebar and initializes the editor with a blank slate, with discard protection when navigating away.

**Architecture:** Add `SystemSelection` and `SystemDraft` types to `SystemsStudio.tsx` mirroring `WorldsStudio.tsx`. The sidebar renders an unsaved draft tile at the top when a draft exists. `requestSelection` checks for dirty edits and prompts `<DiscardDraftConfirm>` before switching.

**Tech Stack:** React 19, TypeScript, Tailwind CSS v4, Lucide React (`Shield`, `Plus`, `Save`, `DiscardDraftConfirm`).

---

### File Structure Map

- **Modify:** `frontend/src/components/SystemsStudio.tsx`
  - Import `DiscardDraftConfirm` from `./launcher/DiscardDraftConfirm`.
  - Introduce `SystemSelection` and `SystemDraft` state.
  - Update `handleNewSystem` to set blank fields and a draft selection.
  - Render the draft tile at the top of the sidebar list.
  - Wire `requestSelection` and `markDirty` to protect unsaved edits.
  - Render `<DiscardDraftConfirm>` dialog for dirty state transitions.

---

### Task 1: Update State, Types, and Handlers in `SystemsStudio.tsx`

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx`

- [x] **Step 1: Add types, selection/draft state, and discard handlers**

In `frontend/src/components/SystemsStudio.tsx`:
1. Import `DiscardDraftConfirm` from `./launcher/DiscardDraftConfirm`.
2. Define:
   ```typescript
   type SystemSelection = { kind: 'saved'; id: string } | { kind: 'draft' } | null;
   interface SystemDraft {
     localId: string;
     dirty: boolean;
   }
   ```
3. Replace `const [selectedID, setSelectedID] = useState<string | null>(null);` with:
   ```typescript
   const [selection, setSelection] = useState<SystemSelection>(null);
   const [draft, setDraft] = useState<SystemDraft | null>(null);
   const [pendingSelection, setPendingSelection] = useState<SystemSelection>(null);
   ```
4. Define:
   ```typescript
   const isDraft = selection?.kind === 'draft';
   const savedID = selection?.kind === 'saved' ? selection.id : null;
   const markDirty = () => setDraft((d) => (d ? { ...d, dirty: true } : d));
   ```
5. Update `handleNewSystem`:
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
6. Update `loadSystemDetail`:
   ```typescript
   const loadSystemDetail = async (id: string) => {
     try {
       const detail = await APIClient.getSystem(id);
       setSelection({ kind: 'saved', id: detail.id });
       setDraft(null);
       setName(detail.name);
       setSlugID(detail.id);
       setVersion(detail.version || '1.0.0');
       setDescription(detail.description || '');
       setRulesPrompt(detail.rules_prompt || REFERENCE_SYSTEM_TEMPLATE.rules_prompt);
       setScript(detail.script || REFERENCE_SYSTEM_TEMPLATE.script);
       setCreationPreamble(detail.character_creation?.preamble || '');
       setCreationFields(detail.character_creation?.fields || []);
     } catch (err) {
       setToast({ type: 'error', message: errorMessage(err) || 'Failed to load system details' });
     }
   };
   ```
7. Add selection request handlers:
   ```typescript
   const applySelection = (target: SystemSelection) => {
     if (target?.kind === 'saved') {
       loadSystemDetail(target.id);
     } else if (target?.kind === 'draft') {
       setSelection({ kind: 'draft' });
     }
   };

   const requestSelection = (target: SystemSelection) => {
     if (selection?.kind === 'draft' && draft?.dirty && target?.kind !== 'draft') {
       setPendingSelection(target);
       return;
     }
     applySelection(target);
   };
   ```

- [x] **Step 2: Update Sidebar and Form JSX in `SystemsStudio.tsx`**

1. In the sidebar list, render the draft tile at the top when `draft` exists:
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
2. For saved systems:
   ```tsx
   onClick={() => requestSelection({ kind: 'saved', id: s.id })}
   className={`p-3 rounded-xl border transition-all cursor-pointer text-left ${
     selection?.kind === 'saved' && selection.id === s.id
       ? 'bg-purple-950/30 border-purple-500/50 shadow-[0_0_15px_rgba(168,85,247,0.15)]'
       : 'bg-stone-900/40 border-stone-800/60 hover:bg-stone-800/40 hover:border-stone-700'
   }`}
   ```
3. Update top title in editor:
   ```tsx
   <h2 className="font-sans text-lg font-bold text-purple-400">
     {selection?.kind === 'saved' ? name || 'Edit System' : 'Create New System'}
   </h2>
   ```
4. Attach `markDirty()` to `onChange` across the editor fields (`name`, `version`, `description`, `rulesPrompt`, `script`, `creationPreamble`, `creationFields`).
5. Render `<DiscardDraftConfirm>`:
   ```tsx
   <DiscardDraftConfirm
     isOpen={Boolean(pendingSelection)}
     draftName={name.trim() || 'Untitled System'}
     onConfirm={() => {
       const target = pendingSelection;
       setPendingSelection(null);
       setDraft(null);
       applySelection(target);
     }}
     onCancel={() => setPendingSelection(null)}
   />
   ```

- [x] **Step 3: Run `mise run test:frontend` to verify compilation**

Run: `mise run test:frontend`
Expected: PASS with 0 errors.

- [x] **Step 4: Commit `SystemsStudio.tsx` changes**

```bash
git add frontend/src/components/SystemsStudio.tsx
git commit -m "feat(studio): add sidebar draft tile and blank slate for new systems"
```

---

### Task 2: Verification and Full Test Suite

**Files:**
- None (verification phase)

- [x] **Step 1: Run frontend build**

Run: `mise run build:frontend`
Expected: PASS.

- [x] **Step 2: Run full test and lint suite**

Run: `mise run test && mise run lint`
Expected: PASS with 0 errors.
