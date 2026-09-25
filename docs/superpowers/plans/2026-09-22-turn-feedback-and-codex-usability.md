# Turn Feedback, Note Corrections, and Codex Usability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide immediate inline feedback when turns are processing, enable non-disruptive note creation from continuity findings without advancing turns or cutting audio, and make the Codex drawer spacious and free of horizontal scrolling with a collapsible sidebar and maximize toggle.

**Architecture:**
- Frontend-driven state management in React 19 + Tailwind v4:
  - `Drawers.tsx` manages size classes and maximize toggle state.
  - `CodexDrawer.tsx` manages collapsible entity sidebar toggle state.
  - `App.tsx` and `ChronicleView.tsx` coordinate `pendingAction` state to immediately render pending turn beats and animated drafting states before streaming arrives.
  - `App.tsx` intercepts continuity note correction clicks, parses the entity name, and presents a lightweight note creation modal that invokes `client.saveEntity` and `client.addressFinding` quietly without triggering `/gm` turns.

**Tech Stack:** React 19, TypeScript, Tailwind CSS v4, Lucide React icons.

---

### Task 1: Drawer Sizing and Full-Width Maximize Toggle

**Files:**
- Modify: `frontend/src/components/Drawers.tsx`
- Test: Frontend build & typecheck (`cd frontend && npx tsc --noEmit`)

**Details:**
1. Update `sizeClasses` in `Drawers.tsx`:
   - `md: 'max-w-md'` (448px)
   - `lg: 'max-w-lg'` (512px)
   - `xl: 'max-w-3xl'` (768px - widened from 576px)
   - `full: 'max-w-full'`
2. Add `isMaximized` state inside `Drawers.tsx` toggled by a button in the drawer header with `Maximize2` and `Minimize2` icons from `lucide-react`.
3. When maximized, the drawer container uses `w-full max-w-full` instead of `sizeClasses[size]`.

- [x] **Step 1: Update Drawers.tsx**

In `frontend/src/components/Drawers.tsx`:
```tsx
import React, { useState } from 'react';
import { X, Maximize2, Minimize2 } from 'lucide-react';

interface DrawersProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  size?: 'md' | 'lg' | 'xl' | 'full';
}

const sizeClasses: Record<'md' | 'lg' | 'xl' | 'full', string> = {
  md: 'max-w-md',
  lg: 'max-w-lg',
  xl: 'max-w-3xl',
  full: 'max-w-full',
};

export const Drawers: React.FC<DrawersProps> = ({ isOpen, onClose, title, children, size = 'md' }) => {
  const [isMaximized, setIsMaximized] = useState(false);

  if (!isOpen) return null;

  const currentSizeClass = isMaximized ? 'w-full max-w-full' : `w-full ${sizeClasses[size]}`;

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/60 backdrop-blur-xs transition-opacity duration-300">
      <div className={`${currentSizeClass} bg-glass-drawer h-full p-6 shadow-2xl flex flex-col transform transition-all duration-300`}>
        <div className="flex items-center justify-between pb-4 border-b border-white/10 mb-4 shrink-0">
          <span className="font-cinzel text-amber-400 font-bold tracking-wider text-base">{title}</span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setIsMaximized((prev) => !prev)}
              className="p-1 hover:text-amber-300 cursor-pointer text-stone-400 transition-colors"
              title={isMaximized ? 'Restore drawer width' : 'Maximize drawer width'}
            >
              {isMaximized ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
            </button>
            <button
              onClick={onClose}
              className="p-1 hover:text-amber-300 cursor-pointer text-stone-400 transition-colors"
              title="Close drawer"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
        </div>
        <div className="flex-1 overflow-y-auto pr-1 min-h-0">
          {children}
        </div>
      </div>
    </div>
  );
};
```

- [x] **Step 2: Typecheck the changes**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS with 0 errors.

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/Drawers.tsx
git commit -m "feat(gui): widen drawer default to 3xl and add maximize toggle"
```

---

### Task 2: Collapsible Entity Sidebar in Codex Drawer

**Files:**
- Modify: `frontend/src/components/CodexDrawer.tsx`
- Test: Frontend build & typecheck (`cd frontend && npx tsc --noEmit`)

**Details:**
1. In `CodexDrawer.tsx`, add `isSidebarOpen: boolean` state. Initialize to `false` if `entity` is already provided, or `true` if no entity is selected.
2. Add a toggle button in the header (`Browse Notes ({count})` with `PanelLeft` / `BookOpen` icon) so the user can show/hide the sidebar whenever they want.
3. When the sidebar is collapsed, the note editor takes 100% of the drawer space, completely eliminating horizontal scrolling.
4. When selecting a note from the sidebar on narrower screens or normal width, keep or auto-collapse the sidebar.
5. Prevent horizontal overflow on textareas, inputs, and selects.

- [x] **Step 1: Update CodexDrawer.tsx**

In `frontend/src/components/CodexDrawer.tsx`:
```tsx
import React, { useMemo, useState, useEffect } from 'react';
import { EntityNote, EntitySummary } from '../types';
import { Save, Volume2, Search, BookOpen, PanelLeftClose, PanelLeft } from 'lucide-react';
import { DEFAULT_VOICE_PROFILES } from '../templates/providerPresets';
import { TurnHistoryList } from './TurnHistoryList';

