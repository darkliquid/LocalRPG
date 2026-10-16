import { errorMessage } from '../lib/errors';
import { useFailures } from '../context/FailuresContext';
import React, { useState, useEffect, useRef } from 'react';
import { APIClient, WorldExistsError } from '../api/client';
import {
  WorldInfo,
  SystemInfo,
  WorldEntitySummary,
  CreateWorldRequest,
  WorldSelection,
  WorldDraft,
  WorldDetail,
  GenerationFailure,
  FolderNode,
  ContentManifestInfo,
  DraftCommitRequest,
  WorldDraftInfo,
} from '../types';
import { AlertCircle, Trash2 } from 'lucide-react';
import { formatGenerationError } from '../lib/generationError';
import { useLightbox } from '../hooks/useLightbox';
import { ImageLightbox } from './ImageLightbox';
import { DiscardDraftConfirm } from './launcher/DiscardDraftConfirm';
import { REFERENCE_WORLD_TEMPLATE } from '../templates/referenceTemplates';
import { NewEntityWizard } from './NewEntityWizard';
import { ContentImportDialog } from './ContentImportDialog';
import { inspectPackageFile } from '../lib/packageInspect';
import { useSaveFilePicker } from '../hooks/useSaveFilePicker';
import { WorldGenerateDialog } from './WorldGenerateDialog';
import { WorldDraftReview } from './WorldDraftReview';
import { Modal } from './Modal';
import { WorldList } from './studios/world/WorldList';
import { WorldHeader } from './studios/world/WorldHeader';
import { WorldReport } from './studios/world/WorldReport';
import { WorldPromptTab } from './studios/world/WorldPromptTab';
import { WorldEntitiesTab } from './studios/world/WorldEntitiesTab';
import { WorldLoreTab } from './studios/world/WorldLoreTab';
import { EntityBatchDialog } from './EntityBatchDialog';
import { WorldEnhanceDialog } from './WorldEnhanceDialog';

interface WorldsStudioProps {
  onWorldSaved?: () => void;
  startMode?: 'new' | 'browse';
}

// typeFromMarkdown reads the type the wizard wrote, so the tree summary matches
// the note without a round trip.
function typeFromMarkdown(markdown: string): string {
  const match = markdown.match(/^type:\s*"?([a-z0-9-]+)"?/m);
  return match ? match[1] : 'concept';
}

