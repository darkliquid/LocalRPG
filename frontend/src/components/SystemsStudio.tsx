import React, { useState, useEffect, useRef } from 'react';
import { APIClient } from '../api/client';
import { SystemInfo, CreateSystemRequest, CharacterCreationField, GenerationFailure, MechanicsSpec, ReferenceSystem, SystemTestFailure, ContentManifestInfo } from '../types';
import { Shield, Plus, Save, FileCode, Info, Check, AlertCircle, RotateCcw, BookOpen, Trash2, Wand2, SlidersHorizontal, Play, Download, Upload } from 'lucide-react';
import { AIGenerateButton } from './ui/AIGenerateButton';
import { formatGenerationError } from '../lib/generationError';
import { DiscardDraftConfirm } from './launcher/DiscardDraftConfirm';
import MarkdownEditor from './editor/MarkdownEditor';
import { MechanicsEditor } from './MechanicsEditor';
import { ContentImportDialog } from './ContentImportDialog';
import { inspectPackageFile } from '../lib/packageInspect';
import { useSaveFilePicker } from '../hooks/useSaveFilePicker';

type SystemSelection = { kind: 'saved'; id: string } | { kind: 'draft' } | null;

interface SystemDraft {
  localId: string;
  dirty: boolean;
}

interface SystemsStudioProps {
  onSystemSaved?: () => void;
  startMode?: 'new' | 'browse';
}

