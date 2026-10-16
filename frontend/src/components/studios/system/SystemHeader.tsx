import type { FormEvent } from 'react';
import {
  BookOpen,
  Download,
  FileCode,
  FlaskConical,
  Lightbulb,
  Info,
  Play,
  RotateCcw,
  Save,
  SlidersHorizontal,
  Trash2,
  Wand2,
} from 'lucide-react';
import { SystemVerifyResult } from '../../../types';
import { SystemTab } from './types';

export interface SystemHeaderProps {
  savedID: string | null;
  name: string;
  slugID: string;
  activeTab: SystemTab;
  setActiveTab: (tab: SystemTab) => void;
  isSaving: boolean;
  isTesting: boolean;
  isGeneratingAll: boolean;
  isExplaining: boolean;
  verificationResult: SystemVerifyResult | null;
  handleSave: (event: FormEvent) => void | Promise<void>;
  handleRunTests: () => void | Promise<void>;
  handleExport: () => void | Promise<void>;
  handleExplain: () => void | Promise<void>;
  handleResetToReference: () => void;
  handleGenerateAllSystemFields: () => void | Promise<void>;
  setShowEnhanceDialog: (open: boolean) => void;
  setDeleteTarget: (target: { id: string; name: string }) => void;
  setDeleteSystemError: (error: string | null) => void;
  setCanForceDelete: (canForce: boolean) => void;
}

// SystemHeader is the detail column's title, its tab bar, and the actions that run
// against the open system: verify, test, explain, export, generate and delete.
export function SystemHeader({
  savedID,
  name,
  slugID,
  activeTab,
  setActiveTab,
  isSaving,
  isTesting,
  isGeneratingAll,
  isExplaining,
  verificationResult,
  handleSave,
  handleRunTests,
  handleExport,
  handleExplain,
  handleResetToReference,
  handleGenerateAllSystemFields,
  setShowEnhanceDialog,
  setDeleteTarget,
  setDeleteSystemError,
  setCanForceDelete,
}: SystemHeaderProps) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/10 pb-3 shrink-0">
      <div className="flex items-center gap-3">
        <h2 className="font-sans text-lg font-bold text-purple-400">
          {savedID ? name || 'Edit System' : 'Create New System'}
        </h2>
        {slugID && (
          <span className="text-xs font-mono text-stone-400 bg-stone-950 px-2 py-0.5 rounded border border-stone-800">
            systems/{slugID}
          </span>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex flex-wrap bg-stone-950/80 p-1 rounded-xl border border-stone-800">
          <button
            type="button"
            onClick={() => setActiveTab('manifest')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
              activeTab === 'manifest' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-white'
            }`}
          >
            <Info className="w-3.5 h-3.5" />
            <span>Manifest</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('rules')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
              activeTab === 'rules' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-white'
            }`}
          >
            <BookOpen className="w-3.5 h-3.5" />
            <span>rules.md</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('mechanics')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
              activeTab === 'mechanics'
                ? 'bg-purple-600 text-white font-bold shadow'
                : 'text-stone-400 hover:text-white'
            }`}
          >
            <SlidersHorizontal className="w-3.5 h-3.5" />
            <span>Mechanics</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('script')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
              activeTab === 'script' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-white'
            }`}
          >
            <FileCode className="w-3.5 h-3.5" />
            <span>mechanics.js</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('tests')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
              activeTab === 'tests' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-white'
            }`}
          >
            <FlaskConical className="w-3.5 h-3.5" />
            <span>Tests</span>
          </button>
        </div>

        <button
          type="button"
          onClick={handleGenerateAllSystemFields}
          disabled={isGeneratingAll || isSaving}
          title="Auto-fill empty system fields with AI"
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-purple-500/40 bg-purple-600/15 hover:bg-purple-600/25 text-purple-300 transition-all cursor-pointer disabled:opacity-50"
        >
          <Wand2 className={`w-3.5 h-3.5 ${isGeneratingAll ? 'animate-spin' : ''}`} />
          <span className="hidden sm:inline">{isGeneratingAll ? 'Generating...' : 'Auto-Fill'}</span>
        </button>

        <button
          type="button"
          onClick={handleResetToReference}
          title="Reset current editor to the comprehensive Narrative 2d6 reference template"
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-stone-800 hover:border-purple-500/50 bg-stone-900/60 hover:bg-stone-800 text-stone-300 hover:text-purple-400 transition-all cursor-pointer"
        >
          <RotateCcw className="w-3.5 h-3.5" />
          <span className="hidden sm:inline">Reset Template</span>
        </button>

        <button
          type="button"
          onClick={handleRunTests}
          disabled={isTesting || isSaving}
          title="Run the system's stored scenarios"
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-emerald-500/40 bg-emerald-600/15 hover:bg-emerald-600/25 text-emerald-300 transition-all cursor-pointer disabled:opacity-50"
        >
          <Play className="w-3.5 h-3.5" />
          <span className="hidden sm:inline">{isTesting ? 'Testing...' : 'Run Tests'}</span>
        </button>

        <button
          type="button"
          onClick={() => setShowEnhanceDialog(true)}
          disabled={!savedID}
          title="Propose additions to this system with AI"
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-amber-500/40 bg-amber-600/15 hover:bg-amber-600/25 text-amber-300 transition-all cursor-pointer disabled:opacity-50"
        >
          <Lightbulb className="w-3.5 h-3.5" />
          <span className="hidden sm:inline">Enhance</span>
        </button>

        <button
          type="button"
          onClick={() => void handleExplain()}
          disabled={!savedID || isExplaining}
          title="Explain this system's mechanics in plain language"
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-sky-500/40 bg-sky-600/15 hover:bg-sky-600/25 text-sky-300 transition-all cursor-pointer disabled:opacity-50"
        >
          <BookOpen className="w-3.5 h-3.5" />
          <span className="hidden sm:inline">{isExplaining ? 'Explaining...' : 'Explain'}</span>
        </button>

        <button
          type="button"
          onClick={handleExport}
          disabled={!savedID}
          title="Export system package (.lrpgsystem)"
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
              setDeleteSystemError(null);
              setCanForceDelete(false);
            }}
            title="Delete this system"
            className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-red-500/30 bg-red-950/20 hover:bg-red-950/40 text-red-300 hover:text-red-200 transition-all cursor-pointer"
          >
            <Trash2 className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Delete System</span>
          </button>
        )}

        <button
          onClick={handleSave}
          disabled={isSaving || (verificationResult !== null && !verificationResult.ok)}
          title={
            verificationResult && !verificationResult.ok
              ? 'Cannot save: verification failed. Regenerate or fix errors.'
              : undefined
          }
          className="flex items-center gap-1.5 text-xs font-sans font-bold px-4 py-2 rounded-xl bg-purple-600 hover:bg-purple-500 text-white shadow-[0_0_15px_rgba(168,85,247,0.35)] active:scale-95 transition-all cursor-pointer disabled:opacity-50"
        >
          <Save className="w-3.5 h-3.5" />
          <span>{isSaving ? 'Saving...' : 'Save System'}</span>
        </button>
      </div>
    </div>
  );
}
