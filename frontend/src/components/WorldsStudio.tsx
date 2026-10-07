import React, { useState, useEffect, useRef } from 'react';
import { APIClient, WorldExistsError } from '../api/client';
import { WorldInfo, SystemInfo, WorldEntitySummary, CreateWorldRequest, WorldSelection, WorldDraft, WorldDetail, GenerationFailure, FolderNode, ContentManifestInfo } from '../types';
import { Globe, Plus, Save, Info, FileText, Check, AlertCircle, Trash2, Tag, Palette, BookOpen, Wand2, Upload, Sparkles, Download } from 'lucide-react';
import { AIGenerateButton } from './ui/AIGenerateButton';
import { formatGenerationError } from '../lib/generationError';
import { useLightbox } from '../hooks/useLightbox';
import { ImageLightbox } from './ImageLightbox';
import { DiscardDraftConfirm } from './launcher/DiscardDraftConfirm';
import { REFERENCE_WORLD_TEMPLATE } from '../templates/referenceTemplates';
import EntityTree from './EntityTree';
import MarkdownEditor from './editor/MarkdownEditor';
import { NewEntityWizard } from './NewEntityWizard';
import { safeImagePreview } from '../utils/security';
import { ContentImportDialog } from './ContentImportDialog';
import { inspectPackageFile } from '../lib/packageInspect';

interface WorldsStudioProps {
  onWorldSaved?: () => void;
  startMode?: 'new' | 'browse';
}

const STARTER_ENTITY_TEMPLATE = `---
name: New Location
type: location
state:
  danger_level: 1
wikilinks: []
---
An intriguing location waiting to be explored.
`;

// typeFromMarkdown reads the type the wizard wrote, so the tree summary matches
// the note without a round trip.
function typeFromMarkdown(markdown: string): string {
  const match = markdown.match(/^type:\s*"?([a-z0-9-]+)"?/m);
  return match ? match[1] : 'concept';
}