interface CodexDrawerProps {
  entity?: EntityNote;
  entities?: EntitySummary[];
  onSelect: (entityId: string) => void;
  onSave: (entityId: string, markdown: string) => void;
  onMerge?: (sourceID: string, intoID: string) => void;
}

export const CodexDrawer: React.FC<CodexDrawerProps> = ({ entity, entities, onSelect, onSave, onMerge }) => {
  const [markdown, setMarkdown] = useState('');
  const [query, setQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState('all');
  const [isSidebarOpen, setIsSidebarOpen] = useState(!entity);

  useEffect(() => {
    if (entity) {
      setMarkdown(entity.markdown);
    } else {
      setIsSidebarOpen(true);
    }
  }, [entity]);

  // The corpus grows every turn, so the browser derives its own facets rather
  // than assuming a fixed set of note types.
  const types = useMemo(() => {
    const seen = new Map<string, number>();
    for (const candidate of entities ?? []) {
      if (candidate.type) seen.set(candidate.type, (seen.get(candidate.type) ?? 0) + 1);
    }
    return Array.from(seen.entries()).sort((a, b) => a[0].localeCompare(b[0]));
  }, [entities]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return (entities ?? []).filter((candidate) => {
      const matchesType = typeFilter === 'all' || candidate.type === typeFilter;
      if (!matchesType) return false;
      if (!needle) return true;
      return (
        candidate.name.toLowerCase().includes(needle) ||
        candidate.id.toLowerCase().includes(needle) ||
        (candidate.tags ?? []).some((tag) => tag.toLowerCase().includes(needle))
      );
    });
  }, [entities, query, typeFilter]);

  const applyVoiceArchetype = (profileId: string) => {
    const profile = DEFAULT_VOICE_PROFILES.find((p) => p.id === profileId);
    if (!profile) return;

    const voiceSnippet = `voice:\n  voice_id: "${profile.voice_id}"\n  pitch: ${profile.pitch}\n  speech_rate: ${profile.speech_rate}`;
    if (markdown.startsWith('---\n')) {
      const secondDashes = markdown.indexOf('\n---\n', 4);
      if (secondDashes !== -1) {
        const fm = markdown.slice(4, secondDashes);
        const rest = markdown.slice(secondDashes + 5);
        let newFm = fm;
        const voiceRegex = /voice:\s*\n(\s+.*\n)*/;
        if (voiceRegex.test(newFm)) {
          newFm = newFm.replace(voiceRegex, voiceSnippet + '\n');
        } else {
          newFm = newFm.trimEnd() + '\n' + voiceSnippet + '\n';
        }
        setMarkdown(`---\n${newFm}---\n${rest}`);
        return;
      }
    }

    setMarkdown(`---\n${voiceSnippet}\n---\n\n${markdown}`);
  };

  return (
    <div className="flex flex-col md:flex-row gap-4 h-full min-h-0 overflow-x-hidden">
      {/* Corpus browser: collapsible sidebar */}
      {isSidebarOpen && (
        <aside className="w-full md:w-72 shrink-0 flex flex-col gap-2 min-h-0 border-b md:border-b-0 md:border-r border-white/10 pb-4 md:pb-0 md:pr-4">
          <div className="flex items-center justify-between pb-1">
            <span className="text-xs font-cinzel text-stone-400 font-bold uppercase tracking-wider">
              Notes ({entities?.length ?? 0})
            </span>
            {entity && (
              <button
                onClick={() => setIsSidebarOpen(false)}
                className="text-stone-400 hover:text-stone-200 p-1 rounded hover:bg-stone-800 cursor-pointer"
                title="Hide notes list"
              >
                <PanelLeftClose className="w-4 h-4" />
              </button>
            )}
          </div>

          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-stone-500" />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search the codex..."
              className="w-full bg-black/50 border border-white/10 rounded-lg pl-8 pr-2 py-1.5 text-xs text-stone-200 placeholder-stone-500 focus:outline-none focus:border-amber-500/60"
            />
          </div>

          <div className="flex flex-wrap gap-1">
            <button
              onClick={() => setTypeFilter('all')}
              className={`text-[10px] font-mono px-2 py-0.5 rounded border transition-colors cursor-pointer ${
                typeFilter === 'all'
                  ? 'bg-amber-600/30 border-amber-500/50 text-amber-200'
                  : 'bg-black/40 border-white/10 text-stone-400 hover:text-stone-200'
              }`}
            >
              all ({entities?.length ?? 0})
            </button>
            {types.map(([type, count]) => (
              <button
                key={type}
                onClick={() => setTypeFilter(type)}
                className={`text-[10px] font-mono px-2 py-0.5 rounded border transition-colors cursor-pointer ${
                  typeFilter === type
                    ? 'bg-amber-600/30 border-amber-500/50 text-amber-200'
                    : 'bg-black/40 border-white/10 text-stone-400 hover:text-stone-200'
                }`}
              >
                {type} ({count})
              </button>
            ))}
          </div>

          <div className="flex-1 min-h-[160px] overflow-y-auto space-y-1 pr-1">
            {filtered.length === 0 ? (
              <p className="text-stone-500 text-xs italic p-2">
                {(entities?.length ?? 0) === 0 ? 'No notes yet. Play a turn and the world will grow.' : 'No notes match.'}
              </p>
            ) : (
              filtered.map((candidate) => (
                <button
                  key={candidate.id}
                  onClick={() => {
                    onSelect(candidate.id);
                  }}
                  className={`w-full text-left px-2.5 py-2 rounded-lg border transition-colors cursor-pointer ${
                    entity?.id === candidate.id
                      ? 'bg-amber-600/20 border-amber-500/40'
                      : 'bg-black/30 border-white/5 hover:border-amber-500/30'
                  }`}
                >
                  <div className="text-xs text-stone-200 truncate font-medium">{candidate.name}</div>
                  <div className="text-[10px] font-mono text-stone-500 truncate">
                    {candidate.type || 'note'}
                    {candidate.location ? ` · ${candidate.location.replace(/\[\[|\]\]/g, '')}` : ''}
                  </div>
                </button>
              ))
            )}
          </div>
        </aside>
      )}

      {/* Note editor */}
      <div className="flex-1 min-w-0 min-h-0 flex flex-col">
        {!entity ? (
          <div className="h-full flex flex-col items-center justify-center text-center gap-2 text-stone-500 italic p-6">
            <BookOpen className="w-8 h-8 text-amber-500/40" />
            <span className="text-sm">Choose a note from the codex to read or edit it.</span>
          </div>
        ) : (
          <div className="space-y-4 flex flex-col h-full min-h-0">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-white/10 pb-3">
              <div className="flex items-center gap-3 min-w-0">
                {!isSidebarOpen && (
                  <button
                    onClick={() => setIsSidebarOpen(true)}
                    className="flex items-center gap-1.5 text-xs font-cinzel px-2.5 py-1.5 rounded-lg bg-stone-900/80 border border-white/10 text-stone-300 hover:text-amber-300 hover:border-amber-500/40 cursor-pointer transition-colors shrink-0"
                    title="Browse other notes"
                  >
                    <PanelLeft className="w-3.5 h-3.5 text-amber-400" />
                    <span>Browse Notes</span>
                  </button>
                )}
                <div className="min-w-0">
                  <h2 className="text-xl font-cinzel text-amber-400 font-bold truncate">{entity.name}</h2>
                  <span className="text-xs font-mono uppercase text-stone-400">{entity.type}</span>
                </div>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                {onMerge && (entities?.length ?? 0) > 1 && (
                  <select
                    onChange={(e) => {
                      if (e.target.value && entity) {
                        onMerge(entity.id, e.target.value);
                        e.target.value = '';
                      }
                    }}
                    className="bg-stone-900 border border-stone-700 rounded-lg pl-2.5 pr-8 py-1.5 text-xs font-mono text-stone-300 focus:outline-none cursor-pointer"
                    defaultValue=""
                  >
                    <option value="" disabled>Merge into...</option>
                    {(entities ?? [])
                      .filter((candidate) => candidate.id !== entity.id)
                      .map((candidate) => (
                        <option key={candidate.id} value={candidate.id}>
                          {candidate.name}
                        </option>
                      ))}
                  </select>
                )}
                <button
                  onClick={() => onSave(entity.id, markdown)}
                  className="flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-amber-600 hover:bg-amber-500 text-stone-950 font-cinzel font-bold text-xs shadow-md transition-all cursor-pointer"
                >
                  <Save className="w-3.5 h-3.5" />
                  <span>Save</span>
                </button>
              </div>
            </div>

            <div className="flex flex-wrap items-center justify-between gap-2 px-1">
              <label className="flex items-center gap-1.5 text-xs text-stone-400 font-cinzel shrink-0">
                <Volume2 className="w-3.5 h-3.5 text-amber-400" />
                <span>Apply Voice Archetype:</span>
              </label>
              <select
                onChange={(e) => {
                  if (e.target.value) {
                    applyVoiceArchetype(e.target.value);
                    e.target.value = '';
                  }
                }}
                className="bg-stone-900 border border-amber-500/30 rounded-lg pl-2.5 pr-8 py-1 text-xs font-mono text-amber-300 focus:outline-none cursor-pointer"
                defaultValue=""
              >
                <option value="" disabled>Select Archetype...</option>
                {DEFAULT_VOICE_PROFILES.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} ({p.voice_id})
                  </option>
                ))}
              </select>
            </div>

            <textarea
              value={markdown}
              onChange={(e) => setMarkdown(e.target.value)}
              className="w-full flex-1 min-h-[360px] bg-black/50 border border-white/10 rounded-xl p-3.5 font-mono text-xs text-stone-200 focus:outline-none focus:border-amber-500/80 shadow-inner resize-none leading-relaxed"
            />

            <TurnHistoryList turns={entity.history} />

            {entity.backlinks && entity.backlinks.length > 0 && (
              <div className="pt-3 border-t border-white/10">
                <span className="text-xs font-cinzel text-stone-400 block mb-2">Referenced By (Backlinks):</span>
                <div className="flex flex-wrap gap-1.5">
                  {entity.backlinks.map((link) => (
                    <button
                      key={link}
                      onClick={() => onSelect(link)}
                      className="text-xs bg-black/40 border border-white/10 px-2 py-0.5 rounded-md text-amber-400 font-mono hover:border-amber-500/40 cursor-pointer transition-colors"
                    >
                      [[{link}]]
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};
```

- [x] **Step 2: Typecheck the changes**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS with 0 errors.

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/CodexDrawer.tsx
git commit -m "feat(frontend): collapse entity sidebar by default in codex drawer"
```

---

### Task 3: Inline Turn-in-Progress & Drafting Activity Indicator

**Files:**
- Modify: `frontend/src/components/ChronicleView.tsx`
- Modify: `frontend/src/App.tsx`
- Test: Frontend build & typecheck (`cd frontend && npx tsc --noEmit`)

**Details:**
1. In `ChronicleView.tsx`:
   - Accept optional props:
     - `turnInFlight?: boolean;`
     - `pendingAction?: { mode: string; text: string } | null;`
     - `streamedProse?: string;`
   - If `turnInFlight && pendingAction`:
     - Render a pending turn beat at the bottom:
       - Action badge with `pendingAction.mode` and `pendingAction.text`.
       - If `streamedProse` is empty:
         - An animated card with a pulsing icon (feather / spinner) and text: *"The narrator is drafting the scene..."*
       - If `streamedProse` is not empty:
         - Render `TurnSegments` with `streamedProse`.
   - Maintain an internal ref on the bottom element (`<div ref={bottomRef} />`) with `useEffect` scrolling it into view whenever `turnInFlight`, `pendingAction`, or `streamedProse` updates.
2. In `App.tsx`:
   - Define state: `const [pendingAction, setPendingAction] = useState<{ mode: string; text: string } | null>(null);`
   - In `handleActionSubmit`: set `setPendingAction({ mode, text })` at start, and clear `setPendingAction(null)` when the turn finishes or stops.
   - Pass `turnInFlight`, `pendingAction`, and `streamedProse` directly into `<ChronicleView ... />`.
   - Remove the separate `streamedProse` card outside `ChronicleView` in `App.tsx` since it now renders inline as part of the pending turn.

- [x] **Step 1: Update ChronicleView.tsx**

In `frontend/src/components/ChronicleView.tsx`:
Add `Loader2`, `Feather`, or `Sparkles` icon from `lucide-react`, accept `turnInFlight`, `pendingAction`, and `streamedProse`, render pending card, and add smooth auto-scroll.

```tsx
import React, { useEffect, useRef } from 'react';
import { Turn } from '../types';
import { TurnSegments } from './TurnSegments';
import { Loader2, Sparkles } from 'lucide-react';

interface PendingAction {
  mode: string;
  text: string;
}

interface ChronicleViewProps {
  turns: Turn[];
  onWikilinkClick: (entityId: string) => void;
  autoPlay?: boolean;
  volume?: number;
  serverPlayback?: boolean;
  onPlayTurnAudio?: (turnNumber: number, segmentIndex?: number) => void;
  onStopAudio?: () => void;
  onCorrect?: (note: string, turnNumber: number) => void;
  addressedTurns?: Set<number>;
  onAddress?: (turnNumber: number) => void;
  turnInFlight?: boolean;
  pendingAction?: PendingAction | null;
  streamedProse?: string;
}

export const ChronicleView: React.FC<ChronicleViewProps> = ({
  turns,
  onWikilinkClick,
  autoPlay = false,
  volume = 1,
  serverPlayback = false,
  onPlayTurnAudio,
  onStopAudio,
  onCorrect,
  addressedTurns,
  onAddress,
  turnInFlight,
  pendingAction,
  streamedProse,
}) => {
  const bottomRef = useRef<HTMLDivElement | null>(null);

  // Auto-scroll when a new turn is added, turn starts, or prose streams in
  useEffect(() => {
    if (turnInFlight || streamedProse) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [turnInFlight, streamedProse, turns.length]);

  // Art is per scene, not per turn: it is shown when the party arrives somewhere
  // new and reused while they stay.
  let previousLocationID: string | undefined;
  const beats = turns.map((turn) => {
    const isSceneChange = !!turn.location_id && turn.location_id !== previousLocationID;
    previousLocationID = turn.location_id ?? previousLocationID;
    return { turn, isSceneChange };
  });

  return (
    <div className="flex-1 overflow-y-auto px-8 py-6 space-y-6">
      {turns.length === 0 && !pendingAction ? (
        <div className="h-full flex items-center justify-center text-stone-500 font-cinzel tracking-wider text-sm italic">
          The chronicle awaits your first action...
        </div>
      ) : (
        beats.map(({ turn, isSceneChange }, index) => (
          <div key={turn.turn_number} className="space-y-4 pb-6 border-b border-white/5 last:border-0">
            {/* Player Input Block */}
            {turn.input_text && (
              <div className="flex items-start gap-3 text-stone-300 text-sm font-sans italic bg-black/40 p-3.5 rounded-xl border border-white/5 shadow-inner">
                <span className="text-amber-400 font-semibold uppercase tracking-wider text-xs font-cinzel">
                  [{turn.mode || 'Action'}]
                </span>
                <span>{turn.input_text}</span>
                {turn.outcome && (
                  <span className="ml-auto text-xs font-mono text-stone-400">{turn.outcome}</span>
                )}
              </div>
            )}

            {/* Scene Illustration if available */}
            {turn.image_url && (
              <div className="my-4 rounded-xl overflow-hidden border border-white/10 shadow-2xl">
                <img src={turn.image_url} alt="Scene illustration" className="w-full object-cover max-h-96" />
              </div>
            )}

            {/* Scene art, when the party has moved somewhere new */}
            {turn.location_art_url && isSceneChange && (
              <div className="my-4 rounded-xl overflow-hidden border border-white/10 shadow-2xl">
                <img
                  src={turn.location_art_url}
                  alt={turn.location_name || 'Scene'}
                  className="w-full object-cover max-h-96"
                />
                {turn.location_name && (
                  <div className="px-3 py-2 text-xs font-cinzel tracking-widest text-stone-400 uppercase">
                    {turn.location_name}
                  </div>
                )}
              </div>
            )}

            {/* Narrated prose and attributed speech, in playback order */}
            <TurnSegments
              segments={turn.segments}
              fallback={turn.prose}
              onEntityClick={onWikilinkClick}
              // Only the newest turn narrates itself: autoplaying every turn would
              // start them all at once on load.
              autoPlay={autoPlay && index === beats.length - 1}
              volume={volume}
              serverPlayback={serverPlayback}
              onPlayTurn={onPlayTurnAudio ? (segmentIndex) => onPlayTurnAudio(turn.turn_number, segmentIndex) : undefined}
              onStopTurn={onStopAudio}
            />

            {turn.truncated && (
              <div className="text-xs font-mono text-amber-400/80 pt-1">
                The narrator was cut off by the model's token limit. Raise the response limit for the gm role in Settings.
              </div>
            )}

            {turn.context_notes && turn.context_notes.length > 0 && (
              <div className="text-xs font-mono text-stone-500 pt-1">
                Context trimmed to fit the prompt budget: {turn.context_notes.join(', ')}. Raise the context budget in
                Settings to keep more.
              </div>
            )}

            {turn.continuity_notes && turn.continuity_notes.length > 0 && (
              <div
                className={`text-xs font-mono pt-1 space-y-1 ${
                  addressedTurns?.has(turn.turn_number) ? 'text-stone-500 opacity-60' : 'text-amber-400/90'
                }`}
              >
                {turn.continuity_notes.map((note, noteIdx) => (
                  <div key={noteIdx} className="flex items-start gap-2">
                    <span>{note}</span>
                    {addressedTurns?.has(turn.turn_number) ? (
                      <span className="shrink-0 px-1.5 py-0.5 rounded border border-stone-700 text-stone-500 text-[10px]">
                        Addressed
                      </span>
                    ) : (
                      <div className="flex items-center gap-1.5 shrink-0">
                        <button
                          onClick={() => onCorrect?.(note, turn.turn_number)}
                          className="px-1.5 py-0.5 rounded border border-amber-500/40 hover:bg-amber-600/20 cursor-pointer transition-colors"
                          title="Review or correct this finding"
                        >
                          Correct
                        </button>
                        {onAddress && (
                          <button
                            onClick={() => onAddress(turn.turn_number)}
                            className="px-1.5 py-0.5 rounded border border-stone-600 hover:bg-stone-800 text-stone-400 cursor-pointer transition-colors text-[10px]"
                            title="Mark this finding as addressed"
                          >
                            Dismiss
                          </button>
                        )}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}

            {/* Entities involved in this turn */}
            {turn.entities_hit && turn.entities_hit.length > 0 && (
              <div className="flex flex-wrap items-center gap-2 pt-1">
                {turn.entities_hit.map((entityId) => (
                  <button
                    key={entityId}
                    onClick={() => onWikilinkClick(entityId)}
                    className="px-2 py-0.5 text-xs font-cinzel tracking-wider rounded-full bg-white/5 border border-white/10 text-stone-300 hover:text-amber-300 hover:border-amber-500/60 cursor-pointer transition-colors"
                  >
                    {entityId}
                  </button>
                ))}
              </div>
            )}
          </div>
        ))
      )}

      {/* Pending Turn in Flight */}
      {turnInFlight && pendingAction && (
        <div className="space-y-4 pb-6 animate-fade-in">
          {/* Immediate Action Bubble */}
          {pendingAction.text && (
            <div className="flex items-start gap-3 text-stone-300 text-sm font-sans italic bg-black/40 p-3.5 rounded-xl border border-amber-500/20 shadow-inner">
              <span className="text-amber-400 font-semibold uppercase tracking-wider text-xs font-cinzel">
                [{pendingAction.mode || 'Action'}]
              </span>
              <span>{pendingAction.text}</span>
            </div>
          )}

          {/* Drafting feedback card or streaming prose */}
          {streamedProse ? (
            <TurnSegments
              segments={[{ kind: 'narration', text: streamedProse }]}
              fallback={streamedProse}
              onEntityClick={onWikilinkClick}
            />
          ) : (
            <div className="flex items-center gap-3 p-4 rounded-xl bg-amber-950/20 border border-amber-500/30 text-stone-300 text-sm animate-pulse">
              <div className="p-2 rounded-lg bg-amber-600/20 text-amber-400">
                <Sparkles className="w-4 h-4 animate-spin" />
              </div>
              <div className="space-y-0.5">
                <div className="font-cinzel text-xs font-bold text-amber-400 uppercase tracking-wider">
                  The narrator is drafting the scene...
                </div>
                <div className="text-xs text-stone-400 font-sans">
                  Weaving your action into the chronicle.
                </div>
              </div>
            </div>
          )}
        </div>
      )}

      <div ref={bottomRef} />
    </div>
  );
};
```

- [x] **Step 2: Update App.tsx with pendingAction**

In `frontend/src/App.tsx`:
Add `pendingAction` state, pass it to `ChronicleView`, and remove the duplicate `streamedProse` container.

```tsx
  const [pendingAction, setPendingAction] = useState<{ mode: string; text: string } | null>(null);
```
In `handleActionSubmit`:
```tsx
    setTurnInFlight(true);
    setStreamedProse('');
    setPendingAction({ mode, text });
```
In `finally`:
```tsx
    } finally {
      abortRef.current = null;
      setTurnInFlight(false);
      setPendingAction(null);
    }
```
In `handleStopTurn`:
```tsx
  const handleStopTurn = () => {
    abortRef.current?.abort();
    setStreamedProse('');
    setPendingAction(null);
  };
```
Pass props to `ChronicleView`:
```tsx
                  <ChronicleView
                    turns={chronicle}
                    onWikilinkClick={handleOpenWikilink}
                    autoPlay={serverAudio ? false : config?.media.tts.auto_play ?? false}
                    volume={config?.media.tts.master_volume ?? 1}
                    serverPlayback={serverAudio}
                    onPlayTurnAudio={handlePlayTurnAudio}
                    onStopAudio={handleStopAudio}
                    onCorrect={handleCorrect}
                    addressedTurns={addressed}
                    onAddress={handleAddress}
                    turnInFlight={turnInFlight}
                    pendingAction={pendingAction}
                    streamedProse={streamedProse}
                  />
```

- [x] **Step 3: Typecheck the changes**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS with 0 errors.

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/ChronicleView.tsx frontend/src/App.tsx
git commit -m "feat(frontend): show immediate inline turn action and drafting indicator"
```

---

### Task 4: Note Creation Modal for Missing Entity Findings

**Files:**
- Create: `frontend/src/components/AddEntityModal.tsx`
- Modify: `frontend/src/App.tsx`
- Test: Frontend build & typecheck (`cd frontend && npx tsc --noEmit`)

**Details:**
1. Create `AddEntityModal.tsx`:
   - Takes `{ isOpen, entityName, onQuickCreate, onEditInCodex, onClose }`.
   - Renders a clean dialog with:
     - Title: "Register Entity Note"
     - Description: `"The chronicle mentioned ${entityName}, but no canon note exists yet."`
     - Type selection (default `character`, option for `location`, `faction`, `item`, `concept`).
     - Buttons: "Quick Register", "Open in Codex", "Cancel".
2. In `App.tsx`:
   - Helper function `parseMissingEntityName(note: string): string | null`:
     - Matches `/"([^"]+)" speaks but has no note/`, `/"([^"]+)" is named but has no note/`, `/"([^"]+)" speaks but is not a known character/`.
   - In `handleCorrect(note: string, turnNumber?: number)`:
     - If `parseMissingEntityName(note)` finds a name:
       - Open `AddEntityModal` with `pendingEntityName` and `pendingTurnNumber`.
     - Else:
       - Pre-fill `ActionConsole` input with `/gm ${note}` without automatically executing it.
   - Quick Register:
     - Generates slug ID: `name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '')`.
     - Calls `client.saveEntity(id, markdown)`.
     - Calls `client.addressFinding(turnNumber, 'continuity')`.
     - Calls `refreshCorpus()`.
     - Shows toast or confirmation.
     - Does NOT execute a turn, and does NOT interrupt playing audio.
   - Open in Codex:
     - Saves initial note (or opens Codex with draft).
     - Opens Codex drawer (`setActiveDrawer('codex')`).
     - Marks finding addressed.

- [x] **Step 1: Create AddEntityModal.tsx**

Write `frontend/src/components/AddEntityModal.tsx`:
```tsx
import React, { useState } from 'react';
import { UserPlus, BookOpen, Check, X } from 'lucide-react';

interface AddEntityModalProps {
  isOpen: boolean;
  entityName: string;
  onQuickCreate: (name: string, type: string) => void;
  onEditInCodex: (name: string, type: string) => void;
  onClose: () => void;
}

export const AddEntityModal: React.FC<AddEntityModalProps> = ({
  isOpen,
  entityName,
  onQuickCreate,
  onEditInCodex,
  onClose,
}) => {
  const [type, setType] = useState('character');

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-fade-in">
      <div className="relative w-full max-w-md bg-stone-900 border border-amber-500/40 rounded-2xl p-6 shadow-2xl space-y-5">
        <div className="flex items-center justify-between border-b border-stone-800 pb-3">
          <div className="flex items-center gap-2">
            <UserPlus className="w-5 h-5 text-amber-400" />
            <h3 className="font-cinzel text-base font-bold text-amber-400">Add Canon Note</h3>
          </div>
          <button
            onClick={onClose}
            className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <p className="text-xs text-stone-300 leading-relaxed">
          The narrator mentioned <span className="font-bold text-amber-300">"{entityName}"</span>, but no note exists in this campaign's canon yet. Adding a note registers them in the world without disrupting the current scene.
        </p>

        <div className="space-y-1.5">
          <label className="text-xs font-cinzel text-stone-400 uppercase tracking-wider block">
            Entity Type
          </label>
          <div className="grid grid-cols-3 gap-1.5">
            {['character', 'location', 'faction', 'item', 'arc', 'concept'].map((t) => (
              <button
                key={t}
                type="button"
                onClick={() => setType(t)}
                className={`text-xs font-mono py-1.5 px-2 rounded-lg border text-center transition-colors cursor-pointer ${
                  type === t
                    ? 'bg-amber-600/30 border-amber-500 text-amber-200 font-bold'
                    : 'bg-black/40 border-stone-800 text-stone-400 hover:text-stone-200'
                }`}
              >
                {t}
              </button>
            ))}
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 rounded-xl border border-stone-700 text-stone-400 hover:bg-stone-800 text-xs font-cinzel cursor-pointer transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => onEditInCodex(entityName, type)}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-amber-500/50 bg-stone-900 hover:bg-stone-800 text-amber-300 text-xs font-cinzel cursor-pointer transition-colors"
          >
            <BookOpen className="w-3.5 h-3.5" />
            <span>Edit in Codex</span>
          </button>
          <button
            type="button"
            onClick={() => onQuickCreate(entityName, type)}
            className="flex items-center gap-1.5 px-4 py-1.5 rounded-xl bg-amber-600 hover:bg-amber-500 text-stone-950 font-cinzel font-bold text-xs shadow-lg cursor-pointer transition-colors"
          >
            <Check className="w-3.5 h-3.5" />
            <span>Quick Register</span>
          </button>
        </div>
      </div>
    </div>
  );
};
```

- [x] **Step 2: Wire AddEntityModal in App.tsx**

In `frontend/src/App.tsx`:
- Import `AddEntityModal`.
- Add states:
  ```tsx
  const [modalEntity, setModalEntity] = useState<{ name: string; turnNumber: number } | null>(null);
  ```
- Implement `parseMissingEntityName`:
  ```tsx
  const parseMissingEntityName = (note: string): string | null => {
    const match = note.match(/"([^"]+)" (?:speaks but has no note|is named but has no note|speaks but is not a known character)/i);
    return match ? match[1] : null;
  };
  ```
- Update `handleCorrect`:
  ```tsx
  const handleCorrect = (note: string, turnNumber?: number) => {
    const entityName = parseMissingEntityName(note);
    if (entityName && turnNumber !== undefined) {
      setModalEntity({ name: entityName, turnNumber });
    } else {
      // For general directives, prefill the action console
      const input = document.getElementById('action-console-input') as HTMLInputElement | null;
      if (input) {
        input.value = `/gm ${note}`;
        input.focus();
      }
    }
  };
  ```
- Handlers for modal:
  ```tsx
  const handleQuickCreateEntity = async (name: string, type: string) => {
    if (!client || !modalEntity) return;
    const slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
    const template = `---\nid: ${slug}\nname: ${name}\ntype: ${type}\n---\n\n`;
    try {
      await client.saveEntity(slug, template);
      await client.addressFinding(modalEntity.turnNumber, 'continuity');
      setAddressed((prev) => new Set(prev).add(modalEntity.turnNumber));
      refreshCorpus();
    } catch (err) {
      console.error('quick create entity failed:', err);
    } finally {
      setModalEntity(null);
    }
  };

  const handleEditInCodexEntity = async (name: string, type: string) => {
    if (!client || !modalEntity) return;
    const slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
    const template = `---\nid: ${slug}\nname: ${name}\ntype: ${type}\n---\n\n`;
    try {
      await client.saveEntity(slug, template);
      await client.addressFinding(modalEntity.turnNumber, 'continuity');
      setAddressed((prev) => new Set(prev).add(modalEntity.turnNumber));
      refreshCorpus();
      const entity = await client.getEntity(slug);
      setSelectedEntity(entity);
      setActiveDrawer('codex');
    } catch (err) {
      console.error('edit in codex entity failed:', err);
    } finally {
      setModalEntity(null);
    }
  };
  ```
- Mount `<AddEntityModal>` inside the app container:
  ```tsx
  <AddEntityModal
    isOpen={modalEntity !== null}
    entityName={modalEntity?.name ?? ''}
    onQuickCreate={handleQuickCreateEntity}
    onEditInCodex={handleEditInCodexEntity}
    onClose={() => setModalEntity(null)}
  />
  ```

- [x] **Step 3: Typecheck the changes**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS with 0 errors.

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/AddEntityModal.tsx frontend/src/App.tsx
git commit -m "feat(frontend): add non-disruptive entity note modal for continuity findings"
```

---

### Task 5: Full Project Verification & Build Gate

**Files:**
- All modified and new files
- Verification of test suites and builds

- [x] **Step 1: Run Go test and vet suite**

Run: `go test -count=1 ./... && go vet ./...`
Expected: PASS with 0 failures, clean vet.

- [x] **Step 2: Run frontend typecheck and production build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: PASS with 0 errors, bundle generated in `pkg/gui/dist`.

- [x] **Step 3: Restore .gitkeep**

Run: `git checkout -- pkg/gui/dist/.gitkeep`
Expected: Working tree clean of deleted `.gitkeep`.

- [x] **Step 4: Verify complete binary compilation**

Run: `go build -o /dev/null ./cmd/localrpg`
Expected: Exit code 0 with embedded assets.
