import type { ChangeEvent, RefObject } from 'react';
import { Lightbulb, Plus, Shield, Trash2, Upload, Wand2 } from 'lucide-react';
import { ReferenceSystem, SystemInfo } from '../../../types';
import { PACKAGE_ACCEPT, PACKAGE_EXTENSIONS } from '../../../lib/constants';
import { SystemDraft, SystemSelection } from './types';

export interface SystemListProps {
  systems: SystemInfo[];
  selection: SystemSelection;
  draft: SystemDraft | null;
  name: string;
  description: string;
  referenceSystems: ReferenceSystem[];
  isLoading: boolean;
  isDraft: boolean;
  importInputRef: RefObject<HTMLInputElement | null>;
  handleFileSelect: (event: ChangeEvent<HTMLInputElement>) => void;
  handleNewSystem: () => void;
  requestSelection: (target: SystemSelection) => void;
  setPendingSelection: (selection: SystemSelection) => void;
  setShowGenerateDialog: (open: boolean) => void;
  setShowBaseCatalogue: (open: boolean) => void;
  setDeleteTarget: (target: { id: string; name: string }) => void;
  setDeleteSystemError: (error: string | null) => void;
  setCanForceDelete: (canForce: boolean) => void;
}

// SystemList is the studio's master column: every system on disk, the unsaved
// draft when there is one, and the controls that make another.
export function SystemList({
  systems,
  selection,
  draft,
  name,
  description,
  referenceSystems,
  isLoading,
  isDraft,
  importInputRef,
  handleFileSelect,
  handleNewSystem,
  requestSelection,
  setPendingSelection,
  setShowGenerateDialog,
  setShowBaseCatalogue,
  setDeleteTarget,
  setDeleteSystemError,
  setCanForceDelete,
}: SystemListProps) {
  return (
    <aside className="w-full md:w-80 h-full bg-stone-950/70 border-r border-white/10 p-4 flex flex-col gap-4 shrink-0 overflow-hidden">
      <div className="flex flex-col gap-2 pb-2 border-b border-stone-800/60 shrink-0">
        <div className="flex items-center gap-2">
          <Shield className="w-4 h-4 text-purple-400" />
          <h3 className="font-sans text-sm font-bold text-stone-200 uppercase tracking-wider">Rule Systems</h3>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <input
            type="file"
            ref={importInputRef}
            onChange={handleFileSelect}
            accept={PACKAGE_ACCEPT.SYSTEM}
            className="hidden"
            aria-label="Choose a system package to import"
          />
          <button
            onClick={() => setShowGenerateDialog(true)}
            title="Generate a system with AI"
            className="flex items-center gap-1 text-xs font-sans px-2 py-1 rounded-lg border border-purple-500/30 bg-purple-500/10 hover:bg-purple-500/20 text-purple-300 transition-all cursor-pointer"
          >
            <Wand2 className="w-3.5 h-3.5 text-purple-400" />
            <span>Generate</span>
          </button>
          <button
            onClick={() => setShowBaseCatalogue(true)}
            disabled={referenceSystems.length === 0}
            title="Start from a reference base system"
            className="flex items-center gap-1 text-xs font-sans px-2 py-1 rounded-lg border border-amber-500/30 bg-amber-500/10 hover:bg-amber-500/20 text-amber-300 transition-all cursor-pointer disabled:opacity-50"
          >
            <Lightbulb className="w-3.5 h-3.5 text-amber-400" />
            <span>From a Base</span>
          </button>
          <button
            onClick={() => importInputRef.current?.click()}
            title={`Import system package (${PACKAGE_EXTENSIONS.SYSTEM}, ${PACKAGE_EXTENSIONS.PACK})`}
            className="flex items-center gap-1 text-xs font-sans px-2 py-1 rounded-lg border border-white/10 bg-white/5 hover:bg-white/10 text-neutral-300 transition-all cursor-pointer"
          >
            <Upload className="w-3.5 h-3.5" />
            <span>Import</span>
          </button>
          <button
            onClick={() => {
              if (isDraft && draft?.dirty) {
                setPendingSelection({ kind: 'draft' });
                return;
              }
              handleNewSystem();
            }}
            className="flex items-center gap-1 text-xs font-sans font-bold px-2.5 py-1 rounded-lg bg-purple-600 hover:bg-purple-500 text-white transition-all cursor-pointer shadow"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>New</span>
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto space-y-2 pr-1 min-h-0">
        {draft && (
          <div
            onClick={() => requestSelection({ kind: 'draft' })}
            className={`p-3 rounded-xl border transition-all cursor-pointer text-left border-dashed ${
              isDraft
                ? 'bg-purple-950/30 border-purple-500/50 shadow-[0_0_15px_rgba(168,85,247,0.15)]'
                : 'bg-stone-900/40 border-stone-700 hover:bg-stone-800/40'
            }`}
          >
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-1.5 min-w-0">
                <Shield className="w-3.5 h-3.5 text-purple-400 shrink-0" />
                <span className="font-serif font-bold text-sm text-stone-200 truncate">
                  {name.trim() || 'Untitled System'}
                </span>
              </div>
              <span className="text-xs font-mono px-1.5 py-0.5 rounded bg-purple-500/20 text-purple-300 shrink-0">
                Draft
              </span>
            </div>
            <div className="text-xs text-stone-400 truncate mt-1">{description.trim() || 'Unsaved system draft'}</div>
          </div>
        )}
        {isLoading && systems.length === 0 ? (
          <div className="text-center py-8 text-xs font-mono text-stone-500 animate-pulse">Loading systems...</div>
        ) : systems.length === 0 ? (
          <div className="text-center py-8 text-xs text-stone-400">
            No systems defined yet. Create your first ruleset!
          </div>
        ) : (
          systems.map((s) => (
            <div
              key={s.id}
              onClick={() => requestSelection({ kind: 'saved', id: s.id })}
              className={`group p-3 rounded-xl border transition-all cursor-pointer text-left ${
                selection?.kind === 'saved' && selection.id === s.id
                  ? 'bg-purple-950/30 border-purple-500/50 shadow-[0_0_15px_rgba(168,85,247,0.15)]'
                  : 'bg-stone-900/40 border-stone-800/60 hover:bg-stone-800/40 hover:border-stone-700'
              }`}
            >
              <div className="flex items-start justify-between gap-1">
                <div className="min-w-0 flex-1">
                  <h4 className="font-sans text-xs font-bold text-stone-200 truncate">{s.name}</h4>
                  <span className="text-xs font-mono text-stone-400 bg-stone-950 px-1.5 py-0.5 rounded border border-stone-800 mt-1 inline-block">
                    v{s.version}
                  </span>
                </div>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    setDeleteTarget({ id: s.id, name: s.name });
                    setDeleteSystemError(null);
                    setCanForceDelete(false);
                  }}
                  aria-label={`Delete system "${s.name}"`}
                  title={`Delete system "${s.name}"`}
                  className="p-1 text-stone-500 hover:text-red-400 rounded hover:bg-red-950/40 transition-colors opacity-40 group-hover:opacity-100 hover:!opacity-100 shrink-0 cursor-pointer"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
              {s.description && <p className="text-xs text-stone-400 truncate mt-1">{s.description}</p>}
            </div>
          ))
        )}
      </div>
    </aside>
  );
}
