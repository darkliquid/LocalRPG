import React, { useState, useEffect } from 'react';
import { APIClient } from '../api/client';
import { WorldInfo, SystemInfo, WorldEntitySummary, CreateWorldRequest } from '../types';
import { Globe, Plus, Save, Info, FileText, Check, AlertCircle, Trash2, Tag, Palette, RotateCcw, BookOpen } from 'lucide-react';
import { REFERENCE_WORLD_TEMPLATE } from '../templates/referenceTemplates';

interface WorldsStudioProps {
  onWorldSaved?: () => void;
}

const STARTER_ENTITY_TEMPLATE = `---
name: The Whispering Bastion
type: location
state:
  danger_level: 2
wikilinks: []
---
An ancient stone fortress overlooking the misty valleys. Legends say its halls whisper secrets to those who wander in twilight.
`;

export const WorldsStudio: React.FC<WorldsStudioProps> = ({ onWorldSaved }) => {
  const [worlds, setWorlds] = useState<WorldInfo[]>([]);
  const [systems, setSystems] = useState<SystemInfo[]>([]);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'lore' | 'prompt' | 'entities'>('lore');

  // World form state
  const [name, setName] = useState(REFERENCE_WORLD_TEMPLATE.name);
  const [slugID, setSlugID] = useState(REFERENCE_WORLD_TEMPLATE.id);
  const [genre, setGenre] = useState(REFERENCE_WORLD_TEMPLATE.genre);
  const [defaultSystem, setDefaultSystem] = useState(REFERENCE_WORLD_TEMPLATE.default_system);
  const [artStyle, setArtStyle] = useState(REFERENCE_WORLD_TEMPLATE.art_style);
  const [tags, setTags] = useState(REFERENCE_WORLD_TEMPLATE.tags.join(', '));
  const [description, setDescription] = useState(REFERENCE_WORLD_TEMPLATE.description);
  const [lorePrompt, setLorePrompt] = useState(REFERENCE_WORLD_TEMPLATE.lore_prompt);

  // Entities state
  const [entities, setEntities] = useState<WorldEntitySummary[]>(
    REFERENCE_WORLD_TEMPLATE.entities.map((e) => ({ id: e.id, name: e.name, type: e.type }))
  );
  const [selectedEntityID, setSelectedEntityID] = useState<string | null>(
    REFERENCE_WORLD_TEMPLATE.entities[0]?.id || null
  );
  const [entityMarkdown, setEntityMarkdown] = useState(
    REFERENCE_WORLD_TEMPLATE.entities[0]?.markdown || ''
  );
  const [entityDrafts, setEntityDrafts] = useState<Record<string, string>>(() => {
    const initial: Record<string, string> = {};
    REFERENCE_WORLD_TEMPLATE.entities.forEach((e) => {
      initial[e.id] = e.markdown;
    });
    return initial;
  });
  const [isNewEntityModal, setIsNewEntityModal] = useState(false);
  const [newEntitySlug, setNewEntitySlug] = useState('');

  const [isLoading, setIsLoading] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  useEffect(() => {
    loadWorlds();
  }, []);

  const loadWorlds = async (selectID?: string) => {
    setIsLoading(true);
    try {
      const [wList, sList] = await Promise.all([
        APIClient.listWorlds(),
        APIClient.listSystems(),
      ]);
      setWorlds(wList);
      setSystems(sList);
      const target = selectID || (wList.length > 0 ? wList[0].id : null);
      if (target) {
        loadWorldDetail(target);
      } else {
        handleNewWorld(sList);
      }
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Failed to load worlds' });
    } finally {
      setIsLoading(false);
    }
  };

  const loadWorldDetail = async (id: string) => {
    try {
      const detail = await APIClient.getWorld(id);
      setSelectedID(detail.id);
      setName(detail.name);
      setSlugID(detail.id);
      setGenre(detail.genre || '');
      setDefaultSystem(detail.default_system || (systems[0]?.id ?? ''));
      setArtStyle(detail.art_style || '');
      setTags(detail.tags ? detail.tags.join(', ') : '');
      setDescription(detail.description || '');
      setLorePrompt(detail.lore_prompt || REFERENCE_WORLD_TEMPLATE.lore_prompt);
      setEntities(detail.entities || []);

      if (detail.entities && detail.entities.length > 0) {
        const first = detail.entities[0];
        setSelectedEntityID(first.id);
        try {
          const ent = await APIClient.getWorldEntity(detail.id, first.id);
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
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Failed to load world details' });
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
    if (selectedID) {
      try {
        const ent = await APIClient.getWorldEntity(selectedID, targetId);
        setEntityDrafts((prev) => ({ ...prev, [targetId]: ent.markdown }));
        setEntityMarkdown(ent.markdown);
      } catch (err: any) {
        setToast({ type: 'error', message: err.message || 'Failed to load entity markdown' });
      }
    } else {
      setEntityMarkdown('');
    }
  };

  const handleNewWorld = (sysList?: SystemInfo[]) => {
    setSelectedID(null);
    setName(REFERENCE_WORLD_TEMPLATE.name);
    setSlugID(REFERENCE_WORLD_TEMPLATE.id);
    setGenre(REFERENCE_WORLD_TEMPLATE.genre);
    const availableSys = sysList && sysList.length > 0 ? sysList : systems;
    const matchingSys = availableSys.find((s) => s.id === REFERENCE_WORLD_TEMPLATE.default_system);
    setDefaultSystem(matchingSys ? matchingSys.id : (availableSys[0]?.id ?? ''));
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
    setActiveTab('lore');
  };

  const handleResetToReference = () => {
    setName(REFERENCE_WORLD_TEMPLATE.name);
    if (!selectedID) {
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
    setToast({ type: 'success', message: 'Reset to The Ashen Reach Reference Template!' });
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
        id: slugID.trim() || undefined,
        name: name.trim(),
        description: description.trim(),
        genre: genre.trim(),
        default_system: defaultSystem || (systems[0]?.id ?? ''),
        art_style: artStyle.trim(),
        tags: parsedTags,
        lore_prompt: lorePrompt.trim(),
      };

      const saved = await APIClient.saveWorld(payload);

      // Save all entity templates (whether new world or edited world)
      const allDrafts = { ...entityDrafts };
      if (selectedEntityID) {
        allDrafts[selectedEntityID] = entityMarkdown;
      }

      for (const ent of entities) {
        const md = allDrafts[ent.id] || STARTER_ENTITY_TEMPLATE;
        await APIClient.saveWorldEntity(saved.id, ent.id, md).catch(() => {});
      }

      setToast({ type: 'success', message: `World "${saved.name}" saved successfully!` });
      await loadWorlds(saved.id);
      if (onWorldSaved) onWorldSaved();
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Failed to save world' });
    } finally {
      setIsSaving(false);
    }
  };

  const handleSaveEntity = async () => {
    if (!selectedEntityID) return;

    setEntityDrafts((prev) => ({ ...prev, [selectedEntityID]: entityMarkdown }));

    if (!selectedID) {
      setToast({ type: 'success', message: `Draft entity "${selectedEntityID}" updated! (Will be persisted when you click Save World)` });
      return;
    }

    try {
      await APIClient.saveWorldEntity(selectedID, selectedEntityID, entityMarkdown);
      setToast({ type: 'success', message: `Entity "${selectedEntityID}" saved!` });
      await loadWorldDetail(selectedID);
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Failed to save entity' });
    }
  };

  const handleDeleteEntity = async (entityId: string) => {
    if (!selectedID) {
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
      return;
    }

    try {
      await APIClient.deleteWorldEntity(selectedID, entityId);
      setToast({ type: 'success', message: `Entity "${entityId}" deleted!` });
      await loadWorldDetail(selectedID);
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Failed to delete entity' });
    }
  };

  const handleCreateNewEntity = async () => {
    if (!newEntitySlug.trim()) return;
    const slug = newEntitySlug.toLowerCase().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, '');

    if (!selectedID) {
      if (entities.some((e) => e.id === slug)) {
        setToast({ type: 'error', message: `Entity "${slug}" already exists` });
        return;
      }
      const newSummary: WorldEntitySummary = { id: slug, name: slug, type: 'concept' };
      setEntities((prev) => [...prev, newSummary]);
      setEntityDrafts((prev) => ({
        ...prev,
        ...(selectedEntityID ? { [selectedEntityID]: entityMarkdown } : {}),
        [slug]: STARTER_ENTITY_TEMPLATE,
      }));
      setSelectedEntityID(slug);
      setEntityMarkdown(STARTER_ENTITY_TEMPLATE);
      setIsNewEntityModal(false);
      setNewEntitySlug('');
      return;
    }

    try {
      await APIClient.saveWorldEntity(selectedID, slug, STARTER_ENTITY_TEMPLATE);
      setIsNewEntityModal(false);
      setNewEntitySlug('');
      await loadWorldDetail(selectedID);
      await handleSelectEntity(slug);
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Failed to create entity' });
    }
  };

  return (
    <div className="flex-1 flex flex-col md:flex-row gap-6 overflow-hidden">
      {/* Left Master Column: Worlds List */}
      <aside className="w-full md:w-80 bg-glass-card rounded-2xl border border-stone-800/80 p-4 flex flex-col gap-4 shadow-xl backdrop-blur-md">
        <div className="flex items-center justify-between pb-2 border-b border-stone-800/60">
          <div className="flex items-center gap-2">
            <Globe className="w-4 h-4 text-purple-400" />
            <h3 className="font-sans text-sm font-bold text-stone-200 uppercase tracking-wider">
              Worlds Studio
            </h3>
          </div>
          <button
            onClick={() => handleNewWorld()}
            className="flex items-center gap-1 text-[11px] font-sans font-bold px-2.5 py-1 rounded-lg bg-purple-600 hover:bg-purple-500 text-white transition-all cursor-pointer shadow"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>New</span>
          </button>
        </div>

        <div className="flex-1 overflow-y-auto space-y-2 pr-1">
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
                onClick={() => loadWorldDetail(w.id)}
                className={`p-3 rounded-xl border transition-all cursor-pointer text-left ${
                  selectedID === w.id
                    ? 'bg-purple-950/30 border-purple-500/50 shadow-[0_0_15px_rgba(168,85,247,0.15)]'
                    : 'bg-stone-900/40 border-stone-800/60 hover:bg-stone-800/40 hover:border-stone-700'
                }`}
              >
                <div className="flex items-center justify-between">
                  <h4 className="font-sans text-xs font-bold text-stone-200 truncate">{w.name}</h4>
                  {w.genre && (
                    <span className="text-[10px] font-mono text-purple-400 bg-stone-950 px-1.5 py-0.5 rounded border border-stone-800">
                      {w.genre}
                    </span>
                  )}
                </div>
                {w.description && (
                  <p className="text-[11px] text-stone-400 truncate mt-1">{w.description}</p>
                )}
              </div>
            ))
          )}
        </div>
      </aside>

      {/* Right Detail Column: Editor */}
      <section className="flex-1 bg-glass-card rounded-2xl border border-stone-800/80 p-6 flex flex-col gap-5 shadow-xl backdrop-blur-md overflow-hidden">
        {/* Top Header & Sub-Tabs */}
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-800/80 pb-3">
          <div className="flex items-center gap-3">
            <h2 className="font-sans text-lg font-bold text-purple-400">
              {selectedID ? name || 'Edit World' : 'Create New World'}
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
              onClick={handleResetToReference}
              title="Reset current editor to the comprehensive Ashen Reach reference template"
              className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-stone-800 hover:border-purple-500/50 bg-stone-900/60 hover:bg-stone-800 text-stone-300 hover:text-purple-400 transition-all cursor-pointer"
            >
              <RotateCcw className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Reset Template</span>
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
          <div className="flex-1 overflow-y-auto space-y-4 pr-1">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                  World Setting Name
                </label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Solitary Defiance"
                  value={name}
                  onChange={(e) => {
                    setName(e.target.value);
                    if (!selectedID) {
                      setSlugID(e.target.value.toLowerCase().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, ''));
                    }
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                  Genre / Setting Style
                </label>
                <input
                  type="text"
                  placeholder="e.g. Gothic Fantasy, Cyberpunk"
                  value={genre}
                  onChange={(e) => setGenre(e.target.value)}
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
                  disabled={!!selectedID}
                  placeholder="e.g. solitary_defiance"
                  value={slugID}
                  onChange={(e) => setSlugID(e.target.value)}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors font-mono disabled:opacity-60"
                />
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
              <label className="text-xs font-sans uppercase tracking-wider text-stone-300 flex items-center gap-1.5">
                <Palette className="w-3.5 h-3.5 text-purple-400" />
                <span>Visual Art Style Prompt Guide</span>
              </label>
              <input
                type="text"
                placeholder="e.g. Dark watercolor gothic, mist, gaslight, copper accents, muted palette"
                value={artStyle}
                onChange={(e) => setArtStyle(e.target.value)}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
              />
              <p className="text-[11px] text-stone-400">
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
                onChange={(e) => setTags(e.target.value)}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                World Synopsis & Lore
              </label>
              <textarea
                rows={4}
                placeholder="Describe the setting, major conflicts, factions, and atmosphere..."
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl p-4 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors resize-none"
              />
            </div>
          </div>
        )}

        {/* Tab 2: Agent Lore Prompt Editor */}
        {activeTab === 'prompt' && (
          <div className="flex-1 flex flex-col gap-2 overflow-hidden">
            <div className="flex items-center justify-between text-[11px] font-mono text-stone-400 px-1">
              <span>AI Storyteller Atmosphere Instructions (prompts/lore.md)</span>
              <span>Injected into LLM context to guide sensory tone & faction conflicts</span>
            </div>
            <textarea
              value={lorePrompt}
              onChange={(e) => setLorePrompt(e.target.value)}
              spellCheck={false}
              className="flex-1 w-full bg-stone-950 border border-stone-800 rounded-xl p-4 text-xs font-mono text-stone-200 leading-relaxed focus:outline-none focus:border-purple-500/50 transition-colors resize-none selection:bg-purple-900/60"
            />
          </div>
        )}

        {/* Tab 3: Starter Entities Manager */}
        {activeTab === 'entities' && (
          <div className="flex-1 flex gap-4 overflow-hidden">
            {/* Entity List */}
            <div className="w-56 shrink-0 bg-stone-950/60 rounded-xl border border-stone-800/80 p-3 flex flex-col gap-2">
              <div className="flex items-center justify-between pb-2 border-b border-stone-800/60">
                <span className="text-[11px] font-sans uppercase tracking-wider text-stone-400">
                  Templates
                </span>
                <button
                  type="button"
                  onClick={() => setIsNewEntityModal(true)}
                  className="text-[10px] font-sans font-bold px-2 py-0.5 rounded bg-purple-600 text-white cursor-pointer hover:bg-purple-500"
                >
                  + Add
                </button>
              </div>

              <div className="flex-1 overflow-y-auto space-y-1.5 pr-1">
                {entities.length === 0 ? (
                  <div className="text-[11px] text-stone-500 py-6 text-center">
                    No starter templates. Click + Add to create one!
                  </div>
                ) : (
                  entities.map((e) => (
                    <div
                      key={e.id}
                      onClick={() => handleSelectEntity(e.id)}
                      className={`group p-2 rounded-lg border text-left cursor-pointer flex items-center justify-between transition-all ${
                        selectedEntityID === e.id
                          ? 'bg-purple-950/40 border-purple-500/50 text-purple-300'
                          : 'bg-stone-900/40 border-stone-800/60 text-stone-300 hover:bg-stone-800'
                      }`}
                    >
                      <div className="truncate">
                        <div className="text-xs font-bold truncate">{e.name || e.id}</div>
                        <div className="text-[10px] text-stone-500">{e.type}</div>
                      </div>
                      <button
                        type="button"
                        onClick={(ev) => {
                          ev.stopPropagation();
                          handleDeleteEntity(e.id);
                        }}
                        className="opacity-0 group-hover:opacity-100 text-stone-500 hover:text-red-400 p-1 transition-opacity"
                        title="Delete entity template"
                      >
                        <Trash2 className="w-3 h-3" />
                      </button>
                    </div>
                  ))
                )}
              </div>
            </div>

            {/* Entity Markdown Editor */}
            <div className="flex-1 min-w-0 flex flex-col gap-2 overflow-hidden">
              {selectedEntityID ? (
                <>
                  <div className="flex items-center justify-between text-xs font-mono text-stone-400 px-1">
                    <span>worlds/{selectedID || slugID || 'draft'}/entities/{selectedEntityID}.md</span>
                    <button
                      type="button"
                      onClick={handleSaveEntity}
                      className="flex items-center gap-1 px-3 py-1 rounded-lg bg-purple-600 hover:bg-purple-500 text-white font-bold text-xs cursor-pointer shadow transition-all"
                    >
                      <Save className="w-3 h-3" />
                      <span>{selectedID ? 'Save Entity' : 'Update Draft'}</span>
                    </button>
                  </div>
                  <textarea
                    value={entityMarkdown}
                    onChange={(e) => setEntityMarkdown(e.target.value)}
                    spellCheck={false}
                    className="flex-1 w-full bg-stone-950 border border-stone-800 rounded-xl p-4 text-xs font-mono text-stone-200 leading-relaxed focus:outline-none focus:border-purple-500/50 transition-colors resize-none selection:bg-purple-900/60"
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

      {/* New Entity Modal */}
      {isNewEntityModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-fade-in">
          <div className="w-full max-w-sm max-h-[85vh] flex flex-col rounded-2xl bg-stone-900 border border-purple-500/30 shadow-2xl overflow-hidden">
            <div className="px-6 py-4 border-b border-stone-800 shrink-0">
              <h3 className="font-sans text-sm font-bold text-purple-400">
                New Starter Entity Template
              </h3>
            </div>
            <div className="p-6 space-y-4 flex-1 min-h-0 overflow-y-auto">
              <div className="space-y-1">
                <label className="text-xs text-stone-300">Entity Slug (filename without .md)</label>
                <input
                  type="text"
                  placeholder="e.g. the_iron_bastion"
                  value={newEntitySlug}
                  onChange={(e) => setNewEntitySlug(e.target.value)}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 font-mono focus:outline-none focus:border-purple-500/50"
                />
              </div>
            </div>
            <div className="flex items-center justify-end gap-2 px-6 py-3 border-t border-stone-800 shrink-0 bg-stone-900/80">
              <button
                type="button"
                onClick={() => setIsNewEntityModal(false)}
                className="px-3 py-1.5 text-xs text-stone-400 hover:text-white cursor-pointer"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleCreateNewEntity}
                disabled={!newEntitySlug.trim()}
                className="px-4 py-1.5 text-xs font-sans font-bold bg-purple-600 text-white rounded-lg disabled:opacity-50 cursor-pointer"
              >
                Create
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
