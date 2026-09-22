import React, { useMemo, useState, useEffect } from 'react';
import { EntityNote, EntitySummary } from '../types';
import { Save, Volume2, Search, BookOpen } from 'lucide-react';
import { DEFAULT_VOICE_PROFILES } from '../templates/providerPresets';
import { TurnHistoryList } from './TurnHistoryList';

interface CodexDrawerProps {
  entity?: EntityNote;
  entities?: EntitySummary[];
  onSelect: (entityId: string) => void;
  onSave: (entityId: string, markdown: string) => void;
}

export const CodexDrawer: React.FC<CodexDrawerProps> = ({ entity, entities, onSelect, onSave }) => {
  const [markdown, setMarkdown] = useState('');
  const [query, setQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState('all');

  useEffect(() => {
    if (entity) setMarkdown(entity.markdown);
  }, [entity]);

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
    const profile = DEFAULT_VOICE_PROFILES.find((p) => p.id === profileId);
    if (!profile) return;

    const voiceSnippet = `voice:\n  voice_id: "${profile.voice_id}"\n  pitch: ${profile.pitch}\n  speech_rate: ${profile.speech_rate}`;
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

  return (
    <div className="flex flex-col md:flex-row gap-4 h-full min-h-0">
      {/* Corpus browser: every note in the campaign, not just the ones a wikilink led to */}
      <aside className="md:w-64 shrink-0 flex flex-col gap-2 min-h-0">
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-stone-500" />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search the codex..."
            className="w-full bg-black/50 border border-white/10 rounded-lg pl-8 pr-2 py-1.5 text-xs text-stone-200 placeholder-stone-500 focus:outline-none focus:border-amber-500/60"
          />
        </div>

        <div className="flex flex-wrap gap-1">
          <button
            onClick={() => setTypeFilter('all')}
            className={`text-[10px] font-mono px-2 py-0.5 rounded border transition-colors cursor-pointer ${
              typeFilter === 'all'
                ? 'bg-amber-600/30 border-amber-500/50 text-amber-200'
                : 'bg-black/40 border-white/10 text-stone-400 hover:text-stone-200'
            }`}
          >
            all ({entities?.length ?? 0})
          </button>
          {types.map(([type, count]) => (
            <button
              key={type}
              onClick={() => setTypeFilter(type)}
              className={`text-[10px] font-mono px-2 py-0.5 rounded border transition-colors cursor-pointer ${
                typeFilter === type
                  ? 'bg-amber-600/30 border-amber-500/50 text-amber-200'
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
                onClick={() => onSelect(candidate.id)}
                className={`w-full text-left px-2 py-1.5 rounded-lg border transition-colors cursor-pointer ${
                  entity?.id === candidate.id
                    ? 'bg-amber-600/20 border-amber-500/40'
                    : 'bg-black/30 border-white/5 hover:border-amber-500/30'
                }`}
              >
                <div className="text-xs text-stone-200 truncate">{candidate.name}</div>
                <div className="text-[10px] font-mono text-stone-500 truncate">
                  {candidate.type || 'note'}
                  {candidate.location ? ` · ${candidate.location.replace(/\[\[|\]\]/g, '')}` : ''}
                </div>
              </button>
            ))
          )}
        </div>
      </aside>

      {/* Note editor */}
      <div className="flex-1 min-w-0 min-h-0">
        {!entity ? (
          <div className="h-full flex flex-col items-center justify-center text-center gap-2 text-stone-500 italic p-6">
            <BookOpen className="w-8 h-8 text-amber-500/40" />
            <span className="text-sm">Choose a note from the codex to read or edit it.</span>
          </div>
        ) : (
          <div className="space-y-4 flex flex-col h-full">
            <div className="flex items-center justify-between border-b border-white/10 pb-3">
              <div>
                <h2 className="text-xl font-cinzel text-amber-400 font-bold">{entity.name}</h2>
                <span className="text-xs font-mono uppercase text-stone-400">{entity.type}</span>
              </div>
              <button
                onClick={() => onSave(entity.id, markdown)}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-amber-600 hover:bg-amber-500 text-stone-950 font-cinzel font-bold text-xs shadow-md transition-all cursor-pointer"
              >
                <Save className="w-3.5 h-3.5" />
                <span>Save</span>
              </button>
            </div>

            <div className="flex items-center justify-between px-1">
              <label className="flex items-center gap-1.5 text-xs text-stone-400 font-cinzel">
                <Volume2 className="w-3.5 h-3.5 text-amber-400" />
                <span>Apply Voice Archetype:</span>
              </label>
              <select
                onChange={(e) => {
                  if (e.target.value) {
                    applyVoiceArchetype(e.target.value);
                    e.target.value = '';
                  }
                }}
                className="bg-stone-900 border border-amber-500/30 rounded-lg pl-2.5 pr-8 py-1 text-xs font-mono text-amber-300 focus:outline-none cursor-pointer"
                defaultValue=""
              >
                <option value="" disabled>Select Archetype...</option>
                {DEFAULT_VOICE_PROFILES.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} ({p.voice_id})
                  </option>
                ))}
              </select>
            </div>

            <textarea
              value={markdown}
              onChange={(e) => setMarkdown(e.target.value)}
              className="w-full flex-1 min-h-[320px] bg-black/50 border border-white/10 rounded-xl p-3 font-mono text-xs text-stone-200 focus:outline-none focus:border-amber-500/80 shadow-inner"
            />

            <TurnHistoryList turns={entity.history} />

            {entity.backlinks && entity.backlinks.length > 0 && (
              <div className="pt-3 border-t border-white/10">
                <span className="text-xs font-cinzel text-stone-400 block mb-2">Referenced By (Backlinks):</span>
                <div className="flex flex-wrap gap-1.5">
                  {entity.backlinks.map((link) => (
                    <button
                      key={link}
                      onClick={() => onSelect(link)}
                      className="text-xs bg-black/40 border border-white/10 px-2 py-0.5 rounded-md text-amber-400 font-mono hover:border-amber-500/40 cursor-pointer transition-colors"
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
    </div>
  );
};