export const WorldsStudio: React.FC<WorldsStudioProps> = ({ onWorldSaved, startMode = 'browse' }) => {
  const { report } = useFailures();
  const [worlds, setWorlds] = useState<WorldInfo[]>([]);
  const [systems, setSystems] = useState<SystemInfo[]>([]);
  const [selection, setSelection] = useState<WorldSelection>(null);
  const [draft, setDraft] = useState<WorldDraft | null>(null);
  const [pendingSelection, setPendingSelection] = useState<WorldSelection>(null);
  const [slugError, setSlugError] = useState(false);
  const [activeTab, setActiveTab] = useState<'lore' | 'prompt' | 'entities'>('lore');

  // World form state (defaults to blank slate)
  const [name, setName] = useState('');
  const [slugID, setSlugID] = useState('');
  const [genre, setGenre] = useState('');
  const [defaultSystem, setDefaultSystem] = useState('');
  const [artStyle, setArtStyle] = useState('');
  const [tags, setTags] = useState('');
  const [description, setDescription] = useState('');
  const [lorePrompt, setLorePrompt] = useState('');

  // Entities state
  const [entities, setEntities] = useState<WorldEntitySummary[]>([]);
  const [worldFolders, setWorldFolders] = useState<FolderNode[]>([]);
  const [selectedEntityID, setSelectedEntityID] = useState<string | null>(null);
  const [entityMarkdown, setEntityMarkdown] = useState('');
  const [entityDrafts, setEntityDrafts] = useState<Record<string, string>>({});
  const [isNewEntityModal, setIsNewEntityModal] = useState(false);

  const [isLoading, setIsLoading] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [isGeneratingAll, setIsGeneratingAll] = useState(false);
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // World artwork (banner & icon) staged for upload on save, or previewed for
  // an unsaved world.
  const [bannerFile, setBannerFile] = useState<File | null>(null);
  const [iconFile, setIconFile] = useState<File | null>(null);
  const [bannerPreview, setBannerPreview] = useState<string | null>(null);
  const [iconPreview, setIconPreview] = useState<string | null>(null);
  const [generatingKind, setGeneratingKind] = useState<'banner' | 'icon' | null>(null);
  const { lightbox, isLightboxOpen, openLightbox, closeLightbox } = useLightbox();

  const bannerInputRef = React.useRef<HTMLInputElement>(null);
  const iconInputRef = React.useRef<HTMLInputElement>(null);
  const importInputRef = useRef<HTMLInputElement>(null);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importManifest, setImportManifest] = useState<ContentManifestInfo | null>(null);
  const [isImporting, setIsImporting] = useState(false);
  const savePicker = useSaveFilePicker();
  // AI generation: a whole-world brief, a draft awaiting review, a batch of
  // entities, and an enhancement diff. Every one of them reviews before it writes.
  const [showGenerate, setShowGenerate] = useState(false);
  const [reviewDraft, setReviewDraft] = useState<WorldDraftInfo | null>(null);
  const [reviewTarget, setReviewTarget] = useState<string | undefined>(undefined);
  const [reviewLoading, setReviewLoading] = useState(false);
  const [reviewError, setReviewError] = useState<string | null>(null);
  const [batchWorldId, setBatchWorldId] = useState<string | null>(null);
  const [enhanceWorldId, setEnhanceWorldId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null);
  const [isDeletingWorld, setIsDeletingWorld] = useState(false);
  const [deleteWorldError, setDeleteWorldError] = useState<string | null>(null);
  const [canForceDelete, setCanForceDelete] = useState(false);
  const detailRequest = React.useRef(0);
  const startModeRef = React.useRef(startMode);

  const isDraft = selection?.kind === 'draft';
  const savedID = selection?.kind === 'saved' ? selection.id : null;
  const markDirty = () => setDraft((d) => (d ? { ...d, dirty: true } : d));
  const reportGenerationError = (failure: GenerationFailure) =>
    setToast({ type: 'error', message: formatGenerationError(failure) });

  // commitReviewedDraft writes the accepted set and opens the world it produced.
  const commitReviewedDraft = async (req: DraftCommitRequest) => {
    setReviewLoading(true);
    setReviewError(null);
    try {
      const world = await APIClient.commitDraft(req);
      setReviewDraft(null);
      setReviewTarget(undefined);
      await loadWorldsRef.current?.(world.id, 'browse');
    } catch (err) {
      setReviewError(errorMessage(err));
    } finally {
      setReviewLoading(false);
    }
  };

  const discardReviewedDraft = async (draftId: string) => {
    try {
      await APIClient.discardDraft({ draft_id: draftId });
    } catch {
      // A draft that is already gone is not a failure worth reporting.
    }
    setReviewDraft(null);
    setReviewTarget(undefined);
  };

  const handleDeleteWorld = async (force = false) => {
    if (!deleteTarget) return;
    setIsDeletingWorld(true);
    setDeleteWorldError(null);
    try {
      await APIClient.deleteWorld(deleteTarget.id, force);
      const deletedName = deleteTarget.name;
      const deletedID = deleteTarget.id;
      setDeleteTarget(null);
      setDeleteWorldError(null);
      setCanForceDelete(false);
      setToast({ type: 'success', message: `World "${deletedName}" deleted.` });

      if (selection?.kind === 'saved' && selection.id === deletedID) {
        requestSelection({ kind: 'draft' });
      }
      await loadWorldsRef.current?.(undefined, 'browse');
    } catch (err) {
      const msg = errorMessage(err);
      setDeleteWorldError(msg);
      if (msg.includes('in use by a campaign') || msg.toLowerCase().includes('conflict')) {
        setCanForceDelete(true);
      }
    } finally {
      setIsDeletingWorld(false);
    }
  };

  const loadWorldsRef = React.useRef<((selectID?: string, mode?: 'new' | 'browse') => Promise<void>) | null>(null);

  useEffect(() => {
    loadWorldsRef.current?.(undefined, startModeRef.current);
  }, []);

  // A world's template folders are loaded once the world is saved, because a
  // draft has no directory to hold them yet.
  useEffect(() => {
    if (!savedID) {
      setWorldFolders([]);
      return;
    }
    let cancelled = false;
    APIClient.listWorldFolders(savedID)
      .then((tree) => {
        if (!cancelled) setWorldFolders(tree);
      })
      .catch(() => {
        if (!cancelled) setWorldFolders([]);
      });
    return () => {
      cancelled = true;
    };
  }, [savedID]);

  const refreshWorldFolders = async () => {
    if (!savedID) return;
    setWorldFolders(await APIClient.listWorldFolders(savedID));
  };

  const loadWorlds = async (selectID?: string, mode: 'new' | 'browse' = startMode) => {
    setIsLoading(true);
    try {
      const [wList, sList] = await Promise.all([APIClient.listWorlds(), APIClient.listSystems()]);
      setWorlds(wList);
      setSystems(sList);
      if (mode === 'new') {
        handleNewWorld(sList);
        return;
      }
      const target = selectID || (wList.length > 0 ? wList[0].id : null);
      if (target) {
        loadWorldDetail(target);
      } else {
        handleNewWorld(sList);
      }
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to load worlds' });
    } finally {
      setIsLoading(false);
    }
  };

  loadWorldsRef.current = loadWorlds;

  const loadWorldDetail = async (id: string) => {
    const token = ++detailRequest.current;
    try {
      const detail = await APIClient.getWorld(id);
      if (token !== detailRequest.current) return;
      setSelection({ kind: 'saved', id: detail.id });
      setDraft(null);
      setName(detail.name);
      setSlugID(detail.id);
      setGenre(detail.genre || '');
      setDefaultSystem(detail.default_system || (systems[0]?.id ?? ''));
      setArtStyle(detail.art_style || '');
      setTags(detail.tags ? detail.tags.join(', ') : '');
      setDescription(detail.description || '');
      setLorePrompt(detail.lore_prompt || '');
      setEntities(detail.entities || []);

      setBannerFile(null);
      setIconFile(null);
      setBannerPreview(`/api/world/${encodeURIComponent(id)}/banner?t=${Date.now()}`);
      setIconPreview(`/api/world/${encodeURIComponent(id)}/icon?t=${Date.now()}`);

      if (detail.entities && detail.entities.length > 0) {
        const first = detail.entities[0];
        setSelectedEntityID(first.id);
        try {
          const ent = await APIClient.getWorldEntity(detail.id, first.id);
          if (token !== detailRequest.current) return;
          setEntityMarkdown(ent.markdown);
          setEntityDrafts({ [first.id]: ent.markdown });
        } catch {
          setEntityMarkdown('');
          setEntityDrafts({});
        }
      } else {
        setSelectedEntityID(null);
        setEntityMarkdown('');
        setEntityDrafts({});
      }
    } catch (err) {
      if (token !== detailRequest.current) return;
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to load world details' });
    }
  };

  const handleSelectEntity = async (targetId: string) => {
    if (targetId === selectedEntityID) return;

    // Flush current editor content to drafts
    const updatedDrafts = { ...entityDrafts };
    if (selectedEntityID) {
      updatedDrafts[selectedEntityID] = entityMarkdown;
      setEntityDrafts(updatedDrafts);
    }

    setSelectedEntityID(targetId);

    // If already in drafts, load it
    if (updatedDrafts[targetId] !== undefined) {
      setEntityMarkdown(updatedDrafts[targetId]);
      return;
    }

    // Otherwise, if world is saved, fetch from API
    if (savedID) {
      try {
        const ent = await APIClient.getWorldEntity(savedID, targetId);
        setEntityDrafts((prev) => ({ ...prev, [targetId]: ent.markdown }));
        setEntityMarkdown(ent.markdown);
      } catch (err) {
        setToast({ type: 'error', message: errorMessage(err) || 'Failed to load entity markdown' });
      }
    } else {
      setEntityMarkdown('');
    }
  };

  const handleNewWorld = (sysList?: SystemInfo[]) => {
    detailRequest.current += 1;
    setSelection({ kind: 'draft' });
    setDraft((prev) => ({ localId: prev?.localId ?? crypto.randomUUID(), dirty: false }));
    setSlugError(false);
    setName('');
    setSlugID('');
    setGenre('');
    const availableSys = sysList && sysList.length > 0 ? sysList : systems;
    setDefaultSystem(availableSys[0]?.id ?? '');
    setArtStyle('');
    setTags('');
    setDescription('');
    setLorePrompt('');
    setEntities([]);
    setSelectedEntityID(null);
    setEntityMarkdown('');
    setEntityDrafts({});
    setBannerFile(null);
    setIconFile(null);
    setBannerPreview(null);
    setIconPreview(null);
    setActiveTab('lore');
  };

  const applySelection = (target: WorldSelection) => {
    setPendingSelection(null);
    if (!target || target.kind === 'draft') {
      handleNewWorld();
      return;
    }
    loadWorldDetail(target.id);
  };

  const requestSelection = (target: WorldSelection) => {
    // Re-selecting the open draft is a no-op; only leaving it can discard work.
    if (target?.kind === 'draft' && isDraft) return;
    if (isDraft && draft?.dirty) {
      setPendingSelection(target);
      return;
    }
    applySelection(target);
  };

  const handleLoadReferenceTemplate = () => {
    markDirty();
    setName(REFERENCE_WORLD_TEMPLATE.name);
    if (isDraft) {
      setSlugID(REFERENCE_WORLD_TEMPLATE.id);
    }
    setGenre(REFERENCE_WORLD_TEMPLATE.genre);
    const matchingSys = systems.find((s) => s.id === REFERENCE_WORLD_TEMPLATE.default_system);
    if (matchingSys) setDefaultSystem(matchingSys.id);
    setArtStyle(REFERENCE_WORLD_TEMPLATE.art_style);
    setTags(REFERENCE_WORLD_TEMPLATE.tags.join(', '));
    setDescription(REFERENCE_WORLD_TEMPLATE.description);
    setLorePrompt(REFERENCE_WORLD_TEMPLATE.lore_prompt);
    setEntities(REFERENCE_WORLD_TEMPLATE.entities.map((e) => ({ id: e.id, name: e.name, type: e.type })));
    setSelectedEntityID(REFERENCE_WORLD_TEMPLATE.entities[0].id);
    setEntityMarkdown(REFERENCE_WORLD_TEMPLATE.entities[0].markdown);
    const initialDrafts: Record<string, string> = {};
    REFERENCE_WORLD_TEMPLATE.entities.forEach((e) => {
      initialDrafts[e.id] = e.markdown;
    });
    setEntityDrafts(initialDrafts);
    setToast({ type: 'success', message: 'Loaded The Ashen Reach reference template' });
  };

  const handleAIGenerate = async (kind: 'banner' | 'icon') => {
    setGeneratingKind(kind);
    try {
      if (savedID) {
        await APIClient.generateWorldAsset(savedID, kind);
        if (kind === 'banner') {
          setBannerPreview(`/api/world/${encodeURIComponent(savedID)}/banner?t=${Date.now()}`);
        } else {
          setIconPreview(`/api/world/${encodeURIComponent(savedID)}/icon?t=${Date.now()}`);
        }
        setToast({ type: 'success', message: `Generated world ${kind}!` });
      } else {
        const blob = await APIClient.generateAssetPreview(
          kind,
          name.trim() || 'New World',
          description.trim(),
          artStyle.trim(),
          genre.trim(),
        );
        const file = new File([blob], `${kind}.png`, { type: blob.type });
        if (kind === 'banner') {
          setBannerFile(file);
          setBannerPreview(URL.createObjectURL(blob));
        } else {
          setIconFile(file);
          setIconPreview(URL.createObjectURL(blob));
        }
        setToast({ type: 'success', message: `Previewed world ${kind}!` });
      }
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || `Failed to generate ${kind}` });
    } finally {
      setGeneratingKind(null);
    }
  };

  const getWorldContext = (): Record<string, string> => ({
    name,
    genre,
    art_style: artStyle,
    description,
    lore_prompt: lorePrompt,
    tags,
  });

  const handleGenerateAllWorldFields = async () => {
    if (isGeneratingAll) return;
    setIsGeneratingAll(true);
    try {
      const res = await APIClient.generateText({
        form_type: 'world',
        field_name: '_all',
        context: getWorldContext(),
        system_id: defaultSystem,
      });
      if (res.fields.name && !name.trim()) setName(res.fields.name);
      if (res.fields.genre && !genre.trim()) setGenre(res.fields.genre);
      if (res.fields.art_style && !artStyle.trim()) setArtStyle(res.fields.art_style);
      if (res.fields.description && !description.trim()) setDescription(res.fields.description);
      if (res.fields.lore_prompt && !lorePrompt.trim()) setLorePrompt(res.fields.lore_prompt);
      if (res.warning) {
        setToast({ type: 'error', message: `Partial: ${res.warning.code} — ${res.warning.message}` });
      } else if (Object.keys(res.fields).length === 0) {
        setToast({ type: 'error', message: 'The model returned nothing to fill.' });
      } else {
        markDirty();
        setToast({ type: 'success', message: 'Auto-filled world fields!' });
      }
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Auto-fill failed' });
    } finally {
      setIsGeneratingAll(false);
    }
  };

  const handleSaveWorld = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setToast({ type: 'error', message: 'World Name is required' });
      return;
    }

    setIsSaving(true);
    setToast(null);
    try {
      const parsedTags = tags
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean);

      const payload: CreateWorldRequest = {
        id: isDraft ? slugID.trim() || undefined : undefined,
        name: name.trim(),
        description: description.trim(),
        genre: genre.trim(),
        default_system: defaultSystem || (systems[0]?.id ?? ''),
        art_style: artStyle.trim(),
        tags: parsedTags,
        lore_prompt: lorePrompt.trim(),
      };

      if (!isDraft && !savedID) return;
      const saved: WorldDetail = isDraft
        ? await APIClient.createWorld(payload)
        : await APIClient.updateWorld(savedID as string, payload);

      // For a newly created draft world, persist the entity templates that were drafted in memory.
      if (isDraft) {
        const allDrafts = { ...entityDrafts };
        if (selectedEntityID) {
          allDrafts[selectedEntityID] = entityMarkdown;
        }

        for (const ent of entities) {
          const md = allDrafts[ent.id];
          if (md) {
            await APIClient.saveWorldEntity(saved.id, ent.id, md, ent.folder).catch(() => {});
          }
        }
      } else if (selectedEntityID && entityDrafts[selectedEntityID] !== undefined) {
        // For an existing world, only save the currently selected entity if it has unpersisted edits.
        const currentEntity = entities.find((e) => e.id === selectedEntityID);
        await APIClient.saveWorldEntity(saved.id, selectedEntityID, entityMarkdown, currentEntity?.folder).catch(
          () => {},
        );
      }

      if (bannerFile) {
        await APIClient.uploadWorldAsset(saved.id, 'banner', bannerFile).catch((err) =>
          report({
            text: 'The world was saved, but its banner could not be uploaded.',
            detail: err instanceof Error ? err.message : String(err),
          }),
        );
      }
      if (iconFile) {
        await APIClient.uploadWorldAsset(saved.id, 'icon', iconFile).catch((err) =>
          report({
            text: 'The world was saved, but its icon could not be uploaded.',
            detail: err instanceof Error ? err.message : String(err),
          }),
        );
      }

      setToast({ type: 'success', message: `World "${saved.name}" saved successfully!` });
      await loadWorlds(saved.id, 'browse');
      if (onWorldSaved) onWorldSaved();
    } catch (err) {
      if (err instanceof WorldExistsError) {
        setSlugError(true);
        setToast({ type: 'error', message: 'A world with this id already exists. Change the name or slug.' });
      } else {
        setToast({ type: 'error', message: errorMessage(err) || 'Failed to save world' });
      }
    } finally {
      setIsSaving(false);
    }
  };

  const handleSaveEntity = async () => {
    if (!selectedEntityID) return;

    setEntityDrafts((prev) => ({ ...prev, [selectedEntityID]: entityMarkdown }));

    if (isDraft) {
      markDirty();
      setToast({
        type: 'success',
        message: `Draft entity "${selectedEntityID}" updated! (Will be persisted when you click Save World)`,
      });
      return;
    }
    if (!savedID) return;

    try {
      const currentEntity = entities.find((e) => e.id === selectedEntityID);
      await APIClient.saveWorldEntity(savedID, selectedEntityID, entityMarkdown, currentEntity?.folder);
      setToast({ type: 'success', message: `Entity "${selectedEntityID}" saved!` });
      await loadWorldDetail(savedID);
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to save entity' });
    }
  };

  const handleDeleteEntity = async (entityId: string) => {
    if (isDraft) {
      const remaining = entities.filter((e) => e.id !== entityId);
      setEntities(remaining);
      const updatedDrafts = { ...entityDrafts };
      delete updatedDrafts[entityId];
      setEntityDrafts(updatedDrafts);

      if (selectedEntityID === entityId) {
        const next = remaining[0];
        setSelectedEntityID(next ? next.id : null);
        setEntityMarkdown(next ? updatedDrafts[next.id] || '' : '');
      }
      setToast({ type: 'success', message: `Entity "${entityId}" removed from draft.` });
      markDirty();
      return;
    }
    if (!savedID) return;

    try {
      await APIClient.deleteWorldEntity(savedID, entityId);
      setToast({ type: 'success', message: `Entity "${entityId}" deleted!` });
      await loadWorldDetail(savedID);
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to delete entity' });
    }
  };

  const handleCreateNewEntity = async (input: { id: string; name: string; markdown: string }) => {
    const { id: slug, markdown } = input;

    if (isDraft) {
      if (entities.some((e) => e.id === slug)) {
        setToast({ type: 'error', message: `Entity "${slug}" already exists` });
        return;
      }
      const newSummary: WorldEntitySummary = { id: slug, name: input.name, type: typeFromMarkdown(markdown) };
      setEntities((prev) => [...prev, newSummary]);
      setEntityDrafts((prev) => ({
        ...prev,
        ...(selectedEntityID ? { [selectedEntityID]: entityMarkdown } : {}),
        [slug]: markdown,
      }));
      setSelectedEntityID(slug);
      setEntityMarkdown(markdown);
      setIsNewEntityModal(false);
      markDirty();
      return;
    }
    if (!savedID) return;

    try {
      await APIClient.saveWorldEntity(savedID, slug, markdown);
      setIsNewEntityModal(false);
      await loadWorldDetail(savedID);
      await handleSelectEntity(slug);
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to create entity' });
    }
  };

  const handleExport = async () => {
    if (!savedID) return;
    const defaultFilename = `${savedID}-1.0.0.lrpgworld`;
    try {
      if (savePicker.nativeDialog) {
        const path = await savePicker.pick({
          title: 'Export World Package',
          default_filename: defaultFilename,
          filters: [
            { display_name: 'LocalRPG World (*.lrpgworld)', pattern: '*.lrpgworld' },
            { display_name: 'All Files (*.*)', pattern: '*.*' },
          ],
        });
        if (!path) return;
        await APIClient.exportContent('world', savedID, path);
        setToast({ type: 'success', message: `Exported world package: ${path}` });
        return;
      }

      const blob = (await APIClient.exportContent('world', savedID)) as Blob;
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = defaultFilename;
      a.click();
      URL.revokeObjectURL(url);
      setToast({ type: 'success', message: `Exported world package: ${defaultFilename}` });
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
      const res = await APIClient.importContent(importFile, conflictMode, 'world');
      setToast({ type: 'success', message: `Successfully ${res.action} world ${res.name} (${res.id})` });
      setImportFile(null);
      setImportManifest(null);
      await loadWorlds(res.id, 'browse');
      if (onWorldSaved) onWorldSaved();
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) });
    } finally {
      setIsImporting(false);
    }
  };

  return (
    <div className="w-full h-full flex flex-col md:flex-row overflow-hidden">
      {/* Left Master Column: Worlds List */}
      <WorldList
        worlds={worlds}
        selection={selection}
        draft={draft}
        name={name}
        isLoading={isLoading}
        importInputRef={importInputRef}
        handleFileSelect={handleFileSelect}
        requestSelection={requestSelection}
        setShowGenerate={setShowGenerate}
        setDeleteTarget={setDeleteTarget}
        setDeleteWorldError={setDeleteWorldError}
        setCanForceDelete={setCanForceDelete}
      />

      {/* Right Detail Column: Editor */}
      <section className="flex-1 h-full min-w-0 bg-stone-900/30 p-6 flex flex-col gap-4 overflow-hidden min-h-0">
        {/* Top Header & Sub-Tabs */}
        <WorldHeader
          savedID={savedID}
          name={name}
          slugID={slugID}
          selection={selection}
          entities={entities}
          activeTab={activeTab}
          setActiveTab={setActiveTab}
          isSaving={isSaving}
          isGeneratingAll={isGeneratingAll}
          handleSaveWorld={handleSaveWorld}
          handleExport={handleExport}
          handleLoadReferenceTemplate={handleLoadReferenceTemplate}
          handleGenerateAllWorldFields={handleGenerateAllWorldFields}
          setEnhanceWorldId={setEnhanceWorldId}
          setBatchWorldId={setBatchWorldId}
          setDeleteTarget={setDeleteTarget}
          setDeleteWorldError={setDeleteWorldError}
          setCanForceDelete={setCanForceDelete}
        />

        {/* Toast Feedback */}
        <WorldReport toast={toast} />
        {/* Tab 1: Lore & Atmosphere Form */}
        {activeTab === 'lore' && (
          <WorldLoreTab
            name={name}
            slugID={slugID}
            slugError={slugError}
            genre={genre}
            artStyle={artStyle}
            description={description}
            tags={tags}
            defaultSystem={defaultSystem}
            systems={systems}
            selection={selection}
            isDraft={isDraft}
            isSaving={isSaving}
            savedID={savedID}
            generatingKind={generatingKind}
            bannerPreview={bannerPreview}
            iconPreview={iconPreview}
            bannerInputRef={bannerInputRef}
            iconInputRef={iconInputRef}
            setName={setName}
            setSlugID={setSlugID}
            setSlugError={setSlugError}
            setGenre={setGenre}
            setArtStyle={setArtStyle}
            setDescription={setDescription}
            setTags={setTags}
            setDefaultSystem={setDefaultSystem}
            setBannerFile={setBannerFile}
            setBannerPreview={setBannerPreview}
            setIconFile={setIconFile}
            setIconPreview={setIconPreview}
            markDirty={markDirty}
            reportGenerationError={reportGenerationError}
            getWorldContext={getWorldContext}
            handleAIGenerate={handleAIGenerate}
            openLightbox={openLightbox}
            setDeleteTarget={setDeleteTarget}
            setDeleteWorldError={setDeleteWorldError}
            setCanForceDelete={setCanForceDelete}
          />
        )}

        {/* Tab 2: Agent Lore Prompt Editor */}
        {activeTab === 'prompt' && (
          <WorldPromptTab
            lorePrompt={lorePrompt}
            setLorePrompt={setLorePrompt}
            defaultSystem={defaultSystem}
            markDirty={markDirty}
            slugID={slugID}
            savedID={savedID}
            reportGenerationError={reportGenerationError}
            getWorldContext={getWorldContext}
          />
        )}

        {/* Tab 3: Starter Entities Manager */}
        {activeTab === 'entities' && (
          <WorldEntitiesTab
            entities={entities}
            entityMarkdown={entityMarkdown}
            setEntityMarkdown={setEntityMarkdown}
            selectedEntityID={selectedEntityID}
            worldFolders={worldFolders}
            savedID={savedID}
            slugID={slugID}
            setIsNewEntityModal={setIsNewEntityModal}
            markDirty={markDirty}
            handleSelectEntity={handleSelectEntity}
            handleSaveEntity={handleSaveEntity}
            handleDeleteEntity={handleDeleteEntity}
            refreshWorldFolders={refreshWorldFolders}
            loadWorldDetail={loadWorldDetail}
            setToast={setToast}
          />
        )}
      </section>

      <DiscardDraftConfirm
        isOpen={pendingSelection !== null}
        onCancel={() => setPendingSelection(null)}
        onDiscard={() => applySelection(pendingSelection)}
      />

      {/* New Entity Modal */}
      <NewEntityWizard
        isOpen={isNewEntityModal}
        existingIds={entities.map((e) => e.id)}
        confirmLabel={isDraft ? 'Add template' : 'Create template'}
        onClose={() => setIsNewEntityModal(false)}
        onConfirm={handleCreateNewEntity}
      />

      {lightbox && (
        <ImageLightbox isOpen={isLightboxOpen} src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}

      {importManifest && (
        <ContentImportDialog
          manifest={importManifest}
          expectedType="world"
          onConfirm={handleConfirmImport}
          onCancel={() => {
            setImportFile(null);
            setImportManifest(null);
          }}
          loading={isImporting}
        />
      )}

      {showGenerate && (
        <WorldGenerateDialog
          onCancel={() => setShowGenerate(false)}
          onDraft={(generated) => {
            setShowGenerate(false);
            setReviewTarget(undefined);
            setReviewError(null);
            setReviewDraft(generated);
          }}
        />
      )}

      {reviewDraft && (
        <WorldDraftReview
          draft={reviewDraft}
          targetWorldId={reviewTarget}
          loading={reviewLoading}
          error={reviewError}
          onCommit={(req) => void commitReviewedDraft(req)}
          onDiscard={(draftId) => void discardReviewedDraft(draftId)}
        />
      )}

      {batchWorldId && (
        <EntityBatchDialog
          worldId={batchWorldId}
          onClose={() => setBatchWorldId(null)}
          onAccepted={(result) => {
            setBatchWorldId(null);
            setToast({
              type: 'success',
              message: `Wrote ${result.written.length} entit${result.written.length === 1 ? 'y' : 'ies'}`,
            });
            void loadWorldsRef.current?.(batchWorldId, 'browse');
          }}
        />
      )}

      {enhanceWorldId && (
        <WorldEnhanceDialog
          worldId={enhanceWorldId}
          onClose={() => setEnhanceWorldId(null)}
          onApplied={(result) => {
            setEnhanceWorldId(null);
            setToast({ type: 'success', message: `Applied ${result.written.join(', ')}` });
            void loadWorldsRef.current?.(enhanceWorldId, 'browse');
          }}
        />
      )}

      {deleteTarget && (
        <Modal
          open
          onClose={() => {
            setDeleteTarget(null);
            setDeleteWorldError(null);
            setCanForceDelete(false);
          }}
          title={`Delete world "${deleteTarget.name}"?`}
          dismissable={!isDeletingWorld}
          className="max-w-md"
        >
          <div className="space-y-4">
            <p className="flex items-center gap-2 text-xs font-sans text-stone-300">
              <Trash2 className="w-4 h-4 shrink-0 text-red-400" aria-hidden="true" />
              This will permanently delete the world directory and all of its lore and entities. This action cannot be
              undone.
            </p>
            {deleteWorldError && (
              <div className="p-3 bg-red-950/40 border border-red-500/30 rounded-xl text-xs text-red-300 flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
                <div className="space-y-1">
                  <p>{deleteWorldError}</p>
                  {canForceDelete && (
                    <p className="text-stone-400">
                      You can force deletion to remove the world anyway. Existing campaigns using this world will lose
                      their world files.
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
                  setDeleteWorldError(null);
                  setCanForceDelete(false);
                }}
                disabled={isDeletingWorld}
                className="text-xs font-sans px-3 py-1.5 rounded-lg border border-stone-700 text-stone-300 hover:text-white hover:bg-stone-800 transition-all cursor-pointer disabled:opacity-50"
              >
                Cancel
              </button>
              {canForceDelete ? (
                <button
                  type="button"
                  onClick={() => void handleDeleteWorld(true)}
                  disabled={isDeletingWorld}
                  className="text-xs font-sans font-bold px-3 py-1.5 rounded-lg bg-red-700 hover:bg-red-600 text-white transition-all cursor-pointer disabled:opacity-50 flex items-center gap-1.5"
                >
                  {isDeletingWorld ? 'Deleting...' : 'Force Delete'}
                </button>
              ) : (
                <button
                  type="button"
                  onClick={() => void handleDeleteWorld(false)}
                  disabled={isDeletingWorld}
                  className="text-xs font-sans font-bold px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white transition-all cursor-pointer disabled:opacity-50 flex items-center gap-1.5"
                >
                  {isDeletingWorld ? 'Deleting...' : 'Delete World'}
                </button>
              )}
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
};