export const SystemsStudio: React.FC<SystemsStudioProps> = ({ onSystemSaved, startMode = 'browse' }) => {
  const [systems, setSystems] = useState<SystemInfo[]>([]);
  const [referenceSystems, setReferenceSystems] = useState<ReferenceSystem[]>([]);
  const [selection, setSelection] = useState<SystemSelection>(null);
  const [draft, setDraft] = useState<SystemDraft | null>(null);
  const [pendingSelection, setPendingSelection] = useState<SystemSelection>(null);
  const [activeTab, setActiveTab] = useState<'manifest' | 'rules' | 'mechanics' | 'script'>('manifest');
  const startModeRef = React.useRef(startMode);

  // Form state
  const [name, setName] = useState('');
  const [slugID, setSlugID] = useState('');
  const [version, setVersion] = useState('1.0.0');
  const [description, setDescription] = useState('');
  const [rulesPrompt, setRulesPrompt] = useState('');
  const [script, setScript] = useState('');
  // The system's declarative mechanics block, edited as one object.
  const [mechanics, setMechanics] = useState<MechanicsSpec>({});
  // Character creation prompts a player answers when starting with this system.
  const [creationPreamble, setCreationPreamble] = useState('');
  const [creationFields, setCreationFields] = useState<CharacterCreationField[]>([]);

  const [isLoading, setIsLoading] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [testFailures, setTestFailures] = useState<SystemTestFailure[]>([]);
  const [isGeneratingAll, setIsGeneratingAll] = useState(false);
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  const [importFile, setImportFile] = useState<File | null>(null);
  const [importManifest, setImportManifest] = useState<ContentManifestInfo | null>(null);
  const [isImporting, setIsImporting] = useState(false);
  const importInputRef = useRef<HTMLInputElement>(null);
  const savePicker = useSaveFilePicker();

  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null);
  const [isDeletingSystem, setIsDeletingSystem] = useState(false);
  const [deleteSystemError, setDeleteSystemError] = useState<string | null>(null);
  const [canForceDelete, setCanForceDelete] = useState(false);

  const isDraft = selection?.kind === 'draft';
  const savedID = selection?.kind === 'saved' ? selection.id : null;
  const markDirty = () => setDraft((d) => (d ? { ...d, dirty: true } : d));

  const reportGenerationError = (failure: GenerationFailure) =>
    setToast({ type: 'error', message: formatGenerationError(failure) });
  const errorMessage = (err: unknown): string =>
    err instanceof Error ? err.message : 'Unexpected error';

  const handleDeleteSystem = async (force = false) => {
    if (!deleteTarget) return;
    setIsDeletingSystem(true);
    setDeleteSystemError(null);
    try {
      await APIClient.deleteSystem(deleteTarget.id, force);
      const deletedName = deleteTarget.name;
      const deletedID = deleteTarget.id;
      setDeleteTarget(null);
      setDeleteSystemError(null);
      setCanForceDelete(false);
      setToast({ type: 'success', message: `System "${deletedName}" deleted.` });

      if (selection?.kind === 'saved' && selection.id === deletedID) {
        handleNewSystem();
      }
      await loadSystems(undefined, 'browse');
      if (onSystemSaved) onSystemSaved();
    } catch (err) {
      const msg = errorMessage(err);
      setDeleteSystemError(msg);
      if (msg.includes('in use') || msg.toLowerCase().includes('conflict')) {
        setCanForceDelete(true);
      }
    } finally {
      setIsDeletingSystem(false);
    }
  };

  useEffect(() => {
    loadSystems(undefined, startModeRef.current);
    loadReferenceSystems();
  }, []);

  const loadReferenceSystems = async () => {
    try {
      const res = await APIClient.listReferenceSystems();
      setReferenceSystems(res.systems ?? []);
    } catch {
      // A failed fetch shows no starting points rather than a stale constant.
      setReferenceSystems([]);
    }
  };

  const loadSystems = async (selectID?: string, mode: 'new' | 'browse' = 'browse') => {
    setIsLoading(true);
    try {
      const list = await APIClient.listSystems();
      setSystems(list);
      if (mode === 'new') {
        handleNewSystem();
        return;
      }
      const target = selectID || (list.length > 0 ? list[0].id : null);
      if (target) {
        loadSystemDetail(target);
      } else {
        handleNewSystem();
      }
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to load systems' });
    } finally {
      setIsLoading(false);
    }
  };

  const loadSystemDetail = async (id: string) => {
    try {
      const detail = await APIClient.getSystem(id);
      setSelection({ kind: 'saved', id: detail.id });
      setDraft(null);
      setName(detail.name);
      setSlugID(detail.id);
      setVersion(detail.version || '1.0.0');
      setDescription(detail.description || '');
      setRulesPrompt(detail.rules_prompt || '');
      setScript(detail.script || '');
      setMechanics(detail.mechanics || {});
      setCreationPreamble(detail.character_creation?.preamble || '');
      setCreationFields(detail.character_creation?.fields || []);
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to load system details' });
    }
  };

  const applySelection = (target: SystemSelection) => {
    setPendingSelection(null);
    if (!target || target.kind === 'draft') {
      handleNewSystem();
      return;
    }
    loadSystemDetail(target.id);
  };

  const requestSelection = (target: SystemSelection) => {
    // Re-selecting the open draft is a no-op; only leaving it can discard work.
    if (target?.kind === 'draft' && isDraft) return;
    if (isDraft && draft?.dirty) {
      setPendingSelection(target);
      return;
    }
    applySelection(target);
  };

  const handleNewSystem = () => {
    setSelection({ kind: 'draft' });
    setDraft({ localId: crypto.randomUUID(), dirty: false });
    setName('');
    setSlugID('');
    setVersion('1.0.0');
    setDescription('');
    setRulesPrompt('');
    setScript('');
    setMechanics({});
    setCreationPreamble('');
    setCreationFields([]);
    setActiveTab('manifest');
  };

  const applyReference = (reference: ReferenceSystem) => {
    setName(reference.name);
    if (!savedID) {
      setSlugID(reference.id);
    }
    setVersion(reference.version);
    setDescription(reference.description);
    setRulesPrompt(reference.rules_prompt);
    setScript(reference.script);
    setMechanics(reference.mechanics ?? {});
    setCreationPreamble('');
    setCreationFields([]);
    markDirty();
    setToast({ type: 'success', message: `Loaded the ${reference.name} reference.` });
  };

  const handleResetToReference = () => {
    const reference =
      referenceSystems.find((r) => r.id === 'narrative_2d6') ?? referenceSystems[0];
    if (!reference) {
      setToast({ type: 'error', message: 'No reference systems are available.' });
      return;
    }
    applyReference(reference);
  };

  const getSystemContext = (): Record<string, string> => ({
    name,
    description,
    rules_prompt: rulesPrompt,
  });

  const handleGenerateAllSystemFields = async () => {
    if (isGeneratingAll) return;
    setIsGeneratingAll(true);
    try {
      const res = await APIClient.generateText({
        form_type: 'system',
        field_name: '_all',
        context: getSystemContext(),
      });
      if (res.fields.name && !name.trim()) setName(res.fields.name);
      if (res.fields.description && !description.trim()) setDescription(res.fields.description);
      if (res.fields.rules_prompt && !rulesPrompt.trim()) setRulesPrompt(res.fields.rules_prompt);
      if (res.warning) {
        setToast({ type: 'error', message: `Partial: ${res.warning.code} — ${res.warning.message}` });
      } else if (Object.keys(res.fields).length === 0) {
        setToast({ type: 'error', message: 'The model returned nothing to fill.' });
      } else {
        setToast({ type: 'success', message: 'Auto-filled system fields!' });
      }
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Auto-fill failed' });
    } finally {
      setIsGeneratingAll(false);
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setToast({ type: 'error', message: 'System Name is required' });
      return;
    }

    setIsSaving(true);
    setToast(null);
    try {
      const payload: CreateSystemRequest = {
        id: slugID.trim() || undefined,
        name: name.trim(),
        version: version.trim() || '1.0.0',
        description: description.trim(),
        rules_prompt: rulesPrompt.trim(),
        script: script,
        character_creation: {
          preamble: creationPreamble.trim() || undefined,
          fields: creationFields,
        },
        mechanics: Object.keys(mechanics).length > 0 ? mechanics : undefined,
      };

      const saved = await APIClient.saveSystem(payload);
      if (saved.warnings && saved.warnings.length > 0) {
        setToast({ type: 'error', message: `Saved with warnings: ${saved.warnings.join('; ')}` });
      } else {
        setToast({ type: 'success', message: `System "${saved.name}" saved successfully!` });
      }
      setDraft(null);
      await loadSystems(saved.id, 'browse');
      if (onSystemSaved) onSystemSaved();
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to save system' });
    } finally {
      setIsSaving(false);
    }
  };

  const handleRunTests = async () => {
    if (!savedID) {
      setToast({ type: 'error', message: 'Save the system before running its scenarios.' });
      return;
    }
    setIsTesting(true);
    setTestFailures([]);
    try {
      const stored = await APIClient.listSystemScenarios(savedID);
      const resp = await APIClient.runSystemTest({
        system: { id: savedID, script, mechanics },
        scenarios: stored.scenarios ?? [],
      });
      const failures = resp.failures ?? [];
      setTestFailures(failures);
      if (failures.length === 0) {
        setToast({ type: 'success', message: `All ${stored.scenarios?.length ?? 0} scenario(s) passed.` });
      } else {
        setToast({ type: 'error', message: `${failures.length} scenario assertion(s) failed.` });
      }
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to run scenarios' });
    } finally {
      setIsTesting(false);
    }
  };

  const handleExport = async () => {
    if (!savedID) return;
    const defaultFilename = `${savedID}-${version || '1.0.0'}.lrpgsystem`;
    try {
      if (savePicker.nativeDialog) {
        const path = await savePicker.pick({
          title: 'Export System Package',
          default_filename: defaultFilename,
          filters: [
            { display_name: 'LocalRPG System (*.lrpgsystem)', pattern: '*.lrpgsystem' },
            { display_name: 'All Files (*.*)', pattern: '*.*' },
          ],
        });
        if (!path) return;
        await APIClient.exportContent('system', savedID, path);
        setToast({ type: 'success', message: `Exported system package: ${path}` });
        return;
      }

      const blob = (await APIClient.exportContent('system', savedID)) as Blob;
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = defaultFilename;
      a.click();
      URL.revokeObjectURL(url);
      setToast({ type: 'success', message: `Exported system package: ${defaultFilename}` });
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) });
    }
  };

  const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    try {
      const info = await inspectPackageFile(file);
      setImportFile(file);
      setImportManifest(info);
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) });
    } finally {
      e.target.value = '';
    }
  };

  const handleConfirmImport = async (conflictMode: 'refuse' | 'rename' | 'overwrite') => {
    if (!importFile) return;
    setIsImporting(true);
    try {
      const res = await APIClient.importContent(importFile, conflictMode, 'system');
      setToast({ type: 'success', message: `Successfully ${res.action} system ${res.name} (${res.id})` });
      setImportFile(null);
      setImportManifest(null);
      await loadSystems(res.id, 'browse');
      if (onSystemSaved) onSystemSaved();
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) });
    } finally {
      setIsImporting(false);
    }
  };

  return (
    <div className="w-full h-full flex flex-col md:flex-row overflow-hidden">
      {/* Left Master Column: Systems List */}
      <aside className="w-full md:w-80 h-full bg-stone-950/70 border-r border-white/10 p-4 flex flex-col gap-4 shrink-0 overflow-hidden">
        <div className="flex items-center justify-between pb-2 border-b border-stone-800/60 shrink-0">
          <div className="flex items-center gap-2">
            <Shield className="w-4 h-4 text-purple-400" />
            <h3 className="font-sans text-sm font-bold text-stone-200 uppercase tracking-wider">
              Rule Systems
            </h3>
          </div>
          <div className="flex items-center gap-1.5">
            <input
              type="file"
              ref={importInputRef}
              onChange={handleFileSelect}
              accept=".lrpgsystem,.lrpgpack"
              className="hidden"
            />
            <button
              onClick={() => importInputRef.current?.click()}
              title="Import system package (.lrpgsystem, .lrpgpack)"
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

        {referenceSystems.length > 0 && (
          <div className="space-y-1.5 shrink-0">
            <span className="text-[11px] font-sans uppercase tracking-wider text-stone-500">Starting points</span>
            <div className="flex flex-wrap gap-1.5">
              {referenceSystems.map((reference) => (
                <button
                  key={reference.id}
                  type="button"
                  onClick={() => applyReference(reference)}
                  className="text-xs font-sans px-2.5 py-1 rounded-lg border border-stone-800 hover:border-purple-500/50 bg-stone-900/60 hover:bg-stone-800 text-stone-300 hover:text-purple-300 cursor-pointer"
                >
                  {reference.name}
                </button>
              ))}
            </div>
          </div>
        )}

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
              <div className="text-xs text-stone-400 truncate mt-1">
                {description.trim() || 'Unsaved system draft'}
              </div>
            </div>
          )}
          {isLoading && systems.length === 0 ? (
            <div className="text-center py-8 text-xs font-mono text-stone-500 animate-pulse">
              Loading systems...
            </div>
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
                    title={`Delete system "${s.name}"`}
                    className="p-1 text-stone-500 hover:text-red-400 rounded hover:bg-red-950/40 transition-colors opacity-40 group-hover:opacity-100 hover:!opacity-100 shrink-0 cursor-pointer"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
                {s.description && (
                  <p className="text-xs text-stone-400 truncate mt-1">{s.description}</p>
                )}
              </div>
            ))
          )}
        </div>
      </aside>

      {/* Right Detail Column: Editor */}
      <section className="flex-1 h-full min-w-0 bg-stone-900/30 p-6 flex flex-col gap-4 overflow-hidden min-h-0">
        {/* Top Header & Sub-Tabs */}
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
                  activeTab === 'manifest'
                    ? 'bg-purple-600 text-white font-bold shadow'
                    : 'text-stone-400 hover:text-white'
                }`}
              >
                <Info className="w-3.5 h-3.5" />
                <span>Manifest</span>
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('rules')}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
                  activeTab === 'rules'
                    ? 'bg-purple-600 text-white font-bold shadow'
                    : 'text-stone-400 hover:text-white'
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
                  activeTab === 'script'
                    ? 'bg-purple-600 text-white font-bold shadow'
                    : 'text-stone-400 hover:text-white'
                }`}
              >
                <FileCode className="w-3.5 h-3.5" />
                <span>mechanics.js</span>
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
              disabled={isSaving}
              className="flex items-center gap-1.5 text-xs font-sans font-bold px-4 py-2 rounded-xl bg-purple-600 hover:bg-purple-500 text-white shadow-[0_0_15px_rgba(168,85,247,0.35)] active:scale-95 transition-all cursor-pointer disabled:opacity-50"
            >
              <Save className="w-3.5 h-3.5" />
              <span>{isSaving ? 'Saving...' : 'Save System'}</span>
            </button>
          </div>
        </div>

        {/* Toast Feedback */}
        {toast && (
          <div
            className={`p-3 rounded-xl text-xs flex items-center gap-2 shrink-0 ${
              toast.type === 'success'
                ? 'bg-emerald-950/60 border border-emerald-500/40 text-emerald-200'
                : 'bg-red-950/60 border border-red-500/40 text-red-200'
            }`}
          >
            {toast.type === 'success' ? (
              <Check className="w-4 h-4 text-emerald-400" />
            ) : (
              <AlertCircle className="w-4 h-4 text-red-400" />
            )}
            <span>{toast.message}</span>
          </div>
        )}

        {testFailures.length > 0 && (
          <div className="p-3 rounded-xl text-xs bg-red-950/40 border border-red-500/40 text-red-200 space-y-1 shrink-0">
            {testFailures.map((failure, index) => (
              <div key={index} className="font-mono">
                {failure.scenario} step {failure.step}: {failure.detail}
              </div>
            ))}
          </div>
        )}

        {/* Tab 1: Manifest Form */}
        {activeTab === 'manifest' && (
          <div className="flex-1 overflow-y-auto min-h-0 space-y-4 pr-2">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                    System Name
                  </label>
                  <AIGenerateButton
                    formType="system"
                    fieldName="name"
                    onError={reportGenerationError}
                    getContext={getSystemContext}
                    onGenerated={(val) => {
                      setName(val);
                      markDirty();
                      if (!savedID) {
                        setSlugID(val.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''));
                      }
                    }}
                  />
                </div>
                <input
                  type="text"
                  required
                  placeholder="e.g. Iron Realm D20"
                  value={name}
                  onChange={(e) => {
                    setName(e.target.value);
                    markDirty();
                    if (!savedID) {
                      setSlugID(e.target.value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''));
                    }
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                  Version
                </label>
                <input
                  type="text"
                  placeholder="1.0.0"
                  value={version}
                  onChange={(e) => { setVersion(e.target.value); markDirty(); }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors font-mono"
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                Directory Slug ID
              </label>
              <input
                type="text"
                disabled={!!savedID}
                placeholder="e.g. iron-realm"
                value={slugID}
                onChange={(e) => { setSlugID(e.target.value); markDirty(); }}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors font-mono disabled:opacity-60"
              />
              <p className="text-xs text-stone-400">
                Unique identifier for the system directory on disk.
              </p>
            </div>

            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                  Rulebook Overview & Philosophy
                </label>
                <AIGenerateButton
                  formType="system"
                  fieldName="description"
                  onError={reportGenerationError}
                  getContext={getSystemContext}
                  onGenerated={(val) => { setDescription(val); markDirty(); }}
                  seed={description}
                />
              </div>
              <textarea
                rows={5}
                placeholder="Describe core dice mechanics, resolution philosophy, and character stats..."
                value={description}
                onChange={(e) => { setDescription(e.target.value); markDirty(); }}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl p-4 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors resize-none"
              />
            </div>

            <div className="space-y-3 pt-2 border-t border-stone-800/70">
              <div className="flex items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                  <BookOpen className="w-4 h-4 text-purple-400" />
                  <span className="font-sans text-xs uppercase font-bold text-stone-200">
                    Character Creation Prompts
                  </span>
                  <span className="text-xs font-mono text-stone-500">({creationFields.length})</span>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    setCreationFields((prev) => [
                      ...prev,
                      { id: `prompt_${prev.length + 1}`, label: 'New Prompt', kind: 'text', generatable: true },
                    ]);
                    markDirty();
                  }}
                  className="flex items-center gap-1 text-xs px-2 py-1 rounded bg-purple-600/20 border border-purple-500/40 text-purple-300 hover:bg-purple-600/30 transition cursor-pointer"
                >
                  <Plus className="w-3 h-3" />
                  <span>Add Prompt</span>
                </button>
              </div>

              <input
                type="text"
                placeholder="Preamble shown above the prompts (optional)"
                value={creationPreamble}
                onChange={(e) => { setCreationPreamble(e.target.value); markDirty(); }}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-xs text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
              />

              {creationFields.length === 0 ? (
                <p className="text-xs text-stone-500">
                  With no prompts defined, the studio falls back to appearance, age, gender, pronouns, background and voice.
                </p>
              ) : (
                <div className="space-y-2">
                  {creationFields.map((field, index) => (
                    <div key={index} className="rounded-lg bg-stone-950/70 border border-stone-800/80 p-2.5 space-y-2">
                      <div className="grid grid-cols-2 gap-2">
                        <input
                          type="text"
                          placeholder="id"
                          value={field.id}
                          onChange={(e) => {
                            const updated = [...creationFields];
                            updated[index] = { ...updated[index], id: e.target.value };
                            setCreationFields(updated);
                            markDirty();
                          }}
                          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs font-mono text-purple-300 focus:outline-none"
                        />
                        <input
                          type="text"
                          placeholder="Label"
                          value={field.label}
                          onChange={(e) => {
                            const updated = [...creationFields];
                            updated[index] = { ...updated[index], label: e.target.value };
                            setCreationFields(updated);
                            markDirty();
                          }}
                          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
                        />
                      </div>
                      <input
                        type="text"
                        placeholder="Prompt shown to the player"
                        value={field.prompt || ''}
                        onChange={(e) => {
                          const updated = [...creationFields];
                          updated[index] = { ...updated[index], prompt: e.target.value };
                          setCreationFields(updated);
                          markDirty();
                        }}
                        className="w-full bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
                      />
                      <div className="flex flex-wrap items-center gap-3 text-xs text-stone-400">
                        <select
                          value={field.kind || 'text'}
                          onChange={(e) => {
                            const updated = [...creationFields];
                            updated[index] = { ...updated[index], kind: e.target.value as CharacterCreationField['kind'] };
                            setCreationFields(updated);
                            markDirty();
                          }}
                          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none cursor-pointer"
                        >
                          {['text', 'long', 'number', 'select', 'voice'].map((kind) => (
                            <option key={kind} value={kind}>{kind}</option>
                          ))}
                        </select>
                        <label className="flex items-center gap-1 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={!!field.required}
                            onChange={(e) => {
                              const updated = [...creationFields];
                              updated[index] = { ...updated[index], required: e.target.checked };
                              setCreationFields(updated);
                              markDirty();
                            }}
                            className="rounded bg-stone-950 border-stone-800 text-purple-600 focus:ring-0"
                          />
                          <span>Required</span>
                        </label>
                        <label className="flex items-center gap-1 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={!!field.generatable}
                            disabled={field.kind === 'voice'}
                            onChange={(e) => {
                              const updated = [...creationFields];
                              updated[index] = { ...updated[index], generatable: e.target.checked };
                              setCreationFields(updated);
                              markDirty();
                            }}
                            className="rounded bg-stone-950 border-stone-800 text-purple-600 focus:ring-0 disabled:opacity-40"
                          />
                          <span>Generatable</span>
                        </label>
                        <button
                          type="button"
                          onClick={() => {
                            setCreationFields((prev) => prev.filter((_, i) => i !== index));
                            markDirty();
                          }}
                          className="ml-auto flex items-center gap-1 text-red-400/80 hover:text-red-300 cursor-pointer"
                        >
                          <Trash2 className="w-3 h-3" />
                          <span>Remove</span>
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {savedID && (
              <div className="p-4 bg-red-950/20 border border-red-500/30 rounded-2xl flex items-center justify-between mt-4">
                <div>
                  <div className="text-xs font-sans font-bold text-red-400 flex items-center gap-1.5">
                    <Trash2 className="w-3.5 h-3.5" />
                    <span>Danger Zone: Delete System</span>
                  </div>
                  <p className="text-xs font-sans text-stone-400 mt-1">
                    Permanently delete this ruleset and its scripts from disk. This cannot be undone.
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    setDeleteTarget({ id: savedID, name: name || savedID });
                    setDeleteSystemError(null);
                    setCanForceDelete(false);
                  }}
                  className="px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white font-sans text-xs font-bold transition-all cursor-pointer shadow"
                >
                  Delete System
                </button>
              </div>
            )}
          </div>
        )}

        {/* Tab 2: Agent Rules Prompt Editor */}
        {activeTab === 'rules' && (
          <div className="flex-1 flex flex-col gap-2 min-h-0 overflow-hidden">
            <div className="flex items-center justify-between text-xs font-mono text-stone-400 px-1 shrink-0">
              <span>AI Storyteller Instructions (prompts/rules.md)</span>
              <div className="flex items-center gap-2">
                <span>Injected into LLM context to guide resolution ladder &amp; mechanics hooks</span>
                <AIGenerateButton
                  formType="system"
                  fieldName="rules_prompt"
                  onError={reportGenerationError}
                  getContext={getSystemContext}
                  onGenerated={(val) => { setRulesPrompt(val); markDirty(); }}
                  seed={rulesPrompt}
                />
              </div>
            </div>
            <MarkdownEditor
              key={`${savedID || slugID || 'draft'}-rules`}
              value={rulesPrompt}
              onChange={(next) => { setRulesPrompt(next); markDirty(); }}
              language="markdown"
              ariaLabel="System rules prompt"
              placeholder="Describe the resolution philosophy, dice mechanics and character stats the engine should follow..."
            />
          </div>
        )}

        {/* Tab 2b: Mechanics Editor */}
        {activeTab === 'mechanics' && (
          <div className="flex-1 overflow-y-auto min-h-0 pr-2">
            <MechanicsEditor
              mechanics={mechanics}
              onChange={(next) => {
                setMechanics(next);
                markDirty();
              }}
            />
          </div>
        )}

        {/* Tab 3: Script Editor */}
        {activeTab === 'script' && (
          <div className="flex-1 flex flex-col gap-2 min-h-0 overflow-hidden">
            <div className="flex items-center justify-between text-xs font-mono text-stone-400 px-1 shrink-0">
              <span>JavaScript Runtime (Goja Sandbox)</span>
              <span>Exports: evaluateRoll(stats, diceExpr)</span>
            </div>
            <MarkdownEditor
              key={`${savedID || slugID || 'draft'}-script`}
              value={script}
              onChange={(next) => { setScript(next); markDirty(); }}
              language="javascript"
              ariaLabel="System mechanics script"
              placeholder="export function evaluateRoll(stats, diceExpr) { ... }"
            />
          </div>
        )}
      </section>

      {/* Discard Draft Confirmation Dialog */}
      <DiscardDraftConfirm
        isOpen={pendingSelection !== null}
        title="Discard unsaved system?"
        description="This system has not been saved. Leaving now discards every field you entered."
        onCancel={() => setPendingSelection(null)}
        onDiscard={() => applySelection(pendingSelection)}
      />

      {importManifest && (
        <ContentImportDialog
          manifest={importManifest}
          expectedType="system"
          onConfirm={handleConfirmImport}
          onCancel={() => {
            setImportFile(null);
            setImportManifest(null);
          }}
          loading={isImporting}
        />
      )}

      {deleteTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm select-none">
          <div className="w-full max-w-md bg-stone-900 border border-white/15 rounded-2xl p-5 shadow-2xl space-y-4">
            <div className="flex items-center gap-2 text-red-400">
              <Trash2 className="w-5 h-5 shrink-0" />
              <h3 className="font-sans text-sm font-bold text-white">
                Delete system &ldquo;{deleteTarget.name}&rdquo;?
              </h3>
            </div>
            <p className="text-xs font-sans text-stone-300">
              This will permanently delete the system directory and all of its mechanics and prompts. This action cannot be undone.
            </p>
            {deleteSystemError && (
              <div className="p-3 bg-red-950/40 border border-red-500/30 rounded-xl text-xs text-red-300 flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
                <div className="space-y-1">
                  <p>{deleteSystemError}</p>
                  {canForceDelete && (
                    <p className="text-stone-400">
                      You can force deletion to remove the system anyway. Existing campaigns or worlds using this system may fail to resolve mechanics.
                    </p>
                  )}
                </div>
              </div>
            )}
            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => {
                  setDeleteTarget(null);
                  setDeleteSystemError(null);
                  setCanForceDelete(false);
                }}
                disabled={isDeletingSystem}
                className="text-xs font-sans px-3 py-1.5 rounded-lg border border-stone-700 text-stone-300 hover:text-white hover:bg-stone-800 transition-all cursor-pointer disabled:opacity-50"
              >
                Cancel
              </button>
              {canForceDelete ? (
                <button
                  type="button"
                  onClick={() => void handleDeleteSystem(true)}
                  disabled={isDeletingSystem}
                  className="text-xs font-sans font-bold px-3 py-1.5 rounded-lg bg-red-700 hover:bg-red-600 text-white transition-all cursor-pointer disabled:opacity-50 flex items-center gap-1.5"
                >
                  {isDeletingSystem ? 'Deleting...' : 'Force Delete'}
                </button>
              ) : (
                <button
                  type="button"
                  onClick={() => void handleDeleteSystem(false)}
                  disabled={isDeletingSystem}
                  className="text-xs font-sans font-bold px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white transition-all cursor-pointer disabled:opacity-50 flex items-center gap-1.5"
                >
                  {isDeletingSystem ? 'Deleting...' : 'Delete System'}
                </button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