export const WorldsStudio: React.FC<WorldsStudioProps> = ({ onWorldSaved, startMode = 'browse' }) => {
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
  const detailRequest = React.useRef(0);
  const startModeRef = React.useRef(startMode);

  const errorMessage = (err: unknown): string =>
    err instanceof Error ? err.message : 'Unexpected error';

  const isDraft = selection?.kind === 'draft';
  const savedID = selection?.kind === 'saved' ? selection.id : null;
  const markDirty = () => setDraft((d) => (d ? { ...d, dirty: true } : d));
  const reportGenerationError = (failure: GenerationFailure) =>
    setToast({ type: 'error', message: formatGenerationError(failure) });

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
      const [wList, sList] = await Promise.all([
        APIClient.listWorlds(),
        APIClient.listSystems(),
      ]);
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
    setEntities(
      REFERENCE_WORLD_TEMPLATE.entities.map((e) => ({ id: e.id, name: e.name, type: e.type }))
    );
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
          genre.trim()
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

      // Save all entity templates (whether new world or edited world)
      const allDrafts = { ...entityDrafts };
      if (selectedEntityID) {
        allDrafts[selectedEntityID] = entityMarkdown;
      }

      for (const ent of entities) {
        const md = allDrafts[ent.id] || STARTER_ENTITY_TEMPLATE;
        await APIClient.saveWorldEntity(saved.id, ent.id, md).catch(() => {});
      }

      if (bannerFile) {
        await APIClient.uploadWorldAsset(saved.id, 'banner', bannerFile).catch(console.error);
      }
      if (iconFile) {
        await APIClient.uploadWorldAsset(saved.id, 'icon', iconFile).catch(console.error);
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
      setToast({ type: 'success', message: `Draft entity "${selectedEntityID}" updated! (Will be persisted when you click Save World)` });
      return;
    }
    if (!savedID) return;

    try {
      await APIClient.saveWorldEntity(savedID, selectedEntityID, entityMarkdown);
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
        setEntityMarkdown(next ? (updatedDrafts[next.id] || '') : '');
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
    try {
      const blob = await APIClient.exportContent('world', savedID);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${savedID}-1.0.0.lrpgpack`;
      a.click();
      URL.revokeObjectURL(url);
      setToast({ type: 'success', message: `Exported world package: ${savedID}.lrpgpack` });
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
      const res = await APIClient.importContent(importFile, conflictMode);
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
      <aside className="w-full md:w-80 h-full bg-stone-950/70 border-r border-white/10 p-4 flex flex-col gap-4 shrink-0 overflow-hidden">
        <div className="flex items-center justify-between pb-2 border-b border-stone-800/60 shrink-0">
          <div className="flex items-center gap-2">
            <Globe className="w-4 h-4 text-purple-400" />
            <h3 className="font-sans text-sm font-bold text-stone-200 uppercase tracking-wider">
              Worlds Studio
            </h3>
          </div>
          <div className="flex items-center gap-1.5">
            <input
              type="file"
              ref={importInputRef}
              onChange={handleFileSelect}
              accept=".lrpgpack,application/gzip,application/x-gzip"
              className="hidden"
            />
            <button
              onClick={() => importInputRef.current?.click()}
              title="Import content package (.lrpgpack)"
              className="flex items-center gap-1 text-xs font-sans px-2 py-1 rounded-lg border border-white/10 bg-white/5 hover:bg-white/10 text-neutral-300 transition-all cursor-pointer"
            >
              <Upload className="w-3.5 h-3.5" />
              <span>Import</span>
            </button>
            <button
              onClick={() => requestSelection({ kind: 'draft' })}
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
                selection?.kind === 'draft'
                  ? 'bg-purple-950/30 border-purple-500/50'
                  : 'bg-stone-900/40 border-stone-700 hover:bg-stone-800/40'
              }`}
            >
              <div className="flex items-center justify-between gap-2">
                <h4 className="font-sans text-xs font-bold text-stone-200 truncate">
                  {name.trim() || 'Untitled World'}
                </h4>
                <span className="text-xs font-mono text-amber-300 bg-amber-950/40 border border-amber-500/30 px-1.5 py-0.5 rounded">
                  unsaved
                </span>
              </div>
            </div>
          )}
          {isLoading && worlds.length === 0 ? (
            <div className="text-center py-8 text-xs font-mono text-stone-500 animate-pulse">
              Loading worlds...
            </div>
          ) : worlds.length === 0 ? (
            <div className="text-center py-8 text-xs text-stone-400">
              No worlds created yet. Craft your first setting!
            </div>
          ) : (
            worlds.map((w) => (
              <div
                key={w.id}
                onClick={() => requestSelection({ kind: 'saved', id: w.id })}
                className={`p-3 rounded-xl border transition-all cursor-pointer text-left ${
                  selection?.kind === 'saved' && selection.id === w.id
                    ? 'bg-purple-950/30 border-purple-500/50 shadow-[0_0_15px_rgba(168,85,247,0.15)]'
                    : 'bg-stone-900/40 border-stone-800/60 hover:bg-stone-800/40 hover:border-stone-700'
                }`}
              >
                <div>
                  <h4 className="font-sans text-xs font-bold text-stone-200 truncate">{w.name}</h4>
                  {w.genre && (
                    <div className="mt-1">
                      <span className="inline-block text-xs font-mono text-purple-400 bg-stone-950 px-1.5 py-0.5 rounded border border-stone-800">
                        {w.genre}
                      </span>
                    </div>
                  )}
                </div>
                {w.description && (
                  <p className="text-xs text-stone-400 truncate mt-1">{w.description}</p>
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
              {selection?.kind === 'saved' ? name || 'Edit World' : 'Create New World'}
            </h2>
            {slugID && (
              <span className="text-xs font-mono text-stone-400 bg-stone-950 px-2 py-0.5 rounded border border-stone-800">
                worlds/{slugID}
              </span>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <div className="flex flex-wrap bg-stone-950/80 p-1 rounded-xl border border-stone-800">
              <button
                type="button"
                onClick={() => setActiveTab('lore')}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
                  activeTab === 'lore'
                    ? 'bg-purple-600 text-white font-bold shadow'
                    : 'text-stone-400 hover:text-white'
                }`}
              >
                <Info className="w-3.5 h-3.5" />
                <span>Lore & Atmosphere</span>
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('prompt')}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
                  activeTab === 'prompt'
                    ? 'bg-purple-600 text-white font-bold shadow'
                    : 'text-stone-400 hover:text-white'
                }`}
              >
                <BookOpen className="w-3.5 h-3.5" />
                <span>lore.md</span>
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('entities')}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1 rounded-lg transition-all cursor-pointer ${
                  activeTab === 'entities'
                    ? 'bg-purple-600 text-white font-bold shadow'
                    : 'text-stone-400 hover:text-white'
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
              title="Export world package (.lrpgpack)"
              className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-white/10 bg-white/5 hover:bg-white/10 text-neutral-300 transition-all cursor-pointer disabled:opacity-50"
            >
              <Download className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Export</span>
            </button>

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

        {/* Toast Feedback */}
        {toast && (
          <div
            className={`p-3 rounded-xl text-xs flex items-center gap-2 ${
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

        {/* Tab 1: Lore & Atmosphere Form */}
        {activeTab === 'lore' && (
          <div className="flex-1 overflow-y-auto min-h-0 space-y-4 pr-2">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                    World Setting Name
                  </label>
                  <AIGenerateButton
                    formType="world"
                    fieldName="name"
                    onError={reportGenerationError}
                    getContext={getWorldContext}
                    onGenerated={(val) => {
                      setName(val);
                      markDirty();
                      if (isDraft) {
                        setSlugID(val.toLowerCase().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, ''));
                      }
                    }}
                    systemID={defaultSystem}
                  />
                </div>
                <input
                  type="text"
                  required
                  placeholder="e.g. Solitary Defiance"
                  value={name}
                  onChange={(e) => {
                    setName(e.target.value);
                    markDirty();
                    setSlugError(false);
                    if (isDraft) {
                      setSlugID(e.target.value.toLowerCase().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, ''));
                    }
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
                />
              </div>

              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                    Genre / Setting Style
                  </label>
                  <AIGenerateButton
                    formType="world"
                    fieldName="genre"
                    onError={reportGenerationError}
                    getContext={getWorldContext}
                    onGenerated={(val) => { setGenre(val); markDirty(); }}
                    systemID={defaultSystem}
                  />
                </div>
                <input
                  type="text"
                  placeholder="e.g. Gothic Fantasy, Cyberpunk"
                  value={genre}
                  onChange={(e) => { setGenre(e.target.value); markDirty(); }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
                />
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                  Directory Slug ID
                </label>
                <input
                  type="text"
                  disabled={selection?.kind === 'saved'}
                  placeholder="e.g. solitary_defiance"
                  value={slugID}
                  onChange={(e) => { setSlugID(e.target.value); setSlugError(false); }}
                  className={`w-full bg-stone-950 border rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none transition-colors font-mono disabled:opacity-60 ${
                    slugError ? 'border-red-500/70 focus:border-red-500' : 'border-stone-800 focus:border-purple-500/50'
                  }`}
                />
                {slugError && (
                  <p className="text-xs text-red-400">
                    That id already exists. Change the name or slug.
                  </p>
                )}
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                  Default Rule System
                </label>
                <select
                  value={defaultSystem}
                  onChange={(e) => setDefaultSystem(e.target.value)}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3.5 pr-9 py-2.5 text-sm text-stone-100 focus:outline-none focus:border-purple-500/50 transition-colors cursor-pointer"
                >
                  {systems.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name} ({s.id})
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <label className="text-xs font-sans uppercase tracking-wider text-stone-300 flex items-center gap-1.5">
                  <Palette className="w-3.5 h-3.5 text-purple-400" />
                  <span>Visual Art Style Prompt Guide</span>
                </label>
                <AIGenerateButton
                  formType="world"
                  fieldName="art_style"
                  onError={reportGenerationError}
                  getContext={getWorldContext}
                  onGenerated={(val) => { setArtStyle(val); markDirty(); }}
                  systemID={defaultSystem}
                  seed={artStyle}
                />
              </div>
              <input
                type="text"
                placeholder="e.g. Dark watercolor gothic, mist, gaslight, copper accents, muted palette"
                value={artStyle}
                onChange={(e) => { setArtStyle(e.target.value); markDirty(); }}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
              />
              <p className="text-xs text-stone-400">
                Injected into image generation prompts to create consistent scene illustrations in this world.
              </p>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-sans uppercase tracking-wider text-stone-300 flex items-center gap-1.5">
                <Tag className="w-3.5 h-3.5 text-purple-400" />
                <span>World Tags (Comma-separated)</span>
              </label>
              <input
                type="text"
                placeholder="e.g. gothic, horror, city, rebellion"
                value={tags}
                onChange={(e) => { setTags(e.target.value); markDirty(); }}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
              />
            </div>

            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                  World Synopsis & Lore
                </label>
                <AIGenerateButton
                  formType="world"
                  fieldName="description"
                  onError={reportGenerationError}
                  getContext={getWorldContext}
                  onGenerated={(val) => { setDescription(val); markDirty(); }}
                  systemID={defaultSystem}
                  seed={description}
                />
              </div>
              <textarea
                rows={4}
                placeholder="Describe the setting, major conflicts, factions, and atmosphere..."
                value={description}
                onChange={(e) => { setDescription(e.target.value); markDirty(); }}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl p-4 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors resize-none"
              />
            </div>

            {/* World Artwork (Banner & Icon) */}
            <div className="space-y-3 pt-2">
              <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                World Artwork
              </label>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                {/* Banner */}
                <div className="p-3 bg-stone-950 border border-stone-800 rounded-xl space-y-2">
                  <span className="text-xs font-sans font-semibold text-stone-400">World Banner</span>
                  <input
                    type="file"
                    ref={bannerInputRef}
                    onChange={() => {
                      const file = bannerInputRef.current?.files?.[0];
                      if (file) {
                        setBannerFile(file);
                        const reader = new FileReader();
                        reader.onloadend = () => {
                          if (typeof reader.result === 'string') {
                            setBannerPreview(reader.result);
                          }
                        };
                        reader.readAsDataURL(file);
                      }
                    }}
                    accept="image/png,image/jpeg,image/webp,image/svg+xml"
                    className="hidden"
                  />
                  <div
                    onClick={() =>
                      bannerPreview ? openLightbox(bannerPreview, 'World banner') : bannerInputRef.current?.click()
                    }
                    className={`h-24 rounded-lg overflow-hidden border border-stone-800 bg-stone-900/40 flex items-center justify-center ${
                      bannerPreview ? 'cursor-zoom-in' : 'cursor-pointer'
                    }`}
                  >
                    {safeImagePreview(bannerPreview) ? (
                      <img
                        src={safeImagePreview(bannerPreview)}
                        alt="Banner Preview"
                        className="w-full h-full object-cover"
                        onError={() => setBannerPreview(null)}
                      />
                    ) : (
                      <span className="text-stone-600 text-xs">Click to upload banner</span>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() => bannerInputRef.current?.click()}
                      disabled={isSaving || generatingKind === 'banner'}
                      className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                    >
                      <Upload className="w-3.5 h-3.5" />
                      <span>Upload</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => handleAIGenerate('banner')}
                      disabled={isSaving || generatingKind === 'banner'}
                      className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                    >
                      <Sparkles className="w-3.5 h-3.5" />
                      <span>{generatingKind === 'banner' ? 'Gen...' : 'AI Gen'}</span>
                    </button>
                  </div>
                </div>

                {/* Icon */}
                <div className="p-3 bg-stone-950 border border-stone-800 rounded-xl space-y-2">
                  <span className="text-xs font-sans font-semibold text-stone-400">World Icon</span>
                  <input
                    type="file"
                    ref={iconInputRef}
                    onChange={() => {
                      const file = iconInputRef.current?.files?.[0];
                      if (file) {
                        setIconFile(file);
                        const reader = new FileReader();
                        reader.onloadend = () => {
                          if (typeof reader.result === 'string') {
                            setIconPreview(reader.result);
                          }
                        };
                        reader.readAsDataURL(file);
                      }
                    }}
                    accept="image/png,image/jpeg,image/webp,image/svg+xml"
                    className="hidden"
                  />
                  <div
                    onClick={() =>
                      iconPreview ? openLightbox(iconPreview, 'World icon') : iconInputRef.current?.click()
                    }
                    className={`h-24 rounded-lg overflow-hidden border border-stone-800 bg-stone-900/40 flex items-center justify-center ${
                      iconPreview ? 'cursor-zoom-in' : 'cursor-pointer'
                    }`}
                  >
                    {safeImagePreview(iconPreview) ? (
                      <img
                        src={safeImagePreview(iconPreview)}
                        alt="Icon Preview"
                        className="w-16 h-16 rounded-xl object-cover"
                        onError={() => setIconPreview(null)}
                      />
                    ) : (
                      <span className="text-stone-600 text-xs">Click to upload icon</span>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() => iconInputRef.current?.click()}
                      disabled={isSaving || generatingKind === 'icon'}
                      className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                    >
                      <Upload className="w-3.5 h-3.5" />
                      <span>Upload</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => handleAIGenerate('icon')}
                      disabled={isSaving || generatingKind === 'icon'}
                      className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                    >
                      <Sparkles className="w-3.5 h-3.5" />
                      <span>{generatingKind === 'icon' ? 'Gen...' : 'AI Gen'}</span>
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Tab 2: Agent Lore Prompt Editor */}
        {activeTab === 'prompt' && (
          <div className="flex-1 flex flex-col gap-2 min-h-0 overflow-hidden">
            <div className="flex items-center justify-between text-xs font-mono text-stone-400 px-1 shrink-0">
              <span>AI Storyteller Atmosphere Instructions (prompts/lore.md)</span>
              <div className="flex items-center gap-2">
                <span>Injected into LLM context to guide sensory tone &amp; faction conflicts</span>
                <AIGenerateButton
                  formType="world"
                  fieldName="lore_prompt"
                  onError={reportGenerationError}
                  getContext={getWorldContext}
                  onGenerated={(val) => { setLorePrompt(val); markDirty(); }}
                  systemID={defaultSystem}
                  seed={lorePrompt}
                />
              </div>
            </div>
            <MarkdownEditor
              key={`${savedID || slugID || 'draft'}-lore`}
              value={lorePrompt}
              onChange={(next) => { setLorePrompt(next); markDirty(); }}
              language="markdown"
              ariaLabel="World lore prompt"
              placeholder="Describe the sensory tone, factions and conflicts the storyteller should hold in mind..."
            />
          </div>
        )}

        {/* Tab 3: Starter Entities Manager */}
        {activeTab === 'entities' && (
          <div className="flex-1 flex gap-4 min-h-0 overflow-hidden">
            {/* Entity List */}
            <div className="w-56 shrink-0 bg-stone-950/60 rounded-xl border border-stone-800/80 p-3 flex flex-col gap-2 min-h-0">
              <div className="flex items-center justify-between pb-2 border-b border-stone-800/60 shrink-0">
                <span className="text-xs font-sans uppercase tracking-wider text-stone-400">
                  Templates
                </span>
                <button
                  type="button"
                  onClick={() => setIsNewEntityModal(true)}
                  className="text-xs font-sans font-bold px-2 py-0.5 rounded bg-purple-600 text-white cursor-pointer hover:bg-purple-500"
                >
                  + Add
                </button>
              </div>

              <div className="flex-1 min-h-0">
                {entities.length === 0 ? (
                  <div className="text-xs text-stone-500 py-6 text-center">
                    No starter templates. Click + Add to create one!
                  </div>
                ) : (
                  <EntityTree
                    folders={worldFolders}
                    entities={entities}
                    selectedId={selectedEntityID ?? undefined}
                    onSelect={(id) => void handleSelectEntity(id)}
                    onMoveEntity={async (id, folder) => {
                      if (!savedID) {
                        setToast({ type: 'error', message: 'Save the world before organising its templates.' });
                        return;
                      }
                      const note = await APIClient.getWorldEntity(savedID, id);
                      await APIClient.saveWorldEntity(savedID, id, note.markdown, folder);
                      await loadWorldDetail(savedID);
                      await refreshWorldFolders();
                    }}
                    onMoveFolder={async (from, to) => {
                      if (!savedID) return;
                      await APIClient.moveWorldFolder(savedID, from, to);
                      await refreshWorldFolders();
                    }}
                    onCreateFolder={async (path) => {
                      if (!savedID) {
                        setToast({ type: 'error', message: 'Save the world before creating folders.' });
                        return;
                      }
                      await APIClient.createWorldFolder(savedID, path);
                      await refreshWorldFolders();
                    }}
                    onDeleteFolder={async (path) => {
                      if (!savedID) return;
                      await APIClient.deleteWorldFolder(savedID, path, true);
                      await refreshWorldFolders();
                    }}
                  />
                )}
              </div>
            </div>

            {/* Entity Markdown Editor */}
            <div className="flex-1 min-w-0 flex flex-col gap-2 min-h-0 overflow-hidden">
              {selectedEntityID ? (
                <>
                  <div className="flex items-center justify-between text-xs font-mono text-stone-400 px-1 shrink-0">
                    <span>worlds/{savedID || slugID || 'draft'}/entities/{selectedEntityID}.md</span>
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        onClick={() => void handleDeleteEntity(selectedEntityID)}
                        title="Delete entity template"
                        className="flex items-center gap-1 px-2 py-1 rounded-lg border border-red-500/40 text-red-300 hover:bg-red-950/40 font-bold text-xs cursor-pointer transition-all"
                      >
                        <Trash2 className="w-3 h-3" />
                        <span>Delete</span>
                      </button>
                      <button
                        type="button"
                        onClick={handleSaveEntity}
                        className="flex items-center gap-1 px-3 py-1 rounded-lg bg-purple-600 hover:bg-purple-500 text-white font-bold text-xs cursor-pointer shadow transition-all"
                      >
                        <Save className="w-3 h-3" />
                        <span>{savedID ? 'Save Entity' : 'Update Draft'}</span>
                      </button>
                    </div>
                  </div>
                  <MarkdownEditor
                    key={`${savedID || slugID || 'draft'}-${selectedEntityID}`}
                    value={entityMarkdown}
                    onChange={(next) => { setEntityMarkdown(next); markDirty(); }}
                    language="markdown-frontmatter"
                    ariaLabel="World entity template markdown"
                  />
                </>
              ) : (
                <div className="flex-1 flex items-center justify-center text-xs text-stone-500 font-mono">
                  Select or create an entity template to edit its Markdown.
                </div>
              )}
            </div>
          </div>
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
          onConfirm={handleConfirmImport}
          onCancel={() => {
            setImportFile(null);
            setImportManifest(null);
          }}
          loading={isImporting}
        />
      )}
    </div>
  );
};
