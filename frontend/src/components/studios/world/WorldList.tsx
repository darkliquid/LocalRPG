import type { ChangeEvent, RefObject } from 'react';
import { Globe, Plus, Sparkles, Trash2, Upload } from 'lucide-react';
import { WorldDraft, WorldInfo, WorldSelection } from '../../../types';

export interface WorldListProps {
  worlds: WorldInfo[];
  selection: WorldSelection;
  draft: WorldDraft | null;
  name: string;
  isLoading: boolean;
  importInputRef: RefObject<HTMLInputElement | null>;
  handleFileSelect: (event: ChangeEvent<HTMLInputElement>) => void;
  requestSelection: (target: WorldSelection) => void;
  setShowGenerate: (open: boolean) => void;
  setDeleteTarget: (target: { id: string; name: string } | null) => void;
  setDeleteWorldError: (error: string | null) => void;
  setCanForceDelete: (canForce: boolean) => void;
}

// WorldList is the studio's master column: every world on disk, the unsaved draft
// when there is one, and the controls that make another.
export function WorldList({
  worlds,
  selection,
  draft,
  name,
  isLoading,
  importInputRef,
  handleFileSelect,
  requestSelection,
  setShowGenerate,
  setDeleteTarget,
  setDeleteWorldError,
  setCanForceDelete,
}: WorldListProps) {
  return (
    <aside className="w-full md:w-80 h-full bg-stone-950/70 border-r border-white/10 p-4 flex flex-col gap-4 shrink-0 overflow-hidden">
      <div className="flex flex-col gap-2 pb-2 border-b border-stone-800/60 shrink-0">
        <div className="flex items-center gap-2">
          <Globe className="w-4 h-4 text-purple-400" />
          <h3 className="font-sans text-sm font-bold text-stone-200 uppercase tracking-wider">Worlds Studio</h3>
        </div>
        <input
          type="file"
          ref={importInputRef}
          onChange={handleFileSelect}
          accept=".lrpgworld,.lrpgpack"
          className="hidden"
        />
        <div className="flex items-center gap-1.5">
          <button
            onClick={() => setShowGenerate(true)}
            title="Generate a world with AI"
            className="flex-1 flex items-center justify-center gap-1 text-xs font-sans px-2 py-1.5 rounded-lg border border-purple-500/40 bg-purple-600/20 hover:bg-purple-600/30 text-purple-300 transition-all cursor-pointer whitespace-nowrap"
          >
            <Sparkles className="w-3.5 h-3.5" />
            <span>Generate</span>
          </button>
          <button
            onClick={() => importInputRef.current?.click()}
            title="Import world package (.lrpgworld, .lrpgpack)"
            className="flex-1 flex items-center justify-center gap-1 text-xs font-sans px-2 py-1.5 rounded-lg border border-white/10 bg-white/5 hover:bg-white/10 text-neutral-300 transition-all cursor-pointer whitespace-nowrap"
          >
            <Upload className="w-3.5 h-3.5" />
            <span>Import</span>
          </button>
          <button
            onClick={() => requestSelection({ kind: 'draft' })}
            className="flex-1 flex items-center justify-center gap-1 text-xs font-sans font-bold px-2 py-1.5 rounded-lg bg-purple-600 hover:bg-purple-500 text-white transition-all cursor-pointer shadow whitespace-nowrap"
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
              selection?.kind === 'draft'
                ? 'bg-purple-950/30 border-purple-500/50'
                : 'bg-stone-900/40 border-stone-700 hover:bg-stone-800/40'
            }`}
          >
            <div className="flex items-center justify-between gap-2">
              <h4 className="font-sans text-xs font-bold text-stone-200 truncate">{name.trim() || 'Untitled World'}</h4>
              <span className="text-xs font-mono text-amber-300 bg-amber-950/40 border border-amber-500/30 px-1.5 py-0.5 rounded">
                unsaved
              </span>
            </div>
          </div>
        )}
        {isLoading && worlds.length === 0 ? (
          <div className="text-center py-8 text-xs font-mono text-stone-500 animate-pulse">Loading worlds...</div>
        ) : worlds.length === 0 ? (
          <div className="text-center py-8 text-xs text-stone-400">
            No worlds created yet. Craft your first setting!
          </div>
        ) : (
          worlds.map((w) => (
            <div
              key={w.id}
              onClick={() => requestSelection({ kind: 'saved', id: w.id })}
              className={`group p-3 rounded-xl border transition-all cursor-pointer text-left ${
                selection?.kind === 'saved' && selection.id === w.id
                  ? 'bg-purple-950/30 border-purple-500/50 shadow-[0_0_15px_rgba(168,85,247,0.15)]'
                  : 'bg-stone-900/40 border-stone-800/60 hover:bg-stone-800/40 hover:border-stone-700'
              }`}
            >
              <div className="flex items-start justify-between gap-1">
                <div className="min-w-0 flex-1">
                  <h4 className="font-sans text-xs font-bold text-stone-200 truncate">{w.name}</h4>
                  {w.genre && (
                    <div className="mt-1">
                      <span className="inline-block text-xs font-mono text-purple-400 bg-stone-950 px-1.5 py-0.5 rounded border border-stone-800">
                        {w.genre}
                      </span>
                    </div>
                  )}
                </div>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    setDeleteTarget({ id: w.id, name: w.name });
                    setDeleteWorldError(null);
                    setCanForceDelete(false);
                  }}
                  aria-label={`Delete world "${w.name}"`}
                  title={`Delete world "${w.name}"`}
                  className="p-1 text-stone-500 hover:text-red-400 rounded hover:bg-red-950/40 transition-colors opacity-40 group-hover:opacity-100 hover:!opacity-100 shrink-0 cursor-pointer"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
              {w.description && <p className="text-xs text-stone-400 truncate mt-1">{w.description}</p>}
            </div>
          ))
        )}
      </div>
    </aside>
  );
}
