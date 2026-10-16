import { errorMessage } from '../lib/errors';
import React, { useState, useEffect, useRef } from 'react';
import { APIClient } from '../api/client';
import {
  SystemInfo,
  CreateSystemRequest,
  CharacterCreationField,
  GenerationFailure,
  MechanicsSpec,
  ReferenceSystem,
  SystemTestFailure,
  ContentManifestInfo,
  SystemDraftInfo,
  SystemVerifyResult,
  Scenario,
} from '../types';
import { Info, Check, AlertCircle, Trash2 } from 'lucide-react';
import { formatGenerationError } from '../lib/generationError';
import { DiscardDraftConfirm } from './launcher/DiscardDraftConfirm';
import { Modal } from './Modal';
import { SystemTestsTab } from './studios/system/SystemTestsTab';
import { SystemScriptTab } from './studios/system/SystemScriptTab';
import { SystemMechanicsTab } from './studios/system/SystemMechanicsTab';
import { SystemRulesTab } from './studios/system/SystemRulesTab';
import { SystemManifestTab } from './studios/system/SystemManifestTab';
import { SystemList } from './studios/system/SystemList';
import { SystemHeader } from './studios/system/SystemHeader';
import { SystemDraft, SystemSelection } from './studios/system/types';
import { ContentImportDialog } from './ContentImportDialog';
import { SystemGenerateDialog } from './SystemGenerateDialog';
import { SystemEnhanceDialog } from './SystemEnhanceDialog';
import { BaseSystemCatalogue } from './BaseSystemCatalogue';
import { MarkdownDocViewer } from './MarkdownDocViewer';
import { inspectPackageFile } from '../lib/packageInspect';
import { useSaveFilePicker } from '../hooks/useSaveFilePicker';

interface SystemsStudioProps {
  onSystemSaved?: () => void;
  startMode?: 'new' | 'browse';
}

