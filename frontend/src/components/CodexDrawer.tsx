import React, { useMemo, useState, useEffect } from 'react';
import { EntityMemory, EntityNote, EntitySummary, TTSConfig, VoiceProfile, GenerationFailure } from '../types';
import { APIClient, GenerationError } from '../api/client';
import { Save, Volume2, Search, BookOpen, PanelLeftClose, PanelLeft, GitMerge, X, Loader2, RotateCw, AlertCircle } from 'lucide-react';
import { formatGenerationError } from '../lib/generationError';
import { TurnHistoryList } from './TurnHistoryList';
import { ImageLightbox } from './ImageLightbox';
import { useLightbox } from '../hooks/useLightbox';

interface CodexDrawerProps {
  gameID?: string;
  entity?: EntityNote;
  entities?: EntitySummary[];
  voiceProfiles?: VoiceProfile[];
  ttsConfig?: TTSConfig;
  activeProvider?: string;
  onAddProfile?: (profile: VoiceProfile) => void;
  onSelect: (entityId: string) => void;
  onSave: (entityId: string, markdown: string) => Promise<void> | void;
  onMerge?: (sourceID: string, intoID: string) => void;
}

// inlineYaml renders a canonical option value so an imported profile's tunables
// survive into the character's frontmatter.
function inlineYaml(value: unknown): string {
  return typeof value === 'string' ? JSON.stringify(value) : String(value);
}

