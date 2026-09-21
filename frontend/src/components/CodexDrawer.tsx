import React, { useState, useEffect } from 'react';
import { EntityNote } from '../types';
import { Save, Volume2 } from 'lucide-react';
import { DEFAULT_VOICE_PROFILES } from '../templates/providerPresets';
import { TurnHistoryList } from './TurnHistoryList';

interface CodexDrawerProps {
  entity?: EntityNote;
  onSave: (entityId: string, markdown: string) => void;
}

export const CodexDrawer: React.FC<CodexDrawerProps> = ({ entity, onSave }) => {
  const [markdown, setMarkdown] = useState('');

  useEffect(() => {
    if (entity) setMarkdown(entity.markdown);
  }, [entity]);

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

  if (!entity) {
    return <div className="p-6 text-stone-500 italic">Select an entity or [[wikilink]] to view notes.</div>;
  }

  return (
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
          className="bg-black/50 border border-white/10 rounded-lg px-2.5 py-1 text-xs font-mono text-amber-300 focus:outline-none cursor-pointer"
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
              <span key={link} className="text-xs bg-black/40 border border-white/10 px-2 py-0.5 rounded-md text-amber-400 font-mono">
                [[{link}]]
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};
