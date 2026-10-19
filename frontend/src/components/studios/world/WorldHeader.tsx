import type { FormEvent } from 'react';
import { BookOpen, Download, FileText, Info, Lightbulb, PackagePlus, Save, Trash2, Wand2 } from 'lucide-react';
import { PACKAGE_EXTENSIONS } from '../../../lib/constants';
import { WorldEntitySummary, WorldSelection } from '../../../types';

export interface WorldHeaderProps {
  savedID: string | null;
  name: string;
  slugID: string;
  selection: WorldSelection;
  entities: WorldEntitySummary[];
  activeTab: 'lore' | 'prompt' | 'entities';
  setActiveTab: (tab: 'lore' | 'prompt' | 'entities') => void;
  isSaving: boolean;
  isGeneratingAll: boolean;
  handleSaveWorld: (event: FormEvent) => void | Promise<void>;
  handleExport: () => void | Promise<void>;
  handleLoadReferenceTemplate: () => void;
  handleGenerateAllWorldFields: () => void | Promise<void>;
  setEnhanceWorldId: (id: string | null) => void;
  setBatchWorldId: (id: string | null) => void;
  setDeleteTarget: (target: { id: string; name: string }) => void;
  setDeleteWorldError: (error: string | null) => void;
  setCanForceDelete: (canForce: boolean) => void;
}

// WorldHeader is the detail column's title, its tab bar, and the actions that run
// against the open world: save, export, seed from a template, generate and delete.
export function WorldHeader({
  savedID,
  name,
  slugID,
  selection,
  entities,
  activeTab,
  setActiveTab,
  isSaving,
  isGeneratingAll,
  handleSaveWorld,
  handleExport,
  handleLoadReferenceTemplate,
  handleGenerateAllWorldFields,
  setEnhanceWorldId,
  setBatchWorldId,
  setDeleteTarget,
  setDeleteWorldError,
  setCanForceDelete,
}: WorldHeaderProps) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/10 pb-3 shrink-0">
      <div className="flex items-center gap-3">
        <h2 className="font-sans text-lg font-bold text-purple-400">
          {selection?.kind === 'saved' ? name || 'Edit World' : 'Create New World'}
        </h2>
        {slugID && (
          <span className="text-xs font-mono text-stone-400 bg-stone-950 px-2 py-0.5 rounded border border-stone-800">
            worlds/{slugID}
          </span>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {savedID && (
          <>
            <button
              type="button"
              onClick={() => setBatchWorldId(savedID)}
              title="Generate a batch of entities for this world"
              className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl border border-sky-500/40 bg-sky-600/20 hover:bg-sky-600/30 text-sky-300 transition-all cursor-pointer"
            >
              <PackagePlus className="w-3.5 h-3.5" />
              <span>Generate entities</span>
            </button>
            <button
              type="button"
              onClick={() => setEnhanceWorldId(savedID)}
              title="Propose lore, entities, and hooks for this world"
              className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl border border-amber-500/40 bg-amber-600/20 hover:bg-amber-600/30 text-amber-300 transition-all cursor-pointer"
            >
              <Lightbulb className="w-3.5 h-3.5" />
              <span>Enhance</span>
            </button>
          </>
        )}
        <div className="flex flex-wrap bg-stone-950/80 p-1 rounded-xl border border-stone-800">
          <button
            type="button"
            onClick={() => setActiveTab('lore')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
              activeTab === 'lore' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-white'
            }`}
          >
            <Info className="w-3.5 h-3.5" />
            <span>Lore & Atmosphere</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('prompt')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
              activeTab === 'prompt' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-white'
            }`}
          >
            <BookOpen className="w-3.5 h-3.5" />
            <span>lore.md</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('entities')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
              activeTab === 'entities' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-white'
            }`}
          >
            <FileText className="w-3.5 h-3.5" />
            <span>Starter Entities ({entities.length})</span>
          </button>
        </div>

        <button
          type="button"
          onClick={handleGenerateAllWorldFields}
          disabled={isGeneratingAll || isSaving}
          title="Auto-fill empty world fields with AI"
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-purple-500/40 bg-purple-600/15 hover:bg-purple-600/25 text-purple-300 transition-all cursor-pointer disabled:opacity-50"
        >
          <Wand2 className={`w-3.5 h-3.5 ${isGeneratingAll ? 'animate-spin' : ''}`} />
          <span className="hidden sm:inline">{isGeneratingAll ? 'Generating...' : 'Auto-Fill'}</span>
        </button>

        <button
          type="button"
          onClick={handleLoadReferenceTemplate}
          title="Load the comprehensive Ashen Reach reference template"
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-stone-800 hover:border-purple-500/50 bg-stone-900/60 hover:bg-stone-800 text-stone-300 hover:text-purple-400 transition-all cursor-pointer"
        >
          <BookOpen className="w-3.5 h-3.5" />
          <span className="hidden sm:inline">Load Reference Template</span>
        </button>

        <button
          type="button"
          onClick={handleExport}
          disabled={!savedID}
          title={`Export world package (${PACKAGE_EXTENSIONS.WORLD})`}
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-white/10 bg-white/5 hover:bg-white/10 text-neutral-300 transition-all cursor-pointer disabled:opacity-50"
        >
          <Download className="w-3.5 h-3.5" />
          <span className="hidden sm:inline">Export</span>
        </button>

        {savedID && (
          <button
            type="button"
            onClick={() => {
              setDeleteTarget({ id: savedID, name: name || savedID });
              setDeleteWorldError(null);
              setCanForceDelete(false);
            }}
            title="Delete this world"
            className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-red-500/30 bg-red-950/20 hover:bg-red-950/40 text-red-300 hover:text-red-200 transition-all cursor-pointer"
          >
            <Trash2 className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Delete World</span>
          </button>
        )}

        <button
          onClick={handleSaveWorld}
          disabled={isSaving}
          className="flex items-center gap-1.5 text-xs font-sans font-bold px-4 py-2 rounded-xl bg-purple-600 hover:bg-purple-500 text-white shadow-[0_0_15px_rgba(168,85,247,0.35)] active:scale-95 transition-all cursor-pointer disabled:opacity-50"
        >
          <Save className="w-3.5 h-3.5" />
          <span>{isSaving ? 'Saving...' : 'Save World'}</span>
        </button>
      </div>
    </div>
  );
}