export const CodexDrawer: React.FC<CodexDrawerProps> = ({
  gameID,
  entity,
  entities,
  voiceProfiles,
  ttsConfig,
  onSelect,
  onSave,
  onMerge,
}) => {
  const [markdown, setMarkdown] = useState('');
  const [query, setQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState('all');
  const [isSidebarOpen, setIsSidebarOpen] = useState(!entity);
  const [isMergeOpen, setIsMergeOpen] = useState(false);
  const [mergeTarget, setMergeTarget] = useState('');
  const [saveError, setSaveError] = useState('');
  const [memories, setMemories] = useState<EntityMemory[]>([]);
  const [previewProfileId, setPreviewProfileId] = useState('');
  const [isPreviewing, setIsPreviewing] = useState(false);
  const { lightbox, isLightboxOpen, openLightbox, closeLightbox } = useLightbox();
  const [sidebarTab, setSidebarTab] = useState<'notes' | 'memories'>('notes');
  const [portraitVersion, setPortraitVersion] = useState(0);
  const [isRegeneratingPortrait, setIsRegeneratingPortrait] = useState(false);
  const [portraitError, setPortraitError] = useState<GenerationFailure | null>(null);

  const profiles = voiceProfiles ?? [];

  useEffect(() => {
    if (entity) {
      setMarkdown(entity.markdown);
    } else {
      setIsSidebarOpen(true);
    }
    setPortraitVersion(0);
    setPortraitError(null);
  }, [entity]);

  // The entity's memory timeline is read-only and fetched on selection.
  useEffect(() => {
    if (!gameID || !entity?.id) {
      setMemories([]);
      return;
    }
    let cancelled = false;
    APIClient.listEntityMemories(gameID, entity.id)
      .then((rows) => {
        if (!cancelled) setMemories(rows);
      })
      .catch(() => {
        if (!cancelled) setMemories([]);
      });
    return () => {
      cancelled = true;
    };
  }, [gameID, entity?.id]);

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
    const profile = profiles.find((p) => p.id === profileId);
    if (!profile) return;

    const lines = [`voice_id: "${profile.voice_id}"`];
    if (profile.provider) lines.push(`provider: "${profile.provider}"`);
    lines.push(`pitch: ${profile.pitch}`);
    lines.push(`speech_rate: ${profile.speech_rate}`);
    if (profile.options && Object.keys(profile.options).length > 0) {
      const entries = Object.entries(profile.options).map(([key, value]) => `${key}: ${inlineYaml(value)}`);
      lines.push(`options:\n    ${entries.join('\n    ')}`);
    }
    const voiceSnippet = `voice:\n  ${lines.join('\n  ')}`;
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

  const handlePreviewVoice = () => {
    const profile = profiles.find((p) => p.id === previewProfileId);
    if (!profile || !ttsConfig || isPreviewing) return;
    setIsPreviewing(true);
    const greetingText = `Greetings. I am ${entity?.name ?? 'ready for the journey'}.`;
    APIClient.testProvider({
      category: 'tts',
      provider: { ...ttsConfig, default_voice: profile.voice_id, pitch: profile.pitch, speech_rate: profile.speech_rate },
      test_prompt: greetingText,
      voice_id: profile.voice_id,
    })
      .then((result) => {
        if (result.audio_data_uri) {
          const audio = new Audio(result.audio_data_uri);
          audio.play().catch(console.error);
        }
      })
      .catch(console.error)
      .finally(() => setIsPreviewing(false));
  };

  const handleSave = async () => {
    if (!entity) return;
    setSaveError('');
    try {
      await onSave(entity.id, markdown);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    }
  };

  const handleRegeneratePortrait = () => {
    if (!gameID || !entity || isRegeneratingPortrait) return;
    setIsRegeneratingPortrait(true);
    setPortraitError(null);
    APIClient.regenerateCharacterPortrait(gameID, entity.id)
      .then((dto) => {
        const parsed = dto.generated_at ? Date.parse(dto.generated_at) : NaN;
        setPortraitVersion(Number.isFinite(parsed) ? parsed : Date.now());
      })
      .catch((err: unknown) => {
        setPortraitError(
          err instanceof GenerationError
            ? err.failure
            : { code: 'provider_error', message: err instanceof Error ? err.message : String(err) }
        );
      })
      .finally(() => setIsRegeneratingPortrait(false));
  };

  const mergeCandidates = (entities ?? []).filter((candidate) => candidate.id !== entity?.id);

  const portraitURL =
    gameID && entity
      ? `/api/game/${encodeURIComponent(gameID)}/character/${encodeURIComponent(entity.id)}/portrait${
          portraitVersion ? `?v=${portraitVersion}` : ''
        }`
      : '';

  return (
    <div className="flex flex-col md:flex-row gap-4 h-full min-h-0 overflow-x-hidden">
      {/* Corpus browser: collapsible sidebar */}
      {isSidebarOpen && (
        <aside className="w-full md:w-72 shrink-0 flex flex-col gap-2 min-h-0 border-b md:border-b-0 md:border-r border-white/10 pb-4 md:pb-0 md:pr-4">
          <div className="flex items-center justify-between gap-2 pb-1">
            <div className="flex flex-wrap gap-1">
              <button
                type="button"
                onClick={() => setSidebarTab('notes')}
                className={`text-xs font-sans font-bold uppercase tracking-wider px-2 py-1 rounded border transition-colors cursor-pointer ${
                  sidebarTab === 'notes'
                    ? 'bg-purple-600/30 border-purple-500/50 text-purple-200'
                    : 'bg-black/40 border-white/10 text-stone-400 hover:text-stone-200'
                }`}
              >
                Notes ({entities?.length ?? 0})
              </button>
              <button
                type="button"
                onClick={() => setSidebarTab('memories')}
                className={`text-xs font-sans font-bold uppercase tracking-wider px-2 py-1 rounded border transition-colors cursor-pointer ${
                  sidebarTab === 'memories'
                    ? 'bg-purple-600/30 border-purple-500/50 text-purple-200'
                    : 'bg-black/40 border-white/10 text-stone-400 hover:text-stone-200'
                }`}
              >
                Memories ({memories.length})
              </button>
            </div>
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

          {sidebarTab === 'notes' && (
            <>
              <div className="relative">
                <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-stone-500" />
                <input
                  type="text"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Search the codex..."
                  className="w-full bg-black/50 border border-white/10 rounded-lg pl-8 pr-2 py-1.5 text-xs text-stone-200 placeholder-stone-500 focus:outline-none focus:border-purple-500/60"
                />
              </div>

              <div className="flex flex-wrap gap-1">
                <button
                  onClick={() => setTypeFilter('all')}
                  className={`text-xs font-mono px-2 py-0.5 rounded border transition-colors cursor-pointer ${
                    typeFilter === 'all'
                      ? 'bg-purple-600/30 border-purple-500/50 text-purple-200'
                      : 'bg-black/40 border-white/10 text-stone-400 hover:text-stone-200'
                  }`}
                >
                  all ({entities?.length ?? 0})
                </button>
                {types.map(([type, count]) => (
                  <button
                    key={type}
                    onClick={() => setTypeFilter(type)}
                    className={`text-xs font-mono px-2 py-0.5 rounded border transition-colors cursor-pointer ${
                      typeFilter === type
                        ? 'bg-purple-600/30 border-purple-500/50 text-purple-200'
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
                          ? 'bg-purple-600/20 border-purple-500/40'
                          : 'bg-black/30 border-white/5 hover:border-purple-500/30'
                      }`}
                    >
                      <div className="text-xs text-stone-200 truncate font-medium">{candidate.name}</div>
                      <div className="text-xs font-mono text-stone-500 truncate">
                        {candidate.type || 'note'}
                        {candidate.location ? ` · ${candidate.location.replace(/\[\[|\]\]/g, '')}` : ''}
                      </div>
                    </button>
                  ))
                )}
              </div>
            </>
          )}

          {sidebarTab === 'memories' && (
            <div className="flex-1 min-h-[160px] overflow-y-auto space-y-1 pr-1">
              {!entity ? (
                <p className="text-stone-500 text-xs italic p-2">Select a note to see its memories.</p>
              ) : memories.length === 0 ? (
                <p className="text-stone-500 text-xs italic p-2">No memories yet.</p>
              ) : (
                memories.map((memory, index) => (
                  <div
                    key={`${memory.turn}-${index}`}
                    className="text-xs text-stone-300 bg-black/30 border border-white/5 rounded px-2 py-1"
                  >
                    <span className="font-mono text-stone-500 mr-1">t{memory.turn}</span>
                    {memory.text}
                  </div>
                ))
              )}
            </div>
          )}
        </aside>
      )}

      {/* Note editor */}
      <div className="flex-1 min-w-0 min-h-0 flex flex-col">
        {!entity ? (
          <div className="h-full flex flex-col items-center justify-center text-center gap-2 text-stone-500 italic p-6">
            <BookOpen className="w-8 h-8 text-purple-500/40" />
            <span className="text-sm">Choose a note from the codex to read or edit it.</span>
          </div>
        ) : (
          <div className="space-y-4 flex flex-col h-full min-h-0">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-white/10 pb-3">
              <div className="flex items-center gap-3 min-w-0">
                {!isSidebarOpen && (
                  <button
                    onClick={() => setIsSidebarOpen(true)}
                    className="flex items-center gap-1.5 text-xs font-sans px-2.5 py-1.5 rounded-lg bg-stone-900/80 border border-white/10 text-stone-300 hover:text-purple-300 hover:border-purple-500/40 cursor-pointer transition-colors shrink-0"
                    title="Browse other notes"
                  >
                    <PanelLeft className="w-3.5 h-3.5 text-purple-400" />
                    <span>Browse Notes</span>
                  </button>
                )}
                {entity.type === 'character' && gameID && (
                  <div className="flex items-center gap-1.5 shrink-0">
                    <div
                      onClick={() => openLightbox(portraitURL, entity.name)}
                      className="w-14 h-14 rounded-xl overflow-hidden shrink-0 border-2 border-purple-500/30 shadow-lg bg-black/40 cursor-zoom-in transition-transform hover:scale-105"
                      title={`View portrait of ${entity.name}`}
                    >
                      <img src={portraitURL} alt={entity.name} className="w-full h-full object-cover" loading="lazy" />
                    </div>
                    <button
                      type="button"
                      onClick={handleRegeneratePortrait}
                      disabled={isRegeneratingPortrait}
                      className="p-1.5 rounded-lg bg-stone-900/80 border border-purple-500/30 text-purple-300 hover:text-purple-100 hover:border-purple-400 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                      title="Regenerate portrait"
                      aria-label="Regenerate portrait"
                    >
                      {isRegeneratingPortrait ? (
                        <Loader2 className="w-4 h-4 animate-spin" />
                      ) : (
                        <RotateCw className="w-4 h-4" />
                      )}
                    </button>
                  </div>
                )}
                <div className="min-w-0">
                  <h2 className="text-xl font-sans text-purple-400 font-bold truncate">{entity.name}</h2>
                  <span className="text-xs font-mono uppercase text-stone-400">{entity.type}</span>
                </div>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                {onMerge && mergeCandidates.length > 0 && (
                  <button
                    onClick={() => {
                      setMergeTarget('');
                      setIsMergeOpen(true);
                    }}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-stone-900 border border-stone-700 hover:border-red-500/50 hover:text-red-300 text-stone-300 text-xs font-sans cursor-pointer transition-colors"
                    title="Fold this note into another"
                  >
                    <GitMerge className="w-3.5 h-3.5" />
                    <span>Merge note…</span>
                  </button>
                )}
                <button
                  onClick={handleSave}
                  className="flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-purple-600 hover:bg-purple-500 text-white font-sans font-bold text-xs shadow-md transition-all cursor-pointer"
                >
                  <Save className="w-3.5 h-3.5" />
                  <span>Save</span>
                </button>
              </div>
            </div>

            {(saveError || entity.parse_error) && (
              <div className="text-xs rounded-lg border border-red-500/40 bg-red-950/40 text-red-200 px-3 py-2">
                {saveError
                  ? `Save failed: ${saveError}`
                  : 'This note could not be parsed. Fix its frontmatter, then save to restore it.'}
              </div>
            )}

            {portraitError && (
              <div className="text-xs rounded-lg border border-red-500/40 bg-red-950/40 text-red-200 px-3 py-2 flex items-start gap-2">
                <AlertCircle className="w-3.5 h-3.5 text-red-400 shrink-0 mt-0.5" />
                <span>{formatGenerationError(portraitError)}</span>
              </div>
            )}

            <div className="flex flex-wrap items-center gap-2 px-1">
              <label className="flex items-center gap-1.5 text-xs text-stone-400 font-sans shrink-0">
                <Volume2 className="w-3.5 h-3.5 text-purple-400" />
                <span>Apply Voice Archetype:</span>
              </label>
              <select
                value={previewProfileId}
                onChange={(e) => {
                  setPreviewProfileId(e.target.value);
                  if (e.target.value) {
                    applyVoiceArchetype(e.target.value);
                  }
                }}
                disabled={profiles.length === 0}
                className="max-w-[280px] truncate bg-stone-900 border border-purple-500/30 rounded-lg pl-2.5 pr-8 py-1 text-xs font-mono text-purple-300 focus:outline-none cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <option value="">
                  {profiles.length === 0 ? 'Configure voices in Settings' : 'Select Archetype…'}
                </option>
                {profiles.map((p) => {
                  const tags = (p.tags ?? []).join(', ');
                  const suffix = tags ? ` [${tags}]` : '';
                  return (
                    <option key={p.id} value={p.id}>
                      {p.name} ({p.voice_id}){suffix}
                    </option>
                  );
                })}
              </select>
              {ttsConfig && previewProfileId && (
                <button
                  onClick={handlePreviewVoice}
                  disabled={isPreviewing}
                  title="Preview this voice with a greeting"
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg bg-stone-900/80 border border-purple-500/30 text-purple-300 hover:text-purple-100 hover:border-purple-400 text-xs cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {isPreviewing
                    ? <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    : <Volume2 className="w-3.5 h-3.5" />
                  }
                  <span>{isPreviewing ? 'Playing…' : 'Preview'}</span>
                </button>
              )}
            </div>

            <textarea
              value={markdown}
              onChange={(e) => setMarkdown(e.target.value)}
              className="w-full flex-1 min-h-[360px] bg-black/50 border border-white/10 rounded-xl p-3.5 font-mono text-xs text-stone-200 focus:outline-none focus:border-purple-500/80 shadow-inner resize-none leading-relaxed"
            />

            <TurnHistoryList turns={entity.history} />

            {entity.backlinks && entity.backlinks.length > 0 && (
              <div className="pt-3 border-t border-white/10">
                <span className="text-xs font-sans text-stone-400 block mb-2">Referenced By (Backlinks):</span>
                <div className="flex flex-wrap gap-1.5">
                  {entity.backlinks.map((link) => (
                    <button
                      key={link}
                      onClick={() => onSelect(link)}
                      className="text-xs bg-black/40 border border-white/10 px-2 py-0.5 rounded-md text-purple-400 font-mono hover:border-purple-500/40 cursor-pointer transition-colors"
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

      {isMergeOpen && entity && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm anim-fade-in">
          <div className="relative w-full max-w-md bg-stone-900 border border-red-500/40 rounded-2xl p-6 shadow-2xl space-y-5">
            <div className="flex items-center justify-between border-b border-stone-800 pb-3">
              <div className="flex items-center gap-2">
                <GitMerge className="w-5 h-5 text-red-400" />
                <h3 className="font-sans text-base font-bold text-red-300">Merge Note</h3>
              </div>
              <button
                onClick={() => setIsMergeOpen(false)}
                className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors cursor-pointer"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <p className="text-xs text-stone-300 leading-relaxed">
              <span className="font-bold text-purple-300">{entity.name}</span> will be folded into the note you choose.
              Its prose, tags, aliases, and turn history move across, and{' '}
              <span className="font-bold text-red-300">{entity.name}</span> is then deleted.
            </p>

            <div className="space-y-1.5">
              <label className="text-xs font-sans text-stone-400 uppercase tracking-wider block">Merge into</label>
              <select
                value={mergeTarget}
                onChange={(e) => setMergeTarget(e.target.value)}
                className="w-full bg-stone-950 border border-stone-700 rounded-lg px-2.5 py-2 text-xs font-mono text-stone-200 focus:outline-none"
              >
                <option value="" disabled>Choose a note…</option>
                {mergeCandidates.map((candidate) => (
                  <option key={candidate.id} value={candidate.id}>
                    {candidate.name}
                  </option>
                ))}
              </select>
            </div>

            <div className="flex items-center justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setIsMergeOpen(false)}
                className="px-3 py-1.5 rounded-xl border border-stone-700 text-stone-400 hover:bg-stone-800 text-xs font-sans cursor-pointer transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={!mergeTarget}
                onClick={() => {
                  if (!mergeTarget) return;
                  onMerge?.(entity.id, mergeTarget);
                  setIsMergeOpen(false);
                }}
                className="px-3.5 py-1.5 rounded-xl bg-red-700 hover:bg-red-600 disabled:opacity-50 disabled:cursor-not-allowed text-white text-xs font-sans font-bold shadow-lg cursor-pointer transition-colors"
              >
                Merge and delete "{entity.name}"
              </button>
            </div>
          </div>
        </div>
      )}
      {lightbox && (
        <ImageLightbox isOpen={isLightboxOpen} src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}
    </div>
  );
};