export const SystemsStudio: React.FC<SystemsStudioProps> = ({ onSystemSaved, startMode = 'browse' }) => {
  const [systems, setSystems] = useState<SystemInfo[]>([]);
  const [referenceSystems, setReferenceSystems] = useState<ReferenceSystem[]>([]);
  const [selection, setSelection] = useState<SystemSelection>(null);
  const [draft, setDraft] = useState<SystemDraft | null>(null);
  const [activeDraftID, setActiveDraftID] = useState<string | null>(null);
  const [pendingSelection, setPendingSelection] = useState<SystemSelection>(null);
  const [activeTab, setActiveTab] = useState<'manifest' | 'rules' | 'mechanics' | 'script' | 'tests'>('manifest');
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
  const [scenarios, setScenarios] = useState<Scenario[]>([]);
  const [isLoadingScenarios, setIsLoadingScenarios] = useState(false);
  const [editingScenario, setEditingScenario] = useState<Scenario | null>(null);
  const [scenarioError, setScenarioError] = useState<string | null>(null);
  const [isGeneratingAll, setIsGeneratingAll] = useState(false);
  const [showGenerateDialog, setShowGenerateDialog] = useState(false);
  const [showEnhanceDialog, setShowEnhanceDialog] = useState(false);
  const [showBaseCatalogue, setShowBaseCatalogue] = useState(false);
  const [isDeriving, setIsDeriving] = useState(false);
  const [explanation, setExplanation] = useState<string | null>(null);
  const [isExplaining, setIsExplaining] = useState(false);
  const [verificationResult, setVerificationResult] = useState<SystemVerifyResult | null>(null);
  const [draftNotes, setDraftNotes] = useState<string[]>([]);
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

  const handleDraftProduced = (generatedDraft: SystemDraftInfo) => {
    setShowGenerateDialog(false);
    setActiveDraftID(generatedDraft.id || null);
    setName(generatedDraft.name);
    setSlugID(generatedDraft.id);
    setVersion(generatedDraft.version || '1.0.0');
    setDescription(generatedDraft.description);
    setMechanics(generatedDraft.mechanics || {});
    setScript(generatedDraft.script || '');
    setRulesPrompt(generatedDraft.rules_prompt || '');
    setVerificationResult(generatedDraft.verify);
    setDraftNotes(generatedDraft.notes || []);
    setSelection({ kind: 'draft' });
    setDraft({ localId: generatedDraft.id || 'draft', dirty: true });
    setToast({ type: 'success', message: `Draft loaded for "${generatedDraft.name}". Review and save.` });
  };

  const handleDerive = async (base: { id: string; name: string }, instruction: string) => {
    setIsDeriving(true);
    try {
      const derived = await APIClient.deriveSystem({ base_id: base.id, instruction });
      setShowBaseCatalogue(false);
      handleDraftProduced(derived);
      setToast({
        type: 'success',
        message: `Derived "${derived.name}" from ${base.name}. Review and save.`,
      });
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to derive a system' });
    } finally {
      setIsDeriving(false);
    }
  };

  const handleExplain = async () => {
    if (!savedID) {
      setToast({ type: 'error', message: 'Save the system before explaining it.' });
      return;
    }
    setIsExplaining(true);
    try {
      const resp = await APIClient.explainSystem(savedID);
      setExplanation(resp.explanation);
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to explain the system' });
    } finally {
      setIsExplaining(false);
    }
  };

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

  // loadSystems is recreated every render, because it closes over handleNewSystem
  // and loadSystemDetail, which close over the loaders in turn. Listing it in the
  // mount effect below would re-run that effect on every render and set state in a
  // loop, so the call goes through a ref holding the latest function. The chain is
  // memoised when this component is split (FE-4), and this indirection goes with it.
  const loadSystemsRef = useRef<((selectID?: string, mode?: 'new' | 'browse') => Promise<void>) | null>(null);
  useEffect(() => {
    loadSystemsRef.current = loadSystems;
  });

  useEffect(() => {
    void loadSystemsRef.current?.(undefined, startModeRef.current);
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
      setVerificationResult(null);
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
    setVerificationResult(null);
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

  // handleStartFromBase seeds a new unsaved system from a reference base. It never
  // touches the open system: starting from a base is how a new system begins, not
  // how the current one is replaced.
  const handleStartFromBase = (base: ReferenceSystem) => {
    setShowBaseCatalogue(false);
    setSelection({ kind: 'draft' });
    setActiveDraftID(null);
    setDraft({ localId: crypto.randomUUID(), dirty: true });
    setName(`${base.name} (copy)`);
    setSlugID(`${base.id}-copy`);
    setVersion(base.version);
    setDescription(base.description);
    setRulesPrompt(base.rules_prompt);
    setScript(base.script);
    setMechanics(base.mechanics ?? {});
    setCreationPreamble('');
    setCreationFields([]);
    setVerificationResult(null);
    setDraftNotes([]);
    setActiveTab('manifest');
    setToast({ type: 'success', message: `Started a new system from ${base.name}.` });
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
    setVerificationResult(null);
    markDirty();
    setToast({ type: 'success', message: `Loaded the ${reference.name} reference.` });
  };

  const handleResetToReference = () => {
    const reference = referenceSystems.find((r) => r.id === 'narrative_2d6') ?? referenceSystems[0];
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
      if (activeDraftID) {
        try {
          await APIClient.discardSystemDraft(activeDraftID);
        } catch {
          // ignore draft cleanup failure
        }
        setActiveDraftID(null);
      }
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

  // The tests tab reads the scenarios a system stores under systems/<id>/tests/,
  // and is the only place they are created or removed.
  useEffect(() => {
    if (activeTab !== 'tests' || !savedID) return;
    let cancelled = false;
    setIsLoadingScenarios(true);
    APIClient.listSystemScenarios(savedID)
      .then((res) => {
        if (!cancelled) setScenarios(res.scenarios ?? []);
      })
      .catch((err) => {
        if (!cancelled) setScenarioError(errorMessage(err));
      })
      .finally(() => {
        if (!cancelled) setIsLoadingScenarios(false);
      });
    return () => {
      cancelled = true;
    };
  }, [activeTab, savedID]);

  const handleSaveScenario = async (scenario: Scenario) => {
    if (!savedID) return;
    setScenarioError(null);
    try {
      const res = await APIClient.saveSystemScenario(savedID, scenario);
      setScenarios(res.scenarios ?? []);
      setEditingScenario(null);
      setToast({ type: 'success', message: `Saved scenario "${scenario.name}".` });
    } catch (err) {
      setScenarioError(errorMessage(err));
    }
  };

  const handleDeleteScenario = async (name: string) => {
    if (!savedID) return;
    setScenarioError(null);
    try {
      await APIClient.deleteSystemScenario(savedID, name);
      setScenarios((prev) => prev.filter((scenario) => scenario.name !== name));
    } catch (err) {
      setScenarioError(errorMessage(err));
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
      <SystemList
        systems={systems}
        selection={selection}
        draft={draft}
        name={name}
        description={description}
        referenceSystems={referenceSystems}
        isLoading={isLoading}
        isDraft={isDraft}
        importInputRef={importInputRef}
        handleFileSelect={handleFileSelect}
        handleNewSystem={handleNewSystem}
        requestSelection={requestSelection}
        setPendingSelection={setPendingSelection}
        setShowGenerateDialog={setShowGenerateDialog}
        setShowBaseCatalogue={setShowBaseCatalogue}
        setDeleteTarget={setDeleteTarget}
        setDeleteSystemError={setDeleteSystemError}
        setCanForceDelete={setCanForceDelete}
      />

      {/* Right Detail Column: Editor */}
      <section className="flex-1 h-full min-w-0 bg-stone-900/30 p-6 flex flex-col gap-4 overflow-hidden min-h-0">
        {/* Top Header & Sub-Tabs */}
        <SystemHeader
          savedID={savedID}
          name={name}
          slugID={slugID}
          activeTab={activeTab}
          setActiveTab={setActiveTab}
          isSaving={isSaving}
          isTesting={isTesting}
          isGeneratingAll={isGeneratingAll}
          isExplaining={isExplaining}
          verificationResult={verificationResult}
          handleSave={handleSave}
          handleRunTests={handleRunTests}
          handleExport={handleExport}
          handleExplain={handleExplain}
          handleResetToReference={handleResetToReference}
          handleGenerateAllSystemFields={handleGenerateAllSystemFields}
          setShowEnhanceDialog={setShowEnhanceDialog}
          setDeleteTarget={setDeleteTarget}
          setDeleteSystemError={setDeleteSystemError}
          setCanForceDelete={setCanForceDelete}
        />

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

        {verificationResult && (
          <div
            data-testid="verification-banner"
            className={`p-3 rounded-xl text-xs flex items-center justify-between gap-3 shrink-0 ${
              verificationResult.ok
                ? 'bg-emerald-950/40 border border-emerald-500/40 text-emerald-200'
                : 'bg-red-950/40 border border-red-500/40 text-red-200'
            }`}
          >
            <div className="flex items-start gap-2 min-w-0">
              {verificationResult.ok ? (
                <Check className="w-4 h-4 text-emerald-400 shrink-0 mt-0.5" />
              ) : (
                <AlertCircle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
              )}
              <div className="space-y-0.5 min-w-0">
                <div className="font-semibold">
                  {verificationResult.ok ? 'System verification passed' : 'System verification reported failures'}
                </div>
                {verificationResult.failures && verificationResult.failures.length > 0 && (
                  <ul className="text-[11px] text-red-300 font-mono list-disc list-inside space-y-0.5">
                    {verificationResult.failures.map((f, i) => (
                      <li key={i}>{f}</li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
            {!verificationResult.ok && (
              <button
                type="button"
                onClick={() => setShowGenerateDialog(true)}
                className="px-2.5 py-1 text-xs font-sans font-medium rounded-lg bg-red-500/20 hover:bg-red-500/30 text-red-200 border border-red-500/40 transition-colors cursor-pointer shrink-0"
              >
                Regenerate
              </button>
            )}
          </div>
        )}

        {draftNotes.length > 0 && (
          <div
            data-testid="draft-notes"
            className="p-3 rounded-xl text-xs flex items-start gap-2 bg-amber-950/40 border border-amber-500/40 text-amber-200 shrink-0"
          >
            <Info className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
            <div className="space-y-0.5 min-w-0">
              <div className="font-semibold">Generation notes</div>
              <ul className="text-[11px] list-disc list-inside space-y-0.5">
                {draftNotes.map((note, i) => (
                  <li key={i}>{note}</li>
                ))}
              </ul>
            </div>
          </div>
        )}

        {/* Tab 1: Manifest Form */}
        {activeTab === 'manifest' && (
          <SystemManifestTab
            name={name}
            slugID={slugID}
            version={version}
            description={description}
            creationPreamble={creationPreamble}
            creationFields={creationFields}
            setName={setName}
            setSlugID={setSlugID}
            setVersion={setVersion}
            setDescription={setDescription}
            setCreationPreamble={setCreationPreamble}
            setCreationFields={setCreationFields}
            savedID={savedID}
            markDirty={markDirty}
            reportGenerationError={reportGenerationError}
            getSystemContext={getSystemContext}
            setDeleteTarget={setDeleteTarget}
            setDeleteSystemError={setDeleteSystemError}
            setCanForceDelete={setCanForceDelete}
          />
        )}

        {/* Tab 2: Agent Rules Prompt Editor */}
        {activeTab === 'rules' && (
          <SystemRulesTab
            rulesPrompt={rulesPrompt}
            setRulesPrompt={setRulesPrompt}
            markDirty={markDirty}
            slugID={slugID}
            savedID={savedID}
            reportGenerationError={reportGenerationError}
            getSystemContext={getSystemContext}
          />
        )}

        {/* Tab 2b: Mechanics Editor */}
        {activeTab === 'mechanics' && (
          <SystemMechanicsTab mechanics={mechanics} setMechanics={setMechanics} markDirty={markDirty} />
        )}

        {/* Tab 3: Script Editor */}
        {activeTab === 'script' && (
          <SystemScriptTab
            slugID={slugID}
            savedID={savedID}
            script={script}
            setScript={setScript}
            markDirty={markDirty}
          />
        )}

        {/* Tab 5: Tests */}
        {activeTab === 'tests' && (
          <SystemTestsTab
            savedID={savedID}
            scenarios={scenarios}
            scenarioError={scenarioError}
            isLoadingScenarios={isLoadingScenarios}
            editingScenario={editingScenario}
            setEditingScenario={setEditingScenario}
            handleSaveScenario={handleSaveScenario}
            handleDeleteScenario={handleDeleteScenario}
          />
        )}
      </section>

      {/* Discard Draft Confirmation Dialog */}
      <DiscardDraftConfirm
        isOpen={pendingSelection !== null}
        title="Discard unsaved system?"
        description="This system has not been saved. Leaving now discards every field you entered."
        onCancel={() => setPendingSelection(null)}
        onDiscard={() => {
          if (activeDraftID) {
            APIClient.discardSystemDraft(activeDraftID).catch(() => {});
            setActiveDraftID(null);
          }
          applySelection(pendingSelection);
        }}
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
        <Modal
          open
          onClose={() => {
            setDeleteTarget(null);
            setDeleteSystemError(null);
            setCanForceDelete(false);
          }}
          title={`Delete system "${deleteTarget.name}"?`}
          dismissable={!isDeletingSystem}
          className="max-w-md"
        >
          <div className="space-y-4">
            <p className="flex items-center gap-2 text-xs font-sans text-stone-300">
              <Trash2 className="w-4 h-4 shrink-0 text-red-400" aria-hidden="true" />
              This will permanently delete the system directory and all of its mechanics and prompts. This action cannot
              be undone.
            </p>
            {deleteSystemError && (
              <div className="p-3 bg-red-950/40 border border-red-500/30 rounded-xl text-xs text-red-300 flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
                <div className="space-y-1">
                  <p>{deleteSystemError}</p>
                  {canForceDelete && (
                    <p className="text-stone-400">
                      You can force deletion to remove the system anyway. Existing campaigns or worlds using this system
                      may fail to resolve mechanics.
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
        </Modal>
      )}

      {showBaseCatalogue && (
        <Modal open onClose={() => setShowBaseCatalogue(false)} title="Start from a base system" className="max-w-2xl">
          <p className="mb-4 text-xs text-neutral-400">
            Clone a tested reference to edit by hand, or derive a variant from it.
          </p>
          <div className="max-h-[65vh] overflow-y-auto">
            <BaseSystemCatalogue
              bases={referenceSystems}
              busy={isDeriving}
              onClone={(base) => {
                const reference = referenceSystems.find((r) => r.id === base.id);
                if (reference) handleStartFromBase(reference);
              }}
              onDerive={handleDerive}
            />
          </div>
        </Modal>
      )}

      {showGenerateDialog && (
        <SystemGenerateDialog onCancel={() => setShowGenerateDialog(false)} onDraft={handleDraftProduced} />
      )}

      {showEnhanceDialog && savedID && (
        <SystemEnhanceDialog
          systemId={savedID}
          onClose={() => setShowEnhanceDialog(false)}
          onApplied={async (result) => {
            setToast({
              type: 'success',
              message: result.written.length > 0 ? 'System enhanced.' : 'No changes applied.',
            });
            await loadSystemDetail(savedID);
          }}
        />
      )}

      {explanation !== null && (
        <Modal open onClose={() => setExplanation(null)} title="How this system works" className="max-w-2xl">
          <div className="max-h-[65vh] overflow-y-auto">
            <MarkdownDocViewer content={explanation} />
          </div>
        </Modal>
      )}
    </div>
  );
};
